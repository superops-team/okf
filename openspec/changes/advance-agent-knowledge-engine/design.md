## Overview

This change deepens OKF from a Git-managed knowledge format into an agent-native knowledge engine. The design keeps OKF's durable source of truth as Markdown/YAML while introducing clearer seams around path resolution, import dispatch, structured query, context packing, and retrieval trace. The implementation must favor small, testable changes over new runtime dependencies: extend current packages first, introduce new packages only when reuse across multiple callers proves the seam is real.

## Review-Driven Adjustments

This design intentionally addresses these risk categories before implementation:

- **Context coherence**: read-only and mutating operations are enumerated once and reused across specs/tasks.
- **Ambiguity reduction**: overlay order, trace default behavior, relation expansion default, and write target semantics are explicit.
- **Minimal implementation**: V1 avoids vector DB, LLM reranker, graph runtime, multi-writer overlay, new daemon, and required disk cache.
- **Compatibility**: legacy `knowledge_dir` remains valid; `ResolveKnowledgeDir(cliDir string)` remains as a compatibility wrapper; JSON envelope changes are additive under `okf.tool.v1`.
- **SDD/TDD fit**: every scenario maps to a unit, CLI, golden, or integration test before implementation work is considered complete.

## Architecture

### Path resolution and overlay

Introduce a single path resolver module that returns both a write target and ordered read paths.

Resolution order for the write target:

1. CLI `--dir` / `-dir`
2. `OKF_KNOWLEDGE_DIR`
3. config file `knowledge_dir`
4. existing local `.okf/knowledge` under the discovered repo root
5. existing local `.okf/knowledge` under current working directory when no repo root is discovered
6. platform default

Read path order SHALL be: resolved write target first, then `knowledge_paths[]` in config order, after de-duplication by canonical absolute path. If config contains `knowledge_dir` but no `knowledge_paths[]`, read paths contain exactly the resolved write target, preserving legacy behavior.

Each loaded concept receives source metadata in memory: `knowledge_path`, `source_rank`, and `source_kind`. The merge layer preserves all concepts by default. For exact generated duplicate identities only, the merged result exposes one primary source by precedence while retaining duplicate source metadata for trace/result metadata; non-generated or title-only duplicates remain separate results.

Read-only path resolution MUST NOT create directories or config files. Mutating resolution MAY create the selected write target. Failure to read the resolved write target is an error for normal operations unless a future explicit overlay-only mode is added; missing optional overlay paths are warnings, while permission denied, non-directory paths, or invalid path syntax are actionable errors.

### Unified import pipeline

`okf add` becomes a thin CLI wrapper over one import dispatcher:

```text
source path
  -> classify file | dir | archive
  -> extract archive when needed
  -> collect markdown candidates
  -> parse and validate OKF frontmatter
  -> compute destination path
  -> apply smart strategy: skip | overwrite | merge | patch
  -> update metadata index
```

The existing `SmartImporter` remains the strategy adapter. It must not bypass OKF concept validation for new files.

Validation semantics are intentionally conservative: if the input is OKF Markdown, it MUST validate before write. If future conversion of non-OKF Markdown is supported, conversion MUST produce valid OKF Markdown before write. Malformed OKF frontmatter is rejected. This aligns with existing file-import requirements while keeping smart merge/patch strategy behavior.

Archive extraction MUST reject absolute paths, parent traversal, symlinks, hardlinks, special files, and entries exceeding configured per-file or total uncompressed size limits. Temporary directories must be cleaned up on success and failure.

### Agent tool CLI

`pkg/tool.Service` remains the canonical interface. `okf tool` is a thin adapter that maps flags to request structs and prints the existing tool envelope, whose `schema_version` SHALL be `okf.tool.v1`. This avoids duplicating service behavior in CLI handlers.

The JSON envelope includes a `mutating` boolean: `true` for `tool init/refresh`, `false` for `tool status/query/context`. Existing top-level fields remain; new fields are additive under `okf.tool.v1`.

### Structured query and context planner

`pkg/tool.Query` reuses `pkg/query` or a shared query engine rather than maintaining a weaker ranking implementation. V1 can keep the existing deterministic `rankConcepts` scoring while replacing candidate filtering with structured query candidates. This reduces churn and avoids rewriting ranking and filtering at the same time.

Minimum V1 structured fields are: `type`, `tag`, `file_path`, `language`, `symbol_kind`, `qualified_name`, `relation_kind`, `start_line`, `end_line`, generated/user-authored provenance, and knowledge path metadata. Relation source/target filters are outside the mandatory V1 field set and must not be exposed until relation metadata is present and covered by tests.

`pkg/tool.Context` consumes structured hits and builds a context plan:

