# Evidence — add-governed-agent-memory

Spec commit: 8712bb7. Implementation branch: spec/governed-agent-memory.
Implementation commits: df66533 (P0-P4), ad56bd8 (targeted coverage and compatibility fixes), 3af326c (current-source legacy E2E hardening and final audit corrections).
Final GAUNTLET source state: 3af326c. Verification date: 2026-09-16. Go version: go1.26.0 linux/amd64.

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
| full | 266,341 | 66,585 | baseline (byte-identical to pre-change) |
| summary | 84,482 | 21,120 | **68.3%** (gate ≥50%) ✓ |
| hit | 130,859 | 32,714 | **50.9%** (gate ≥40%) ✓ |

**Implementation note**: Initial approach added `omitempty` to 9 existing ManifestItem fields, which reduced full-mode output for zero-valued fields (broke S30 byte-compatibility). Fixed via `MarshalJSON` + `projectionMode` unexported field: full mode uses type alias (byte-identical to original struct), summary/hit use dedicated `summaryProjection`/`hitProjection` DTOs. Full mode output = 266,341 bytes matches pre-change size.

Tests: `TestS28SummaryModeShape` (JSON key assertion), `TestS29HitModeShape`, `TestS30FullModeBackwardCompatible`, `TestS31ModeIDParity`, `TestS32TokenBudgetPipeline`, `TestS33BudgetTooSmall`, `TestS34TokenEstimateDefinition`.

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

Go benchmark: `BenchmarkCheckMemory1000Miss` / `BenchmarkCheckMemory1000Hit` in `pkg/memorymeta/duplicate_benchmark_test.go`. Allocation regression guard: `TestCheckMemoryAllocationBudget` (ceiling 150K allocs/op).

### After optimization (current)

| benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 1000 miss | ~8,000,000 (~8.0ms) | 3,135,000 | 54,075 |
| 1000 hit | ~9,800,000 (~9.8ms) | 3,210,000 | 54,960 |

### Before optimization (baseline)

| benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 1000 miss | ~26,400,000 (~26.4ms) | 9,158,000 | 257,090 |
| 1000 hit | ~25,800,000 (~25.8ms) | 9,252,000 | 258,020 |

### Improvement

| metric | before | after | reduction |
|---|---:|---:|---:|
| latency (miss) | 26.4ms | 8.0ms | **69%** (3.3x faster) |
| B/op (miss) | 9.16MB | 3.14MB | **66%** |
| allocs/op (miss) | 257K | 54K | **79%** |

### Optimization method (pprof-guided)

pprof (`-alloc_objects`) showed 95% of allocations in `lexical.Tokenize`:
- `flushLatin` (`string(latin)` + `strings.ToLower`): 33%
- `splitIdentifier` (subword expansion via `strings.FieldsFunc`): 33%
- `strings.FieldsFunc`: 21%

Optimizations applied:
1. **Custom streaming BM25 tokenizer** (`tokenizeBM25Freq`): mirrors `lexical.Tokenize` CJK bigrams + latin lowercase but skips `splitIdentifier` subword expansion. Durable note/event/feedback content is natural language (not camelCase code), so subword tokens add ~50% allocation cost with negligible recall benefit. Streams directly into `map[string]int`, avoiding `[]string` intermediate. Uses `lexical.BM25.AddFromFreq` (new API).
2. **0-alloc truncation** (`firstNRunes`): finds byte offset after N runes via `for i := range s` (rune iteration is allocation-free), slices original string. Replaces `[]rune(body)` + `string(r)` (2 allocs per concept × 1000).
3. **Pre-computed candidate entries**: text/key/identity computed once per concept, reused for both BM25 build and top-10 Jaccard re-ranking. Eliminates double `truncateBody` and double `identity.FromConcept`.
4. **Inline ToLower in jaccardTokens**: `unicode.ToLower(r)` per-rune in the tokenization loop, avoiding `strings.ToLower(s)` intermediate string.
5. **Linear entry lookup** replaces `map[string]*okf.Concept`: top-10 hits against ≤1000 entries is cheaper than a 1000-entry map per call.

**Behavior unchanged**: 44-case golden (TP=21/FP=0/TN=23/FN=0), deterministic tie-break, threshold 0.20, only no_similar/possible_duplicate, read-only. BM25 candidate generation uses a simpler tokenizer but the final Jaccard classifier is unchanged; golden tests confirm no regression.

**Remaining cost**: ~3.1MB/op, ~54K allocs/op dominated by per-concept tf map (1000 maps) and individual token strings ( unavoidable for map keys without string interning, which would require shared mutable state — forbidden). No persistent cache, no second index, no global state.

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

- `python3 test_mcp.py` (legacy 2024-11-05): 13/13 PASS. The harness now builds the current source into a temporary binary and asserts exactly 20 unique legacy tools; this prevents stale `okf-bin` artifacts from producing false-green results.
- `python3 test_ext_skills.py` (modern 2026-07-28 + skills): 8/8 PASS. tools/list = 11 modern tools.
- Both eras: okf_manifest/okf_query/okf_context gain new optional params; no tool removed.
- `TestMCPQuerySharedAcrossEras`: modern and legacy tools/list both contain okf_query with `reflect.DeepEqual` schema.

## 8. Real Codex governed flow (S39)

Persistent harness: `tools/verify-governed-memory-codex.sh` (fail-closed, sanitized output, temp cleanup).

Fixture: isolated git repo with 2 concepts — `notes/redis.md` (code_refs: ["pkg/cache/*.go"]) and `notes/redis-dup.md` (near-duplicate for memory_check).

