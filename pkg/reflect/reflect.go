// Package reflect implements bounded multi-round retrieval: round 1 lexical/
// semantic, poison gate, round 2 relation expansion, RRF fusion, and
// abstention when evidence is thin. It is in-process, deterministic, and calls
// no LLM.
package reflect

import (
	"sort"
)

// DefaultRRFK is the RRF constant (matching the existing semantic RRF).
const DefaultRRFK = 60

// MaxRoundsHardCap prevents unbounded expansion.
const MaxRoundsHardCap = 3

// ScoredHit is one fused result.
type ScoredHit struct {
	ID    string
	Score float64
}

// RRF fuses multiple ranked lists using Reciprocal Rank Fusion (k=60).
// Ties are broken by ID lexicographically for determinism.
func RRF(rankings [][]string, k int) []ScoredHit {
	scores := map[string]float64{}
	for _, list := range rankings {
		for rank, id := range list {
			scores[id] += 1.0 / float64(k+rank+1)
		}
	}
	out := make([]ScoredHit, 0, len(scores))
	for id, s := range scores {
		out = append(out, ScoredHit{ID: id, Score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Evidence is one recalled evidence item.
type Evidence struct {
	ID    string  `json:"okf_id"`
	Score float64 `json:"score"`
	Round int     `json:"source_round"`
}

// RoundTrace records one round's hits.
type RoundTrace struct {
	Round int      `json:"round"`
	Hits  []string `json:"hits"`
}

// Result is the reflect output.
type Result struct {
	Evidence    []Evidence   `json:"evidence"`
	Trace       []RoundTrace `json:"trace"`
	NeedClarify bool         `json:"need_clarify"`
	TrapBlocked bool         `json:"trap_blocked"`
	Suggestion  string       `json:"suggestion,omitempty"`
}

// Options controls reflect behavior.
type Options struct {
	MinEvidence int
	MaxRounds   int
}

// Run executes the bounded reflect workflow.
// queryFn returns ranked concept IDs for a question (round 1).
// relationFn returns related concept IDs for an anchor (round 2).
// trapGate returns true if an ID is a poison trap that must be dropped.
func Run(question string, opts Options,
	queryFn func(string) []string,
	relationFn func(string) []string,
	trapGate func(string) bool,
) (*Result, error) {
	if opts.MinEvidence <= 0 {
		opts.MinEvidence = 2
	}
	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 2
	}
	if maxRounds > MaxRoundsHardCap {
		maxRounds = MaxRoundsHardCap
	}

	res := &Result{}

	// Round 1: lexical/semantic query.
	round1 := queryFn(question)
	res.Trace = append(res.Trace, RoundTrace{Round: 1, Hits: round1})

	// Poison gate: drop trap candidates.
	var alive []string
	trapDropped := 0
	for _, id := range round1 {
		if trapGate != nil && trapGate(id) {
			trapDropped++
			continue
		}
		alive = append(alive, id)
	}
	if trapDropped > 0 {
		res.TrapBlocked = true
	}

	// Round 2: relation expansion from alive anchors (if maxRounds >= 2).
	var round2 []string
	if maxRounds >= 2 && len(alive) > 0 {
		seen := map[string]bool{}
		for _, id := range alive {
			for _, rel := range relationFn(id) {
				if !seen[rel] {
					seen[rel] = true
					round2 = append(round2, rel)
				}
			}
		}
		res.Trace = append(res.Trace, RoundTrace{Round: 2, Hits: round2})
	}

	// Fuse with RRF.
	fused := RRF([][]string{alive, round2}, DefaultRRFK)
	for i, h := range fused {
		round := 1
		for _, id := range alive {
			if id == h.ID {
				round = 1
				break
			}
		}
		for _, id := range round2 {
			if id == h.ID {
				round = 2
				break
			}
		}
		res.Evidence = append(res.Evidence, Evidence{ID: h.ID, Score: h.Score, Round: round})
		_ = i
	}

	if len(res.Evidence) < opts.MinEvidence {
		res.NeedClarify = true
		res.Suggestion = "Not enough evidence in memory. Please provide more context or clarify the question."
	}

	return res, nil
}
