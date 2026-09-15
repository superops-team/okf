# References — add-governed-agent-memory

## Feasibility experiments (2026-09-16, local, temporary code, no formal changes)

### Experiment 1: code_refs glob matching (lexical)

**Method**: Simulated concepts with code_refs patterns, queried `pkg/mcp/server.go`.

**Result**:
- Query `pkg/mcp/server.go` matched exactly `arch/layers` (pattern `pkg/mcp/*.go`).
- `**/*.go` would match everything — requires depth limit (each ** max 8 segments, max 2 ** per pattern).
- for_path is one input path vs pattern match — no file scanning.

**Conclusion**: for_path matching feasible with Go stdlib + bounded recursive matcher.

### Experiment 2: governance default inference

**Decision**: No path-based inference in v1. All concepts default to `context` without explicit field. `convention/` auto-constraint was considered but rejected (strong inference without evidence, minimal surprise violation). Templates/docs may suggest explicit `governance: constraint`.

### Experiment 3: duplicate detection REAL pilot (seeded durable concepts)

**Method**: Created temporary bundle with 5 seeded durable note concepts (realistic content). Built BM25 index from seeds only (durable types note/event/feedback). 10 test cases: 5 near-duplicate positives (rephrased seed content), 5 novel negatives (quantum, baking, Chinese, code identifiers, common English words). BM25 top-10 candidates → normalized token Jaccard [0,1] re-rank. Threshold 0.20.

**Seeded durable concepts** (5):
1. note-mcp-arch: "MCP server dual protocol design..."
2. note-bm25: "BM25 lexical search implementation..."
3. note-stable-id: "Stable okf_id identity resolution..."
4. note-durable-write: "Durable note event feedback write path..."
5. note-agent-skill: "Agent skill W01-W07 canonical workflow..."

**Tokenization**: Unicode lowercase, code identifiers (_ -), Chinese per-character, set Jaccard, body truncate 500 chars.

**Result (REAL PILOT, n=10, 5 durable seeds, BM25 top10 + Jaccard threshold=0.20)**:

| | Predicted duplicate | Predicted novel |
|---|---|---|
| Actual duplicate | TP=5 | FN=0 |
| Actual novel | FP=0 | TN=5 |

- Precision=1.00, Recall=1.00, FPR=0.00

**Per-case Jaccard scores** (sorted desc):
- 0.459 dup-skill-rephrase (duplicate)
- 0.455 dup-stableid-rephrase (duplicate)
- 0.372 dup-bm25-rephrase (duplicate)
- 0.308 dup-mcp-rephrase (duplicate)
- 0.286 dup-write-rephrase (duplicate)
- 0.024 novel-common (novel)
- 0.000 novel-quantum, novel-baking, novel-chinese, novel-codeid (novel)

**Clear separation**: duplicates 0.286–0.459, novels 0.000–0.024. Threshold 0.20 works.

**Caveat**: PILOT ONLY (n=10, 5 seeds). Golden set (implementation phase) requires ≥40 cases with more durable seeds, diverse negatives (Chinese/English/code identifiers/common words), and edge cases (partial overlap, cross-type similarity). Acceptance gate: Precision≥0.85, Recall≥0.70, FPR≤0.15.

**Previous invalid pilot** (code_file-centric KB, no seeded durable concepts, Jaccard-only): TP=0 FP=0 TN=5 FN=5. That was invalid because "actual duplicates" had no corresponding concepts. Replaced by this seeded pilot.

### Experiment 4: progressive disclosure token measurement (REAL)

**Method**: Built actual ManifestItem JSON for 329 concepts from `.okf/knowledge/`. Measured JSON output bytes for three projection modes. Token estimate = JSON bytes / 4.

**Result (329 concepts, real measurement)**:

| Mode | JSON bytes | Tokens (bytes/4) | Tokens/concept | Reduction vs full |
|---|---:|---:|---:|---:|
| full (default) | 145,916 | 36,479 | 110.9 | — |
| hit | 65,247 | 16,311 | 49.6 | −55.3% |
| summary | 57,351 | 14,337 | 43.6 | −60.7% |

