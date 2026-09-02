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

// EvalReport is the full benchmark output: per-case results + aggregates.
type EvalReport struct {
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

// docIDOf 返回概念在评测中的文档标识。
//
// 使用 FilePath 而非 Resource：Resource 是 OKF frontmatter 的可选字段，
// 转换类概念（pkg/convert 产物）不写该字段，实测恒为空串，
// 会使所有指标恒为 0（假绿基线）。FilePath 始终存在且与 golden set
// 的 expected_docs（文件名）口径一致。
func docIDOf(c *query.Concept) string {
	if c == nil {
		return ""
	}
	return c.FilePath
}

// SearchStrategy 是被评测的检索策略：给定 bundle 与查询，返回排序后的概念。
// 抽成函数类型是为了让同一 golden set 能对比多种策略
// （纯语义 / 纯词法 / 不同权重的混合），否则评测只能测死一条链路。
type SearchStrategy func(bundle *query.KnowledgeBundle, q string) []*query.Concept

// DefaultStrategy 是既有的词法子串检索（query.Search），作为对比基线。
func DefaultStrategy(bundle *query.KnowledgeBundle, q string) []*query.Concept {
	return query.Search(bundle, q)
}

// RunBenchmark runs every case against the bundle's default search and scores results.
func RunBenchmark(bundle *query.KnowledgeBundle, cases []EvalCase, k int) *EvalReport {
	return RunBenchmarkWith(bundle, cases, k, DefaultStrategy)
}

// RunBenchmarkWith 用指定策略执行评测；strategy 为 nil 时回退 DefaultStrategy。
func RunBenchmarkWith(bundle *query.KnowledgeBundle, cases []EvalCase, k int, strategy SearchStrategy) *EvalReport {
	if strategy == nil {
		strategy = DefaultStrategy
	}
	report := &EvalReport{K: k, Cases: make([]CaseResult, 0, len(cases))}
	positive := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		results := strategy(bundle, c.Query)
		docs := make([]string, 0, len(results))
		for _, concept := range results {
			docs = append(docs, docIDOf(concept))
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

// CompareStrategies 对同一 golden set 跑多种策略，返回 策略名 → 报告。
func CompareStrategies(bundle *query.KnowledgeBundle, cases []EvalCase, k int, strategies map[string]SearchStrategy) map[string]*EvalReport {
	out := make(map[string]*EvalReport, len(strategies))
	for name, s := range strategies {
		out[name] = RunBenchmarkWith(bundle, cases, k, s)
	}
	return out
}

// FormatComparison 渲染多策略对比表。策略名按字典序排列，保证输出可复现。
func FormatComparison(reports map[string]*EvalReport) string {
	names := make([]string, 0, len(reports))
	for n := range reports {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "%-24s %10s %10s %10s %10s\n", "Strategy", "Recall", "Precision", "MRR", "NDCG")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 68))
	for _, n := range names {
		a := reports[n].AggregateNonNegative
		fmt.Fprintf(&b, "%-24s %10.4f %10.4f %10.4f %10.4f\n", n, a.Recall, a.Precision, a.MRR, a.NDCG)
	}
	return b.String()
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
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
