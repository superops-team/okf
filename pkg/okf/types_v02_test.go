package okf

import (
	"testing"
	"time"
)

// --- TrustTier tests (spec §5.3) ---

func TestTrustTier_Unverified(t *testing.T) {
	c := &Concept{Type: "Metric"}
	if got := c.TrustTier(); got != TrustUnverified {
		t.Errorf("expected TrustUnverified, got %v", got)
	}
}

func TestTrustTier_MachineConfirmed(t *testing.T) {
	c := &Concept{
		Type: "Metric",
		Verified: []VerificationEvent{
			{By: "process:nightly", At: "2026-06-12T08:00:00Z"},
		},
	}
	if got := c.TrustTier(); got != TrustMachineConfirmed {
		t.Errorf("expected TrustMachineConfirmed, got %v", got)
	}
}

func TestTrustTier_HumanReviewed(t *testing.T) {
	c := &Concept{
		Type: "Metric",
		Verified: []VerificationEvent{
			{By: "human:ahormati", At: "2026-06-25T09:00:00Z"},
		},
	}
	if got := c.TrustTier(); got != TrustHumanReviewed {
		t.Errorf("expected TrustHumanReviewed, got %v", got)
	}
}

func TestTrustTier_HumanAndMachine(t *testing.T) {
	c := &Concept{
		Type: "Metric",
		Verified: []VerificationEvent{
			{By: "process:nightly", At: "2026-06-12T08:00:00Z"},
			{By: "human:ahormati", At: "2026-06-25T09:00:00Z"},
		},
	}
	if got := c.TrustTier(); got != TrustHumanReviewed {
		t.Errorf("expected TrustHumanReviewed (human takes precedence), got %v", got)
	}
}

func TestTrustTier_String(t *testing.T) {
	cases := []struct {
		tier TrustTier
		want string
	}{
		{TrustUnverified, "unverified"},
		{TrustMachineConfirmed, "machine-confirmed"},
		{TrustHumanReviewed, "human-reviewed"},
	}
	for _, c := range cases {
		if got := c.tier.String(); got != c.want {
			t.Errorf("TrustTier(%d).String() = %q, want %q", c.tier, got, c.want)
		}
	}
}

// --- IsStale tests (spec §5.5) ---

func TestIsStale_Fresh(t *testing.T) {
	c := &Concept{StaleAfter: "2026-12-31"}
	ref := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	if c.IsStale(ref) {
		t.Error("expected not stale when ref < stale_after")
	}
}

func TestIsStale_Stale(t *testing.T) {
	c := &Concept{StaleAfter: "2026-06-15"}
	ref := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	if !c.IsStale(ref) {
		t.Error("expected stale when ref >= stale_after")
	}
}

func TestIsStale_OnBoundary(t *testing.T) {
	c := &Concept{StaleAfter: "2026-08-25"}
	ref := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if !c.IsStale(ref) {
		t.Error("expected stale on the boundary day (today >= stale_after)")
	}
}

func TestIsStale_Empty(t *testing.T) {
	c := &Concept{}
	ref := time.Now()
	if c.IsStale(ref) {
		t.Error("expected not stale when stale_after is empty")
	}
}

func TestIsStale_InvalidDate(t *testing.T) {
	c := &Concept{StaleAfter: "not-a-date"}
	ref := time.Now()
	if c.IsStale(ref) {
		t.Error("expected not stale (no panic) when stale_after is invalid")
	}
}

// --- Attested Computation tests (spec §10) ---

func TestIsAttestedComputation(t *testing.T) {
	c := &Concept{Type: "Attested Computation"}
	if !c.IsAttestedComputation() {
		t.Error("expected true for Attested Computation type")
	}
}

func TestIsAttestedComputation_False(t *testing.T) {
	c := &Concept{Type: "Metric"}
	if c.IsAttestedComputation() {
		t.Error("expected false for non-Attested Computation type")
	}
}

func TestGetComputationBody_Inline(t *testing.T) {
	c := &Concept{
		Content: "# Definition\nSome text.\n\n# Computation\n\n```sql\nSELECT 1\n```\n\nMore text.",
	}
	code, ok := c.GetComputationBody()
	if !ok {
		t.Fatal("expected to find computation body")
	}
	if code != "SELECT 1" {
		t.Errorf("expected 'SELECT 1', got %q", code)
	}
}

func TestGetComputationBody_NoHeading(t *testing.T) {
	c := &Concept{Content: "# Definition\nSome text."}
	_, ok := c.GetComputationBody()
	if ok {
		t.Error("expected false when no # Computation heading")
	}
}

func TestGetComputationBody_NoFence(t *testing.T) {
	c := &Concept{Content: "# Computation\n\nJust prose, no code fence."}
	_, ok := c.GetComputationBody()
	if ok {
		t.Error("expected false when # Computation has no fenced code block")
	}
}

func TestGetComputationBody_EmptyContent(t *testing.T) {
	c := &Concept{}
	_, ok := c.GetComputationBody()
	if ok {
		t.Error("expected false for empty content")
	}
}

// --- EffectiveStatus tests (spec §5.4) ---

func TestEffectiveStatus_DefaultStable(t *testing.T) {
	c := &Concept{}
	if got := c.EffectiveStatus(); got != StatusStable {
		t.Errorf("expected StatusStable for empty status, got %v", got)
	}
}

