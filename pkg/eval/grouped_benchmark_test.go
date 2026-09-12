package eval

import (
	"testing"

	"github.com/superops-team/okf/pkg/query"
)

// groupedMetricBundle builds a tiny bundle where two chunks share one source
// (src/doc1.md) and a third lives in src/doc2.md. Lexical search for
// "grouptoken" returns all three, so source projection merges doc1's two
// chunks into a single group slot.
func groupedMetricBundle(t *testing.T) *query.KnowledgeBundle {
	t.Helper()
	bundle := &query.KnowledgeBundle{Concepts: []*query.Concept{
		{Type: "source", Title: "Doc1 A", FilePath: "src/doc1.md", Content: "grouptoken alpha",
			CustomFields: map[string]interface{}{"source_path": "src/doc1.md"}},
		{Type: "source", Title: "Doc1 B", FilePath: "src/doc1.md", Content: "grouptoken beta",
			CustomFields: map[string]interface{}{"source_path": "src/doc1.md"}},
		{Type: "source", Title: "Doc2", FilePath: "src/doc2.md", Content: "grouptoken gamma",
			CustomFields: map[string]interface{}{"source_path": "src/doc2.md"}},
	}}
	bundle.BuildIndex()
	return bundle
}

// TestEvalGroupedMetrics verifies the grouped eval report carries NDCG,
// diversity and same-source occupancy, that source grouping allocates at most
// one slot per source, and that grouped relevant-source recall is not lower than
// the raw candidate set (S47).
func TestEvalGroupedMetrics(t *testing.T) {
	t.Parallel()
	bundle := groupedMetricBundle(t)
	cases := []EvalCase{
		{Query: "grouptoken", ExpectedDocs: []string{"src/doc1.md", "src/doc2.md"}},
	}

	raw := RunBenchmark(bundle, cases, 5)
	grouped := RunGroupedBenchmark(bundle, cases, 5, query.GroupSource)

	if len(grouped.Cases) != 1 {
		t.Fatalf("grouped cases = %d, want 1", len(grouped.Cases))
	}
	cr := grouped.Cases[0]

	// All four grouped metrics are present and in [0,1].
	for name, v := range map[string]float64{
		"relevant_source_recall": cr.RelevantSourceRecall,
		"group_ndcg":             cr.GroupNDCG,
		"diversity":              cr.Diversity,
		"same_source_occupancy":  cr.SameSourceOccupancy,
	} {
		if v < -1e-9 || v > 1+1e-9 {
			t.Fatalf("%s = %.4f out of [0,1]", name, v)
		}
	}

	// Source projection collapses doc1's two chunks into one slot → 2 groups.
	if cr.Groups != 2 {
		t.Fatalf("source groups = %d, want 2 (doc1 merged, doc2 alone)", cr.Groups)
	}
	if cr.RelevantSourceRecall != 1.0 {
		t.Fatalf("relevant-source recall = %.4f, want 1.0 (both sources covered)", cr.RelevantSourceRecall)
	}
	if cr.GroupNDCG <= 0 || cr.GroupNDCG > 1+1e-9 {
		t.Fatalf("group NDCG = %.4f, want (0,1]", cr.GroupNDCG)
	}
	// Diversity = groups/totalHits = 2/3 ≈ 0.667.
	approx(t, cr.Diversity, 2.0/3.0, "diversity")
	// Occupancy = largest group (doc1, 2 hits) / total (3) = 0.667.
	approx(t, cr.SameSourceOccupancy, 2.0/3.0, "occupancy")

	// S47 gate: grouped relevant-source recall must not fall below the raw
	// candidate-set recall.
	if cr.RelevantSourceRecall < raw.Cases[0].Recall-1e-9 {
		t.Fatalf("grouped recall %.4f < raw recall %.4f (S47 regression)",
			cr.RelevantSourceRecall, raw.Cases[0].Recall)
	}

	// At most one slot per source: reconstruct the projected groups and confirm
	// no source path appears in two groups.
	results := DefaultStrategy(bundle, "grouptoken")
	pool := conceptsToResultHits(results)
	groups, _, err := query.Project(pool, query.GroupSource, false, 5)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int)
	for _, g := range groups {
		src := groupCoveredSource(g)
		seen[src]++
		if seen[src] > 1 {
			t.Fatalf("source %s occupies more than one group slot", src)
		}
	}
	if len(groups) != 2 {
		t.Fatalf("projected source groups = %d, want 2", len(groups))
	}
}

// TestEvalGroupedMetricsConcept exercises concept grouping and the aggregate
// averaging, ensuring the report string renders without panics.
func TestEvalGroupedMetricsConcept(t *testing.T) {
	t.Parallel()
	bundle := groupedMetricBundle(t)
	cases := []EvalCase{
		{Query: "grouptoken", ExpectedDocs: []string{"src/doc1.md"}},
		{Query: "nosuchquery", ExpectedDocs: nil}, // negative case
	}
	report := RunGroupedBenchmark(bundle, cases, 5, query.GroupConcept)
	if len(report.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(report.Cases))
	}
	// Aggregate averages only over the positive case.
	if report.Aggregate.GroupNDCG < 0 || report.Aggregate.GroupNDCG > 1+1e-9 {
		t.Fatalf("aggregate NDCG out of range: %v", report.Aggregate.GroupNDCG)
	}
	if report.String() == "" {
		t.Fatal("grouped report String() is empty")
	}
}
