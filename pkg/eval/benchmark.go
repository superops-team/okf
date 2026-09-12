package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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

// GroupedCaseResult holds the grouped-projection utility metrics for one case
// (S47). The raw Recall@k comparison stays on the ungrouped CaseResult fields.
type GroupedCaseResult struct {
	Query                string
	Groups               int
	RelevantSourceRecall float64
	GroupNDCG            float64
	Diversity            float64
	SameSourceOccupancy  float64
}

// GroupedAggregate holds mean grouped scores over a set of cases.
type GroupedAggregate struct {
	RelevantSourceRecall float64
	GroupNDCG            float64
	Diversity            float64
	SameSourceOccupancy  float64
}

// GroupedEvalReport is the report for one projected run (S47).
type GroupedEvalReport struct {
	Mode      string
	GroupBy   query.GroupBy
	K         int
	Cases     []GroupedCaseResult
	Aggregate GroupedAggregate
}

// EvalReport is the full benchmark output: per-case results + aggregates.
type EvalReport struct {
	// Mode 标识本次评测的检索路径：lexical / semantic-rrf / semantic-rerank。
	Mode                 string
	K                    int
	Cases                []CaseResult
	Aggregate            Aggregate // mean over all cases
	AggregateNonNegative Aggregate // mean over cases with non-empty expected (positive queries)
	PositiveCount        int
	NegativeCount        int
}

// LoadGoldenSet reads a golden set JSON file and returns the whole set
// (including its declared K), so callers can honour the file's own cut-off.
func LoadGoldenSet(path string) (*GoldenSet, error) {
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
	return &set, nil
}

// LoadGoldenCases reads a golden set JSON file and returns its cases.
func LoadGoldenCases(path string) ([]EvalCase, error) {
	set, err := LoadGoldenSet(path)
	if err != nil {
		return nil, err
	}
	return set.Cases, nil
}

// docIDOf returns the source-level document identifier used for evaluation.
// FilePath is authoritative for loaded bundles; Resource is retained as a
// compatibility fallback for in-memory callers. Derived __cN chunk names are
// normalized to their source document so one source is scored at most once.
func docIDOf(c *query.Concept) string {
	if c == nil {
		return ""
	}
	id := c.FilePath
	if id == "" {
		id = c.Resource
	}
	return normalizeChunkResource(id)
}

// SearchStrategy is the retrieval strategy under evaluation.
type SearchStrategy func(bundle *query.KnowledgeBundle, q string) []*query.Concept

// DefaultStrategy is the existing lexical substring search baseline.
func DefaultStrategy(bundle *query.KnowledgeBundle, q string) []*query.Concept {
	return query.Search(bundle, q)
}

// RunBenchmark runs every case against the default lexical search and scores results.
func RunBenchmark(bundle *query.KnowledgeBundle, cases []EvalCase, k int) *EvalReport {
	return RunBenchmarkWith(bundle, cases, k, DefaultStrategy)
}

