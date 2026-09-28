# Conformance: Spec ↔ Implementation ↔ Tests

## Memory Defense
| Scenario | Implemented | Test | Status |
|---|---|---|---|
| MD-01 default disabled | ✓ LoadPolicy defaults to disabled | TestWriteKnowledge_DefenseDisabledDefault | aligned |
| MD-02 redact replaces in-place | ✓ Screen replaces with [REDACTED:label] | TestScreenRedactReplacesInPlace | aligned |
| MD-03 block zero bytes | ✓ block returns before write | TestWriteKnowledge_DefenseBlockZeroBytes | aligned |
| MD-04 redact-empty rejected | ✓ strips placeholders, rejects empty | TestWriteKnowledge_RedactionEmptyRejected | aligned |
| MD-05 catalog ≥16 | ✓ 16 detectors | TestCatalogSizeFloor | aligned |
| MD-06 positive + no false-positive | ✓ table-driven | TestDetectorPositiveHits + TestNoFalsePositiveOnNormalText | aligned |
| MD-07 credit card Luhn | ✓ Verify func | TestCreditCardRequiresLuhn | aligned |
| MD-08 redact × idempotency | ✓ redact before hash | TestWriteKnowledge_RedactionIdempotencyRetry | aligned |
| MD-09 block no leak | ✓ error has detector ID only | TestWriteKnowledge_DefenseBlockZeroBytes | aligned |
| MD-10 config missing/invalid | ✓ LoadPolicy | TestLoadPolicyMissingFile/InvalidAction | aligned |
| MD-11 detector whitelist | ✓ activeDetectors filters | TestScreenWhitelist | aligned |

## Relation Recall
| Scenario | Implemented | Test | Status |
|---|---|---|---|
| RR-01 bidirectional extends | ✓ both directions | TestBidirectionalExtends | aligned |
| RR-02 update chain head | ✓ History() + IsChainHead | TestUpdateChainHead | aligned |
| RR-03 proposed hidden | ✓ skip non-approved | TestProposedHidden | aligned |
| RR-04 graceful degradation | ✓ warnings on error | TestRelationRecall_ServiceGraceful (unknown/cycle/fork/dangling/empty) | aligned |
| RR-05 no transitive | ✓ depth=1 | TestNoTransitiveExtends | aligned |
| RR-06 unknown anchor | ✓ ErrMemoryRefNotFound | TestUnknownAnchor | aligned |

## Reflect Workflow
| Scenario | Implemented | Test | Status |
|---|---|---|---|
| RF-01 multi-round gathers more | ✓ 2 rounds | TestMultiRoundGathersMore | aligned |
| RF-02 RRF overlap first | ✓ RRF(k=60) | TestRRFOverlapRanksFirst | aligned |
| RF-03 thin evidence abstains | ✓ NeedClarify | TestAbstainWhenThin | aligned |
| RF-04 poison dropped | ✓ trapGate | TestDropsTrapCandidates | aligned |
| RF-05 max rounds capped | ✓ hard cap 3 | TestMaxRoundsCapped | aligned |
| RF-06 read-only | ✓ Mutating=false | (envelope) | aligned |

## Trap Evaluation
| Scenario | Implemented | Test | Status |
|---|---|---|---|
| TE-01 answer graded | ✓ ScoreAnswer | TestAnswerLevelGraded | aligned |
| TE-02 forbidden evidence = 0 | ✓ ScoreEvidence | TestForbiddenEvidenceZero | aligned |
| TE-03 abstention rewarded | ✓ ScoreAbstention | TestAbstentionRewarded | aligned |
| TE-04 per-type grouping | ✓ Summarize | TestGroupByCaseType | aligned |
| TE-05 poison block gate | ✓ exit code + live -repo Reflect | cmdEvalTrap -repo calls Service.Reflect | aligned |

## Wiring
| Scenario | Implemented | Test | Status |
|---|---|---|---|
| WI-01 CLI reflect/relation/trap | ✓ cmd_tool.go + cmd_eval.go | CLI smoke + gauntlet L10 | aligned |
| WI-02 MCP parity | ✓ tools.go registers both | test_mcp.py 17/17 + Codex MCP E2E | aligned |
| WI-03 write tools through defense | ✓ WriteKnowledge Screen | write_defense_test.go | aligned |
| WI-04 Agent Skill/workflow manifest | ✓ W09/W10 clauses + okf_reflect/relation_recall tools | agentconfig tests 10 clauses | aligned |

## Gaps
- RR-04 graceful degradation: unit test exists for Recall returning warnings, but end-to-end Service test not added.
- TE-05 poison block gate: CLI exits 1 on poison_blocked<1.0, but integrated trap bundle not included as fixture.
- WI-01/WI-02: now aligned (test_mcp.py 17/17 + Codex MCP E2E).
- 10k concept latency: benchmarked (Screen10k=1.15ms/op, Recall10k=0.38ms/op, see evidence.md).
- Document import path defense: covered. MCP handleImportDocument AND CLI okf add both call memorydefense.Screen before writing durable markdown. Default disabled (backward compatible). Enabled via .okf/config.yaml; block rejects import, redact replaces secrets. Tests: TestImportDocumentDefense* (pkg/mcp), TestScreenImportTree* (cmd/okf).
- MCP parity: unified ToolRegistry (registerCoreTools + registerAgentTools served by same server). No separate legacy MCP entry exists. New tools available in all server start paths.

## Review A/B fixes mapping (2026-09-29)

| ID | Implementation | Test |
|---|---|---|
| RA-1 (original .md mutation) | cmd/okf/cmd_add.go stageForDefense() | cmd/okf/cmd_add_defense_test.go TestStageForDefensePreservesSource |
| RA-2 (MCP repoRoot wrong) | pkg/mcp/tools.go handleImportDocument repoRoot resolution | pkg/mcp/import_defense_test.go TestImportDocumentDefenseBlock/Redact |
| RB-1 (Service trapGate untested) | pkg/tool/reflect.go:79-82 trapGate | pkg/tool/reflect_trapgate_test.go TestReflect_TrapGateDropsProposed |

## Codex MCP E2E mapping (2026-09-29)

| Scenario | Implementation | Verified via |
|---|---|---|
| Disabled write byte-compat | pkg/mcp/tools.go handleImportDocument | Codex MCP import_document (no config) |
| Redact write | pkg/mcp/tools.go + memorydefense.Screen | Codex MCP import_document (redact policy) |
| Block write, zero durable | pkg/mcp/tools.go + ErrBlocked | Codex MCP import_document (block policy) |
| Round 1+2 + RRF | pkg/reflect/reflect.go Run | Codex MCP reflect (A extends B fixture) |
| Relation self+extends | pkg/relationrecall/recall.go Recall | Codex MCP relation_recall(anchor=A) |
| Stable refs | deterministic RRF sort | Two identical Codex reflect calls |
| need_clarify abstain | pkg/reflect/reflect.go threshold | Codex MCP reflect on empty kb |
