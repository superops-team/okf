# Design: IR Evaluation Benchmark

## Architecture

```
pkg/eval/
├── metrics.go              # pure IR metric functions (Recall@K, Precision@K, MRR, NDCG@K)
├── metrics_test.go         # unit tests for every metric (boundaries + invariants)
├── benchmark.go            # EvalCase, EvalReport, RunBenchmark (loads golden set, runs query.Search, scores)
├── benchmark_test.go       # end-to-end: build bundle from 7 fixtures, run 20 cases, assert baseline bounds
└── testdata/
    └── golden_queries.json # 20 golden cases: {query, expected_docs (relevance order)}
```

`tools/eval.sh` is a thin wrapper: `go test ./pkg/eval -run TestEvalBenchmark -v` that prints the report.

## Key Design Decisions

### 1. New package `pkg/eval`, not extending `pkg/query`

`pkg/query` owns retrieval. `pkg/eval` owns *measuring* retrieval. Separating them keeps `query` minimal and avoids a circular dependency (eval imports query; query must not import eval). This matches the existing `pkg/convert` (conversion) vs `pkg/okf` (core) separation pattern.

### 2. Metrics operate on `[]string` (doc identifiers), not `[]*Concept`

IR metrics are pure ranking functions. Taking `[]string` (document titles/resources) makes them:
- Trivially unit-testable without building a bundle.
- Reusable for any future retrieval backend (BM25, vector) — just map results to strings.
- Free of coupling to `query.Concept` struct.

The benchmark runner maps `query.Search` results to `Concept.Resource` (e.g. `sample.pdf.md`) before scoring.

### 3. Golden set format: JSON with `expected_docs` in relevance order

```json
{
  "cases": [
    {"query": "banana", "expected_docs": ["sample.xlsx.md"]},
    {"query": "OKF", "expected_docs": ["sample.pdf.md", "sample.docx.md", "sample.pptx.md", "sample.txt.md"]}
  ]
}
```

- `expected_docs` is ordered by descending relevance. This is required for NDCG (ideal ranking = expected order) and MRR.
- A case with `expected_docs: []` is a negative query (zero results expected).
- Document identifiers match `Concept.Resource` after import (e.g. `sample.pdf.md`), consistent with the existing `TestCmdAddDocumentsSearchable`.

### 4. Metric semantics (canonical definitions)

- **Precision@K**: `|relevant ∩ topK| / K`. If fewer than K results, denominator is actual result count (not K) — this is the standard "precision at cutoff" definition used by trec_eval.
- **Recall@K**: `|relevant ∩ topK| / |relevant|`. If relevant set is empty, recall = 1.0 by convention (no relevant docs to miss).
- **MRR**: `1 / rank_of_first_relevant`, where rank is 1-based. If no relevant result, MRR = 0.
- **NDCG@K**: `DCG@K / IDCG@K`, where `DCG = sum_{i=1..K} rel_i / log2(i+1)`, `rel_i = 1` if result i is in expected set (binary relevance), `IDCG` = DCG of the ideal ranking (expected_docs truncated to K). If expected set empty, NDCG = 1.0.

### 5. Benchmark report

`EvalReport` contains per-case scores + aggregate (mean over all cases, and mean over non-negative cases only). Output format:

```
=== IR Eval Benchmark (K=5, 20 cases) ===
Metric        All cases   Non-negative
Recall@5      0.95        0.95
Precision@5   0.76        0.76
MRR           0.88        0.88
NDCG@5        0.85        0.85
---
Per-case details (query → recall/precision/mrr/ndcg, top1):
  banana → 1.00/1.00/1.00/1.00 top1=sample.xlsx.md
  ...
```

### 6. Baseline assertion strategy

The end-to-end test (`TestEvalBenchmark`) does **not** assert exact scores (fragile to retrieval changes). Instead it asserts:
- All positive queries have Recall@5 ≥ 0.5 (at least half the expected docs are in top 5).
- All negative queries return 0 results (Recall=1.0, Precision=1.0 by convention, MRR=0).
- The aggregate MRR > 0.7 (the existing search is already quite good on these fixtures).
- The report is non-empty and well-formed.

This gives a regression gate without overfitting to exact numbers. The actual baseline scores are printed and documented in `docs/knowledge/releases.md`.

### 7. Reproducibility

- `tools/eval.sh` is the single entrypoint (consistent with `tools/gauntlet.sh`).
- The golden set is committed JSON, not generated at runtime.
- The 7 fixtures are already committed under `pkg/convert/testdata/` (reused, not duplicated).
- No network, no LLM, no randomness — deterministic.

## File Change Summary

| File | Change | Reason |
|---|---|---|
| `pkg/eval/metrics.go` | new | IR metric pure functions |
| `pkg/eval/metrics_test.go` | new | unit tests (boundaries, invariants) |
| `pkg/eval/benchmark.go` | new | EvalCase/Report/RunBenchmark |
| `pkg/eval/benchmark_test.go` | new | end-to-end on 7 fixtures + 20 cases |
| `pkg/eval/testdata/golden_queries.json` | new | golden query set |
| `tools/eval.sh` | new | one-command eval entrypoint |
| `README.md` | modify | add eval section |
| `docs/knowledge/releases.md` | modify | v1.3.0 baseline scores |
