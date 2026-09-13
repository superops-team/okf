# Evidence — add-agent-knowledge-discovery (fresh run)

- Implementation source commit: `3c7f58caba08efccf938648f609573370e740ff5` (docs: S46 amendment Approved)
- Evidence commit: this file + conformance.md committed separately after fresh run (two-SHA split avoids circular claim)
- Verification timestamp (UTC): 2026-09-13T00:17:06Z (verify-discovery); gauntlet run immediately after (EXIT=0)
- Toolchain: go version go1.26.7 linux/amd64
- Go/tool versions: go1.26.7 / toolchain auto
- One-command entry point: `tools/verify-agent-discovery.sh`
- All numbers below come from this run (not reused).
- S46 spec-amendment: **Approved** by user explicit approval 2026-09-13 (see `spec-amendment.md`). Approved revision supersedes original S46 baseline clause; base-vs-head zero-regression proof retained.

## Build / versions
- `go build -o okf ./cmd/okf`: OK
- okf version: okf version 0.7.0 (built with 0.4.1)

## S06/S07 identity migration
- dry-run missing=3, two consecutive dry-run JSON files byte-identical: PASS
- first apply missing=3; second apply missing=0 (idempotent): PASS
- assigned okf_id: `okf_<redacted>` (actual value not persisted to evidence)

## S11 stable ref resolution across entries
- renamed alpha.md -> alpha-renamed.md; `okf identity resolve` -> path=alpha-renamed.md: PASS
- unknown ref -> `concept_ref_not_found`: PASS
- MCP `okf_resolve` -> path="alpha-renamed.md"; `okf_manifest` total=3 (CLI/MCP path parity): PASS

## S17/S18/S20/S21/S26 Manifest
- CLI `okf tool manifest`: total=3, default limit=100, items with estimate_kind=file_bytes_div4: 3
- Markdown body leak check: no body content in manifest output: PASS
- limit=999 rejected with `invalid_request`: PASS
- MCP `okf_manifest` total=3 (Service/CLI/MCP parity): PASS

## S27/S30/S31/S34 grouped retrieval
- group-by source groups: 2
- ungrouped search (S27) still runs: PASS
- invalid group-by 'bogus' rejected: PASS

## S35-S45 project agent integration
- clients plan->apply->apply(zero diff)->status->remove: cursor=ok claude-code=ok codex=ok
- no credential-like field in generated reports: PASS
- unsupported client rejected: PASS

## S46/S47 retrieval eval
- golden set: pkg/eval/testdata/golden_semantic.json (28 cases, natural-language; lexical-substring scores Recall@5=0.0769)
- hybrid-default (S46 baseline): Recall@5=0.9231, MRR=0.6615
- semantic-only: Recall@5=0.9231
- reproducible baseline (spec-amendment): hybrid Recall@5≈0.92, MRR≈0.66 on current tree with chunk-level index; historical 0.9615/0.7256 superseded (see spec-amendment.md)
- base-vs-head: identical with same content (zero code regression from v3 key change)
- S47 grouped eval (hybrid strategy, all three projections):
- concept: srcRecall=0.9231 ndcg=0.8021 diversity=1.0000 occupancy=0.2250
- source: srcRecall=0.9231 ndcg=0.8021 diversity=1.0000 occupancy=0.2250
- folder: srcRecall=0.9231 ndcg=1.0000 diversity=0.2250 occupancy=1.0000

## S48 resource bounds (1,000 files)
```
BENCH files=1000 body_bytes_on_disk=262144000 frontmatter_bytes=51000 bytes_read=4096000 per_file_prefetch=4096
```
- Interpretation: 250 MiB of Markdown bodies on disk; the Manifest read only
  bytes_read (frontmatter + one 4 KiB prefetch per file). Bodies are never streamed.
- Projection is O(n) over the bounded scored candidate window; path-escape and
  representative-score invariants are enforced by unit/property tests
  (TestProjectFolder, TestProjectDeterministic, property suite).

## S49 quality gate
- `go build ./...`, `go vet ./...`: PASS (precondition)
- targeted mutation set: see tools/mutants-agent-discovery.sh (5/5 killed)
- this script itself: PASS

## Full validation summary (fresh run, implementation commit 3c7f58c)

| Suite | Result | Key numbers |
|-------|--------|-------------|
| `tools/verify-agent-discovery.sh` | PASS (exit 0) | identity/manifest/grouped/agent/eval/benchmark all green |
| `tools/verify-agent-usability.sh` | **156 passed, 0 failed, 0 skipped** | A1–G4 all executed; zero-skip enforced |
| `tools/gauntlet.sh` | PASS (exit 0) | build/vet/staticcheck/tests/race/coverage(69%)/shuffle/property(20)/secret-scan/mod-verify/mutation(18/18+5/5)/real-exec |
| `test_mcp.py` (MCP E2E) | ALL TESTS PASSED (13/13) | Content-Length stdio protocol |
| `go test ./...` | all green | — |
| `go test -race` (identity/agentconfig/manifest/query) | clean | — |
| `go test -shuffle=on` | no order dependency | — |

## S46/S47 retrieval eval (approved baseline)
- hybrid-default (S46, approved reproducible baseline): Recall@5=0.9231, MRR=0.6615
- base-vs-head (aaafcbb vs implementation, same content/toolchain/index config): identical → zero code regression
- machine gate `TestHybridBaselineGate_ChunkLevel`: Recall@5≥0.90, MRR≥0.60; negative controls (broken semantic channel / wrong weights) fail gate
- S47 grouped eval (hybrid, all projections ≥ raw): concept/source/folder srcRecall all =0.9231

