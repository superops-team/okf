# Conformance Audit — add-agent-knowledge-discovery

> Status: local implementation and evidence matrix complete; strict three-client model-E2E release acceptance is blocked by Claude Code and Cursor authentication. Every S01–S61 Scenario maps to a real
> implementation symbol, an executable test, and a fresh-run command. Numbers in
> this file come from `tools/verify-agent-discovery.sh` (see `evidence.md`),
> which rebuilds the CLI from the current source and exercises every entry point.
> Real Agent client E2E evidence comes from `tools/verify-real-agent-e2e.sh`
> (four-layer: adapter fixture / official config discovery / real model MCP calls /
> final answer & effect). Clients without model authentication are reported as
> `BLOCKED_AUTH`, never aggregated into `PASS`.

## Audit metadata

- Implementation commits (branch `spec/agent-knowledge-discovery`, on top of `f771d6d`):
  - `070decd` — P0–P3 implementation + tests (identity, manifest, projection, agentconfig, wiring)
  - `47ec717` — P4 docs, version 0.7.0, quality-gate scripts (gauntlet, verify, mutants)
  - `018996c` — S47 grouped eval hybrid strategy fix + first spec-amendment draft
  - `33c33e1` — S47 folder relevant-source recall fix (CoveredSources), chunk-level S46 gate `TestHybridBaselineGate_ChunkLevel` + baseline artifact, spec-amendment status → Proposed/pending, verify S47 gate assertion
  - `02d1312` — evidence/conformance refresh after S47 fix
  - `2c6805d` — first-round usability fixes + 64-assertion suite
  - `96811e2` — four-way audit second edition, D4 unknown-keys JSON merge, eval invalid group_by validation
  - `4a70ba3` — zero-skip usability audit (156 assertions), D5 conflict preflight (unowned whole-file artifacts), E3 derived-chunk parent_okf_id propagation
  - `3c7f58c` — S46 spec-amendment Approved by user explicit approval; conformance S46 partial→fully; tasks checklist complete
- Evidence commit: this `conformance.md` and `evidence.md` are committed separately after the final fresh run. Two-SHA split avoids circular claim: evidence numbers were produced from the implementation at `3c7f58c`, then recorded here.
- S46 spec-amendment: **Approved** by user explicit approval on 2026-09-13 (see `spec-amendment.md`). Approved revision supersedes original S46 baseline clause; base-vs-head zero-regression proof retained.
- Verification timestamp (UTC): 2026-09-13T00:17:06Z (verify-discovery run); gauntlet run immediately after (EXIT=0, coverage 69%, mutation 23/23 killed); usability 156/0/0; MCP E2E 13/13.
- Go/tool versions: `go version go1.26.7 linux/amd64`; no new third-party dependency.
- One-command entry point: `tools/verify-agent-discovery.sh` (fresh run); gate: `tools/gauntlet.sh` (L1–L10 + L6a/b/L7/L8/L9b).
- Reproduce: `go build ./... && go vet ./... && go test ./... && tools/mutants-agent-discovery.sh && tools/verify-agent-discovery.sh && tools/gauntlet.sh`.

## Scenario mapping

Alignment legend: `fully` = exact behavior implemented and verified by a fresh run; `aligned` = met through an equivalent existing implementation with evidence.

