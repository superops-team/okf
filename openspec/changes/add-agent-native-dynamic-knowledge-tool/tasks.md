## 0. Scope locks and design validation

- [ ] 0.1 Lock V1 boundaries in code comments/tests: no MCP, no external `okf` binary, no daemon, no required disk cache, no worktree support, no auto-refresh by default.
- [ ] 0.2 Define read/write operation classification: `status/query/context/lint` read-only; `init/refresh` mutating; future `lint --fix` mutating.
- [ ] 0.3 Define generated metadata contract: `generated`, `generator`, `generator_version`, `source_path`, `source_kind`, `source_commit`.
- [ ] 0.4 Validate that every requirement maps to a testable behavior and remove ambiguous “should” wording from V1 acceptance paths.

## 1. Contract and public API

- [ ] 1.1 Define `ToolEnvelope`, `ToolError`, `Freshness`, `StatusRequest`, `InitRequest`, `RefreshRequest`, `QueryRequest`, `ContextRequest` and operation-specific response payloads.
- [ ] 1.2 Set `schema_version` to the fixed value `okf.tool.v1` for all agent-facing JSON responses.
- [ ] 1.3 Define stable error codes: `knowledge_not_initialized`, `not_git_repository`, `invalid_request`, `invalid_query`, `stale_index`, `refresh_required`, `cache_rebuild_failed`, `budget_exceeded`, `frontmatter_invalid`, `worktree_not_supported`.
- [ ] 1.4 Add golden JSON tests for successful and failed `status`, `init`, `refresh`, `query` and `context` responses.
- [ ] 1.5 Add Go service interface for embedded agent usage, with CLI/Bingo as thin wrappers over the same service.
- [ ] 1.6 Add tests proving read-only operations do not create, remove or modify `.okf/knowledge`, `.okf/state.json` or cache files.

## 2. Dynamic index, freshness and generated safety

- [ ] 2.1 Implement `status` freshness check comparing Git HEAD and `.okf/state.json.last_indexed_commit`, returning readiness, counts and warnings.
- [ ] 2.2 Implement `init` as an explicit full generated knowledge build for `.okf/knowledge` with generated metadata.
- [ ] 2.3 Implement `refresh` modes: `incremental`, `full`, `cache-only`; keep `auto` disabled by default or reject unless explicitly enabled.
- [ ] 2.4 Implement safe generated artifact deletion: delete only records with `generated=true`, `generator=okf.git` and matching `source_path`; preserve human or ambiguous concepts.
- [ ] 2.5 Add optional derived cache metadata if disk cache is implemented: schema version, source knowledge fingerprint, last indexed commit, generated concept count, relation count.
- [ ] 2.6 Implement cache load/rebuild fallback from `.okf/knowledge` or direct bundle scanning; missing disk cache must not fail `query/context`.
- [ ] 2.7 Add tests for fresh/stale state, missing state, corrupt cache, cache schema mismatch, no-cache path, file deletion and human concept preservation.

## 3. Query service

- [ ] 3.1 Implement ranked structured query over text, type, tags, file path, language, symbol kind, qualified name, relation kind and recent-change hints.
- [ ] 3.2 Implement deterministic integer scoring and fixed tie-break order: score, exactness, source preference, file path, line number, title, concept path/ID.
- [ ] 3.3 Return source navigation for every code hit using `file_path:start_line-end_line` when available.
- [ ] 3.4 Return `score`, `reason`, `provenance`, `freshness` and warnings for each result set.
- [ ] 3.5 Add golden ranking tests for exact symbol, file path, title/content, relation and tie-break cases.
- [ ] 3.6 Add query integration tests against generated temp Git repositories.

## 4. Context bundle service

- [ ] 4.1 Implement context planner that groups hits by file, symbol and relation neighborhood.
- [ ] 4.2 Implement snippet extraction from working tree with line-range expansion and missing-file warnings.
- [ ] 4.3 Implement deterministic token estimation as `ceil(rune_count / 4)` for V1.
- [ ] 4.4 Implement budget truncation with explicit omitted-item reporting and deterministic lower-ranked item removal.
- [ ] 4.5 Add golden context bundle tests for natural-language query, symbol focus, file focus, recent changes, stale freshness and deleted-file handling.

## 5. Bingo CLI and agent integration

- [ ] 5.1 Add OKF as a Bingo compile-time package dependency; built Bingo binary must include OKF capability without requiring a separate `okf` executable.
- [ ] 5.2 Document dependency strategy: local `replace` allowed only for development; release must use a reproducible module version or internal mirror.
- [ ] 5.3 Add `bingo okf` root command registration in Bingo's Cobra CLI.
- [ ] 5.4 Add `bingo okf status --json` and concise human output.
- [ ] 5.5 Add `bingo okf init --repo ... --dir ... --json`.
- [ ] 5.6 Add `bingo okf refresh --mode <incremental|full|cache-only> --json`; reject or explicitly gate `auto`.
- [ ] 5.7 Add `bingo okf query --q ... --json` with filters and limit.
- [ ] 5.8 Add `bingo okf context --q ... --budget ... --json`.
- [ ] 5.9 Add `bingo okf lint --json` as read-only, or explicitly document why lint is deferred from V1.
- [ ] 5.10 Reject Bingo `--worktree` for `okf` commands with `worktree_not_supported`.
- [ ] 5.11 Add command registration and behavior tests in Bingo; tests must prove the command uses OKF package seams rather than shelling out to `okf`.
- [ ] 5.12 Add an integration example for current agent using embedded Go API; keep `bingo okf ... --json` fallback documented in tests or examples.

## 6. Validation and performance

- [ ] 6.1 Run `openspec validate add-agent-native-dynamic-knowledge-tool --strict` and fix all issues.
- [ ] 6.2 Run OKF `go test ./...` after P1/P2/P3 changes.
- [ ] 6.3 Run Bingo targeted tests for CLI registration and OKF command behavior.
- [ ] 6.4 Run `go test -race ./...` after cache/index concurrency is introduced; skip race run only if implementation stays single-threaded and document why.
- [ ] 6.5 Add benchmarks for cache/direct bundle rebuild, ranked query and context bundle construction.
- [ ] 6.6 Validate against one real repo with existing `.okf/knowledge` and compare MCP CodeGraph replacement workflow latency manually.

## 7. Suggested development schedule

- [ ] 7.1 Day 1: contract types, fixed JSON envelope, generated metadata rules and golden response fixtures.
- [ ] 7.2 Days 2-3: status/init/refresh service, freshness checks and generated deletion-safety tests.
- [ ] 7.3 Days 4-5: deterministic query scoring, filters, tie-breaks and temp Git repo integration tests.
- [ ] 7.4 Day 6: context planner, snippet extraction, token budget accounting and missing-file tests.
- [ ] 7.5 Days 7-8: Bingo `okf` commands, JSON/human output, no-shell-out tests and worktree rejection.
- [ ] 7.6 Day 9: OpenSpec strict validation, OKF tests, Bingo targeted tests and benchmark baselines.
- [ ] 7.7 Day 10: real-repo validation, latency comparison and final docs/readme updates if required.
