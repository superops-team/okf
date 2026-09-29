package mcp

import (
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// TestMCPTemporalConversionPopulatesInfo proves the toLintConcepts conversion
// site projects memorymeta's parsed temporal metadata onto lint.TemporalInfo.
func TestMCPTemporalConversionPopulatesInfo(t *testing.T) {
	t.Parallel()

	c := &okf.Concept{
		Type:     "note",
		FilePath: "concepts/a.md",
		CustomFields: map[string]any{
			"okf_id":       "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"memory_state": "approved",
			"project":      "team-x",
			"memory_relation": map[string]any{
				"kind":    "updates",
				"targets": []any{"okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			},
			"memory_review": map[string]any{"action": "approve"},
		},
	}

	out := toLintConcepts([]*okf.Concept{c})
	if len(out) != 1 {
		t.Fatalf("expected 1 converted concept, got %d", len(out))
	}
	got := out[0].Temporal
	if got == nil {
		t.Fatal("expected TemporalInfo populated, got nil")
	}
	if !got.HasTemporal {
		t.Error("expected HasTemporal true")
	}
	if got.State != "approved" {
		t.Errorf("State = %q, want approved", got.State)
	}
	if got.RelationKind != "updates" || len(got.RelationTargets) != 1 || got.RelationTargets[0] != "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("Relation = %q %v", got.RelationKind, got.RelationTargets)
	}
	if !got.HasReviewRecord {
		t.Error("expected HasReviewRecord true")
	}
	if out[0].OKFID != "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("OKFID = %q", out[0].OKFID)
	}
	if out[0].Project != "team-x" {
		t.Errorf("Project = %q", out[0].Project)
	}
}

// TestMCPTemporalConversionNilForLegacy proves a concept without memory_*
// fields maps to a nil Temporal (legacy parity).
func TestMCPTemporalConversionNilForLegacy(t *testing.T) {
	t.Parallel()

	c := &okf.Concept{Type: "note", FilePath: "concepts/legacy.md"}
	out := toLintConcepts([]*okf.Concept{c})
	if len(out) != 1 {
		t.Fatalf("expected 1 converted concept, got %d", len(out))
	}
	if out[0].Temporal != nil {
		t.Fatalf("legacy concept must map to nil Temporal, got %+v", out[0].Temporal)
	}
}
