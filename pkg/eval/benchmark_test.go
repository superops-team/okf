package eval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/superops-team/okf/pkg/convert"
	"github.com/superops-team/okf/pkg/query"
)

const convertFixtureDir = "../../pkg/convert/testdata"

var benchmarkFixtures = []string{
	"sample.pdf", "sample.docx", "sample.xlsx",
	"sample.pptx", "sample.html", "sample.csv", "sample.txt",
}

// buildBenchmarkBundle converts all 7 fixtures to markdown and constructs a
// query.KnowledgeBundle. This exercises the real conversion output (not
// hand-written content) so retrieval quality is measured on actual data.
func buildBenchmarkBundle(t *testing.T) *query.KnowledgeBundle {
	t.Helper()
	bundle := &query.KnowledgeBundle{}
	for _, f := range benchmarkFixtures {
		r, err := convert.ConvertToMarkdown(context.Background(), filepath.Join(convertFixtureDir, f), nil)
		if err != nil {
			t.Fatalf("ConvertToMarkdown(%s): %v", f, err)
		}
		bundle.Concepts = append(bundle.Concepts, &query.Concept{
			Type:     "source",
			Title:    f,
			Resource: f + ".md",
			Content:  r.Markdown,
		})
	}
	bundle.BuildIndex()
	return bundle
}

// TestEvalBenchmark is the end-to-end IR quality benchmark. It loads the
// golden query set, runs every query against a real 7-document bundle,
// scores all four metrics, and asserts baseline quality bounds.
func TestEvalBenchmark(t *testing.T) {
	t.Parallel()
	bundle := buildBenchmarkBundle(t)
	cases, err := LoadGoldenCases("testdata/golden_queries.json")
	if err != nil {
		t.Fatalf("LoadGoldenCases: %v", err)
	}
	if len(cases) != 20 {
		t.Fatalf("golden set has %d cases, want 20", len(cases))
	}

	report := RunBenchmark(bundle, cases, 5)

	// Print the full report for baseline documentation.
	t.Logf("\n%s", report.String())

	// --- Aggregate assertions ---
	if report.Aggregate.MRR <= 0.7 {
		t.Errorf("aggregate MRR = %.4f, want > 0.7 (existing search should rank relevant docs well)", report.Aggregate.MRR)
	}
	if report.AggregateNonNegative.Recall < 0.8 {
		t.Errorf("positive aggregate Recall@5 = %.4f, want >= 0.8", report.AggregateNonNegative.Recall)
	}
	if report.PositiveCount != 18 {
		t.Errorf("positive count = %d, want 18", report.PositiveCount)
	}
	if report.NegativeCount != 2 {
		t.Errorf("negative count = %d, want 2", report.NegativeCount)
	}

	// --- Per-case assertions ---
	for i, cr := range report.Cases {
		c := cases[i]
		if len(c.ExpectedDocs) == 0 {
			// negative query: must return 0 results
			if len(cr.Results) != 0 {
				t.Errorf("negative query %q returned %d results: %v", c.Query, len(cr.Results), cr.Results)
			}
			continue
		}
		// positive query: at least half the expected docs must be in top-5
		if cr.Recall < 0.5 {
			t.Errorf("query %q Recall@5 = %.4f (< 0.5); expected %v, got %v",
				c.Query, cr.Recall, c.ExpectedDocs, cr.Results)
		}
	}
}

// TestLoadGoldenCases verifies the golden set file is well-formed and
// covers all required categories.
func TestLoadGoldenCases(t *testing.T) {
	t.Parallel()
	cases, err := LoadGoldenCases("testdata/golden_queries.json")
	if err != nil {
		t.Fatalf("LoadGoldenCases: %v", err)
	}
	hasNegative := false
	hasMultiHit := false
	formats := map[string]bool{}
	for _, c := range cases {
		if len(c.ExpectedDocs) == 0 {
			hasNegative = true
		}
		if len(c.ExpectedDocs) > 1 {
			hasMultiHit = true
		}
		for _, doc := range c.ExpectedDocs {
			formats[doc] = true
		}
	}
	if !hasNegative {
		t.Error("golden set has no negative (zero-result) cases")
	}
	if !hasMultiHit {
		t.Error("golden set has no multi-hit cases")
	}
	for _, f := range benchmarkFixtures {
		doc := f + ".md"
		if !formats[doc] {
			t.Errorf("golden set does not cover format %s", doc)
		}
	}
}

// TestRunBenchmarkEmptyBundle verifies the runner handles an empty bundle
// without panicking and returns zero scores for positive queries.
func TestRunBenchmarkEmptyBundle(t *testing.T) {
	t.Parallel()
	bundle := &query.KnowledgeBundle{}
	cases := []EvalCase{
		{Query: "anything", ExpectedDocs: []string{"sample.pdf.md"}},
		{Query: "nothing", ExpectedDocs: []string{}},
	}
	report := RunBenchmark(bundle, cases, 5)
	if report.Aggregate.MRR != 0 {
		t.Errorf("empty bundle MRR = %v, want 0", report.Aggregate.MRR)
	}
	if report.Cases[0].Recall != 0 {
		t.Errorf("positive case on empty bundle recall = %v, want 0", report.Cases[0].Recall)
	}
	// negative case on empty bundle: recall=1.0 (no relevant docs to miss), precision=0 (no results)
	if report.Cases[1].Recall != 1.0 {
		t.Errorf("negative case recall = %v, want 1.0", report.Cases[1].Recall)
	}
}
