# Conformance: IR Evaluation Benchmark

**Change:** add-ir-eval-benchmark
**Spec:** specs/ir-eval-benchmark/spec.md
**Date:** 2026-09-01
**Baseline commit:** feat/ir-eval-benchmark

## Summary

| Requirement | Scenarios | Mapped | Status |
|---|---|---|---|
| IR Metric Functions | 10 | 10 | fully |
| Golden Query Benchmark Set | 4 | 4 | fully |
| Benchmark Runner | 4 | 4 | fully |
| Reproducible Eval Command | 2 | 2 | fully |

**Overall: 20/20 scenarios fully aligned.**

## Requirement: IR Metric Functions

| # | Scenario | Test | Status |
|---|---|---|---|
| 1 | Precision@K fewer results than K → 1.0 | `TestPrecisionAtK_FewerResultsThanK` | fully |
| 2 | Precision@K mixed relevance → 2/3 | `TestPrecisionAtK_MixedRelevance` | fully |
| 3 | Precision@K empty results → 0 | `TestPrecisionAtK_EmptyResults` | fully |
| 4 | Precision@K none relevant → 0 | `TestPrecisionAtK_NoneRelevant` | fully |
| 5 | Recall@K partial recall → 0.5 | `TestRecallAtK_PartialRecall` | fully |
| 6 | Recall@K empty expected → 1.0 | `TestRecallAtK_EmptyExpected` | fully |
| 7 | Recall@K all recalled → 1.0 | `TestRecallAtK_AllRecalled` | fully |
| 8 | Recall@K empty results → 0 | `TestRecallAtK_EmptyResults` | fully |
| 9 | MRR first relevant at rank 3 → 1/3 | `TestMRR_FirstRelevantAtRank3` | fully |
| 10 | MRR first relevant at rank 1 → 1.0 | `TestMRR_FirstRelevantAtRank1` | fully |
| 11 | MRR no relevant → 0 | `TestMRR_NoRelevant` | fully |
| 12 | MRR empty results → 0 | `TestMRR_EmptyResults` | fully |
| 13 | NDCG perfect ranking → 1.0 | `TestNDCG_PerfectRanking` | fully |
| 14 | NDCG irrelevant doc pushes relevant down → (0,1) | `TestNDCG_IrrelevantDocPushesRelevantDown` | fully |
| 15 | NDCG all-relevant reorder is 1.0 (binary relevance) | `TestNDCG_AllRelevantReorderedIsOne` | fully |
| 16 | NDCG empty expected → 1.0 | `TestNDCG_EmptyExpected` | fully |
| 17 | NDCG no relevant in results → 0 | `TestNDCG_NoRelevantInResults` | fully |
| 18 | NDCG partial top-K exact value | `TestNDCG_PartialTopK` | fully |
| 19 | All metrics ∈ [0,1] (property, 500 random cases) | `TestMetricsInUnitInterval` | fully |

## Requirement: Golden Query Benchmark Set

| # | Scenario | Test | Status |
|---|---|---|---|
| 20 | Golden set covers all 7 formats | `TestLoadGoldenCases` (formats map check) | fully |
| 21 | Golden set includes negative (zero-result) cases | `TestLoadGoldenCases` (hasNegative check) | fully |
| 22 | Golden set includes multi-hit cases | `TestLoadGoldenCases` (hasMultiHit check) | fully |
| 23 | Golden set includes table-content queries (apple/bar/alpha) | golden set content + `TestEvalBenchmark` per-case | fully |
| 24 | Golden set includes list-content queries (item one/item two) | golden set content + `TestEvalBenchmark` per-case | fully |

**Golden set composition:** 20 cases = 7 single-hit (one per format) + 5 multi-hit + 3 table-content + 2 list-content + 1 title-match + 2 negative.

## Requirement: Benchmark Runner

| # | Scenario | Test | Status |
|---|---|---|---|
| 25 | Benchmark produces aggregate scores (all 4 metrics) | `TestEvalBenchmark` (report.Aggregate assertions) | fully |
| 26 | Positive queries Recall@5 ≥ 0.5 | `TestEvalBenchmark` (per-case recall assertion) | fully |
| 27 | Negative queries return 0 results | `TestEvalBenchmark` (per-case negative 0-results assertion) | fully |
| 28 | Report includes per-case detail (query, 4 metrics, top1) | `TestEvalBenchmark` (report.String() output via t.Log) | fully |
| 29 | Empty bundle handled without panic | `TestRunBenchmarkEmptyBundle` | fully |
| 30 | Aggregate MRR > 0.7 (baseline quality gate) | `TestEvalBenchmark` (report.Aggregate.MRR > 0.7) | fully |
| 31 | Positive aggregate Recall@5 ≥ 0.8 | `TestEvalBenchmark` (report.AggregateNonNegative.Recall ≥ 0.8) | fully |

## Requirement: Reproducible Eval Command

| # | Scenario | Verification | Status |
|---|---|---|---|
| 32 | `tools/eval.sh` exits 0 and prints full report | Manual: `GO=<go1.26> tools/eval.sh` → exit 0, report printed | fully |
| 33 | `tools/eval.sh` is deterministic (no randomness/network) | Manual: two consecutive runs produce identical scores; benchmark uses committed fixtures + golden set, no network | fully |

## Baseline Scores (actual run, K=5, 20 cases)

```
Metric          All cases   Positive
Recall@K           1.0000     1.0000
Precision@K        0.9000     1.0000
MRR                0.9000     1.0000
NDCG@K             1.0000     1.0000
```

- 18/18 positive queries: correct top-1, Recall=Precision=MRR=NDCG=1.0
- 2/2 negative queries: 0 results
- All-cases Precision/MRR = 0.9 because 2 negative cases (Precision=0, MRR=0) pull down the 20-case average; positive-only is 1.0

## Known Limits

1. **Binary relevance only:** NDCG uses binary relevance (relevant/not). Graded relevance (e.g. title-match > body-match) is not modeled — this is a deliberate minimal-implementation choice. Adding graded relevance would require extending `EvalCase.ExpectedDocs` to weighted pairs.
2. **Small fixture set:** 7 small documents (~1-2 paragraphs each). The benchmark proves correctness of the metric pipeline and establishes a baseline, but does not stress-test ranking on large corpora or ambiguous queries. A larger corpus benchmark is future work.
3. **okf Search is unordered:** `query.Search` returns results in bundle insertion order (no relevance ranking). MRR=1.0 for all positive queries reflects that the first result happens to be correct for these fixtures, not that a ranking algorithm is at work. The benchmark will become more meaningful when/if okf adds ranked retrieval.
4. **No LLM-as-judge metrics:** Faithfulness, answer relevancy, groundedness are not included because okf has no generation layer. These belong to a future RAG integration.

## Implementation Files

| File | Purpose |
|---|---|
| `pkg/eval/metrics.go` | Pure IR metric functions (PrecisionAtK, RecallAtK, MRR, NDCG) |
| `pkg/eval/metrics_test.go` | 19 unit tests + 1 property test (500 random cases) |
| `pkg/eval/benchmark.go` | EvalCase, GoldenSet, CaseResult, EvalReport, LoadGoldenCases, RunBenchmark, String() |
| `pkg/eval/benchmark_test.go` | End-to-end benchmark + golden set validation + empty-bundle edge case |
| `pkg/eval/testdata/golden_queries.json` | 20 golden cases (committed, deterministic) |
| `tools/eval.sh` | One-command reproducible benchmark entrypoint |
