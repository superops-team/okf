package eval

import (
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/query"
)

func strategyBundle() *query.KnowledgeBundle {
	return &query.KnowledgeBundle{Concepts: []*query.Concept{
		{Type: "documentation", Title: "alpha", FilePath: "alpha.md", Content: "vector index build"},
		{Type: "documentation", Title: "beta", FilePath: "beta.md", Content: "lint rules codes"},
		{Type: "documentation", Title: "gamma", FilePath: "gamma.md", Content: "mcp server tools"},
	}}
}

// RunBenchmarkWith 必须接受可注入的检索策略，否则无法对比
// 纯语义 / 纯词法 / 混合等多种策略（评测只能测死一条链路）。
func TestRunBenchmarkWithInjectedStrategy(t *testing.T) {
	b := strategyBundle()
	cases := []EvalCase{{Query: "vector", ExpectedDocs: []string{"alpha.md"}}}

	// 策略 A：恒返回正确答案 → 指标满分
	perfect := func(_ *query.KnowledgeBundle, _ string) []*query.Concept {
		return []*query.Concept{b.Concepts[0]}
	}
	rep := RunBenchmarkWith(b, cases, 5, perfect)
	if rep.Aggregate.MRR != 1.0 {
		t.Errorf("完美策略 MRR = %v，期望 1.0", rep.Aggregate.MRR)
	}

	// 策略 B：恒返回错误答案 → 指标为 0
	wrong := func(_ *query.KnowledgeBundle, _ string) []*query.Concept {
		return []*query.Concept{b.Concepts[1]}
	}
	rep2 := RunBenchmarkWith(b, cases, 5, wrong)
	if rep2.Aggregate.MRR != 0.0 {
		t.Errorf("错误策略 MRR = %v，期望 0.0", rep2.Aggregate.MRR)
	}
	if rep.Aggregate.MRR == rep2.Aggregate.MRR {
		t.Error("不同策略给出相同指标，说明策略未真正生效")
	}
}

// RunBenchmark 必须与 RunBenchmarkWith(默认策略) 等价（向下兼容）。
func TestRunBenchmarkDelegatesToDefaultStrategy(t *testing.T) {
	b := strategyBundle()
	cases := []EvalCase{
		{Query: "vector", ExpectedDocs: []string{"alpha.md"}},
		{Query: "lint", ExpectedDocs: []string{"beta.md"}},
	}
	a := RunBenchmark(b, cases, 5)
	c := RunBenchmarkWith(b, cases, 5, nil) // nil 策略 → 回退默认
	if a.Aggregate != c.Aggregate {
		t.Errorf("RunBenchmark %+v 与默认策略 %+v 不一致", a.Aggregate, c.Aggregate)
	}
}

// 报告需能按策略名区分，便于多策略对比输出。
func TestCompareStrategiesProducesOneReportPerStrategy(t *testing.T) {
	b := strategyBundle()
	cases := []EvalCase{{Query: "vector", ExpectedDocs: []string{"alpha.md"}}}
	strategies := map[string]SearchStrategy{
		"good": func(_ *query.KnowledgeBundle, _ string) []*query.Concept {
			return []*query.Concept{b.Concepts[0]}
		},
		"bad": func(_ *query.KnowledgeBundle, _ string) []*query.Concept {
			return []*query.Concept{b.Concepts[2]}
		},
	}
	reports := CompareStrategies(b, cases, 5, strategies)
	if len(reports) != 2 {
		t.Fatalf("返回 %d 个报告，期望 2", len(reports))
	}
	if reports["good"].Aggregate.MRR <= reports["bad"].Aggregate.MRR {
		t.Errorf("good(%v) 应优于 bad(%v)", reports["good"].Aggregate.MRR, reports["bad"].Aggregate.MRR)
	}
}

// 对比表输出必须包含每个策略名与指标（便于人工审阅与写入 conformance）。
func TestFormatComparisonIncludesAllStrategies(t *testing.T) {
	b := strategyBundle()
	cases := []EvalCase{{Query: "vector", ExpectedDocs: []string{"alpha.md"}}}
	reports := CompareStrategies(b, cases, 5, map[string]SearchStrategy{
		"semantic-only": func(_ *query.KnowledgeBundle, _ string) []*query.Concept {
			return []*query.Concept{b.Concepts[0]}
		},
		"hybrid": func(_ *query.KnowledgeBundle, _ string) []*query.Concept {
			return []*query.Concept{b.Concepts[0]}
		},
	})
	out := FormatComparison(reports)
	for _, name := range []string{"semantic-only", "hybrid", "Recall", "MRR"} {
		if !strings.Contains(out, name) {
			t.Errorf("对比输出缺少 %q:\n%s", name, out)
		}
	}
}

// 对比表顺序必须确定（map 遍历顺序随机会导致输出不可复现）。
func TestFormatComparisonIsDeterministic(t *testing.T) {
	b := strategyBundle()
	cases := []EvalCase{{Query: "vector", ExpectedDocs: []string{"alpha.md"}}}
	mk := func() map[string]*EvalReport {
		return CompareStrategies(b, cases, 5, map[string]SearchStrategy{
			"zulu":  func(_ *query.KnowledgeBundle, _ string) []*query.Concept { return b.Concepts[:1] },
			"alpha": func(_ *query.KnowledgeBundle, _ string) []*query.Concept { return b.Concepts[:1] },
			"mike":  func(_ *query.KnowledgeBundle, _ string) []*query.Concept { return b.Concepts[:1] },
		})
	}
	first := FormatComparison(mk())
	for i := 0; i < 5; i++ {
		if got := FormatComparison(mk()); got != first {
			t.Fatalf("第 %d 次输出不一致:\n%s\nvs\n%s", i, got, first)
		}
	}
}
