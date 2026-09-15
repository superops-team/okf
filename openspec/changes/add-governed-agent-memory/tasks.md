# Implementation Tasks — add-governed-agent-memory

## Completion contract

P0–P4 are dependency order only. Completion requires all 40 Scenarios, pkg/memorymeta accessor, all tool extensions, validation, golden set, benchmarks, and conformance.md. One phase is not releasable.

## P0 — memorymeta accessor and validation (0.75 person-days)

### T0.1 pkg/memorymeta package
- **Files**: `pkg/memorymeta/governance.go`, `pkg/memorymeta/coderefs.go`, `pkg/memorymeta/validate.go`
- **RED first**: Governance reads from CustomFields, normalizes, defaults to context; unknown → context+warning (non-strict) / error (strict).
- **Implement**: GovernanceLevel type, Governance(), SetGovernance(), CodeRefs(), SetCodeRefs(), Validate().
- **Tests**: table-driven parse/normalize/default/validation.
- **Scenarios**: S01–S04, S09, S37–S38.

### T0.2 Concept parser/serializer integration
- **Files**: `pkg/okf/parser.go` (no struct change; CustomFields already captures unknown fields)
- **RED first**: governance/code_refs in frontmatter appear in CustomFields; re-serialization preserves them.
- **Implement**: verify CustomFields already captures governance/code_refs; add round-trip tests.
- **Tests**: frontmatter → CustomFields → frontmatter round-trip.
- **Scenarios**: S01, S10, S38.

## P1 — for_path and governance filtering (1.0 person-day)

### T1.1 for_path matching engine
- **Files**: `pkg/memorymeta/coderefs.go`, `pkg/tool/manifest.go`
- **RED first**: exact match, single-segment glob, recursive glob depth 8 limit, path canonicalization, rejection of absolute/../NUL/backslash.
- **Implement**: MatchCodeRefs(path string, patterns []string) (matched string, bool), bounded recursive matcher (8 levels), lexical only (no FS).
- **Tests**: S11–S15, S19; precision/recall fixture.
- **Scenarios**: S11–S15, S19.

### T1.2 Manifest/Query extensions
- **Files**: `pkg/tool/manifest.go`, `pkg/tool/query.go`, `pkg/mcp/tools.go` (both eras)
- **RED first**: `--for_path` returns matching concepts; `--governance` filters; default order unchanged; governance sort only with new params; hold warning.
- **Implement**: for_path, governance params on ManifestRequest/QueryRequest; MCP tool schema updates (modern + legacy).
- **Tests**: S05–S08, S16; default order regression test.
- **Scenarios**: S05–S08, S16.

### T1.3 stale-refs FS scanner
- **Files**: `pkg/tool/manifest.go`
- **RED first**: `--stale-refs` lists concepts with non-existent code_refs; symlink escape/unreadable → incomplete+warnings; 50k file bound.
- **Implement**: advisory stale detection, fail-closed on errors, no auto-fix.
- **Tests**: S17–S18.
- **Scenarios**: S17–S18.

## P2 — memory_check via okf_query (1.0 person-day)

### T2.1 Duplicate detection engine
- **Files**: `pkg/tool/query.go`, `pkg/memorymeta/duplicate.go` (new)
- **RED first**: BM25 top-10 candidates, Jaccard [0,1] re-rank, classification no_similar/possible_duplicate, configurable threshold.
- **Implement**: CheckMemory(content, kind, tags) MemoryCheckResult; reuse existing BM25 index (verify cache behavior); Jaccard on title+desc+first-500-chars.
- **Tests**: S20–S23, S28; golden set ≥40 cases.
- **Scenarios**: S20–S23, S28.

### T2.2 Query integration and audit trace
- **Files**: `pkg/tool/query.go`, `pkg/tool/write.go`
- **RED first**: `--memory-check true` returns advisory (read-only); no allow_duplicate; audit trace bounded (3 candidates, 256 bytes); idempotent replay.
- **Implement**: memory_check param on QueryRequest; Metadata memory_check field; no write blocking.
- **Tests**: S24–S27.
- **Scenarios**: S24–S27.

### T2.3 Golden set and calibration
- **Files**: `pkg/memorymeta/duplicate_test.go`, testdata/
- **RED first**: ≥40 cases (positives: actual note/decision duplicates; negatives: common words, Chinese, code identifiers); confusion matrix; Precision/Recall/FPR.
- **Implement**: golden query fixture, measurement script.
- **Tests**: S21–S22, S28; pilot confusion matrix documented.
- **Scenarios**: S21–S22, S28.

## P3 — Progressive disclosure (0.75 person-day)

### T3.1 Manifest summary/hit modes
- **Files**: `pkg/tool/manifest.go`, `pkg/mcp/tools.go`
- **RED first**: `--mode summary` minimal fields; `--mode hit` adds code_refs/tags/status; `--mode full` backward compatible.
- **Implement**: ManifestMode type, field projection, JSON bytes/4 token estimate.
- **Tests**: S29–S31, S35; real token measurement on 329-concept corpus.
- **Scenarios**: S29–S31, S35.

### T3.2 max_tokens budget and ID parity
- **Files**: `pkg/tool/manifest.go`
- **RED first**: filter→sort→offset→limit→token budget; next_offset/omitted_count/truncated; budget_too_small error with min_required_tokens; ID parity across modes.
- **Implement**: token-aware pagination, stable sort, error handling.
- **Tests**: S32–S34.
- **Scenarios**: S32–S34.

