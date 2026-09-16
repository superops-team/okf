package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/manifest"
)

// S17 (service layer): --stale-refs scans the repo FS and annotates items.
func TestServiceManifestStaleRefsAnnotatesItems(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "gone.md"), `---
type: decision
title: Gone
code_refs:
  - deleted/file.go
  - pkg/kept/a.go
---
Body.
`)
	mustWriteToolFile(t, filepath.Join(repo, "pkg", "kept", "a.go"), "package kept\n")

	svc := NewService(Config{RepoPath: repo})
	env := svc.Manifest(context.Background(), ManifestRequest{StaleRefs: true})
	if !env.OK {
		t.Fatalf("manifest: %v", env.Error)
	}
	result := mustManifestResult(t, env)
	var item *manifest.ManifestItem
	for i := range result.Items {
		if result.Items[i].Title == "Gone" {
			item = &result.Items[i]
		}
	}
	if item == nil {
		t.Fatalf("Gone missing: %+v", result.Items)
	}
	if len(item.StaleCodeRefs) != 1 || item.StaleCodeRefs[0] != "deleted/file.go" {
		t.Fatalf("stale_code_refs = %v", item.StaleCodeRefs)
	}
	if result.Incomplete {
		t.Fatal("clean repo scan must not be incomplete")
	}
}

// S17 (fail-closed): a symlink escaping the repo root flags incomplete + warnings.
func TestServiceManifestStaleRefsSymlinkEscape(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "g.md"), `---
type: decision
title: G
code_refs:
  - pkg/x.go
---
Body.
`)
	outside := t.TempDir()
	link := filepath.Join(repo, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	defer os.Remove(link)

	svc := NewService(Config{RepoPath: repo})
	env := svc.Manifest(context.Background(), ManifestRequest{StaleRefs: true})
	if !env.OK {
		t.Fatalf("manifest: %v", env.Error)
	}
	result := mustManifestResult(t, env)
	if !result.Incomplete {
		t.Fatal("symlink escape must set incomplete=true")
	}
	if len(result.ScanWarnings) == 0 {
		t.Fatal("symlink escape must surface scan warnings")
	}
}

// S06/S08 through the Service: governance filter + sort round-trips.
func TestServiceManifestGovernanceFilterThroughService(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "h.md"), `---
type: decision
title: H
governance: hold
code_refs:
  - pkg/x.go
---
H body.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "c.md"), `---
type: decision
title: C
governance: constraint
code_refs:
  - pkg/x.go
---
C body.
`)
	svc := NewService(Config{RepoPath: repo})
	env := svc.Manifest(context.Background(), ManifestRequest{ForPath: "pkg/x.go"})
	if !env.OK {
		t.Fatalf("manifest: %v", env.Error)
	}
	result := mustManifestResult(t, env)
	if !result.GovernanceWarning {
		t.Fatal("hold present → governance_warning")
	}
	if result.Items[0].Governance != "hold" {
		t.Fatalf("hold must sort first: %+v", result.Items)
	}
}

// S35 (P3): okf_context --refs reads the concept body, budget-bounded.
func TestServiceContextRefsReadsConceptBody(t *testing.T) {
	repo := initToolTestRepo(t)
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "body.md"), `---
type: note
title: Body Concept
okf_id: `+id+`
---
This is the concept body text for ref resolution.
`)
	svc := NewService(Config{RepoPath: repo})
	env := svc.Context(context.Background(), ContextRequest{Refs: []string{id}, BudgetTokens: 4000})
	if !env.OK {
		t.Fatalf("context: %v", env.Error)
	}
	cr := env.Result.(ContextResult)
	if len(cr.Items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(cr.Items), cr.Items)
	}
	if !strings.Contains(cr.Items[0].Snippet, "concept body text") {
		t.Fatalf("snippet = %q", cr.Items[0].Snippet)
	}
}

// S35: query OR refs at least one required.
func TestServiceContextRequiresQueryOrRefs(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "body.md"), `---
type: note
title: Body
---
body text
`)
	svc := NewService(Config{RepoPath: repo})
	env := svc.Context(context.Background(), ContextRequest{})
	if env.OK || env.Error == nil || env.Error.Code != ErrInvalidQuery {
		t.Fatalf("empty query+refs must fail: %+v", env)
	}
}

// S35: unknown ref becomes an omission/warning, not a crash.
func TestServiceContextUnknownRefOmitted(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "body.md"), `---
type: note
title: Body
---
body text
`)
	svc := NewService(Config{RepoPath: repo})
	env := svc.Context(context.Background(), ContextRequest{
		Refs: []string{"okf_00000000000000000000000000000000"},
	})
	if !env.OK {
		t.Fatalf("unknown ref must be advisory, not fatal: %v", env.Error)
	}
	cr := env.Result.(ContextResult)
	if len(cr.Omissions) == 0 {
		t.Fatal("unknown ref must produce an omission")
	}
}

func mustManifestResult(t *testing.T, env ToolEnvelope) *manifest.ManifestResult {
	t.Helper()
	result, ok := env.Result.(*manifest.ManifestResult)
	if !ok {
		t.Fatalf("result type = %T", env.Result)
	}
	return result
}
