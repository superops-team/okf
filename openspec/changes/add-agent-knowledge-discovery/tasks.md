# Implementation Tasks

## Completion contract

P0-P4 are dependency order only. This change is complete only when all four product capabilities, all 50 Scenarios, all public entry points, all documentation, final EVIDENCE and `conformance.md` are complete. A phase is not a releasable subset.

## P0 — Stable identity foundation (3.0 person-days)

### T0.1 Canonical identity package
- **Files**: `pkg/identity/identity.go`, `identity_test.go`, `identity_property_test.go`
- **Implement**: grammar, `crypto/rand` generator, ref/URI, registry, resolver, duplicate/invalid errors.
- **Wiring**: pure package used by writers, Manifest, query keying and Service DTOs.
- **Tests**: `TestLegacyIdentityState`, `TestStableIDRoundTrip`, `TestInvalidStableID`, `TestDuplicateStableID`, `TestNewStableID`, `TestStableIDEntropyFailure`, `TestStableIDProperties`.
- **Scenarios**: S01–S05, S11.

### T0.2 Canonical OKF→query conversion
- **Files**: `pkg/query/adapter.go`, CLI/MCP call sites and tests.
- **Implement**: one conversion that copies all query fields and defensively clones `CustomFields`/tags.
- **Remove**: divergent `toQueryBundle` and `mcpToQueryBundle` implementations.
- **Tests**: `TestConceptAdapterPreservesCustomFields`, `TestConceptAdapterDoesNotAliasMaps`, CLI/MCP parity.
- **Scenarios**: S02, S13.

### T0.3 Writer identity wiring
- **Files**: shared final-destination persistence helper plus `pkg/git.SaveKnowledgeBase`, `pkg/okf.SmartImportSource` final writes, MCP document import, derived chunk persistence and durable `pkg/tool.WriteKnowledge`.
- **Implement**: do not add random IDs in conversion staging; at the final target preserve an existing valid `okf_id`, generate only for a new owned target, and persist `parent_okf_id` on derived chunks. Keep durable-capture `concept_id` unchanged as its deterministic idempotency/path handle and add stable ref fields only additively.
- **Tests**: table-driven `TestOwnedWritersPersistStableID`, `TestOwnedWriterRefreshPreservesStableID`, `TestConversionStagingHasNoRandomID`, `TestDerivedChunkHasParentID`, existing idempotency suites.
- **Scenarios**: S12.

### T0.4 Explicit migration command
- **Files**: `pkg/identity/migrate.go`, `pkg/tool` resolver operation, `cmd/okf/cmd_identity.go`, `pkg/mcp/tools.go` and tests.
- **Implement**: dry-run/apply migration plus shared Service/CLI/MCP stable-ref resolution, per-file atomic replacement, cross-file rollback attempt, path safety, JSON output.
- **Tests**: `TestIdentityEnsureDryRunDeterministic` (no entropy reads), `TestIdentityEnsureIdempotent`, `TestIdentityEnsurePreflight`, `TestIdentityEnsureRollback`, `TestIdentityEnsureUnsafePath`, `TestStableRefSurvivesMove`, `TestStableRefNotFound`, CLI/MCP parity.
- **Scenarios**: S06–S10, S16.

### T0.5 Identity-aware vector index v3
- **Files**: `pkg/query/semantic.go`, vector index metadata/status/build/load code and tests.
- **Implement**: v3 stable/legacy/chunk keys, strict v2 rejection, rebuild remediation.
- **Tests**: `TestIdentityAwareIndexKeys`, `TestVectorV2RequiresRebuild`, `TestIdentityMigrationReportsRebuild`.
- **Scenarios**: S14–S16.

## P1 — Metadata-only Manifest (2.5 person-days)

### T1.1 Bounded frontmatter reader
- **Files**: `pkg/manifest/reader.go`, tests and fixtures.
- **Implement**: 256 KiB cap, delimiter detection, metadata decode, injected reader/stat boundary.
- **Tests**: `TestManifestDoesNotParseBody` with instrumented bytes-read bound, `TestManifestHeaderLimit`, `TestManifestMissingDelimiter`, `TestManifestInvalidYAML` fixtures and stable warning codes.
- **Scenarios**: S20–S23.

