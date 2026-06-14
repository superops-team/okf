## Why

OKF 当前能生成项目知识库，但核心路径仍停留在 MVP 状态：没有测试安全网，`update` 命令没有真正持久化增量结果，代码分析只产生文件级元数据，性能也受逐文件 Git 子进程和串行扫描限制。现在需要把它升级为可验证、增量正确、能按需回答代码结构问题的轻量索引器，避免继续在不可靠基础上叠加 LSP 级能力。

## What Changes

- 补齐核心包的单元测试、CLI smoke test 与性能基准，确保 parser、lint、query、bundle、git 扫描和增量更新都有可回归验证。
- 修正增量更新语义：`okf update` MUST 加载现有知识库，对 added/modified/deleted 文件做 upsert/delete，并保存回 `.okf/knowledge`。
- 让代码分析从“文件元数据”升级到“代码结构索引”：先落实现有 imports/functions 字段，再为 Go 文件引入 AST 解析，提取 package、imports、functions、methods、types、interfaces 与行号范围。
- 增加轻量关系图：package/file import graph、file-to-symbol 归属关系、symbol metadata，为后续 callers/callees/impact 做铺垫。
- 提升大型仓库索引效率：批量获取 Git metadata、并发文件分析、预编译正则、查询侧建立 type/tag/resource/title 内存索引，并用 benchmark 锁定性能目标。
- 不引入完整 LSP/gopls/SSA 调用图，不默认把源码全文写入知识库。源码内容采用 path + line range 按需读取策略。

## Capabilities

### New Capabilities

- `test-suite-foundation`: 覆盖 OKF 核心包、CLI 行为、增量更新和性能基准的测试能力。
- `incremental-knowledge-index`: 正确、幂等、可恢复的增量知识库更新能力。
- `code-symbol-index`: 基于 Go AST 与通用解析 fallback 的代码符号、导入和关系索引能力。
- `index-performance`: 面向大型仓库的高效扫描、保存和查询能力。

### Modified Capabilities

- 无。当前仓库没有已归档的 OpenSpec specs，本 change 以新增能力定义目标行为。

## Impact

- 主要影响 Go 代码：`cmd/okf/main.go`、`pkg/git/*`、`pkg/okf/*`、`pkg/parser/*`、`pkg/query/*`、`pkg/lint/*`。
- 需要新增测试文件和 benchmark 文件，覆盖所有核心包。
- 可能新增内部包，例如 `internal/index`、`internal/symbols` 或 `pkg/git/symbols.go`，用于封装符号模型、AST 解析与关系图生成。
- 不要求变更 OKF Markdown frontmatter 的现有基础字段，但会扩展 concept content 中的结构化章节，并可通过 `CustomFields` 或后续 spec 扩展 machine-readable metadata。
- 不新增外部服务依赖。Go AST 使用标准库；如需 `golang.org/x/tools/go/packages` 或 SSA，必须另起 change。
