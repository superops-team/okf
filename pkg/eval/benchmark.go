package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/superops-team/okf/pkg/query"
)

// EvalCase is a single golden query with its expected relevant documents.
type EvalCase struct {
	Query        string   `json:"query"`
	ExpectedDocs []string `json:"expected_docs"`
}

// GoldenSet is the on-disk format of the golden query benchmark file.
type GoldenSet struct {
	Description string     `json:"description"`
	K           int        `json:"k"`
	Cases       []EvalCase `json:"cases"`
}

// CaseResult holds the scored outcome for one EvalCase.
type CaseResult struct {
	Query     string
	Results   []string // document resources returned by search
	Recall    float64
	Precision float64
	MRR       float64
	NDCG      float64
	Top1      string
}

// Aggregate holds mean scores over a set of cases.
type Aggregate struct {
	Recall    float64
	Precision float64
	MRR       float64
	NDCG      float64
}

// EvalReport is the full benchmark output: per-case results + aggregates.
type EvalReport struct {
	K                     int
	Cases                 []CaseResult
	Aggregate             Aggregate // mean over all cases
	AggregateNonNegative  Aggregate // mean over cases with non-empty expected (positive queries)
	PositiveCount         int
	NegativeCount         int
}

// LoadGoldenCases reads a golden set JSON file and returns its cases.
func LoadGoldenCases(path string) ([]EvalCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read golden set: %w", err)
	}
	var set GoldenSet
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("parse golden set: %w", err)
	}
	if len(set.Cases) == 0 {
		return nil, fmt.Errorf("golden set at %s contains no cases", path)
	}
	return set.Cases, nil
}

// RunBenchmark runs every case against the bundle's search and scores results.
func RunBenchmark(bundle *query.KnowledgeBundle, cases []EvalCase, k int) *EvalReport {
	report := &EvalReport{K: k, Cases: make([]CaseResult, 0, len(cases))}
	positive := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		results := query.Search(bundle, c.Query)
		docs := make([]string, 0, len(results))
		for _, concept := range results {
			docs = append(docs, concept.Resource)
		}
		cr := CaseResult{
			Query:     c.Query,
			Results:   docs,
			Recall:    RecallAtK(docs, c.ExpectedDocs, k),
			Precision: PrecisionAtK(docs, c.ExpectedDocs, k),
			MRR:       MRR(docs, c.ExpectedDocs),
			NDCG:      NDCG(docs, c.ExpectedDocs, k),
		}
		if len(docs) > 0 {
			cr.Top1 = docs[0]
		}
		report.Cases = append(report.Cases, cr)
		if len(c.ExpectedDocs) > 0 {
			report.PositiveCount++
			positive = append(positive, cr)
		} else {
			report.NegativeCount++
		}
	}
	report.Aggregate = meanScores(report.Cases)
	report.AggregateNonNegative = meanScores(positive)
	return report
}

func meanScores(cases []CaseResult) Aggregate {
	if len(cases) == 0 {
		return Aggregate{}
	}
	var a Aggregate
	for _, c := range cases {
		a.Recall += c.Recall
		a.Precision += c.Precision
		a.MRR += c.MRR
		a.NDCG += c.NDCG
	}
	n := float64(len(cases))
	a.Recall /= n
	a.Precision /= n
	a.MRR /= n
	a.NDCG /= n
	return a
}

// String renders a human-readable benchmark report.
func (r *EvalReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== IR Eval Benchmark (K=%d, %d cases: %d positive, %d negative) ===\n",
		r.K, len(r.Cases), r.PositiveCount, r.NegativeCount)
	fmt.Fprintf(&b, "%-14s %10s %10s\n", "Metric", "All cases", "Positive")
	fmt.Fprintf(&b, "%-14s %10.4f %10.4f\n", "Recall@K", r.Aggregate.Recall, r.AggregateNonNegative.Recall)
	fmt.Fprintf(&b, "%-14s %10.4f %10.4f\n", "Precision@K", r.Aggregate.Precision, r.AggregateNonNegative.Precision)
	fmt.Fprintf(&b, "%-14s %10.4f %10.4f\n", "MRR", r.Aggregate.MRR, r.AggregateNonNegative.MRR)
	fmt.Fprintf(&b, "%-14s %10.4f %10.4f\n", "NDCG@K", r.Aggregate.NDCG, r.AggregateNonNegative.NDCG)
	b.WriteString("---\nPer-case (query → recall/precision/mrr/ndcg, top1):\n")
	for _, c := range r.Cases {
		fmt.Fprintf(&b, "  %-22s → %.2f/%.2f/%.2f/%.2f  top1=%s\n",
			truncate(c.Query, 22), c.Recall, c.Precision, c.MRR, c.NDCG, c.Top1)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
