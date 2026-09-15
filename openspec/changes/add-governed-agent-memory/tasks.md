# Implementation Tasks — add-governed-agent-memory

## Completion contract

P0–P4 are dependency order only. Completion requires all 34 Scenarios, both new fields, all tool extensions, validation, benchmarks, and `conformance.md`. One phase is not releasable.

## P0 — Data model and validation (0.75 person-days)

### T0.1 Governance field on Concept
- **Files**: `pkg/okf/types.go`, `pkg/okf/parser.go`, `pkg/okf/validator.go`
- **RED first**: parse `governance: Constraint` → normalized `constraint`; unknown → `context` + warning; `convention/` default.
- **Implement**: `GovernanceLevel` type, `EffectiveGovernance()` method, parser support, validator strict/non-strict modes.
- **Tests**: table-driven parse/normalize/default/validation.
- **Scenarios**: S01–S03, S07, S31–S33.

### T0.2 code_refs field on Concept
- **Files**: `pkg/okf/types.go`, `pkg/okf/parser.go`, `pkg/okf/validator.go`
- **RED first**: parse list, normalize paths, reject absolute/traversal, validate glob syntax.
- **Implement**: `CodeRefs []string`, path normalization, glob validation, traversal rejection.
- **Tests**: path normalization, traversal rejection, glob syntax validation, empty/missing.
- **Scenarios**: S08, S12–S13, S31–S32.

## P1 — for_path and governance filtering (1.0 person-day)

### T1.1 for_path matching engine
- **Files**: `pkg/tool/manifest.go`, new `pkg/tool/coderefs.go`
- **RED first**: exact match, single-segment glob, recursive glob depth limit, symlink escape.
- **Implement**: `MatchCodeRefs(path string, patterns []string) (matched string, ok bool)`, bounded recursive matcher (max 8 levels, max 1000 files).
- **Tests**: S09–S11, S13, S14; precision/recall fixture.
- **Scenarios**: S09–S14, S16.

### T1.2 Manifest/Query extensions
- **Files**: `pkg/tool/manifest.go`, `pkg/tool/query.go`, `pkg/mcp/tools.go`
- **RED first**: `--for_path` returns matching concepts; `--governance` filters; sort hold→constraint→context.
- **Implement**: `for_path` and `governance` params on ManifestRequest/QueryRequest; MCP tool schema updates.
- **Tests**: filter, sort, combined for_path+governance, hold_warning field.
- **Scenarios**: S04–S06, S16.

### T1.3 Stale code_refs detection
- **Files**: `pkg/tool/manifest.go`
- **RED first**: `--stale-refs` lists concepts with non-existent code_refs.
- **Implement**: advisory stale detection, no auto-fix.
- **Tests**: S15.
- **Scenarios**: S15.

## P2 — Search-before-write advisory (1.0 person-day)

### T2.1 Duplicate detection engine
- **Files**: `pkg/tool/write.go`, `pkg/tool/duplicate.go` (new)
- **RED first**: BM25 search over existing concepts, score normalization, classification (no_duplicate/possible_duplicate/possible_conflict).
- **Implement**: `CheckDuplicates(title, body, conceptType) DuplicateCheckResult`, reuse existing BM25 index.
- **Tests**: novel content → no_duplicate; similar → possible_duplicate; conflicting type → possible_conflict; threshold config.
- **Scenarios**: S17–S19, S23.

### T2.2 Write integration and audit trace
- **Files**: `pkg/tool/write.go`, `pkg/okf/types.go`
- **RED first**: `--check-duplicates` returns advisory result; `--allow-duplicate` overrides; audit trace persisted.
- **Implement**: duplicate check before write, `search_before_write` frontmatter, `allow_duplicate` param.
- **Tests**: S17–S21, concurrent writes.
- **Scenarios**: S17–S22.

### T2.3 Performance and concurrency
- **Files**: `pkg/tool/duplicate.go`, benchmarks
- **RED first**: < 50ms added latency with 1000 concepts; concurrent-safe.
- **Implement**: index reuse, read-only check, benchmark.
- **Tests**: S22, S24; benchmark with 1000-concept fixture.
- **Scenarios**: S22, S24.

## P3 — Progressive disclosure (0.75 person-day)

