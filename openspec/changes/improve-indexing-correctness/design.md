## Context

OKF 当前仓库是一个 Go 1.21 CLI + library 项目。现有核心结构如下：CLI 入口在 `cmd/okf/main.go`，Git 扫描和生成在 `pkg/git/git.go` 与 `pkg/git/generator.go`，解析/序列化在 `pkg/parser/parser.go`，bundle API 在 `pkg/okf/api.go` 和 `pkg/okf/types.go`，查询在 `pkg/query/query.go`，lint 在 `pkg/lint/lint.go`。

已验证的现状：

- `go test ./...` 能运行，但所有包都是 `[no test files]`。
- `cmdUpdate` 调用 `git.UpdateFromLastCommit` 后没有保存返回的 bundle，只打印结果；相关逻辑在 `cmd/okf/main.go:162`、`cmd/okf/main.go:173`、`cmd/okf/main.go:176`。
- `Config.IncludeFiles` 有默认值，但 `ShouldInclude` 没有使用 include patterns；字段和默认值在 `pkg/git/git.go:15`、`pkg/git/git.go:32`，过滤逻辑在 `pkg/git/git.go:193`。
- `FileSummary` 有 `Imports` 和 `Functions` 字段，但 `AnalyzeFile` 没有填充；结构定义在 `pkg/git/git.go:139`，分析函数在 `pkg/git/git.go:155`。
- `ExtractImports` / `ExtractFunctions` 当前使用正则 fallback，并且在热路径内重复编译正则；相关逻辑在 `pkg/git/git.go:232`、`pkg/git/git.go:263`。
- `SaveKnowledgeBase` 在 `pkg/git/generator.go:375` 使用本文件私有 `serialize`，而不是复用 `pkg/parser/parser.go:81` 的 `SerializeConcept`。

本设计把工作分成 P0/P1/P2/P3 四层：P0 建安全网和修正确性，P1 做增量正确和基础结构提取，P2 做 Go AST 与关系图，P3 做性能优化和索引。完整 LSP、SSA 调用图、跨语言语义理解不在本 change 内。

## Goals / Non-Goals

**Goals:**

- P0：为 parser、lint、query、okf、git 和 CLI 建立测试安全网。
- P0：修复 `okf update` 不落盘、include patterns 不生效、重复序列化路径等正确性问题。
- P1：让 `AnalyzeFile` 真实填充 imports/functions，并将结构信息写入 concept 内容。
- P1：实现基于 last indexed commit 的增量更新，覆盖多 commit diff、added/modified/deleted 文件。
- P2：为 Go 文件引入 AST 解析，提取 package/import/symbol/type/method/interface/line range。
- P2：生成轻量关系图：file imports、package imports、file owns symbols。
- P3：批量 Git metadata、并发文件分析、预编译正则、查询索引和 benchmark。

**Non-Goals:**

- 不实现 gopls/LSP 集成。
- 不实现 SSA call graph、callers/callees 或精确 impact graph。
- 不默认把完整源码写入 `.okf/knowledge`。
- 不改变 OKF Markdown frontmatter 的核心字段契约。
- 不引入数据库、daemon、远程服务或后台索引进程。

## Decisions

### Decision 1: 先修测试和正确性，再做 AST 和性能

选择：P0/P1/P2/P3 分层交付，每层都必须有测试和验证命令。

理由：当前没有任何测试，直接改 AST、并发和增量状态会放大回归风险。先让现有行为可测，再升级索引能力。

备选方案：一次性重写成 codegraph/LSP 风格。放弃原因：范围过大，且会把当前 CLI 的正确性 bug 留在新架构下。

### Decision 2: 增量更新以“加载现有 bundle + upsert/delete + 保存”为核心

选择：`okf update` 不再只返回 incremental bundle，而是读取 `.okf/knowledge`，对 changed files 应用 upsert/delete，然后调用统一保存路径。

需要新增或调整的数据：

- `.okf/state.json` 或等价 state 文件，记录 `last_indexed_commit`、`schema_version`、`generated_at`。
- 每个 file concept 继续以 `Resource` / `FilePath` 记录源文件相对路径，作为 upsert/delete 的稳定键。

