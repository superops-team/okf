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

Optional frontmatter `governance` (constraint|hold|context), stored in CustomFields, accessed via `pkg/memorymeta`.

- **Default**: all concepts default to `context`. **No path-based inference** (no `convention/` → constraint). Only explicit values take effect.
- Unknown values: non-strict → `context` + warning; strict → error.
- Hold is advisory only: tools return `governance_warning`; OKF server never blocks writes. Agent Skill SHOULD request user confirmation before modifying hold-governed code.
- Default manifest order unchanged when no new params; governance sort (hold→constraint→context, then original order) activates only with `--for_path` or `--governance`.

### 2. Code-to-knowledge binding (code_refs + for_path)

Optional frontmatter `code_refs` (list of repo-relative paths/globs). `for_path` parameter on `okf tool manifest`/`okf tool query` matches input paths lexically.

- **Lexical matching only**: canonicalize repo-relative path; reject absolute/`..`/NUL/backslash. No EvalSymlinks, no file-existence requirement (supports not-yet-created/renamed targets).
- Pattern bounds: max 16 patterns/concept, max 256 bytes/pattern, max 32 path segments/pattern, max 2 `**` operators, each `**` matches max 8 segments.
- `stale-refs` (advisory): the only FS scanner; fail-closed on symlink escape/unreadable (returns `incomplete: true` + warnings, never silently empty). Scan bound 50,000 entries.

### 3. Search-before-write (read-only memory_check via okf_query)

Extend existing `okf tool query -q <content> --memory-check` with a read-only `memory_check` bool. Returns a dedicated `MemoryCheckResult` envelope (skips normal Query ranking output). MCP: `query` field + `memory_check: true`.

- **No `allow_duplicate`/override** (zombie when advisory never blocks). **No `possible_conflict`** (cannot infer semantic conflict without LLM; false capability).
- **No write blocking**: memory_check is read-only. Agent Skill W06 instructs: first check, then explicitly write via MCP `okf_note`/`okf_log`/`okf_feedback` (no CLI for durable writes).
- Candidate source: durable types only (note/event/feedback) by default; `--types` can extend. Avoids code_file concept pollution.
- BM25: build per-call on bounded durable candidate set (Service has no BM25 cache); budget for 1000 durable concepts. BM25 top-10 candidates → normalized token Jaccard [0,1] threshold.
- Tokenization: Unicode lowercase, code identifiers (`_` `-`), Chinese per-character, set Jaccard, empty set=0, body truncate 500 chars, deterministic tie-break.
- Threshold: candidate default 0.20 (pilot-validated on 5 durable seeds: Precision=1.00 Recall=1.00 FPR=0.00, n=10); golden set ≥40 calibrates final default. Acceptance gate: Precision≥0.85, Recall≥0.70, FPR≤0.15.
- **No audit trace persistence**: memory_check is read-only; result used by caller per-call. No client-supplied metadata stored as "audit" (untrusted, no audit value).

### 4. Progressive disclosure (manifest modes + context refs)

Extend `okf tool manifest` with `--mode summary|hit|full`:
- `summary`: okf_id, title, type, governance, 1-line description (≤80 chars)
- `hit`: summary + tags, code_refs, status, stale_after
- `full`: existing full metadata (default, backward compatible)

Real measurement on 329 concepts: full=36,479 tokens, hit=16,311 (−55%), summary=14,337 (−61%). Token estimate = canonical JSON item bytes/4 (not file_bytes/4).

- **ID parity**: without `--max-tokens`, summary/hit/full return identical concept ID sets and order.
- `--max_tokens` pipeline: filter → stable sort → offset → limit → token budget. Returns `next_offset`, `omitted_count` (budget_omitted vs total_remaining distinguished), `truncated`. Budget < first item → `budget_too_small` error with `min_required_tokens` (dynamic, = first eligible item estimate).
- **Follow-up for full body**: add `refs` parameter to existing `okf tool context` (query OR refs at least one; resolves stable refs to concept body, subject to budget). This extends existing ContextRequest, not a new tool.

## Non-goals

- No database, external LLM, cloud service, or new MCP server/protocol.
- No AAG or new instruction syntax.
- No mandatory write blocking, auto-delete, auto-merge, or auto-archive.
- No second index or second fact source: governance/code_refs are CustomFields; duplicate check builds BM25 per-call on durable set.
- No core Concept struct modification: all new fields via CustomFields + `pkg/memorymeta`.
- No audit trace persistence or client-supplied "audit" metadata.
- No CLI for durable writes (MCP tools/Service only).

## Compatibility

- All new frontmatter fields optional; existing concepts work unchanged.
- Default manifest order unchanged; full mode default and byte/semantic compatible.
- `okf tool context` gains optional `refs`; existing `query`-only calls unchanged.
- OKF v0.2: governance/code_refs are extension fields (spec §7 open family), stored in CustomFields.

## Real entry points (verified against current code)

- CLI: `okf tool manifest --for-path <path> --mode summary --max-tokens N`
- CLI: `okf tool query -q <content> --types note,event --memory-check`
- CLI: `okf tool context -q <query>` or `okf tool context --refs okf_abc...,okf_def...`
- CLI: `okf lint --strict` (validation)
- MCP (both eras): `okf_manifest`, `okf_query`, `okf_context` gain optional params; `okf_note`/`okf_log`/`okf_feedback` unchanged (always write).
- Service: `pkg/tool.Service.Manifest`, `Service.Query`, `Service.Context` gain params; new `pkg/memorymeta` accessor; `pkg/manifest` for manifest projection.
- No new CLI subcommands or MCP tools in v1.

## Success criteria (measurable)

- `for_path` precision ≥ 0.9, recall ≥ 0.8 on curated fixture.
- Duplicate detection (golden ≥40): Precision≥0.85, Recall≥0.70, FPR≤0.15. Pilot (n=10, 5 durable seeds, threshold 0.20): Precision=1.00, Recall=1.00, FPR=0.00.
- Progressive disclosure: 329-concept corpus token reduction summary≥50%, hit≥40% (regression fixture); ID parity 100% without max_tokens.
- Governance: 100% explicit hold/constraint surfaced; 0% unmarked misclassified.
- All existing tests pass; gauntlet green; coverage ≥ 60%.