- group by file and symbol
- prefer exact symbol ranges
- expand snippets by deterministic surrounding lines
- merge overlapping ranges in the same file
- expand one relation hop for selected symbols/files only when `include_relations=true`, limited by a small deterministic `max_neighbors_per_hit`
- pack items within budget using deterministic ordering
- return explicit omissions

The source reader SHALL only read files under the resolved repo root or explicitly allowed source roots. Paths outside allowed roots are omitted with warnings. The planner SHALL apply max bytes per file, max candidate items, and max budget limits before reading snippets.

### Retrieval trace

Trace is not a logging side channel. It is a response field that lets an agent understand why a result appeared. Each step is stable and testable, for example:

- `path_resolution`
- `bundle_load`
- `filter_apply`
- `score_candidate`
- `tie_break`
- `relation_expand`
- `snippet_pack`
- `budget_omit`
- `freshness_warning`

Trace output is disabled by default and enabled only by `include_trace=true` or `--include-trace`. The trace avoids raw source content duplication and secrets. It uses repo-relative source paths and stable knowledge path labels in V1; absolute path trace fields are deferred to a future explicit local-navigation mode.

V1 `TraceStep` remains small: `type`, `message`, `refs[]`, `counts{}`, `score_delta`, `omission_reason`, and `warnings[]`. Arbitrary nested `inputs` are not part of the stable schema.

## Data Flow

### Query

```text
Tool CLI / Go API
  -> path resolver
  -> overlay bundle loader
  -> shared query engine
  -> deterministic ranker
  -> trace builder
  -> ToolEnvelope<QueryResult>
```

### Context

```text
Tool CLI / Go API
  -> path resolver
  -> overlay bundle loader
  -> shared query engine
  -> context planner
  -> working tree source reader
  -> budget packer
  -> trace builder
  -> ToolEnvelope<ContextResult>
```

### Import

```text
okf add
  -> path resolver write target
  -> import dispatcher
  -> validation
  -> smart strategy adapter
  -> metadata index save
```

## Error Handling

- Missing config file is not an error.
- Invalid configured paths return actionable errors with the config source.
- Missing optional overlay paths are warnings. Permission denied, non-directory paths, invalid path syntax, or failure to read the resolved write target are errors unless explicitly configured otherwise in a future change.
- Import validation errors are per-file errors; directory import continues and exits non-zero if any file failed.
- `query/context` never auto-refresh or write knowledge by default.
- Trace builder validates each `TraceStep` against the schema. Invalid steps are omitted, counted in `trace_warnings`, and covered by tests; production query/context must not fail solely due to trace validation errors.

## Backward Compatibility

- Existing configs with only `knowledge_dir` continue to work; `knowledge_paths[]` defaults to empty.
- Existing `ResolveKnowledgeDir(cliDir string)` remains as a compatibility wrapper over the new resolver and preserves directory creation behavior for mutating legacy callers.
- Existing `okf.tool.v1` top-level envelope fields remain; new fields are optional and additive.
- Existing single-path query ordering remains unchanged except where structured metadata produces a strictly higher exactness score; tests must cover legacy single-path ordering.
- Context response keeps existing snippet/item fields and adds plan/trace metadata as optional fields rather than replacing the shape.
- Import behavior becomes stricter for malformed OKF Markdown, consistent with existing file-import requirements. If a legacy caller relied on invalid Markdown pass-through, it must opt into a future conversion mode rather than bypass validation.

## SDD / TDD Landing Model

- SDD: every requirement scenario in `specs/*/spec.md` maps to at least one named test or golden fixture before implementation starts.
- TDD: each milestone starts with failing tests for the highest-risk contract, then implementation, then refactor.
- M1 tests focus on resolver precedence and read-only no-write behavior.
- M2 tests focus on import validation, per-file errors, archive safety, dry-run/detect-only no-write behavior.
- M3 tests focus on JSON envelope stability, structured filters, read-only tool query/context/status.
- M4 tests focus on symbol range extraction, same-file range merge, deterministic omissions, trace golden output.

## Testing Strategy

- Unit tests for path precedence, config file loading, source annotation, and overlay deduplication.
- CLI tests for `okf config get/list/set`, `okf add`, and `okf tool ... --json`.
- Golden JSON tests for tool envelope, query/context results, warnings, errors, and trace steps.
- Integration tests with temp Git repositories covering generated knowledge, user-authored knowledge, overlay paths, symbol search, relation expansion, and stale freshness.
- Regression tests proving read-only operations do not modify knowledge dirs, state files, metadata index, or cache files.

## Milestones and Schedule

### M1: Path Resolver and Basic Overlay (1.5 days)

- Add config-file-aware resolver, `knowledge_paths[]`, source reporting, read-only mode, and compatibility wrapper.
- Wire config/add/tool service callers to the resolver without changing old single-dir behavior.

Primary implementation files:

