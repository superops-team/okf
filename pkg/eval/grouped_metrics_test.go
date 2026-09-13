package eval

import (
	"math"
	"testing"

	"github.com/superops-team/okf/pkg/query"
)

// gh builds a minimal GroupedHit for metric tests. hitCount is the group size;
// source is written into the representative's SourcePath (and, when empty, the
// concept path is used via groupCoveredSource). coveredSources is the full set
// of source/concept identifiers the group contains (for multi-source groups
// like folder projections); when nil it defaults to {source}.
func gh(key string, hitCount int, source string, rank int, coveredSources ...string) query.GroupedHit {
	cs := coveredSources
	if len(cs) == 0 {
		cs = []string{source}
	}
	return query.GroupedHit{
		GroupKey:       key,
		HitCount:       hitCount,
		ConceptCount:   hitCount,
		CoveredSources: cs,
		Representative: query.ResultHit{
			Rank:        rank,
			SourcePath:  source,
			ConceptPath: source,
		},
	}
}

// --- RelevantSourceRecallAtK ---

func TestRelevantSourceRecallAtK_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		groups   []query.GroupedHit
		relevant []string
		k        int
		want     float64
	}{
		{
			name:     "empty relevant → 1.0",
			groups:   []query.GroupedHit{gh("a", 1, "src/a.md", 1)},
			relevant: nil,
			k:        5,
			want:     1.0,
		},
		{
			name: "all relevant covered in top-k",
			groups: []query.GroupedHit{
				gh("s:a", 2, "src/a.md", 1),
				gh("s:b", 1, "src/b.md", 2),
			},
			relevant: []string{"src/a.md", "src/b.md"},
			k:        5,
			want:     1.0,
		},
		{
			name: "only half covered",
			groups: []query.GroupedHit{
				gh("s:a", 2, "src/a.md", 1),
				gh("s:x", 1, "src/x.md", 2),
			},
			relevant: []string{"src/a.md", "src/b.md"},
			k:        5,
			want:     0.5,
		},
		{
			name: "k truncation drops a relevant group",
			groups: []query.GroupedHit{
				gh("s:x", 1, "src/x.md", 1),
				gh("s:a", 2, "src/a.md", 2),
				gh("s:b", 1, "src/b.md", 3),
			},
			relevant: []string{"src/a.md", "src/b.md"},
			k:        2,
			want:     0.5, // top-2 = x,a → only a covered
		},
		{
			name: "normalization mismatch ignored",
			groups: []query.GroupedHit{
				gh("s:a", 1, "src/a.md", 1),
			},
			relevant: []string{"src/other.md"},
			k:        5,
			want:     0.0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RelevantSourceRecallAtK(tc.groups, tc.relevant, tc.k)
			approx(t, got, tc.want, "RelevantSourceRecallAtK")
		})
	}
}

// TestRelevantSourceRecallAtK_MultiSourceFolder is the S47 regression test: a
// folder group contains multiple sources; the representative is one source but
// the relevant source is another member of the same folder. Recall must count
// the group as covering ALL its member sources, not just the representative.
func TestRelevantSourceRecallAtK_MultiSourceFolder(t *testing.T) {
	t.Parallel()
	// One folder group "folder:src" contains two concepts: a.md (representative,
	// rank 1) and b.md (member, rank 2). The relevant source is b.md.
	groups := []query.GroupedHit{
		gh("folder:src", 2, "src/a.md", 1, "src/a.md", "src/b.md"),
	}
	got := RelevantSourceRecallAtK(groups, []string{"src/b.md"}, 5)
	approx(t, got, 1.0, "folder group must cover all member sources, not just representative")

	// Two relevant sources in the same folder → both covered by one group.
	got = RelevantSourceRecallAtK(groups, []string{"src/a.md", "src/b.md"}, 5)
	approx(t, got, 1.0, "folder group covers both member sources")

	// Relevant source NOT in the folder → not covered.
	got = RelevantSourceRecallAtK(groups, []string{"src/c.md"}, 5)
	approx(t, got, 0.0, "source outside folder is not covered")
}

// --- GroupNDCG ---

