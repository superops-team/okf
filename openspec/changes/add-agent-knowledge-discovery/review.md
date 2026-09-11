# Review: Agent Knowledge Discovery and Integration

## 评审范围与结论

- 评审对象：`docs/prd/agent-knowledge-discovery.md` 与 `openspec/changes/add-agent-knowledge-discovery/` 全部规划文件。
- 事实依据：当前 `main` 上的 `pkg/okf`、`pkg/query`、`pkg/tool`、`pkg/mcp`、`pkg/git`、`pkg/convert`、`cmd/okf`，以及已有 retrieval/durable/document-import OpenSpec。
- 评审方法：按 `AGENTS.md` 19 个强制维度做两轮校验；先找上下文、接线、兼容和边界问题，再做反向搜索消除已废弃设计残留。
- 总结：发现并整改 **19 个设计问题**（9 个必须修正、10 个歧义/精简项）。整改后无已知未关闭的 critical/high 设计问题；实现尚未开始，S01–S50 当前均为规划期 `gap`，不能宣称功能完成。

## 已修正的关键问题

1. 明确现有 durable capture 的 `concept_id` 是 64 位确定性幂等/路径句柄，新随机 `okf_id` 与其双字段并存，禁止替换旧语义。
2. 取消在转换 staging 中生成随机 ID；只在最终持久化目标生成或保留 ID，重复刷新/重导入必须保留原 ID。
3. `pkg/identity` 不依赖 `pkg/query`，由 query 调用字符串级 key helper，消除潜在 import cycle。
4. 明确稳定 URI 的真实公开入口：`Service.Resolve`、`okf identity resolve`、MCP `okf_resolve`。
5. Manifest 从不现实的“结束符后零字节读取”改为固定 4 KiB 缓冲、最多一次预取；正文不解析、不返回。
6. Manifest 缺结束符、header 超限、非法 YAML 分别使用稳定 warning code。
7. 分组发生于既有有限、已评分候选池的最终 dedupe/TopK 输出整形前；不放大 CandidateK、不二次召回。
8. `chunk` 只表示入口已暴露的最细命中，不虚构恢复已经被既有入口折叠的底层向量块。
9. JSON 配置只承诺未知 key/value 语义保持；TOML/Markdown marker 承诺区块外字节保持，避免不现实的全格式字节无损承诺。
10. 删除 `.okf/agent/install-state.json` 和原文件快照设计，以 semantic key/marker/header 自描述所有权，减少第二状态源。
11. Agent apply/remove 在非交互模式必须显式 `--yes`；无确认不得写入。
12. `identity ensure` dry-run 不消耗随机源、不输出候选随机 ID，保证重复预览字节确定。
13. JSON 中仅有 `mcpServers.okf` 名称不足以证明归 OKF 所有；新增非敏感 `OKF_MANAGED=agentconfig-v1` 标记，无标记同名 key 一律 conflict，防止误删用户配置。

---

## 维度 1：上下文逻辑连贯统一 —— 通过（整改 3 项）

- **问题**：最初将 Manifest 视作空 query 的 Context；但现有 Context 会读取正文并要求 query。
  - **整改**：Manifest 作为 `pkg/tool.Service` 的独立只读 operation，复用 Service envelope，不复制 query/context。
- **问题**：classic CLI/MCP 的 `okf.Concept → query.Concept` 转换遗漏 `CustomFields`，新身份和 source 元数据会丢失。
  - **整改**：规划统一 `pkg/query` adapter，并删除两处重复转换。
- **问题**：durable `concept_id` 与新 `okf_id` 名称近似、语义不同。
  - **整改**：Spec 明确旧字段保持 deterministic handle 语义，新字段只承担随机稳定身份。

## 维度 2：杜绝空谈 —— 通过（整改 2 项）

- Stable URI 原先只有纯函数，没有真实用户入口；现已固定 Service/CLI/MCP 三入口及 not-found 错误。
- Agent Integration 不再仅写“支持 Cursor/Claude/Codex”，已写死项目文件路径、owned key/block/header、plan/apply/status/remove 和 fixture gate。

