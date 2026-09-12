package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/superops-team/okf/pkg/identity"
)

// S11: the CLI entry `okf identity resolve --ref <uri>` resolves a stable ref
// to the concept's current path even after the file is moved.
func TestCmdIdentityResolveAfterMove(t *testing.T) {
	repo := initIdentityCLITestRepo(t)

	const okfID = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	oldRel := "notes/original.md"
	newRel := "notes/renamed.md"
	writeIdentityKBConcept(t, repo, oldRel, "Original Title", okfID)
	uri := identity.CanonicalURI(okfID)

	resolve := func(ref string) (int, map[string]any) {
		code := 0
		var parsed map[string]any
		out := captureStdout(t, func() {
			code = cmdIdentity([]string{"resolve", "--ref", ref, "--repo", repo, "--json"})
		})
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("resolve output not valid JSON: %v\n%s", err, out)
		}
		return code, parsed
	}

	// Before move.
	code, env := resolve(uri)
	if code != 0 {
		t.Fatalf("resolve before move exit = %d, env = %v", code, env)
	}
	if got := resultPath(t, env); got != oldRel {
		t.Fatalf("before move: path = %q, want %q", got, oldRel)
	}

	// Move the file on disk.
	kbDir := filepath.Join(repo, ".okf", "knowledge")
	if err := os.Rename(filepath.Join(kbDir, filepath.FromSlash(oldRel)), filepath.Join(kbDir, filepath.FromSlash(newRel))); err != nil {
		t.Fatal(err)
	}
	writeIdentityKBConcept(t, repo, newRel, "Renamed Title", okfID)

	// After move: same ref, new path.
	code, env = resolve(uri)
	if code != 0 {
		t.Fatalf("resolve after move exit = %d, env = %v", code, env)
	}
	if got := resultPath(t, env); got != newRel {
		t.Fatalf("after move: path = %q, want %q", got, newRel)
	}

	// Unknown valid-syntax ref -> concept_ref_not_found, non-zero exit.
	code, env = resolve("okf://concept/okf_ffffffffffffffffffffffffffffffff")
	if code == 0 {
		t.Fatal("unknown ref should exit non-zero")
	}
	errObj, _ := env["error"].(map[string]any)
	if codeVal, _ := errObj["code"].(string); codeVal != "concept_ref_not_found" {
		t.Fatalf("unknown ref error code = %v, want concept_ref_not_found", errObj)
	}
}

func initIdentityCLITestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	cmd := exec.Command("git", "init", "-q", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", repo, "config", "user.name", "Test").Run()
	return repo
}

func writeIdentityKBConcept(t *testing.T, repo, relPath, title, okfID string) {
	t.Helper()
	path := filepath.Join(repo, ".okf", "knowledge", filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ntype: note\ntitle: " + title + "\nokf_id: " + okfID + "\n---\nbody\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resultPath(t *testing.T, env map[string]any) string {
	t.Helper()
	result, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("envelope has no result object: %v", env)
	}
	path, _ := result["path"].(string)
	return path
}
