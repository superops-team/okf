# Change: 新增通用 MCP 持久知识能力

## Why

OKF 已有 MCP server 和 `pkg/tool.Service`，但 MCP 工具尚未完整暴露 status/init/refresh/query/context，也缺少面向 agent 的持久 note/event/feedback 写入与定向检索入口。

本变更为任意 MCP client 提供统一、可持久化、可审计的项目知识能力。路径解析、查询、context packing 与持久化必须复用 OKF service/domain 层，禁止 MCP handler 复制实现。

## What Changes

- 为 MCP server 增加 `--repo`、`--dir`，构造共享 `pkg/tool.Service`。
- 增加 service-backed `okf_status`、`okf_init`、`okf_refresh`、`okf_query`、`okf_context` 工具。
- 增加 `okf_note`、`okf_log`、`okf_feedback` 持久知识写入工具；写入 OKF concept/bundle，支持幂等键和原子保存。
- 增加 `okf_ask`，复用 `Service.Query` 检索 note/log/feedback concepts。
- 统一 MCP 与 CLI/tool JSON envelope 的 schema version、error code、repo root、knowledge dir、freshness 与 warnings。
- 保留既有 bundle/list/get/search/lint/document-import MCP 工具，标注其能力边界；新 agent-facing 工具不调用旧简单 contains search。
- 增加绝对/相对 `KnowledgeDir` 的 CLI、service、MCP 回归测试。
- 增加通用持久知识采集说明，覆盖 note/event/feedback、普通 Markdown 输入与隔离知识目录。

## Capabilities

### Added

- `mcp-agent-knowledge`: 通过 MCP 暴露 OKF 统一 service 和显式项目知识写入。

### Modified

- `tool-service-v1`: 增加结构化持久知识写入操作；现有 status/init/refresh/query/context 语义保持兼容。
- `knowledge-path-resolution`: MCP 与 CLI/tool 共用绝对/相对路径解析。

## Impact

- 代码：`pkg/mcp`、`pkg/tool`、`cmd/okf`、MCP Python E2E。
- 协议：新增 MCP 工具，不删除现有工具；写工具具有可见 side effect。
- 数据：新增 concept 类型 `note`、`event`、`feedback`，不修改既有 Concept 核心字段。
- 安全：所有 source/project/path 必须规范化并限制在配置的 repository/knowledge roots；不保存 secrets 或原始凭证。
- 兼容：现有 `okf mcp --bundle` 保持可用；`--repo`/`--dir` 为新增参数。
