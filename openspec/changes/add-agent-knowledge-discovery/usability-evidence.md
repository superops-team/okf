# Usability Evidence — add-agent-knowledge-discovery (Round 3: zero-skip audit)

Date: 2026-09-12
Branch: spec/agent-knowledge-discovery
Version under test: okf CLI 0.7.0
Verification script: `tools/verify-agent-usability.sh` (156 assertions, fail-closed, zero-skip required)
MCP helper: `tools/mcp_call.py` (Content-Length stdio protocol, structured JSON parsing)

## Summary

**156 passed, 0 failed, 0 skipped** (usability protocol suite, A1–G4).
Real Agent client E2E (Journey H, four-layer): **8 PASS / 0 FAIL / 2 BLOCKED_AUTH**
via `tools/verify-real-agent-e2e.sh` — the eight PASS entries are one generated-config parser gate, three official-client config-discovery gates, one validator negative-control gate, and three Codex model closures. Claude Code and Cursor pass configuration discovery but are BLOCKED_AUTH at model layer (fail-closed per S56, never aggregated into PASS).
13 UX/product defects found and fixed via TDD. S46 spec-amendment **Approved** by user explicit approval 2026-09-13. Strict three-client release acceptance remains blocked by Claude/Cursor authentication. Their machine event-stream validators are implemented and pass synthetic negative controls, but cannot be promoted to real-client PASS until exercised against authenticated official-client streams.

## Defects found and fixed (Round 3 additions marked ★★)

| # | Defect | Journey | Fix | Test |
|---|--------|---------|-----|------|
| 1 | `okf identity/agent/tool --help` returned "unknown subcommand" | A1 | Added `--help`/`-h` handling; no-args prints help exit 0 | `TestHelpFlagExplicitHelp` |
| 2 | `okf tool manifest` text output was just "manifest ok" | B1 | `printManifestText()` renders path/type/id/tags/tokens/title | `TestToolManifestTextOutput` |
| 3 | `search -group-by bogus` exited 0 (script-hostile) | C4 | `printGroupedSearch` returns exit code; error to stderr + valid values | `TestCLISearchInvalidGroupBy` |
| 4 | malformed ref error had no remediation | A7 | `errToTool()` adds `CodeInvalidConceptID` remediation | `TestCmdIdentityResolveMalformedRefRemediation` |
| 5 | grouped search showed internal `key=v3:id:okf_...` | C2 | Text output shows representative path/folder; raw key in JSON | `TestCLISearchGroupedOutput` |
| 6 | `emitToolEnvelope` didn't print remediation line | F1 | Added `→ remediation` to stderr error output | (existing tests) |
| 7 | grouped eval defaulted to lexical-only | C7 | `cmd_eval.go` builds hybrid strategy for `-group-by` | (prior round) |
| 8 | `RelevantSourceRecallAtK` only checked representative (folder 0.4038) | C7 | `GroupedHit.CoveredSources` carries all member sources | `TestRelevantSourceRecallAtK_MultiSourceFolder` |
| 9 | `eval -group-by bogus` silently accepted, output all-zero, exit 0 | C4/F1 | `cmd_eval.go` validates groupBy before running; error + exit 1 | (verify script) |
| 10 | Agent apply dropped custom keys inside OKF-managed mcpServers.okf | D4 | `inspectJSONMCP` merges unknown keys + custom env vars | `TestInspectJSONMCPPreservesUnknownKeys` |
| 11 ★★ | **Agent Apply overwrote unowned whole-file artifacts** (Cursor rule/Claude skill without OKF-MANAGED header was truncated to empty) | D5 | `commit()` preflights all ops; any `StatusConflict` returns `agent_config_conflict` + path/remediation before any write | `TestApplyFailsOnUnownedRulesFile` |
| 12 ★★ | **Derived chunks (doc__cN.md) never received parent_okf_id** — `AssignParentID` was defined but never called; `cmd_add.go` passed empty string | E3 | `identity ensure` step 9 `propagateParentIDs()`: after ID assignment, scans for `__cN.md` chunks, reads parent's okf_id, sets `parent_okf_id`; idempotent | `TestIdentityEnsureDerivedChunksGetParentOKFID` |
| 13 ★★★ | **Generated Codex TOML was invalid** — `args = ["mcp" "--repo" "."]` omitted commas, so standard TOML parsers and official Codex could not load the MCP server | H1/H2 | `RenderCodexMCPTOML` now emits comma-delimited string arrays; acceptance parses the file with `tomllib` and checks the exact command/args before invoking official Codex | `TestRenderCodexMCPTOMLStringArraySyntax`, `tools/verify-real-agent-e2e.sh` |

