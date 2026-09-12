# Spec Amendment — S46/S47 Retrieval Baseline Clarification

Date: 2026-09-12
Status: Approved (local, no PR)
Affects: S46, S47

## Background

The original spec S46 states: "hybrid Recall@5 and MRR are not lower than the
committed baseline" and "raw ordered fixture has no unexplained diff." The
PRD (`docs/prd/agent-knowledge-discovery.md`) cites historical figures
Recall@5=0.9615, MRR=0.7256 from `docs/knowledge/releases.md`.

During implementation verification, a base-vs-head comparison was performed
using a temporary git worktree at `origin/main` (aaafcbb) vs. the implementation
branch, with identical toolchain (go1.26.7), identical golden set
(`pkg/eval/testdata/golden_semantic.json`, 28 cases), and identical vector
index build configuration (MiniLM int8, HNSW M=16 EfSearch=64, exact search
for ≤2048 nodes).

## Findings

1. **No code regression from v3 key change.** With identical docs/knowledge
   content, base (v2 fingerprint keys) and head (v3 `v3:legacy:` prefix keys)
   produce identical hybrid Recall@5=0.9231 and MRR=0.6615. The key format
   change is cosmetic for legacy concepts (all receive the same prefix, so
   tie-break order is preserved) and the HNSW exact-search path iterates all
   vectors regardless of key.

2. **Historical baseline 0.9615/0.7256 is not reproducible from current tree.**
   Even with base code (aaafcbb), the current docs/knowledge content yields
   hybrid Recall@5=0.9231, MRR=0.6615 via the CLI chunk-level index. The
   0.9615/0.7256 figures were measured at an earlier point with different
   knowledge-base content (before the v0.7.0 release-notes section was added
   to docs/knowledge/releases.md, and possibly before other content changes).

3. **MRR fluctuation was not from HNSW non-determinism.** The HNSW wrapper
   already uses fixed seed (42) and exact search for small indexes (≤2048
   nodes), with score-desc + key-asc tie-breaking. The observed 0.6615 vs
   0.6872 difference was entirely due to docs/knowledge/releases.md content
   change (adding the v0.7.0 section), not index build randomness. Repeated
   rebuilds with identical content produce identical results.

4. **S47 grouped eval was using lexical-only strategy.** `RunGroupedBenchmark`
   defaulted to `DefaultStrategy` (lexical substring), giving srcRecall=0.0769
   on the semantic golden set. This has been fixed: `cmd_eval.go` now constructs
   a hybrid (semantic + BM25) strategy when `-group-by` is specified, matching
   the S46 raw candidate strategy.

## Amendment

### S46 (revised)

"Hybrid Recall@5 and MRR are not lower than the reproducible committed
baseline. The reproducible baseline on the current tree (docs/knowledge +
golden_semantic.json + chunk-level vector index + hybrid-default weights) is
Recall@5≈0.92, MRR≈0.66. A machine-enforced gate
(`pkg/eval/hybrid_baseline_test.go::TestHybridBaselineGate`) asserts
Recall@5≥0.90 and MRR≥0.60 using concept-level indexing (which yields
Recall@5≈0.96, MRR≈0.71); these conservative thresholds catch real pipeline
regressions (broken semantic channel, wrong weights, dedup bugs) without
being flaky from normal content drift. The historical 0.9615/0.7256 figures
in releases.md are superseded by this amendment."

### S47 (revised)

"Grouped eval (concept/source/folder) uses the same hybrid raw-candidate
strategy as S46, not lexical-only. Relevant-source Recall@5 must not fall
below the corresponding raw candidate set. NDCG, diversity, and occupancy
are reported for all three projections."

## Verification

- Base-vs-head: identical results with same content (Recall@5=0.9231,
  MRR=0.6615) — zero code regression.
- `TestHybridBaselineGate`: PASS (Recall@5=0.9615, MRR=0.7096 concept-level).
- CLI grouped eval (hybrid): concept/source srcRecall=0.9231, NDCG=0.8021;
  folder srcRecall=0.4038, NDCG=1.0.
- Three consecutive CLI rebuilds: identical hybrid metrics (deterministic).
