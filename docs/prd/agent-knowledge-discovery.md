# PRD：OKF Agent Knowledge Discovery

- **状态**：Proposed
- **版本**：1.0
- **对应 OpenSpec**：`openspec/changes/add-agent-knowledge-discovery/`
- **目标版本**：OKF 0.7.0

## 1. 背景与机会

OKF 已形成“多格式导入 → Markdown Concept → 本地混合检索 → MCP/Context → durable capture”的闭环，当前检索基线为 Hybrid Recall@5 0.9615、MRR 0.7256。下一阶段不应再造检索内核，而应补齐 Agent 长期使用知识时的身份、发现、组织与接入体验。

用户当前会遇到四类断点：

1. 文档改名或移动后，路径引用、feedback evidence 和外部 Agent 保存的链接失效。
2. Agent 为判断“有什么知识可用”常需加载正文或运行搜索，冷启动成本和 token 消耗偏高。
3. 同一批召回无法按 chunk、Concept、文档 source、目录知识域切换，证据粒度与导航粒度混在一起。
4. MCP 工具虽然存在，但 Cursor、Claude Code、Codex 的项目配置和使用规则需要人工维护，且容易漂移或覆盖用户内容。

## 2. 产品目标

### G1 长期可引用

让拥有 `okf_id` 的 Concept 在路径和标题变化后仍通过 `okf://concept/<id>` 被解析。

### G2 低成本发现

让 Agent 在不读正文、不运行向量模型时，以确定性 Manifest 了解知识库中有哪些条目、可信度、时效性和当前位置。

### G3 多粒度消费

让调用方在不改变底层召回算法的情况下，选择证据级、概念级、文档级或目录级结果投影。

### G4 可维护接入

让三类 Agent 通过同一 canonical workflow 接入 OKF，安装可预览、重复执行幂等、配置漂移可诊断、卸载可恢复。

## 3. 用户与核心场景

| 用户 | 场景 | 当前问题 | 目标体验 |
|---|---|---|---|
| Coding Agent | 保存一次知识引用后跨重构复用 | 路径变更导致引用失效 | 通过稳定 URI 找到新路径 |
| 大上下文 Agent | 进入陌生仓库先发现知识 | 全文加载或盲搜成本高 | 先读 Manifest，再选 2–3 个条目 |
| 开发者 | 查具体证据或浏览知识域 | top-K 粒度固定、同源结果拥挤 | 显式选择 group_by 粒度 |
| 团队维护者 | 给不同 Agent 配置 OKF | 配置重复、提示词漂移、卸载困难 | 一条命令 plan/apply/status/remove |
| CI/审计人员 | 判断能力是否按 Spec 落地 | 文档与代码可能漂移 | Scenario→实现→测试→证据逐条对照 |

## 4. 功能范围

### 4.1 Stable Concept ID

- 扩展字段：`okf_id`。
- 格式：`okf_` + 32 位小写十六进制字符，共 128 bit 随机值。
- 稳定 URI：`okf://concept/<okf_id>`。
- 生成：标准库 `crypto/rand`；不增加 UUID 依赖。
- 读取：普通 Load/Parse 只保留字段，不自动写盘。
- 新写入：仅在最终目标写盘前确保合法 ID；转换 staging 不生成随机 ID。最终目标已存在时保留原 `okf_id`，首次创建才生成；派生 chunk 保存 `parent_okf_id` 继承父 Concept 身份。
- 旧库迁移：`okf identity ensure` 只输出确定性的路径/动作计划，不消耗随机源、不打印候选 ID；`--apply` 才在全量预检后生成并回填。每个文件使用同目录临时文件原子替换，跨文件失败执行可验证回滚但不宣称跨文件事务原子性。
- 冲突：非法或重复 ID 不允许 identity-aware 操作继续；迁移在写入前全量预检。
- 索引：新增 identity-aware index key；存在 ID 时用 ID，无 ID 时回退 legacy fingerprint；索引格式显式升级。

### 4.2 Manifest 元数据发现