func TestGroupNDCG_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		groups    []query.GroupedHit
		relevance map[string]float64
		k         int
		want      float64
	}{
		{
			name:      "empty relevance → 1.0",
			groups:    []query.GroupedHit{gh("a", 1, "a.md", 1)},
			relevance: map[string]float64{},
			k:         5,
			want:      1.0,
		},
		{
			name: "perfect ranking → 1.0",
			groups: []query.GroupedHit{
				gh("a", 1, "a.md", 1),
				gh("b", 1, "b.md", 2),
			},
			relevance: map[string]float64{"a": 1, "b": 1},
			k:         2,
			want:      1.0,
		},
		{
			name: "irrelevant first slot lowers NDCG",
			groups: []query.GroupedHit{
				gh("x", 1, "x.md", 1),
				gh("a", 1, "a.md", 2),
				gh("b", 1, "b.md", 3),
			},
			relevance: map[string]float64{"a": 1, "b": 1},
			k:         2,
			want:      (1.0 / math.Log2(3)) / (1.0 + 1.0/math.Log2(3)),
		},
		{
			name:      "no relevant in results → 0",
			groups:    []query.GroupedHit{gh("x", 1, "x.md", 1)},
			relevance: map[string]float64{"a": 1},
			k:         5,
			want:      0.0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GroupNDCG(tc.groups, tc.relevance, tc.k)
			approx(t, got, tc.want, "GroupNDCG")
		})
	}
}

func TestGroupNDCG_GradedRelevance(t *testing.T) {
	t.Parallel()
	// Gains 3,1,1. Ideal top-3 ordering: 3,1,1. Actual: 1,3,1.
	groups := []query.GroupedHit{
		gh("low", 1, "low.md", 1),
		gh("high", 1, "high.md", 2),
		gh("mid", 1, "mid.md", 3),
	}
	rel := map[string]float64{"low": 1, "high": 3, "mid": 1}
	actual := 1.0/math.Log2(2) + 3.0/math.Log2(3) + 1.0/math.Log2(4)
	ideal := 3.0/math.Log2(2) + 1.0/math.Log2(3) + 1.0/math.Log2(4)
	got := GroupNDCG(groups, rel, 3)
	approx(t, got, actual/ideal, "GroupNDCG graded")
}

// --- Diversity ---

func TestDiversity_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		groups []query.GroupedHit
		want   float64
	}{
		{
			name: "all hits in one group → 1/n",
			groups: []query.GroupedHit{
				gh("s", 4, "s.md", 1),
			},
			want: 0.25,
		},
		{
			name: "every hit its own group → 1.0",
			groups: []query.GroupedHit{
				gh("a", 1, "a.md", 1),
				gh("b", 1, "b.md", 2),
				gh("c", 1, "c.md", 3),
			},
			want: 1.0,
		},
		{
			name:   "empty → 0",
			groups: nil,
			want:   0.0,
		},
		{
			name: "balanced two groups",
			groups: []query.GroupedHit{
				gh("a", 2, "a.md", 1),
				gh("b", 2, "b.md", 2),
			},
			want: 0.5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Diversity(tc.groups)
			approx(t, got, tc.want, "Diversity")
		})
	}
}

// --- SameSourceOccupancy ---

func TestSameSourceOccupancy_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		groups []query.GroupedHit
		want   float64
	}{
		{
			name: "one group dominates → 1.0",
			groups: []query.GroupedHit{
				gh("a", 8, "a.md", 1),
				gh("b", 2, "b.md", 2),
			},
			want: 0.8,
		},
		{
			name: "even split → 0.5",
			groups: []query.GroupedHit{
				gh("a", 3, "a.md", 1),
				gh("b", 3, "b.md", 2),
			},
			want: 0.5,
		},
		{
			name:   "empty → 0",
			groups: nil,
			want:   0.0,
		},
		{
			name: "single group → 1.0",
			groups: []query.GroupedHit{
				gh("a", 5, "a.md", 1),
			},
			want: 1.0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SameSourceOccupancy(tc.groups)
			approx(t, got, tc.want, "SameSourceOccupancy")
		})
	}
}
