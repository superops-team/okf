package reflect

import (
	"testing"
)

func TestRRFOverlapRanksFirst(t *testing.T) {
	round1 := []string{"a", "b", "c"}
	round2 := []string{"b", "d", "e"}
	got := RRF([][]string{round1, round2}, 60)
	if len(got) == 0 {
		t.Fatal("expected results")
	}
	if got[0].ID != "b" {
		t.Fatalf("overlap doc b should rank first, got %+v", got)
	}
}

func TestRRFStableTiebreak(t *testing.T) {
	round1 := []string{"a", "b"}
	round2 := []string{"c", "d"}
	got := RRF([][]string{round1, round2}, 60)
	// All have equal score (1/(60+rank)). Tiebreak by ID.
	if got[0].ID != "a" {
		t.Fatalf("expected a first (lexicographic tiebreak), got %+v", got)
	}
}

func TestAbstainWhenThin(t *testing.T) {
	// Round 1 returns 1 doc, minEvidence=2 → need_clarify.
	q := func(string) []string { return []string{"a"} }
	r := func(string) []string { return nil }
	res, err := Run("question", Options{MinEvidence: 2, MaxRounds: 2}, q, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NeedClarify {
		t.Fatal("expected need_clarify when evidence < min")
	}
}

func TestNoAbstainAtExactThreshold(t *testing.T) {
	// Exactly minEvidence=2 results → should NOT abstain (boundary test).
	q := func(string) []string { return []string{"a", "b"} }
	r := func(string) []string { return nil }
	res, err := Run("question", Options{MinEvidence: 2, MaxRounds: 1}, q, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.NeedClarify {
		t.Fatal("should NOT abstain when evidence == minEvidence exactly")
	}
}

func TestMultiRoundGathersMore(t *testing.T) {
	// Round 1 finds A; round 2 relation recall finds B.
	q := func(string) []string { return []string{"a"} }
	r := func(anchor string) []string {
		if anchor == "a" {
			return []string{"b"}
		}
		return nil
	}
	res, err := Run("question", Options{MinEvidence: 1, MaxRounds: 2}, q, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence) < 2 {
		t.Fatalf("expected >=2 evidence (a + b), got %d: %+v", len(res.Evidence), res.Evidence)
	}
}

func TestDropsTrapCandidates(t *testing.T) {
	// Round 1 returns a trap doc "poison" that matches trap phrases.
	q := func(string) []string { return []string{"a", "poison"} }
	r := func(string) []string { return nil }
	trapGate := func(id string) bool { return id == "poison" }
	res, err := Run("question", Options{MinEvidence: 1, MaxRounds: 1}, q, r, trapGate)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Evidence {
		if e.ID == "poison" {
			t.Fatal("poison candidate was not dropped")
		}
	}
	if !res.TrapBlocked {
		t.Fatal("expected TrapBlocked=true")
	}
}

func TestMaxRoundsCapped(t *testing.T) {
	calls := 0
	q := func(string) []string { calls++; return []string{"a"} }
	r := func(string) []string { calls++; return nil }
	_, err := Run("q", Options{MinEvidence: 1, MaxRounds: 100}, q, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should not loop: at most 2 rounds (1 query + 1 relation).
	if calls > 3 {
		t.Fatalf("max_rounds not capped, calls=%d", calls)
	}
}