### T1.2 Manifest model, filters and index observation
- **Files**: `pkg/manifest/manifest.go`, tests.
- **Implement**: DTO, identity registry, filters, status/trust/stale/source summary, token estimate, deterministic order, passive index status.
- **Tests**: `TestManifestDefaults`, `TestManifestPaginationValidation`, `TestManifestFilters`, `TestManifestMetadata`, `TestManifestTokenEstimate`, `TestManifestIndexObservation`, `TestManifestDuplicateIDFails`.
- **Scenarios**: S17–S25.

### T1.3 Service, CLI and MCP wiring
- **Files**: `pkg/tool/service.go`, `cmd/okf/cmd_tool.go`, `pkg/mcp/tools.go`, tests, `test_mcp.py`.
- **Implement**: `OperationManifest`, request decoder with explicit limit-presence semantics, `okf tool manifest`, `okf_manifest`.
- **Tests**: `TestServiceManifest`, `TestToolManifestJSON`, `TestMCPManifestSchema`, `TestMCPManifestParity`, stdio E2E.
- **Scenarios**: S17–S26.

## P2 — Hierarchical retrieval projection (2.5 person-days)

### T2.1 Shared projection engine
- **Files**: `pkg/query/project.go`, tests and property tests.
- **Implement**: shared `ResultHit`, safe path keys, parent identity, representatives, counts, deterministic order and group-limit shaping over the existing bounded scored candidate window.
- **Tests**: `TestProjectChunk`, `TestProjectConcept`, `TestProjectSource`, `TestProjectFolder`, `TestProjectDeterministic`, `TestProjectInvalidGroup`, properties for idempotence/path normalization.
- **Scenarios**: S28–S33.

### T2.2 Public query wiring
- **Files**: classic CLI search, `pkg/tool.Service.Query`, MCP legacy/service handlers and tests.
- **Implement**: optional `group_by`, `include_group_members`, additive groups; omitted behavior unchanged; when grouping is explicit, expose the already sorted bounded candidates before current final dedupe/TopK shaping and truncate after projection.
- **Tests**: `TestGroupingOmittedCompatibility`, `TestGroupingEntryPointParity`, CLI golden, MCP JSON contract.
- **Scenarios**: S27, S34.

### T2.3 Retrieval evaluation extension
- **Files**: `pkg/eval`, golden queries and report fixtures.
- **Implement**: relevant-source Recall@K, NDCG, diversity, same-source occupancy for grouping; do not change RRF defaults based on this task alone.
- **Tests**: metric unit tests, deterministic rebuild comparison, baseline gate.
- **Scenarios**: S46–S48.

## P3 — Agent Integration (3.5 person-days)

### T3.1 Canonical workflow model
- **Files**: `pkg/agentconfig/workflow.go`, templates, golden tests.
- **Implement**: clause IDs for status→manifest/query→context→task→optional persistence, real tool/command inventory validation.
- **Tests**: `TestCanonicalWorkflowCoverage`, `TestGeneratedToolsExist`, per-client golden.
- **Scenarios**: S37–S38.

### T3.2 Cursor adapter
- **Files**: `pkg/agentconfig/cursor.go`, fixtures/tests.
- **Implement**: semantic merge of `.cursor/mcp.json:mcpServers.okf` with non-secret `OKF_MANAGED=agentconfig-v1` ownership marker; owned `.cursor/rules/okf.md`.
- **Tests**: semantic preservation of unknown JSON, byte preservation outside TOML/Markdown markers, malformed JSON conflict, apply twice, status, remove.
- **Scenarios**: S35–S43, S45.

### T3.3 Claude Code adapter
- **Files**: `pkg/agentconfig/claude.go`, fixtures/tests.
- **Implement**: semantic merge of `.mcp.json:mcpServers.okf` with non-secret `OKF_MANAGED=agentconfig-v1` ownership marker; owned `.claude/skills/okf/SKILL.md`.
- **Tests**: same shared conformance suite plus skill frontmatter validation.
- **Scenarios**: S35–S43, S45.