| Scenario | Requirement | Implementation path/symbol | Automated test / evidence | Fresh command / result | Align |
|---|---|---|---|---|---|
| S01 | Optional stable identity | `pkg/identity.FromConcept`; existing parse/lint/query unchanged | `TestLegacyIdentityState` + legacy regression | `go test ./pkg/identity/ ./pkg/okf/` green | fully |
| S02 | Optional stable identity | parser/serializer + `pkg/query/adapter.go` round-trip | `TestStableIDRoundTrip`, `TestConceptAdapterPreservesCustomFields` | `go test ./pkg/identity/ ./pkg/query/` green | fully |
| S03 | Optional stable identity | `identity.Parse`; registry/Manifest/migrate validation | `TestInvalidStableID` | `go test ./pkg/identity/ ./pkg/manifest/` green | fully |
| S04 | Optional stable identity | `identity.BuildRegistry` duplicate fail-closed | `TestDuplicateStableID` (kills M-AD1 mutant) | `go test ./pkg/identity/` green; mutants 5/5 | fully |
| S05 | Optional stable identity | `identity.New`/`newID` injectable entropy | `TestNewStableID`, `TestStableIDEntropyFailure`, `TestStableIDProperties` | `go test -run Property ./pkg/...` (20 property tests) | fully |
| S06 | Explicit migration | `identity.Ensure` dry-run planner | `TestIdentityEnsureDryRunDeterministic` | verify: two dry-run JSON byte-identical, missing=3 | fully |
| S07 | Explicit migration | `identity.Ensure` apply writer | `TestIdentityEnsureIdempotent` | verify: apply missing=3 → second apply missing=0 | fully |
| S08 | Explicit migration | migration preflight before first write | `TestIdentityEnsurePreflight` | `go test ./pkg/identity/` green | fully |
| S09 | Explicit migration | atomic per-file replace + rollback | `TestIdentityEnsureRollback` | `go test ./pkg/identity/` green | fully |
| S10 | Explicit migration | safe-path resolver: symlink escape rejected before any write; dry-run surfaces escapes as `Unsafe` warnings; dangling/unresolvable symlinks fail closed (preflight error, safe sibling untouched); unreadable regular files are still skipped, not migrated | `TestIdentityEnsureUnsafePath`, `TestIdentityEnsureDryRunUnsafeReported` (M2 dry-run Unsafe branch), `TestIdentityEnsureDanglingSymlinkFailsClosed` (M3 dangling branch) | `go test ./pkg/identity/` green | fully |
| S11 | Stable resolution | `Registry.Resolve`; `okf identity resolve`; MCP `okf_resolve` | `TestStableRefSurvivesMove`, CLI/MCP parity | verify: rename → resolve path=alpha-renamed.md; MCP resolve path parity | fully |
| S12 | Writer identity wiring | owned-writer final-destination helper; chunks persist `parent_okf_id` | owned-writer table tests; `TestDerivedChunkHasParentID` | `go test ./pkg/git/ ./pkg/convert/ ./pkg/tool/` green | fully |
| S13 | Query metadata adapter | `pkg/query/adapter.go` canonical conversion | `TestConceptAdapterPreservesCustomFields`, no-alias test | `go test ./pkg/query/ -run TestConceptAdapter` green | fully |
| S14 | Index identity | `identity.Key`/`ChunkKey` v3 key builder | `TestIdentityAwareIndexKeys` | `go test ./pkg/vectorindex/ -run Identity` green | fully |
| S15 | Index compatibility | vector metadata load/status strict v2 rejection | `TestVectorV2RequiresRebuild` | `go test ./pkg/vectorindex/` green; README v3 note | fully |
| S16 | Migration index impact | `EnsureReport.VectorRebuildRequired` | `TestIdentityMigrationReportsRebuild` | `go test ./pkg/identity/` green | fully |
| S17 | Manifest discovery | `tool.Service.Manifest`; CLI/MCP | `TestManifestDefaults`, `TestToolManifestJSON` | verify: manifest total=3, default limit=100 | fully |
| S18 | Manifest discovery | request decode/validation (limit presence) | `TestManifestPaginationValidation` | verify: limit=999 → `invalid_request` | fully |
| S19 | Manifest discovery | Manifest filters OR/AND semantics | `TestManifestFilters` | `go test ./pkg/manifest/ -run Filter` green | fully |
| S20 | Manifest discovery | `ManifestItem` bounded metadata projection | `TestManifestMetadata` | verify: no body leak; estimate_kind=file_bytes_div4 | fully |
| S21 | Manifest discovery | token estimator `ceil(bytes/4)` | `TestManifestTokenEstimate` | verify: estimated_tokens present on all 3 items | fully |
| S22 | Manifest bounded read | 4 KiB bounded frontmatter reader | `TestManifestDoesNotParseBody` (kills M-AD2 mutant) | verify BENCH: 250 MiB bodies → 4 MiB read | fully |
| S23 | Manifest bounded read | header error classifier | `TestManifestHeaderLimit`, `TestManifestMissingDelimiter`, `TestManifestInvalidYAML` | `go test ./pkg/manifest/` green | fully |
| S24 | Manifest side effects | passive index metadata observer, zero build spies | `TestManifestIndexObservation` | `go test ./pkg/manifest/` green | fully |
| S25 | Manifest identity safety | registry validation before result | `TestManifestDuplicateIDFails` | `go test ./pkg/manifest/` green | fully |
| S26 | Manifest public parity | Service/CLI JSON/MCP shared contract | `TestMCPManifestParity`, `TestToolManifestJSON` | verify: MCP okf_manifest total=3 == CLI total=3 | fully |
| S27 | Projection compatibility | ungrouped query path unchanged | `TestGroupingOmittedCompatibility` + text goldens | verify: ungrouped search still runs; `-group-by` omitted unchanged | fully |
| S28 | Chunk projection | `Project` chunk key/range | `TestProjectChunk` | `go test ./pkg/query/ -run TestProjectChunk` green | fully |
| S29 | Concept projection | parent-id fallback to legacy key | `TestProjectConcept` | `go test ./pkg/query/ -run TestProjectConcept` green | fully |
| S30 | Source projection | source key + unique counts | `TestProjectSource` | verify: group-by source groups=2, one slot/source | fully |
| S31 | Folder projection | safe path normalize/fallback | `TestProjectFolder` (kills M-AD3 mutant) | `go test ./pkg/query/ -run TestProjectFolder` green | fully |
| S32 | Projection determinism | order/limit/representative shaping | `TestProjectDeterministic`, `TestProjectIdempotenceProperty` | `go test -run Property ./pkg/query/` green; M-AD4 killed | fully |
| S33 | Projection validation | public request decoders reject unknown groups | `TestProjectInvalidGroup` | verify: group-by bogus → `invalid_group_by` | fully |
| S34 | Projection public parity | CLI/Service/`okf_query` shared projector (legacy text tools `okf_semantic_search`/`okf_search` out of scope; grouping is via `okf_query`) | `TestGroupingEntryPointParity`, `grouping_cli_test.go` | `go test ./cmd/okf/ ./pkg/query/` green | fully |
| S35 | Agent planning | `agentconfig.Service.Plan` read-only; confirmation gate | `TestAgentPlanReadOnly`, `TestAdapterIdempotenceAndPreservation` | verify: plan writes nothing; apply without --yes refused in tests | fully |
| S36 | Agent apply preservation | shared adapter merge; unknown-key preservation | `TestAdapterIdempotenceAndPreservation` | verify: apply → apply(second) zero diff for 3 clients | fully |
| S37 | Canonical workflow | typed workflow renderer | `TestCanonicalWorkflowCoverage`, tool-inventory test | `go test ./pkg/agentconfig/ -run Workflow` green | fully |
| S38 | Client contracts | cursor/claude/codex renderers + validators | per-client golden tests in `conformance_test.go` | `go test ./pkg/agentconfig/` green | fully |
| S39 | Agent status | ownership/status classifier | `TestAdapterStatusStates` | verify: status reports installed for 3 clients | fully |
| S40 | Agent remove | owned key/block/header remover, no force | `TestAdapterRemoveOwnership` | verify: remove for 3 clients clean | fully |
| S41 | Agent conflict safety | host parser/ownership preflight | `TestAdapterConflicts` (kills M-AD5 mutant) | `go test ./pkg/agentconfig/ -run TestAdapterConflicts` green | fully |
| S42 | Secret safety | redaction + no credential field in generated config (secret scan coverage is provided by the OKF-owned-entry whole-path write coverage plus output hashing via `hashShort`, so no literal secret material reaches reports) | `TestAgentSecretRedaction` + gauntlet L7 secret scan | verify: no credential-like field in reports; L7 scan clean | fully |
| S43 | Agent path safety | repo-boundary/symlink validator | `TestAgentSymlinkEscape` | `go test ./pkg/agentconfig/ -run Symlink` green | fully |
| S44 | Agent rollback | per-file atomic writer + multi-file rollback | `TestAgentRollback` | `go test ./pkg/agentconfig/` green | fully |
| S45 | Agent client compatibility | adapter registry/version gate | `TestUnsupportedAgentClient` | verify: `--client nosuch` rejected | fully |
| S46 | Existing retrieval quality | hybrid retrieval path unchanged (v3 key prefix only; vectors/scores untouched). Base-vs-head worktree comparison confirms zero code regression. S46 spec-amendment **Approved** by user explicit approval 2026-09-13; approved revision supersedes original baseline clause. | `TestHybridBaselineGate_ChunkLevel` (exact CLI pipeline: chunking→embedding→v3 chunk key→HNSW→BM25+semantic hybrid; asserts Recall@5≥0.90, MRR≥0.60; baseline artifact `testdata/hybrid_baseline_chunk_level.json`) + `okf eval -compare` | verify: hybrid-default Recall@5=0.9231, MRR=0.6615 (chunk-level); base (aaafcbb) with same content = identical; negative controls (broken semantic channel / wrong weights) fail gate. Historical 0.9615/0.7256 not reproducible from current tree (documented in approved amendment). | fully |
| S47 | Grouped retrieval utility | grouped eval uses hybrid raw-candidate strategy; `RelevantSourceRecallAtK` checks ALL group members' covered sources (fixed: was representative-only) | `TestEvalGroupedMetrics`, `TestRelevantSourceRecallAtK_MultiSourceFolder`, `grouped_metrics_test.go` | verify: concept srcRecall=0.9231; source srcRecall=0.9231; folder srcRecall=0.9231 (fixed from 0.4038). All three ≥ raw hybrid Recall@5=0.9231. | fully |
| S48 | Resource bounds | bounded reader + O(n) projection | `TestManifestBenchmark1000FileBytesRead`, `TestManifestDoesNotParseBody`, path properties | verify BENCH: files=1000 body=250MiB bytes_read=4MiB | fully |
| S49 | Full gauntlet | `tools/gauntlet.sh` + `tools/verify-agent-discovery.sh` + mutants | L1–L10 + L6a/b/L7/L8 + `mutants-agent-discovery.sh` (5/5) | verify script exit 0; gauntlet layers added | fully |
| S50 | Spec conformance | this audit + conformance checker | 61-row matrix below | no partial/gap for S01–S50; S51–S55 Codex fully, Claude/Cursor model closure partial (BLOCKED_AUTH); S57–S61 fully; commit + commands recorded | fully |
| S51 | Official clients consume generated project config | `pkg/agentconfig` adapters generate `.cursor/mcp.json`, `.mcp.json`, `.codex/config.toml`; official client CLIs inspect project MCP config | `tools/verify-real-agent-e2e.sh` layer `official_config_discovery` | fresh: Codex PASS (codex-cli 0.153.4 discovers okf), Claude Code PASS (official CLI config discovery), Cursor PASS (official CLI config discovery); absent client = BLOCKED_CLIENT_MISSING | fully |
| S52 | Real Agent produces source-grounded answer through OKF MCP | `okf mcp --repo .` stdio server; official Agent client calls okf_status→okf_manifest→okf_query→okf_context in order; canary fact in final answer | `tools/verify-real-agent-e2e.sh` layer `model_read_answer` (Codex); JSONL event stream machine-validated | fresh: Codex PASS (real model called all 4 tools in order, no shell/file/CLI, canary in final answer); Claude Code BLOCKED_AUTH (no model credential); Cursor BLOCKED_AUTH | partial |
| S53 | Agent acceptance cannot bypass OKF MCP | event-stream audit: no shell, terminal, direct file-read, or direct OKF CLI execution; `tools/mcp_call.py`/`test_mcp.py` results excluded from Agent-client evidence | `tools/verify-real-agent-e2e.sh` JSONL audit (Codex) | fresh: Codex PASS (zero bypass events in JSONL); Claude/Cursor not run (BLOCKED_AUTH) | partial |
| S54 | Real Agent recovers from structured OKF error | Agent resolves invalid ref → observes `invalid_concept_id` + remediation → calls okf_manifest → retries okf_resolve → final answer reports resolved path | `tools/verify-real-agent-e2e.sh` layer `model_error_recovery` (Codex) | fresh: Codex PASS (real model consumed structured error, retried, resolved path in answer); Claude/Cursor BLOCKED_AUTH | partial |
| S55 | Real Agent performs controlled durable capture | Agent calls okf_note with idempotency key → okf_query reads back → note persisted under knowledge dir → final answer contains durable content | `tools/verify-real-agent-e2e.sh` layer `model_controlled_write` (Codex) | fresh: Codex PASS (note persisted, query readback, content in answer); Claude/Cursor BLOCKED_AUTH | partial |
| S56 | Client capability status is fail-closed | `tools/verify-real-agent-e2e.sh` records per-client per-layer status and executes validator negative controls; BLOCKED_* never aggregated into PASS; config discovery reported separately from model/tool/answer closure | `tools/verify-real-agent-e2e.sh` results.tsv + summary.json + `real-agent-evidence.md` | fresh: 8 PASS / 0 FAIL / 2 BLOCKED_AUTH; counts include parser gate, three config-discovery gates, validator negative-controls, and three Codex model closures; strict mode remains blocked by Claude/Cursor auth | fully |
| S57 | Code metadata combination filter returns results | `pkg/tool/service.go` `filteredConceptsForQuery` delegates to `querypkg.Query.Execute` (content-aware, case-insensitive, substring over body/regex index); `matchesQueryFilters` post-filters only Type/Types/Project/Tag; exact custom-field comparisons for file path/language/symbol/qualified-name/relation endpoints removed | `TestFilteredConceptsCodeMetadataCombination`, `TestFilteredConceptsNoFalsePositive` | `go test ./pkg/tool/ -run 'TestFilteredConceptsCodeMetadataCombination|TestFilteredConceptsNoFalsePositive'` green | fully |
| S58 | Context returns token-bounded source snippet for matched symbol | `pkg/tool/service.go` `rankConcepts` falls back to `symbolLocationFromContent` + `parseSymbolLocation` when `start_line` custom field is 0 (exact name > substring > first hit); `Service.Context` extracts the parsed range and hardcodes `Provenance="repo.source"` | `TestRankConceptsSymbolLocationFromContent`, `TestContextExtractsSymbolBody` | `go test ./pkg/tool/ -run 'TestRankConceptsSymbolLocationFromContent|TestContextExtractsSymbolBody'` green; snippet spans multiple lines and excludes unrelated symbols | fully |
| S59 | Status reports code concept count | `pkg/tool/service.go` `StatusResult.CodeConceptCount` (`json:"code_concept_count,omitempty"`) from `stats.TypeCounts["code_file"]`; omitted when zero | `TestStatusReportsCodeConceptCount`, `TestStatusOmitsCodeConceptCountWhenNone` | `go test ./pkg/tool/ -run 'TestStatusReportsCodeConceptCount|TestStatusOmitsCodeConceptCountWhenNone'` green | fully |
| S60 | Code metadata filtering consistent across Service, CLI and MCP | all three entry points route through the same `Service.Query`/`filteredConceptsForQuery` path; unified substring semantics locked by renamed `...UseSubstringSemantics` tests | `TestQueryStructuredFiltersUseSubstringSemantics`, `TestQueryRelationSourceAndTargetFiltersUseSubstringSemantics`, `TestFilteredConceptsCodeMetadataCombination` | `go test ./pkg/tool/ -run 'SubstringSemantics'` green; Service/CLI/MCP share one filter implementation | fully |
| S61 | Non-code concept filtering is unchanged | `matchesQueryFilters` still applies Type/Types/Project/Tag; empty filters pass through unchanged | `TestFilteredConceptsBackwardCompat` | `go test ./pkg/tool/ -run TestFilteredConceptsBackwardCompat` green | fully |

