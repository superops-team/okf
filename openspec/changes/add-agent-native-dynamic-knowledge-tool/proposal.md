## Why

当前 OKF 已经是面向 AI Agent 的项目级知识库：`Concept`/`KnowledgeBundle` 用 Markdown + YAML frontmatter 表达知识（`/Users/bytedance/workspace/github/okf/pkg/okf/types.go:11`、`/Users/bytedance/workspace/github/okf/pkg/okf/types.go:51`），CLI 能从 Git 仓库生成、更新、lint、show、search（`/Users/bytedance/workspace/github/okf/cmd/okf/main.go:20`）。近期 changes 还补齐了 Go AST 符号、增量状态和 CodeGraph 兼容模型（`/Users/bytedance/workspace/github/okf/pkg/git/code_dimension.go:31`、`/Users/bytedance/workspace/github/okf/pkg/git/state.go:16`）。

但如果当前 agent 仍依赖 MCP 里的 CodeGraph 做仓库知识检索，会遇到三个问题：

- 稳定性：MCP server、插件安装和外部进程链路会成为检索路径上的额外故障点。
- 性能：一次问答需要跨 MCP 边界请求 CodeGraph，再回到 agent，容易受 server 冷启动、索引加载和 JSON-RPC 往返影响。
- 可维护性：CodeGraph 需要额外安装、配置和迁移；OKF 作为你维护的项目，本应承接当前 agent 的仓库知识管理能力。

因此需要一个 agent-native 的动态知识库管理工具：OKF 自己负责索引和检索，以 Go package 形式被 Bingo 内置，并通过 `bingo okf` 子命令暴露给用户和 agent，替代“agent → MCP → CodeGraph”的路径。`okf` 独立 CLI 可以继续存在于 OKF 项目里，但 Bingo 集成 MUST NOT shell out 到外部 `okf` 二进制，也不能要求用户额外下载。

## What Changes

- 新增 `dynamic-knowledge-index` 能力：维护 `.okf/state.json`、generated concepts 和可选本地 query cache 的一致性，支持显式初始化、显式刷新、安全 full rebuild 和派生 cache rebuild。
- 新增 `agent-native-knowledge-tool` 能力：提供稳定的机器可读工具接口，agent 可通过 Go library 直接调用；Bingo 通过内置 `bingo okf ... --json` 暴露相同能力，不需要 MCP server 或外部 `okf` binary。
- 新增 `agent-context-retrieval` 能力：把搜索结果从“概念列表”升级为“任务相关上下文包”，包含 symbols、relations、file locations、snippets budget、provenance 和 freshness。
- 增加 Bingo 入口：在 `bingo` 根命令下新增 `okf` 子命令族，用于管理当前仓库 `.okf/knowledge`，并调用 OKF package API 而不是外部命令。
- OKF 独立 CLI 可保留 `okf tool ... --json` 作为开发/调试入口，但不是 Bingo 的运行时依赖。
- 引入本地索引 cache 策略：OKF Markdown 仍是可读、可 diff 的 source of truth；cache 仅作为可删除、可重建的性能层，V1 不要求持久化磁盘 cache。

## V1 Scope Locks

为保证第一版可落地、可测试、可安全接入 Bingo，本 change 明确锁定以下边界：

- **Read-only by default**：`status`、`query`、`context` 不得写 `.okf/knowledge`、`.okf/state.json` 或 cache 文件；所有写入只能由 `init`、`refresh`、`lint --fix` 等显式 mutating operation 触发。
- **No auto-refresh by default**：V1 查询默认不自动改写知识库。若未来支持 `--refresh=auto`，必须显式打开，且受 changed-file threshold 和 allow-stale policy 保护。
- **No disk cache required**：V1 可以只用进程内索引或直接 bundle scan；若实现磁盘 cache，必须是 derived-only、schema-versioned、可删除重建，且不能成为验收前置条件。
- **No worktree support in V1**：`bingo okf` 不接入 Bingo `--worktree` 语义；所有 OKF 命令在当前或指定 Git repo 上执行，并复用 Bingo 现有 `rejectWorktreeFlag` 风格拒绝 worktree flag。
- **Generated deletion safety**：刷新只允许删除带有明确 generated metadata 且 `generator=okf.git`、`source_path` 匹配的文件；人类手写 concept 永不因路径相似被自动删除。
- **Stable JSON envelope**：所有 agent-facing JSON 响应使用固定 `schema_version: "okf.tool.v1"`，并通过 golden tests 锁定字段。
- **Deterministic ranking first**：V1 ranking 使用可解释的整数打分和固定 tie-break，不引入向量库、LLM reranker 或非确定性外部服务。

## Capabilities

### New Capabilities

- `dynamic-knowledge-index`: 动态维护仓库知识索引，保证 full generation、incremental update、query freshness metadata 和可选 cache 收敛。
- `agent-native-knowledge-tool`: 为 agent 暴露无 MCP 依赖的工具协议和 JSON schema。
- `agent-context-retrieval`: 为 agent 任务构造 token-budget-aware 的上下文包。
- `bingo-okf-command`: 在 Bingo CLI 中内置知识库管理子命令，单个 `bingo` binary 即可完成 OKF 初始化、刷新、查询、上下文构建和状态检查。

### Modified Capabilities

- `code-knowledge-dimension`: 继续负责代码实体和关系表达，但本 change 要求这些实体可被 agent 工具接口高效查询和上下文打包。
- `incremental-knowledge-index`: 从“更新文件 concept”扩展为“更新文件 concept + generated metadata safety + query freshness metadata”。

## Impact

- 主要影响 OKF Go 包：`pkg/query`（结构化查询与 deterministic ranking）、`pkg/git`（index freshness/rebuild/generated metadata）、`pkg/okf`（agent-facing API model）、CLI（JSON 工具接口）。
- 主要影响 Bingo Go 包：`internal/ui/cli/cmd` 需要注册 `okf` root subcommand；新增薄 adapter package 调用 OKF package service；`go.mod` 需要以编译期依赖方式引入 OKF module，从而把能力打进 Bingo binary。
- 可能新增包：`pkg/tool`（service interface、JSON contract、status/init/refresh/query/context orchestration）、`pkg/context` 或 `pkg/tool/context`（上下文包构建）、`pkg/index`（可选 cache/freshness helper）。
- Bingo 依赖策略：使用正式 module 版本优先；开发期可用 `replace github.com/superops-team/okf => /Users/bytedance/workspace/github/okf`，但 release 前必须移除本地绝对路径 replace，避免不可复现构建。
- 不要求引入 MCP、CodeGraph runtime、SQLite、daemon、向量库、LLM reranker 或外部服务。
- 不要求用户安装 `okf` CLI；Bingo 集成不得通过 `exec.Command("okf", ...)` 调用外部工具。
- 不删除 CodeGraph compatibility；保留导入/兼容能力，但默认路径变成 OKF 原生索引。