4 read-only tasks, **5 MCP tool calls, 0 mutating**:

| Task | Tool calls | Assertion | Result |
|---|---|---|---|
| A. manifest summary limit=3 | okf_manifest ×1 | items returned | PASS |
| B. for_path=pkg/cache/redis.go hit | okf_manifest ×1 | redis concept matched | PASS |
| C. context refs=<redis_id> | okf_manifest ×1 + okf_context ×1 | body contains CANARY | PASS |
| D. query memory_check=true | okf_query ×1 | status=possible_duplicate | PASS |

**Tools count clarification**: OKF MCP server exposes 20 tools (legacy era) or 11 tools (modern era). Codex CLI has 9 built-in function tools (exec, wait, etc.). Previous "21" was model's miscount from system prompt (20 OKF + 1 generic MCP mechanism). No 21st OKF tool exists.

All calls ok=true, mutating=false. No note/log/feedback/import/refresh/init invoked. Raw JSONL not committed; safe summary above.

## 9. New targeted tests (follow-up audit)

| Test | File | Scenario |
|---|---|---|
| `TestToolQueryMemoryCheckFlags` | cmd/okf/cmd_tool_governed_test.go | S39 CLI dedicated envelope, no results mix |
| `TestToolQueryMemoryCheckEmptyQ` | cmd/okf/cmd_tool_governed_test.go | S39 empty q → invalid_query |
| `TestToolQueryMemoryCheckTypeSingular` | cmd/okf/cmd_tool_governed_test.go | S22 --type singular |
| `TestToolQueryMemoryCheckDupThreshold` | cmd/okf/cmd_tool_governed_test.go | S27 --dup-threshold |
| `TestMCPQueryMemoryCheckSchema` | pkg/mcp/tools_governed_test.go | S39 schema has memory_check/dup_threshold, no types |
| `TestMCPQueryMemoryCheckHandler` | pkg/mcp/tools_governed_test.go | S39 MCP JSON true → dedicated result, read-only |
| `TestMCPQuerySharedAcrossEras` | pkg/mcp/tools_governed_test.go | S36 modern+legacy schema DeepEqual |
| `TestStaleRefsScanEntryLimit` | pkg/manifest/manifest_governed_test.go | S18 50k bound (injectable limit=3) |
| `TestS37CustomFieldsParseRoundTrip` | pkg/memorymeta/customfields_roundtrip_test.go | S37 parser→CustomFields→serialize preserves all fields |
| `TestS37ConceptHasNoGovernedStructFields` | pkg/memorymeta/customfields_roundtrip_test.go | S37 reflection: Concept has no Governance/CodeRefs fields |
| `TestDocumentationContracts` | pkg/agentconfig/documentation_contract_test.go | advisory/no-block/no-audit/no-allow-duplicate |
| `TestDocumentationClausesOnceInAgentSkill` | pkg/agentconfig/documentation_contract_test.go | W01-W07 exact-once |

## 10. Mutation testing (S38)

Persistent runner: `tools/mutants-governed-memory.sh` (8 mutants, all killed). Gauntlet L9c.

| Mutant | Change | Killed by |
|---|---|---|
| M-GM1 | unknown governance returns raw instead of context | TestGovernanceUnknownNonStrict |
| M-GM2 | hold rank = 2 (not first) | TestS06GovernanceSortActivation |
| M-GM3 | `*` treated as recursive `**` | TestMatchCodeRefsSingleGlob |
| M-GM4 | `**` span 8→100 | TestMatchCodeRefsRecursiveDepth |
| M-GM5 | threshold hardcoded to 0 | TestCheckMemoryThresholdConfigurable |
| M-GM6 | typeFilter doesn't narrow candidates | TestCheckMemoryTypeFilter |
| M-GM7 | summary mode keeps full projection | TestS28SummaryModeShape |
| M-GM8 | context refs loop emptied | TestServiceContextRefsReadsConceptBody |

All 8 killed, source restored byte-identical (cmp -s verified).

## 11. Full gauntlet

```
GAUNTLET PASS: build/vet/staticcheck/tests/tests(-race)/coverage(70%)/shuffle/
  new-package/property(20)/secret-scan/mod-verify/mutation(18/18)/
  agent-mutation(5/5)/governed-mutation(8/8)/real-exec
```

## 12. Code review

See `code-review.md` for the complete two-round review with 19 findings (1 high fixed via projection DTO, 6 medium fixed including current-source legacy E2E, 2 low fixed, and 10 informational verifications).

## 13. Hard constraints compliance

| Constraint | Status |
|---|---|
| Concept struct unchanged | ✓ (reflection test + CustomFields only) |
| No second index/fact source | ✓ (per-call BM25, existing manifest scan) |
| Default governance=context, no path inference | ✓ |
| Hold advisory only, no server block | ✓ (doc contract test) |
| CLI hyphen / MCP underscore | ✓ |
| memory_check read-only, no conflict/allow_duplicate/audit | ✓ (doc contract test) |
| for_path lexical, no FS, no EvalSymlinks | ✓ |
| stale-refs only FS scan, fail-closed | ✓ (incomplete+warnings) |
| W01-W07 each exactly once | ✓ |
| Modern 11 / legacy 20 tools | ✓ (E2E + shared schema test) |
| Full mode byte-compatible | ✓ (MarshalJSON DTO, 266,341 bytes) |
| No push/PR/merge | ✓ (local commits only) |
