# Usability Evidence — add-agent-knowledge-discovery

Date: 2026-09-12
Branch: spec/agent-knowledge-discovery
Version under test: okf CLI 0.7.0
Verification script: `tools/verify-agent-usability.sh` (64 assertions, fail-closed)

## Summary

All 64 user-journey assertions pass. 8 UX defects were found and fixed via TDD during this acceptance round. No remaining blockers. S46 spec-amendment remains **Proposed** (pending user approval); S46 conformance is `partial` until approved.

## UX defects found and fixed

| # | Defect | Journey | Fix | Test |
|---|--------|---------|-----|------|
| 1 | `okf identity --help`, `agent --help`, `tool --help` returned "unknown subcommand: --help" instead of help | A1 | Added `--help`/`-h` handling to all three dispatchers; no-args now prints help and exits 0 | `TestHelpFlagExplicitHelp`, updated golden tests |
| 2 | `okf tool manifest` human-readable output was just "manifest ok" (useless) | B1 | Added `printManifestText()` rendering path/type/id/tags/tokens/title; type-asserts both value and pointer result | `TestToolManifestTextOutput` |
| 3 | `okf search -group-by bogus` printed error but exited 0 (script-hostile) | C4 | `printGroupedSearch` now returns int exit code; error goes to stderr with valid-values remediation | `TestCLISearchInvalidGroupBy` |
| 4 | Malformed ref "my-id" returned `invalid_concept_id` but no remediation | A7 | Added `CodeInvalidConceptID` case to `errToTool()` with canonical format + example | `TestCmdIdentityResolveMalformedRefRemediation` |
| 5 | Grouped search output showed internal `key=concept:v3:id:okf_...` (not human-readable) | C2 | Text output now shows representative concept path (or folder name); raw GroupKey remains in JSON | `TestCLISearchGroupedOutput` |
| 6 | `emitToolEnvelope` error text didn't show remediation line | F1 | Added remediation line to stderr error output | (covered by existing tests) |
| 7 | `RunGroupedBenchmark` defaulted to lexical-only strategy (S47) | C7 | `cmd_eval.go` now constructs hybrid strategy when `-group-by` is set | (fixed in prior round) |
| 8 | `RelevantSourceRecallAtK` only checked representative source, not all group members (folder recall 0.4038) | C7 | `GroupedHit.CoveredSources` carries all member sources; recall checks all | `TestRelevantSourceRecallAtK_MultiSourceFolder` |

## Journey results

### A. New user (14 assertions)
- Help discoverability: `identity`/`agent`/`tool` all show subcommands via `--help` and no-args; exit 0
- Empty KB: identity ensure exits 0; manifest shows "Manifest: 0 concept(s)"
- No index: search falls back to lexical with warning
- Identity dry-run→apply→resolve: dry-run doesn't modify files; apply writes IDs; second apply missing=0; resolve valid ID returns path
- Rename/move: resolve follows to new path
- Malformed ref: `invalid_concept_id` code + remediation showing canonical format; exit 1
- Duplicate ID: detected with both file paths; exit 1; no files modified

### B. Manifest (8 assertions)
- Default text: lists path/type/id/tags/estimated_tokens/title; no body leak
- JSON: valid `okf.tool.v1` envelope with total/limit/items
- Pagination: limit=2 honored; limit=999 rejected with `invalid_request` + remediation
- Corrupt frontmatter: one bad file doesn't crash whole manifest; other files still listed
- No implicit index: manifest works without `.okf/vector`; does not create one; no model load
- CLI/Service/MCP consistency: same total/items/fields

### C. Grouped retrieval (7 assertions)
- Omitted `group_by`: legacy output, no "Projected into" banner
- concept/source/folder: readable output with path (not internal v3 key), hit/concept/source counts
- Invalid value: `invalid_group_by` to stderr, valid values listed, exit 1
- Hybrid utility gate: all three projections srcRecall=0.9231 ≥ raw hybrid Recall@5=0.9231
- Negative control: lexical-only would give ~0.08, gate discriminates

### D. Agent integration (12 assertions)
- Lifecycle: plan→apply→status→second apply (0 diff) for cursor/claude-code/codex
- Unowned config conflict: `agent_config_conflict` error, file unchanged, exit 1
- Unbalanced markers: detected as conflict
- Generated config: valid JSON with `command: okf`, `args: [mcp, --repo, .]`, `OKF_MANAGED` env
- MCP startup: generated config's server responds to `initialize` with `okf-mcp-server`

### E. Compatibility (3 assertions)
- Legacy bundle (no IDs): manifest and search work normally
- Writer preserves ID: existing okf_id not overwritten on re-import
- v2 index: status warns incompatibility; remediation suggests rebuild

### F. Recoverability (8 assertions)
- Error rubric: `invalid_concept_id`, `invalid_request` all have code + message + remediation
- Exit codes: all failures exit 1; successes exit 0
- JSON clean: stdout is valid JSON; errors on stderr
- Help examples: identity/agent help includes copy-pasteable examples

### G. Real agent smoke (2 assertions)
- MCP stdio: `initialize` responds with server info
- `tools/list`: includes okf_* tools

## Negative control

The usability script includes a negative control proving it catches regressions: hybrid folder recall=0.9231 vs lexical ~0.08. If the hybrid strategy were broken (e.g. reverted to lexical), the C7 gate would fail.

## Remaining limitations / user decisions needed

1. **S46 spec-amendment is Proposed** (not self-approved). The historical baseline 0.9615/0.7256 is not reproducible from the current tree; base-vs-head confirms zero code regression. User must decide whether to approve the amendment (updating baseline to reproducible 0.9231/0.6615) or investigate the historical measurement. Until approved, S46 conformance is `partial`.

2. **v2 index remediation** is informational (status warns); full v2→v3 migration requires `okf vector rebuild`. This is by design (rebuild is a deliberate, potentially slow operation).

3. **Agent config generation** uses `"command": "okf"` assuming `okf` is in PATH. For users who install to a non-standard location, they need to adjust the path or add to PATH. The config comments explain this.

## Verification commands

```bash
# Full usability suite (64 assertions)
bash tools/verify-agent-usability.sh

# Full regression suite
bash tools/gauntlet.sh

# MCP E2E
python3 test_mcp.py
```
