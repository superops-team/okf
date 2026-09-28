# Proposal: Reflective Retrieval and Memory Defense

## Summary

在已交付的时态记忆层（`memory_state` / `memory_confidence` / `memory_relation` / `memory_review`，见 `add-temporal-memory-relations`）之上，补齐四个缺口：

1. **Memory Defense**：写入路径前置 regex 扫描，对正文 content 做敏感信息检测，动作 `redact`（替换为 `[REDACTED:type]`）或 `block`（零字节落盘），per-repo 策略走 `.okf/config.yaml`。
2. **Reflect Workflow**：有界两轮只读检索（轮 1 词法/语义 → 轮 2 关系边扩展），RRF(k=60) 融合，证据不足返回 `need_clarify` 弃权；进程内同步、不调 LLM、可重放。
3. **Relation Retrieval**：从 anchor `okf_id` 出发，双向召回 approved `extends` 邻居 + 沿 `updates` 链走到当前 head；proposed/declined 不进默认结果。
4. **Trap Evaluation**：本地 golden 评测集，答案级 + 证据级 + 弃权级三层打分；每个仓库预埋一条 proposed 恶意记忆作为 poison trap；deterministic gate 默认不依赖 LLM judge。

## Motivation

- **写入路径不扫正文**：现状 `pkg/tool/write.go` 的 `hasCredentialField` 只按 metadata 字段名粗粒度拒绝；content 正文里的 GitHub PAT、AWS key、JWT、PEM、信用卡、数据库连接串完全不扫描。
- **检索单轮**：`query.Search` 一次召回即返回，不沿关系边扩展，证据不足不追问。
- **关系无一等召回操作**：只能走 history 视图翻链，没有"从这条记忆出发拉兄弟记忆"的 agent 级操作。
- **评测无答案级/陷阱级**：只有文档级 IR 指标（Recall@K/MRR/NDCG），没有答案正确性、证据支持性、投毒拦截。

## Non-goals（架构红线，第一阶段不做）

- 不引入 PostgreSQL / 图数据库 / Redis / 第二向量库 / 后台 Worker / 守护进程 / 定时 consolidation。
- 不引入第二事实源；所有数据仍是 Git 仓库里的 Markdown + frontmatter。
- 不做自动无审核写入；proposed 仍须经 `memory_review` approve 才进 current 视图。
- 不引入在线 LLM 做 redaction 或 trap judge；v1 是纯 Go regex + deterministic gate。
- 不做跨库/跨 bank 隔离；不做任意深度图遍历（关系召回硬上限 2 跳）。
- 不做中文 PII（手机号/身份证）regex——v1 明确不支持，避免误杀。
- 不做 cross-encoder rerank；不做 Hindsight 式 bank_id 多租户。

## Decisions

- **配置位置**：`.okf/config.yaml` 的 `memory_defense` 段（repo-local，per-repo）；缺省 `enabled: false`，向后兼容。
- **redact 时机**：在 `normalizeWriteKnowledgeRequest` 之后、`hashKnowledgePayload` 之前对 `payload.Content` 做替换；先 redact 再算 hash，保证幂等重试语义。
- **RRF 常数**：k=60（与现有语义 RRF 一致）。
- **Reflect 轮数**：硬上限 3，默认 2；`min_evidence` 默认 2。
- **关系召回深度**：extends 只拉直接邻居（1 跳），updates 链走到 head（复用 `TemporalView.History`）。
- **Trap gate**：deterministic——proposed 状态概念天然不进 current 视图；poison gate 额外丢弃命中配置陷阱词表的候选；默认不依赖 LLM。

## Consequences

- 新增 `pkg/memorydefense`（regex catalog + Screen）、`pkg/relationrecall`（extends 邻接 + updates 链）、`pkg/reflect`（两轮编排 + RRF）、`pkg/trapeval`（三层打分 + golden loader）。
- `Service.WriteKnowledge` 插入 Screen 调用；`Service` 新增 `Reflect` / `RelationRecall` 只读方法。
- CLI 新增 `okf tool reflect` / `okf tool relation` / `okf eval trap`；MCP 新增 `okf_reflect` / `okf_relation_recall`（双时代）。
- 向后兼容：未配置 `memory_defense` 时行为与现状字节级一致；proposed 记忆审核流程不变。
