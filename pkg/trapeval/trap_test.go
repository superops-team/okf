package trapeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAnswerLevelGraded(t *testing.T) {
	c := Case{MustContain: []string{"prefer", "evening"}}
	ans := "the user prefers evening meetings"
	s := ScoreAnswer(c, ans)
	if s < 0.99 {
		t.Fatalf("answer score = %v, want ~1.0", s)
	}
}

func TestForbiddenEvidenceZero(t *testing.T) {
	c := Case{ForbiddenEvidence: []string{"okf_poison"}}
	refs := []string{"okf_good", "okf_poison"}
	s := ScoreEvidence(c, refs, map[string]bool{"okf_good": true, "okf_poison": true})
	if s != 0.0 {
		t.Fatalf("evidence score = %v, want 0.0 (cited forbidden)", s)
	}
}

func TestExpectedEvidenceSupport(t *testing.T) {
	c := Case{ExpectedEvidence: []string{"okf_good"}}
	refs := []string{"okf_good"}
	s := ScoreEvidence(c, refs, map[string]bool{"okf_good": true})
	if s != 1.0 {
		t.Fatalf("evidence score = %v, want 1.0", s)
	}
}

func TestAbstentionRewarded(t *testing.T) {
	c := Case{AbstainOK: true}
	if s := ScoreAbstention(c, true); s != 1.0 {
		t.Fatalf("correct abstain = %v, want 1.0", s)
	}
	if s := ScoreAbstention(c, false); s != 0.0 {
		t.Fatalf("forced answer on abstain case = %v, want 0.0", s)
	}
}

func TestLoadGoldenJSON(t *testing.T) {
	dir := t.TempDir()
	data := `[
		{"question": "q1", "case_type": "single-hop", "must_contain": ["a"]},
		{"question": "q2", "case_type": "abstain", "abstain_ok": true}
	]`
	path := filepath.Join(dir, "cases.json")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := LoadCases(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[1].CaseType != "abstain" {
		t.Fatalf("cases = %+v", cases)
	}
}

func TestGroupByCaseType(t *testing.T) {
	cases := []Case{
		{Question: "q1", CaseType: "single-hop"},
		{Question: "q2", CaseType: "single-hop"},
		{Question: "q3", CaseType: "abstain"},
	}
	scores := []CaseScore{
		{CaseType: "single-hop", AnswerScore: 1.0},
		{CaseType: "single-hop", AnswerScore: 0.5},
		{CaseType: "abstain", AbstentionScore: 1.0},
	}
	report := Summarize(cases, scores)
	if report.PerType["single-hop"].Count != 2 {
		t.Fatalf("single-hop count = %d, want 2", report.PerType["single-hop"].Count)
	}
	if report.PerType["abstain"].AbstentionMean != 1.0 {
		t.Fatalf("abstain mean = %v, want 1.0", report.PerType["abstain"].AbstentionMean)
	}
}

// Ensure json import is used.
var _ = json.Marshal