- 入口：`okf tool manifest`、MCP `okf_manifest`，共用 `pkg/tool.Service.Manifest`。
- 不要求 query；支持 type/tag/status/stale/folder_prefix 过滤。
- 默认 `limit=100`、最大 500、`offset>=0`。
- 稳定排序：规范化 bundle-relative path 升序，路径相同再按 ID 升序。
- 返回：ID/URI/identity state、当前路径、title、description、type、tags、有效 status、trust tier、stale、generated/verified 时间、source 摘要、文件大小、估算 token、顶层索引状态。
- token 估算口径：`ceil(file_size_bytes/4)`，字段同时返回 `estimate_kind=file_bytes_div4`，不伪装成精确 tokenizer 结果。
- Manifest 只解析 frontmatter（单文件 header 上限 256 KiB）和 `stat`；固定 4 KiB 缓冲允许结束符之后最多一次预取，但正文不进入 YAML/响应。缺结束符、超限和非法 YAML 分别输出稳定 warning code 并省略单文件，重复 ID 整个请求失败。
- 不创建、加载或重建向量索引，不初始化 embedding runtime。

### 4.3 分层检索聚合

- 显式参数：`group_by=chunk|concept|source|folder`。
- 未传参数：保持各入口当前行为，不发生隐式兼容变更。
- 聚合复用现有各通道在最终 source 去重/TopK 前已经产生的有限候选池，不放大 CandidateK、不触发第二轮检索；唯一组不足 K 时允许返回少于 K 组。未分组仍走原路径。它不修改 channel weight、RRF、BM25、embedding、HNSW 或召回候选生成。
- 代表命中：组内原始 rank 最小者。
- 组排序：按代表命中的原始 rank；相同则按规范化 group key。
- 组分数：代表命中的原分数；不求和、不平均，避免未经评测的重排公式。
- 返回补充：`group_by`、`group_key`、`hit_count`、`concept_count`、`source_count`、代表命中的 location/score/provenance/ref。
- folder 定义：source path 优先，否则 concept path；取 bundle-relative POSIX 目录，根为 `.`。绝对路径或 `..` 逃逸不得成为目录组，退回独立 concept 组并产生 warning。
- group key 优先级：
  - chunk：当前入口已经暴露的最细命中（有 range 用 range，无 range 用原始 rank 保持命中独立），不承诺恢复被既有检索折叠的底层向量块；
  - concept：合法 `okf_id`，否则 legacy fingerprint；
  - source：合法 `source_path`，否则 concept key；
  - folder：合法 source/concept path 的目录，否则 concept key。

### 4.4 Agent Integration

- 入口：`okf agent plan|apply|status|remove --client cursor|claude-code|codex|all`。
- 仅项目级，不写用户主目录。
- canonical workflow：status → manifest/query → context → 执行任务 → 显式 note/feedback。
- 统一说明 read-only/mutating、token budget、trace、evidence、idempotency 与需要用户确认的写操作。
- 客户端适配：
  - Cursor：项目 MCP JSON + OKF rule；
  - Claude Code：项目 MCP JSON + OKF skill；
  - Codex：项目 TOML managed block + 根 `AGENTS.md` 受管区块；
  - `AGENTS.md`：仅维护带版本 marker 的 OKF 区块。
- 生成的 MCP 启动命令固定为 `okf mcp --repo .`，依赖项目级客户端以仓库根为工作目录；无法保证该契约的客户端版本返回 unsupported，不写绝对机器路径。
- `apply` 只写 OKF 拥有的 key/区块/文件；未知字段和用户文本必须保留。
- `status` 输出 installed/drifted/conflict/missing/unsupported；drifted 表示可识别的 OKF 受管内容与当前确定性渲染不同，conflict 表示所有权不明确或宿主文件无法安全解析。
- `remove` 仅删除带受认可 `OKF_MANAGED` 标记的 JSON key、成对 marker 区块或 OKF ownership header 文件；同名但无标记、未知、不平衡或未受管内容始终拒绝删除。
- 不创建安装状态文件，不保存原文件快照、凭据或知识正文；所有权直接由受管 key/marker/header 判定。

#### 4.4.1 真实 Agent 客户端验收四层模型（S51–S56）

Adapter fixture 和直接 MCP protocol 调用是前置条件，但**不是**官方 Agent 端到端证据。验收按四层分别报告，每层每客户端独立状态：

