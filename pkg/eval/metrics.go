// Package eval provides information-retrieval quality metrics (Recall@K,
// Precision@K, MRR, NDCG@K) and a benchmark runner for evaluating okf
// search quality against a golden query set.
package eval

import "math"

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