// RunBenchmarkWith runs every case through strategy; nil uses DefaultStrategy.
func RunBenchmarkWith(bundle *query.KnowledgeBundle, cases []EvalCase, k int, strategy SearchStrategy) *EvalReport {
	if strategy == nil {
		strategy = DefaultStrategy
	}
	report := &EvalReport{K: k, Cases: make([]CaseResult, 0, len(cases))}
	positive := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		results := strategy(bundle, c.Query)
		docs := make([]string, 0, len(results))
		seen := make(map[string]struct{}, len(results))
		for _, concept := range results {
			id := docIDOf(concept)
			if id == "" {
				continue
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			docs = append(docs, id)
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

// normalizeChunkResource maps a chunk identifier back to its source document.
func normalizeChunkResource(res string) string {
	if i := strings.LastIndex(res, "__c"); i >= 0 {
		rest := res[i+3:]
		if len(rest) >= 3 && rest[0] >= '0' && rest[0] <= '9' && strings.HasSuffix(rest, ".md") {
			return res[:i] + ".md"
		}
	}
	return res
}

// CompareStrategies runs several strategies against one golden set.
func CompareStrategies(bundle *query.KnowledgeBundle, cases []EvalCase, k int, strategies map[string]SearchStrategy) map[string]*EvalReport {
	out := make(map[string]*EvalReport, len(strategies))
	for name, strategy := range strategies {
		out[name] = RunBenchmarkWith(bundle, cases, k, strategy)
	}
	return out
}

// conceptsToResultHits adapts the already-ordered, fused candidate pool into the
// entry-point-agnostic projection input. It mirrors the Service and CLI adapters
// so the shared query.Project engine sees identical identity/source signals.
func conceptsToResultHits(concepts []*query.Concept) []query.ResultHit {
	hits := make([]query.ResultHit, 0, len(concepts))
	for i, c := range concepts {
		if c == nil {
			continue
		}
		okfID, _ := c.CustomFields["okf_id"].(string)
		parentID, _ := c.CustomFields["parent_okf_id"].(string)
		sourcePath, _ := c.CustomFields["source_path"].(string)
		hits = append(hits, query.ResultHit{
			OKFID:             okfID,
			ParentOKFID:       parentID,
			LegacyFingerprint: query.Fingerprint(c),
			Ref:               docIDOf(c),
			ConceptPath:       c.FilePath,
			SourcePath:        sourcePath,
			Rank:              i + 1,
			Score:             float64(len(concepts) - i), // representative rank governs ordering; score is informational
			Provenance:        "lexical",
		})
	}
	return hits
}

// RunGroupedBenchmark projects the raw candidate pool into groups and scores the
// grouped-utility metrics (S47). It runs the existing ungrouped strategy to get
// the candidate pool, projects it through the shared query.Project engine with
// groupBy, and computes relevant-source recall, group NDCG, diversity and
// same-source occupancy. It never changes channel candidates or scores.
func RunGroupedBenchmark(bundle *query.KnowledgeBundle, cases []EvalCase, k int, groupBy query.GroupBy) *GroupedEvalReport {
	return RunGroupedBenchmarkWith(bundle, cases, k, groupBy, DefaultStrategy)
}

// RunGroupedBenchmarkWith is the strategy-injected variant of RunGroupedBenchmark.
func RunGroupedBenchmarkWith(bundle *query.KnowledgeBundle, cases []EvalCase, k int, groupBy query.GroupBy, strategy SearchStrategy) *GroupedEvalReport {
	if strategy == nil {
		strategy = DefaultStrategy
	}
	report := &GroupedEvalReport{GroupBy: groupBy, K: k, Cases: make([]GroupedCaseResult, 0, len(cases))}
	var sums GroupedAggregate
	positive := 0
	for _, c := range cases {
		results := strategy(bundle, c.Query)
		pool := conceptsToResultHits(results)
		groups, _, err := query.Project(pool, groupBy, false, k)
		if err != nil {
			// A bad groupBy at this point would be a wiring bug; surface an
			// all-zero case rather than panic the whole run.
			groups = nil
		}
		relevant := toSet(c.ExpectedDocs)
		relMap := make(map[string]float64, len(groups))
		for _, g := range groups {
			if _, ok := relevant[groupCoveredSource(g)]; ok {
				relMap[g.GroupKey] = 1.0
			}
		}
		cr := GroupedCaseResult{
			Query:                c.Query,
			Groups:               len(groups),
			RelevantSourceRecall: RelevantSourceRecallAtK(groups, c.ExpectedDocs, k),
			GroupNDCG:            GroupNDCG(groups, relMap, k),
			Diversity:            Diversity(groups),
			SameSourceOccupancy:  SameSourceOccupancy(groups),
		}
		report.Cases = append(report.Cases, cr)
		if len(c.ExpectedDocs) > 0 {
			sums.RelevantSourceRecall += cr.RelevantSourceRecall
			sums.GroupNDCG += cr.GroupNDCG
			sums.Diversity += cr.Diversity
			sums.SameSourceOccupancy += cr.SameSourceOccupancy
			positive++
		}
	}
	if positive > 0 {
		n := float64(positive)
		sums.RelevantSourceRecall /= n
		sums.GroupNDCG /= n
		sums.Diversity /= n
		sums.SameSourceOccupancy /= n
	}
	report.Aggregate = sums
	return report
}

// String renders a human-readable grouped-projection report.
func (r *GroupedEvalReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== Grouped IR Eval (by=%s, K=%d, %d positive cases) ===\n", r.GroupBy, r.K, len(r.Cases))
	fmt.Fprintf(&b, "%-22s %10s %10s %10s %10s\n", "Metric", "SrcRecall", "NDCG", "Diversity", "Occupancy")
	fmt.Fprintf(&b, "%-22s %10.4f %10.4f %10.4f %10.4f\n", "Aggregate",
		r.Aggregate.RelevantSourceRecall, r.Aggregate.GroupNDCG, r.Aggregate.Diversity, r.Aggregate.SameSourceOccupancy)
	return b.String()
}

// FormatComparison renders reports in deterministic strategy-name order.
func FormatComparison(reports map[string]*EvalReport) string {
	names := make([]string, 0, len(reports))
	for name := range reports {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %10s %10s %10s %10s\n", "Strategy", "Recall", "Precision", "MRR", "NDCG")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 68))
	for _, name := range names {
		a := reports[name].AggregateNonNegative
		fmt.Fprintf(&b, "%-24s %10.4f %10.4f %10.4f %10.4f\n", name, a.Recall, a.Precision, a.MRR, a.NDCG)
	}
	return b.String()
}

// SemanticSearcher adapts SemanticSearch to the evaluation strategy contract.
func SemanticSearcher(backend query.SemanticBackend) SearchStrategy {
	return func(bundle *query.KnowledgeBundle, q string) []*query.Concept {
		results, err := query.SemanticSearch(bundle, q, backend, query.SearchOptions{TopK: len(bundle.Concepts)})
		if err != nil {
			return nil
		}
		concepts := make([]*query.Concept, 0, len(results))
		for _, result := range results {
			if result.Concept != nil {
				concepts = append(concepts, result.Concept)
			}
		}
		return concepts
	}
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
	mode := r.Mode
	if mode == "" {
		mode = "lexical"
	}
	fmt.Fprintf(&b, "=== IR Eval Benchmark (%s, K=%d, %d cases: %d positive, %d negative) ===\n",
		mode, r.K, len(r.Cases), r.PositiveCount, r.NegativeCount)
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
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
