## 0. Review gates and scope locks

- [x] 0.1 Confirm V1 boundaries in tests/comments: deterministic-first, no required vector DB, no LLM reranker, no new daemon, no external service, no auto-refresh by default.
- [x] 0.2 Define operation mutability in one shared table or constants: `status/query/context/config get/config list/tool status/tool query/tool context` read-only; `init/refresh/add/config set/tool init/tool refresh` mutating.
- [x] 0.3 Define stable envelope compatibility: `schema_version`, top-level fields, error code names, and documented result field semantics are backward-compatible under `okf.tool.v1`; new fields are optional.
- [x] 0.4 Map every OpenSpec scenario in this change to a unit, CLI, golden, or integration test name before implementation starts.
- [x] 0.5 Follow TDD for production code: write the failing test first, verify it fails for the expected reason, implement the smallest code change, then refactor only after green.

## M1. Path resolver and basic overlay read model

- [x] 1.1 Extend config model with optional `knowledge_paths[]` while preserving existing `knowledge_dir` behavior.
- [x] 1.2 Implement `ResolveKnowledgePaths` with options for CLI dir, env, config path, repo root, working dir, read-only mode, and ensure-write mode.
- [x] 1.3 Preserve `ResolveKnowledgeDir(cliDir string)` as a compatibility wrapper over the new resolver for existing mutating callers.
- [x] 1.4 Make config-file `knowledge_dir` participate in precedence: CLI, env, config, repo-root `.okf/knowledge`, cwd `.okf/knowledge` when no repo root exists, platform default.
- [x] 1.5 Ensure read-only resolution does not create config files, platform default dirs, knowledge dirs, metadata files, state files, or cache files.
- [x] 1.6 Implement read path order: resolved write target first, then `knowledge_paths[]` in config order, de-duplicated by canonical absolute path.
- [x] 1.7 Add in-memory source annotation for loaded concepts when overlay loading is used: knowledge path label, source kind, source rank, and duplicate source metadata.
- [x] 1.8 Update `config get/list` to report effective value and source for `knowledge_dir`; report `knowledge_paths[]` exactly as configured plus resolved read path summary when requested by JSON or verbose output.
- [x] 1.9 Tests: missing config, invalid config, CLI/env/config/local/default precedence, repo-root vs cwd local lookup, read-only no-write, overlay ordering, duplicate handling, and legacy single-dir behavior.
- [x] 1.10 File-level plan: update `pkg/okf/config.go`; add `pkg/okf/paths.go` only if resolver code would make `config.go` hard to read; update `cmd/okf/cmd_config.go`; update `pkg/tool/service.go` resolver usage.
- [x] 1.11 TDD order: first add failing tests in `pkg/okf/config_test.go`, then CLI source-reporting tests in `cmd/okf`, then service read-only resolver tests in `pkg/tool`.

## M2. Unified smart import dispatcher

- [x] 2.1 Introduce an import dispatcher that classifies source as file, directory, or archive before invoking smart strategy logic.
- [x] 2.2 Route `okf add` through the dispatcher instead of bypassing archive extraction and validation paths.
- [x] 2.3 Validate every OKF Markdown candidate before write; malformed OKF frontmatter MUST be rejected with per-file errors.
- [x] 2.4 Keep smart import as strategy adapter for skip, overwrite, merge, and patch behavior after validation.
- [x] 2.5 Ensure directory/archive import continues after per-file validation errors and exits non-zero if any file failed.
- [x] 2.6 Ensure `--detect-only` and `--dry-run` do not write knowledge files or persist metadata index changes.
- [x] 2.7 Harden archive extraction: reject absolute paths, parent traversal, symlinks, hardlinks, special files, and entries exceeding configured per-file or total uncompressed size limits.
- [x] 2.8 Reuse the same validation helper in watch daemon imports; full dispatcher reuse is optional in V1 if the same validation/strategy semantics are enforced.
- [x] 2.9 Tests: file, directory, archive, invalid frontmatter, mixed valid/invalid directory, path traversal, symlink/hardlink archive entries, dry-run, force, merge, patch, detect-only, watch validation, and metadata index persistence.
- [x] 2.10 File-level plan: implement dispatcher in `pkg/okf/import.go`; add validation safety net in `pkg/okf/smart_import.go`; make `cmd/okf/cmd_add.go` a thin dispatcher wrapper; share validation helper with `pkg/okf/watch_daemon.go`.
- [x] 2.11 TDD order: invalid Markdown no-write, archive traversal no-write, detect-only metadata no-write, mixed directory partial success, watch invalid file no-write.

## M3. Agent tool CLI and structured query