## 维度 3：消除歧义 —— 通过（整改 5 项）

已定案：

- `okf_id` grammar、URI、随机源和错误码；
- Manifest `limit` 的未传/显式 0 语义、默认值 100、上限 500；
- Manifest filter 的维度间 AND、维度内 OR；
- stable status 默认值、folder root `.`、路径规范化和 fallback；
- Agent status 枚举、ownership、drift/conflict/remove 语义及非交互确认。

## 维度 4：大模型语义精准 —— 通过（整改 3 项）

- “Manifest 不读正文”改为可执行定义：正文不解析/不返回，buffer 允许最多 4 KiB 预取。
- “chunk 聚合”改为“入口已暴露的最细命中”，避免误解为恢复 HNSW 内部 chunk。
- “preserve config”拆分为 JSON 语义保持与 marker 文件区块外字节保持，避免实现者过度承诺。

## 维度 5：SDD/TDD 适配 —— 通过

- 50 个 Scenario 均有命名测试和真实入口矩阵。
- filesystem、entropy、reader、embedder/index 和 write/rename 均设置可注入边界，能先写 RED 测试。
- golden/fixture/property/eval/E2E 分层明确；实现必须按 RED→GREEN→REFACTOR 推进。
- Spec 尚未取得用户逐字审批，实施前应把本次评审后的版本作为批准基线。

## 维度 6：最小化实现 —— 通过（整改 2 项）

- 不引入 SQLite/LanceDB、全局 catalog、第二知识模型或第二 ranking。
- 删除 agent install-state 和全量 TOML parser；JSON 语义 merge，TOML/Markdown 仅 marker block。
- 分组仅替换显式请求时的最终 output shaping；未分组路径不变。

## 维度 7：向下兼容 —— 通过（需实现验证）

- 旧 Concept 无 ID 仍可 parse/lint/query，标记 `legacy-unstable`。
- generic parser 保留非法扩展值，identity-aware 操作 fail closed。
- `concept_id`、现有 ToolEnvelope 与无 `group_by` 响应保持兼容；新增字段为 additive。
- vector v2 明确 incompatible，禁止静默混用 v3 key；提供 rebuild remediation。
- 实现阶段必须用现有 golden 和 eval 证明，而非只依赖文档声明。

## 维度 8：存量业务破坏性影响 —— 通过（已显式化）

- ID 回填会修改 frontmatter，默认 dry-run，只有显式 apply 写入。
- v3 索引使旧索引必须 rebuild；迁移结果显式报告影响。
- Agent apply/remove 会修改项目文件，要求确认、ownership 检查、原子单文件写和回滚。
- JSON 可能重排空白/key 顺序，但未知值语义保持；Release Notes 必须披露。

## 维度 9：功能失效/运行异常风险 —— 通过（整改 4 项）

- 多文件写失败：不声称事务原子；从内存原始字节回滚并报告失败路径。
- symlink/path escape：plan 阶段 fail closed。
- Manifest 大正文/坏 frontmatter：256 KiB header cap、4 KiB buffer、稳定 warning、继续其他文件。
- 分组唯一 key 不足 K：允许少于 K，不二次召回、不隐藏扩大候选池。

## 维度 10：整体可行性 —— 通过，风险可控

- 所需原语均已存在：frontmatter extension、Service envelope、query score/result、MCP registry、原子写模式、OKF v0.2 trust/stale helpers。
- 核心阻碍不是技术不可行，而是多入口一致性和写入保 ID；已转化为 adapter/writer contract 测试。
- 预计 13.5 person-days 是规划估算，非承诺工期；实现后需用真实数据修正。

## 维度 11：分层任务、测试与排期 —— 通过

- P0 identity → P1 Manifest → P2 projection → P3 Agent adapters → P4 evidence/conformance。
- 每个 task 包含 files、implement、tests、scenario；总矩阵为 S01–S50。
- 排期只表示依赖，不构成阶段性交付。

