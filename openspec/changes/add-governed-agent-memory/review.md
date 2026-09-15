# Review — add-governed-agent-memory

## Review round 1: initial Spec review (15 findings)

All fixed in ce11157. See git history for details. Key: CustomFields decision, advisory-only, no second index, default order unchanged.

## Review round 2: implicit issues (8 findings)

All fixed in ce11157. Key: hold advisory, stale-refs fail-closed, ID parity, budget_too_small.

## Independent acceptance review round 3 (22 findings, fixed in ce11157)

All 22 findings documented and fixed. Key: ghost commands, pilot confusion matrix, token measurement,逐行 matrix.

## Independent acceptance review round 4 (9 categories, all fixed in this commit)

### A. Ghost interfaces (5 findings)

| # | Finding | Fix |
|---|---|---|
| A1 | `okf tool query --content` doesn't exist | Changed to `okf tool query -q <content> --memory-check`; MCP uses `query` field + `memory_check` bool. memory_check=true requires non-empty query. |
| A2 | `okf tool note` doesn't exist | Changed to MCP `okf_note`/Service WriteKnowledge; explicitly stated no CLI for durable writes. |
| A3 | `okf tool resolve --ref`/`okf tool query --ref` don't exist | Real is `okf identity resolve --ref` (returns path only). Added `refs` param to existing `okf tool context` (query OR refs at least one). S35 scenario. |
| A4 | `okf tool lint --strict` doesn't exist | Changed to `okf lint --strict` (existing CLI). S38. |
| A5 | CLI bool written as `--memory-check true` | Changed to `--memory-check` (CLI bool); MCP JSON uses `true`. |

### B. memory_check contract (3 findings)

| # | Finding | Fix |
|---|---|---|
| B1 | QueryRequest used non-existent content/title | Uses existing Query/Types/Project/Tag. Candidate source defaults to durable types (note,event,feedback); --types can extend. |
| B2 | "reuse existing BM25 index" unverified (Service has no cache) | BM25 built per-call on bounded durable candidate set (max 1000). No "verify cache behavior" ambiguity. |
| B3 | Tokenization/Jaccard incomplete; threshold hardcoded as success | Complete definition: Unicode lowercase, code identifiers (_ -), Chinese per-char, set Jaccard, empty=0, body 500 chars, deterministic tie-break. Threshold candidate 0.20 (pilot), golden ≥40 calibrates final. Acceptance gate Precision≥0.85 Recall≥0.70 FPR≤0.15. |

### C. Pilot invalid (1 finding)

| # | Finding | Fix |
|---|---|---|
| C1 | Previous "5 actual duplicates" had no corresponding concepts → invalid FN=5 | **Redid real pilot**: seeded 5 durable note concepts in temporary bundle, 5 near-duplicate positives + 5 novels (quantum/baking/Chinese/code-id/common-words). BM25 top10 + Jaccard threshold 0.20. Result: TP=5 FP=0 TN=5 FN=0, Precision=1.00 Recall=1.00 FPR=0.00. Pilot n=10, 5 seeds. Golden ≥40 required for implementation. |

### D. Audit trace (1 finding)

| # | Finding | Fix |
|---|---|---|
| D1 | memory_check read-only, client-supplied memory_check_result cannot be proven real → pseudo-audit | **Deleted audit trace entirely**. No S27 (old), no memory_check_result metadata, no idempotency check design. memory_check result used by caller per-call only. Scenarios renumbered S01–S39. |

### E. Governance (2 findings)

| # | Finding | Fix |
|---|---|---|
| E1 | S06 filter `constraint,hold` but order included context → contradiction | Fixed: for_path without governance filter sorts hold→constraint→context; governance filter sorts only filtered levels. S06 has both sub-cases. |
| E2 | hold "MUST request user confirmation" but server can't enforce | Changed to SHOULD (agent may be non-interactive). Tool only warning. S07. |

### F. code_refs bounds (2 findings)

| # | Finding | Fix |
|---|---|---|
| F1 | segment≤8 too small, confused with ** span | Redefined: max patterns=16, max pattern bytes=256, max pattern path segments=32, max ** operators=2, each ** matches max 8 segments. for_path input max 1024 bytes/32 segments. Segment counting defined precisely. |
| F2 | backslash input ambiguous | Decision: reject backslash (forward slash canonical on all platforms). Examples and docs统一. S14. |

### G. Progressive disclosure (4 findings)

| # | Finding | Fix |
|---|---|---|
| G1 | ~44/~50/~111 per-concept set as behavior requirement | Numbers moved to references.md as corpus measurement. Spec requires field shape + reproducible measurement. Success: 329-concept total reduction summary≥50% hit≥40% (regression fixture). |
| G2 | token estimate vague ("response JSON tokens") | Defined: per-item canonical JSON (stable field order, no pretty-print) bytes/4 ceil. Budget accumulates item estimates only, envelope excluded. Distinct from file_bytes/4. S34. |
| G3 | min_required_tokens hardcoded 50 | Dynamic: = first eligible item token estimate by same algorithm. S33. |
| G4 | max_tokens/limit/offset next_offset/omitted vague | Defined precisely: next_offset=index after last returned in filtered+sorted list; omitted_count={budget_omitted, total_remaining}; no partial items. S32. |

