# Evidence: Reflective Retrieval and Memory Defense

Fresh run date: 2026-09-28. All numbers from the last clean test run on branch `feat/add-reflective-retrieval-and-memory-defense`.

## Build & Static Analysis
- `go build ./...`: PASS (exit 0)
- `go vet ./pkg/... ./cmd/...`: PASS (zero warnings)
- `gofmt -l pkg/ cmd/`: clean (no files listed)

## Test Results
| Package | Tests | Race | Shuffle | Coverage |
|---|---|---|---|---|
| pkg/memorydefense | 13 pass | PASS | PASS | 90.8% |
| pkg/relationrecall | 5 pass | PASS | PASS | 91.9% |
| pkg/reflect | 6 pass | PASS | PASS | 96.5% |
| pkg/trapeval | 6 pass | PASS | PASS | 84.4% |
| pkg/tool (write_defense) | 6 pass | PASS | PASS | (inherited) |

## Fuzz
- `FuzzScreenNeverCrashes`: 37,698 execs in 6s, 0 crashes, 0 panics

## Spike 17 items mapped
| Spike test | Where | Status |
|---|---|---|
| 8 secret positive hits | catalog_test.go TestDetectorPositiveHits | PASS |
| Redact in place | catalog_test.go TestScreenRedactReplacesInPlace | PASS |
| Block on high severity | catalog_test.go TestScreenBlockReturnsError | PASS |
| No false positive normal text | catalog_test.go TestNoFalsePositiveOnNormalText | PASS |
| Pattern floor ≥16 | catalog_test.go TestCatalogSizeFloor | PASS (16) |
| Trap eval answer hit | trap_test.go TestAnswerLevelGraded | PASS |
| Poison leak detection | trap_test.go TestForbiddenEvidenceZero | PASS |
| Abstention rewarded | trap_test.go TestAbstentionRewarded | PASS |
| Trap bundle poison gate | reflect_test.go TestDropsTrapCandidates | PASS |
| Extends neighbors | recall_test.go TestBidirectionalExtends | PASS |
| Update chain head | recall_test.go TestUpdateChainHead | PASS |
| Hides proposed | recall_test.go TestProposedHidden | PASS |
| Multi-round > single | reflect_test.go TestMultiRoundGathersMore | PASS |
| Drops trap evidence | reflect_test.go TestDropsTrapCandidates | PASS |
| Abstain when thin | reflect_test.go TestAbstainWhenThin | PASS |
| RRF overlap first | reflect_test.go TestRRFOverlapRanksFirst | PASS |
| Example screen redaction | fuzz_test.go TestRedactionDeterministic | PASS |

## Spike bugs fixed
1. **Credit card false positive**: Luhn check added. `1234567890123456` (byte count) does NOT match; `4111-1111-1111-1111` does.
2. **Extends bidirectional recall**: Both outgoing (anchor extends others) and incoming (others extend anchor) directions are covered.

## New risks covered
- Redaction × payload_hash/idempotency: TestWriteKnowledge_RedactionIdempotencyRetry PASS
- Chinese PII: TestWriteKnowledge_NoFalsePositiveChinese PASS (no false positive)
- Config missing/invalid: TestLoadPolicyMissingFile / TestLoadPolicyInvalidAction PASS
- Block zero bytes: TestWriteKnowledge_DefenseBlockZeroBytes PASS
- Error no leak: TestScreenBlockReturnsError checks error message excludes original token

## Round 2 verification (2026-09-28)

### Test counts after additions
| Package | Tests | Coverage |
|---|---|---|
| pkg/memorydefense | 14 pass (13+1 determinism) | 90.8% |
| pkg/relationrecall | 9 pass (5+4 edge: cycle/fork/dangling/declined) | 91.9% |
| pkg/reflect | 7 pass (6+1 boundary exact-threshold) | 96.5% |
| pkg/trapeval | 7 pass (6+1 poison ratio) | 84.4% |

### Performance benchmark (10k concepts)
| Benchmark | Latency | Allocs/op | B/op |
|---|---|---|---|
| BenchmarkRecall10k | 378 µs/op | 5 | 384 |
| BenchmarkScreen10k (redact, 10KB) | 1.13 ms/op | 715 | 154,900 |
| BenchmarkScreen10kBlock (10KB) | 207 µs/op | 715 | 125,761 |

Acceptability: 378µs for O(n) relation scan over 10k concepts is well under 10ms SLA for CLI/MCP. Screen redact at 1.1ms on 10KB content is dominated by string building; acceptable for write path.

