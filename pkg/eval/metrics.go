// Package eval provides information-retrieval quality metrics (Recall@K,
// Precision@K, MRR, NDCG@K) and a benchmark runner for evaluating okf
// search quality against a golden query set.
package eval

import (
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/superops-team/okf/pkg/query"
)

// toSet converts a slice to a lookup set.
func toSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[it] = struct{}{}
	}
	return m
}

// topN returns the first n items of results (or all if shorter).
func topN(results []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if n >= len(results) {
		return results
	}
	return results[:n]
}

// PrecisionAtK computes precision at cutoff k: relevant items in top-k
// divided by the actual number of results returned (capped at k).
// If results is empty, returns 0.
func PrecisionAtK(results []string, expected []string, k int) float64 {
	if len(results) == 0 {
		return 0
	}
	top := topN(results, k)
	if len(top) == 0 {
		return 0
	}
	exp := toSet(expected)
	hits := 0
	for _, r := range top {
		if _, ok := exp[r]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(top))
}

// RecallAtK computes recall at cutoff k: relevant items in top-k divided
// by the total number of expected relevant items. If expected is empty,
// returns 1.0 (no relevant docs to miss).
func RecallAtK(results []string, expected []string, k int) float64 {
	if len(expected) == 0 {
		return 1.0
	}
	top := topN(results, k)
	exp := toSet(expected)
	hits := 0
	for _, r := range top {
		if _, ok := exp[r]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(expected))
}

// MRR computes Mean Reciprocal Rank: 1 / rank of the first relevant
// result (1-based). If no relevant result is found, returns 0.
func MRR(results []string, expected []string) float64 {
	exp := toSet(expected)
	for i, r := range results {
		if _, ok := exp[r]; ok {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// dcg computes Discounted Cumulative Gain for binary relevance:
// sum_{i=1..n} rel_i / log2(i+1), where rel_i = 1 if result i is relevant.
func dcg(results []string, exp map[string]struct{}, k int) float64 {
	top := topN(results, k)
	score := 0.0
	for i, r := range top {
		if _, ok := exp[r]; ok {
			score += 1.0 / math.Log2(float64(i+2)) // i is 0-based, so i+2 = rank+1
		}
	}
	return score
}

// NDCG computes Normalized Discounted Cumulative Gain at cutoff k.
// expected is treated as the ideal ranking (descending relevance).
// Binary relevance is used (an item is either relevant or not).
// If expected is empty, returns 1.0. If IDCG is 0, returns 0.
func NDCG(results []string, expected []string, k int) float64 {
	if len(expected) == 0 {
		return 1.0
	}
	exp := toSet(expected)
	actual := dcg(results, exp, k)
	ideal := dcg(expected, exp, k) // ideal ranking = expected order
	if ideal == 0 {
		return 0
	}
	return actual / ideal
}

// ---------------------------------------------------------------------------
// Grouped-projection metrics (S47). These operate on the already-projected
// []query.GroupedHit produced by query.Project. They never re-run retrieval
// and never mutate the groups.
// ---------------------------------------------------------------------------

// normalizedEvalPath canonicalizes a relative file path for metric comparison:
// trimmed, cleaned, "/" separators. Empty stays empty. It does NOT enforce the
// query-package safety contract because these identifiers already come from
// loaded concepts; it only makes "/" and "\" consistent.
func normalizedEvalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// groupCoveredSource returns the normalized source/document identifier a group
// represents. The group representative's source_path is authoritative; when a
// concept has no source_path (e.g. the lexical benchmark bundle) the concept
// path (the generated document file) is used so it can be matched against
// FilePath-based golden expected docs.
func groupCoveredSource(g query.GroupedHit) string {
	if s := normalizedEvalPath(g.Representative.SourcePath); s != "" {
		return s
	}
	return normalizedEvalPath(g.Representative.ConceptPath)
}

// RelevantSourceRecallAtK is the fraction of relevant sources that are covered
// by at least one representative among the top-k groups. A source is covered when
// the normalized identifier of a top-k group equals it. If relevantSources is
// empty it returns 1.0 (no relevant sources to miss).
func RelevantSourceRecallAtK(groups []query.GroupedHit, relevantSources []string, k int) float64 {
	if len(relevantSources) == 0 {
		return 1.0
	}
	covered := make(map[string]struct{})
	for _, g := range topNGroups(groups, k) {
		if src := groupCoveredSource(g); src != "" {
			covered[src] = struct{}{}
		}
	}
	hits := 0
	for _, r := range relevantSources {
		if _, ok := covered[normalizedEvalPath(r)]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(relevantSources))
}

// GroupNDCG is NDCG@k over projected groups. relevance maps a group key to a
// graded gain (>= 0); groups absent from the map score 0. The ideal ranking
// truncates the k highest available gains. This is the group-level analogue of
// the binary document NDCG above; it is a separate function because Go has no
// overloading. If relevance is empty it returns 1.0; if no gain is available it
// returns 0.
func GroupNDCG(groups []query.GroupedHit, relevance map[string]float64, k int) float64 {
	if len(relevance) == 0 {
		return 1.0
	}
	actual := 0.0
	for i, g := range topNGroups(groups, k) {
		if gain := relevance[g.GroupKey]; gain > 0 {
			actual += gain / math.Log2(float64(i+2))
		}
	}
	gains := make([]float64, 0, len(groups))
	for _, g := range groups {
		if gain := relevance[g.GroupKey]; gain > 0 {
			gains = append(gains, gain)
		}
	}
	slices.Sort(gains)
	slices.Reverse(gains) // descending: best gains fill the first ideal slots
	if len(gains) > k {
		gains = gains[:k]
	}
	ideal := 0.0
	for i, gain := range gains {
		ideal += gain / math.Log2(float64(i+2))
	}
	if ideal == 0 {
		return 0
	}
	return actual / ideal
}

// Diversity is how evenly the raw hits are spread across groups: the number of
// (already distinct) group keys divided by the total hits they hold. 1 means
// every exposed hit is its own group; 1/n means all hits collapse into one.
// Returns 0 when there are no hits.
func Diversity(groups []query.GroupedHit) float64 {
	total := 0
	for _, g := range groups {
		total += g.HitCount
	}
	if total == 0 {
		return 0
	}
	return float64(len(groups)) / float64(total)
}

// SameSourceOccupancy is the share of all hits held by the single largest group
// (max group hit_count / total hits). It measures how much one source/concept
// monopolizes the result list; 1 means one group holds every hit. Returns 0 when
// there are no hits.
func SameSourceOccupancy(groups []query.GroupedHit) float64 {
	total := 0
	largest := 0
	for _, g := range groups {
		total += g.HitCount
		largest = max(largest, g.HitCount)
	}
	if total == 0 {
		return 0
	}
	return float64(largest) / float64(total)
}

// topNGroups returns the first n groups (or all when n <= 0 / n >= len).
func topNGroups(groups []query.GroupedHit, n int) []query.GroupedHit {
	if n <= 0 || n >= len(groups) {
		return groups
	}
	return groups[:n]
}