### H. Tasks real file paths (1 finding)

| # | Finding | Fix |
|---|---|---|
| H1 | tasks wrote pkg/tool/manifest.go, query.go (don't exist as primary) | Corrected: Manifest primary in `pkg/manifest/manifest.go` + `pkg/tool/service.go`; Query in `pkg/tool/service.go` + `pkg/tool/query.go`; Context in `pkg/tool/service.go`. All task file paths verified. |

### I. Review/machine validation (1 finding)

| # | Finding | Fix |
|---|---|---|
| I1 | review didn't record round 4 findings; machine validation needed | This section records all 9 categories. Machine validation run: no ghost commands, scenario continuous S01–S39,逐行 matrix, no audit trace, no invalid pilot, CustomFields consistent. |

### Round 4 status: all findings fixed.

## 19-dimension compliance check (AGENTS.md)

| Dimension | Status | Notes |
|---|---|---|
| 1. Context logic coherent | ✅ | Extends existing Concept/CustomFields/Manifest/Query/Context; no parallel paths |
| 2. No empty talk | ✅ | Every scenario has implementation entry + test |
| 3. No ambiguity | ✅ | Defaults, thresholds, bounds, token formulas all defined |
| 4. Semantic precision | ✅ | governance values, glob bounds, Jaccard formula precise |
| 5. SDD/TDD ready | ✅ | RED-first tests specified in tasks |
| 6. Minimal implementation | ✅ | pkg/memorymeta only new package; reuses BM25 per-call, Manifest, Query, Context |
| 7. Backward compat | ✅ | All fields optional; full mode default; default order unchanged; context refs optional |
| 8. Breaking changes documented | ✅ | No breaking changes; all extensions additive |
| 9. Risk预判 | ✅ | TOCTOU, symlink, threshold noise, false positives addressed |
| 10. Feasibility verified | ✅ | 3 local experiments completed (references.md) incl. real seeded pilot |
| 11. Layered tasks | ✅ | P0-P4 dependency order clear |
| 12. Extensible | ✅ | governance 3 values; code_refs standard glob with bounds |
| 13. No over-design | ✅ | No database/LLM/new MCP server/AAG/auto-merge/audit persistence |
| 14. Small and efficient | ✅ | Reuses BM25 algorithm (per-call build), Manifest, CustomFields |
| 15. Continuous optimization | ✅ | Thresholds configurable; golden set calibration |
| 16. Architecture unified | ✅ | Same tool surface, same data model, same Skill |
| 17. Full wiring + tests | ✅ | 39 scenarios mapped逐行 to tests + entry points |
| 18. Schedule only dependency | ✅ | P0-P4 all required; 4.0 person-days |
| 19. Spec-impl conformance | ✅ | conformance.md required at P4.3 |

## Key risks and mitigations

| Risk | Mitigation |
|---|---|
| Governance over-use | Default context; no path inference; docs guide appropriate use |
| code_refs bitrot | --stale-refs advisory (fail-closed); no auto-fix |
| Duplicate detection false positives | BM25+Jaccard; golden set calibration; advisory only |
| Duplicate detection false negatives | Pilot shows BM25+Jaccard works on durable set (Recall=1.00 pilot); golden ≥40 with real positives |
| Token budget omits important concepts | Opt-in; governance sort; budget_too_small error; ID parity without budget |
| Hold ignored by agent | MCP Skill W02 SHOULD require hold check + user confirmation |
| Glob ReDoS | Bounded matcher (16 patterns, 256 bytes, 32 segments, 2 **, 8 segment ** span) |
| BM25 per-call build cost | Bounded to 1000 durable concepts; benchmark in implementation; no cache complexity |
| CustomFields performance | Accessor O(1) map lookup per field; no caching |

## Review conclusion

Spec is ready for implementation approval after 4 rounds of review (49 total findings, all fixed). All 39 scenarios testable, all thresholds defined or deferred to golden-set calibration with acceptance gates, all backward compatibility concerns addressed, all entry points verified against current code. No second fact source, no duplicate index, no over-design, no ghost commands, no pseudo-audit.

Key decisions requiring user approval:
1. **CustomFields + pkg/memorymeta** (not Concept struct changes) — AGENTS.md constraint.
2. **No path-based governance inference** (all default context) — minimal surprise.
3. **memory_check read-only via okf_query -q --memory-check** (no write blocking, no allow_duplicate, no audit persistence).
4. **No conflict classification** — cannot infer without LLM.
5. **BM25 per-call build on bounded durable set** (no Service cache, max 1000 concepts).
6. **for_path lexical only** (no EvalSymlinks, no file-existence).
7. **stale-refs fail-closed** (incomplete+warnings, never silent empty).
8. **context refs added to existing okf_context** (query OR refs, not new tool).
9. **Token estimate = canonical JSON item bytes/4** (budget items only, no envelope).