### Mutation testing
| Mutation | Target | Killed? |
|---|---|---|
| M1: reverse block severity (`==High` → `!=High`) | screen.go | YES — TestWriteKnowledge_DefenseBlockZeroBytes |
| M2: reverse approved filter (`!=Approved` → `==Approved`) | recall.go | YES — TestDeclinedHidden |
| M3: change abstain threshold (`<` → `<=`) | reflect.go | Initially escaped; added TestNoAbstainAtExactThreshold → now KILLED |

### Secret / supply-chain scan
- grep-based secret scan on all new .go files: only matches are fake test tokens (ABCDEF...) in _test.go files. No real secrets.
- `go mod verify`: all modules verified.
- 54 dependencies total, no new dependencies added.

### CLI smoke test
- `okf tool reflect -q "deployment database"` → returns ok:true, envelope correct
- `okf tool relation --anchor okf_aaa...` → returns self + extends neighbor
- `okf eval trap -golden cases.json` → prints 4 means, exits 0
- Binary built from clean tree, ran on real git repo fixture.

### Legacy MCP
- Unified ToolRegistry: registerCoreTools + registerAgentTools both served by same MCP server.
- New tools okf_reflect/okf_relation_recall registered in registerAgentTools.
- No separate legacy MCP server exists; dual_era_test.go confirms unified registry.

### Code review
- See code-review.md for Round 1 (3 fixes) and Round 2 (9 observations, 0 fixes needed).

## Round 3 final verification (fresh run 2026-09-28)

### Static checks
- gofmt: clean
- go vet: clean
- staticcheck: clean on all new packages
- go mod verify: all modules verified

### Fresh test run (race + shuffle)
| Package | Result | Coverage |
|---|---|---|
| pkg/memorydefense | ok (race+shuffle) | 90.8% |
| pkg/relationrecall | ok (race+shuffle) | 91.9% |
| pkg/reflect | ok (race+shuffle) | 96.4% |
| pkg/trapeval | ok (race+shuffle) | 86.6% |
| pkg/tool | ok (race+shuffle) | inherited |
| pkg/mcp | ok (race+shuffle) | inherited |

### Fuzz (fresh)
- FuzzScreenNeverCrashes: 54,925 execs in 11s, 0 crashes

### Codex E2E (real multi-turn)
- Model: gpt-5.6-sol__dev via Xeart Router
- MCP server: okf binary stdio, repo=/tmp/okf-e2e-repo
- Round 1: okf_reflect("postgresql") → found okf_1111 (source_round=1)
- Round 2: relation expansion → found okf_2222 (source_round=2)
- RRF fused both, need_clarify=false (2 evidence >= min=2)
- okf_relation_recall("okf_1111") → self + extends neighbor okf_2222
- Stable refs verified: same query → same evidence ids

### MCP stdio E2E (test_mcp.py)
- 17 tests all pass (was 13, added 4 for reflect/relation)
- Tests cover: success call, empty question error, unknown anchor error, empty anchor error
- Content-Length framing verified on real subprocess

### Document import defense
- handleImportDocument now calls memorydefense.Screen on converted markdown before writing
- Default disabled (backward compatible); enabled via .okf/config.yaml
- Block action rejects import; redact replaces secrets in-place

---

## Independent Review A/B final fresh verification (2026-09-29, HEAD 8b796f8)

> **Note**: All numbers below are from the 2026-09-29 fresh run at HEAD `8b796f8`. Earlier sections are historical; the latest acceptance gates are in this section.

### Review A findings (obvious issues, RED→GREEN)

| ID | Severity | File | Issue | RED evidence | Fix |
|---|---|---|---|---|---|
| RA-1 | High | cmd/okf/cmd_add.go screenImportTree | Direct .md import redacted user's ORIGINAL files in-place (data corruption). | TestStageForDefensePreservesSource failed: original file content changed after redact. | Added `stageForDefense()`: when defense enabled and no staging dir, copy .md files to temp dir, screen temp, import from temp. Original never touched. |
| RA-2 | Medium | pkg/mcp/tools.go handleImportDocument | `LoadPolicy(filepath.Dir(bundlePath))` resolved to `<repo>/.okf/.okf/config.yaml` — config never found, MCP Defense silently disabled. | TestImportDocumentDefenseBlock/Redact failed: no config loaded, secrets passed through. | Compute repoRoot from bundlePath (if base=="knowledge" → parent). |

