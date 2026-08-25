# Specification: okf-v02-core-types

## Description

OKF v0.2 核心类型系统，扩展 Concept 以支持 provenance（sources）、trust（generated/verified）、lifecycle（status/stale_after）和 Attested Computation 字段，并提供 trust tier 派生、staleness 判断等纯计算方法。

## Requirements

### Requirement: Concept v0.2 字段扩展

`Concept` 结构体 MUST 包含以下 v0.2 字段，所有新增字段均为 optional（omitempty）：

- `Sources []Source` — 来源列表，对应 spec §5.1
- `UsageWindow *UsageWindow` — usage_count 的时间窗口，对应 spec §5.1
- `Generated *GeneratedInfo` — 内容生成信息，对应 spec §5.2
- `Verified []VerificationEvent` — 验证事件列表，对应 spec §5.2
- `Status ConceptStatus` — 生命周期状态，对应 spec §5.4
- `StaleAfter string` — 过期日期（YYYY-MM-DD），对应 spec §5.5
- `Runtime string` — Attested Computation 运行时，对应 spec §10.2
- `Parameters []Parameter` — Attested Computation 参数，对应 spec §10.2
- `Computation string` — Attested Computation 文件路径，对应 spec §10.2
- `Executor *ExecutorRef` — 执行器引用，对应 spec §10.2
- `Attester *AttesterRef` — 验证器引用，对应 spec §10.2

`Title` 字段 MUST 为 optional（v0.2 只有 type 是必填，spec §4.1）。
`Timestamp` 字段 MUST 保留为 legacy v0.1 回退字段。

#### Scenario: v0.2 concept 完整字段序列化

- **WHEN** 一个包含所有 v0.2 字段的 Concept 被序列化为 YAML
- **THEN** 所有字段正确输出，零值字段因 omitempty 不出现
- **AND** 反序列化后字段值与原始一致（round-trip 无损）

#### Scenario: 零值 Concept 序列化

- **WHEN** 一个只有 Type 和 Title 的 Concept 被序列化
- **THEN** 输出仅包含 type 和 title（以及非零值的旧字段）
- **AND** 所有 v0.2 新字段因 omitempty 不出现

### Requirement: Source 类型

`Source` 结构体 MUST 包含：
- `Resource string` — REQUIRED within an entry（spec §5.1）
- `ID string` — 可选，用于 per-claim attribution
- `Title string` — 可选，人类可读标签
- `Author string` — 可选，actor convention（spec §7）
- `UsageCount int` — 可选，使用次数
- `LastModified string` — 可选，YYYY-MM-DD

#### Scenario: Source 含 credibility signals

- **WHEN** Source 包含 author、usage_count、last_modified
- **THEN** 序列化和反序列化后所有信号值正确

### Requirement: GeneratedInfo 类型

`GeneratedInfo` 结构体 MUST 包含：
- `By string` — REQUIRED within generated（spec §5.2），actor convention
- `At string` — 可选，ISO 8601 datetime

#### Scenario: GeneratedInfo 序列化

- **WHEN** GeneratedInfo 包含 by 和 at
- **THEN** YAML 输出为 `generated: { by: ..., at: ... }` 格式
- **AND** 反序列化后值正确

### Requirement: VerificationEvent 类型

`VerificationEvent` 结构体 MUST 包含：
- `By string` — actor convention
- `At string` — 可选，ISO 8601 datetime

#### Scenario: VerificationEvent 列表序列化

- **WHEN** Verified 包含多个 VerificationEvent
- **THEN** YAML 输出为列表格式
- **AND** 反序列化后顺序和值正确

### Requirement: TrustTier 派生

`Concept.TrustTier()` 方法 MUST 按以下规则派生（spec §5.3）：
- 无 `verified` 字段或空列表 → `TrustUnverified`
- `verified` 仅含非 `human:` 前缀的 actor → `TrustMachineConfirmed`
- `verified` 含至少一个 `human:<id>` actor → `TrustHumanReviewed`

#### Scenario: unverified concept

- **WHEN** Concept 的 Verified 为空或 nil
- **THEN** TrustTier() 返回 TrustUnverified

#### Scenario: machine-confirmed concept

- **WHEN** Concept 的 Verified 仅含 `{by: "process:nightly"}`
- **THEN** TrustTier() 返回 TrustMachineConfirmed

#### Scenario: human-reviewed concept

- **WHEN** Concept 的 Verified 含 `{by: "human:alice"}`（无论是否还有其他 verifier）
- **THEN** TrustTier() 返回 TrustHumanReviewed

### Requirement: Staleness 判断

`Concept.IsStale(referenceTime time.Time)` 方法 MUST：
- `StaleAfter` 为空 → 返回 false
- `StaleAfter` 为有效 YYYY-MM-DD 且 `referenceTime >= staleAfter` → 返回 true
- `StaleAfter` 为无效日期 → 返回 false（不 panic）

#### Scenario: fresh concept

- **WHEN** StaleAfter = "2026-12-31"，referenceTime = "2026-08-25"
- **THEN** IsStale 返回 false

#### Scenario: stale concept

- **WHEN** StaleAfter = "2026-06-15"，referenceTime = "2026-08-25"
- **THEN** IsStale 返回 true

#### Scenario: 无 stale_after

- **WHEN** StaleAfter 为空
- **THEN** IsStale 返回 false

#### Scenario: 无效日期格式

- **WHEN** StaleAfter = "not-a-date"
- **THEN** IsStale 返回 false，不 panic

### Requirement: ConceptStatus 枚举

`ConceptStatus` 类型 MUST 支持：
- `StatusDraft` = "draft"
- `StatusStable` = "stable"
- `StatusDeprecated` = "deprecated"
- 空值 → 视为 stable（spec §5.4）

#### Scenario: status 序列化

- **WHEN** Status = StatusDraft
- **THEN** YAML 输出为 `status: draft`
- **AND** 反序列化后值正确

### Requirement: KnowledgeBundle.OKFVersion

`KnowledgeBundle` 结构体 MUST 新增 `OKFVersion string` 字段，用于记录 bundle-root index.md 中声明的 `okf_version`（spec §12）。

#### Scenario: OKFVersion 记录

- **WHEN** 加载一个 bundle-root index.md 含 `okf_version: "0.2"` 的 bundle
- **THEN** KnowledgeBundle.OKFVersion = "0.2"
