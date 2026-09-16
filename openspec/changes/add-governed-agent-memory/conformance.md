# Conformance — add-governed-agent-memory

Spec commit: 8712bb7 (latest approved). Implementation branch: spec/governed-agent-memory.

## Scenario → implementation → test matrix (S01–S39)

| # | Scenario | Implementation entry | Automated test | Status |
|---|---|---|---|---|
| S01 | Governance read from CustomFields, normalized | `memorymeta.Governance()` | `TestGovernanceReadAndNormalize` | fully |
| S02 | Default governance=context, no path inference | `memorymeta.Governance()` | `TestGovernanceDefaultContext` | fully |
| S03 | Explicit governance takes effect | `memorymeta.Governance()` | `TestGovernanceExplicit` | fully |
| S04 | Unknown value: non-strict context+warning, strict error | `memorymeta.Governance()`, `Validate()` | `TestGovernanceUnknownNonStrict`, `TestValidateGovernanceStrict` | fully |
| S05 | Default manifest order unchanged without new params | `manifest.Build()` | `TestS05DefaultOrderUnchanged` | fully |
| S06 | Governance sort activation (for-path no filter → hold→constraint→context; filter → filtered levels only) | `manifest.Build()` | `TestS06GovernanceSortActivation`, `TestServiceManifestGovernanceFilterThroughService` | fully |
| S07 | Hold advisory warning only, no server block | `manifest.Build()` | `TestS07HoldAdvisoryWarning` | fully |
| S08 | Governance filtering | `manifest.Build()` | `TestS08GovernanceFilter` | fully |
| S09 | SetGovernance writes to CustomFields | `memorymeta.SetGovernance()` | `TestSetGovernance` | fully |
| S10 | code_refs read from CustomFields, normalized | `memorymeta.CodeRefs()` | `TestCodeRefsRead`, `TestCodeRefsEmpty`, `TestCodeRefsNilCustomFields` | fully |
| S11 | for_path exact match | `memorymeta.MatchCodeRefs()`, `manifest.Build()` | `TestMatchCodeRefsExact`, `TestS11ForPathExactMatch` | fully |
| S12 | for_path single-segment glob | `memorymeta.MatchCodeRefs()` | `TestMatchCodeRefsSingleGlob`, `TestS12ForPathSingleSegmentGlob` | fully |
| S13 | for_path recursive glob depth limit (each ** max 8) | `memorymeta.MatchCodeRefs()` | `TestMatchCodeRefsRecursiveDepth`, `TestS13ForPathDoubleStarDepth` | fully |
| S14 | for_path canonicalization, reject absolute/../NUL/backslash | `memorymeta.normalizeCodeRef()`, `MatchCodeRefs()` | `TestCodeRefsPathRejection`, `TestS14ForPathCanonicalization` | fully |
| S15 | for_path supports not-yet-created paths (lexical, no FS) | `memorymeta.MatchCodeRefs()` | `TestMatchCodeRefsNoFS`, `TestS15ForPathLexicalNoFS` | fully |
| S16 | code_refs pattern bounds (16/256 bytes/32 segments/2 **) | `memorymeta.CodeRefs()` | `TestCodeRefsBounds` | fully |
| S17 | stale-refs FS scan, fail-closed on symlink escape/unreadable | `manifest.Build()` stale-refs mode | `TestS17StaleRefsScan`, `TestServiceManifestStaleRefsSymlinkEscape`, `TestServiceManifestStaleRefsAnnotatesItems` | fully |
| S18 | stale-refs scan bound 50k entries | `manifest.Build()` (injectable `maxStaleScanEntries`) | `TestStaleRefsScanEntryLimit` (exact limit complete, over limit incomplete+warning, access ≤ limit) | fully |
| S19 | code_refs does not duplicate code_file concepts | `manifest.Build()` | `TestS19NoDuplicateCodeFileBinding` | fully |
| S20 | memory_check returns no_similar for novel content | `memorymeta.CheckMemory()`, `Service.Query()` | `TestCheckMemoryNoSimilar` | fully |
| S21 | memory_check returns possible_duplicate | `memorymeta.CheckMemory()` | `TestCheckMemoryPossibleDuplicate`, `TestCheckMemoryGolden` | fully |
| S22 | BM25 top10 candidate generation + Jaccard [0,1] threshold, durable types default, --type singular | `memorymeta.CheckMemory()` | `TestCheckMemoryJaccardTokenizer`, `TestCheckMemoryJaccardValue`, `TestCheckMemoryDefaultTypes`, `TestCheckMemoryTypeFilter` | fully |
| S23 | No conflict classification | `memorymeta.CheckMemory()` | `TestCheckMemoryGolden` (only no_similar/possible_duplicate) | fully |
| S24 | No write blocking, no allow_duplicate | `Service.Query()` memory_check branch | `TestCheckMemoryReadOnly` | fully |
| S25 | memory_check deterministic, read-only | `memorymeta.CheckMemory()` | `TestCheckMemoryDeterministic`, `TestCheckMemoryReadOnly` | fully |
| S26 | Concurrent memory_check calls safe | `memorymeta.CheckMemory()` (per-call BM25 build) | `go test -race` | fully |
| S27 | Threshold configurable, query required when memory_check=true | `memorymeta.CheckMemory()`, `Service.Query()` | `TestCheckMemoryThresholdConfigurable`, `TestCheckMemoryEmptyContent` | fully |
| S28 | Summary mode minimal fields (okf_id/title/type/governance/1-line desc≤80) | `manifest.Build()` mode projection | `TestS28SummaryModeShape` | fully |
| S29 | Hit mode includes code_refs/tags/status/stale_after | `manifest.Build()` mode projection | `TestS29HitModeShape` | fully |
| S30 | Full mode default, backward compatible | `manifest.Build()` | `TestS30FullModeBackwardCompatible` | fully |
| S31 | ID parity across modes (without max_tokens) | `manifest.Build()` | `TestS31ModeIDParity` | fully |
| S32 | max_tokens pipeline: filter→sort→offset→limit→budget; next_offset/omitted_count(budget_omitted vs total_remaining)/truncated | `manifest.Build()` | `TestS32TokenBudgetPipeline`, `TestS32BudgetOmittedVsTotalRemaining` | fully |
| S33 | budget_too_small error with dynamic min_required_tokens | `manifest.Build()` | `TestS33BudgetTooSmall` | fully |
| S34 | Token estimate = Go encoding/json default item bytes/4 ceil, no envelope | `manifest.Build()` | `TestS34TokenEstimateDefinition` | fully |
| S35 | Follow-up via okf_context refs (query OR refs at least one) | `Service.Context()` | `TestServiceContextRefsReadsConceptBody`, `TestServiceContextRequiresQueryOrRefs`, `TestServiceContextUnknownRefOmitted`, `TestMCPContextRefsHandler`, `TestToolContextRefsFlag` | fully |
| S36 | Existing concepts without new fields work unchanged | `memorymeta.Governance()`, `CodeRefs()` defaults | existing test suite + `TestCodeRefsEmpty` | fully |
| S37 | CustomFields preserved alongside new fields | Concept parser (CustomFields inline), memorymeta accessors | `TestS37CustomFieldsParseRoundTrip` (parser→CustomFields→serialize preserves governance/code_refs/my_custom), `TestS37ConceptHasNoGovernedStructFields` (reflection: no Governance/CodeRefs on Concept) | fully |
| S38 | Strict validation catches malformed new fields; mutation coverage | `memorymeta.Validate(strict=true)`, `okf lint`, `tools/mutants-governed-memory.sh` | `TestValidateGovernanceStrict`, `TestCodeRefsBounds`, 8/8 mutants killed (M-GM1..M-GM8) | fully |
| S39 | Real CLI/MCP entry points verified; Codex real agent flow | `cmd/okf/cmd_tool.go`, `pkg/mcp/tools.go`, `tools/verify-governed-memory-codex.sh` | `TestToolManifestGovernedFlags`, `TestToolContextRefsFlag`, `TestToolQueryMemoryCheckFlags`, `TestToolQueryMemoryCheckEmptyQ`, `TestToolQueryMemoryCheckTypeSingular`, `TestToolQueryMemoryCheckDupThreshold`, `TestMCPManifestGovernedSchema/Handler`, `TestMCPQueryMemoryCheckSchema/Handler`, `TestMCPQuerySharedAcrossEras`, `TestMCPContextRefsSchema/Handler`, Codex harness 4/4 PASS 0 mutating | fully |

## Coverage summary

- Total scenarios: 39
- Fully covered: 39
- Partial: 0
- Gap: 0

## No unexplained gaps

All S01–S39 map to implementation entries and automated tests. Every scenario has a real CLI or MCP entry point. No partial or gap items.

## Golden set (memory_check)

- 50 test cases (≥40 required; includes 6 identifier-form positives)
- TP=27, FP=0, TN=23, FN=0
- Precision=1.000 (gate ≥0.85) ✓
- Recall=1.000 (gate ≥0.70) ✓
- FPR=0.000 (gate ≤0.15) ✓
- Test: `TestCheckMemoryGolden` in `pkg/memorymeta/duplicate_golden_test.go`

## Modern/legacy MCP compatibility

- Modern era: 11 tools (okf_manifest, okf_query, okf_context gain new optional params)
- Legacy era: 20 tools (same params wired through shared registration)
- Both eras tested: `TestMCPManifestGovernedSchema`, `TestMCPContextRefsSchema`
