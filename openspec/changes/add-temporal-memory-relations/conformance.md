# Conformance: Temporal Memory Relations and Review

One row per spec scenario S01–S44. "Status" is `fully` or `partial`.
Every automated-test column cites a real Go test function (or, for S41–S44, the
real benchmark / shell script). "Implementation entry" cites `file:symbol` as
shipped. "Real entry" is the user/agent-facing CLI subcommand, MCP tool, or
read path.

Legend for status: **fully** = behavior shipped and locked by an automated test
or the required harness. (No partial scenarios remain.)

| Scenario | Status | Implementation entry (file:symbol) | Automated test | Real entry (CLI subcommand / MCP tool) |
|---|---|---|---|---|
| S01 Missing state defaults to approved | fully | `pkg/memorymeta/temporal.go: State` | `TestS01MissingStateDefaultsApproved` (`pkg/memorymeta/temporal_test.go`) | `memorymeta.State`; default query/write path |
| S02 Explicit states normalized | fully | `pkg/memorymeta/temporal.go: State` / `SetState` | `TestS02StateNormalization` (`pkg/memorymeta/temporal_test.go`) | `memorymeta.State` |
| S03 Proposed confidence + evidence contract | fully | `pkg/memorymeta/temporal.go: ValidateTemporal` / `Confidence` / `hasEvidenceRef` | `TestS03ProposedConfidenceAndEvidence` (`pkg/memorymeta/temporal_test.go`) | `ValidateTemporal`; `Service.WriteKnowledge` preflight |
| S04 Relation namespaced & typed | fully | `pkg/memorymeta/temporal.go: Relation` / `SetRelation` / `normalizeTarget` | `TestS04RelationNamespacingAndNormalization` (`pkg/memorymeta/temporal_test.go`) | `memorymeta.Relation` (code `relation_*` untouched) |
| S05 Relation bounds & Stable-ID validation | fully | `pkg/memorymeta/temporal.go: ValidateTemporal` / `validateRelationShape` | `TestS05RelationBoundsAndIDs` (`pkg/memorymeta/temporal_test.go`) | `ValidateTemporal`; `okf lint --strict` |
| S06 Malformed conservative + strict | **fully** | `pkg/memorymeta/temporal.go: ValidateTemporal(strict)`; `pkg/tool/temporal_query.go: ValidateTemporalBundle`; `pkg/lint/temporal_lint.go: temporalIssues` wired into `LintBundle` | `TestS06MalformedConservativeAndStrict` (`pkg/memorymeta/temporal_test.go`); `TestTemporalT33StrictLint` (`pkg/tool/context_temporal_test.go`); `TestTemporalMalformedStateIsReported` / `TestTemporalMalformedRelationIsReported` / `TestTemporalDanglingUnresolvableTarget` / `TestTemporalCrossProject` / `TestTemporalAmbiguousFork` / `TestTemporalCycle` / `TestTemporalHistoryTooDeep` / `TestTemporalNonStrictIsWarning` / `TestTemporalStrictIsError` (`pkg/lint/temporal_lint_test.go`) | `okf lint` (warning) / `okf lint --strict` (error) |
| S07 Temporal CustomFields round-trip lossless | fully | `pkg/memorymeta/temporal.go` typed accessors; existing parser (no production change) | `TestS07TemporalRoundTrip` + `TestS07NoNamedTemporalStructFields` (`pkg/memorymeta/temporal_roundtrip_test.go`) | parser/serializer round-trip; reflection over `Concept` |
| S08 Legacy & extends stay current | fully | `pkg/memorymeta/temporal_view.go: BuildTemporalView` | `TestS08LegacyAndExtendsCurrent` (`pkg/memorymeta/temporal_view_test.go`) | `BuildTemporalView` |
| S09 Linear chain selects one head | fully | `pkg/memorymeta/temporal_view.go: BuildTemporalView` | `TestS09LinearChainSelectsHead` (`pkg/memorymeta/temporal_view_test.go`) | `BuildTemporalView` |
| S10 Proposed/declined relations inactive | fully | `pkg/memorymeta/temporal_view.go: BuildTemporalView` | `TestS10ProposedDeclinedInactive` (`pkg/memorymeta/temporal_view_test.go`) | `BuildTemporalView` |
| S11 Update must target current head | fully | `pkg/tool/temporal_preflight.go: preflightApprovedRelation` / `mapInvalidReason`; `pkg/tool/temporal_lock.go` | stale-head block inside `TestTemporalS22PreflightRollbackZeroWrite` (`pkg/tool/write_temporal_test.go`) | `Service.WriteKnowledge` (approved write preflight) |
| S12 Multiple update heads isolate invalid component | fully | `pkg/memorymeta/temporal_view.go: BuildTemporalView` (`ambiguous_update_head`) | `TestS12AmbiguousFork` (`pkg/memorymeta/temporal_view_test.go`) | `BuildTemporalView`; `okf lint --strict` |
| S13 Update cycles isolate invalid component | fully | `pkg/memorymeta/temporal_view.go: BuildTemporalView` (color DFS cycle) | `TestS13Cycle` (`pkg/memorymeta/temporal_view_test.go`) | `BuildTemporalView`; `okf lint --strict` |
| S14 Dangling / non-durable / cross-project fail | fully | `pkg/memorymeta/temporal_view.go: BuildTemporalView` / `effectiveProject`; `pkg/tool/temporal_preflight.go: conceptProject` | `TestS14DanglingAndNonDurable` + `TestS14CrossProject` (`pkg/memorymeta/temporal_view_test.go`) | projection + relation preflight |
| S15 History resolves from any chain member | fully | `pkg/memorymeta/temporal_view.go: TemporalView.History`; `pkg/tool/temporal_query.go: buildMemoryHistory` | `TestS15HistoryFromAnyMember` + `TestHistoryStandaloneAndMissing` (`pkg/memorymeta/temporal_view_test.go`) | `Service.Query` history mode |
| S16 History traversal bounded (≤128) | fully | `pkg/memorymeta/temporal_view.go: TemporalView.History` (`temporal_history_too_deep`) | `TestS16HistoryDepthBound` (`pkg/memorymeta/temporal_view_test.go`) | `Service.Query` history mode |
| S17 Legacy default parity + safe temporal default | fully | `pkg/tool/temporal_query.go: filterConceptsForView`; `pkg/tool/service.go` Query/Context | `TestS17LegacyRepoAllCurrent` (`temporal_view_test.go`); `TestTemporalS17LegacyQueryParity` (`query_temporal_test.go`); `TestTemporalS17LegacyContextParity` (`context_temporal_test.go`) | `okf tool query` / `okf tool context` (no `--memory-view`) |
| S18 Legacy durable write stays approved | fully | `pkg/tool/write.go: WriteKnowledgeRequest`; `Service.WriteKnowledge` | `TestTemporalS18LegacyWriteByteSemanticParity` (`pkg/tool/write_temporal_test.go`) | `okf_note` / `okf_log` / `okf_feedback` |
| S19 Approved update write activates immediately | fully | `pkg/tool/temporal_preflight.go: preflightApprovedRelation`; `pkg/tool/temporal_lock.go` | `TestTemporalS19ApprovedUpdateActivatesImmediately` (`pkg/tool/write_temporal_test.go`) | `Service.WriteKnowledge` |
| S20 Proposed relation quarantined + review handle | fully | `pkg/tool/write.go: MemoryState`/`MemoryConfidence`; `pkg/tool/temporal_preflight.go` | `TestTemporalS20ProposedQuarantine` (`pkg/tool/write_temporal_test.go`) | `okf_note`/`okf_log`/`okf_feedback` proposed write |
| S21 Temporal fields enter idempotency hash | fully | `pkg/tool/write.go: writeKnowledgePayload` / `payload_hash` | `TestTemporalS21IdempotencyHashAndRetryAfterReview` (`pkg/tool/write_temporal_test.go`) | `Service.WriteKnowledge` idempotency |
| S22 Relation preflight + atomic rollback (zero writes) | fully | `pkg/tool/temporal_preflight.go`; `pkg/tool/write.go` atomic write/verify/rollback | `TestTemporalS22PreflightRollbackZeroWrite` (`pkg/tool/write_temporal_test.go`) | `Service.WriteKnowledge` |
| S23 Concurrent approved updates serialize | fully | `pkg/tool/temporal_lock.go: repositoryTemporalLock` | `TestTemporalS23ConcurrentApprovedUpdatesSerialize` (`pkg/tool/write_temporal_test.go`, `-race`) | `Service.WriteKnowledge` |
| S24 Deterministic lock order (no deadlock) | fully | `pkg/tool/temporal_lock.go: repositoryTemporalLock` (repo → per-path) | `TestTemporalS24LockOrderNoDeadlock` (`pkg/tool/write_temporal_test.go`) | write stress (temporal + legacy mix) |
| S25 Approve proposed memory | fully | `pkg/tool/memory_review.go: Service.ReviewMemory` / `applyReviewTransition` | `TestTemporalS25Approve` (`pkg/tool/memory_review_test.go`) | `okf_memory_review`; `okf tool memory-review --action approve` |
| S26 Decline proposed memory | fully | `pkg/tool/memory_review.go: ReviewMemory` | `TestTemporalS26Decline` (`pkg/tool/memory_review_test.go`) | `okf_memory_review`; `--action decline` |
| S27 Undo + confidence restore + updater guard | fully | `pkg/tool/memory_review.go: applyReviewTransition` / `findApprovedUpdater`; `pkg/memorymeta/temporal.go: ReviewRecord` | `TestTemporalS27UndoConfidenceAndHasApprovedUpdater` (`pkg/tool/memory_review_test.go`) | `okf_memory_review`; `--action undo` |
| S28 Invalid transition rejected | fully | `pkg/tool/memory_review.go: validateReviewTransition` | `TestTemporalS28InvalidTransitionsAndDurability` (`pkg/tool/memory_review_test.go`) | `okf_memory_review` |
| S29 Stale reviewer loses CAS race | fully | `pkg/tool/memory_review.go: ReviewMemory` (`expected_state` compare, `memory_state_conflict`) | `TestTemporalS29CASRace` (`pkg/tool/memory_review_test.go`, `-race`) | `okf_memory_review` CAS |
| S30 Approval revalidates target freshness | fully | `pkg/tool/memory_review.go: ReviewMemory` (fresh preflight on approve) | `TestTemporalS30ApprovedThenTargetNotCurrent` (`pkg/tool/memory_review_test.go`) | `okf_memory_review --action approve` |
| S31 Review persistence atomic + bounded | fully | `pkg/tool/memory_review.go: restoreOriginalBytes` / `setReviewRecord` | `TestTemporalS31PersistenceFailureRestoresOriginalBytes` (`pkg/tool/memory_review_test.go`) | review same-file atomic rewrite |
| S32 Current query filters before ranking | fully | `pkg/tool/temporal_query.go: filterConceptsForView` / `annotateQueryHit` | `TestTemporalS32CurrentPreRankFilter` (`pkg/tool/query_temporal_test.go`) | `Service.Query` (`current`/`all`) |
| S33 History query uses exactly one ref | fully | `pkg/tool/temporal_query.go: classifyTemporalQuery` / `buildMemoryHistory` | `TestTemporalS33History` (`query_temporal_test.go`); `TestToolQueryHistoryAllowsEmptyQueryWithRefs` + `TestToolQueryHistoryRequiresRefs` (`cmd/okf/cmd_tool_temporal_test.go`) | `okf tool query --memory-view history --refs` |
| S34 Review queue body-free + deterministic | fully | `pkg/tool/temporal_query.go: buildMemoryReviewQueue` | `TestTemporalS34ReviewQueue` (`query_temporal_test.go`); `TestToolQueryMemoryReviewQueueAllowsEmptyQuery` (`cmd_tool_temporal_test.go`) | `okf tool query --memory-review-queue` |
| S35 Query modes mutually exclusive | fully | `pkg/tool/temporal_query.go: classifyTemporalQuery` | `TestTemporalS35MutualExclusion` (`pkg/tool/query_temporal_test.go`) | `Service.Query` validation |
| S36 Context current/all + explicit refs | fully | `pkg/tool/temporal_query.go: annotateContextItem`; `pkg/tool/service.go` Context | `TestTemporalS36ContextViews` (`pkg/tool/context_temporal_test.go`) | `okf tool context --memory-view` |
| S37 CLI entry points & naming real | fully | `cmd/okf/cmd_tool.go` (query/context flags + `memory-review` dispatcher) | `TestToolQueryMemoryViewAllFlag`, `TestToolQueryDefaultStillRequiresQuery`, `TestToolContextMemoryViewFlag`, `TestToolMemoryReviewApproveEnvelope`, `TestToolMemoryReviewMissingFlags`, `TestToolHelpDocumentsTemporalModes` (`cmd/okf/cmd_tool_temporal_test.go`) | `okf tool query` / `context` / `memory-review` |
| S38 MCP schemas match across eras (12 / 21) | fully | `pkg/mcp/tools.go` (extended schemas + `okf_memory_review`) | `TestMCPTemporalSchemaSharedAcrossEras`, `TestMCPMemoryReviewToolPresentBothEras`, `TestMCPTemporalWriteFieldsAccepted`, `TestMCPTemporalUnknownFieldRejected`, `TestMCPTemporalWriteRequestMapping`, `TestMCPTemporalQueryRequestMapping` (`pkg/mcp/tools_temporal_test.go`) | modern+legacy MCP |
| S39 Agent workflow requires review consent (W08) | fully | `pkg/agentconfig/workflow.go` clause `W08` ("Propose, never self-approve") | `TestDocumentationContracts` + `TestDocumentationClausesOnceInAgentSkill` (`documentation_contract_test.go`); `TestRenderAgentSkillWorkflowOnce` (`skill_test.go`) | portable Agent Skill |
| S40 Temporal golden topology (property) | fully | `pkg/memorymeta/temporal_view.go: BuildTemporalView` | `TestS40PermutationIndependence` (`pkg/memorymeta/temporal_view_test.go`) | golden topology fixture |
| S41 10k benchmarks, no hidden cache/index | fully | `pkg/memorymeta/temporal_benchmark_test.go`; `pkg/tool/memory_review_benchmark_test.go` | `BenchmarkTemporalBuildView10k`, `BenchmarkTemporalHistoryChain3`; `BenchmarkMemoryReviewQueueOrdering` | `go test -bench=. -benchmem -benchtime=20x` |
| S42 Targeted mutants all killed | fully | `tools/mutants-temporal-memory.sh` (T-M1…T-M7); wired as gauntlet `L9d` | shell runner: `tools/mutants-temporal-memory.sh` | mutation runner (byte-restore after each) |
| S43 Real Codex authorized flow | fully | `tools/verify-temporal-memory-codex.sh` | E2E script: `tools/verify-temporal-memory-codex.sh` | real Codex client + `okf_memory_review` |
| S44 Full regression & conformance | fully | `tools/gauntlet.sh` (incl. `L9d`); this matrix | `tools/gauntlet.sh` (race/shuffle, modern+legacy MCP E2E, parser round-trip, default parity, secret scan) | GAUNTLET + conformance |

