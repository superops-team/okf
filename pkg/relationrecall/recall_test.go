package relationrecall

import (
	"testing"

	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/okf"
)

func mkConcept(id, typ string, fields map[string]any) *okf.Concept {
	cf := map[string]any{}
	for k, v := range fields {
		cf[k] = v
	}
	cf["okf_id"] = id
	return &okf.Concept{Type: typ, Title: id, CustomFields: cf}
}

func extendsFields(targets ...string) map[string]any {
	anyTargets := make([]any, len(targets))
	for i, t := range targets {
		anyTargets[i] = t
	}
	return map[string]any{
		"memory_state": "approved",
		"memory_relation": map[string]any{
			"kind":    "extends",
			"targets": anyTargets,
		},
	}
}

func updatesFields(target string) map[string]any {
	return map[string]any{
		"memory_state": "approved",
		"memory_relation": map[string]any{
			"kind":    "updates",
			"targets": []any{target},
		},
	}
}

func proposedFields() map[string]any {
	return map[string]any{
		"memory_state":      "proposed",
		"memory_confidence": 0.8,
		"provenance":        map[string]any{"evidence_refs": []any{"okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}
}

func TestBidirectionalExtends(t *testing.T) {
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", nil)
	b := mkConcept("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "note", extendsFields("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	c := mkConcept("okf_cccccccccccccccccccccccccccccccc", "note", extendsFields("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	view := memorymeta.BuildTemporalView([]*okf.Concept{a, b, c})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range res.Hits {
		got[h.OKFID] = h.Edge
	}
	if got["okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] != "extends" {
		t.Fatalf("B not returned as extends neighbor of A: %+v", res.Hits)
	}
	if got["okf_cccccccccccccccccccccccccccccccc"] != "extends" {
		t.Fatalf("C not returned as extends neighbor of A: %+v", res.Hits)
	}

	// Incoming direction: recalling B should find A.
	res2, err := Recall("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", view)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range res2.Hits {
		if h.OKFID == "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" && h.Edge == "extends" {
			found = true
		}
	}
	if !found {
		t.Fatalf("A not returned as extends neighbor of B (incoming direction): %+v", res2.Hits)
	}
}

func TestUpdateChainHead(t *testing.T) {
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", nil)
	b := mkConcept("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "note", updatesFields("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	c := mkConcept("okf_cccccccccccccccccccccccccccccccc", "note", updatesFields("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
	view := memorymeta.BuildTemporalView([]*okf.Concept{a, b, c})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	var head string
	for _, h := range res.Hits {
		if h.IsChainHead {
			head = h.OKFID
		}
	}
	if head != "okf_cccccccccccccccccccccccccccccccc" {
		t.Fatalf("chain head = %q, want C", head)
	}
	// A, B, C all present.
	ids := map[string]bool{}
	for _, h := range res.Hits {
		ids[h.OKFID] = true
	}
	if !ids["okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] || !ids["okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] || !ids["okf_cccccccccccccccccccccccccccccccc"] {
		t.Fatalf("chain incomplete: %+v", res.Hits)
	}
}

func TestProposedHidden(t *testing.T) {
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", nil)
	p := mkConcept("okf_e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1", "note", proposedFields())
	// P extends A but is proposed.
	p.CustomFields["memory_relation"] = map[string]any{
		"kind":    "extends",
		"targets": []any{"okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	view := memorymeta.BuildTemporalView([]*okf.Concept{a, p})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Hits {
		if h.OKFID == "okf_e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1" {
			t.Fatal("proposed P leaked into recall results")
		}
	}
}

func TestUnknownAnchor(t *testing.T) {
	view := memorymeta.BuildTemporalView(nil)
	_, err := Recall("okf_ffffffffffffffffffffffffffffffff", view)
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

func TestNoTransitiveExtends(t *testing.T) {
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", nil)
	b := mkConcept("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "note", extendsFields("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	c := mkConcept("okf_cccccccccccccccccccccccccccccccc", "note", extendsFields("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
	view := memorymeta.BuildTemporalView([]*okf.Concept{a, b, c})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Hits {
		if h.OKFID == "okf_cccccccccccccccccccccccccccccccc" {
			t.Fatal("C (transitive extends) should not be returned at depth 1")
		}
	}
}

func TestExtendsCycle(t *testing.T) {
	// A extends B, B extends A — cycle at depth 1. Both should appear as
	// direct neighbors; no infinite loop.
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", extendsFields("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
	b := mkConcept("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "note", extendsFields("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	view := memorymeta.BuildTemporalView([]*okf.Concept{a, b})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	foundB := false
	for _, h := range res.Hits {
		if h.OKFID == "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
			foundB = true
		}
	}
	if !foundB {
		t.Fatalf("B missing from cycle recall: %+v", res.Hits)
	}
}

func TestExtendsFork(t *testing.T) {
	// A extends B and C (fork). Both should be returned.
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", extendsFields(
		"okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"okf_cccccccccccccccccccccccccccccccc",
	))
	b := mkConcept("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "note", nil)
	c := mkConcept("okf_cccccccccccccccccccccccccccccccc", "note", nil)
	view := memorymeta.BuildTemporalView([]*okf.Concept{a, b, c})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, h := range res.Hits {
		got[h.OKFID] = true
	}
	if !got["okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] || !got["okf_cccccccccccccccccccccccccccccccc"] {
		t.Fatalf("fork neighbors missing: %+v", res.Hits)
	}
}

func TestExtendsDanglingTarget(t *testing.T) {
	// A extends a non-existent ID. Should not panic; dangling target is skipped.
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", extendsFields(
		"okf_dddddddddddddddddddddddddddddddd",
	))
	view := memorymeta.BuildTemporalView([]*okf.Concept{a})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Hits {
		if h.OKFID == "okf_dddddddddddddddddddddddddddddddd" {
			t.Fatal("dangling target leaked into results")
		}
	}
}

func TestDeclinedHidden(t *testing.T) {
	a := mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note", nil)
	d := mkConcept("okf_d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1", "note", map[string]any{
		"memory_state": "declined",
	})
	d.CustomFields["memory_relation"] = map[string]any{
		"kind":    "extends",
		"targets": []any{"okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	view := memorymeta.BuildTemporalView([]*okf.Concept{a, d})

	res, err := Recall("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Hits {
		if h.OKFID == "okf_d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1" {
			t.Fatal("declined D leaked into recall results")
		}
	}
}