### Review B findings (boundary/security)

New edge tests added (all GREEN on original implementation):
- TestScreenEmptyInput: empty string no panic/no hits
- TestScreenMultipleSecretsRedact: 2 secrets both redacted
- TestScreenLargeInputNoCrash: 1MB normal text, no false positive
- TestScreenInvalidConfigAction: unknown action passes through
- TestScreenMultilinePEM: block triggers on PEM marker
- TestExtendsSelfLoopSkipped: self-extends produces exactly 1 hit
- **RB-1**: TestReflect_TrapGateDropsProposed — Service-layer trapGate directly tested (approved present, proposed poison absent). This was the M4 mutation gap.

### Full-suite fresh run

| Command | Result |
|---|---|
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `staticcheck ./...` | exit 0, no findings |
| `go test ./...` | **27 packages all ok** (no failures) |
| `go test -race ./...` | **27 packages all ok** |
| `go test -shuffle=on ./...` | **27 packages all ok** |

### Coverage
- New packages: memorydefense 90.8%, relationrecall 94.6%, reflect 96.4%, trapeval 86.6%
- Whole-repo (gauntlet): **72%** (threshold 60%)

### Fuzz
- `go test ./pkg/memorydefense/ -fuzz=FuzzScreenNeverCrashes -fuzztime=15s`
- **77,594 execs**, 99 interesting seeds, 0 crashes

### Mutation
- **Hand-targeted mutations (this change set): 5/5 killed**
  - M1 (RA-1 remove staging copy): killed by CLI smoke integration
  - M2 (RA-2 wrong repoRoot): killed by TestImportDocumentDefense*
  - M3 (block severity high→medium): killed by TestScreenBlockReturnsError
  - M4 (trapGate filter inverted): killed by TestReflect_TrapGateDropsProposed (RB-1)
  - M5 (abstain `<`→`<=`): killed by TestAbstainWhenThin/TestNoAbstainAtExactThreshold
- **Gauntlet built-in mutations: 38/38 killed** (18 convert + 5 agent-discovery + 8 governed-memory + 7 temporal-memory)
- These are different scopes: 5 hand-targeted = the new code; 38 gauntlet = whole-repo existing mutants.

### Supply chain & secret scan
- `go mod verify`: all modules verified
- Secret scan (gauntlet L7): clean — only test-fixture fake tokens in test files

### CLI smoke (real binary)
- Normal add (disabled): exit 0, 1 imported, byte-for-byte unchanged
- Redact add: exit 0, **source file unchanged** (stageForDefense works), kb file contains `[REDACTED:github_pat]`
- Block add: non-zero exit, staging cleaned up
- Durable writes verified: redacted content in kb, original source untouched

### MCP stdio E2E
- `python3 test_mcp.py`: **17/17 tests pass**
- Covers: okf_reflect success/empty, okf_relation_recall success/empty/unknown anchor, import document disabled/block/redact/clean

### Codex E2E (real, 2026-09-29)
- Model: `gpt-5.6-sol__dev` via Xeart Router (127.0.0.1:18080 proxy)
- Codex version: 0.153.4
- 4 shell tool calls:
  1. Baseline "Say hello" → connectivity confirmed
  2. `okf tool reflect -q "PostgreSQL deployment"` → evidence=[approved], need_clarify=false; correct poison-free results
  3. `okf add` with secret (disabled policy) → exit 0, source unchanged, secret passed through (backward compat)
  4. `okf add` with secret (block enabled) → skipped (already imported), source unchanged
- Durable writes: 1 under disabled policy
- No secret leaked in any Codex output

### Benchmark (10k concepts)
- BenchmarkScreen10k: 1.19 ms/op, 155 KB/op, 716 allocs
- BenchmarkScreen10kBlock: 0.20 ms/op (early exit)
- BenchmarkRecall10k: 0.38 ms/op, 384 B/op, 5 allocs

### Gauntlet
- Command: `tools/gauntlet.sh`
- Result: **GAUNTLET PASS**
- Coverage: 72% (threshold 60%)
- All layers: build/vet/gofmt/staticcheck/tests/race/coverage/shuffle/property/secret-scan/mod-verify/mutation/real-exec

### Final state
- Branch: `feat/add-reflective-retrieval-and-memory-defense`
- HEAD: `8b796f8`
- Commits since base `879bae3`: **16**
- Working tree: clean