## S06 / T3.3 strict-lint wiring (now complete)

Whole-bundle strict temporal validation is wired end-to-end into the
`pkg/lint` `Issue` pipeline. `pkg/lint/temporal_lint.go` runs a self-contained
graph pass inside `LintBundle` reading additive `Temporal` fields on
`lint.Concept` (populated at both conversion sites — `cmd/okf/main.go
lintBundleWithConfig` and `pkg/mcp/tools.go toLintConcepts` — via the
`memorymeta.State/Relation/Confidence` accessors). It reports malformed
state/confidence/relation, dangling targets, cross-project edges, ambiguous
forks, cycles, and chains deeper than 128 nodes. In non-strict mode these are
**warnings** (bundle still usable); under `--strict` they are **errors**
(`HasErrors=true`). Legacy/no-temporal bundles produce zero temporal issues
(byte/semantic parity preserved). Codes: `invalid_memory_state`,
`invalid_memory_relation`, `memory_relation_dangling_target`,
`memory_relation_cross_project`, `ambiguous_update_head`,
`memory_relation_cycle`, `temporal_history_too_deep`.

## Summary

- Fully conformant: **44** of 44 scenarios.
- Partial: **0**.
- No unexplained gaps: every scenario maps to a named implementation symbol,
  a named automated test, and a real user/agent entry point.