- [x] 3.1 Add `okf tool` root command with `status`, `init`, `refresh`, `query`, and `context` subcommands.
- [x] 3.2 Make every `okf tool ... --json` command call `pkg/tool.Service` directly and print the stable `okf.tool.v1` envelope.
- [x] 3.3 Add `mutating` boolean to tool envelopes or operation metadata: true for tool init/refresh, false for tool status/query/context.
- [x] 3.4 Add V1 query flags: `--type`, `--tag`, `--file-path`, `--language`, `--symbol-kind`, `--qualified-name`, `--relation-kind`, `--limit`, and `--include-trace`.
- [x] 3.5 Add relation source/target flags only if relation metadata is present in the query model and covered by golden tests.
- [x] 3.6 Refactor `pkg/tool.Query` to reuse `pkg/query` or a shared query engine for structured candidate filtering while preserving current deterministic ranking for existing single-path, no-new-exactness queries.
- [x] 3.7 Extend `QueryRequest` and `QueryHit` additively with language, qualified name, knowledge path, source rank, and trace fields; avoid removing existing fields.
- [x] 3.8 Return source navigation as `file_path:start_line-end_line` for symbol and code hits whenever line ranges are available.
- [x] 3.9 Tests: tool JSON success/error envelope, invalid query, missing knowledge, stale freshness, structured filters, exact symbol, file path, language, relation kind, overlay source priority, tie-break stability, and read-only no-write.
- [x] 3.10 File-level plan: register `tool` in `cmd/okf/main.go`; add `cmd/okf/cmd_tool.go`; extend `pkg/tool/service.go`; update `pkg/query/query.go` only for missing metadata needed by service tests.
- [x] 3.11 TDD order: CLI JSON envelope, mutating flag, invalid query error envelope, structured filter candidate set, read-only no-write, single-path ordering compatibility.

## M4. Context planner and compact retrieval trace

- [x] 4.1 Add `ContextRequest` flags additively: `include_relations`, `include_trace`, and explicit budget tokens.
- [x] 4.2 Implement context planning over structured hits: group by source file and symbol, choose primary hits before optional neighbor expansion.
- [x] 4.3 Prefer symbol line ranges over first keyword match when extracting snippets.
- [x] 4.4 Expand snippets by deterministic surrounding lines and merge overlapping or adjacent ranges in the same file.
- [x] 4.5 Keep relation expansion disabled by default; when `include_relations=true`, limit expansion to one hop and a small deterministic `max_neighbors_per_hit`.
- [x] 4.6 Implement deterministic budget packing with explicit omissions for lower-ranked items, missing files, empty snippets, duplicate ranges, unsafe paths, and budget overflow.
- [x] 4.7 Restrict source reads to repo root or explicitly allowed source roots; omit and warn for paths outside allowed roots.
- [x] 4.8 Define compact `TraceStep` schema with type, message, refs, counts, score delta, omission reason, and warnings; do not include arbitrary raw inputs or full source content.
- [x] 4.9 Keep trace output disabled by default and enabled only by `include_trace=true` / `--include-trace`.
- [x] 4.10 Tests: symbol focus, file focus, same-file merge, relation expansion opt-in, missing source file, unsafe source path, stale freshness, budget truncation, query trace golden, context trace golden, and trace schema stability.
- [x] 4.11 File-level plan: implement private planner helpers in `pkg/tool/service.go` first; split to `pkg/tool/context_plan.go` only after tests pass and file readability requires it; expose flags in `cmd/okf/cmd_tool.go`.
- [x] 4.12 TDD order: symbol range preference, same-file merge, budget omissions, unsafe path omission, default no trace, include-trace golden, include-relations opt-in.

## 5. Compatibility and regression validation

- [x] 5.1 Run `openspec validate advance-agent-knowledge-engine --strict` and fix all issues.
- [x] 5.2 Run `go test ./pkg/okf ./pkg/query ./pkg/tool ./cmd/okf` after each milestone.
- [x] 5.3 Run `go test ./...` after M2, M3, and M4 complete.
- [x] 5.4 Add a test proving legacy single `knowledge_dir` configs still produce exactly one read path and old write target behavior.
- [x] 5.5 Add a test proving existing `okf.tool.v1` top-level fields remain present after new optional fields are added.
- [x] 5.6 Add a test proving single-path query ordering remains unchanged except for documented exactness improvements.
- [x] 5.7 Validate against a temp or fixture repo with generated knowledge plus a user overlay knowledge path.
- [x] 5.8 Record benchmark or test log for `okf tool query` and `okf tool context` on a named fixture; fail only on an agreed threshold in a future performance change.

## 6. Suggested development schedule

- [x] 6.1 PR 1 / Day 1-1.5: M1 path resolver, config source reporting, read-only no-write tests.
- [x] 6.2 PR 2 / Days 2-3: M2 import dispatcher, validation, archive hardening, metadata no-write tests.
- [x] 6.3 PR 3 / Days 4-5: M3 tool CLI, JSON golden tests, structured query candidate filtering.
- [x] 6.4 PR 4 / Days 6-8: M4 context planner, opt-in relation expansion, compact trace, golden tests.
- [x] 6.5 Day 9: full regression, real/fixture repo validation, OpenSpec strict validation, final docs/readme update only if user-facing command behavior changed.
