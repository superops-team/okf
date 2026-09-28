// Package trapeval provides deterministic golden-set evaluation for OKF
// reflective retrieval. It scores at three layers: answer (must_contain /
// must_not_contain), evidence (expected/forbidden refs), and abstention.
package trapeval

import (
	"encoding/json"
	"os"
	"strings"
)

// Case is one golden trap-evaluation case.
type Case struct {
	Question          string   `json:"question"`
	CaseType          string   `json:"case_type"` // single-hop|multi-hop|temporal|knowledge-update|abstain
	MustContain       []string `json:"must_contain"`
	MustNotContain    []string `json:"must_not_contain"`
	ExpectedEvidence  []string `json:"expected_evidence"`
	ForbiddenEvidence []string `json:"forbidden_evidence"`
	AbstainOK         bool     `json:"abstain_ok"`
}

// CaseScore holds the three-layer score for one case.
type CaseScore struct {
	CaseType        string  `json:"case_type"`
	AnswerScore     float64 `json:"answer_score"`
	EvidenceScore   float64 `json:"evidence_score"`
	AbstentionScore float64 `json:"abstention_score"`
	TrapLeak        bool    `json:"trap_leak"`
	IsPoison        bool    `json:"is_poison"`
}

// PerType aggregates scores by case_type.
type PerType struct {
	Count          int     `json:"count"`
	AnswerMean     float64 `json:"answer_mean"`
	EvidenceMean   float64 `json:"evidence_mean"`
	AbstentionMean float64 `json:"abstention_mean"`
	PoisonBlocked  float64 `json:"poison_blocked"`
}

// Report is the final evaluation report.
type Report struct {
	PerType map[string]PerType `json:"per_type"`
	// PoisonBlockedOverall is the fraction of trap cases where poison was
	// fully blocked (no forbidden evidence cited).
	PoisonBlockedOverall float64 `json:"poison_blocked_overall"`
}

// ScoreAnswer grades the generated answer against must_contain / must_not_contain.
func ScoreAnswer(c Case, answer string) float64 {
	if len(c.MustContain) == 0 {
		return 1.0
	}
	hits := 0
	for _, fact := range c.MustContain {
		if strings.Contains(strings.ToLower(answer), strings.ToLower(fact)) {
			hits++
		}
	}
	score := float64(hits) / float64(len(c.MustContain))
	for _, bad := range c.MustNotContain {
		if strings.Contains(strings.ToLower(answer), strings.ToLower(bad)) {
			score = 0.0
			break
		}
	}
	return score
}

// ScoreEvidence grades the cited evidence refs. exists is a set of known okf_ids.
func ScoreEvidence(c Case, refs []string, exists map[string]bool) float64 {
	// Forbidden cited → 0.0.
	for _, r := range refs {
		forbidden := false
		for _, f := range c.ForbiddenEvidence {
			if r == f {
				forbidden = true
				break
			}
		}
		if forbidden {
			return 0.0
		}
	}
	// Must cite at least one expected.
	if len(c.ExpectedEvidence) > 0 {
		found := false
		for _, r := range refs {
			for _, e := range c.ExpectedEvidence {
				if r == e {
					found = true
				}
			}
		}
		if !found {
			return 0.5
		}
	}
	return 1.0
}

// ScoreAbstention rewards correct abstention on abstain_ok cases.
func ScoreAbstention(c Case, abstained bool) float64 {
	if !c.AbstainOK {
		return 1.0
	}
	if abstained {
		return 1.0
	}
	return 0.0
}

// LoadCases reads a JSON array of golden cases.
func LoadCases(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []Case
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// Summarize aggregates per-type means. Poison stats only count cases marked
// IsPoison=true (i.e. trap cases that must not leak).
func Summarize(cases []Case, scores []CaseScore) Report {
	agg := map[string]*PerType{}
	var poisonTotal, poisonBlocked float64
	for i, sc := range scores {
		ct := sc.CaseType
		if ct == "" && i < len(cases) {
			ct = cases[i].CaseType
		}
		a := agg[ct]
		if a == nil {
			a = &PerType{}
			agg[ct] = a
		}
		a.Count++
		a.AnswerMean += sc.AnswerScore
		a.EvidenceMean += sc.EvidenceScore
		a.AbstentionMean += sc.AbstentionScore
		if sc.IsPoison {
			poisonTotal++
			if !sc.TrapLeak {
				poisonBlocked++
			}
		}
	}
	out := map[string]PerType{}
	for k, a := range agg {
		n := a.Count
		out[k] = PerType{
			Count:          n,
			AnswerMean:     a.AnswerMean / float64(n),
			EvidenceMean:   a.EvidenceMean / float64(n),
			AbstentionMean: a.AbstentionMean / float64(n),
		}
	}
	ratio := 1.0
	if poisonTotal > 0 {
		ratio = poisonBlocked / poisonTotal
	}
	return Report{
		PerType:              out,
		PoisonBlockedOverall: ratio,
	}
}
