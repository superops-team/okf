# References — add-governed-agent-memory

## Feasibility experiments (2026-09-16, local, temporary code, no formal changes)

### Experiment 1: code_refs glob matching (lexical)

**Method**: Simulated 5 concepts with code_refs patterns, queried `pkg/mcp/server.go`.

**Result**:
- Query `pkg/mcp/server.go` matched exactly `arch/layers` (pattern `pkg/mcp/*.go`).
- `**/*.go` would match everything — requires depth limit (set to 8 levels).
- Direct `filepath.Match` handles single-segment; recursive needs custom bounded matcher.
- for_path is one input path vs pattern match — no file scanning.

**Conclusion**: for_path matching feasible with Go stdlib + bounded recursive matcher. Precision high with specific patterns.

### Experiment 2: governance default inference

**Method**: Simulated concepts with various governance values and path locations.

**Decision**: No path-based inference in v1. All concepts default to `context` without explicit field. `convention/` auto-constraint was considered but rejected (strong inference without evidence, minimal surprise violation). Templates/docs may suggest explicit `governance: constraint`.

### Experiment 3: duplicate detection pilot (confusion matrix)

**Method**: 329 concepts from `.okf/knowledge/` (mostly `code_file` type, auto-generated from source). 10 test cases (5 near-duplicate positives, 5 novel negatives). Title+body Jaccard at 0.15 threshold.

**Result (PILOT, n=10, Jaccard-only)**:

| | Predicted duplicate | Predicted novel |
|---|---|---|
| Actual duplicate | TP=0 | FN=5 |
| Actual novel | FP=0 | TN=5 |

- Precision: N/A (0 predicted duplicates)
- Recall: 0.00
- FPR: 0.00

**Analysis**: Jaccard-only has 0 recall on code_file-centric KB because test "duplicates" (MCP server, BM25, etc.) are domain concepts that don't exist as separate concepts in the code_file-centric knowledge base. BM25 candidate generation is essential to improve recall. Golden set (implementation phase) needs ≥40 cases with actual note/decision concept positives, Chinese/English/code identifiers, and common-word negatives.

**Conclusion**: Advisory non-blocking is essential given threshold uncertainty. BM25 top-N + Jaccard re-rank is the designed approach. Pilot does NOT validate thresholds; golden set calibration required in implementation.

### Experiment 4: progressive disclosure token measurement (REAL)

**Method**: Built actual ManifestItem JSON for 329 concepts from `.okf/knowledge/`. Measured JSON output bytes for three projection modes. Token estimate = JSON bytes / 4.

**Result (329 concepts, real measurement)**:

| Mode | JSON bytes | Tokens (bytes/4) | Tokens/concept | Reduction vs full |
|---|---:|---:|---:|---:|
| full (default) | 145,916 | 36,479 | 110.9 | — |
| hit | 65,247 | 16,311 | 49.6 | −55.3% |
| summary | 57,351 | 14,337 | 43.6 | −60.7% |

- Total source file bytes: 1,932,655 (file_bytes/4 = 483,164 tokens — this is the existing ManifestItem.EstimatedTokens, NOT the response budget).
- Response token budget uses JSON projection bytes/4, distinct from file_bytes/4.

**Conclusion**: Summary mode reduces response tokens by ~61%, hit by ~55%. ID parity guaranteed without max_tokens.

## Existing capabilities verified (not re-implemented)

| Capability | Location | Status |
|---|---|---|
| BM25 lexical search | `pkg/lexical/` | exists, reuse for candidate generation |
| Semantic + hybrid search | `pkg/query/`, `pkg/vectorindex/` | exists |
| Durable note/log/feedback | `pkg/tool/write.go` | exists, WriteKnowledgeRequest has Kind/Content/Project/Tags/Metadata/IdempotencyKey/EvidenceRefs (NO title) |
| Stable ID (okf_id) | `pkg/tool/identity.go` | exists, reuse |
| Manifest | `pkg/tool/manifest.go`, `pkg/manifest/` | exists, ManifestItem has okf_id/ref/path/title/description/type/tags/status/trust_tier/stale/file_size_bytes/estimated_tokens |
| Query/Context | `pkg/tool/query.go` | exists, extend with memory_check |
| MCP Skills (W01-W07) | `pkg/agentconfig/workflow.go` | exists, extend clauses |
| Agent Integration | `pkg/agentconfig/` | exists |
| CustomFields | `pkg/okf/types.go` (inline YAML map) | exists, USE for governance/code_refs |
| OKF v0.2 fields | `pkg/okf/types.go` | exists, preserve |
| CLI entry | `okf tool <operation>` (status/init/refresh/query/context/manifest) | verified, NOT `okf manifest/query` |
| Service BM25 cache | `pkg/tool/service.go` has NO BM25 cache field | must verify/build per call or add caching in implementation |

## Related work

- okf-agent-memory (external): governance model, code_refs, search-before-write. Key difference: their approach modifies concept types; ours uses CustomFields + accessor per AGENTS.md constraint.

## Key decisions (with rationale)

1. **CustomFields + pkg/memorymeta, not Concept struct**: AGENTS.md prohibits core model changes for feature extensions. CustomFields is the OKF v0.2 extension mechanism.
2. **No path-based governance default**: minimal surprise, backward compatible. All concepts default context.
3. **memory_check read-only via okf_query**: avoids zombie allow_duplicate; check-then-write is Agent Skill responsibility.
4. **No conflict classification**: cannot infer semantic conflict without external LLM (non-goal).
5. **BM25 + Jaccard, not raw BM25 threshold**: BM25 raw scores not [0,1]; Jaccard provides normalized threshold.
6. **for_path lexical only**: supports not-yet-created/renamed paths; no TOCTOU from FS.
7. **stale-refs fail-closed**: silent empty would mislead as "no stale refs"; incomplete+warnings is honest.
8. **JSON bytes/4 for response budget**: distinct from existing file_bytes/4; deterministic and measurable.
9. **No new MCP tools in v1**: extend existing okf_manifest/okf_query params; both eras.
