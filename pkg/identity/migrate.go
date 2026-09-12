package identity

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/superops-team/okf/pkg/parser"
)

// EnsureReport is the JSON contract of `okf identity ensure`. It is produced
// deterministically: dry-run never consumes randomness and never prints proposed
// ids, so repeated dry-run output is byte-identical.
type EnsureReport struct {
	Operation  string           `json:"operation"`
	Apply      bool             `json:"apply"`
	Missing    int              `json:"missing"`
	Existing   int              `json:"existing"`
	Invalid    []string         `json:"invalid"`
	Duplicates []DuplicateEntry `json:"duplicates"`
	Changes    []Change         `json:"changes"`
	// Unsafe lists bundle-relative paths whose write target escapes the real
	// root (symlink escape / unresolved boundary). They are never silently
	// listed as planned adds: dry-run reports them as warnings, apply fails
	// closed before writing anything.
	Unsafe                []string `json:"unsafe"`
	VectorRebuildRequired bool     `json:"vector_rebuild_required,omitempty"`
}

// Change describes one planned/planned-and-applied file action.
type Change struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// DuplicateEntry records the two bundle-relative paths claiming one okf_id.
type DuplicateEntry struct {
	OKFID string   `json:"okf_id"`
	Paths []string `json:"paths"`
}

// migrationEnv abstracts filesystem access so tests can inject failures.
type migrationEnv struct {
	readFile     func(path string) ([]byte, error)
	writeAtomic  func(path string, data []byte) error
	lstat        func(path string) (fs.FileInfo, error)
	readDir      func(root string) ([]fs.DirEntry, error)
	evalSymlinks func(path string) (string, error)
}

var realMigrationEnv = migrationEnv{
	readFile: os.ReadFile,
	writeAtomic: func(path string, data []byte) error {
		return atomicReplace(path, data)
	},
	lstat:        os.Lstat,
	readDir:      os.ReadDir,
	evalSymlinks: filepath.EvalSymlinks,
}

// planEntry is one concept file's migration decision.
type planEntry struct {
	absPath    string
	relPath    string
	explicitID string // "" means legacy/no id
	action     string // "add" | "" (keep)
	isSymlink  bool   // entry is itself a symlink (target may be dangling/outside)
}

// Ensure scans the knowledge root and plans (apply=false) or applies
// (apply=true) stable okf_id assignment. It fails closed on invalid or
// duplicate explicit ids and on unsafe paths before writing anything.
func Ensure(root string, apply bool) (*EnsureReport, error) {
	return ensureWithEnv(root, apply, realMigrationEnv)
}

func ensureWithEnv(root string, apply bool, env migrationEnv) (*EnsureReport, error) {
	root = filepath.Clean(root)
	report := &EnsureReport{Operation: "identity.ensure", Apply: apply, Invalid: []string{}, Duplicates: []DuplicateEntry{}, Changes: []Change{}, Unsafe: []string{}}

	// 1. Enumerate concept files.
	entries, err := collectConceptFiles(root, env)
	if err != nil {
		return report, err
	}

	// 2. Build the plan and validate explicit ids.
	plan := make([]planEntry, 0, len(entries))
	idToPaths := make(map[string][]string)
	for _, e := range entries {
		raw, parseErr := readExistingIDRaw(env, e.absPath)
		if parseErr != nil {
			if e.isSymlink {
				// A symlink whose target cannot be read (dangling, looped or
				// outside the boundary) must NOT be silently dropped from the
				// plan: collect it as a planned add so the unsafe-path preflight
				// resolves and rejects it fail-closed. Regular unreadable files
				// (bad YAML, permission) stay skipped — they are not a boundary.
				plan = append(plan, planEntry{absPath: e.absPath, relPath: e.relPath, action: "add", isSymlink: true})
			}
			continue
		}
		pe := planEntry{absPath: e.absPath, relPath: e.relPath, explicitID: raw, isSymlink: e.isSymlink}
		if raw == "" {
			pe.action = "add"
			plan = append(plan, pe)
			continue
		}
		if _, err := Parse(raw); err != nil {
			report.Invalid = append(report.Invalid, e.relPath)
			continue
		}
		idToPaths[raw] = append(idToPaths[raw], e.relPath)
		plan = append(plan, pe)
	}

	// 3. Duplicates fail closed. Iterate a sorted OKFID list so JSON output is
	//    deterministic (Go map iteration order is randomized).
	dupIDs := make([]string, 0, len(idToPaths))
	for id, paths := range idToPaths {
		if len(paths) > 1 {
			dupIDs = append(dupIDs, id)
		}
	}
	sort.Strings(dupIDs)
	for _, id := range dupIDs {
		paths := idToPaths[id]
		sort.Strings(paths)
		report.Duplicates = append(report.Duplicates, DuplicateEntry{OKFID: id, Paths: paths})
	}

	// 4. Preflight: any invalid or duplicate blocks before the first write.
	if len(report.Duplicates) > 0 {
		first := report.Duplicates[0]
		return report, ErrDuplicateConceptID.withPaths(first.Paths...).
			withMessage("identity ensure preflight: duplicate okf_id %s in %v", first.OKFID, first.Paths)
	}
	if len(report.Invalid) > 0 {
		return report, ErrInvalidConceptID.withMessage("identity ensure preflight: invalid okf_id in %v", report.Invalid)
	}

	// 5. Unsafe-path preflight over the WHOLE plan (S10: "fails before writing").
	//    rejectUnsafePath is read-only (Lstat/EvalSymlinks), so running it for
	//    every planned add before touching a single file means an unsafe target
	//    can never produce a transient write. plan is already sorted by relPath,
	//    so the collected list is deterministic.
	unsafeSet := make(map[string]struct{}, len(plan))
	var unsafeErr error
	for _, pe := range plan {
		if pe.action != "add" {
			continue
		}
		if err := rejectUnsafePath(root, pe.absPath, env); err != nil {
			unsafeSet[pe.relPath] = struct{}{}
			report.Unsafe = append(report.Unsafe, pe.relPath)
			if unsafeErr == nil {
				unsafeErr = err
			}
		}
	}

	// 6. Summarize. Unsafe paths are NOT counted as missing and NOT listed as
	//    planned adds: dry-run surfaces them as warnings instead.
	missing := 0
	for _, pe := range plan {
		if pe.action != "add" {
			continue
		}
		if _, bad := unsafeSet[pe.relPath]; bad {
			continue
		}
		missing++
		report.Changes = append(report.Changes, Change{Path: pe.relPath, Action: "add"})
	}
	report.Missing = missing
	report.Existing = len(plan) - missing
	if vectorIndexPresent(root, env) && missing > 0 {
		report.VectorRebuildRequired = true
	}

	if !apply {
		return report, nil
	}

	// 7. Apply preflight: any unsafe path blocks before the first write (zero
	//    writes; rollback is a no-op because nothing has been touched yet).
	if len(unsafeSet) > 0 {
		return report, fmt.Errorf("identity ensure preflight: refusing to write unsafe path(s) %v: %w",
			report.Unsafe, unsafeErr)
	}

	// 8. Apply: full preflight already passed. Atomic per-file replace with
	//    rollback from in-memory original bytes.
	written := []fileState{}
	for _, pe := range plan {
		if pe.action != "add" {
			continue
		}
		original, err := env.readFile(pe.absPath)
		if err != nil {
			return rollback(written, env, fmt.Errorf("read %s: %w", pe.relPath, err))
		}
		id, err := New()
		if err != nil {
			return rollback(written, env, err)
		}
		updated, err := injectID(pe.absPath, original, id)
		if err != nil {
			return rollback(written, env, fmt.Errorf("inject id into %s: %w", pe.relPath, err))
		}
		if err := env.writeAtomic(pe.absPath, updated); err != nil {
			return rollback(written, env, fmt.Errorf("write %s: %w", pe.relPath, err))
		}
		written = append(written, fileState{absPath: pe.absPath, rel: pe.relPath, original: original})
	}
	return report, nil
}

