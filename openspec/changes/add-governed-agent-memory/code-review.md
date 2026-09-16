# Code Review — add-governed-agent-memory

Implementation commit: df66533 (initial) + follow-up fixes.
Spec commit: 8712bb7.

## Round 1 — Explicit review (compile, interfaces, logic, errors, naming, data flow)

### Finding 1: Token projection omitempty breaks full-mode backward compatibility
- **Severity**: high
- **File**: `pkg/manifest/manifest.go` ManifestItem struct
- **Issue**: Initial implementation added `omitempty` to 9 existing fields (identity_state, path, status, trust_tier, stale, source_count, file_size_bytes, estimated_tokens, estimate_kind) to achieve summary/hit token reduction. This changed full-mode JSON for zero-valued fields (e.g., `"stale":false` disappeared, `"source_count":0` disappeared). S30 claimed "byte-compatible" but this was not true for concepts with zero-valued fields.
- **Fix**: Reverted all 9 fields to original non-omitempty tags. Implemented `MarshalJSON` on ManifestItem with `projectionMode` unexported field: full mode uses type alias (byte-identical to original), summary/hit modes use dedicated `summaryProjection`/`hitProjection` DTOs with only required fields.
- **Test**: `TestS28SummaryModeShape`, `TestS29HitModeShape` updated to assert JSON keys (not struct fields). `TestS30FullModeBackwardCompatible` confirms full mode retains all legacy fields.
- **Verification**: full mode output = 266,341 bytes (matches pre-change size); summary = 84,482 (68.3% reduction); hit = 130,859 (50.9% reduction).

### Finding 2: staticcheck S1011 in validate.go
- **Severity**: low
- **File**: `pkg/memorymeta/validate.go`
- **Issue**: Loop `for _, w := range codeWarns { errs = append(errs, w) }` could be `errs = append(errs, codeWarns...)`.
- **Fix**: Replaced with variadic append.
- **Test**: existing `TestValidateGovernanceStrict` still passes.

### Finding 3: gofmt on coderefs.go/coderefs_test.go
- **Severity**: low
- **File**: `pkg/memorymeta/coderefs.go`, `coderefs_test.go`
- **Issue**: gofmt flagged alignment.
- **Fix**: `gofmt -w`.

### Finding 4: S39 CLI memory_check missing dedicated test
- **Severity**: medium
- **File**: `cmd/okf/cmd_tool_governed_test.go`
- **Issue**: Only manifest/context CLI tests existed; no `okf tool query --memory-check` test.
- **Fix**: Added `TestToolQueryMemoryCheckFlags`, `TestToolQueryMemoryCheckEmptyQ`, `TestToolQueryMemoryCheckTypeSingular`, and `TestToolQueryMemoryCheckDupThreshold`; all pass in the final targeted run.

### Finding 5: MCP query governed schema/handler missing test
- **Severity**: medium
- **File**: `pkg/mcp/tools_governed_test.go`
- **Issue**: No okf_query memory_check schema/handler test.
- **Fix**: Added `TestMCPQueryMemoryCheckSchema`, `TestMCPQueryMemoryCheckHandler`, and `TestMCPQuerySharedAcrossEras`; the final targeted run passes.

### Finding 6: S18 50k entry bound no independent test
- **Severity**: medium
- **File**: `pkg/manifest/manifest.go` walkStaleScan
- **Issue**: maxStaleScanEntries was a const; no test verified the bound behavior.
- **Fix**: Made `maxStaleScanEntries` injectable and added `TestStaleRefsScanEntryLimit`, covering exact-limit completion and over-limit `incomplete+warning` behavior without constructing 50,000 files.

### Finding 7: S37 CustomFields round-trip only map-level, no parser round-trip
- **Severity**: medium
- **Issue**: SetCodeRefs writes to map, but no test verifies parser→Concept→CustomFields→serialize round-trip preserves governance/code_refs/my_custom.
- **Fix**: Added `TestS37CustomFieldsParseRoundTrip` and `TestS37ConceptHasNoGovernedStructFields`; the parser/serializer round-trip preserves governance, code_refs, and unrelated custom fields while the core Concept type remains unchanged.

### Finding 8: No governed-memory mutation runner
- **Severity**: medium
- **Issue**: Existing mutants.sh covers chunking/lexical; no mutants for governance/for_path/memory_check/projection/context refs.
- **Fix**: Added `tools/mutants-governed-memory.sh` with 8 targeted mutants and wired it into Gauntlet L9c; all 8 are killed and byte-for-byte restoration is verified.

### Finding 9: Codex evidence not persistent, for_path 0 results in real corpus
- **Severity**: medium
- **Issue**: Previous Codex run used the 329-concept corpus which has no code_refs, so for_path returned 0 matches — doesn't prove the feature works. No persistent harness script.
- **Fix**: Added persistent `tools/verify-governed-memory-codex.sh` with a dedicated code_refs and durable-duplicate fixture. The final run verifies summary, for_path hit, context refs body retrieval, and possible_duplicate with zero mutating calls.

