# Proposal: Governed Agent Memory Enhancement

## Problem

OKF already provides durable capture (note/log/feedback), Stable Identity, Manifest, query/context, BM25 lexical + semantic hybrid search, MCP Skills (W01–W07), and Agent Integration. Three gaps limit agent memory effectiveness:

1. **No governance semantics**: Concepts are undifferentiated. An agent cannot distinguish a mandatory architectural constraint from an informative fact from an active freeze.
2. **No code-to-knowledge binding**: When editing `pkg/mcp/server.go`, the agent cannot discover which concepts govern that file.
3. **No search-before-write**: Agents can create duplicate notes without checking existing memory.
4. **No progressive disclosure**: `okf tool manifest` returns full metadata for all concepts, which can exceed context budgets.

## Proposed solution

Four additive, backward-compatible enhancements. **All new fields are stored in existing CustomFields (OKF v0.2 extension mechanism), accessed via a typed accessor package `pkg/memorymeta`. The core Concept struct is NOT modified.**

### 1. Governance semantics (CustomFields + typed accessor)

Add optional frontmatter fields `governance` (constraint|hold|context) and `code_refs` ([]string). These are stored in `Concept.CustomFields` and accessed via `pkg/memorymeta` typed accessors/normalizers/validators. No second set of fields; parser/query adapters read from CustomFields through the accessor.

- Default: all concepts default to `context`. **No path-based inference** (no `convention/` → constraint). Only explicit values take effect.
- Unknown values: non-strict → `context` + warning; strict → error.
- Hold is advisory only: tools return `governance_warning`; OKF server never blocks writes or edits. Agent Skill instructs user confirmation before modifying hold-governed code.

### 2. Code-to-knowledge binding (code_refs + for_path)

`code_refs`: list of repo-relative paths or globs. `for_path` parameter on `okf tool manifest`/`okf tool query` returns concepts whose patterns match the input path.

- **Lexical matching only**: canonicalize repo-relative path, reject absolute/`..`/NUL/backslash ambiguity. No `EvalSymlinks`, no file-existence requirement (supports not-yet-created/renamed targets).
- Pattern bounds: max 16 patterns per concept, max 256 chars per pattern, max 8 path segments. `**` recursion bounded to 8 levels.
- `stale-refs` (advisory): scans filesystem to find concepts whose code_refs match no existing files. **Fail closed on symlink escape/unreadable**: returns `incomplete: true` + warnings, never silently empty.

### 3. Search-before-write (read-only advisory via okf_query)

Extend existing `okf tool query` with `memory_check: true` parameter. Returns BM25 top-N candidates re-ranked by normalized token Jaccard [0,1]. Classification: `no_similar` (Jaccard < threshold) or `possible_duplicate` (Jaccard >= threshold).

- **No `allow_duplicate`/override** (zombie parameter when advisory never blocks).
- **No conflict classification** (cannot infer semantic conflict without external LLM; false capability).
- **No write blocking**: `memory_check` is read-only. Agent Skill W06 instructs: first check, then explicitly write. The write tools (okf_note etc.) always write.
- BM25 raw scores are not [0,1]; use BM25 for top-10 candidate generation, then Jaccard for threshold decision.
- Threshold calibrated on golden set (>=40 cases); pilot (n=10) shows Jaccard recall=0 on code_file-centric KB, indicating BM25 candidate generation is essential.

### 4. Progressive disclosure (manifest modes)

Extend `okf tool manifest` with `--mode summary|hit|full`:
- `summary`: okf_id, title, type, governance, 1-line description (~44 tokens/concept measured)
- `hit`: summary + tags, code_refs, status, stale_after (~50 tokens/concept measured)
- `full`: existing full metadata (default, backward compatible; ~111 tokens/concept measured)

Real measurement on 329 concepts: full=36,479 tokens, hit=16,311 (−55%), summary=14,337 (−61%). Token estimate = JSON response bytes/4 (not file_bytes/4).

- **ID parity guarantee**: without `--max-tokens`, summary/hit/full return identical concept ID sets and order.
- `--max-tokens`: filter → stable sort → offset → limit → token budget. Returns `next_offset`, `omitted_count`, `truncated`. If budget < first item, returns `budget_too_small` error with `min_required_tokens` (no infinite loop).
- Follow-up for full body: use existing `okf tool query` or `okf tool resolve` (not a non-existent `okf_context --id`).

## Non-goals

- No database, external LLM, cloud service, or new MCP server/protocol.
- No AAG or new instruction syntax.
- No mandatory write blocking, auto-delete, auto-merge, or auto-archive.
- No second index or second fact source: governance/code_refs are CustomFields; duplicate check reuses existing BM25.
- No core Concept struct modification: all new fields via CustomFields + `pkg/memorymeta` accessor.
- No `okf_context --id` (interface doesn't exist); follow-up via existing `okf tool query`/`okf tool resolve`.

## Compatibility

- All new frontmatter fields optional; existing concepts work unchanged (default governance=context, code_refs=empty).
- Default manifest order unchanged when no new params used. Governance sort (hold→constraint→context, then original order) activates only with `--for_path` or `--governance`.
- Full mode is default and byte/semantic compatible with existing manifest.
- OKF v0.2: governance/code_refs are extension fields (spec §7 open family), stored in CustomFields.

## Real entry points

- CLI: `okf tool manifest --for-path <path> --mode summary --max-tokens N`
- CLI: `okf tool query --for-path <path> --governance hold --memory-check true`
- MCP: existing `okf_manifest`, `okf_query` tools gain new optional params (both modern and legacy eras).
- Service: `pkg/tool.Service.Manifest`, `Service.Query` gain params; new `pkg/memorymeta` accessor package.
- No new CLI subcommands or MCP tools in v1.

## Success criteria (measurable, from real experiments)

- `for_path` precision ≥ 0.9, recall ≥ 0.8 on curated fixture (implementation phase).
- Duplicate detection: golden set ≥40 cases; Precision/Recall/FPR computed from confusion matrix. Pilot (n=10, Jaccard only): TP=0 FP=0 TN=5 FN=5 — BM25 candidate generation expected to improve recall.
- Progressive disclosure: summary/hit/full ID set parity 100%; token reduction measured (summary −61%, hit −55% on 329 concepts).
- Governance: 100% of explicit hold/constraint values surfaced; 0% of unmarked concepts incorrectly classified.
- All existing tests pass; gauntlet green; coverage ≥ 60%.
