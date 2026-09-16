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
- **Fix**: Dispatched to tests agent (TestToolQueryMemoryCheckFlags, TestToolQueryMemoryCheckEmptyQ, TestToolQueryMemoryCheckTypeSingular, TestToolQueryMemoryCheckDupThreshold).

### Finding 5: MCP query governed schema/handler missing test
- **Severity**: medium
- **File**: `pkg/mcp/tools_governed_test.go`
- **Issue**: No okf_query memory_check schema/handler test.
- **Fix**: Dispatched to tests agent (TestMCPQueryMemoryCheckSchema, TestMCPQueryMemoryCheckHandler).

### Finding 6: S18 50k entry bound no independent test
- **Severity**: medium
- **File**: `pkg/manifest/manifest.go` walkStaleScan
- **Issue**: maxStaleScanEntries was a const; no test verified the bound behavior.
- **Fix**: Dispatched to tests agent (convert to var, TestStaleRefsScanEntryLimit with small injected limit).

### Finding 7: S37 CustomFields round-trip only map-level, no parser round-trip
- **Severity**: medium
- **Issue**: SetCodeRefs writes to map, but no test verifies parser→Concept→CustomFields→serialize round-trip preserves governance/code_refs/my_custom.
- **Fix**: Dispatched to tests agent.

### Finding 8: No governed-memory mutation runner
- **Severity**: medium
- **Issue**: Existing mutants.sh covers chunking/lexical; no mutants for governance/for_path/memory_check/projection/context refs.
- **Fix**: Dispatched to mutants agent (tools/mutants-governed-memory.sh, 6-8 mutants, gauntlet L9c).

### Finding 9: Codex evidence not persistent, for_path 0 results in real corpus
- **Severity**: medium
- **Issue**: Previous Codex run used the 329-concept corpus which has no code_refs, so for_path returned 0 matches — doesn't prove the feature works. No persistent harness script.
- **Fix**: Dispatched to Codex agent (tools/verify-governed-memory-codex.sh with dedicated fixture containing code_refs and durable duplicate).

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
- **Verification**: OKF MCP server modern era has 11 tools. Codex CLI may wrap MCP tools in a namespace (e.g., `mcp__okf__toolname`) and also expose built-in tools. The "21" count likely includes Codex built-ins + OKF tools. Dispatched to Codex agent to distinguish OKF MCP tools vs Codex namespace wrapper in output.

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
- **Verification**: governance/code_refs stored in `Concept.CustomFields` (inline YAML map). `pkg/memorymeta` accessors read/write CustomFields only. No fields added to Concept struct. Tests agent will add compile-time/reflection assertion.

### Finding 18: Dual-era MCP compatibility — verified
- **Severity**: info
- **File**: `pkg/mcp/tools.go`
- **Verification**: okf_manifest/okf_query/okf_context use shared registration function; both modern (11 tools) and legacy (20 tools) eras get the updated schemas. `test_mcp.py` (legacy) and `test_ext_skills.py` (modern) both pass.

## Summary

| Round | Findings | High | Medium | Low | Info |
|---|---|---|---|---|---|
| Round 1 (explicit) | 9 | 1 | 5 | 2 | 1 |
| Round 2 (implicit) | 9 | 0 | 0 | 0 | 9 |

All high/medium findings fixed or dispatched. No critical issues. No security vulnerabilities. No spec violations.
