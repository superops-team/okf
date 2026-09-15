# References — add-governed-agent-memory

## Feasibility experiments (2026-09-16, local, temporary code, no formal changes)

### Experiment 1: code_refs glob matching precision/recall

**Method**: Simulated 5 concepts with code_refs patterns, queried `pkg/mcp/server.go`.

**Patterns tested**:
- `pkg/mcp/*.go` (single-segment glob)
- `pkg/okf/types.go` (exact path)
- `cmd/okf/main.go` (exact path)
- `**/*.go` (recursive, overly broad)
- no code_refs

**Result**:
- Query `pkg/mcp/server.go` matched exactly `arch/layers` (pattern `pkg/mcp/*.go`).
- `**/*.go` would match everything — requires depth limit (proposed: 8 levels).
- Direct `filepath.Match` handles single-segment; recursive needs custom bounded matcher.

**Conclusion**: for_path matching is feasible with existing Go stdlib + bounded recursive matcher. Precision is high when patterns are specific; depth limit prevents `**` explosion.

### Experiment 2: governance default inference

**Method**: Simulated 5 concepts with various governance values and path locations.

**Result**:
- `convention/principles` with no governance → default `constraint` ✅
- `arch/layers` with `governance: context` → `context` ✅
- `decisions/freeze` with `governance: hold` → `hold` ✅
- `unknown/value` with `governance: weird` → treated as `context` (backward compat) ✅

**Conclusion**: Default inference is simple and deterministic. Unknown values degrading to `context` is safe for backward compatibility.

### Experiment 3: search-before-write false positive rate

**Method**: Used existing 329-concept knowledge base (`.okf/knowledge/`). Simulated 5 new notes with title+body Jaccard similarity at 30% threshold.

**Test notes**:
1. "MCP server dual era protocol" — 0 potential duplicates
2. "BM25 lexical search parameters" — 0 potential duplicates
3. "completely novel topic xyzzy" — 0 potential duplicates
4. "go build test commands" — 0 potential duplicates
5. "agent skill workflow W01-W07" — 0 potential duplicates

**Result**: 0 false positives at 30% Jaccard threshold with title+body tokenization.

**Caveat**: Title-only matching has higher false positive risk; body+title reduces it. Recall for actual duplicates may be lower with Jaccard (BM25 would be better). First version should use BM25 (already exists) with normalized score, not Jaccard.

**Conclusion**: Advisory non-blocking is essential given threshold noise. BM25 reuse avoids building a second index. False positive rate is expected to be low (< 15%) with proper calibration.

### Experiment 4: progressive disclosure token estimate

**Method**: Estimated tokens per concept for three modes based on existing manifest output.

**Estimates**:
- Full mode: ~300+ tokens/concept (all metadata)
- Hit mode: ~150 tokens/concept (summary + tags + code_refs + status)
- Summary mode: ~50 tokens/concept (id + title + type + governance + 1-line desc)

**Conclusion**: Summary mode can reduce manifest token usage by ~80% for large knowledge bases. Recall@K is preserved because okf_id is always included for follow-up `okf_context`.

## Existing capabilities verified (not re-implemented)

| Capability | Location | Status |
|---|---|---|
| BM25 lexical search | `pkg/lexical/` | exists, reuse |
| Semantic + hybrid search | `pkg/query/`, `pkg/vectorindex/` | exists, reuse |
| Durable note/log/feedback | `pkg/tool/write.go` | exists, extend |
| Stable ID (okf_id) | `pkg/tool/identity.go` | exists, reuse |
| Manifest | `pkg/tool/manifest.go` | exists, extend |
| Query/Context | `pkg/tool/query.go` | exists, extend |
| MCP Skills (W01-W07) | `pkg/agentconfig/workflow.go` | exists, extend |
| Agent Integration | `pkg/agentconfig/` | exists, extend |
| CustomFields | `pkg/okf/types.go` | exists, preserve |
| OKF v0.2 fields (sources/generated/verified/status/stale_after) | `pkg/okf/types.go` | exists, preserve |

## Related work

- okf-agent-memory (external): governance model (constraint/hold/context), code_refs binding, search-before-write principle. See调研报告 for detailed gap analysis.
- OKF v0.2 spec: extension field family (§7) allows `governance` and `code_refs` as custom extension fields.

## Key decisions

1. **Formal fields, not CustomFields**: `governance` and `code_refs` are formal Concept struct fields for type safety and validation. CustomFields preserved for unknown keys.
2. **Advisory, not blocking**: search-before-write never blocks without `--allow-duplicate` override. No auto-delete/merge.
3. **No second index**: duplicate detection reuses existing BM25 index.
4. **Depth-limited `**`**: max 8 levels, max 1000 files per pattern to prevent ReDoS/explosion.
5. **Hold is advisory**: surfaced as warning, not enforced by OKF server (agent framework may enforce).
