# Proposal: Agent Knowledge Discovery and Integration

## Summary

在不改变 OKF v0.2 必填模型、不引入第二事实源、不替换现有 BM25/MiniLM/HNSW/RRF 检索内核的前提下，为 OKF 增加四项协同能力：

1. **Stable Concept ID**：可选扩展字段 `okf_id` 与稳定 URI `okf://concept/<okf_id>`，使 Concept 重命名或移动后仍可被长期引用。
2. **Manifest 元数据发现**：通过 frontmatter-only 扫描低成本列出知识元数据，不读取正文、不初始化 embedding、不隐式构建索引。
3. **分层检索聚合**：在现有各通道已经产生的有界、已评分候选上，于最终输出去重/TopK 整形前提供 `chunk|concept|source|folder` 投影，保留底层分数、来源与确定性 trace；未分组路径不变。
4. **Agent Integration**：以项目级、幂等、可预览、可诊断、可逆的方式配置 Cursor、Claude Code、Codex，并从单一模板生成 MCP 与使用规范。

四项能力构成一条链路：稳定身份 → 低成本发现 → 分层检索 → Agent 工作流编排。

## Motivation

OKF 已有本地文档导入、混合检索、检索评测、MCP、token-bounded context 和 durable capture。当前主要缺口不是新的向量数据库或搜索算法，而是：

- 路径/标题变化会改变索引指纹和外部引用，长期引用不可靠；
- Agent 在冷启动、低 token、索引缺失时缺少低成本候选发现入口；
- 检索结果只能按当前固定粒度消费，难以在证据、Concept、文档和目录之间切换；
- MCP 已存在，但多种 Agent 缺少一致、可维护、不会覆盖用户配置的接入工作流。

## Requirements

### MUST

- `okf_id` 为可选扩展 frontmatter 字段，不成为 OKF v0.2 必填字段；旧 bundle 无需迁移即可继续读取。
- 普通读取不得静默写文件；显式 `okf identity ensure --apply` 才为旧 Concept 回填 ID。
- 新写入的 Concept 仅在最终持久化边界生成/保留 ID；临时 staging 不生成随机 ID，重复刷新/重导入同一受管目标不得改变已有合法 ID。
- 稳定 URI 在文件移动/重命名后仍解析到当前路径；重复 ID 必须 fail closed。
- 向量索引切换为 identity-aware key 时显式提升格式版本；旧索引报可操作的 rebuild 提示，不静默返回错误结果。
- Manifest 只解析 frontmatter 和文件元信息；允许固定缓冲的有限预取，但不得解析或返回 Markdown 正文、初始化向量模型或触发索引构建。
- Manifest、grouped query 的 CLI 与 MCP 必须复用 `pkg/tool.Service` 或共享纯函数，不复制业务逻辑。
- 分层聚合仅改变结果投影，不改变召回、RRF 权重、embedding、HNSW 或 BM25 排名算法。
- 未显式传 `group_by` 时保留现有 CLI/MCP 行为和响应字段语义。
- Agent 配置必须支持 plan/apply/status/remove，严格限制在仓库内，保留未知配置和用户规则，检测漂移后拒绝破坏性覆盖。
- Cursor、Claude Code、Codex 的适配内容由一个 canonical workflow 模板生成；契约 fixture 和 golden 测试防止三套说明漂移。
- 所有 Requirement 均有真实 CLI/MCP/Service 接线与针对性测试；所有 Scenario 映射到测试。

### SHOULD

- Stable ID 使用标准库 `crypto/rand` 生成，不引入 UUID 依赖。
- Manifest 默认最多返回 100 条、最大 500 条，按 bundle-relative path 稳定排序，并支持 offset/limit 和元数据过滤。
- 聚合结果包含代表性命中、命中数、Concept 数、source 数、稳定引用及原始 trace。
- Agent 集成仅支持项目级配置，避免未经授权修改用户主目录或全局客户端设置。

### MAY

- 后续增加全局 catalog、Web/Desktop UI 或其他 Agent adapter。
- 后续在独立变更中增加 stable ID alias/history；本变更不保存路径历史。

## Non-Goals

- 不修改 Google OKF v0.2 标准字段或将 `okf_id` 声称为标准字段。
- 不引入 SQLite/LanceDB 作为第二事实源或替换 Markdown Concept。
- 不替换现有 BM25、MiniLM、HNSW、weighted RRF、chunk 生命周期或检索评测体系。
- 不引入外部 embedding API、Node、Rust、Tauri、Python、CGO 或新的运行时。
- 不新增 Web/Desktop UI，不实现跨项目全局知识目录。
- 不复制 OpenContext 的 `.ideas` 协议、固定 0.7/0.3 权重或阻断式“先写知识再回答”提示。
- 不在 Agent 配置中写入 token、密钥、账号或绝对用户主目录。

## Dependency Order and Completion Boundary

`P0 Stable ID → P1 Manifest → P2 分层聚合 → P3 Agent Integration → P4 全量验证与一致性审计`。

P0-P4 仅表示依赖顺序。四项能力、测试、文档、真实 CLI/MCP 验证和 `conformance.md` 全部完成后，整项变更才可宣称完成；不得只交付 P0 或 M1 子集。

## Expected Impact

- 新增：`pkg/identity`、`pkg/manifest`、`pkg/agentconfig` 和相应测试。
- 修改：`pkg/query`、`pkg/tool`、`pkg/mcp`、`cmd/okf` 的窄接口扩展。
- 修复：CLI/MCP 的 `okf.Concept → query.Concept` 转换必须复制 `CustomFields`，避免 `okf_id`、`source_path` 等元数据丢失。
- 兼容：旧 Markdown 继续可解析；未使用新参数时现有命令输出和 MCP JSON 字段保持兼容；向量索引格式升级需要显式 rebuild。
- 依赖：默认不新增第三方 Go 依赖。

## Risks

- **ID 冲突或非法 ID**：identity-aware 操作 fail closed；普通解析保持兼容；`identity ensure` 在任何写入前完成全量冲突预检。
- **索引格式变化**：版本提升、状态可见、旧索引明确提示 rebuild，检索仍可降级词法路径。
- **元数据读取退化为全文解析**：以 256 KiB 上限、4 KiB 缓冲预取界限和 instrumented-reader 测试阻断。
- **聚合掩盖证据**：代表命中保留原始 location/score/provenance，`chunk` 模式可显式展开。
- **客户端配置损坏**：plan-first、自描述 ownership、语义合并、确定性渲染漂移检测和进程内原始字节回滚测试。
- **多入口契约漂移**：CLI/MCP 共用 Service，三类 Agent 共用 canonical workflow，并以 schema/golden 测试锁定。