- `pkg/okf/config.go`: extend `Config`, keep `LoadConfig`/`SaveConfig`, preserve `ResolveKnowledgeDir` wrapper.
- `pkg/okf/paths.go`: add resolver types only if keeping `config.go` small; otherwise keep resolver in `config.go` for V1.
- `cmd/okf/cmd_config.go`: report source metadata and `knowledge_paths[]`.
- `pkg/tool/service.go`: add config path/write-dir/read-path awareness without changing existing top-level envelope fields.

TDD slice:

1. RED: resolver uses config `knowledge_dir` when CLI/env are absent.
2. GREEN: minimal config read in resolver.
3. RED: read-only resolver does not create default dirs.
4. GREEN: split read-only and ensure-write behavior.
5. RED: legacy single-dir config produces one read path.
6. GREEN: add read path derivation and de-duplication.

### M2: Unified Smart Import Dispatcher (2 days)

- Route `okf add` through a dispatcher that classifies file/dir/archive, validates candidates, applies smart strategy, and persists metadata only after actual writes.
- Reuse validation helper from watch daemon without introducing a new daemon.

Primary implementation files:

- `pkg/okf/import.go`: add dispatcher entrypoint and archive candidate collection glue.
- `pkg/okf/smart_import.go`: add validation safety net before new-file writes.
- `cmd/okf/cmd_add.go`: replace manual candidate loop with dispatcher call.
- `pkg/okf/watch_daemon.go`: reuse validation helper before single-file smart import.

TDD slice:

1. RED: invalid OKF Markdown passed to smart import is rejected before write.
2. GREEN: add validation helper in dispatcher and safety net in `SmartImporter`.
3. RED: archive import with traversal entry cannot write outside temp root.
4. GREEN: harden extraction checks.
5. RED: detect-only leaves target and metadata unchanged.
6. GREEN: persist metadata only after non-dry-run writes.

### M3: Tool CLI and Structured Query (2 days)

- Add `okf tool` JSON CLI as thin wrapper over `pkg/tool.Service`.
- Extend query request/result fields and reuse structured candidate filtering from `pkg/query` while keeping deterministic ranking stable.

Primary implementation files:

- `cmd/okf/main.go`: register `tool` command.
- `cmd/okf/cmd_tool.go`: thin flag-to-service adapter with JSON output only in V1.
- `pkg/tool/service.go`: add `mutating`, request/result fields, and structured candidate filtering.
- `pkg/query/query.go`: add missing metadata passthrough only when required by service tests.

TDD slice:

1. RED: `okf tool status --json` returns `okf.tool.v1` envelope with `mutating:false`.
2. GREEN: add command wrapper over existing service.
3. RED: `okf tool query --language go --qualified-name X` filters structured candidates.
4. GREEN: wire tool query through shared query filtering, keep current scoring.
5. RED: query/status/context do not modify `.okf` files.
6. GREEN: ensure service uses read-only resolver and no refresh path.

### M4: Context Planner and Compact Trace (2.5 days)

- Prefer symbol ranges, merge same-file ranges, pack snippets deterministically, return omissions, and add optional compact trace output.

Primary implementation files:

- `pkg/tool/service.go`: add private planner helpers or split to `pkg/tool/context_plan.go` only if file size blocks readability.
- `pkg/tool/service_test.go`: golden-style tests for context planning and trace.
- `cmd/okf/cmd_tool.go`: expose `--include-relations` and `--include-trace` flags.

TDD slice:

1. RED: context with symbol line range chooses that range instead of first keyword line.
2. GREEN: add planned range extraction.
3. RED: overlapping same-file ranges become one context item.
4. GREEN: add deterministic range merge.
5. RED: over-budget context reports stable omissions.
6. GREEN: add deterministic budget packing.
7. RED: `include_trace=true` emits stable compact trace; default omits trace.
8. GREEN: add trace builder and golden assertions.

## Risk and Mitigation

- **Path resolver creates files during read-only operations**: split read-only and mutating resolution modes; test file tree before/after.
- **Overlay hides broken primary knowledge dir**: treat resolved write target read failure as error in normal mode.
- **Import validation breaks pass-through Markdown**: document V1 accepts valid OKF Markdown; future conversion mode must produce valid OKF Markdown before write.
- **Archive extraction escapes temp dir or exhausts resources**: enforce canonical target path, reject links/special files, and cap entry sizes.
- **Query ranking changes unexpectedly**: preserve old ranking for single-path inputs except explicit exactness improvements; golden tests lock ordering.
- **Context planner reads unsafe paths**: restrict source reads to repo root or explicit allowed roots.
- **Trace schema grows too quickly**: keep V1 schema compact and optional.

## Non-goals

- No required vector database or LLM reranker in this change.
- No OpenViking compatibility layer.
- No multi-writer synchronization across knowledge paths.
- No full LSP/SSA call graph requirement.
- No new daemon requirement beyond reusing existing watch daemon import semantics.
- No graph database or complete call graph runtime.