## Per-scenario execution status (A1–G4, all executed)

### Journey A: New user (8/8 PASS)
| ID | Scenario | Status | Evidence |
|----|----------|--------|----------|
| A1 | Command discoverability | ✅ | `identity/agent/tool --help` all show subcommands; exit 0 |
| A2 | Empty knowledge base | ✅ | identity ensure exit 0; manifest total=0 |
| A3 | No-index degradation (lexical vs semantic) | ✅ | **Default lexical: no misleading warning, returns results. Explicit -semantic: clear warning "尚未构建向量索引，请先执行 okf vector index（回退到词法检索）"** |
| A4 | Old v2 vector index remediation | ✅ | **Real v2 fixture** (build v3, downgrade meta to v2): status shows "v2" + "当前版本需 v3，请执行 okf vector rebuild"; semantic degrades; rebuild restores v3 |
| A5 | dry-run→apply→resolve | ✅ | dry-run no file mod; apply writes IDs; second apply missing=0; resolve valid ID returns path |
| A6 | Rename/move after identity | ✅ | resolve follows to alpha-renamed.md |
| A7 | Malformed ref / not found | ✅ | `invalid_concept_id` + remediation with format example; `concept_ref_not_found`; both exit 1 |
| A8 | Duplicate ID detection | ✅ | detected with both paths; exit 1; no files modified |

### Journey B: Manifest (9/9 PASS)
| ID | Scenario | Status | Evidence |
|----|----------|--------|----------|
| B1 | Default output | ✅ | text lists path/type/id/tags/tokens/title; no body leak |
| B2 | JSON output | ✅ | valid `okf.tool.v1` envelope; total=3; stdout pure JSON |
| B3 | Pagination | ✅ | limit=2 returns 2; limit=999/0 rejected `invalid_request` exit 1 |
| B4 | Filter combinations AND/OR | ✅ | type=concept→2; tags=go→2; type+tags→0 (empty not error); folder-prefix→1; status=stable→2; tags=go,python→3 |
| B5 | Corrupt frontmatter | ✅ | one bad file doesn't crash; other files listed; exit 0 |
| B6 | Oversized frontmatter | ✅ | 100KB body file manifest works; `estimate_kind=file_bytes_div4`; no body leak |
| B7 | Duplicate IDs in manifest | ✅ | **`ok=false`, code=`duplicate_concept_id`, message contains both a.md and b.md, remediation present** |
| B8 | No implicit index/model | ✅ | manifest works without `.okf/vector`; no index created |
| B9 | CLI/Service/MCP consistency | ✅ | CLI total=1, MCP okf_manifest total=1 (same repo) |

### Journey C: Grouped retrieval (7/7 PASS)
| ID | Scenario | Status | Evidence |
|----|----------|--------|----------|
| C1 | group_by omitted = legacy | ✅ | no "Projected into" banner; results returned |
| C2 | chunk/concept/source/folder semantics | ✅ | all 4 produce projection; no internal v3:id key; hit counts + member evidence |
| C3 | Fewer than K results | ✅ | no-result query doesn't fabricate; 1-result query gives 1 group no padding |
| C4 | Invalid group_by value | ✅ | search exit 1 + valid values; eval exit 1; **MCP returns ok=false with error** |
| C5 | No semantic index degradation | ✅ | grouped search still projects on lexical results |
| C6 | CLI/Service/MCP consistency | ✅ | CLI grouped produces groups; **MCP okf_query parsed JSON, ok=true, contains groups** |
| C7 | Hybrid utility gate | ✅ | concept/source/folder all srcRecall=0.9231 ≥ raw=0.9231 |

