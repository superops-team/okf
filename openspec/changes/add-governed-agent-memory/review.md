# Review — add-governed-agent-memory

## Review round 1: initial Spec review (15 findings)

### Findings

| # | Severity | Finding | Fix |
|---|---|---|---|
| R1.1 | high | strict mode undefined | Defined: strict rejects unknown governance; non-strict warns + context. |
| R1.2 | high | recursive glob depth unspecified | Fixed: max 8 levels, max 1000→removed (for_path doesn't scan files). |
| R1.3 | medium | "conflicting content" ambiguous | Removed conflict classification entirely (cannot infer without LLM). |
| R1.4 | medium | token budget partial concept undefined | Fixed: no partial concepts; budget_too_small error with min_required_tokens. |
| R1.5 | medium | code_refs vs resource field unclear | Added design §4.5 table clarifying distinction. |
| R1.6 | low | <50ms threshold unjustified | Removed; performance threshold from real benchmark in implementation. |
| R1.7 | low | governance inheritance unclear | Fixed: no inheritance, per-concept only. |

### Round 1 status: all findings fixed.

## Review round 2: implicit issues (8 findings)

| # | Severity | Finding | Fix |
|---|---|---|---|
| R2.1 | high | audit trace = second fact source | Clarified: part of concept's own CustomFields metadata, bounded, no separate log. |
| R2.2 | high | hold = server-enforced block | Fixed: hold advisory only; server never blocks; Agent Skill asks user. |
| R2.3 | high | governance sort changes default manifest order | Fixed: default order unchanged; sort only with --for_path/--governance. |
| R2.4 | medium | symlink TOCTOU in for_path | Fixed: for_path is lexical only, no EvalSymlinks; stale-refs handles FS. |
| R2.5 | medium | max_tokens regresses Recall | Fixed: ID parity guarantee (not Recall@5); manifest has no query ranking. |
| R2.6 | medium | stale-refs returns empty on error | Fixed: fail-closed, incomplete:true + warnings, never silent empty. |
| R2.7 | low | concurrent write TOCTOU | Documented: advisory reflects check-time state; acceptable. |
| R2.8 | low | custom governance field conflict | Fixed: CustomFields accessor; formal field NOT added to Concept struct. |

### Round 2 status: all findings fixed.

## Independent acceptance review (22 findings, all fixed)

### A. Data model/architecture

| # | Finding | Fix |
|---|---|---|
| A1 | Concept struct modified despite AGENTS.md prohibition | **Fixed**: governance/code_refs stored in CustomFields; new `pkg/memorymeta` typed accessor; Concept struct unchanged. Removed "formal field migration/precedence". |
| A2 | audit trace unbounded | **Fixed**: max 3 candidates, only okf_id+ref+score, ≤256 bytes, idempotent replay doesn't re-run. |

### B. Governance

| # | Finding | Fix |
|---|---|---|
| B3 | default sort always hold→constraint→context contradicts review | **Fixed**: default order unchanged; governance sort only with --for_path/--governance. Added S05 regression scenario. |
| B4 | "fail closed for holds" contradicts advisory | **Fixed**: removed fail-closed language; hold returns governance_warning only; server never blocks. |
| B5 | convention/ default constraint is strong inference without evidence | **Fixed**: v1 defaults ALL concepts to context; no path-based inference. Templates may suggest explicit constraint. |

### C. code_refs / for_path

| # | Finding | Fix |
|---|---|---|
| C6 | "max 1000 files per pattern" wrong for for_path (doesn't scan FS) | **Fixed**: for_path is one-input-path vs pattern match; defined pattern count/length/segment bounds; ** recursion complexity bound. Only stale-refs scans FS (50k bound). |
| C7 | for_path requires EvalSymlinks/file existence | **Fixed**: lexical canonicalization only; no EvalSymlinks; supports not-yet-created/renamed paths. stale-refs fail-closed on symlink escape/unreadable (returns incomplete+warnings, not silent empty). |
| C8 | code_refs vs source_path/file_path/code_file unclear | **Fixed**: design §4.5 table; S19 scenario; code_refs = target code governed, not concept's own file or code_file concept path. |

### D. Search-before-write

| # | Finding | Fix |
|---|---|---|
| D9 | allow_duplicate is zombie (advisory never blocks) | **Fixed**: deleted allow_duplicate/override. memory_check is read-only via okf_query. Agent Skill W06: check-then-explicit-write. |
| D10 | possible_conflict is false capability (no LLM) | **Fixed**: deleted conflict classification; only no_similar/possible_duplicate. |
| D11 | WriteKnowledgeRequest has no title; BM25 raw score not [0,1] | **Fixed**: uses actual Content/Kind/Project/Tags/Metadata. BM25 top-10 candidates + Jaccard [0,1] threshold. Removed "BM25 cached" unverified claim. |
| D12 | 5 novel samples → FPR=0 claim invalid; no duplicate recall | **Fixed**: pilot confusion matrix honestly reported (TP=0 FP=0 TN=5 FN=5, Jaccard-only on code_file-centric KB). Golden set ≥40 required for implementation. |
| D13 | concurrency/idempotency/audit bounds undefined | **Fixed**: check_only read-only; same idempotency replay stable; concurrent writes may not see each other (documented); audit ≤3 candidates ≤256 bytes. |

### E. Progressive disclosure

| # | Finding | Fix |
|---|---|---|
| E14 | "full 300+ token" estimate not measured | **Fixed**: real measurement on 329 concepts: full=110.9, hit=49.6, summary=43.6 tokens/concept (JSON bytes/4). |
| E15 | Recall@5 concept invalid for manifest (no query ranking) | **Fixed**: ID parity guarantee (100% set/order match without max_tokens). for_path ID parity across modes. |
| E16 | okf_context --id doesn't exist | **Fixed**: follow-up via existing `okf tool query --ref` or `okf tool resolve --ref`. |
| E17 | max_tokens offset/limit interaction undefined; budget<first → infinite loop | **Fixed**: pipeline filter→sort→offset→limit→token budget; budget_too_small error with min_required_tokens; no "at most ~16" vague claim. |
| E18 | token estimate conflates file_bytes/4 with response JSON bytes/4 | **Fixed**: response budget = JSON bytes/4; existing ManifestItem.EstimatedTokens = file_bytes/4 (source file estimate). Distinct. |

### F. Wiring/naming/tests

| # | Finding | Fix |
|---|---|---|
| F19 | CLI entry is `okf tool`, not `okf manifest/query/note` | **Fixed**: all entry points corrected to `okf tool manifest/query`; durable writes via MCP tools/Service. S40 verifies. |
| F20 | test matrix used range rows (S01–S03) | **Fixed**:逐行 matrix S01–S40, each with test + entry point. |
| F21 | performance thresholds cite unverified "BM25 ~300µs" | **Fixed**: removed; thresholds from real benchmark in implementation phase; O(n×p) and pattern bounds defined. |
| F22 | review didn't record independent acceptance findings | **Fixed**: this section records all 22 findings and fixes. |

### Independent review status: all 22 findings fixed.

## 19-dimension compliance check (AGENTS.md)

| Dimension | Status | Notes |
|---|---|---|
| 1. Context logic coherent | ✅ | Extends existing Concept/CustomFields/Manifest/Query; no parallel paths |
| 2. No empty talk | ✅ | Every scenario has implementation entry + test |
| 3. No ambiguity | ✅ | Defaults, thresholds, error types all defined |
| 4. Semantic precision | ✅ | governance values, glob syntax, Jaccard formula precise |
| 5. SDD/TDD ready | ✅ | RED-first tests specified in tasks |
| 6. Minimal implementation | ✅ | pkg/memorymeta only new package; reuses BM25/Manifest/Query |
| 7. Backward compat | ✅ | All fields optional; full mode default; default order unchanged |
| 8. Breaking changes documented | ✅ | No breaking changes; all extensions additive |
| 9. Risk预判 | ✅ | TOCTOU, symlink, threshold noise, false positives addressed |
| 10. Feasibility verified | ✅ | 3 local experiments completed (references.md) |
| 11. Layered tasks | ✅ | P0-P4 dependency order clear |
| 12. Extensible | ✅ | governance values fixed 3 for clarity; code_refs standard glob |
| 13. No over-design | ✅ | No database/LLM/new MCP server/AAG/auto-merge |
| 14. Small and efficient | ✅ | Reuses BM25, Manifest, CustomFields |
| 15. Continuous optimization | ✅ | Thresholds configurable; golden set calibration |
| 16. Architecture unified | ✅ | Same tool surface, same data model, same Skill |
| 17. Full wiring + tests | ✅ | 40 scenarios mapped逐行 to tests + entry points |
| 18. Schedule only dependency | ✅ | P0-P4 all required; 4.0 person-days |
| 19. Spec-impl conformance | ✅ | conformance.md required at P4.4 |

## Key risks and mitigations

| Risk | Mitigation |
|---|---|
| Governance over-use | Default context; no path inference; docs guide appropriate use |
| code_refs bitrot | --stale-refs advisory (fail-closed); no auto-fix |
| Duplicate detection false positives | BM25+Jaccard; golden set calibration; advisory only |
| Duplicate detection false negatives | Pilot shows Jaccard-only recall=0; BM25 candidate generation essential; golden set with real positives |
| Token budget omits important concepts | Opt-in; governance sort; budget_too_small error; ID parity without budget |
| Hold ignored by agent | MCP Skill W02 explicitly requires hold check + user confirmation |
| Glob ReDoS | Bounded matcher (8 levels, 16 patterns, 256 chars, 8 segments) |
| CustomFields performance | Accessor caches nothing; O(1) map lookup per field |

## Review conclusion

Spec is ready for implementation approval after independent review. All 40 scenarios testable, all thresholds defined or deferred to golden-set calibration, all backward compatibility concerns addressed. No second fact source, no duplicate index, no over-design, no ghost commands.

Key decisions requiring user approval:
1. **CustomFields + pkg/memorymeta** (not Concept struct changes) — approved by AGENTS.md constraint.
2. **No path-based governance inference** (all default context) — minimal surprise/backward compat.
3. **memory_check read-only via okf_query** (no write blocking, no allow_duplicate) — advisory only.
4. **No conflict classification** — cannot infer semantic conflict without LLM.
5. **for_path lexical only** (no FS, no EvalSymlinks) — supports not-yet-created paths.
6. **stale-refs fail-closed** (incomplete+warnings, never silent empty).
