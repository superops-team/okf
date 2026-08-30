# mcp-agent-knowledge Specification

## ADDED Requirements

### Requirement: MCP 必须暴露 service-backed repository knowledge 工具

OKF MCP server SHALL 暴露 `okf_status`、`okf_init`、`okf_refresh`、`okf_query`、`okf_context`，并 SHALL 调用同一 `pkg/tool.Service` 实例。

#### Scenario: 查询使用统一 service

- **GIVEN** MCP server 以 repo 和 knowledge dir 启动
- **WHEN** client 调用 `okf_query`
- **THEN** 返回与 `okf tool query` 相同 schema version、repo root、knowledge dir、freshness、warnings 和 result 语义
- **AND** handler 不执行独立 contains 搜索

#### Scenario: 未初始化状态不产生副作用

- **GIVEN** repository 尚未初始化 OKF knowledge
- **WHEN** client 调用 `okf_status`
- **THEN** 返回结构化 `knowledge_not_initialized`
- **AND** 不创建 knowledge 文件或目录

#### Scenario: context 遵守预算

- **WHEN** client 调用 `okf_context` 并设置 `budget_tokens`
- **THEN** `used_tokens` 不大于预算
- **AND** omission 原因结构化返回

### Requirement: MCP 路径解析必须保留绝对目录语义

MCP server SHALL 通过 `pkg/tool.Service` 解析 `--repo` 与 `--dir`。

#### Scenario: 绝对目录

- **GIVEN** `--dir` 是绝对路径
- **WHEN** client 调用 `okf_init`
- **THEN** knowledge 写入该绝对路径
- **AND** 不在 repo root 下生成同名拼接路径

#### Scenario: 相对目录

- **GIVEN** `--dir` 是相对路径
- **WHEN** client 调用 `okf_init`
- **THEN** knowledge 写入 repo root 下该相对路径

### Requirement: MCP 必须支持持久 note、event 与 feedback

OKF MCP server SHALL 暴露 `okf_note`、`okf_log`、`okf_feedback`，将输入保存为 OKF concepts。

#### Scenario: note 跨进程可查询

- **WHEN** client 使用唯一 idempotency key 写入 note
- **AND** server 进程退出并重新启动
- **THEN** `okf_ask` 返回该 note

#### Scenario: 幂等重试

- **WHEN** 相同 idempotency key 与相同内容提交两次
- **THEN** 返回同一 concept identity
- **AND** bundle concept 数量只增加一次

#### Scenario: 幂等冲突

- **WHEN** 相同 idempotency key 与不同内容提交
- **THEN** 返回 conflict error
- **AND** 已保存内容保持不变

#### Scenario: feedback 保留证据引用

- **WHEN** client 写入带 `evidence_refs` 的 feedback
- **THEN** principle、category 和 evidence refs 以 provenance 字段持久化
- **AND** `okf_ask` 可返回该 feedback

#### Scenario: 并发相同幂等键

- **WHEN** 两个独立 goroutine 或 server request 并发提交相同 idempotency key 和内容
- **THEN** 最终只存在一个 concept
- **AND** bundle 可被重新加载且无数据竞争

#### Scenario: 写失败保持原子性

- **GIVEN** 保存路径在 commit 阶段失败
- **WHEN** client 写入 note/log/feedback
- **THEN** 返回结构化失败
- **AND** 原 bundle 仍可加载
- **AND** 查询不返回半写记录

### Requirement: ask 必须复用统一 Query

`okf_ask` SHALL 复用 `pkg/tool.Service.Query` 并限定为 note、event、feedback 类型。

#### Scenario: project 隔离

- **GIVEN** 两个 project 各有一条 note
- **WHEN** `okf_ask` 指定其中一个 project
- **THEN** 只返回该 project 的记录

### Requirement: 写工具必须 fail closed

写工具 SHALL 拒绝空内容、未知 kind、未知字段、超限输入、缺失 idempotency key、路径逃逸和明显 credential 字段。

#### Scenario: 路径逃逸被拒绝

- **WHEN** server 配置解析得到的 knowledge path 逃逸允许的 repository/knowledge root 或经过 symlink 指向非授权位置
- **THEN** 返回 path_outside_root
- **AND** 非授权位置不发生写入

#### Scenario: 非法输入

- **WHEN** 输入包含未知字段或空 content
- **THEN** 返回 invalid_request
- **AND** knowledge bundle 不发生变化

### Requirement: 旧 MCP 工具必须保持兼容

新增 agent-facing 工具 SHALL 不删除或改变现有 bundle/list/get/search/lint/document-import 工具契约。

#### Scenario: 旧工具回归

- **WHEN** 运行现有 Go MCP 测试和 `python3 test_mcp.py`
- **THEN** 原测试全部通过