### T3.3 Follow-up via query/resolve
- **Files**: `pkg/tool/query.go`, `pkg/tool/identity_resolve.go`
- **RED first**: `okf tool query --ref <okf_id>` returns full body; `okf tool resolve --ref` works.
- **Implement**: verify existing ref-based query works; document as follow-up path.
- **Tests**: S36.
- **Scenarios**: S36.

## P4 — Agent Skill, docs, CLI, final alignment (0.5 person-day)

### T4.1 MCP Skill workflow extension
- **Files**: `pkg/agentconfig/workflow.go`, `pkg/agentconfig/workflow_test.go`
- **RED first**: W01 uses summary mode; W02 checks holds via for_path; W03 uses for_path; W06 instructs memory_check-then-write.
- **Implement**: extend canonical clauses W01/W02/W03/W06.
- **Tests**: workflow coverage, no ownership markers.
- **Scenarios**: S07 (indirect), all agent-facing.

### T4.2 CLI and MCP wiring
- **Files**: `cmd/okf/main.go` (tool subcommand), `pkg/mcp/tools.go` (both eras)
- **RED first**: `okf tool manifest --for-path --mode --max-tokens`, `okf tool query --for-path --governance --memory-check` work via CLI; MCP okf_manifest/okf_query gain params in both eras.
- **Implement**: CLI flag parsing, MCP schema updates.
- **Tests**: S40; CLI smoke tests.
- **Scenarios**: S40.

### T4.3 Documentation and release notes
- **Files**: `docs/knowledge/`, `README.md`, `README.zh-CN.md`, release notes.
- **Cover**: CustomFields approach, governance values, code_refs syntax, for_path usage, memory_check, progressive disclosure, non-goals.
- **Tests**: documentation contract tests.
- **Scenarios**: all (documentation).

### T4.4 Final evidence and conformance
- **Fresh run**: gauntlet, build/vet/test/race/shuffle, benchmarks, for_path precision/recall, duplicate golden confusion matrix, token measurement.
- **Gate**: conformance.md maps S01–S40逐行. No unexplained partial/gap.
- **Scenarios**: S39, all.

## Scenario → test → real entry-point matrix (逐行)

| Scenario | Primary automated test | Real entry point |
|---|---|---|
| S01 | memorymeta governance parse test | `memorymeta.Governance()` |
| S02 | default governance test | `memorymeta.Governance()` |
| S03 | explicit governance test | `memorymeta.Governance()` |
| S04 | unknown value strict/non-strict test | `memorymeta.Validate()` |
| S05 | default order regression test | `okf tool manifest` |
| S06 | governance sort activation test | `okf tool manifest --governance` |
| S07 | hold advisory warning test | `okf tool manifest --for-path` |
| S08 | governance filter test | `okf tool manifest --governance` |
| S09 | SetGovernance CustomFields test | `memorymeta.SetGovernance()` |
| S10 | code_refs parse test | `memorymeta.CodeRefs()` |
| S11 | for_path exact match test | `okf tool manifest --for-path` |
| S12 | for_path single-glob test | `okf tool manifest --for-path` |
| S13 | for_path recursive depth test | `okf tool manifest --for-path` |
| S14 | path canonicalization/rejection test | `okf tool manifest --for-path` |
| S15 | not-yet-created path test | `okf tool manifest --for-path` |
| S16 | code_refs pattern bounds test | `memorymeta.CodeRefs()` |
| S17 | stale-refs fail-closed test | `okf tool manifest --stale-refs` |
| S18 | stale-refs scan bound test | `okf tool manifest --stale-refs` |
| S19 | code_refs vs code_file test | `okf tool manifest --for-path` |
| S20 | memory_check no_similar test | `okf tool query --memory-check` |
| S21 | memory_check possible_duplicate test | `okf tool query --memory-check` |
| S22 | BM25+Jaccard pipeline test | `memorymeta.CheckMemory()` |
| S23 | no conflict classification test | `okf tool query --memory-check` |
| S24 | no allow_duplicate test | `okf tool note` (always writes) |
| S25 | memory_check determinism test | `okf tool query --memory-check` |
| S26 | concurrent memory_check test | `go test -race` |
| S27 | audit trace bounds test | persisted concept Metadata |
| S28 | threshold config test | `okf tool query --dup-threshold` |
| S29 | summary mode fields test | `okf tool manifest --mode summary` |
| S30 | hit mode fields test | `okf tool manifest --mode hit` |
| S31 | full mode backward compat test | `okf tool manifest` |
| S32 | ID parity test | `okf tool manifest --mode` |
| S33 | max_tokens pipeline test | `okf tool manifest --max-tokens` |
| S34 | budget_too_small test | `okf tool manifest --max-tokens` |
| S35 | JSON bytes/4 token test | manifest response measurement |
| S36 | follow-up query/resolve test | `okf tool query --ref` |
| S37 | backward compat test | existing test suite |
| S38 | CustomFields preservation test | concept round-trip |
| S39 | strict validation test | `okf tool lint --strict` |
| S40 | CLI/MCP entry point test | `okf tool manifest/query`, MCP okf_manifest/okf_query |

## Estimated schedule

| Phase | Estimate | Exit criterion |
|---|---:|---|
| P0 | 0.75 d | memorymeta package + validation |
| P1 | 1.0 d | for_path matching, governance filter, stale-refs |
| P2 | 1.0 d | memory_check + golden set calibration |
| P3 | 0.75 d | summary/hit modes + max_tokens |
| P4 | 0.5 d | Skill, CLI/MCP wiring, docs, conformance |
| **Total** | **4.0 person-days** | all phases, not only P0 |
