# Spec Amendment — S46 Retrieval Baseline Clarification

Date: 2026-09-12
Status: **Proposed — pending user approval** (not approved; executor cannot self-approve)
Affects: S46 only (S47 was a code bug, fixed per original spec — no amendment needed)

## Approval status

This document is a **proposed** amendment. Until the user explicitly approves
it, the original spec S46 ("hybrid Recall@5 and MRR are not lower than the
committed baseline") remains authoritative. The implementation satisfies the
original spec via base-vs-head comparison (see below); the amendment only
seeks to clarify which baseline numbers are reproducible from the current tree.
S46 conformance is marked `partial` until this amendment is approved.

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

4. **S47 was a code bug, not a spec issue.** `RunGroupedBenchmark` defaulted
   to `DefaultStrategy` (lexical), and `RelevantSourceRecallAtK` only checked
   the group representative's source, not all group members. Both fixed per
   original spec S47: grouped eval now uses hybrid strategy and checks all
   group members' sources. No amendment needed for S47.

## Original-spec compliance (no amendment needed)

- **S47**: Fixed per original spec. Grouped eval uses hybrid raw-candidate
  strategy; `RelevantSourceRecallAtK` checks all group members' covered
  sources. concept/source/folder all achieve srcRecall=0.9231 = raw hybrid
  Recall@5=0.9231.

## Proposed S46 amendment (pending approval)

### S46 (proposed revision)

"Hybrid Recall@5 and MRR are not lower than the reproducible committed
baseline. The reproducible baseline on the current tree (docs/knowledge +
golden_semantic.json + chunk-level vector index + hybrid-default weights) is
Recall@5≈0.92, MRR≈0.66. A machine-enforced chunk-level gate
(`TestHybridBaselineGate_ChunkLevel`) replicates the exact CLI pipeline
(chunking→embedding→v3 chunk key→HNSW→BM25+semantic hybrid→golden metrics)
and asserts Recall@5≥0.90, MRR≥0.60, with a committed baseline artifact
(`testdata/hybrid_baseline_chunk_level.json`). The historical 0.9615/0.7256
figures in releases.md were measured at an earlier point with different
knowledge-base content and are not reproducible from the current tree."

### Why original spec is still met without amendment

Base-vs-head worktree comparison (aaafcbb vs. implementation branch, same
docs/knowledge content, same toolchain, same golden set, same index build
config) yields **identical** hybrid Recall@5=0.9231 and MRR=0.6615. The v3
key change is cosmetic for legacy concepts (all receive same `v3:legacy:`
prefix, so tie-break order is preserved) and the HNSW exact-search path
iterates all vectors regardless of key. Therefore "before/after does not
decline" is satisfied by executable proof, even though the absolute numbers
differ from the historical releases.md figure.

## Verification

- Base-vs-head: identical results with same content (Recall@5=0.9231,
  MRR=0.6615) — zero code regression from v3 key change.
- `TestHybridBaselineGate_ChunkLevel`: PASS (Recall@5=0.9231, MRR=0.6615,
  exact CLI chunk-level pipeline; baseline artifact committed).
- Negative controls: broken semantic channel (vector weight=0) fails gate;
  inverted weights produce different metrics.
- S47 grouped eval (hybrid, all three projections): concept srcRecall=0.9231,
  source srcRecall=0.9231, folder srcRecall=0.9231 (fixed from 0.4038 by
  checking all group members' covered sources). All ≥ raw hybrid Recall@5.
- Three consecutive CLI rebuilds: identical hybrid metrics (deterministic).
