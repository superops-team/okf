## Context

OKF 当前已经具备三层基础：

1. **知识表达层**：`Concept` 是 Markdown 文件和 frontmatter 的单元，`KnowledgeBundle` 是知识集合（`/Users/bytedance/workspace/github/okf/pkg/okf/types.go:11`、`/Users/bytedance/workspace/github/okf/pkg/okf/types.go:51`）。
2. **仓库生成层**：`GenerateBundle` 扫描 Git tracked files，生成 file concepts、project overview、relationship graph、code repository 和 relation index（`/Users/bytedance/workspace/github/okf/pkg/git/generator.go:38`、`/Users/bytedance/workspace/github/okf/pkg/git/generator.go:108`、`/Users/bytedance/workspace/github/okf/pkg/git/generator.go:112`）。
3. **查询层**：`pkg/query` 已有 in-memory index，支持 type/tag/resource/title/code language/file path/symbol/relation 维度（`/Users/bytedance/workspace/github/okf/pkg/query/query.go:36`、`/Users/bytedance/workspace/github/okf/pkg/query/query.go:80`）。CLI `search` 当前把 bundle 转为 query bundle 后执行检索（`/Users/bytedance/workspace/github/okf/cmd/okf/main.go:310`、`/Users/bytedance/workspace/github/okf/cmd/okf/main.go:404`）。

这意味着新需求不应该重建一个并行 CodeGraph，也不应该把 OKF 变成 MCP server。推荐把 OKF 升级为“agent 可直接调用的本地知识 package”，再由 Bingo 注册内置 `bingo okf` 子命令：Markdown/Concept 是 durable truth，进程内索引/可选磁盘 cache 是性能层，agent-facing Go API/JSON contract 是集成层。

## Goals / Non-Goals

**Goals:**

- 让当前 agent 能直接调用 OKF 完成仓库知识检索，不依赖 MCP，不要求安装 CodeGraph，也不要求安装独立 `okf` CLI。
- 在 Bingo 中新增内置 `okf` 子命令族，用于管理当前项目知识库。
- 支持动态知识库管理：初始化、显式刷新、可选缓存重建、状态检查、过期检测、安全 generated artifact 清理。
- 支持 agent 任务上下文：按自然语言、文件、符号、包、最近变更、关系邻域查询，并输出可控 token budget 的 context bundle。
- 保持 OKF 的开放格式优势：人类可读、Git 可 diff、cache 可删除重建。
- 保留 CodeGraph 兼容导入作为可选增强，不把 CodeGraph 作为默认运行依赖。

**Non-Goals:**

- 不实现 MCP server 作为第一集成路径。
- 不让 Bingo 通过 shell 调用外部 `okf` 二进制；Bingo 必须调用 OKF Go package。
- 不要求后台 daemon、watch 常驻进程或磁盘 cache；watch/cache 可以作为后续便利能力，但不是查询前置条件。
- 不实现 LSP/SSA 级精确调用图；第一阶段依赖现有 AST/regex/关系索引，richer edges 仍可从兼容导入保留。
- 不把完整源码复制进 `.okf/knowledge`；snippet 由 context builder 按需读取 Git 工作区文件。
- 不支持 Bingo worktree orchestration；V1 明确拒绝 `--worktree`。
- 不引入外部数据库、向量数据库、LLM reranker 或外部服务作为必需依赖。

## Approach Options

### Option A: 继续依赖 MCP CodeGraph，OKF 只做导出层

优点：复用 CodeGraph 已有图查询语义。缺点：不能解决 MCP 不稳定、性能差、额外安装配置的问题。结论：不推荐。

### Option B: OKF 内置长期 daemon + DB，agent 连本地服务

优点：可做高性能常驻索引、watch 和复杂查询。缺点：引入服务生命周期、端口、权限、崩溃恢复和安装复杂度，和“无需 MCP/额外配置”的目标冲突。结论：后续可选，不作为第一版。

### Option C: OKF 原生 package + Bingo 内置子命令 + 可重建本地索引

优点：用户只需要 `bingo` 单 binary，无 MCP、无 server、无额外 `okf` 安装；agent 可通过 Go API 直接调用，用户和非 Go agent 可通过 `bingo okf ... --json` 调用；进程内索引可从 `.okf/knowledge` 重建；与现有 Concept/Bundle、增量状态、query index 自然衔接。缺点：需要在 Bingo 增加编译期 OKF module 依赖，并维护一层 CLI adapter；复杂跨文件语义弱于 LSP/SSA。结论：推荐作为第一版。

## Architecture