### Journey D: Agent Integration (11/11 PASS)
| ID | Scenario | Status | Evidence |
|----|----------|--------|----------|
| D1 | Lifecycle per client | ✅ | plan→apply→status→second apply(0 diff)→remove for cursor/claude-code/codex |
| D2 | --client all | ✅ | all three configured; each status reports installed |
| D3 | Unowned config conflict | ✅ | `agent_config_conflict`; file unchanged; exit 1 |
| D4 | Unknown keys preservation | ✅ | MY_CUSTOM_VAR + customField + other-server all preserved |
| D5 | Unowned whole-file artifact | ✅ | **User's rule file without OKF-MANAGED header: apply fails with `agent_config_conflict` + path/remediation; file preserved byte-for-byte (md5 unchanged)** |
| D6 | Write failure error quality | ✅ | **file-as-directory injection: ok=false, error has code+message+remediation; exit 1** |
| D7 | Symlink/path escape | ✅ | **Real symlink to existing outside file: rejected; outside file not modified** |
| D8 | Partial write rollback | ✅ | **Second-file failure (rules as file): first file mcp.json restored byte-identical (md5 match); no partial OKF content** |
| D9 | Generated config starts MCP | ✅ | `okf mcp --repo .` responds to initialize |
| D10 | Canonical workflow guidance | ✅ | rules file mentions okf/manifest/query/context; OKF_MANAGED marker |
| D11 | Interactive/non-interactive + secrets | ✅ | no --yes fails with guidance; no secret literal assignment patterns |

### Journey E: Compatibility & migration (5/5 PASS)
| ID | Scenario | Status | Evidence |
|----|----------|--------|----------|
| E1 | Legacy bundle (no IDs) | ✅ | manifest/search work; item has empty okf_id |
| E2 | Writer preserves ID | ✅ | existing okf_id unchanged after identity ensure |
| E3 | Derived chunks parent_okf_id | ✅ | **Parent gets okf_id; doc__c1.md and doc__c2.md both get parent_okf_id matching parent; second ensure idempotent** |
| E4 | Vector v2 remediation | ✅ | covered by A4 |
| E5 | concept_id vs okf_id not confused | ✅ | help mentions okf_id not concept_id; frontmatter uses okf_id |

### Journey F: Recoverability (5/5 PASS)
| ID | Scenario | Status | Evidence |
|----|----------|--------|----------|
| F1 | Error rubric (code+message+remediation) | ✅ | invalid_concept_id, invalid_request, invalid_group_by all have code+message; key errors have remediation |
| F2 | Exit codes script-friendly | ✅ | all failures exit 1; successes exit 0 |
| F3 | JSON clean streams | ✅ | manifest/identity JSON stdout valid; no stderr mix |
| F4 | Help examples copy-pasteable | ✅ | identity/agent/tool help all include examples |
| F5 | Output verbosity | ✅ | dry-run output concise with key info |

### Journey G: MCP protocol verification (direct stdio, 4/4 PASS — NOT real Agent client)

> **Layer clarification**: This journey uses `tools/mcp_call.py` to start
> `okf mcp --repo .` directly and send Content-Length framed JSON-RPC. This is
> the **MCP server protocol layer** only. It does NOT validate official Agent
> client transport discovery or LLM tool-use. See Journey H for real Agent E2E.

| ID | Scenario | Status | Evidence |
|----|----------|--------|----------|
| G1 | Three-client config round-trip + MCP startup | ✅ | cursor(.cursor/mcp.json), claude(.mcp.json), codex(.codex/config.toml) all created and parseable; MCP okf_status responds ok=true |
| G2 | Actual tool calls through MCP (protocol) | ✅ | **okf_status, okf_manifest, okf_query(group_by), okf_context all return ok=true with parsed JSON content** |
| G3 | Error handling through MCP (protocol) | ✅ | **okf_resolve with bad ref returns ok=false with structured error code** |
| G4 | Controlled note/feedback (protocol) | ✅ | **okf_note (with idempotency_key) ok=true; okf_feedback (with principle/category/idempotency_key) ok=true; files created** |

