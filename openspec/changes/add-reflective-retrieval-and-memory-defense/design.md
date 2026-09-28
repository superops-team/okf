# Design: Reflective Retrieval and Memory Defense

## Context

OKF 已有：稳定 `okf_id`、Git/Markdown 唯一事实源、durable note/event/feedback 写入、provenance、governance、code_refs、memory-check、双时代 MCP，以及时态层（`memory_state` approved/proposed/declined、`updates`/`extends` 关系、`TemporalView` current/history 投影）。本 change 在其上叠加四个模块，不改核心 Concept/Status 模型。

## Architecture

```
┌────────────── Write path ──────────────┐
│ WriteKnowledge(req)                    │
│  ├─ normalizeWriteKnowledgeRequest     │
│  │    └─ hasCredentialField(metadata)   │ ← 现有：metadata 字段名
│  ├─ Screen(content, policy)            │ ← 新增：正文 regex
│  │    ├─ allow → 继续                  │
│  │    ├─ redact → payload.Content=替换后│
│  │    └─ block → ErrMemoryDefenseBlocked│
│  ├─ hashKnowledgePayload(payload)      │ ← redact 后再 hash
│  ├─ idempotency check                   │
│  └─ buildKnowledgeConcept → write file │
└─────────────────────────────────────────┘

┌────────────── Read path ────────────────┐
│ okf_reflect(question)                   │
│  Round1: Service.Query(lexical+semantic)│
│  PoisonGate: drop trap candidates       │
│  Round2: RelationRecall(anchors)        │
│    ├─ extends 双向邻居 (approved only)  │
│    └─ updates 链 → head (History)       │
│  RRF(k=60) fuse → rank                  │
│  evidence < min_evidence → need_clarify │
└─────────────────────────────────────────┘
```

## Module 1: Memory Defense (`pkg/memorydefense`)

### Catalog
16 个内置 detector，每个有 `ID`、`Label`、`Severity`（high/medium）、compiled `*regexp.Regexp`。block 动作只拦 `Severity=high`；redact 动作拦所有命中。

覆盖：GitHub PAT、GitHub OAuth、AWS Access Key、OpenAI API key、Anthropic API key、Google API key、JWT（HS256/RS256 compact）、PEM private key block、Postgres DSN、MySQL DSN、Slack bot token、Slack webhook、Stripe live key、信用卡（Luhn 校验）、SSN（US）、Generic API key 长 token。

### 关键设计决策

- **信用卡 regex 假阳修复（spike bug #1）**：裸 `\b\d{13,16}\b` 会把"1234567890123456 字节"误判。v1 要求：允许分隔符（`-`/空格）、长度 13/15/16、**Luhn 校验通过**才命中。这是行为约束，不是纯 regex。
- **redact 不改变 hash 输入顺序**：`Screen` 在 `normalizeWriteKnowledgeRequest` 返回的 `payload.Content` 上原地替换；之后 `hashKnowledgePayload(payload)` 对替换后内容算 hash。同一 idempotency_key 重试时，第二次 Screen 再次对原始 content 做相同替换（确定性 regex），产生相同 payload → 相同 hash → 命中已有记录。
- **block 零字节落盘**：block 在写文件前返回错误，不创建临时文件，不调用 `buildKnowledgeConcept`。
- **redact 后 content 为空** → `ErrRedactionEmpty`（避免空文档写入）。
- **配置缺失/非法**：`.okf/config.yaml` 不存在或无 `memory_defense` 段 → `enabled=false`（安全默认=不扫描，向后兼容）。`action` 非法值 → 启动期/加载时错误，不静默降级。`detectors: []` = 全部启用；白名单只保留列出的 ID。
- **不泄露原始秘密**：block 错误消息只报 detector ID（如 `github_pat`），不报命中的原文；redaction 记录里只存 detector 列表 + 命中次数，不存原文。

### Config schema

```yaml
memory_defense:
  enabled: true
  action: redact        # redact | block
  detectors: []         # 空 = 全部；白名单如 [github_pat, aws_access_key]
```

## Module 2: Relation Retrieval (`pkg/relationrecall`)

### Problem
现有 `TemporalView` 只索引 `updates` 有向边，不索引 `extends` 边。extends 邻居需要双向召回：
- **出边**：anchor 的 `memory_relation.kind=extends` 指向的目标。
- **入边**：其他概念的 `memory_relation.kind=extends` targets 里包含 anchor（spike bug #2：初版漏了这个方向）。

