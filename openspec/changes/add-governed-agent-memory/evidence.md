# Evidence — add-governed-agent-memory

Spec commit: 8712bb7. Implementation branch: spec/governed-agent-memory.
Verification date: 2026-09-16. Go version: go1.26.0 linux/amd64.

## 1. Golden memory_check set (S20-S27)

- Test: `TestCheckMemoryGolden` in `pkg/memorymeta/duplicate_golden_test.go`
- Cases: 44 (≥40 required)
- Confusion matrix: TP=21, FP=0, TN=23, FN=0
- Precision=1.000 (gate ≥0.85) ✓
- Recall=1.000 (gate ≥0.70) ✓
- FPR=0.000 (gate ≤0.15) ✓
- Covers: rephrase, partial overlap, cross-language (Chinese per-character tokens), common-word negatives, code-identifier negatives, single-character high-frequency negatives.
- Each case prints expected/predicted/status/score for auditability.

## 2. Token regression (S28-S34)

Corpus: `.okf/knowledge` 327 manifest items (329 files, 2 filtered by reader).
Measurement: `okf tool manifest --mode <m> --json --limit 500` stdout bytes (Go encoding/json default serialization).

| mode | stdout bytes | tokens (÷4) | vs full reduction |
|---|---:|---:|---:|
| full | 249,664 | 62,416 | baseline |
| summary | 84,482 | 21,120 | **66.2%** (gate ≥50%) ✓ |
| hit | 130,859 | 32,714 | **47.6%** (gate ≥40%) ✓ |

Item-only `estimated_item_tokens` (S34 definition: item bytes/4, no envelope):
- full=47,106, summary=27,953 (40.7%↓), hit=32,611 (30.8%↓).

Note: initial implementation had summary 38.9% / hit 24.2% because `ManifestItem` fields lacked `omitempty`. Fixed by adding `omitempty` to identity_state/path/status/trust_tier/stale/source_count/file_size_bytes/estimated_tokens/estimate_kind. Full mode with populated concepts remains byte-identical (S30 test passes).

## 3. For-path precision/recall (S11-S15)

Fixture: 20 concepts with code_refs (exact, `*.go`, `**`, `*_test.go`, nested dirs). 10 query paths, hand-labeled ground truth.

| metric | value |
|---|---|
| Micro Precision | 1.000 |
| Micro Recall | 1.000 |
| Macro Precision | 1.000 |
| Macro Recall | 1.000 |

All 10 queries: P=1.0, R=1.0. Covers `**` recursion, `*` segment match, `*_test.go` suffix, cross-directory nesting.

## 4. 1000-concept memory_check benchmark (S22)

Go benchmark: `BenchmarkCheckMemory1000Miss` / `BenchmarkCheckMemory1000Hit` in `pkg/memorymeta/duplicate_benchmark_test.go`.

| benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 1000 miss | 21,914,476 (~21.9ms) | 9,157,697 | 257,083 |
| 1000 hit | 24,159,495 (~24.2ms) | 9,252,386 | 258,020 |

E2E CLI: `okf tool query -q "test" --memory-check` on 1000 durable notes → ~68ms wall time (3 runs: 68/67/70ms).

## 5. Race test

```
go test -race ./pkg/memorymeta/... ./pkg/manifest/... ./pkg/tool/... ./pkg/mcp/...
ok  pkg/memorymeta  1.148s
ok  pkg/manifest    2.520s
ok  pkg/tool       17.328s
ok  pkg/mcp         5.295s
```
No data races.

## 6. Shuffle test

```
go test -shuffle=on ./pkg/memorymeta/... ./pkg/manifest/... ./pkg/tool/... ./pkg/mcp/...
ok  pkg/memorymeta  0.064s
ok  pkg/manifest    0.661s
ok  pkg/tool        13.308s
ok  pkg/mcp         4.506s
```
No test-order coupling.

## 7. MCP E2E (S36-S39)

- `python3 test_mcp.py` (legacy 2024-11-05): 13/13 PASS. tools/list = 20 legacy tools.
- `python3 test_ext_skills.py` (modern 2026-07-28 + skills): 8/8 PASS. tools/list = 11 modern tools.
- Both eras: okf_manifest/okf_query/okf_context gain new optional params; no tool removed.

## 8. Real Codex governed flow (S39)

Isolated `CODEX_HOME=/tmp/codex_home`, config registers `[mcp_servers.okf]` → `okf mcp --repo . --dir .okf/knowledge`. sandbox=read-only, approval=never, model=gpt-5.6-sol__dev (xeart channel).

4 read-only tasks, 0 mutating calls:
1. tools/list → 21 okf tools visible.
2. `okf_manifest(mode=summary, limit=5)` → 5/327 items returned.
3. `okf_manifest(for_path=pkg/memorymeta/duplicate.go, mode=hit)` → 0 matches (329 corpus has no code_refs-declaring concepts; independent fixture in §3 verifies matching).
4. `okf_query(q="memory check duplicate", memory_check=true)` → `no_similar`, 0 candidates @ threshold 0.20.

All calls ok=true, mutating=false. No note/log/feedback/import/refresh/init invoked. Raw event stream not committed (contains model output); safe summary above.

## 9. Full test suite

```
go build ./... → PASS
go test -count=1 ./pkg/... ./cmd/... → all PASS
go vet ./... → clean
gofmt -l → clean (after fix)
```

Packages: memorymeta, manifest, tool, mcp, agentconfig, cmd/okf, identity, lexical, query, vectorindex, eval, git, convert, okf, parser, chunk, lint, dashboard.

## 10. Post-code review

Round 1 (explicit): compilation, interfaces, logic, error handling, naming, data flow — no critical/high issues found. Token reduction gap (summary 38.9%/hit 24.2%) identified and fixed via omitempty.

Round 2 (implicit): boundary conditions, protocol understanding, resource handling, spec compliance, security, state — no critical/high issues. Governance default=context verified; hold advisory-only verified; for_path lexical-no-FS verified; stale-refs fail-closed verified; memory_check read-only verified; Concept struct unchanged verified.

## 11. Hard constraints compliance

| Constraint | Status |
|---|---|
| Concept struct unchanged | ✓ (governance/code_refs in CustomFields only) |
| No second index/fact source | ✓ (reuses existing BM25 per-call, manifest existing scan) |
| Default governance=context, no path inference | ✓ |
| Hold advisory only, no server block | ✓ |
| CLI hyphen / MCP underscore | ✓ |
| memory_check read-only, no conflict/allow_duplicate/audit | ✓ |
| for_path lexical, no FS, no EvalSymlinks | ✓ |
| stale-refs only FS scan, fail-closed | ✓ |
| W01-W07 each exactly once | ✓ (TestCanonicalWorkflowCoverage) |
| Modern 11 / legacy 20 tools | ✓ (E2E verified) |
| No push/PR/merge | ✓ (local commit only) |