```text
Agent / Bingo
  ├─ embedded Go API: tool.Service.Status/Init/Refresh/Query/Context
  └─ built-in CLI: bingo okf <operation> --json
        │
        ▼
Bingo internal adapter    internal/ui/cli/cmd registers `okf`; no external binary
pkg/tool                  service interface, JSON envelope, stable errors
pkg/context or tool ctx   context bundle planning, snippet budget, citation locations
pkg/query                 ranked structured search over concepts/entities/relations
pkg/index (optional V1)   derived cache/fingerprint helpers; not a required source of truth
pkg/git                   Git scan, AST/regex extraction, incremental update, state
pkg/okf + parser          Markdown Concept/Bundle load/save source of truth
        │
        ▼
.okf/
  knowledge/              generated + human concepts, source of truth
  state.json              last_indexed_commit and generator state
  cache/                  optional derived cache; safe to delete
```

Key design choices:

- `pkg/tool` owns the stable service boundary. CLI commands are thin adapters over this service, so Go agents and `bingo okf --json` share behavior.
- `status`、`query`、`context` are read-only by default. They may build in-memory indexes but MUST NOT mutate `.okf/knowledge`、`.okf/state.json` or cache files.
- `init` and `refresh` are the only V1 write operations. `lint` is read-only unless explicitly invoked with a future `--fix`.
- Any cache is derived-only. If cache is missing/corrupt/schema-incompatible, V1 may rebuild in memory or degrade to direct bundle scan with warnings; it MUST NOT require a disk cache to answer queries.
- Bingo MUST depend on OKF at compile time and call package APIs directly. `bingo okf` MUST NOT call `exec.Command("okf", ...)`, probe `$PATH`, or ask users to install another binary.
- V1 rejects Bingo worktree flags for all `okf` subcommands to avoid hidden repository mutations and cross-repo state confusion.

## Tool Contract

V1 exposes five operations. OKF package owns the Go interface; Bingo owns the built-in user-facing command:

1. `status`: returns repo root、knowledge dir、last indexed commit、HEAD、freshness、readiness、concept/entity/relation counts、warnings。
2. `init`: creates or initializes `.okf/knowledge` by running a full generated knowledge build; returns generated counts and state metadata.
3. `refresh`: executes `incremental | full | cache-only` explicitly; `auto` is accepted only if explicitly enabled and MUST NOT be default in V1.
4. `query`: inputs text/filter/ranking/limit; outputs ranked results with concept、entity、relation、locations、score、reason、provenance、freshness。
5. `context`: inputs task/query、focus files/symbols、budget、include policy; outputs an agent context bundle.

### JSON Envelope

All JSON responses use the same top-level envelope:

```json
{
  "schema_version": "okf.tool.v1",
  "operation": "query",
  "ok": true,
  "repo_root": "/repo",
  "knowledge_dir": "/repo/.okf/knowledge",
  "freshness": {
    "head": "abc123",
    "last_indexed_commit": "abc123",
    "stale": false,
    "changed_files": 0
  },
  "warnings": [],
  "result": {}
}
```

Failure responses preserve the envelope and use stable error codes:

```json
{
  "schema_version": "okf.tool.v1",
  "operation": "query",
  "ok": false,
  "repo_root": "/repo",
  "error": {
    "code": "knowledge_not_initialized",
    "message": ".okf/knowledge does not exist",
    "remediation": "Run `bingo okf init --repo /repo` or `bingo okf refresh --mode full`."
  },
  "warnings": []
}
```

Stable V1 error codes:

- `knowledge_not_initialized`
- `not_git_repository`
- `invalid_request`
- `invalid_query`
- `stale_index`
- `refresh_required`
- `cache_rebuild_failed`
- `budget_exceeded`
- `frontmatter_invalid`
- `worktree_not_supported`

Recommended Bingo CLI examples:

```bash
bingo okf status --repo . --json
bingo okf init --repo . --json
bingo okf refresh --repo . --mode incremental --json
bingo okf query --repo . --q "lark adapter message split" --limit 20 --json
bingo okf context --repo . --q "where are lark channel replies formatted" --budget 12000 --json
```

OKF 独立 CLI 可以提供等价 `okf tool ... --json` 用于 OKF 项目自身调试，但 Bingo 的验收标准以 `bingo okf` 为准。

Go API sketch:

```go
svc := tool.NewService(tool.Config{RepoPath: repo})
bundle, err := svc.Context(context.Background(), tool.ContextRequest{
    Query: "where are lark channel replies formatted",
    BudgetTokens: 12000,
    AllowStale: true,
})
```

## Bingo Command Design

