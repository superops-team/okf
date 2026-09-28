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
| RR-04 graceful degradation | ✓ warnings on error | (integration: Service.RelationRecall) | partial |
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
| TE-05 poison block gate | ✓ exit code | (CLI: cmdEvalTrap) | partial |

## Wiring
| Scenario | Implemented | Test | Status |
|---|---|---|---|
| WI-01 CLI reflect/relation/trap | ✓ cmd_tool.go + cmd_eval.go | build verified | partial |
| WI-02 MCP parity | ✓ tools.go registers both | build verified | partial |
| WI-03 write tools through defense | ✓ WriteKnowledge Screen | write_defense_test.go | aligned |

## Gaps
- RR-04 graceful degradation: unit test exists for Recall returning warnings, but end-to-end Service test not added.
- TE-05 poison block gate: CLI exits 1 on poison_blocked<1.0, but integrated trap bundle not included as fixture.
- WI-01/WI-02: CLI/MCP build verified but E2E python test not extended.
- 10k concept latency: not benchmarked (requires large fixture).
- Document import path defense: covered. MCP handleImportDocument AND CLI okf add both call memorydefense.Screen before writing durable markdown. Default disabled (backward compatible). Enabled via .okf/config.yaml; block rejects import, redact replaces secrets. Tests: TestImportDocumentDefense* (pkg/mcp), TestScreenImportTree* (cmd/okf).
- MCP parity: unified ToolRegistry (registerCoreTools + registerAgentTools served by same server). No separate legacy MCP entry exists. New tools available in all server start paths.