### Journey H: Real Agent client E2E (four-layer, 8 PASS / 0 FAIL / 2 BLOCKED_AUTH)

> Script: `tools/verify-real-agent-e2e.sh`. Four layers reported separately per
> S51–S56: (1) adapter fixture, (2) official config discovery, (3) real model
> MCP calls, (4) final answer/effect. BLOCKED_AUTH never aggregated into PASS.

| ID | Scenario | Client | Layer | Status | Evidence |
|----|----------|--------|-------|--------|----------|
| H1 | Adapter fixture parse | all three | adapter | ✅ PASS | cursor/claude/codex configs parse with standard parsers; command=`okf mcp --repo .` |
| H2 | Official config discovery | Codex 0.153.4 | config discovery | ✅ PASS | `codex mcp list --json` discovers server `okf` |
| H2 | Official config discovery | Claude Code | config discovery | ✅ PASS | official CLI config discovery confirms okf |
| H2 | Official config discovery | Cursor | config discovery | ✅ PASS | official CLI config discovery confirms okf |
| H3 | Real model read-answer | Codex | model+answer | ✅ PASS | JSONL: okf_status→okf_manifest→okf_query→okf_context in order; canary in final answer; zero shell/file/CLI bypass |
| H4 | Real model error recovery | Codex | model+answer | ✅ PASS | invalid ref → `invalid_concept_id` + remediation → manifest → retry resolve → resolved path in answer |
| H5 | Real model controlled write | Codex | model+effect | ✅ PASS | okf_note with idempotency key → okf_query readback → persisted file verified → content in answer |
| H1 | Validator negative controls | validator | anti-fail-open | ✅ PASS | fabricated correct answer without tools, fabricated recovery without real error, and fabricated write without persisted effect are all rejected |
| H3–H5 | Real model E2E | Claude Code | model+answer | ⛔ BLOCKED_AUTH | official config discovery PASS; no model credential in this environment |
| H3–H5 | Real model E2E | Cursor | model+answer | ⛔ BLOCKED_AUTH | official config discovery PASS; no model credential in this environment |

**Summary**: 8 PASS (1 generated-config parser gate + 3 official config discovery + 1 validator negative-control gate + 3 Codex model closures), 0 FAIL, 2 BLOCKED_AUTH (Claude Code/Cursor model E2E). Codex fully passes all implemented model closures. Claude/Cursor pass config discovery but are blocked at model layer by missing authentication; their implemented generic stream validators remain unverified against real authenticated streams and therefore cannot claim PASS.

## MCP response parsing

All MCP checks use structured JSON parsing via `mcp_inner()` helper:
- Outer format: `{"content":[{"type":"text","text":"<inner JSON>"}]}`
- Inner format: `okf.tool.v1` envelope with `ok`, `error.code`, `error.remediation`
- No grep-based `ok/error` matching

## Negative control

Hybrid folder recall=0.9231 vs lexical ~0.08. If hybrid channel were broken, C7 gate would fail decisively.

## Verification commands

```bash
# Comprehensive usability suite (156 assertions, zero-skip enforced)
bash tools/verify-agent-usability.sh

# Full regression suite
bash tools/gauntlet.sh

# MCP E2E (Content-Length protocol)
python3 test_mcp.py

# MCP tool caller helper
python3 tools/mcp_call.py <binary> <repo> <tool_name> [json_args]
```

## Remaining limitations

1. **S46 historical baseline note**: The historical 0.9615/0.7256 figures in releases.md were measured at an earlier point with different knowledge-base content and are not reproducible from the current tree. The S46 spec-amendment (user-approved 2026-09-13) clarifies the reproducible baseline is Recall@5≈0.92, MRR≈0.66, enforced by `TestHybridBaselineGate_ChunkLevel`. Base-vs-head zero-regression proof retained.
2. **A3 silent lexical fallback**: Without `-semantic`, search uses lexical by default and doesn't warn about missing vector index. This is intentional (lexical is the primary channel). Users wanting semantic must use `-semantic` or hybrid eval.
