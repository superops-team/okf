# Proposal: Governed Agent Memory Enhancement

## Problem

OKF already provides a rich knowledge base with durable capture (note/log/feedback), Stable Identity, Manifest, query/context, BM25 lexical + semantic hybrid search, MCP Skills (W01–W07), and Agent Integration. However, three gaps limit how effectively an AI agent can *consume and govern* this memory:

1. **No governance semantics**: Concepts are undifferentiated. An agent cannot distinguish a mandatory architectural constraint ("zero external dependencies") from an informative background fact ("we used PostgreSQL in 2023") from an active freeze ("auth subsystem in migration"). This causes blind modification and authority blindness.

2. **No code-to-knowledge binding**: When an agent prepares to edit `pkg/mcp/server.go`, it has no way to discover which concepts govern that file. Knowledge is not proactively surfaced at the point of code modification.

3. **No search-before-write**: Agents can create duplicate or conflicting notes without checking existing memory. This causes concept proliferation and hallucinated divergence.

4. **No progressive disclosure**: `okf_manifest` returns full metadata for all concepts, which can exceed context budgets. Agents need a summary → hit → body hierarchy to load only what's relevant.

## Proposed solution

Add four additive, backward-compatible enhancements to the existing OKF knowledge model and tool surface:

### 1. Governance semantics (`governance` field)

Add an optional `governance` frontmatter field with three values:
- `constraint` — mandatory guardrail; agent must adhere
- `hold` — execution freeze; agent must not modify referenced code without human signoff
- `context` — informative domain knowledge (default for most concepts)

Default inference: concepts under `convention/` default to `constraint`; all others default to `context`. Unknown values are treated as `context` for backward compatibility. Governance is exposed via Manifest/Query/Context filtering and sorting, and consumed by the Agent Skill (W01–W07 extension).

### 2. Code-to-knowledge binding (`code_refs` + `for_path`)

Add an optional `code_refs` frontmatter field: a list of repo-relative paths or globs. Add a `for_path` parameter to `okf_manifest` and `okf_query` that returns concepts whose `code_refs` match the given path. This enables proactive discovery of governing knowledge when editing code.

### 3. Search-before-write (advisory)

Before `okf_note`/`okf_log`/`okf_feedback` persists, run a similarity search over existing concepts. Return an advisory result: `no_duplicate`, `possible_duplicate` (with candidate IDs and similarity), or `possible_conflict` (with conflicting concept IDs). First version is **advisory, non-blocking**; the caller may override with `allow_duplicate: true`. An audit trace is recorded.

### 4. Progressive disclosure

Extend `okf_manifest` to support a `summary` mode (title + type + governance + one-line description, ~50 tokens/concept) and a `hit` mode (summary + tags + code_refs, ~150 tokens/concept). Full body remains via `okf_context`/`okf_query`. This reduces token consumption while preserving Recall@K.

## Non-goals

- **No database, external LLM, or cloud service** — all operations are local, in-process, using existing BM25/semantic indexes.
- **No new MCP server or protocol** — enhancements extend existing `okf_manifest`/`okf_query`/`okf_note`/`okf_log`/`okf_feedback` tools.
- **No AAG or new instruction syntax** — governance is consumed through existing MCP Skill workflow extensions.
- **No mandatory write blocking** — search-before-write is advisory only; no auto-delete, auto-merge, or auto-archive.
- **No second index or second fact source** — governance/code_refs are concept frontmatter fields; progressive disclosure reuses existing Manifest data.

## Compatibility

- All new fields are optional; existing concepts without `governance`/`code_refs` continue to work with defaults.
- `governance` and `code_refs` are stored as formal Concept struct fields (not CustomFields) for type safety and validation, with CustomFields fallback for unknown keys.
- No breaking changes to existing tool signatures; new parameters are optional with backward-compatible defaults.
- OKF v0.2 spec compatibility: `governance` and `code_refs` are extension fields (spec §7 open family), not core spec changes.

## Impact

- **Packages**: `pkg/okf` (Concept struct, validation), `pkg/tool` (Manifest/Query/Write services), `pkg/agentconfig` (Skill workflow extension), `pkg/mcp` (tool parameter exposure).
- **CLI**: `okf manifest --for-path`, `okf query --for-path`, `okf note --check-duplicates`.
- **MCP tools**: `okf_manifest`/`okf_query` gain `for_path`; `okf_note`/`okf_log`/`okf_feedback` gain advisory duplicate check response.
- **Docs**: `docs/knowledge/`, README, MCP server docs, release notes.

## Success criteria

- `for_path` precision ≥ 0.9 and recall ≥ 0.8 on a curated fixture.
- Search-before-write duplicate detection recall ≥ 0.7 with false positive rate ≤ 0.15 on golden set.
- Progressive disclosure reduces manifest token usage by ≥ 40% with Recall@5 no regression.
- Governance filtering: 100% of `hold` concepts surfaced in `for_path` queries; 0% of `context` concepts incorrectly marked as constraints.
- All existing tests pass; gauntlet green; coverage ≥ 60%.
