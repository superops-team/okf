package tool

// P3 temporal query views (S17, S32-S35). These tests pin the additive query
// surface: legacy byte parity, current/all projection, history envelope, review
// queue ordering, and the §7.1 mutual-exclusion matrix.

import (
	"context"
	"path/filepath"
	"testing"
)

const (
	temporalOldID  = "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	temporalNewID  = "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	temporalPropID = "okf_cccccccccccccccccccccccccccccccc"
	temporalProp2  = "okf_dddddddddddddddddddddddddddddddd"
)

// writeTemporalNote writes a durable note concept under .okf/knowledge/concepts.
func writeTemporalNote(t *testing.T, repo, name, frontmatter, body string) {
	t.Helper()
	path := filepath.Join(repo, ".okf", "knowledge", "concepts", name)
	content := "---\ntype: note\n" + frontmatter + "---\n" + body + "\n"
	mustWriteToolFile(t, path, content)
}

// temporalChainBundle writes an approved two-revision chain: v1 (old,
// superseded) updated by v2 (current head) plus a quarantined proposal. Each
// note points source_path at its own concept file so Context can pack it.
func temporalChainBundle(t *testing.T, repo string) {
	writeTemporalNote(t, repo, "old.md",
		"title: routing\nokf_id: "+temporalOldID+"\nmemory_state: approved\n"+
			"source_path: .okf/knowledge/concepts/old.md\n",
		"Old routing policy content mentions routing.")
	writeTemporalNote(t, repo, "new.md",
		"title: Routing Policy v2\nokf_id: "+temporalNewID+"\nmemory_state: approved\n"+
			"source_path: .okf/knowledge/concepts/new.md\n"+
			"memory_relation:\n  kind: updates\n  targets:\n    - "+temporalOldID+"\n",
		"New routing policy content mentions routing.")
	writeTemporalNote(t, repo, "proposed.md",
		"title: Routing Proposal\nokf_id: "+temporalPropID+"\nmemory_state: proposed\n"+
			"source_path: .okf/knowledge/concepts/proposed.md\n"+
			"memory_confidence: 0.8\n"+
			"provenance:\n  evidence_refs:\n    - ref-a\n    - ref-b\n"+
			"generated:\n  by: author\n  at: \"2024-01-01T00:00:00Z\"\n",
		"Proposed routing content mentions routing.")
}