Bingo 当前 CLI 使用 Cobra 注册根命令和子命令，根命令在 `/Users/bytedance/workspace/bytedance/bingo/internal/ui/cli/cmd/root.go:269` 创建，并在 `/Users/bytedance/workspace/bytedance/bingo/internal/ui/cli/cmd/root.go:289` 到 `/Users/bytedance/workspace/bytedance/bingo/internal/ui/cli/cmd/root.go:295` 注册 `exec/acp/gateway/doctor/migrate-config/setup/resume`。OKF 子命令应遵循同一模式：新增 `newOKFCmd(opts)`，在 root 注册 `root.AddCommand(newOKFCmd(opts))`，并在命令处理里调用 OKF package service。

Suggested command family:

```text
bingo okf status   # 状态和 freshness；read-only
bingo okf init     # 初始化当前 repo 的 .okf/knowledge；mutating
bingo okf refresh  # incremental/full/cache-only；mutating except cache-only if memory-only
bingo okf query    # 结构化检索；read-only
bingo okf context  # 生成 agent context bundle；read-only
bingo okf lint     # 检查知识库规范；V1 may defer or read-only only
```

Command-level rules:

- `--repo` default is current working directory and resolves to Git root through OKF package logic.
- `--dir` default is `.okf/knowledge` for compatibility with OKF package defaults.
- `--json` MUST emit the same `okf.tool.v1` schema as package service response.
- Human-readable output can be concise, but JSON output is the contract used by agent.
- V1 rejects `--worktree` with `worktree_not_supported`, following Bingo's existing rejection helper style rather than preparing a separate worktree.
- Tests should mirror Bingo's existing command registration tests such as `/Users/bytedance/workspace/bytedance/bingo/internal/ui/cli/cmd/commands_test.go:180`.

## Generated Metadata and Deletion Safety

Generated concepts MUST carry enough metadata to distinguish them from human-authored concepts. V1 minimum frontmatter fields:

```yaml
generated: true
generator: okf.git
generator_version: 1
source_path: internal/gateway/channel/lark/adapter.go
source_kind: file
source_commit: abc123
```

Refresh deletion rules:

- A generated concept may be deleted only when `generated=true`, `generator=okf.git`, and `source_path` belongs to the deleted or regenerated source file set.
- A human-authored concept, or a concept missing generated metadata, MUST NOT be deleted automatically.
- If metadata is ambiguous, refresh MUST keep the file and report a warning.
- Full refresh can replace generated artifacts by first computing the desired generated set, then deleting only previously generated artifacts absent from the desired set.

## Query and Ranking

`query` combines multiple signals rather than only Markdown free text. V1 uses deterministic integer scoring:

| Signal | Score |
| --- | ---: |
| Exact qualified symbol match | +100 |
| Exact file path or package path match | +90 |
| Exact concept title match | +80 |
| Symbol prefix/camel/word match | +60 |
| Relation source/target/kind match | +50 |
| Tag/type/language filter match | +30 |
| Content/description term match | +10 per term, capped at +40 |
| Recent changed-file hint match | +15 |
| Human-authored explanatory concept boost | +10 |
| Generated navigable code concept boost | +5 |
| Stale freshness penalty | -20 |

Result ordering MUST be deterministic for identical index state:

1. score descending
2. exactness bucket descending
3. generated concepts after human-authored concepts when scores tie for explanation queries; generated first for symbol/file queries
4. file path ascending
5. line number ascending
6. title ascending
7. stable concept ID/path ascending

Each result MUST include `score` and `reason` so agents can explain why context was selected.

## Context Bundle Shape

`context` returns a compact bundle optimized for agent prompts:

```json
{
  "schema_version": "okf.tool.v1",
  "operation": "context",
  "ok": true,
  "repo_root": "/repo",
  "knowledge_dir": "/repo/.okf/knowledge",
  "freshness": {"head": "...", "last_indexed_commit": "...", "stale": false},
  "result": {
    "query": "where are lark channel replies formatted",
    "budget": {"requested_tokens": 12000, "estimated_tokens": 8420},
    "items": [
      {
        "kind": "symbol",
        "title": "cmdSearch",
        "location": "cmd/okf/main.go:310-373",
        "reason": "exact symbol and query match",
        "snippet": "...",
        "relations": ["calls query.SearchWithMatches", "reads .okf/knowledge"],
        "provenance": "okf-go-ast"
      }
    ],
    "omitted": [{"reason": "budget", "count": 12}]
  },
  "warnings": []
}
```

Snippets are read on demand from the working tree and bounded by line ranges. Token estimation is deterministic and intentionally approximate: `estimated_tokens = ceil(rune_count / 4)` for V1. The bundle MUST include enough `file_path:line_number` locations for the agent to navigate without another graph service.

## Freshness and Dynamic Update Semantics

