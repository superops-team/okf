# Design: OKF MCP 持久知识能力

## Context

OKF 当前有两条能力路径：

1. `pkg/tool.Service`：status/init/refresh/query/context，拥有路径解析、freshness、结构化过滤和 context budget 语义。
2. `pkg/mcp/tools.go`：bundle/list/get/search/lint/document-import，部分逻辑独立，搜索为简单 contains。

本变更不再扩展第二套知识查询实现，而是让 agent-facing MCP handler 调用 `pkg/tool.Service`。Brain/feedback 写入需要进入 OKF 核心 concept/bundle 持久层，写完后立即可由同一个 Service 查询。

## Goals

- MCP/CLI/tool 使用同一 service 和路径事实源。
- 为通用 MCP client 提供 repository knowledge 与 durable capture 能力。
- 保持旧 MCP 工具兼容。
- 所有写入幂等、原子、可审计、可跨进程读取。

## Non-Goals

- 不引入任何 host application 私有包或私有数据结构。
- 不在 OKF 管理 host application 的运行时会话记忆。
- 不增加通用数据库；继续使用 OKF bundle/concept 存储。
- 不自动执行 LLM 蒸馏；`okf_feedback` 接收调用方已结构化的原则与证据。

## Decisions

### D1. MCP 共享一个 `tool.Service`

`ServerConfig` 增加 `RepoPath`、`KnowledgeDir`。Server 初始化时创建一个 `tool.Service`，handlers 只进行 JSON schema 校验、调用 Service、序列化 envelope。

备选“每个 handler 自己 load bundle”被拒绝，因为会复制路径、freshness、overlay 和排序语义。

### D2. 新工具命名

- `okf_status`
- `okf_init`
- `okf_refresh`
- `okf_query`
- `okf_context`
- `okf_note`
- `okf_ask`
- `okf_log`
- `okf_feedback`

使用 underscore 避免不同 MCP client 对点号命名的不一致。

### D3. 显式知识写入服务

在 `pkg/tool` 增加 `WriteKnowledge`（或同等窄接口），请求字段为：

- kind: `note|event|feedback`
- content（必填）
- project（可选）
- tags（可选）
- metadata（小型 JSON，可选）
- idempotency_key（必填）
- evidence_refs（feedback 可选）

服务把请求转换为 `okf.Concept`：

- type 为 kind；
- title 为受限长度的确定性摘要；
- content 保存正文；
- tags 规范化去重；
- custom fields 保存 project、metadata、idempotency key、provenance；
- concept identity 由 repo identity + kind + idempotency key 稳定派生。

相同 key + 相同内容返回原记录；相同 key + 不同内容返回 conflict，不覆盖。

### D4. 原子保存

写流程：解析路径 → 加载 bundle → 校验/构造 concept → 在内存中 upsert → 写临时目录/文件 → fsync/rename 或调用已有原子保存原语 → 回读验证。任何失败返回 error envelope，不暴露半写状态。

### D5. `okf_ask` 复用 Query

`okf_ask` 是 `Service.Query` 的窄投影，默认筛选 `note|event|feedback`，可传 project 和 limit。不得调用旧 `okf_search` contains 实现。

### D6. Wiki 作为输入，不作为特殊状态机

OKF 的 document import/refresh 扫描 Markdown 文档，包括 `wiki/`。不新增 `wiki/index.md`、`wiki/log.md` 自动写入规则。Wiki 分类、关系和 freshness 均由 concept metadata 与现有 lint/query 表达。

### D7. 写操作安全

- MCP tool annotation 标识 mutating。
- content、tags、metadata 设置大小上限。
- 拒绝空内容、未知字段、路径逃逸、无 idempotency key。
- 检测明显 credential 字段名并拒绝；错误响应不回显敏感值。
- `repo` 和 `dir` 只来自 server 启动配置，不允许单次工具调用改写根目录。

### D8. 路径语义

`pkg/tool.Service.resolve` / `okf.ResolveKnowledgePaths` 是唯一解析入口：

- 绝对 dir：`filepath.Clean(dir)`，保持绝对。
- 相对 dir：相对 repo root。
- MCP、CLI tool、普通 CLI 使用相同 service config。

### D9. 反馈沉淀职责

OKF 负责“持久化、校验、索引和检索反馈原则”，不依赖 host application 的私有 distiller。`okf_feedback` 接收调用方显式给出的 principle、category、evidence refs 和 idempotency key；MCP tool description/prompt 明确建议 agent 仅在用户纠正、可复用失败原则或显式沉淀请求时调用。OKF 不监听 host application 私有事件总线，也不复制其 runtime 类型。

### D10. Wiki 内容边界

OKF 将 repository 中的 Wiki 或其他 Markdown 视为普通 source，通过 init/refresh/query/context 管理，不引入特殊 Wiki 状态机或索引日志维护策略。

## Scenario Matrix

| 场景 | 预期 |
|---|---|
| MCP status 未初始化 | 结构化 `knowledge_not_initialized`，不创建文件 |
| MCP init + 绝对 dir | 只写绝对目标目录 |
| MCP init + 相对 dir | 写 repo root 下相对目录 |
| note 重复 key/同内容 | 返回同一 concept，数量不增 |
| note 重复 key/不同内容 | conflict，原内容不变 |
| log metadata 超限 | invalid_request，无写入 |
| feedback 含 evidence refs | refs 保存在 provenance fields |
| ask project filter | 只返回指定 project |
| context budget | used tokens 不超过 budget |
| 并发写同一 key | 最终单一 concept，无损坏 bundle |
| 保存中途失败 | 原 bundle 可完整读取 |
| repo/dir escape | 拒绝并返回结构化错误 |
| 进程重启 | note/log/feedback 可查询 |
| 旧 MCP tools | 行为保持不变 |

## Rollout

1. 先增加 service 与 handler 测试，不删除旧 MCP 工具。
2. 完成 Python MCP 双进程测试。
3. Host application 可在真实 MCP 验证通过后切换到该通用能力。
4. 若 rollout 失败，host application 可恢复原有客户端配置；OKF 新工具可独立回滚，不改变旧 bundle schema。
