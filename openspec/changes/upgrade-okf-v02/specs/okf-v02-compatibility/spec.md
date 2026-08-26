# Specification: okf-v02-compatibility

## Description

OKF v0.1→v0.2 向后兼容支持：加载 v0.1 bundle 时自动迁移（timestamp→generated.at，# Citations→sources），保存时可配置是否输出 legacy timestamp，v0.2 bundle 可被 v0.1 消费者加载（新字段被忽略），全链路协议兼容性验证。

## Requirements

### Requirement: v0.1 timestamp → v0.2 generated.at 自动迁移

加载 v0.1 concept 时，当 `generated` 字段不存在但 `timestamp` 存在时，MUST 自动设置 `Generated = {By: "unknown", At: timestamp}`（spec §13.1：consumers MAY fall back to legacy timestamp）。

#### Scenario: v0.1 concept 仅含 timestamp

- **WHEN** 加载一个 v0.1 concept，frontmatter 含 `timestamp: "2026-05-28T22:53:05Z"`，无 generated
- **THEN** Concept.Generated = {By: "unknown", At: "2026-05-28T22:53:05Z"}
- **AND** Concept.Timestamp = "2026-05-28T22:53:05Z"（保留原值）

#### Scenario: v0.2 concept 含 generated 和 timestamp

- **WHEN** 加载一个 v0.2 concept，frontmatter 含 generated 和 timestamp
- **THEN** Concept.Generated 使用 frontmatter 中的值
- **AND** 不被 timestamp 覆盖

#### Scenario: v0.1 concept 无 timestamp

- **WHEN** 加载一个既无 generated 也无 timestamp 的 concept
- **THEN** Concept.Generated = nil

### Requirement: v0.1 # Citations → v0.2 sources 自动迁移

加载 v0.1 concept 时，当 `sources` 字段为空但 body 含 `# Citations` 章节时，SHOULD 从 Citations 列表提取 sources（spec §13.1：consumers MAY still parse legacy # Citations body list）。

#### Scenario: v0.1 body 含 Citations URL 列表

- **WHEN** body 含 `# Citations` 章节，列出 3 个 URL
- **AND** frontmatter 无 sources
- **THEN** Concept.Sources 长度为 3
- **AND** 每个 Source.Resource 为对应 URL
- **AND** Source.ID 自动生成（如 citation-0, citation-1, citation-2）

#### Scenario: v0.2 concept 含 sources 和 Citations

- **WHEN** frontmatter 含 sources，body 也含 # Citations
- **THEN** Concept.Sources 使用 frontmatter 中的值
- **AND** 不从 body 提取

#### Scenario: 无 Citations 章节

- **WHEN** body 不含 # Citations
- **THEN** Concept.Sources 保持为空（如果 frontmatter 也无 sources）

### Requirement: 保存时 legacy timestamp 配置

`SaveOptions` MUST 新增 `LegacyTimestamp bool` 配置：
- `false`（默认）：不输出 `timestamp`，仅输出 `generated.at`
- `true`：同时输出 `timestamp`（值为 `generated.at`），用于 v0.1 消费者回退

#### Scenario: 默认保存不输出 timestamp

- **WHEN** SaveOptions.LegacyTimestamp = false（默认）
- **AND** Concept 含 Generated
- **THEN** 序列化输出含 generated，不含 timestamp

#### Scenario: LegacyTimestamp=true 输出 timestamp

- **WHEN** SaveOptions.LegacyTimestamp = true
- **AND** Concept.Generated.At = "2026-06-20T22:53:05Z"
- **THEN** 序列化输出含 generated 和 timestamp
- **AND** timestamp 值 = "2026-06-20T22:53:05Z"

#### Scenario: 无 Generated 时不输出 timestamp

- **WHEN** Concept.Generated = nil
- **THEN** 无论 LegacyTimestamp 配置如何，都不输出 timestamp

### Requirement: v0.2 bundle 可被 v0.1 消费者加载

v0.2 bundle MUST 可被 v0.1 消费者加载（spec §12：minor version bump 是 backward-compatible additions）。v0.1 消费者会忽略未知字段（sources/generated/verified/status/stale_after/runtime 等）。

#### Scenario: v0.1 消费者加载 v0.2 concept

- **WHEN** 一个 v0.2 concept（含 sources/generated/verified 等新字段）被 v0.1 parser（仅识别 type/title/description/resource/tags/timestamp）加载
- **THEN** v0.1 字段（type/title/description/resource/tags）正确解析
- **AND** 新字段被忽略（不报错）

#### Scenario: v0.2 Attested Computation 被 v0.1 消费者加载

- **WHEN** 一个 type = "Attested Computation" 的 v0.2 concept 被 v0.1 消费者加载
- **THEN** v0.1 消费者将其视为普通 concept（容忍未知 type，spec §4.1）
- **AND** 不报错

### Requirement: 混合 bundle 加载

bundle 中 MAY 同时包含 v0.1 concept 和 v0.2 concept。加载时 MUST 全部正确处理：v0.1 concept 自动迁移，v0.2 concept 保持原样。

#### Scenario: 混合 bundle

- **WHEN** bundle 含 2 个 v0.1 concept（仅 timestamp）和 3 个 v0.2 concept（含 generated）
- **THEN** 所有 5 个 concept 正确加载
- **AND** v0.1 concept 的 Generated 被自动设置
- **AND** v0.2 concept 的 Generated 保持原值

### Requirement: okf_version 声明与消费

bundle-root index.md MAY 声明 `okf_version: "0.2"`（spec §12）。消费者不理解声明版本时 SHOULD 尝试 best-effort consumption 而非拒绝。

#### Scenario: 加载 v0.2 bundle

- **WHEN** bundle-root index.md 含 okf_version: "0.2"
- **THEN** KnowledgeBundle.OKFVersion = "0.2"
- **AND** bundle 正常加载

#### Scenario: 加载无版本声明的 bundle

- **WHEN** bundle 无 bundle-root index.md 或 index.md 无 okf_version
- **THEN** KnowledgeBundle.OKFVersion 为空
- **AND** bundle 正常加载（默认按最新版本处理）

### Requirement: 全链路协议兼容性

v0.1 → v0.2 → v0.1 全链路 MUST 无错误：
1. v0.1 bundle 被 v0.2 消费者加载（自动迁移）
2. v0.2 消费者保存为 v0.2 格式
3. v0.2 bundle 被 v0.1 消费者加载（新字段被忽略）

#### Scenario: 全链路 round-trip

- **WHEN** 一个 v0.1 bundle 经过 v0.2 加载→v0.2 保存→v0.1 加载
- **THEN** 全链路无错误
- **AND** v0.1 消费者能正确识别 type/title/description/resource/tags
- **AND** v0.2 新字段在 v0.1 加载时被忽略

### Requirement: 破坏性变更明确标记

v0.2 的两个 deliberate breaking changes（spec §13.1）MUST 在代码和文档中明确标记：
1. `timestamp` 被 `generated.at` 取代（保留 timestamp 为 legacy，lint OKF020 提示迁移）
2. body `# Citations` 被 `sources` 取代（加载时自动迁移，保存时不输出 Citations）

#### Scenario: legacy 字段标记

- **WHEN** 查看 Concept.Timestamp 字段注释
- **THEN** 注释明确标记为 "legacy v0.1 field, use Generated.At instead"
