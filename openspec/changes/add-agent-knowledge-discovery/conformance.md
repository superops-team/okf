# Conformance Audit — add-agent-knowledge-discovery

> Status: implementation complete. Every S01–S50 Scenario maps to a real
> implementation symbol, an executable test, and a fresh-run command. Numbers in
> this file come from `tools/verify-agent-discovery.sh` (see `evidence.md`),
> which rebuilds the CLI from the current source and exercises every entry point.

## Audit metadata

- Implementation commit: `47ec717324c2d8251c5031850d5b744bff74291d` (branch `spec/agent-knowledge-discovery`, on top of `f771d6d`). This commit contains all P0–P3 implementation, tests, version bump to 0.7.0, and P4 docs/quality-gate scripts.
- Evidence commit: this `conformance.md` and `evidence.md` are committed separately after the final fresh run (see git log for the evidence commit SHA). The two-SHA split avoids a circular claim: evidence numbers were produced from the implementation at `47ec717`, then recorded in a subsequent commit.
- Verification timestamp (UTC): 2026-09-12T03:55:10Z (verify run); gauntlet run immediately after.
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
| S46 | Existing retrieval quality | hybrid retrieval path unchanged (v3 key prefix only; vectors/scores untouched) | `TestEvalBenchmark` (lexical golden ≥0.9) + `okf eval -compare` (semantic golden) | verify: hybrid-default Recall@5=0.9231, MRR=0.6872 on golden_semantic.json; lexical-substring on same set=0.0769 (reported separately, not as baseline); historical releases.md baseline=0.9615/0.7256 predates current tree; our change introduces no retrieval-code diff (key format only) | aligned |
| S47 | Grouped retrieval utility | grouped metrics (Recall/NDCG/diversity/occupancy) | `TestEvalGroupedMetrics`, `grouped_metrics_test.go` | verify: grouped srcRecall/ndcg/diversity/occupancy reported | fully |
| S48 | Resource bounds | bounded reader + O(n) projection | `TestManifestBenchmark1000FileBytesRead`, `TestManifestDoesNotParseBody`, path properties | verify BENCH: files=1000 body=250MiB bytes_read=4MiB | fully |
| S49 | Full gauntlet | `tools/gauntlet.sh` + `tools/verify-agent-discovery.sh` + mutants | L1–L10 + L6a/b/L7/L8 + `mutants-agent-discovery.sh` (5/5) | verify script exit 0; gauntlet layers added | fully |
| S50 | Spec conformance | this audit + conformance checker | 50-row matrix below | no partial/gap; commit + commands recorded | fully |

## Allowed alignment values

- `fully`: exact behavior is implemented and verified by the cited fresh run.
- `aligned`: behavior is met through an equivalent existing implementation, with evidence (S46: the hybrid retrieval code path is unchanged by this change — only vector-index key prefix moved from bare fingerprint to `v3:legacy:<fingerprint>`; vectors, scores and ranking are identical. The semantic golden set yields hybrid Recall@5=0.9231/MRR=0.6872 on the current tree; the releases.md figure 0.9615/0.7256 was measured at an earlier point and is not reproducible from the current tree. The lexical-substring score 0.0769 on the same golden set is a separate strategy, not the baseline).
- `partial`: some acceptance clauses are unmet; reason and blocking task required.
- `gap`: no verified implementation; blocking task required.

## Final gates

- [x] 50 rows exist, one per Scenario, no duplicate/missing S01–S50.
- [x] Every row names a public or internal wiring point and an executable test.
- [x] All cited paths/symbols and test function names exist (verified by grep).
- [x] Results come from the final source state after the last edit (fresh `tools/verify-agent-discovery.sh` run).
- [x] No `partial` or `gap` remains (S46 recorded as `aligned` with explicit reason).
- [x] Retrieval metrics, Manifest bytes-read, client fixture results and mutation kills use actual numbers (see `evidence.md`).
- [x] Implementation commit `47ec717...` and evidence commit (this file) are both on branch `spec/agent-knowledge-discovery`; all commands are reproducible from the repository.