### T3.4 Codex adapter
- **Files**: `pkg/agentconfig/codex.go`, fixtures/tests.
- **Implement**: delimited `.codex/config.toml` MCP block and root `AGENTS.md` managed block; conflict with unowned table.
- **Tests**: comments/unknown TOML preserved, marker balance, AGENTS content preserved, apply twice, remove.
- **Scenarios**: S35–S43, S45.

### T3.5 Plan/apply/status/remove orchestration
- **Files**: `pkg/agentconfig/service.go`, `cmd/okf/cmd_agent.go`, self-describing ownership helpers and tests.
- **Implement**: project boundary, per-file atomic replacement plus cross-file rollback, self-describing ownership, redaction, secret guard and explicit confirmation semantics (`apply --yes`, `remove --yes` in non-interactive mode).
- **Tests**: `TestAgentPlanReadOnly`, `TestAgentMutationRequiresConfirmation`, shared adapter conformance, `TestAgentRollback`, `TestAgentSecretRedaction`, `TestAgentSymlinkEscape`, real CLI fixture E2E.
- **Scenarios**: S35–S45.

## P4 — Documentation, quality gate and conformance (2.0 person-days)

### T4.1 User and developer documentation
- **Files**: README, README.zh-CN, CLI/MCP/agent knowledge docs, Release Notes.
- **Cover**: optional ID, explicit migration, index rebuild, Manifest limits, grouping compatibility, client ownership/uninstall, no credential persistence.
- **Tests**: command help golden and documentation contract assertions.
- **Scenarios**: S01, S06, S15, S17, S27, S35–S45.

### T4.2 Expand gauntlet and mutation set
- **Files**: `tools/gauntlet.sh`, `tools/mutants.sh` or a separate persisted targeted mutation script.
- **Mutants**: accept duplicate ID; read past frontmatter; allow path escape; recompute group score; overwrite unknown client config.
- **Negative controls**: each new custom gate must be demonstrated failing on a known-bad fixture.
- **Scenarios**: S49.

### T4.3 Real execution and performance evidence
- **Persisted entry point**: `tools/verify-agent-discovery.sh`.
- **Run**: identity dry-run/apply/rename/resolve; Manifest CLI/MCP; grouped query; all-client plan/apply/status/remove; current retrieval eval; 1,000-file benchmark.
- **Record**: actual counts, bytes read, evaluation metrics, command versions and commit SHA in `evidence.md`.
- **Scenarios**: S11, S22, S26, S34–S49.

### T4.4 Spec-to-implementation audit
- **File**: `conformance.md`.
- **Implement**: map S01–S50 to code, test, last fresh command/result and status `fully|aligned|partial|gap`.
- **Gate**: no unexplained partial/gap; no scenario without an executable test.
- **Scenarios**: S50.

## Scenario → test → entry-point matrix

