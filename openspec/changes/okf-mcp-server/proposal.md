# Proposal: OKF MCP Server

## Summary

为 okf 项目实现 Model Context Protocol (MCP) Server，使 okf 知识库能够被各种 AI Agents（Claude、Cursor、Copilot、自定义 Agent 等）通过标准化协议集成和接入。参考 GitHub MCP Server 的实现模式，提供工具(Tools)、资源(Resources)、提示词(Prompts)三类核心原语。

## Motivation

- **生态集成**：MCP 已成为 AI Agent 与外部系统交互的事实标准，Claude Desktop、Cursor、VS Code Copilot、各类自定义 Agent 框架均已支持 MCP Client
- **降低集成成本**：一次实现 MCP Server，所有 MCP Client 均可接入，无需为每个平台单独开发插件
- **Agent 原生工作流**：AI Agent 可以直接通过 MCP 工具查询、创建、更新 okf 概念，执行 lint 验证，运行 attested computation，实现知识库的自动化维护
- **参考实现**：GitHub MCP Server 已验证此模式的可行性和价值，okf 作为知识库格式标准，应有对应的 MCP Server 参考实现

## Requirements

### MUST
- 实现 MCP 2025-11-25 规范兼容的 Server，支持 stdio 和 Streamable HTTP 两种传输
- 提供完整的 Tools 能力：知识库加载/保存、概念 CRUD、查询搜索、lint 验证、工具服务、attested computation
- 提供 Resources 能力：bundle、concept、index、log 资源的 URI 寻址
- 提供 Prompts 能力：概念解释、知识库总结、变更审查等模板
- 支持 OKF SPEC v0.2 所有字段（provenance、trust、lifecycle、attested computation）
- 向后兼容 v0.1 格式知识库
- 提供完整的错误处理和结构化错误响应
- 提供配置机制：bundle 路径、权限控制、工具集开关

### SHOULD
- 支持增量更新和实时索引刷新通知
- 提供工具集(toolsets)分组机制，类似 GitHub MCP Server 的 context/repos/issues 分组
- 支持采样(Sampling)回调，Server 可请求 Client 执行 LLM 补全
- 提供完成(Completion)能力，为参数输入提供上下文建议
- 支持多 bundle 同时加载和切换

### MAY
- 提供 WebSocket 传输支持
- 提供认证和权限控制机制
- 提供审计日志
- 提供遥测和使用统计

## Non-Goals

- 不实现 MCP Client 功能（仅 Server 端）
- 不实现可视化 UI 界面（由 MCP Client 提供）
- 不实现分布式部署和集群管理（单机单进程）
- 不替代现有的 CLI 工具，而是作为补充接入方式

## Impact

- 新增 `pkg/mcp/` 包：MCP Server 核心实现
- 新增 `cmd/okf-mcp/` 或扩展 `cmd/okf/`：MCP Server 入口
- 新增依赖：MCP Go SDK（`github.com/mark3labs/mcp-go` 或官方 SDK）
- 文档更新：README 增加 MCP Server 使用说明
- 测试：新增 MCP Server 集成测试

## Risks

- **依赖稳定性**：MCP Go SDK 生态尚在快速发展，API 可能变更
- **权限安全**：MCP 工具暴露了文件系统写操作，需要谨慎的权限控制
- **性能**：大知识库的查询和序列化可能需要优化
- **兼容性**：不同 MCP Client 的实现差异可能需要适配