1. **Adapter fixture**：`okf agent apply` 生成可被标准解析器解析的项目配置。
2. **Official config discovery**：已安装的官方客户端 CLI 检查项目配置并发现 server `okf`，命令为 `okf mcp --repo .`。
3. **Real model MCP calls**：已认证官方 Agent 按序调用 okf_status→okf_manifest→okf_query→okf_context；JSONL 事件流机器校验；无 shell/文件/CLI 绕过。
4. **Final answer / effect**：最终回答包含 canary 事实；持久化 note 验证；错误恢复可观察。

缺少可执行文件、认证、MCP 审批、模型访问或机器校验器的客户端报告为具体 `BLOCKED_*` 状态，**绝不聚合为 PASS**。配置发现始终与模型/工具/回答闭环分开报告。`tools/mcp_call.py` 和 `test_mcp.py` 仅验证 MCP server protocol 层，不计入 Agent 客户端证据。

## 5. 成功指标

### 功能与兼容

- 100% Spec Scenario (S01–S56) 有实现接线和自动化测试。
- 路径/标题重命名后 stable URI 解析成功率 100%。
- 旧 bundle 在不回填 ID 时 parse/lint/query 回归全绿。
- 未指定 `group_by` 时既有 CLI/MCP golden 输出无非预期变化。
- 三客户端 apply 二次执行产生 0 diff；remove 只删除 OKF 拥有内容。JSON 宿主文件保证未知 key 的语义值不变；marker 型 TOML/Markdown 保证受管区块外字节不变。
- 真实 Agent 客户端 E2E 按四层模型报告：Codex 全四层 PASS；Claude/Cursor 在无模型凭据环境为 BLOCKED_AUTH（fail-closed，不聚合为 PASS）。

### 检索质量

- 未分组 hybrid baseline 的 Recall@5、MRR 不低于当前已提交基线。
- concept/source 投影的 relevant-source Recall@5 不低于未投影结果。
- `group_by=source` 的 top-K 中同一 source 占位数恒为 1。
- 同一索引重建两次的投影顺序和 trace 完全一致。

### 性能与资源

- Manifest 通过可注入 Reader 证明正文不被解析/返回，结束符后的缓冲预取不超过 4 KiB。
- 1,000 文件基准中 Manifest 的 bytes-read 与 frontmatter 总量同阶，不随正文体积线性增长；基准结果记录到 EVIDENCE，不设易受 CI 噪声影响的绝对毫秒门槛。
- 分层投影时间复杂度 O(n log n) 或更优、额外内存 O(n)，n 为召回候选数且受现有 candidate limit 约束。
- Agent plan/status 不启动 MCP server、embedding 或索引构建。

## 6. 非目标与边界

- 不做全局跨项目知识库、Web/Desktop UI、远程同步。
- 不让 Manifest 变成无 query 的全文搜索。
- 不让 grouped retrieval 成为第二套 ranking。
- 不把 client adapter 私有格式扩散到 Service/Query 核心。
- 不修改用户非 OKF 配置，不持久化凭据。

## 7. 发布与迁移

1. 先发布可选 `okf_id`、identity ensure 和 resolver。
2. 同一变更内发布 Manifest、group_by 和 Agent Integration；阶段仅表示开发依赖，不是分拆交付。
3. 索引格式升级后 `okf vector status` 显示 incompatible，并提示 `okf vector rebuild`；无索引时保持词法降级。
4. Release Notes 明确：旧 Markdown 无需迁移；只有需要稳定 URI 时才执行显式回填。
5. 合入前运行 `tools/gauntlet.sh`、MCP E2E、客户端 golden/round-trip、检索 eval 和 `conformance.md` 审计。

## 8. 验收定义

只有以下条件全部满足才算**本地实现与能力矩阵完成**：四项功能全部接线；P0-P4 全部任务完成；S01-S50 全绿；S51-S56 均有真实入口、机器测试和明确状态；真实 CLI/MCP/三客户端 fixture 验证通过；检索质量与兼容门槛通过；所有 `partial/gap/BLOCKED_*` 均有原因和解阻动作；最终 `conformance.md` 与最后一次 fresh gauntlet evidence 对齐。

只有 `REQUIRE_ALL_AGENT_MODELS=1 tools/verify-real-agent-e2e.sh` 退出 0，才算**三客户端模型闭环发布验收完成**。当前 Codex 全闭环 PASS；Claude Code/Cursor 因官方客户端未登录保持 `BLOCKED_AUTH`，因此严格发布门禁尚未完成。