func TestEffectiveStatus_Explicit(t *testing.T) {
	cases := []ConceptStatus{StatusDraft, StatusStable, StatusDeprecated}
	for _, s := range cases {
		c := &Concept{Status: s}
		if got := c.EffectiveStatus(); got != s {
			t.Errorf("expected %v, got %v", s, got)
		}
	}
}

// --- Bundle filter tests (v0.2 query dimensions) ---

func TestFilterByTrustTier(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(&Concept{Type: "Metric", Title: "unverified"})
	b.AddConcept(&Concept{Type: "Metric", Title: "machine", Verified: []VerificationEvent{{By: "process:x"}}})
	b.AddConcept(&Concept{Type: "Metric", Title: "human", Verified: []VerificationEvent{{By: "human:alice"}}})

	got := b.FilterByTrustTier(TrustHumanReviewed)
	if len(got) != 1 || got[0].Title != "human" {
		t.Errorf("expected 1 human-reviewed concept, got %d", len(got))
	}
}

func TestFilterByStatus(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(&Concept{Type: "Metric", Title: "draft", Status: StatusDraft})
	b.AddConcept(&Concept{Type: "Metric", Title: "stable"}) // empty => stable
	b.AddConcept(&Concept{Type: "Metric", Title: "deprecated", Status: StatusDeprecated})

	got := b.FilterByStatus(StatusStable)
	if len(got) != 1 || got[0].Title != "stable" {
		t.Errorf("expected 1 stable concept, got %d", len(got))
	}
}

func TestFilterFresh(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(&Concept{Type: "Metric", Title: "fresh", StaleAfter: "2099-01-01"})
	b.AddConcept(&Concept{Type: "Metric", Title: "stale", StaleAfter: "2020-01-01"})

	ref := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	got := b.FilterFresh(ref)
	if len(got) != 1 || got[0].Title != "fresh" {
		t.Errorf("expected 1 fresh concept, got %d", len(got))
	}
}

func TestFilterBySource(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(&Concept{
		Type:  "Metric",
		Title: "with-source",
		Sources: []Source{
			{ID: "rev-policy", Resource: "https://wiki.acme.com/finance/revenue"},
		},
	})
	b.AddConcept(&Concept{Type: "Metric", Title: "no-source"})

	got := b.FilterBySource("rev-policy")
	if len(got) != 1 || got[0].Title != "with-source" {
		t.Errorf("expected 1 concept with source id, got %d", len(got))
	}

	got = b.FilterBySource("https://wiki.acme.com/finance/revenue")
	if len(got) != 1 {
		t.Errorf("expected 1 concept with source resource, got %d", len(got))
	}
}

func TestSearch_Sources(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(&Concept{
		Type:  "Metric",
		Title: "revenue",
		Sources: []Source{
			{Title: "Revenue recognition policy", Author: "team:finance-fpa"},
		},
	})
	b.AddConcept(&Concept{Type: "Metric", Title: "unrelated"})

	// Search by source title
	got := b.Search("revenue recognition")
	if len(got) != 1 || got[0].Title != "revenue" {
		t.Errorf("expected search by source title to find 1 concept, got %d", len(got))
	}

	// Search by source author
	got = b.Search("finance-fpa")
	if len(got) != 1 {
		t.Errorf("expected search by source author to find 1 concept, got %d", len(got))
	}
}

// --- Stats v0.2 tests ---

func TestStats_V02Dimensions(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(&Concept{Type: "Metric", Title: "unverified"})
	b.AddConcept(&Concept{Type: "Metric", Title: "machine", Verified: []VerificationEvent{{By: "process:x"}}})
	b.AddConcept(&Concept{Type: "Attested Computation", Title: "ac", Runtime: "bigquery"})
	b.AddConcept(&Concept{Type: "Metric", Title: "stale", StaleAfter: "2020-01-01"})

	stats := b.Stats()
	if stats.TotalConcepts != 4 {
		t.Errorf("expected 4 total concepts, got %d", stats.TotalConcepts)
	}
	if stats.TrustTierCounts[TrustUnverified] != 3 {
		t.Errorf("expected 3 unverified, got %d", stats.TrustTierCounts[TrustUnverified])
	}
	if stats.TrustTierCounts[TrustMachineConfirmed] != 1 {
		t.Errorf("expected 1 machine-confirmed, got %d", stats.TrustTierCounts[TrustMachineConfirmed])
	}
	if stats.AttestedComputationCount != 1 {
		t.Errorf("expected 1 attested computation, got %d", stats.AttestedComputationCount)
	}
	if stats.StaleCount < 1 {
		t.Errorf("expected at least 1 stale concept, got %d", stats.StaleCount)
	}
	if stats.StatusCounts[StatusStable] != 4 {
		t.Errorf("expected 4 stable (default), got %d", stats.StatusCounts[StatusStable])
	}
}

// --- NewConcept / NewBundle v0.2 defaults ---

func TestNewConcept_V02Defaults(t *testing.T) {
	c := NewConcept("Metric", "Test")
	if c.Type != "Metric" {
		t.Errorf("expected type Metric, got %s", c.Type)
	}
	if c.Generated == nil || c.Generated.By != "unknown" {
		t.Error("expected Generated to be set with By=unknown")
	}
	if c.EffectiveStatus() != StatusStable {
		t.Error("expected default status stable")
	}
}

func TestNewBundle_V02Defaults(t *testing.T) {
	b := NewBundle("test")
	if b.OKFVersion != "0.2" {
		t.Errorf("expected OKFVersion 0.2, got %s", b.OKFVersion)
	}
}
