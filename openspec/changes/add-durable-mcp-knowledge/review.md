# Review: Add Durable MCP Knowledge

## 评审范围与依据

- 评审对象：`openspec/changes/add-durable-mcp-knowledge/` 的 proposal、design、delta spec、tasks。
- 源码证据：`pkg/tool/service.go`、`pkg/mcp/server.go`、`pkg/mcp/tools.go`、`cmd/okf/main.go`、`pkg/okf` 保存/解析路径。
- 规范依据：仓库 `AGENTS.md` 的 19 维要求、Scenario 接线覆盖、P0-P4 全量完成与 conformance 门禁。
- OpenSpec：`openspec validate add-durable-mcp-knowledge --strict` 已通过。
- 总体评价：**通过，可进入 TDD；阻塞项 0**。

## 已发现并修复的问题

| ID | 级别 | 问题 | 处置 |
|---|---|---|---|
| R-1 | 阻塞 | design 中未初始化、并发同 key、feedback evidence、路径逃逸只有矩阵，没有正式 Requirement/Scenario | 补入 delta spec，并加入 Scenario 覆盖矩阵 |
| R-2 | 阻塞 | tasks 未声明 P0-P4 全量完成门槛，可能阶段性误报完成 | tasks 顶部增加完成门槛，明确任一 partial/gap 不得完成 |
| R-3 | 阻塞 | Scenario 没有一对一映射实现入口和测试 | 建立 S1-S15 覆盖矩阵，映射 Go/Python 测试与接线点 |
| R-4 | 改进 | feedback “自动沉淀”职责可能误解为 OKF 监听 host 私有事件 | D9 定案：OKF 只保存显式结构化 principle/evidence，不依赖 host runtime |
| R-5 | 改进 | Wiki 管理可能被误解为搬运/删除文件 | D10 定案：Wiki 作为目标 repo 普通 Markdown source，本次不搬运/删除 |

## 十九维评审

| 维度 | 判定 | 证据与结论 |
|---|---|---|
| 1. 上下文逻辑连贯 | 通过 | proposal 的 9 个工具、design 的 D1-D10、spec 的 Requirements、tasks 的 S1-S15 对齐。 |
| 2. 杜绝空谈 | 通过 | 写入 identity、冲突、原子性、路径、安全、重启和旧工具回归均有测试入口。 |
| 3. 消除歧义 | 通过 | 工具名、kind、idempotency、project、路径来源、Wiki/feedback 边界已定案。 |
| 4. 语义精准 | 通过 | agent-facing service 与 legacy bundle tools 分开；ask 明确投影 Service.Query。 |
| 5. SDD/TDD 适配 | 通过 | 每组任务明确 RED/GREEN/REFACTOR；failure seam、并发、双进程均可构造。 |
| 6. 最小化实现 | 通过 | 复用 `pkg/tool.Service`、Concept/Bundle 与现有 MCP server；不改核心 Concept schema。 |
| 7. 向下兼容 | 通过 | 旧 MCP tools 和 `--bundle` 保留；新工具为 additive。 |
| 8. 破坏性影响 | 通过 | 新增 mutating tools 有明确 annotation、validation、idempotency 和错误契约。 |
| 9. 风险预判 | 通过 | 覆盖路径逃逸、symlink、secret、输入超限、并发、半写、重启和冲突。 |
| 10. 可行性 | 通过 | `pkg/tool.Service` 与 MCP server 已存在；绝对 KnowledgeDir 已有 service 回归测试。 |
| 11. 分层任务/测试/排期 | 通过 | P0 规范→P1 MCP→P2 写入→P3 ask/E2E→P4 文档/门禁。 |
| 12. 可扩展性 | 通过 | `WriteKnowledge` 是窄 service，新增 kind 不需复制 handler 查询栈。 |
| 13. 拒绝过度设计 | 通过 | 不引数据库、不监听 host 私有事件、不建 Wiki 状态机、不改变核心模型。 |
| 14. 小而高效 | 通过 | 改动集中 `pkg/tool`、`pkg/mcp`、`cmd/okf` 与测试/文档。 |
| 15. 持续优化 | 通过 | 统一 envelope、strict decode、稳定 identity、共享 resolver 减少语义漂移。 |
| 16. 架构统一 | 通过 | MCP adapter→tool service→OKF domain/storage，符合现有分层。 |
| 17. 全需求接线与覆盖 | 通过 | S1-S15 一对一列出测试和接线点；实现后 conformance 必须回填实测。 |
| 18. 排期仅表依赖 | 通过 | 完成门槛明确 P0-P4 全部属于交付范围。 |
| 19. spec-实现一致性 | 通过（设计期） | tasks 6.3 要求生成 `conformance.md`；任何 partial/gap 阻塞交付。 |

## 评审结论

- 初始阻塞项：3
- 已修复：3
- 剩余阻塞项：0
- SDD 状态：proposal/design/spec/tasks 齐全，strict validate 通过。
- 结论：**允许进入测试先行实现；完成声明仍需 S1-S15 全部实测、gauntlet 与 conformance fully。**
