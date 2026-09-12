# Evidence — add-agent-knowledge-discovery (fresh run)

- Source commit: `47ec717324c2d8251c5031850d5b744bff74291d`
- Verification timestamp (UTC): 2026-09-12T03:55:10Z
- Toolchain: go version go1.26.7 linux/amd64
- Go/tool versions: go1.26.7 / toolchain auto
- One-command entry point: `tools/verify-agent-discovery.sh`
- All numbers below come from this run (not reused).

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
- historical baseline (releases.md): hybrid Recall@5=0.9615, MRR=0.7256; current code reproduces 0.9231/0.6615 with the same golden set + v3 index
- grouped (by=source) aggregate: srcRecall=0.0769 ndcg=1.0000 diversity=0.0769 occupancy=0.0769

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