### T3.1 Manifest summary/hit modes
- **Files**: `pkg/tool/manifest.go`, `pkg/mcp/tools.go`
- **RED first**: `--mode summary` returns minimal fields; `--mode hit` includes code_refs/tags; `--mode full` backward compatible.
- **Implement**: `ManifestMode` type, field projection, token estimation.
- **Tests**: S25–S27, S30.
- **Scenarios**: S25–S27, S30.

### T3.2 Token budget and truncation
- **Files**: `pkg/tool/manifest.go`
- **RED first**: `--max-tokens N` limits concept count; `truncated: true` flag; governance priority sort.
- **Implement**: token-aware pagination, stable sort, total count.
- **Tests**: S28.
- **Scenarios**: S28.

### T3.3 Recall@K verification
- **Files**: benchmarks, `pkg/tool/manifest_test.go`
- **RED first**: summary mode Recall@5 == full mode on golden queries.
- **Implement**: golden query fixture, recall measurement.
- **Tests**: S29.
- **Scenarios**: S29.

## P4 — Agent Skill, docs, and final alignment (0.5 person-day)

### T4.1 MCP Skill workflow extension
- **Files**: `pkg/agentconfig/workflow.go`, `pkg/agentconfig/workflow_test.go`
- **RED first**: W02 gains hold check; W03 gains for_path lookup; W01 uses summary mode.
- **Implement**: extend canonical clauses W01/W02/W03 with governance/for_path guidance.
- **Tests**: workflow coverage, no ownership markers.
- **Scenarios**: S06 (indirect), all agent-facing scenarios.

### T4.2 Documentation and release notes
- **Files**: `docs/knowledge/`, `README.md`, `README.zh-CN.md`, release notes.
- **Cover**: governance semantics, code_refs syntax, for_path usage, search-before-write, progressive disclosure, non-goals.
- **Tests**: documentation contract tests (required/prohibited claims).
- **Scenarios**: all (documentation).

### T4.3 Final evidence and conformance
- **Fresh run**: gauntlet, build/vet/test/race/shuffle, benchmarks, for_path precision/recall, duplicate detection recall/FPR, token savings.
- **Gate**: `conformance.md` maps S01–S34. No unexplained partial/gap.
- **Scenarios**: S34, all.

## Scenario → test → real entry-point matrix

| Scenario | Primary automated test | Real entry point |
|---|---|---|
| S01–S03 | governance parse/default tests | `okf lint`, concept parse |
| S04–S06 | manifest governance filter/sort tests | `okf manifest --governance` |
| S07 | validator strict mode test | `okf lint --strict` |
| S08 | code_refs parse test | concept parse |
| S09–S11 | for_path matching tests | `okf manifest --for_path` |
| S12–S13 | path normalization/traversal tests | `okf lint` |
| S14 | matched pattern transparency test | `okf manifest --for_path` |
| S15 | stale-refs detection test | `okf manifest --stale-refs` |
| S16 | for_path+governance combined test | `okf manifest --for_path --governance` |
| S17–S19 | duplicate detection classification tests | `okf note --check-duplicates` |
| S20 | allow_duplicate override test | `okf note --allow-duplicate` |
| S21 | audit trace persistence test | persisted concept frontmatter |
| S22 | concurrent write test | `go test -race` |
| S23 | threshold config test | `okf note --dup-threshold` |
| S24 | duplicate check benchmark | `go test -bench` |
| S25–S27 | manifest mode tests | `okf manifest --mode` |
| S28 | token budget test | `okf manifest --max-tokens` |
| S29 | Recall@K golden test | golden query fixture |
| S30 | for_path + mode combined test | `okf manifest --for_path --mode` |
| S31–S33 | backward compat tests | existing test suite + new |
| S34 | strict validation test | `okf lint --strict` |

## Estimated schedule

| Phase | Estimate | Exit criterion |
|---|---:|---|
| P0 | 0.75 d | governance + code_refs data model and validation |
| P1 | 1.0 d | for_path matching, governance filtering, stale refs |
| P2 | 1.0 d | search-before-write advisory + audit trace |
| P3 | 0.75 d | progressive disclosure summary/hit/token budget |
| P4 | 0.5 d | Skill extension, docs, conformance |
| **Total** | **4.0 person-days** | all phases, not only P0 |