备选方案：每次 update 都 full regeneration。放弃原因：正确但不满足增量目标，也无法支撑大型仓库性能。

### Decision 3: Go 用 AST，其他语言保留正则 fallback

选择：新增 Go AST analyzer，优先处理 `.go`；非 Go 文件继续使用 precompiled regex fallback。

Go AST 解析输出：

- package name
- imports
- symbols：function、method、struct、interface、type alias
- receiver type
- exported status
- start/end line
- parse warnings

备选方案：所有语言都用 tree-sitter。放弃原因：引入外部依赖和构建复杂度，不适合当前 Go 1.21 轻量 CLI 阶段。

### Decision 4: 关系图先做轻量 graph，不做调用图

选择：本阶段只生成可从文件和 AST 直接得到的关系：

- `file -> import`
- `package -> import`
- `file -> symbol`
- `package -> symbol`

理由：这些关系稳定、成本低、测试容易，能支持 Agent 快速定位代码结构。精确 callers/callees 需要类型信息、SSA 或 LSP，不适合塞进本 change。

### Decision 5: 源码按需读取，不写入知识库全文

选择：concept 存 path、line ranges、symbols、imports、summary，不默认嵌入完整源码。

理由：避免知识库膨胀和潜在敏感源码复制。Git 已经是源码存储，OKF 应保存导航和摘要。

### Decision 6: 性能优化以“减少无谓工作”为先

选择顺序：批量 Git metadata > 并发文件分析 > 预编译 regex > 查询索引 > 可选缓存。

理由：当前最可疑瓶颈是每文件多次 Git 子进程和串行处理。先减少进程启动和串行 I/O，再讨论持久缓存。

## Risks / Trade-offs

- [Risk] AST 输出改变 concept 内容，可能影响已有用户脚本读取 markdown。→ Mitigation: 不改 frontmatter 基础字段；新增章节使用稳定标题；在 README 后续更新中说明格式。
- [Risk] 并发分析引入非确定性输出。→ Mitigation: 汇总结果按 file path 和 symbol identity 排序后再保存；`go test -race ./...` 必须通过。
- [Risk] last indexed commit state 损坏会造成漏更新。→ Mitigation: state 缺失或无效时安全退回 full regeneration，并在 CLI 输出说明。
- [Risk] 删除文件处理误删用户手写 concept。→ Mitigation: 只删除 `Resource` 或 metadata 标记为 generated source file 的 concept；非 generated concept 不自动删除。
- [Risk] 查询索引与现有 Search 语义不一致。→ Mitigation: indexed path 只用于 type/tag/resource/title/symbol 精确维度；free-text 保持现有语义，直到另起 spec。

## Migration Plan

1. 新增测试夹具和单元测试，不改生产行为。
2. 修正 `ShouldInclude`、统一 serializer、让现有 ExtractImports/ExtractFunctions 落入 summary 和 concept。
3. 修正 update 持久化，新增 `.okf/state.json`，实现 added/modified/deleted upsert/delete。
4. 新增 Go AST analyzer 和 symbol model，更新 concept 内容结构。
5. 新增轻量关系图模型和输出。
6. 引入批量 Git metadata、worker pool、precompiled regex、query index。
7. 跑 `go test ./...`、`go test -race ./...`、`go test -bench=. -benchmem ./...`。

Rollback 策略：每一层应独立提交。P0/P1 失败时直接 revert 对应提交。P2/P3 失败时可通过配置关闭 AST analyzer 或 concurrency，退回顺序扫描和 regex fallback。

## Open Questions

- Symbol metadata 应先只写入 concept body，还是同步写入 `CustomFields`？建议第一版写 body + 内部结构体，等 schema 稳定后再归档 machine-readable 字段。
- 删除文件是否需要 tombstone 模式？建议默认删除 stale generated concept，tombstone 作为后续配置项。
- Benchmark 目标阈值需要在实现后用本机和一个 1k 文件 fixture 测量后确定，不在 spec 中硬编码绝对耗时。