### Design
新增 `RelationRecall(anchorID string, view *memorymeta.TemporalView, bundle) *RecallResult`：
1. anchor 自身（若 approved 且健康）。
2. 遍历 view.Entries，收集所有 approved 的 extends 边：
   - outgoing: `e.Relation.Kind==extends && targets 包含 anchor` → e 是 anchor 的 extends 子节点。
   - incoming: anchor.Relation.Kind==extends → anchor 的 targets 是 anchor extends 的目标。
3. 沿 updates 链调 `view.History(anchor)`，链上所有 approved 成员返回，head 标 `is_chain_head=true`。
4. proposed/declined/InvalidReason!="" 的条目跳过。
5. depth 硬上限 1（不递归 extends 邻居的邻居）。

### Edge cases
- **cycle/fork/dangling**：`History` 已 fail-closed 返回 stable error；RelationRecall 包装为 warning，不崩，不影响 extends 结果。
- **anchor 不存在**：`ErrMemoryRefNotFound`。
- **fork（多入边 updates）**：`History` 已返回 `ErrAmbiguousUpdateHead`；RelationRecall 返回 extends 部分 + warning。

## Module 3: Reflect Workflow (`pkg/reflect`)

### Design
`Reflect(question, minEvidence, maxRounds, queryFn, relationFn, poisonGateFn)`：
1. Round 1：调现有 `Service.Query`（lexical + semantic RRF），取 top-K 候选。
2. Poison gate：丢弃命中陷阱词表或 state=proposed 的候选。
3. Round 2：从 round-1 存活 anchor 调 `RelationRecall`，拉 extends 邻居 + updates head。
4. RRF(k=60) 融合两轮排名：每轮对每个文档计 `1/(k+rank)`，求和。
5. 证据数 < minEvidence → `need_clarify=true`，返回建议追问。
6. `maxRounds` 硬截断 3。

### Determinism
- 纯内存操作，不调 LLM。
- RRF 排序稳定：同分时按 okf_id 字典序排序。
- 同一份 bundle 同一问题 → 同一 trace。

## Module 4: Trap Evaluation (`pkg/trapeval`)

### Golden case schema
```json
{
  "question": "...",
  "case_type": "single-hop|multi-hop|temporal|knowledge-update|abstain",
  "must_contain": ["fact1", "fact2"],
  "must_not_contain": ["poison phrase"],
  "expected_evidence": ["okf_..."],
  "forbidden_evidence": ["okf_poison..."],
  "abstain_ok": false
}
```

### 三层打分
- **答案级**：生成答案 contains 所有 must_contain（比例 0~1），不含 must_not_contain。
- **证据级**：evidence_refs 都存在；至少一个 expected_evidence；引用 forbidden_evidence → 0.0。
- **弃权级**：abstain_ok=true 时正确 need_clarify 得 1.0，强行答得 0.0。

### Poison trap
每个评测仓库预埋一条 `proposed` 状态恶意记忆。Reflect 默认不返回 proposed；即使被错误召回，poison gate 也丢弃。三层任意一层泄漏 → `poison_blocked < 1.0` → 退出码非零。

## Config loading for memory_defense

`pkg/memorydefense.LoadPolicy(repoRoot)` 读 `<repoRoot>/.okf/config.yaml`，只解 `memory_defense` 子段（yaml.v3 容错）。文件不存在 → `Policy{Enabled:false}`。非法 action → error。这是 repo-local per-repo 策略，不走全局 `~/.okf/config.yaml`。

## Wiring points

| Layer | File | Change |
|---|---|---|
| Service | `pkg/tool/write.go` | normalize 后插入 `Screen` |
| Service | `pkg/tool/service.go` | 新增 `Reflect` / `RelationRecall` 方法 |
| CLI | `cmd/okf/cmd_tool.go` | 新增 `reflect` / `relation` 子命令 |
| CLI | `cmd/okf/cmd_eval.go` | 新增 `trap` 子命令 |
| MCP | `pkg/mcp/tools.go` | 注册 `okf_reflect` / `okf_relation_recall` |
| MCP | `pkg/mcp/modern_handlers.go` | 双时代 handler |
| Agent Skill | 现有 tool manifest | 标注只读/只写 |

## Risks

1. redaction × idempotency：必须端到端测同 key 重试。
2. 文档导入路径（`pkg/convert` → SmartImportSource）是否经过 Screen——v1 只扫 `WriteKnowledge`，文档导入走另一条管道，本期不强制（在 spec 里标为 known gap + 测试验证当前行为）。
3. 10k 概念延迟：复用 temporal benchmark fixture 加测。
4. 中文 PII：v1 不支持，spec 明确声明 + 测试验证不误杀正常中文。
