# Proposal: IR Evaluation Benchmark

## Problem

okf currently proves search *correctness* (functional assertions: a query returns the right set) but cannot quantify search *quality*. There is no Recall@K, Precision@K, MRR, or NDCG@K, no golden query set, and no reproducible eval command. This means:

- We cannot detect regressions in ranking quality (e.g. a change that still returns the right docs but pushes the best one to rank 5).
- We cannot compare retrieval strategies (indexed vs free-text, future BM25/vector) on a common baseline.
- The 7-format document import feature (PR #8) verified searchability manually, but that verification is not a persisted, repeatable benchmark.

## Goal

Add a minimal, pure-Go IR evaluation layer that:

1. Implements the four canonical IR metrics (Recall@K, Precision@K, MRR, NDCG@K) as pure, well-tested functions.
2. Provides a golden query benchmark set (20 cases over the 7 existing document fixtures) with expected relevant documents in relevance order.
3. Runs the benchmark against a real okf knowledge bundle (built from the 7 fixtures) and outputs a machine-readable + human-readable report.
4. Exposes a one-command entrypoint (`tools/eval.sh`) so the baseline is reproducible in CI and by any developer.

## Non-Goals

- No LLM-as-judge metrics (faithfulness, answer relevancy) — okf has no generation layer; those belong to a future RAG integration.
- No vector/BM25 retrieval — this change only *measures* the existing `query.Search`; it does not change the retrieval algorithm.
- No new document fixtures — reuse the 7 fixtures already committed under `pkg/convert/testdata/`.
- No external dependencies — metrics implemented with the standard library only.

## Success Criteria

- `tools/eval.sh` exits 0 and prints baseline scores for all four metrics (K=5).
- Every metric function has unit tests covering: empty results, all relevant, none relevant, K > result count, duplicate entries, multi-hit queries.
- The golden set includes at least: single-hit, multi-hit, zero-hit (negative), table-content, list-content, title-match, body-match cases.
- `conformance.md` maps every spec Scenario to a test.
- Full gauntlet (`tools/gauntlet.sh`) passes.
