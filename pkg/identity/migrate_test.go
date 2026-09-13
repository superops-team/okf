package identity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/parser"
)

func writeConceptFile(t *testing.T, root, rel, frontmatterExtra string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ntype: source\ntitle: " + filepath.Base(rel) + "\n" + frontmatterExtra + "---\nbody\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// S06: dry-run is deterministic, consumes no randomness, prints no proposed
// ids, leaves files byte-identical, and repeated output is byte-identical.
func TestIdentityEnsureDryRunDeterministic(t *testing.T) {
	root := t.TempDir()
	writeConceptFile(t, root, "a.md", "")
	writeConceptFile(t, root, "b.md", "")
	writeConceptFile(t, root, "stable.md", "okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e\n")

	beforeA, _ := os.ReadFile(filepath.Join(root, "a.md"))

	first, err := Ensure(root, false)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	second, err := Ensure(root, false)
	if err != nil {
		t.Fatalf("second dry-run: %v", err)
	}
	j1, _ := json.Marshal(first)
	j2, _ := json.Marshal(second)
	if string(j1) != string(j2) {
		t.Fatalf("dry-run not byte-identical:\n%s\n%s", j1, j2)
	}
	if first.Missing != 2 || first.Existing != 1 {
		t.Fatalf("want missing=2 existing=1, got %+v", first)
	}
	if len(first.Changes) != 2 {
		t.Fatalf("want 2 planned changes, got %d", len(first.Changes))
	}
	// No proposed id printed in the report.
	if strings.Contains(string(j1), "okf_") {
		t.Fatalf("dry-run must not print proposed ids: %s", j1)
	}
	// Files unchanged.
	afterA, _ := os.ReadFile(filepath.Join(root, "a.md"))
	if string(beforeA) != string(afterA) {
		t.Fatalf("dry-run modified a.md")
	}
}

// S07: apply is idempotent — second run reports zero changes.
func TestIdentityEnsureIdempotent(t *testing.T) {
	root := t.TempDir()
	writeConceptFile(t, root, "a.md", "")
	writeConceptFile(t, root, "stable.md", "okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e\n")

	first, err := Ensure(root, true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if first.Missing != 1 {
		t.Fatalf("first apply missing=%d, want 1", first.Missing)
	}
	idBefore := readFileID(t, filepath.Join(root, "a.md"))
	if _, err := Parse(idBefore); err != nil {
		t.Fatalf("first apply did not mint valid id: %v", err)
	}

	second, err := Ensure(root, true)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if second.Missing != 0 || len(second.Changes) != 0 {
		t.Fatalf("second apply must report zero changes, got %+v", second)
	}
	idAfter := readFileID(t, filepath.Join(root, "a.md"))
	if idBefore != idAfter {
		t.Fatalf("apply mutated existing id: %q -> %q", idBefore, idAfter)
	}
}

// S08: invalid or duplicate explicit ids fail before the first write.
func TestIdentityEnsurePreflight(t *testing.T) {
	t.Run("duplicate", func(t *testing.T) {
		root := t.TempDir()
		writeConceptFile(t, root, "a.md", "okf_id: okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
		writeConceptFile(t, root, "b.md", "okf_id: okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
		before, _ := os.ReadFile(filepath.Join(root, "a.md"))
		_, err := Ensure(root, true)
		if !errors.Is(err, ErrDuplicateConceptID) {
			t.Fatalf("expected duplicate_concept_id, got %v", err)
		}
		after, _ := os.ReadFile(filepath.Join(root, "a.md"))
		if string(before) != string(after) {
			t.Fatalf("preflight duplicate check modified a file")
		}
	})
	t.Run("invalid", func(t *testing.T) {
		root := t.TempDir()
		writeConceptFile(t, root, "a.md", "")
		writeConceptFile(t, root, "bad.md", "okf_id: OKF_UPPERCASE\n")
		_, err := Ensure(root, true)
		if !errors.Is(err, ErrInvalidConceptID) {
			t.Fatalf("expected invalid_concept_id, got %v", err)
		}
		if _, err := readFileIDValid(filepath.Join(root, "a.md")); err == nil {
			t.Fatalf("invalid preflight must not have written a.md")
		}
	})
}

// S09: an injected rename failure after one replacement rolls the first file
// back to its original bytes.
func TestIdentityEnsureRollback(t *testing.T) {
	root := t.TempDir()
	writeConceptFile(t, root, "a.md", "")
	writeConceptFile(t, root, "b.md", "")
	origA, _ := os.ReadFile(filepath.Join(root, "a.md"))
	origB, _ := os.ReadFile(filepath.Join(root, "b.md"))

	// Fail the second planned replace (b.md) but allow the rollback write of
	// a.md to succeed, so we observe a clean in-memory rollback.
	env := realMigrationEnv
	env.writeAtomic = func(path string, data []byte) error {
		if strings.HasSuffix(path, "b.md") {
			return errors.New("injected rename failure")
		}
		return realMigrationEnv.writeAtomic(path, data)
	}

	_, err := ensureWithEnv(root, true, env)
	if err == nil {
		t.Fatalf("expected injected failure")
	}
	// a.md was replaced (call 1) then rolled back to original bytes.
	restoredA, _ := os.ReadFile(filepath.Join(root, "a.md"))
	if string(restoredA) != string(origA) {
		t.Fatalf("a.md not rolled back to original bytes")
	}
	restoredB, _ := os.ReadFile(filepath.Join(root, "b.md"))
	if string(restoredB) != string(origB) {
		t.Fatalf("b.md unexpectedly changed")
	}
}

// S10: paths that escape the root are rejected before writing.
func TestIdentityEnsureUnsafePath(t *testing.T) {
	root := t.TempDir()
	writeConceptFile(t, root, "ok.md", "")
	// A symlink that points outside the root.
	outside := filepath.Join(t.TempDir(), "escape.md")
	_ = os.WriteFile(outside, []byte("---\ntype: source\n---\nbody\n"), 0o644)
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := Ensure(root, true)
	if err == nil {
		t.Fatalf("expected unsafe-path rejection, got nil error")
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("outside file touched: %v", statErr)
	}
}

func readFileID(t *testing.T, path string) string {
	t.Helper()
	pc, err := parser.ParseConcept(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := pc.CustomFields[Field].(string)
	return raw
}

func readFileIDValid(path string) (string, error) {
	pc, err := parser.ParseConcept(path)
	if err != nil {
		return "", err
	}
	raw, _ := pc.CustomFields[Field].(string)
	return Parse(raw)
}

// M2 (second-round review lock-in): a dry-run over a root that contains a
// symlink escaping the real root surfaces the escape as an Unsafe warning. It
// must NOT error, must NOT count the unsafe path as missing, and must only plan
// the safe legacy file as an add.
func TestIdentityEnsureDryRunUnsafeReported(t *testing.T) {
	root := t.TempDir()
	writeConceptFile(t, root, "ok.md", "")
	// Symlink pointing outside the root to an existing (readable) file.
	outside := filepath.Join(t.TempDir(), "escape.md")
	_ = os.WriteFile(outside, []byte("---\ntype: source\ntitle: outside\n---\nbody\n"), 0o644)
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	rep, err := Ensure(root, false)
	if err != nil {
		t.Fatalf("dry-run must not error on unsafe paths: %v", err)
	}
	if len(rep.Unsafe) != 1 {
		t.Fatalf("want 1 unsafe path, got %+v", rep.Unsafe)
	}
	if !strings.Contains(rep.Unsafe[0], "link.md") {
		t.Fatalf("unsafe path %q does not reference link.md", rep.Unsafe[0])
	}
	if rep.Missing != 1 {
		t.Fatalf("want missing=1 (safe ok.md only), got %d", rep.Missing)
	}
	if len(rep.Changes) != 1 || rep.Changes[0].Path != "ok.md" || rep.Changes[0].Action != "add" {
		t.Fatalf("changes must only contain ok.md add, got %+v", rep.Changes)
	}
}

// M3 (second-round review lock-in): a dangling symlink (target outside the root
// and non-existent) fails closed under apply. Ensure returns an error and the
// safe sibling file is never written.
func TestIdentityEnsureDanglingSymlinkFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeConceptFile(t, root, "ok.md", "")
	before, _ := os.ReadFile(filepath.Join(root, "ok.md"))

	link := filepath.Join(root, "link.md")
	if err := os.Symlink("/nonexistent/target.md", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	rep, err := Ensure(root, true)
	if err == nil {
		t.Fatalf("expected dangling-symlink rejection, got nil error; report=%+v", rep)
	}
	after, _ := os.ReadFile(filepath.Join(root, "ok.md"))
	if string(before) != string(after) {
		t.Fatalf("ok.md must not be written when an unsafe path is present")
	}
}

// L1 (second-round review lock-in): duplicate explicit ids are reported in
// ascending OKFID order regardless of on-disk file order, and repeated dry-run
// serialization is byte-identical.
func TestIdentityEnsureDuplicatesSortedDeterministic(t *testing.T) {
	root := t.TempDir()
	idA := "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB := "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	// Deliberately non-alphabetical file layout and id assignment order.
	writeConceptFile(t, root, "z.md", "okf_id: "+idB+"\n")
	writeConceptFile(t, root, "a.md", "okf_id: "+idA+"\n")
	writeConceptFile(t, root, "m.md", "okf_id: "+idB+"\n")
	writeConceptFile(t, root, "c.md", "okf_id: "+idA+"\n")

	first, err := Ensure(root, false)
	if err == nil {
		t.Fatal("expected duplicate preflight error")
	}
	if len(first.Duplicates) != 2 {
		t.Fatalf("want 2 duplicate groups, got %+v", first.Duplicates)
	}
	if !(first.Duplicates[0].OKFID < first.Duplicates[1].OKFID) {
		t.Fatalf("duplicates not ascending: %q then %q",
			first.Duplicates[0].OKFID, first.Duplicates[1].OKFID)
	}
	for _, d := range first.Duplicates {
		if len(d.Paths) != 2 {
			t.Fatalf("duplicate %s has paths %v, want 2", d.OKFID, d.Paths)
		}
	}

	j1, _ := json.Marshal(first)
	second, _ := Ensure(root, false)
	j2, _ := json.Marshal(second)
	if string(j1) != string(j2) {
		t.Fatalf("duplicate dry-run not byte-identical:\n%s\n%s", j1, j2)
	}
}

// E3: derived chunks (doc__cN.md) must receive parent_okf_id pointing to the
// parent concept's okf_id after identity ensure. Without this, concept-level
// grouping cannot trace chunks back to their parent.
func TestIdentityEnsureDerivedChunksGetParentOKFID(t *testing.T) {
	root := t.TempDir()
	// Parent concept (no id yet).
	writeConceptFile(t, root, "doc.md", "")
	// Two derived chunks (no id, no parent_okf_id yet).
	writeConceptFile(t, root, "doc__c1.md", "derived: \"true\"\n")
	writeConceptFile(t, root, "doc__c2.md", "derived: \"true\"\n")

	if _, err := Ensure(root, true); err != nil {
		t.Fatalf("apply: %v", err)
	}

	parentID := readFileID(t, filepath.Join(root, "doc.md"))
	if parentID == "" {
		t.Fatal("parent did not receive okf_id")
	}

	for _, chunk := range []string{"doc__c1.md", "doc__c2.md"} {
		data, err := os.ReadFile(filepath.Join(root, chunk))
		if err != nil {
			t.Fatalf("read %s: %v", chunk, err)
		}
		// Parse frontmatter to check parent_okf_id.
		c, err := parser.ParseConceptBytes(chunk, data)
		if err != nil {
			t.Fatalf("parse %s: %v", chunk, err)
		}
		pid, _ := c.CustomFields[ParentField].(string)
		if pid != parentID {
			t.Errorf("%s parent_okf_id = %q, want %q", chunk, pid, parentID)
		}
	}

	// Idempotent: second apply must not change parent_okf_id.
	if _, err := Ensure(root, true); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	for _, chunk := range []string{"doc__c1.md", "doc__c2.md"} {
		data, _ := os.ReadFile(filepath.Join(root, chunk))
		c, _ := parser.ParseConceptBytes(chunk, data)
		pid, _ := c.CustomFields[ParentField].(string)
		if pid != parentID {
			t.Errorf("%s parent_okf_id changed after second apply: %q, want %q", chunk, pid, parentID)
		}
	}
}
