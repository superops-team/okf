# Conformance Audit — Planned Gate

> Status: implementation not started. This file is a planning gate, not implementation evidence. Every row remains `gap` until the cited behavior exists and is verified by a final fresh run.

## Audit metadata

- Source commit: pending implementation
- Verification timestamp: pending implementation
- Go/tool versions: Go 1.26.0 / toolchain go1.26.7 planned; final versions must be captured by the verification script
- One-command entry point: `tools/verify-agent-discovery.sh` (to be implemented in T4.3)
- Spec approval: pending

## Scenario mapping

| Scenario | Requirement | Planned implementation path/symbol | Planned automated test/evidence | Fresh command/result | Alignment |
|---|---|---|---|---|---|
| S01 | Optional stable identity | `pkg/identity.FromConcept`; existing parse/lint/query | `TestLegacyIdentityState` + legacy regression | Not run; blocked by P0 | gap |
| S02 | Optional stable identity | parser/serializer + canonical query adapter | `TestStableIDRoundTrip` | Not run; blocked by P0 | gap |
| S03 | Optional stable identity | `identity.Parse`, registry/Manifest/resolve validation | `TestInvalidStableID` | Not run; blocked by P0 | gap |
| S04 | Optional stable identity | `identity.BuildRegistry` | `TestDuplicateStableID` | Not run; blocked by P0 | gap |
| S05 | Optional stable identity | `identity.New` with injectable entropy source | `TestNewStableID`, `TestStableIDEntropyFailure`, properties | Not run; blocked by P0 | gap |
| S06 | Explicit migration | `okf identity ensure` dry-run planner | `TestIdentityEnsureDryRunDeterministic` | Not run; blocked by P0 | gap |
| S07 | Explicit migration | identity apply writer | `TestIdentityEnsureIdempotent` | Not run; blocked by P0 | gap |
| S08 | Explicit migration | migration preflight | `TestIdentityEnsurePreflight` | Not run; blocked by P0 | gap |
| S09 | Explicit migration | migration atomic-file writer/rollback | `TestIdentityEnsureRollback` | Not run; blocked by P0 | gap |
| S10 | Explicit migration | migration safe path resolver | `TestIdentityEnsureUnsafePath` | Not run; blocked by P0 | gap |
| S11 | Stable resolution | `Service.Resolve`; CLI `identity resolve`; MCP `okf_resolve` | `TestStableRefSurvivesMove`, not-found and parity tests | Not run; blocked by P0 | gap |
| S12 | Writer identity wiring | final-destination writer helper; generator/import/MCP/capture/chunks | owned-writer table tests + staging/refresh/parent-ID tests | Not run; blocked by P0 | gap |
| S13 | Query metadata adapter | canonical OKF→query conversion | `TestConceptAdapterPreservesCustomFields`, no-alias/parity tests | Not run; blocked by P0 | gap |
| S14 | Index identity | query/vector v3 key builder | `TestIdentityAwareIndexKeys` | Not run; blocked by P0 | gap |
| S15 | Index compatibility | vector metadata load/status/search | `TestVectorV2RequiresRebuild` | Not run; blocked by P0 | gap |
| S16 | Migration index impact | identity plan/apply result | `TestIdentityMigrationReportsRebuild` | Not run; blocked by P0 | gap |
| S17 | Manifest discovery | `Service.Manifest`; CLI/MCP adapters | `TestManifestDefaults` | Not run; blocked by P1 | gap |
| S18 | Manifest discovery | request decode/validation | `TestManifestPaginationValidation` | Not run; blocked by P1 | gap |
| S19 | Manifest discovery | Manifest filters | `TestManifestFilters` | Not run; blocked by P1 | gap |
| S20 | Manifest discovery | `ManifestItem` projection | `TestManifestMetadata` | Not run; blocked by P1 | gap |
| S21 | Manifest discovery | token estimator | `TestManifestTokenEstimate` | Not run; blocked by P1 | gap |
| S22 | Manifest bounded read | 4 KiB frontmatter reader | `TestManifestDoesNotParseBody` instrumented reader | Not run; blocked by P1 | gap |
| S23 | Manifest bounded read | header error classifier | header limit/missing delimiter/invalid YAML tests | Not run; blocked by P1 | gap |
| S24 | Manifest side effects | passive index metadata observer | `TestManifestIndexObservation` with zero-call spies | Not run; blocked by P1 | gap |
| S25 | Manifest identity safety | registry validation before result | `TestManifestDuplicateIDFails` | Not run; blocked by P1 | gap |
| S26 | Manifest public parity | Service/CLI JSON/MCP shared contract | `TestMCPManifestParity` + stdio E2E | Not run; blocked by P1 | gap |
| S27 | Projection compatibility | ungrouped query path | `TestGroupingOmittedCompatibility` + existing goldens | Not run; blocked by P2 | gap |
| S28 | Chunk projection | `query.Project` chunk key | `TestProjectChunk` | Not run; blocked by P2 | gap |
| S29 | Concept projection | `query.Project` parent/stable key | `TestProjectConcept` | Not run; blocked by P2 | gap |
| S30 | Source projection | `query.Project` source key/counts | `TestProjectSource` | Not run; blocked by P2 | gap |
| S31 | Folder projection | safe path normalization/fallback | `TestProjectFolder` | Not run; blocked by P2 | gap |
| S32 | Projection determinism | `query.Project` order/limit shaping | `TestProjectDeterministic` + properties | Not run; blocked by P2 | gap |
| S33 | Projection validation | public request decoders | `TestProjectInvalidGroup` | Not run; blocked by P2 | gap |
| S34 | Projection public parity | classic CLI/Service/legacy and service MCP adapters | `TestGroupingEntryPointParity` | Not run; blocked by P2 | gap |
| S35 | Agent planning/confirmation | `agentconfig.Service`; CLI plan/apply/remove | read-only and confirmation tests | Not run; blocked by P3 | gap |
| S36 | Agent apply preservation | shared adapter merge/write | `TestAdapterIdempotenceAndPreservation` | Not run; blocked by P3 | gap |
| S37 | Canonical workflow | typed workflow renderer | `TestCanonicalWorkflowCoverage`, tool inventory validation | Not run; blocked by P3 | gap |
| S38 | Client contracts | Cursor/Claude/Codex renderers | per-client golden and syntax validators | Not run; blocked by P3 | gap |
| S39 | Agent status | ownership/status classifier | `TestAdapterStatusStates` | Not run; blocked by P3 | gap |
| S40 | Agent remove | owned key/block/header remover | `TestAdapterRemoveOwnership` | Not run; blocked by P3 | gap |
| S41 | Agent conflict safety | host parser/ownership preflight | `TestAdapterConflicts` | Not run; blocked by P3 | gap |
| S42 | Secret safety | plan/status/error redaction and generated-value guard | `TestAgentSecretRedaction` + secret scan | Not run; blocked by P3/P4 | gap |
| S43 | Agent path safety | repository path/symlink validator | `TestAgentSymlinkEscape` | Not run; blocked by P3 | gap |
| S44 | Agent rollback | per-file atomic writer + multi-file rollback | `TestAgentRollback` | Not run; blocked by P3 | gap |
| S45 | Agent client compatibility | adapter registry/version gate | `TestUnsupportedAgentClient` | Not run; blocked by P3 | gap |
| S46 | Existing retrieval quality | unchanged ungrouped eval path | committed eval + raw golden baseline gate | Not run; blocked by P4 | gap |
| S47 | Grouped retrieval utility | grouped eval metrics/report | grouped Recall/NDCG/diversity/occupancy tests | Not run; blocked by P2/P4 | gap |
| S48 | Resource bounds | Manifest/projector benchmarks | 1,000-file bytes-read and allocation evidence | Not run; blocked by P4 | gap |
| S49 | Full gauntlet | `tools/gauntlet.sh` + persisted verifier | final fresh gauntlet, E2E, mutations, secret scan | Not run; blocked by P0–P4 | gap |
| S50 | Spec conformance | this audit + conformance checker | 50-row mechanical checker | Planning matrix exists; implementation evidence absent | gap |

## Allowed alignment values

- `fully`: exact behavior is implemented and verified by the cited fresh run.
- `aligned`: behavior is met through an equivalent existing implementation, with evidence.
- `partial`: some acceptance clauses are unmet; reason and blocking task required.
- `gap`: no verified implementation; blocking task required.

## Final gates

- [x] 50 rows exist, one per Scenario, with no duplicate/missing ID in the planning matrix.
- [x] Every row names a planned public or internal wiring point.
- [x] Every row names a planned executable automated test/evidence source.
- [ ] All cited paths/symbols exist in the final implementation.
- [ ] Results come from the final source state after the last edit.
- [ ] No `partial` or `gap` remains without explicit approved downgrade.
- [ ] Retrieval metrics, Manifest bytes-read, client fixture results and mutation kills use actual numbers.
- [ ] Source commit and all commands are reproducible from the repository.