- `status` compares `.okf/state.json.last_indexed_commit` with Git HEAD and reports `stale=true` when they differ.
- `query` and `context` default to no mutation. If stale, they return results only when safe to load existing knowledge; response includes freshness warnings and optional remediation.
- A future explicit `--refresh=auto` may run incremental refresh before querying only when changed-file count is under threshold and caller explicitly enables it.
- Any successful mutating `refresh` MUST update state and invalidate/rebuild affected in-memory or disk cache metadata.
- Cache-only rebuild MUST NOT change generated concept files.
- Full rebuild MUST converge with repeated incremental updates except volatile timestamps.

## Error Handling

- Missing `.okf/knowledge`: `query/context` return typed `knowledge_not_initialized` with remediation `bingo okf init` or `bingo okf refresh --mode full`.
- Corrupt cache: rebuild in memory and continue; if rebuild fails, degrade to direct bundle scan with warning.
- Corrupt concept frontmatter: skip invalid concept, include parser warning, and keep process exit non-zero only for strict lint mode.
- Git unavailable or not a repo: return `not_git_repository` with repo path.
- Stale index: return stale warning and remediation unless caller set a stricter freshness policy.
- Worktree flag: return `worktree_not_supported` in V1.

## Layered Delivery Plan

### P0: Contract and Scope Guard

- Define `okf.tool.v1` envelope, request/response types, error codes, no-mutation defaults and generated metadata contract.
- Add golden JSON tests and fixture coverage before wiring Bingo.

### P1: OKF Service Core

- Implement `Status`, `Init`, `Refresh` service methods over existing OKF parser/git/query packages.
- Enforce read/write boundary with tests proving query/context do not create or modify files.

### P2: Deterministic Query

- Implement ranked query with integer scoring, stable tie-breaks, filters and navigable locations.
- Add golden ranking tests and temp Git repo integration tests.

### P3: Context Builder

- Implement snippet extraction, deterministic token estimation, omission accounting and stale warnings.
- Add budget truncation and missing source file tests.

### P4: Bingo Integration

- Add OKF module dependency, Cobra `bingo okf` command, human output and JSON output.
- Add tests proving `bingo okf` does not shell out to external `okf` and rejects worktree flags.

## Testing Strategy

- Contract tests for JSON schemas, stable field names, error codes and backwards-compatible envelope.
- Unit tests for generated metadata deletion safety: generated artifact deletion, human concept preservation, ambiguous metadata warning.
- Unit tests for freshness decisions: fresh, stale, missing state, corrupt cache, schema mismatch, no disk cache.
- Golden tests for deterministic query ranking and tie-break ordering.
- Golden tests for context bundle truncation, estimated token counts, omitted reasons and missing source files.
- Integration tests using temp Git repos: init → query → commit change → explicit incremental refresh → query reflects changed symbol → delete file → generated result disappears while human concept remains.
- Bingo targeted tests: command registration, help output, JSON output, no external `okf` execution, worktree rejection.
- Performance benchmarks: 1k files / 10k symbols query p95, context bundle construction, optional cache rebuild from concepts.

## Development Schedule

Assuming one primary developer and review after each layer:

- Day 1: P0 contract types, generated metadata spec, golden response fixtures.
- Days 2-3: P1 status/init/refresh service and deletion-safety tests.
- Days 4-5: P2 ranked query, filters, deterministic ordering, integration fixtures.
- Day 6: P3 context bundle planner, snippet extraction, token budget tests.
- Days 7-8: P4 Bingo command registration, JSON/human output, no-shell-out tests.
- Day 9: strict OpenSpec validation, `go test ./...`, targeted Bingo tests, benchmark baselines.
- Day 10: real-repo validation against OKF/Bingo repos, MCP CodeGraph replacement workflow latency comparison, docs/readme updates if required.

## Risks / Mitigations

- [Risk] Cache becomes another stale source of truth. → Cache is optional derived-only, schema-versioned and rebuildable; stale cache never overrides `.okf/knowledge` or Git state.
- [Risk] CLI JSON contract churn breaks agent integration. → Version every response with `schema_version: "okf.tool.v1"` and add golden contract tests.
- [Risk] Bingo accidentally depends on external `okf` executable. → Add tests or code review guard that `bingo okf` calls package interfaces and does not invoke `exec.Command("okf", ...)`.
- [Risk] Auto refresh slows or mutates interactive agent turns unexpectedly. → No auto-refresh by default in V1; explicit refresh operations own writes.
- [Risk] Generated cleanup deletes human knowledge. → Require generated metadata and source_path match before deletion; ambiguous files are preserved with warning.
- [Risk] Context bundle over-includes low-value generated docs. → Ranking separates navigation artifacts from explanatory human concepts and enforces budget accounting.
- [Risk] First native analyzer lacks CodeGraph precision. → Preserve CodeGraph-compatible import for richer edges while making it optional.
