package tool

// Benchmark for the review-queue ordering over a proposed-concept set
// (references.md Experiment D). The corpus mirrors the memorymeta temporal
// corpus: ~5000 approved notes plus ~500 proposed concepts (each carrying a
// confidence, generated.at, and provenance.evidence_refs so the real sort keys
// are exercised).
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=20x ./pkg/tool/ -run '^$'
//
// Baseline (recorded 2026-09-20, AMD EPYC 9Y24, go1.26, benchtime=20x;
// NO hard regression gate yet):
//
//	BenchmarkMemoryReviewQueueOrdering-3  592238 ns/op  61568 B/op  708 allocs/op

import (
	"fmt"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// benchReviewID returns a deterministic, unique canonical okf id for index n.
func benchReviewID(n int) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.WriteString("okf_")
	state := uint64(n+1)*0x9e3779b97f4a7c15 + 0x517cc1b727220a95
	for i := 0; i < 32; i++ {
		state = state*6364136223867930055 + 1442695040888963407
		b.WriteByte(hex[(state>>uint(i%8*4))&0xf])
	}
	return b.String()
}

// buildReviewQueueCorpus assembles ~5000 approved notes plus ~500 proposed
// concepts with varied confidence/generated.at so the review-queue sort is real.
func buildReviewQueueCorpus() []*okf.Concept {
	const (
		approvedNotes = 5000
		proposedNotes = 500
	)
	corpus := make([]*okf.Concept, 0, approvedNotes+proposedNotes)

	for i := 0; i < approvedNotes; i++ {
		corpus = append(corpus, &okf.Concept{
			Type:  "note",
			Title: fmt.Sprintf("Approved runbook %d", i),
			CustomFields: map[string]any{
				"okf_id": benchReviewID(i),
			},
		})
	}
	for i := 0; i < proposedNotes; i++ {
		conf := 0.5 + float64(i%50)/100.0
		day := (i % 28) + 1
		corpus = append(corpus, &okf.Concept{
			Type:  "note",
			Title: fmt.Sprintf("Proposal %d", i),
			CustomFields: map[string]any{
				"okf_id":            benchReviewID(approvedNotes + i),
				"memory_state":      "proposed",
				"memory_confidence": conf,
				"provenance": map[string]any{
					"evidence_refs": []any{benchReviewID(i % approvedNotes)},
				},
			},
			Generated: &okf.GeneratedInfo{
				By: "bench",
				At: fmt.Sprintf("2026-01-%02dT00:00:00Z", day),
			},
		})
	}
	return corpus
}

// BenchmarkMemoryReviewQueueOrdering measures buildMemoryReviewQueue over the
// ~500-proposed set: it scans the whole corpus, filters proposed durable
// concepts, sorts by confidence DESC / generated.at ASC / okf_id ASC, and
// materializes the top-100 body-free items.
func BenchmarkMemoryReviewQueueOrdering(b *testing.B) {
	corpus := buildReviewQueueCorpus()
	// The queue materializes at most memoryReviewQueueMaxLimit items; the sort
	// still processes all ~500 proposed candidates regardless of the cap.
	if got := buildMemoryReviewQueue(corpus, memoryReviewQueueMaxLimit); len(got.Items) != memoryReviewQueueMaxLimit {
		b.Fatalf("precondition: review queue returned %d items, want %d", len(got.Items), memoryReviewQueueMaxLimit)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = buildMemoryReviewQueue(corpus, memoryReviewQueueMaxLimit)
	}
}