| Scenario | Primary automated test/evidence | Real entry point |
|---|---|---|
| S01 | `TestLegacyIdentityState` + legacy regression | parse/lint/query |
| S02 | `TestStableIDRoundTrip` | parser/serializer/adapters |
| S03 | `TestInvalidStableID` | identity/manifest CLI/MCP |
| S04 | `TestDuplicateStableID` | registry/manifest/migration |
| S05 | `TestNewStableID`, property test | identity generator |
| S06 | `TestIdentityEnsureDryRunDeterministic` | `okf identity ensure` |
| S07 | `TestIdentityEnsureIdempotent` | `okf identity ensure --apply` |
| S08 | `TestIdentityEnsurePreflight` | migration apply |
| S09 | `TestIdentityEnsureRollback` | injected filesystem apply |
| S10 | `TestIdentityEnsureUnsafePath` | migration CLI |
| S11 | `TestStableRefSurvivesMove` + CLI/MCP parity | Service `Resolve` / `okf identity resolve` / `okf_resolve` |
| S12 | `TestOwnedWritersPersistStableID` | generator/import/note/log/feedback |
| S13 | `TestConceptAdapterPreservesCustomFields` | CLI/MCP/Service conversion |
| S14 | `TestIdentityAwareIndexKeys` | vector build |
| S15 | `TestVectorV2RequiresRebuild` | vector status/search |
| S16 | `TestIdentityMigrationReportsRebuild` | migration result |
| S17 | `TestManifestDefaults` | Service/CLI/MCP |
| S18 | `TestManifestPaginationValidation` | public decoders |
| S19 | `TestManifestFilters` | Manifest service |
| S20 | `TestManifestMetadata` | Manifest service |
| S21 | `TestManifestTokenEstimate` | Manifest service |
| S22 | `TestManifestDoesNotParseBody` | bounded reader |
| S23 | `TestManifestHeaderLimit` | bounded reader |
| S24 | `TestManifestIndexObservation` | Service Manifest |
| S25 | `TestManifestDuplicateIDFails` | Service/CLI/MCP |
| S26 | `TestMCPManifestParity` + stdio E2E | CLI/MCP |
| S27 | `TestGroupingOmittedCompatibility` | all search entries |
| S28 | `TestProjectChunk` | shared projector |
| S29 | `TestProjectConcept` | shared projector |
| S30 | `TestProjectSource` | shared projector |
| S31 | `TestProjectFolder` | shared projector |
| S32 | `TestProjectDeterministic` + property | shared projector |
| S33 | `TestProjectInvalidGroup` | CLI/MCP/Service |
| S34 | `TestGroupingEntryPointParity` | CLI/MCP/Service |
| S35 | `TestAgentPlanReadOnly` | `okf agent plan` |
| S36 | shared `TestAdapterIdempotenceAndPreservation` | `okf agent apply` |
| S37 | `TestCanonicalWorkflowCoverage` | rendered rules/skills |
| S38 | per-client golden/validator | adapter files |
| S39 | shared `TestAdapterStatusStates` | `okf agent status` |
| S40 | shared `TestAdapterRemoveOwnership` | `okf agent remove` |
| S41 | shared `TestAdapterConflicts` | plan/apply |
| S42 | `TestAgentSecretRedaction` + secret scan | plan/status/error output |
| S43 | `TestAgentSymlinkEscape` | all mutating agent commands |
| S44 | `TestAgentRollback` | `okf agent apply` |
| S45 | `TestUnsupportedAgentClient` | all agent commands |
| S46 | existing eval + baseline gate | `okf eval` |
| S47 | grouped eval tests/report | `okf eval -group-by` |
| S48 | Manifest/projector benchmarks | persisted verification script |
| S49 | final fresh gauntlet evidence | `tools/verify-agent-discovery.sh` |
| S50 | conformance checker | `conformance.md` |

## Schedule (dependency planning, not delivery slicing)

| Phase | Estimate | Dependency |
|---|---:|---|
| P0 Stable identity | 3.0 person-days | none |
| P1 Manifest | 2.5 person-days | P0 registry/ref |
| P2 Hierarchical projection | 2.5 person-days | P0 identity adapter |
| P3 Agent Integration | 3.5 person-days | P1/P2 public contracts |
| P4 Evidence/conformance | 2.0 person-days | P0–P3 |
| **Total** | **13.5 person-days** | complete only after P4 |

## Merge checklist

- [x] S01–S50 all mapped and green.
- [x] No new third-party dependency, or spec revised and explicitly approved before addition.
- [x] Existing no-group query output compatibility proven.
- [x] Current hybrid baseline does not regress (base-vs-head zero regression; S46 amendment user-approved 2026-09-13).
- [x] `tools/gauntlet.sh` and new targeted mutants pass.
- [x] Real CLI and MCP stdio flows pass.
- [x] Cursor/Claude/Codex fixture apply→apply→status→remove passes.
- [x] Secret scan passes and no credential is present in fixtures/state/output.
- [x] `review.md` has no unresolved critical/high issue.
- [x] `conformance.md` has no unexplained partial/gap (S46 fully after user-approved amendment).