## Allowed alignment values

- `fully`: exact behavior is implemented and verified by the cited fresh run.
- `aligned`: behavior is met through an equivalent existing implementation, with evidence.
- `partial`: some acceptance clauses are unmet; reason and blocking task required.
- `gap`: no verified implementation; blocking task required.

## Final gates

- [x] 61 rows exist, one per Scenario, no duplicate/missing S01–S61.
- [x] Every row names a public or internal wiring point and an executable test.
- [x] All cited paths/symbols and test function names exist (verified by grep).
- [x] Results come from the final source state after the last edit (fresh `tools/verify-agent-discovery.sh` run + fresh `tools/verify-real-agent-e2e.sh` run).
- [x] S01–S50 no `gap` or `partial`. S46 is `fully` — spec-amendment.md was **Approved** by user explicit approval on 2026-09-13; the approved revision supersedes the original S46 baseline clause. Base-vs-head zero-regression proof and chunk-level machine gate retained. S47 is `fully` (code bug fixed per original spec).
- [x] S51–S55: Codex `fully` (real model E2E passed: read-answer, error-recovery, controlled-write, no-bypass). Claude Code and Cursor model closure are `partial` / `BLOCKED_AUTH` (official config discovery PASS; no model credential in this environment; fail-closed per S56). Not aggregated into PASS.
- [x] S56 `fully`: capability status fail-closed; fresh evidence reports 8 PASS / 0 FAIL / 2 BLOCKED_AUTH and strict mode remains non-zero while blocked.
- [x] S57–S61 `fully`: unified code-metadata filtering (`filteredConceptsForQuery` → `querypkg.Query.Execute`, post-filter limited to Type/Types/Project/Tag), body-derived symbol range (`symbolLocationFromContent`/`parseSymbolLocation`), token-bounded Context snippet with `repo.source` provenance, additive `code_concept_count`, and unchanged non-code filtering; locked by `pkg/tool/service_code_query_test.go` and the renamed `...UseSubstringSemantics` tests.
- [x] Retrieval metrics, Manifest bytes-read, client fixture results and mutation kills use actual numbers (see `evidence.md`).
- [x] Real Agent E2E uses official client JSONL event streams, not `tools/mcp_call.py` or `test_mcp.py` (those are MCP protocol layer only, S53).
- [x] Implementation commits `070decd`/`47ec717`/`018996c`/`33c33e1`/`02d1312`/`2c6805d`/`96811e2`/`4a70ba3`/`3c7f58c` and evidence commit (this file) are all on branch `spec/agent-knowledge-discovery`; all commands are reproducible from the repository.
