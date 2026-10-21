package memorymeta

// Benchmark for the temporal projection over a 10,000-concept durable corpus
// (references.md Experiment D layout):
//
//	6000  independent approved notes        (no relation)
//	1000  linear update chains of length 3 (3000 concepts: oldest<-middle<-newest)
//	500   extends edges                    (500 concepts elaborating approved notes)
//	500   proposed/declined concepts       (proposed-updates edges stay inactive)
//	                                  total = 10000 durable concepts
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=20x ./pkg/memorymeta/ -run '^$'
//
// Baseline (recorded 2026-09-20, AMD EPYC 9Y24, go1.26, benchtime=20x;
// NO hard regression gate yet — gates are set only after a repeatable baseline
// per the spec):
//
//	BenchmarkTemporalBuildView10k-3     19489136 ns/op   6154850 B/op   31132 allocs/op
//	BenchmarkTemporalHistoryChain3-3     171674 ns/op       368 B/op       6 allocs/op

import (
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// benchDurableID returns a deterministic, unique canonical okf id (okf_ + 32
// hex chars) for corpus index n. It reuses the package test helper validID so
// the ids pass identity.Parse against the ^okf_[0-9a-f]{32}$ grammar; the
// offset avoids colliding with the fixed idA..idP constants.
func benchDurableID(n int) string { return validID(200000 + n) }

// buildTemporalCorpus10k assembles the 10k durable corpus described in the file
// header. It returns the corpus plus the newest id of the first update chain
// (a healthy current head) for the History benchmark.
func buildTemporalCorpus10k() (corpus []*okf.Concept, chainHead string) {
	const (
		independentNotes = 6000
		chains           = 1000
		chainLen         = 3
		extendsEdges     = 500
		proposedNotes    = 400
		declinedNotes    = 100
	)
	corpus = make([]*okf.Concept, 0, independentNotes+chains*chainLen+extendsEdges+proposedNotes+declinedNotes)
	idx := 0

	// Independent approved notes.
	for i := 0; i < independentNotes; i++ {
		corpus = append(corpus, mkMem(benchDurableID(idx), "note", "", "", ""))
		idx++
	}

	// Linear update chains of length 3: ids[0] oldest, ids[i] updates ids[i-1].
	// The newest (ids[chainLen-1]) is the healthy current head.
	for c := 0; c < chains; c++ {
		first := idx
		for j := 0; j < chainLen; j++ {
			id := benchDurableID(idx)
			if c == 0 && j == chainLen-1 {
				chainHead = id
			}
			if j == 0 {
				corpus = append(corpus, mkMem(id, "note", "", "", ""))
			} else {
				corpus = append(corpus, mkMem(id, "note", "", "", "updates", benchDurableID(first+j-1)))
			}
			idx++
		}
	}

	// Extends edges: each extends an independent approved note (inactive for
	// currentness; the elab concept itself remains current).
	for i := 0; i < extendsEdges; i++ {
		target := benchDurableID(i % independentNotes)
		corpus = append(corpus, mkMem(benchDurableID(idx), "note", "", "", "extends", target))
		idx++
	}

	// Proposed concepts: half carry an inactive proposed-updates edge to an
	// independent approved note; the rest are bare proposals.
	for i := 0; i < proposedNotes; i++ {
		id := benchDurableID(idx)
		if i%2 == 0 {
			target := benchDurableID(i % independentNotes)
			corpus = append(corpus, mkMem(id, "note", "", "proposed", "updates", target))
		} else {
			corpus = append(corpus, mkMem(id, "note", "", "proposed", ""))
		}
		idx++
	}

	// Declined concepts: never enter the current view.
	for i := 0; i < declinedNotes; i++ {
		target := benchDurableID(i % independentNotes)
		corpus = append(corpus, mkMem(benchDurableID(idx), "note", "", "declined", "updates", target))
		idx++
	}

	return corpus, chainHead
}

// BenchmarkTemporalBuildView10k measures BuildTemporalView (the current
// projection) over the full 10k corpus. The view is discarded each iteration so
// the allocation budget reflects a cold projection.
func BenchmarkTemporalBuildView10k(b *testing.B) {
	corpus, _ := buildTemporalCorpus10k()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = BuildTemporalView(corpus)
	}
}

// BenchmarkTemporalHistoryChain3 measures view.History(head) on a healthy
// length-3 update chain. The view is built once outside the loop so the number
// isolates the history walk (oldest->newest) rather than the projection.
func BenchmarkTemporalHistoryChain3(b *testing.B) {
	corpus, head := buildTemporalCorpus10k()
	view := BuildTemporalView(corpus)
	if _, err := view.History(head); err != nil {
		b.Fatalf("precondition: History(head=%q) err: %v", head, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := view.History(head); err != nil {
			b.Fatal(err)
		}
	}
}

// sanity check the corpus shape once outside the benchmark harness.
func TestTemporalBenchCorpusShape(t *testing.T) {
	corpus, head := buildTemporalCorpus10k()
	if len(corpus) != 10000 {
		t.Fatalf("corpus size = %d, want 10000", len(corpus))
	}
	view := BuildTemporalView(corpus)
	if !view.HasTemporalData {
		t.Fatal("corpus must carry temporal data")
	}
	if !view.IsCurrent(head) {
		t.Fatalf("chain head %q must be current", head)
	}
	if h, err := view.History(head); err != nil || len(h.Items) != 3 {
		t.Fatalf("History(head) = %+v, err=%v; want 3 items", h, err)
	}
}
