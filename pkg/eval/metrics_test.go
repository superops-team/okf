package eval

import (
	"math"
	"testing"
	"testing/quick"
)

const eps = 1e-9

func approx(t *testing.T, got, want float64, name string) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

// --- Precision@K ---

func TestPrecisionAtK_FewerResultsThanK(t *testing.T) {
	t.Parallel()
	// 2 results, both relevant, K=5 → denominator is 2 (actual count), not 5
	got := PrecisionAtK([]string{"a", "b"}, []string{"a", "b"}, 5)
	approx(t, got, 1.0, "Precision@K")
}

func TestPrecisionAtK_MixedRelevance(t *testing.T) {
	t.Parallel()
	// results [a,b,c,d], a and c relevant, K=3 → top3=[a,b,c], 2 relevant → 2/3
	got := PrecisionAtK([]string{"a", "b", "c", "d"}, []string{"a", "c"}, 3)
	approx(t, got, 2.0/3.0, "Precision@K")
}

func TestPrecisionAtK_EmptyResults(t *testing.T) {
	t.Parallel()
	got := PrecisionAtK([]string{}, []string{"a"}, 5)
	approx(t, got, 0.0, "Precision@K")
}

func TestPrecisionAtK_NoneRelevant(t *testing.T) {
	t.Parallel()
	got := PrecisionAtK([]string{"x", "y"}, []string{"a"}, 5)
	approx(t, got, 0.0, "Precision@K")
}

// --- Recall@K ---

func TestRecallAtK_PartialRecall(t *testing.T) {
	t.Parallel()
	// expected {a,b,c,d}, results [a,x,b,y], K=4 → 2 of 4 recalled → 0.5
	got := RecallAtK([]string{"a", "x", "b", "y"}, []string{"a", "b", "c", "d"}, 4)
	approx(t, got, 0.5, "Recall@K")
}

func TestRecallAtK_EmptyExpected(t *testing.T) {
	t.Parallel()
	// no relevant docs to miss → 1.0 by convention
	got := RecallAtK([]string{"a", "b"}, []string{}, 5)
	approx(t, got, 1.0, "Recall@K")
}

func TestRecallAtK_AllRecalled(t *testing.T) {
	t.Parallel()
	got := RecallAtK([]string{"a", "b", "c"}, []string{"a", "b", "c"}, 5)
	approx(t, got, 1.0, "Recall@K")
}

func TestRecallAtK_EmptyResults(t *testing.T) {
	t.Parallel()
	got := RecallAtK([]string{}, []string{"a"}, 5)
	approx(t, got, 0.0, "Recall@K")
}

// --- MRR ---

func TestMRR_FirstRelevantAtRank3(t *testing.T) {
	t.Parallel()
	// results [x,y,a,b], a is first relevant → rank 3 → 1/3
	got := MRR([]string{"x", "y", "a", "b"}, []string{"a", "b"})
	approx(t, got, 1.0/3.0, "MRR")
}

func TestMRR_FirstRelevantAtRank1(t *testing.T) {
	t.Parallel()
	got := MRR([]string{"a", "x", "y"}, []string{"a"})
	approx(t, got, 1.0, "MRR")
}

func TestMRR_NoRelevant(t *testing.T) {
	t.Parallel()
	got := MRR([]string{"x", "y", "z"}, []string{"a"})
	approx(t, got, 0.0, "MRR")
}

func TestMRR_EmptyResults(t *testing.T) {
	t.Parallel()
	got := MRR([]string{}, []string{"a"})
	approx(t, got, 0.0, "MRR")
}

// --- NDCG@K ---

func TestNDCG_PerfectRanking(t *testing.T) {
	t.Parallel()
	// expected [a,b,c] (relevance order), results [a,b,c], K=3 → 1.0
	got := NDCG([]string{"a", "b", "c"}, []string{"a", "b", "c"}, 3)
	approx(t, got, 1.0, "NDCG")
}

func TestNDCG_IrrelevantDocPushesRelevantDown(t *testing.T) {
	t.Parallel()
	// expected [a,b,c], results [x,a,b], K=2 → top2=[x,a], only a relevant
	// DCG = 0 + 1/log2(3) = 0.6309
	// IDCG (ideal top2 = a,b) = 1/log2(2) + 1/log2(3) = 1 + 0.6309 = 1.6309
	// NDCG ≈ 0.3869
	got := NDCG([]string{"x", "a", "b"}, []string{"a", "b", "c"}, 2)
	want := (1.0 / math.Log2(3)) / (1.0 + 1.0/math.Log2(3))
	approx(t, got, want, "NDCG")
	if got >= 1.0 || got <= 0.0 {
		t.Fatalf("NDCG = %v, want (0,1)", got)
	}
}

func TestNDCG_AllRelevantReorderedIsOne(t *testing.T) {
	t.Parallel()
	// Binary relevance: when all top-K results are relevant, their internal
	// order does not change NDCG (every relevant doc has gain=1).
	// This locks the binary-relevance semantics.
	got := NDCG([]string{"c", "b", "a"}, []string{"a", "b", "c"}, 3)
	approx(t, got, 1.0, "NDCG")
}

func TestNDCG_EmptyExpected(t *testing.T) {
	t.Parallel()
	got := NDCG([]string{"a", "b"}, []string{}, 5)
	approx(t, got, 1.0, "NDCG")
}

func TestNDCG_NoRelevantInResults(t *testing.T) {
	t.Parallel()
	got := NDCG([]string{"x", "y"}, []string{"a", "b"}, 5)
	approx(t, got, 0.0, "NDCG")
}

func TestNDCG_PartialTopK(t *testing.T) {
	t.Parallel()
	// expected [a,b,c,d], results [a,x,b,y,c,z], K=3 → top3=[a,x,b], 2 relevant
	// DCG = 1/log2(2) + 0 + 1/log2(4) = 1 + 0.5 = 1.5
	// IDCG (ideal top3 = a,b,c) = 1/log2(2)+1/log2(3)+1/log2(4) = 1+0.6309+0.5 = 2.1309
	// NDCG ≈ 1.5/2.1309 ≈ 0.7039
	got := NDCG([]string{"a", "x", "b", "y", "c", "z"}, []string{"a", "b", "c", "d"}, 3)
	want := 1.5 / (1.0 + 1.0/math.Log2(3) + 0.5)
	approx(t, got, want, "NDCG")
}

// --- Property: all metrics in [0,1] ---

func TestMetricsInUnitInterval(t *testing.T) {
	t.Parallel()
	// property: for any results and expected, all metrics ∈ [0,1]
	f := func(results, expected []string, k int) bool {
		if k <= 0 {
			k = 5
		}
		if k > 20 {
			k = 20
		}
		p := PrecisionAtK(results, expected, k)
		r := RecallAtK(results, expected, k)
		m := MRR(results, expected)
		n := NDCG(results, expected, k)
		return p >= -eps && p <= 1+eps &&
			r >= -eps && r <= 1+eps &&
			m >= -eps && m <= 1+eps &&
			n >= -eps && n <= 1+eps
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("property failed: %v", err)
	}
}
