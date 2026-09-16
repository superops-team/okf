# Release Notes — add-governed-agent-memory

## New capabilities

### Governance (constraint / hold / context)

- Each concept may declare `governance` in frontmatter (stored in `Concept.CustomFields`).
- Levels: `constraint` (mandatory), `hold` (advisory freeze; agent SHOULD request user confirmation; server never blocks), `context` (informative; default).
- Unknown values degrade to `context` + warning in non-strict mode; `okf lint --strict` rejects them.
- `pkg/memorymeta` provides typed accessor `Governance()`, `SetGovernance()`, and `Validate()`.

### Code references (`code_refs`) and `--for-path`

- `code_refs` is a list of repo-relative glob patterns in frontmatter.
- Bounds: max 16 patterns, 256 bytes each, 32 path segments, 2 `**` operators (each matching at most 8 segments).
- Rejects absolute paths, `..` traversal, NUL, and backslash.
- `okf tool manifest --for-path <path>` performs lexical matching (file need not exist).
- `--stale-refs` is the only filesystem scan (max 50k entries); symlink escape/unreadable returns `incomplete` + warnings.

### Progressive disclosure

- `okf tool manifest --mode summary|hit|full` (default `full`, backward compatible).
- `--max-tokens` budget: filter → sort → offset → limit → budget; item tokens = Go `encoding/json` bytes / 4, no envelope.
- Returns `next_offset`, `omitted_count` (budget_omitted vs total_remaining), `truncated`, `budget_too_small` with `min_required_tokens`.

### Memory check (read-only duplicate detection)

- `okf tool query -q <content> --memory-check` returns `MemoryCheckResult` (`no_similar` | `possible_duplicate` with top-3 candidates + Jaccard scores).
- BM25 top-10 over ≤1000 durable concepts (note/event/feedback default; `--type` restricts), then deterministic Unicode token Jaccard.
- Read-only; no writes, no blocking, no `allow_duplicate`, no conflict classification.

### Context refs

- `okf tool context --refs <id1,id2>` reads full body by stable concept ID.
- Either `query` or `refs` must be non-empty; both share `--budget-tokens`.

### Agent Skill updates

- W01: recommends `okf_manifest --mode summary` for fast overview.
- W02: check `--for-path` holds before modifying code; request user confirmation on `hold`.
- W03: use `--for-path` for code-related discovery; `okf_context --refs` for full body.
- W06: call `okf_query --memory-check` before writing; review candidates then write explicitly.

## Compatibility

- Core `Concept` struct unchanged; all extension data in `CustomFields`.
- No second index or second fact source.
- Default manifest shape and order unchanged (S05/S30 regression tests).
- Modern 11 tools / legacy 20 tools both gain optional params; no tool removed.
- CLI flags use hyphens; MCP/JSON fields use underscores.

## Behavior changes

- `ManifestItem` fields that were previously always serialized (`identity_state`, `path`, `status`, `trust_tier`, `stale`, `source_count`, `file_size_bytes`, `estimated_tokens`, `estimate_kind`) now use `omitempty`. For full mode with populated concepts this is byte-identical; for empty/zero values the field is omitted. This enables progressive disclosure token reduction.
