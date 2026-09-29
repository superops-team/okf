package tool

// P3 temporal context views (S17, S36) and the strict whole-bundle lint pass
// (T3.3).

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// S17: legacy context must not leak temporal annotations onto packed items.
func TestTemporalS17LegacyContextParity(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha Routing
source_path: .okf/knowledge/concepts/alpha.md
---
Alpha routing body content used for context.
`)
	svc := NewService(Config{RepoPath: repo})

	for _, view := range []string{"", "current", "all"} {
		resp := svc.Context(context.Background(), ContextRequest{Query: "routing", MemoryView: view})
		if !resp.OK {
			t.Fatalf("view=%q context OK=false, error=%#v", view, resp.Error)
		}
		result := resp.Result.(ContextResult)
		if len(result.Items) == 0 {
			t.Fatalf("view=%q expected at least one context item", view)
		}
		for _, it := range result.Items {
			if it.MemoryState != "" || it.MemoryCurrent != nil {
				t.Fatalf("view=%q legacy item leaked annotations: %+v", view, it)
			}
		}
	}
}

// itemFile reports the base filename a context item was packed from.
func itemFile(it ContextItem) string { return filepath.Base(it.SourcePath) }

// itemHasFile reports whether any item was packed from the given file name.
func itemHasFile(items []ContextItem, file string) bool {
	for _, it := range items {
		if itemFile(it) == file {
			return true
		}
	}
	return false
}

// S36: context current/all projection and explicit-ref survival.
func TestTemporalS36ContextViews(t *testing.T) {
	repo := initToolTestRepo(t)
	temporalChainBundle(t, repo)
	svc := NewService(Config{RepoPath: repo})

	// Current view: query-driven packing excludes the superseded old revision.
	cur := svc.Context(context.Background(), ContextRequest{
		Query: "routing", MemoryView: "current", BudgetTokens: 10000,
	})
	if !cur.OK {
		t.Fatalf("current context OK=false, error=%#v", cur.Error)
	}
	curItems := cur.Result.(ContextResult).Items
	if itemHasFile(curItems, "old.md") {
		t.Fatalf("current context must exclude superseded old revision, got %+v", curItems)
	}
	if !itemHasFile(curItems, "new.md") {
		t.Fatalf("current context must include the current head, got %+v", curItems)
	}

	// All view: query-driven packing keeps the old revision, annotated.
	all := svc.Context(context.Background(), ContextRequest{
		Query: "routing", MemoryView: "all", BudgetTokens: 10000,
	})
	if !all.OK {
		t.Fatalf("all context OK=false, error=%#v", all.Error)
	}
	allItems := all.Result.(ContextResult).Items
	if !itemHasFile(allItems, "old.md") {
		t.Fatalf("all context must keep the superseded old revision, got %+v", allItems)
	}

	// Explicit ref to the superseded concept MUST survive even in current view.
	ref := svc.Context(context.Background(), ContextRequest{
		Refs: []string{temporalOldID}, MemoryView: "current", BudgetTokens: 10000,
	})
	if !ref.OK {
		t.Fatalf("ref context OK=false, error=%#v", ref.Error)
	}
	refItems := ref.Result.(ContextResult).Items
	if len(refItems) != 1 {
		t.Fatalf("explicit ref items = %d, want 1, got %+v", len(refItems), refItems)
	}
	if refItems[0].MemoryState != "approved" || refItems[0].MemoryCurrent == nil || *refItems[0].MemoryCurrent != false {
		t.Fatalf("explicit ref item annotations = state:%q current:%v, want approved/false",
			refItems[0].MemoryState, refItems[0].MemoryCurrent)
	}

	// history is rejected by Context (it has its own Query mode).
	hist := svc.Context(context.Background(), ContextRequest{
		Query: "routing", MemoryView: "history",
	})
	if hist.OK || hist.Error == nil || hist.Error.Code != ErrInvalidRequest {
		t.Fatalf("context history: ok=%v code=%v, want %s", hist.OK, codeOf(hist), ErrInvalidRequest)
	}
}

// T3.3: strict whole-bundle temporal lint surfaces violations with stable codes
// and safe bundle-relative paths, and is silent on healthy concepts.
func TestTemporalT33StrictLint(t *testing.T) {
	bad := &okf.Concept{
		Type:     "note",
		FilePath: "concepts/proposed_bad.md",
		CustomFields: map[string]any{
			"okf_id":       temporalPropID,
			"memory_state": "proposed",
			// missing memory_confidence and provenance.evidence_refs -> strict violation.
		},
	}
	good := &okf.Concept{
		Type:     "note",
		FilePath: "concepts/approved.md",
		CustomFields: map[string]any{
			"okf_id": temporalNewID,
			// approved, no confidence -> healthy.
		},
	}

	issues := ValidateTemporalBundle([]*okf.Concept{bad, good})
	if len(issues) == 0 {
		t.Fatal("expected strict lint issues for malformed proposal, got none")
	}
	for _, is := range issues {
		if is.Code != "invalid_memory_state" {
			t.Fatalf("unexpected lint code %q: %s", is.Code, is.Message)
		}
		if is.Path != "concepts/proposed_bad.md" {
			t.Fatalf("issue path = %q, want safe bundle-relative path", is.Path)
		}
		if is.OKFID != temporalPropID {
			t.Fatalf("issue okf_id = %q, want %q", is.OKFID, temporalPropID)
		}
	}
}