// fileState records an in-memory original byte snapshot for rollback.
type fileState struct {
	absPath  string
	rel      string
	original []byte
}

// rollback restores already-written files from their in-memory original bytes.
// A rollback failure yields identity_migration_partial with affected paths.
func rollback(written []fileState, env migrationEnv, cause error) (*EnsureReport, error) {
	if len(written) == 0 {
		return nil, cause
	}
	var failed []string
	for _, w := range written {
		if err := env.writeAtomic(w.absPath, w.original); err != nil {
			failed = append(failed, w.rel)
		}
	}
	if len(failed) > 0 {
		return nil, ErrIdentityMigrationPartial.withPaths(failed...).
			withMessage("identity migration partial: rollback failed for %v after: %v", failed, cause)
	}
	return nil, cause
}

// injectID re-serializes the concept at path with the given okf_id added.
func injectID(path string, original []byte, id string) ([]byte, error) {
	pc, err := parser.ParseConcept(path)
	if err != nil {
		return nil, err
	}
	if pc.CustomFields == nil {
		pc.CustomFields = map[string]any{}
	}
	pc.CustomFields[Field] = id
	return parser.SerializeConcept(pc, true)
}

// collectConceptFiles walks root for *.md files, returning absolute and
// bundle-relative (slash-normalized) paths.
func collectConceptFiles(root string, env migrationEnv) ([]planEntry, error) {
	var out []planEntry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir // skip .git, .okf internals
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		out = append(out, planEntry{
			absPath:   path,
			relPath:   filepath.ToSlash(rel),
			isSymlink: d.Type()&fs.ModeSymlink != 0,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].relPath < out[j].relPath })
	return out, nil
}

// readExistingIDRaw returns the explicit okf_id string in the file (possibly "").
func readExistingIDRaw(env migrationEnv, path string) (string, error) {
	pc, err := parser.ParseConcept(path)
	if err != nil {
		return "", err
	}
	raw, _ := pc.CustomFields[Field].(string)
	return raw, nil
}

// rejectUnsafePath ensures target stays within the real knowledge root (no
// symlink escape, no absolute/.. traversal). It fails closed: if either the
// root or the target cannot be resolved through symlinks, the boundary cannot
// be verified and the path is rejected rather than silently allowed.
func rejectUnsafePath(root, target string, env migrationEnv) error {
	cleanRel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("unsafe path %s: %w", target, err)
	}
	if cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || filepath.IsAbs(cleanRel) {
		return fmt.Errorf("path escapes knowledge root: %s", cleanRel)
	}
	rootResolved, err := env.evalSymlinks(root)
	if err != nil {
		return fmt.Errorf("unsafe path %s: cannot resolve knowledge root through symlinks: %w", target, err)
	}
	targetResolved, err := env.evalSymlinks(target)
	if err != nil {
		return fmt.Errorf("unsafe path %s: cannot resolve target through symlinks: %w", target, err)
	}
	rel, err := filepath.Rel(rootResolved, targetResolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes knowledge root via symlink: %s", target)
	}
	return nil
}

func vectorIndexPresent(root string, env migrationEnv) bool {
	meta := filepath.Join(root, ".okf", "vector", "index.meta.json")
	_, err := env.readFile(meta)
	return err == nil
}

// atomicReplace writes data to path via a temp file in the same directory,
// fsyncs it, and renames it into place (per-file atomic replace).
func atomicReplace(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".okf-id-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
