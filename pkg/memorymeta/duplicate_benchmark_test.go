package memorymeta

// Benchmark for CheckMemory duplicate detection against a 1000-concept durable
// corpus. Mirrors the E2E fixture used in the governed-agent-memory evaluation:
// 1000 note concepts with mixed bodies, one query that should return no_similar.
//
// Run with: go test -bench=BenchmarkCheckMemory -benchmem -run=^$ ./pkg/memorymeta/...

import (
	"fmt"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// benchDurableCorpus builds n durable note concepts with varied bodies. The
// bodies are long enough to exercise the Jaccard tokenization path but short
// enough to keep the corpus in-memory.
func benchDurableCorpus(n int) []*okf.Concept {
	out := make([]*okf.Concept, 0, n)
	for i := range n {
		title := fmt.Sprintf("Review runbook %d", i)
		body := fmt.Sprintf(
			"Note %d: review the runbook for topic %d. The team should follow the documented steps "+
				"and verify each checkpoint before moving to the next stage. Keep the runbook up to date "+
				"with any operational lessons learned from the most recent incident review session.",
			i, i%20,
		)
		out = append(out, mkConcept(
			fmt.Sprintf("okf_%032d", i),
			"note",
			title,
			body,
			[]string{"note", fmt.Sprintf("topic%d", i%10)},
		))
	}
	return out
}

// BenchmarkCheckMemory1000Miss measures CheckMemory over a 1000-concept durable
// pool with a query that does not match any candidate (no_similar path).
func BenchmarkCheckMemory1000Miss(b *testing.B) {
	corpus := benchDurableCorpus(1000)
	const query = "test"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = CheckMemory(corpus, query, "", "", "", 0.20)
	}
}

// BenchmarkCheckMemory1000Hit exercises the matching path: the query is taken
// from one corpus entry so at least one candidate clears the Jaccard threshold.
func BenchmarkCheckMemory1000Hit(b *testing.B) {
	corpus := benchDurableCorpus(1000)
	// Pull a body from the middle of the corpus so the match has to scan.
	hitBody := corpus[500].Content
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = CheckMemory(corpus, hitBody, "", "", "", 0.20)
	}
}
