# Specification: okf-v02-parser

## Description

OKF v0.2 解析器升级，支持所有 v0.2 frontmatter 字段、verified 字段灵活解析（list/single mapping）、保留文件名识别（index.md/log.md）、bundle-root okf_version 解析，以及 v0.1→v0.2 自动回退迁移。

## Requirements

### Requirement: v0.2 frontmatter 完整解析

解析器 MUST 正确解析所有 v0.2 frontmatter 字段：sources、usage_window、generated、verified、status、stale_after、runtime、parameters、computation、executor、attester。

#### Scenario: 完整 v0.2 concept 解析

- **WHEN** 解析一个含所有 v0.2 字段的 markdown 文件
- **THEN** 返回的 Concept 所有字段值正确
- **AND** CustomFields 保留未知字段

#### Scenario: sources 含 credibility signals

- **WHEN** frontmatter 含 sources 列表，每个条目含 author、usage_count、last_modified
- **THEN** Concept.Sources 正确解析所有信号
- **AND** usage_window 作为兄弟字段正确解析

### Requirement: verified 字段灵活解析

`verified` 字段 MUST 支持两种形式（spec §5.2）：
- 列表形式：`verified: [{by: ..., at: ...}, ...]`
- 单个 mapping：`verified: {by: ..., at: ...}` → MUST 当作一元列表

#### Scenario: verified 列表形式

- **WHEN** frontmatter 中 verified 是 YAML 列表
- **THEN** Concept.Verified 解析为对应长度的切片

#### Scenario: verified 单个 mapping 形式

- **WHEN** frontmatter 中 verified 是单个 mapping `{by: "human:alice", at: "..."}`
- **THEN** Concept.Verified 解析为一元列表，包含该 VerificationEvent

#### Scenario: verified 空值

- **WHEN** frontmatter 中无 verified 字段
- **THEN** Concept.Verified 为 nil

#### Scenario: verified 无效格式降级

- **WHEN** frontmatter 中 verified 是无法识别的格式（如字符串）
- **THEN** 解析器不返回 error，Verified 降级为 nil

### Requirement: title 不再必填

解析器 MUST NOT 因缺少 title 而返回 error（spec §4.1：只有 type 是必填）。title 为空时，MUST 从文件名派生显示名。

#### Scenario: 无 title 的 concept

- **WHEN** 解析一个 frontmatter 只有 type、没有 title 的文件
- **THEN** 解析成功，不返回 error
- **AND** Concept.Title 从文件名派生（如 `customer-orders.md` → "customer orders"）

#### Scenario: 无 type 的 concept

- **WHEN** 解析一个 frontmatter 没有 type 的文件
- **THEN** 返回 ParseError（type 是唯一必填字段，spec §4.1）

### Requirement: 保留文件名识别

解析器 MUST 识别保留文件名 `index.md` 和 `log.md`（spec §3.1），这些文件 MUST NOT 被当作 Concept 解析。

#### Scenario: index.md 识别

- **WHEN** 调用 `IsReservedFilename("path/to/index.md")`
- **THEN** 返回 true

#### Scenario: log.md 识别

- **WHEN** 调用 `IsReservedFilename("path/to/log.md")`
- **THEN** 返回 true

#### Scenario: 普通 concept 文件

- **WHEN** 调用 `IsReservedFilename("path/to/concept.md")`
- **THEN** 返回 false

#### Scenario: LoadBundle 跳过保留文件

- **WHEN** 加载一个包含 index.md 和 log.md 的 bundle
- **THEN** 这些文件不出现在 Concepts 列表中
- **AND** 不返回解析错误

### Requirement: bundle-root index.md 的 okf_version 解析

bundle-root 的 `index.md` MAY 携带 `okf_version` frontmatter key（spec §12），这是 index.md 唯一允许有 frontmatter 的情况。加载时 MUST 读取并设置到 `KnowledgeBundle.OKFVersion`。

#### Scenario: bundle-root index 含 okf_version

- **WHEN** bundle 根目录的 index.md frontmatter 含 `okf_version: "0.2"`
- **THEN** KnowledgeBundle.OKFVersion = "0.2"

#### Scenario: 子目录 index 无 okf_version

- **WHEN** 子目录的 index.md 含 frontmatter
- **THEN** 解析器忽略其 frontmatter（仅 bundle-root index 允许 frontmatter）

### Requirement: v0.1 回退：timestamp → generated.at

当 `generated` 字段不存在但 `timestamp` 存在时，解析器 MUST 自动设置 `Generated = {By: "unknown", At: timestamp}`（spec §13.1 breaking change，消费者 MAY 回退）。

#### Scenario: v0.1 concept 含 timestamp

- **WHEN** 解析一个 v0.1 concept，frontmatter 含 `timestamp: "2026-05-28T..."`，无 generated
- **THEN** Concept.Generated 被设置为 `{By: "unknown", At: "2026-05-28T..."}`
- **AND** Concept.Timestamp 保留原值

#### Scenario: v0.2 concept 含 generated

- **WHEN** 解析一个 v0.2 concept，frontmatter 含 generated，同时也含 timestamp
- **THEN** Concept.Generated 使用 frontmatter 中的值，不被 timestamp 覆盖

### Requirement: v0.1 回退：# Citations → sources

当 `sources` 字段为空但 body 含 `# Citations` 章节时，解析器 SHOULD 从 Citations 列表提取 sources（spec §13.1 breaking change，消费者 MAY 仍解析 legacy # Citations）。

#### Scenario: v0.1 body 含 Citations

- **WHEN** 解析一个 v0.1 concept，body 含 `# Citations` 章节，列出多个 URL，frontmatter 无 sources
- **THEN** Concept.Sources 被填充，每个条目 Resource 为对应 URL
- **AND** ID 自动生成（如 citation-0, citation-1）

#### Scenario: v0.2 concept 含 sources

- **WHEN** 解析一个 v0.2 concept，frontmatter 含 sources，body 也含 # Citations
- **THEN** Concept.Sources 使用 frontmatter 中的值，不从 body 提取

### Requirement: v0.2 字段序列化

`SerializeConcept` MUST 支持所有 v0.2 字段的序列化，零值字段因 omitempty 不输出。

#### Scenario: v0.2 concept round-trip

- **WHEN** 一个含所有 v0.2 字段的 Concept 被序列化再反序列化
- **THEN** 所有字段值与原始一致（无损 round-trip）

#### Scenario: 最小 concept 序列化

- **WHEN** 一个只有 type 的 Concept 被序列化
- **THEN** 输出仅含 `type: ...`，所有可选字段不出现