## 维度 12：后续可扩展性 —— 通过

- Stable identity、Manifest、projection、agentconfig 各自有窄包边界。
- 新 Agent adapter 复用 canonical workflow 与 shared conformance suite。
- 新 grouping 仅需增加显式 enum/key projector 和评测；不侵入 ranking。
- 未引入 alias/history；若未来需要，必须独立 Spec，不污染当前最小模型。

## 维度 13：警惕过度设计 —— 通过（已删除 4 项）

已拒绝/删除：SQLite catalog、LanceDB 替换、install-state、原文件快照、仅凭同名 JSON key 推断所有权、force 删除、全局客户端配置、通用 TOML 重写依赖、固定新排序公式。

## 维度 14：小而高效 —— 通过

- ID 用 `crypto/rand`；不新增 UUID 依赖。
- Manifest 只扫描 bounded frontmatter；不启动 embedding/HNSW。
- grouping 为 O(n) projection；n 受既有候选上限约束。
- Agent 只管理至多三个已知客户端和少量固定文件。

## 维度 15：持续优化与代码直观性 —— 通过（实现约束）

- 统一 OKF→query adapter 消除 CLI/MCP 重复转换。
- identity string helper 避免包循环。
- ownership 由文件自身表达，状态计算无额外数据库。
- 实现必须遵守 Modern Go 指南并优先小函数/显式错误类型。

## 维度 16：架构统一 —— 通过

- Markdown 仍是唯一事实源；`okf_id` 为扩展字段。
- `pkg/tool.Service` 仍是 agent-facing 统一边界。
- MCP 只做 schema/transport adapter，不复制业务逻辑。
- Agent config 独立于 query/MCP 内核，只渲染项目接入文件。

## 维度 17：全需求接线与覆盖 —— 通过（规划层）

- S01–S50 全部映射到测试和入口。
- writer 覆盖 generator/import/MCP import/note/event/feedback，并补 staging 不生成 ID、refresh 保 ID、chunk parent ID。
- Service/CLI/MCP 覆盖 identity resolve、Manifest、grouping。
- 三客户端共享 conformance suite；文档、secret scan、mutation 和 real execution 均纳入 gate。
- 代码尚未实现，故当前 conformance 均为 `gap`；只有 fresh run 后才能改为 `fully/aligned`。

## 维度 18：P0/P1/P2 仅表示依赖 —— 通过

`tasks.md` completion contract 明确 P0–P4、四项产品能力、50 个 Scenario、文档、EVIDENCE 和 conformance 全部完成才可合入；不得只完成 Stable ID 或 Manifest 即宣称完成。

## 维度 19：Spec ↔ 实现 ↔ 测试一致性 —— 通过（任务已设计）

- `conformance.md` 预置 50 个独立场景行。
- T4.4 要求每行填写实现 symbol、自动化测试、最后 fresh 命令/结果和 alignment。
- `partial/gap` 必须有阻塞任务；最终不得无解释残留。
- Source commit、工具版本、检索指标、bytes-read、mutation kills 必须来自最后源状态。

---

## 两轮评审结论

### 第一轮：显性问题

共关闭 9 项：模型/字段冲突、入口缺失、import cycle、staging 随机 ID、配置保持承诺错误、Manifest 零预取、grouping 接线位置错误、额外 install-state、JSON 同名 key 被误判为 OKF-owned。

### 第二轮：隐性问题

共关闭 10 项：dry-run 随机不确定、group 数不足语义、底层 chunk 误承诺、warning code 混淆、remove 确认缺失、JSON 字节保持误承诺、回滚原子性误导、MCP 启动 cwd/绝对路径、derived chunk parent ID、现有 `concept_id` 向下兼容。

## 评审后准入状态

- **设计准入**：通过，可进入显式 Spec 审批。
- **实现准入**：待用户批准本次修订后的 Spec。
- **发布准入**：未通过；代码、RED/GREEN、gauntlet、真实评测、CI 和最终 conformance 均尚未执行。
