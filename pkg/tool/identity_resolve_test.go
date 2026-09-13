package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/superops-team/okf/pkg/identity"
)

// S11: a stable okf_id ref survives rename/move of its file. After moving the
// concept file (and changing its title), Service.Resolve must return the new
// bundle-relative path. A syntactically valid but unknown ref fails with
// concept_ref_not_found.
func TestStableRefSurvivesMove(t *testing.T) {
	repo := t.TempDir()
	runToolGit(t, repo, "init")
	runToolGit(t, repo, "config", "user.name", "Test User")
	runToolGit(t, repo, "config", "user.email", "test@example.com")

	const okfID = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	const oldRel = "notes/original.md"
	const newRel = "notes/renamed.md"

	writeKBConcept(t, repo, oldRel, "Original Title", okfID)

	svc := NewService(Config{RepoPath: repo})
	uri := identity.CanonicalURI(okfID)

	// Before move: resolves to the original path.
	resp := svc.Resolve(context.Background(), ResolveRequest{Ref: uri})
	if !resp.OK {
		t.Fatalf("resolve before move failed: %#v", resp.Error)
	}
	first := requireResolveResult(t, resp)
	if first.Path != oldRel {
		t.Fatalf("before move: path = %q, want %q", first.Path, oldRel)
	}

	// Move the file (and change its title) without touching okf_id.
	kbDir := filepath.Join(repo, ".okf", "knowledge")
	if err := os.Rename(filepath.Join(kbDir, filepath.FromSlash(oldRel)), filepath.Join(kbDir, filepath.FromSlash(newRel))); err != nil {
		t.Fatal(err)
	}
	writeKBConcept(t, repo, newRel, "Renamed Title", okfID)

	// After move: resolves to the new path via the stable id.
	resp = svc.Resolve(context.Background(), ResolveRequest{Ref: uri})
	if !resp.OK {
		t.Fatalf("resolve after move failed: %#v", resp.Error)
	}
	after := requireResolveResult(t, resp)
	if after.Path != newRel {
		t.Fatalf("after move: path = %q, want %q", after.Path, newRel)
	}
	if after.OKFID != okfID {
		t.Fatalf("okf_id = %q, want %q", after.OKFID, okfID)
	}

	// Unknown but syntactically valid ref: concept_ref_not_found.
	missing := svc.Resolve(context.Background(), ResolveRequest{Ref: "okf://concept/okf_ffffffffffffffffffffffffffffffff"})
	if missing.OK {
		t.Fatal("unknown ref must not resolve")
	}
	if missing.Error == nil || missing.Error.Code != string(identity.CodeConceptRefNotFound) {
		t.Fatalf("unknown ref error = %#v, want code %q", missing.Error, identity.CodeConceptRefNotFound)
	}

	// Empty ref fails with invalid_request.
	empty := svc.Resolve(context.Background(), ResolveRequest{Ref: "  "})
	if empty.OK {
		t.Fatal("empty ref must not resolve")
	}
}

// writeKBConcept writes a minimal concept file carrying a canonical okf_id into
// the repo's .okf/knowledge directory.
func writeKBConcept(t *testing.T, repo, relPath, title, okfID string) {
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

func requireResolveResult(t *testing.T, resp ToolEnvelope) ResolveResult {
	t.Helper()
	if !resp.OK {
		t.Fatalf("envelope not OK: %#v", resp.Error)
	}
	r, ok := resp.Result.(ResolveResult)
	if !ok {
		t.Fatalf("result type = %T, want ResolveResult", resp.Result)
	}
	return r
}