## Round 2 — Implicit review (protocol, boundaries, security, performance, compatibility)

### Finding 10: Context refs unknown ref — verified not silent
- **Severity**: info (no fix needed)
- **File**: `pkg/tool/service.go:691-696`
- **Verification**: Unknown refs produce `warnings = append(warnings, "unresolvable ref: "+ref)` and `ContextOmission{Reason: "ref_unresolved"}`. Warnings are included in `ContextResult.Warnings`. `TestServiceContextUnknownRefOmitted` locks this. Not silent.

### Finding 11: Stale-refs fail-closed — verified
- **Severity**: info (no fix needed)
- **File**: `pkg/manifest/manifest.go:850-882`
- **Verification**: Unreadable directory entry → markIncomplete. Symlink escape → markIncomplete. Entry cap → markIncomplete. Walk error → markIncomplete. Never silently returns empty. `TestServiceManifestStaleRefsSymlinkEscape` locks symlink case.

### Finding 12: memory_check benchmark cost — documented
- **Severity**: info
- **File**: `pkg/memorymeta/duplicate.go`
- **Data**: 1000 concepts → ~22-24ms/op, ~9.1MB/op, ~257K allocs/op. Dominated by tokenizing 1000 bodies for BM25. Spec has no hard performance threshold. Acceptable for v1; future optimization could cache BM25 index or use incremental tokenization.
- **Decision**: Document as known cost; no blocking issue.

### Finding 13: Codex tools count = 21 investigation
- **Severity**: info
- **Verification**: Direct E2E catalog checks prove the OKF MCP server exposes 20 legacy tools and 11 modern tools. Codex 0.153.4 uses the legacy era and separately exposes 9 built-in function tools; MCP calls are routed through the `okf` namespace. The prior "21" value was a model self-report from prompt-visible names, not a server catalog count. The persistent Codex harness now reports these scopes separately.

### Finding 14: Governance hold advisory — verified no server block
- **Severity**: info
- **File**: `pkg/manifest/manifest.go` Build, `pkg/agentconfig/workflow.go` W02
- **Verification**: hold only sets `GovernanceWarning: true` in result; no error, no write blocking. Agent Skill W02 says "SHOULD request user confirmation" (not MUST). `TestS07HoldAdvisoryWarning` locks this.

### Finding 15: for_path lexical no FS — verified
- **Severity**: info
- **File**: `pkg/memorymeta/coderefs.go` MatchCodeRefs
- **Verification**: Pure string/segment matching, no os.Stat, no filepath.EvalSymlinks. `TestMatchCodeRefsNoFS` and `TestS15ForPathLexicalNoFS` lock this.

### Finding 16: memory_check read-only — verified
- **Severity**: info
- **File**: `pkg/tool/service.go` Query memory_check branch, `pkg/memorymeta/duplicate.go`
- **Verification**: CheckMemory only reads concepts, computes BM25+Jaccard, returns result. No file writes, no Concept modifications. `TestCheckMemoryReadOnly` locks this (compares file hashes before/after).

### Finding 17: Concept struct unchanged — verified
- **Severity**: info
- **File**: `pkg/okf/types.go`
- **Verification**: governance/code_refs are stored only in `Concept.CustomFields` (inline YAML map). `pkg/memorymeta` accessors read/write CustomFields only. `TestS37ConceptHasNoGovernedStructFields` reflects over the core type and `TestS37CustomFieldsParseRoundTrip` verifies parser/serializer preservation.

### Finding 18: Dual-era MCP compatibility — verified
- **Severity**: info
- **File**: `pkg/mcp/tools.go`
- **Verification**: okf_manifest/okf_query/okf_context use shared registration function; both modern (11 tools) and legacy (20 tools) eras get the updated schemas. `test_mcp.py` (legacy) and `test_ext_skills.py` (modern) both pass.

### Finding 19: Legacy MCP E2E used a stale prebuilt binary and did not assert the exact catalog
- **Severity**: medium
- **File**: `test_mcp.py`
- **Issue**: The harness launched repository-local `okf-bin`, which could lag the current source. It also checked only eight required names, allowing an 18-tool stale binary to pass despite the legacy contract requiring exactly 20 tools.
- **Fix**: The harness now builds the current `./cmd/okf` source into a temporary binary on every run (unless an explicit `OKF_BIN` override is provided), asserts exactly 20 unique legacy tool names, and removes the temporary binary afterward.
- **Verification**: Fresh E2E reports exactly 20 legacy tools; `TestModernToolsListHas11Tools` reports exactly 11 modern tools.

## Summary

| Round | Findings | High | Medium | Low | Info |
|---|---|---|---|---|---|
| Round 1 (explicit) | 10 | 1 | 6 | 2 | 1 |
| Round 2 (implicit) | 9 | 0 | 0 | 0 | 9 |

All high and medium findings were fixed and verified by the tests named above. No critical issues, unresolved security vulnerabilities, or known Spec violations remain.