- Total source file bytes: 1,932,655 (file_bytes/4 = 483,164 — this is existing ManifestItem.EstimatedTokens, NOT response budget).
- Response token budget = JSON projection bytes/4, distinct from file_bytes/4.

**Conclusion**: Summary mode reduces response tokens by ~61%, hit by ~55%. Success criterion: 329-concept corpus total reduction summary≥50%, hit≥40% (regression fixture). ID parity guaranteed without max_tokens.

## Existing capabilities verified (not re-implemented)

| Capability | Location | Status |
|---|---|---|
| BM25 lexical search | `pkg/lexical/` | exists, per-call build on bounded set |
| Semantic + hybrid search | `pkg/query/`, `pkg/vectorindex/` | exists |
| Durable note/log/feedback | `pkg/tool/write.go` | exists, WriteKnowledgeRequest has Kind/Content/Project/Tags/Metadata/IdempotencyKey/EvidenceRefs (NO title) |
| Stable ID (okf_id) | `pkg/tool/identity.go` | exists, `okf identity resolve --ref` returns path only |
| Manifest | `pkg/manifest/manifest.go`, `pkg/tool/service.go` | exists, ManifestItem has okf_id/ref/path/title/description/type/tags/status/trust_tier/stale/file_size_bytes/estimated_tokens |
| Query/Context | `pkg/tool/service.go` (QueryRequest, ContextRequest) | exists, Query has Query/Types/Project/Tag; Context has Query/BudgetTokens (no refs yet) |
| MCP Skills (W01-W07) | `pkg/agentconfig/workflow.go` | exists, extend clauses |
| Agent Integration | `pkg/agentconfig/` | exists |
| CustomFields | `pkg/okf/types.go` (inline YAML map) | exists, USE for governance/code_refs |
| OKF v0.2 fields | `pkg/okf/types.go` | exists, preserve |
| CLI entry | `okf tool <operation>` (status/init/refresh/query/context/manifest) | verified, NOT `okf manifest/query` |
| CLI lint | `okf lint --strict` | verified, NOT `okf tool lint` |
| Service BM25 cache | `pkg/tool/service.go` has NO BM25 cache field | per-call build required |
| Durable write CLI | NONE — only MCP tools/Service | verified |

## Related work

- okf-agent-memory (external): governance model, code_refs, search-before-write. Key difference: their approach modifies concept types; ours uses CustomFields + accessor per AGENTS.md constraint.

## Key decisions (with rationale)

1. **CustomFields + pkg/memorymeta, not Concept struct**: AGENTS.md prohibits core model changes. CustomFields is OKF v0.2 extension mechanism.
2. **No path-based governance default**: minimal surprise, backward compatible. All concepts default context.
3. **memory_check read-only via okf query -q --memory-check**: avoids zombie allow_duplicate; check-then-write is Agent Skill responsibility. Dedicated MemoryCheckResult envelope.
4. **No conflict classification**: cannot infer semantic conflict without external LLM (non-goal).
5. **BM25 per-call build on bounded durable set**: Service has no cache; avoid cache complexity; max 1000 durable concepts.
6. **Candidate source durable types only** (note,event,feedback) by default: avoids code_file pollution; --types can extend.
7. **for_path lexical only**: supports not-yet-created/renamed paths; no TOCTOU from FS.
8. **stale-refs fail-closed**: silent empty would mislead as "no stale refs"; incomplete+warnings is honest.
9. **No audit trace persistence**: read-only check; client-supplied metadata untrusted, no audit value. Result used per-call.
10. **context refs added to existing okf_context**: query OR refs; not new tool; uses existing identity resolve for ref→path.
11. **Token estimate = canonical JSON item bytes/4**: deterministic, measurable; budget items only (no envelope); distinct from file_bytes/4.
12. **3 governance values only** (constraint/hold/context): no "review"/"deprecated" (latter uses existing status field).
13. **Pattern bounds**: 16 patterns, 256 bytes, 32 segments, 2 ** operators, each ** max 8 segments — prevents ReDoS/explosion.