// S17: a legacy bundle with no temporal metadata must behave byte/semantic
// identical: no annotations leak onto hits, regardless of view.
func TestTemporalS17LegacyQueryParity(t *testing.T) {
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha Routing
---
Alpha routing body content.
`)

	svc := NewService(Config{RepoPath: repo})
	for _, view := range []string{"", "current", "all"} {
		resp := svc.Query(context.Background(), QueryRequest{Query: "routing", Limit: 5, MemoryView: view})
		if !resp.OK {
			t.Fatalf("view=%q query OK=false, error=%#v", view, resp.Error)
		}
		result, ok := resp.Result.(QueryResult)
		if !ok {
			t.Fatalf("view=%q result type = %T, want QueryResult", view, resp.Result)
		}
		if len(result.Results) != 1 {
			t.Fatalf("view=%q result count = %d, want 1", view, len(result.Results))
		}
		hit := result.Results[0]
		if hit.MemoryState != "" || hit.MemoryCurrent != nil {
			t.Fatalf("view=%q legacy hit leaked temporal annotations: state=%q current=%v", view, hit.MemoryState, hit.MemoryCurrent)
		}
	}
}

// S32: current view drops non-current durable concepts BEFORE ranking/TopK, so
// an excluded (superseded) concept that would outrank the current head cannot
// consume the limit. All view keeps both, annotated.
func TestTemporalS32CurrentPreRankFilter(t *testing.T) {
	repo := initToolTestRepo(t)
	temporalChainBundle(t, repo)
	svc := NewService(Config{RepoPath: repo})

	// limit=1: without the pre-rank filter the exact-title "routing" (old) hit
	// would win the single slot. The filter removes it first.
	current := svc.Query(context.Background(), QueryRequest{
		Query: "routing", Limit: 1, MemoryView: "current",
	})
	if !current.OK {
		t.Fatalf("current OK=false, error=%#v", current.Error)
	}
	curResult := current.Result.(QueryResult)
	if len(curResult.Results) != 1 {
		t.Fatalf("current result count = %d, want 1", len(curResult.Results))
	}
	if curResult.Results[0].okfID != temporalNewID {
		t.Fatalf("current head hit okf_id = %q, want current %q", curResult.Results[0].okfID, temporalNewID)
	}
	if curResult.Results[0].MemoryCurrent == nil || *curResult.Results[0].MemoryCurrent != true {
		t.Fatalf("current head hit memory_current = %v, want true", curResult.Results[0].MemoryCurrent)
	}

	// All view keeps the superseded old revision and annotates it as non-current.
	all := svc.Query(context.Background(), QueryRequest{
		Query: "routing", Limit: 10, MemoryView: "all",
	})
	if !all.OK {
		t.Fatalf("all OK=false, error=%#v", all.Error)
	}
	allResult := all.Result.(QueryResult)
	var oldHit, newHit *QueryHit
	for i := range allResult.Results {
		switch allResult.Results[i].okfID {
		case temporalOldID:
			oldHit = &allResult.Results[i]
		case temporalNewID:
			newHit = &allResult.Results[i]
		}
	}
	if oldHit == nil || newHit == nil {
		t.Fatalf("all view must keep both revisions, got %d hits", len(allResult.Results))
	}
	if oldHit.MemoryCurrent == nil || *oldHit.MemoryCurrent != false {
		t.Fatalf("superseded hit memory_current = %v, want false", oldHit.MemoryCurrent)
	}
	// The approved-updates edge is owned by the current head (new revision).
	if newHit.MemoryRelationKind != "updates" {
		t.Fatalf("current head relation kind = %q, want updates", newHit.MemoryRelationKind)
	}
	if len(newHit.MemoryRelationTargets) != 1 || newHit.MemoryRelationTargets[0] != temporalOldID {
		t.Fatalf("current head relation targets = %v, want [%s]", newHit.MemoryRelationTargets, temporalOldID)
	}
}

// S33: history envelope happy path + rejection matrix.
func TestTemporalS33History(t *testing.T) {
	repo := initToolTestRepo(t)
	temporalChainBundle(t, repo)
	svc := NewService(Config{RepoPath: repo})

	// Happy path: one ref, empty query.
	resp := svc.Query(context.Background(), QueryRequest{
		MemoryView: "history", Refs: []string{temporalNewID},
	})
	if !resp.OK {
		t.Fatalf("history OK=false, error=%#v", resp.Error)
	}
	hist, ok := resp.Result.(*MemoryHistoryResult)
	if !ok {
		t.Fatalf("history result type = %T, want *MemoryHistoryResult", resp.Result)
	}
	if hist.CurrentRef != temporalNewID {
		t.Fatalf("current_ref = %q, want %q", hist.CurrentRef, temporalNewID)
	}
	if len(hist.Items) != 2 {
		t.Fatalf("history items = %d, want 2", len(hist.Items))
	}
	// Oldest -> newest: old then new.
	if hist.Items[0].OKFID != temporalOldID || !hist.Items[1].Current {
		t.Fatalf("history order/current flags = %+v", hist.Items)
	}
	if hist.Items[0].MemoryState != "approved" {
		t.Fatalf("item[0] state = %q, want approved", hist.Items[0].MemoryState)
	}

	cases := []struct {
		name string
		req  QueryRequest
	}{
		{"non_empty_query", QueryRequest{MemoryView: "history", Refs: []string{temporalNewID}, Query: "routing"}},
		{"zero_refs", QueryRequest{MemoryView: "history"}},
		{"many_refs", QueryRequest{MemoryView: "history", Refs: []string{temporalOldID, temporalNewID}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := svc.Query(context.Background(), tc.req)
			if r.OK || r.Error == nil || r.Error.Code != ErrInvalidRequest {
				t.Fatalf("history reject: ok=%v code=%v, want %s", r.OK, codeOf(r), ErrInvalidRequest)
			}
		})
	}

	// Unknown ref surfaces memory_ref_not_found.
	missing := svc.Query(context.Background(), QueryRequest{
		MemoryView: "history", Refs: []string{"okf_ffffffffffffffffffffffffffffffff"},
	})
	if missing.OK || missing.Error == nil || missing.Error.Code != "memory_ref_not_found" {
		t.Fatalf("missing ref: ok=%v code=%v, want memory_ref_not_found", missing.OK, codeOf(missing))
	}
}

// S34: review queue lists proposed concepts, body-free, ordered and capped.
func TestTemporalS34ReviewQueue(t *testing.T) {
	repo := initToolTestRepo(t)
	temporalChainBundle(t, repo)
	// A second, higher-confidence proposal with an earlier timestamp.
	writeTemporalNote(t, repo, "proposed2.md",
		"title: Routing Proposal Two\nokf_id: "+temporalProp2+"\nmemory_state: proposed\n"+
			"memory_confidence: 0.95\n"+
			"provenance:\n  evidence_refs:\n    - ref-c\n"+
			"generated:\n  by: author\n  at: \"2023-12-31T00:00:00Z\"\n",
		"Second proposal content mentions routing.")
	svc := NewService(Config{RepoPath: repo})

	resp := svc.Query(context.Background(), QueryRequest{MemoryReviewQueue: true})
	if !resp.OK {
		t.Fatalf("queue OK=false, error=%#v", resp.Error)
	}
	q, ok := resp.Result.(MemoryReviewQueueResult)
	if !ok {
		t.Fatalf("queue result type = %T, want MemoryReviewQueueResult", resp.Result)
	}
	if len(q.Items) != 2 {
		t.Fatalf("queue items = %d, want 2", len(q.Items))
	}
	// confidence DESC: 0.95 before 0.8.
	if q.Items[0].OKFID != temporalProp2 || q.Items[0].Confidence != 0.95 {
		t.Fatalf("queue[0] = %+v, want high-confidence proposal", q.Items[0])
	}
	if q.Items[1].OKFID != temporalPropID {
		t.Fatalf("queue[1] = %+v, want lower-confidence proposal", q.Items[1])
	}
	// Body-free: no Content field on the item; evidence_refs + relation surfaced.
	if len(q.Items[0].EvidenceRefs) != 1 || q.Items[1].Confidence != 0.8 {
		t.Fatalf("queue item evidence/confidence = %+v", q.Items[1])
	}

	// Limit cap: 1000 is clamped to the max; explicit small limit honored.
	big := svc.Query(context.Background(), QueryRequest{MemoryReviewQueue: true, Limit: 1000})
	bigItems := big.Result.(MemoryReviewQueueResult).Items
	if len(bigItems) > memoryReviewQueueMaxLimit {
		t.Fatalf("queue items = %d, exceeds max %d", len(bigItems), memoryReviewQueueMaxLimit)
	}
	one := svc.Query(context.Background(), QueryRequest{MemoryReviewQueue: true, Limit: 1})
	if got := len(one.Result.(MemoryReviewQueueResult).Items); got != 1 {
		t.Fatalf("limit=1 queue items = %d, want 1", got)
	}
}

// S35: mutual-exclusion matrix.
func TestTemporalS35MutualExclusion(t *testing.T) {
	repo := initToolTestRepo(t)
	temporalChainBundle(t, repo)
	svc := NewService(Config{RepoPath: repo})

	cases := []struct {
		name string
		req  QueryRequest
	}{
		{"queue_with_query", QueryRequest{MemoryReviewQueue: true, Query: "routing"}},
		{"queue_with_memory_check", QueryRequest{MemoryReviewQueue: true, MemoryCheck: true, Query: "routing"}},
		{"queue_with_group_by", QueryRequest{MemoryReviewQueue: true, GroupBy: "concept"}},
		{"queue_with_refs", QueryRequest{MemoryReviewQueue: true, Refs: []string{temporalNewID}}},
		{"queue_with_history", QueryRequest{MemoryReviewQueue: true, MemoryView: "history", Refs: []string{temporalNewID}}},
		{"history_with_query", QueryRequest{MemoryView: "history", Refs: []string{temporalNewID}, Query: "routing"}},
		{"history_zero_refs", QueryRequest{MemoryView: "history"}},
		{"bad_view", QueryRequest{Query: "routing", MemoryView: "future"}},
		{"current_with_refs", QueryRequest{Query: "routing", MemoryView: "current", Refs: []string{temporalNewID}}},
		{"all_with_refs", QueryRequest{Query: "routing", MemoryView: "all", Refs: []string{temporalNewID}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := svc.Query(context.Background(), tc.req)
			if r.OK || r.Error == nil || r.Error.Code != ErrInvalidRequest {
				t.Fatalf("mutual exclusion %q: ok=%v code=%v, want %s", tc.name, r.OK, codeOf(r), ErrInvalidRequest)
			}
		})
	}
}

// codeOf is a small helper to read the envelope error code in failures.
func codeOf(r ToolEnvelope) string {
	if r.Error == nil {
		return ""
	}
	return r.Error.Code
}

// keep path/filepath and testing imports used by helpers above.
