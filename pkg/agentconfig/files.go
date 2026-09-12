package agentconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio"
)

// FileStatus is the per-owned-item status reported by plan/status.
type FileStatus string

const (
	StatusMissing     FileStatus = "missing"
	StatusInstalled   FileStatus = "installed"
	StatusDrifted     FileStatus = "drifted"
	StatusConflict    FileStatus = "conflict"
	StatusUnsupported FileStatus = "unsupported"
)

// FileAction describes the planned mutation for one file.
type FileAction string

const (
	ActionNoChange FileAction = "no_change"
	ActionCreate   FileAction = "create"
	ActionMerge    FileAction = "merge"
	ActionReplace  FileAction = "replace"
	ActionRemove   FileAction = "remove"
)

// hashShort returns a stable redacted hash of bytes (12 hex chars). It is the
// only representation of file content allowed in public output, so literal
// values never leak through plan/status envelopes.
func hashShort(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:12]
}

// writeFileAtomic replaces path atomically: temp file in the same directory,
// fsync, then rename. Parent directories are created.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	pf, err := renameio.TempFile(filepath.Dir(path), path)
	if err != nil {
		return err
	}
	defer pf.Cleanup()
	if _, err := pf.Write(data); err != nil {
		return err
	}
	if err := pf.Chmod(0o644); err != nil {
		return err
	}
	return pf.CloseAtomicallyReplace()
}

// removeOwnedFile deletes a whole-file owned path only after ownership has
// already been verified by the caller.
func removeOwnedFile(path string) error {
	return os.Remove(path)
}

// within reports whether resolved path p is equal to or inside root.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	// Any ".." component means p escaped root.
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// safeJoin resolves a repository-relative adapter path under root, rejecting
// absolute paths, ".." escapes and symlink escapes. It returns the resolved
// absolute path.
func safeJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", errUnsafePath("empty adapter path")
	}
	if filepath.IsAbs(rel) {
		return "", errUnsafePath("absolute adapter path not allowed: " + rel)
	}
	clean := filepath.Clean(rel)
	for _, seg := range strings.Split(filepath.ToSlash(clean), "/") {
		if seg == ".." {
			return "", errUnsafePath("adapter path escapes root: " + rel)
		}
	}

	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootReal = root
	}

	cur := rootReal
	for i, seg := range strings.Split(filepath.ToSlash(clean), "/") {
		if seg == "" || seg == "." {
			continue
		}
		next := filepath.Join(cur, seg)
		fi, lerr := os.Lstat(next)
		if lerr != nil {
			// next does not exist: append the remaining segments verbatim.
			rest := strings.Join(append([]string{next}, splitRel(clean, i+1)...), "/")
			if !within(rootReal, rest) {
				return "", errUnsafePath("adapter path escapes root: " + rel)
			}
			return rest, nil
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			resolved, rerr := filepath.EvalSymlinks(next)
			if rerr != nil || !within(rootReal, resolved) {
				return "", errUnsafePath("symlink escape not allowed: " + rel)
			}
			cur = resolved
			continue
		}
		cur = next
	}
	if !within(rootReal, cur) {
		return "", errUnsafePath("adapter path escapes root: " + rel)
	}
	return cur, nil
}

// splitRel returns the slash-separated segments of clean from index start.
func splitRel(clean string, start int) []string {
	segs := strings.Split(filepath.ToSlash(clean), "/")
	if start >= len(segs) {
		return nil
	}
	return segs[start:]
}
