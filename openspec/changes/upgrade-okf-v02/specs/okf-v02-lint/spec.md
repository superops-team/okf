# Specification: okf-v02-lint

## Description

OKF v0.2 lint 规则升级，对齐 spec §11 conformance：title 从 required 降级为 recommended，timestamp 改为 generated.at，新增 sources/generated/verified/status/stale_after/Attested Computation 字段校验规则，所有 v0.2 新字段规则均为 Warning/Info（除 Attested Computation 的 runtime 必填为 Error）。

## Requirements

### Requirement: OKF002 title 降级

OKF002 规则 MUST 从 Error 降级为 Warning（spec §4.1：只有 type 是必填，title 是 recommended）。

#### Scenario: 无 title 的 concept

- **WHEN** lint 一个无 title 的 concept
- **THEN** OKF002 触发为 Warning（不是 Error）
- **AND** 消息提示 "title is recommended, may be derived from filename"

#### Scenario: 有 title 的 concept

- **WHEN** lint 一个有 title 的 concept
- **THEN** OKF002 不触发

### Requirement: OKF004 type 小写强制移除

OKF004 规则 MUST NOT 强制 type 全小写（spec §4.1：type 值不注册，示例含 "BigQuery Table"）。改为仅在 type 为空时与 OKF001 合并提示。

#### Scenario: 大小写混合的 type

- **WHEN** lint 一个 type = "BigQuery Table" 的 concept
- **THEN** OKF004 不触发

#### Scenario: 全小写 type

- **WHEN** lint 一个 type = "metric" 的 concept
- **THEN** OKF004 不触发

### Requirement: OKF005 改为 generated.at 校验

OKF005 规则 MUST 从检查 `timestamp` 改为检查 `generated.at` ISO 8601 格式（spec §5.2）。

#### Scenario: generated.at 有效 ISO 8601

- **WHEN** concept 含 `generated: {by: "agent/v1", at: "2026-06-20T22:53:05Z"}`
- **THEN** OKF005 不触发

#### Scenario: generated.at 无效格式

- **WHEN** concept 含 `generated: {by: "agent/v1", at: "not-a-date"}`
- **THEN** OKF005 触发为 Warning

#### Scenario: 无 generated 字段

- **WHEN** concept 无 generated 字段
- **THEN** OKF005 不触发（generated 是 optional）

### Requirement: OKF020 legacy timestamp 提示

新增 OKF020 规则（Info）：当 concept 含 `timestamp` 但无 `generated` 时，提示建议迁移到 `generated.at`（spec §13.1 breaking change）。

#### Scenario: 仅含 timestamp

- **WHEN** concept 含 timestamp 但无 generated
- **THEN** OKF020 触发为 Info，提示 "timestamp is legacy v0.1, migrate to generated.at"

#### Scenario: 含 generated

- **WHEN** concept 含 generated（无论是否还有 timestamp）
- **THEN** OKF020 不触发

### Requirement: OKF012 sources[].resource 必填校验

新增 OKF012 规则（Warning）：当 sources 存在时，每个条目 MUST 含非空 resource（spec §5.1：resource REQUIRED within an entry）。

#### Scenario: source 含 resource

- **WHEN** sources 条目含非空 resource
- **THEN** OKF012 不触发

#### Scenario: source 缺 resource

- **WHEN** sources 条目 resource 为空
- **THEN** OKF012 触发为 Warning

#### Scenario: 无 sources

- **WHEN** concept 无 sources
- **THEN** OKF012 不触发

### Requirement: OKF013 generated.by 必填校验

新增 OKF013 规则（Warning）：当 generated 存在时，`by` 字段 MUST 非空（spec §5.2：generated.by REQUIRED within generated）。

#### Scenario: generated 含 by

- **WHEN** generated.by 非空
- **THEN** OKF013 不触发

#### Scenario: generated 缺 by

- **WHEN** generated 存在但 by 为空
- **THEN** OKF013 触发为 Warning

### Requirement: OKF014 verified[].by actor 格式校验

新增 OKF014 规则（Warning）：verified 条目的 by 字段 SHOULD 符合 actor convention（spec §7）：`human:<id>`、`process:<id>` 或 `<producer>/<version>`。

#### Scenario: valid human actor

- **WHEN** verified[].by = "human:alice"
- **THEN** OKF014 不触发

#### Scenario: valid process actor

- **WHEN** verified[].by = "process:nightly"
- **THEN** OKF014 不触发

#### Scenario: valid agent actor

- **WHEN** verified[].by = "reference_agent/gemini-2.5-pro"
- **THEN** OKF014 不触发

#### Scenario: invalid actor format

- **WHEN** verified[].by = "just a name"
- **THEN** OKF014 触发为 Warning

### Requirement: OKF015 status 枚举校验

新增 OKF015 规则（Warning）：status 字段 SHOULD 为 draft/stable/deprecated 之一（spec §5.4）。

#### Scenario: valid status

- **WHEN** status = "draft" 或 "stable" 或 "deprecated"
- **THEN** OKF015 不触发

#### Scenario: invalid status

- **WHEN** status = "unknown"
- **THEN** OKF015 触发为 Warning

#### Scenario: 空 status

- **WHEN** status 为空
- **THEN** OKF015 不触发（默认 stable，spec §5.4）

### Requirement: OKF016 stale_after 日期格式校验

新增 OKF016 规则（Warning）：stale_after 字段 SHOULD 为 YYYY-MM-DD 格式（spec §5.5）。

#### Scenario: valid stale_after

- **WHEN** stale_after = "2026-09-23"
- **THEN** OKF016 不触发

#### Scenario: invalid stale_after

- **WHEN** stale_after = "09/23/2026"
- **THEN** OKF016 触发为 Warning

### Requirement: OKF017 Attested Computation runtime 必填

新增 OKF017 规则（Error）：当 concept type 为 "Attested Computation" 时，`runtime` 字段 MUST 非空（spec §10.2：runtime REQUIRED for this type）。

#### Scenario: Attested Computation 含 runtime

- **WHEN** type = "Attested Computation" 且 runtime = "bigquery"
- **THEN** OKF017 不触发

#### Scenario: Attested Computation 缺 runtime

- **WHEN** type = "Attested Computation" 且 runtime 为空
- **THEN** OKF017 触发为 Error

#### Scenario: 非 Attested Computation type

- **WHEN** type = "Metric" 且 runtime 为空
- **THEN** OKF017 不触发

### Requirement: OKF018 parameters 字段校验

新增 OKF018 规则（Warning）：当 parameters 存在时，每个条目 SHOULD 含非空 name 和 type（spec §10.2）。

#### Scenario: valid parameters

- **WHEN** parameters 含 `{name: "year", type: "integer", required: true}`
- **THEN** OKF018 不触发

#### Scenario: parameter 缺 name

- **WHEN** parameters 条目 name 为空
- **THEN** OKF018 触发为 Warning

### Requirement: OKF019 executor/attester resource 校验

新增 OKF019 规则（Warning）：当 executor 或 attester 存在时，其 resource 字段 MUST 非空（spec §10.2）。

#### Scenario: executor 含 resource

- **WHEN** executor.resource 非空
- **THEN** OKF019 不触发

#### Scenario: executor 缺 resource

- **WHEN** executor 存在但 resource 为空
- **THEN** OKF019 触发为 Warning

### Requirement: conformance 对齐

lint 规则 MUST NOT 因以下原因返回 Error（spec §11：消费者 MUST NOT reject）：
- 缺少可选 frontmatter 字段
- 未知 type 值
- 未知额外 frontmatter key
- 断链
- 缺少 index.md

#### Scenario: 未知 type

- **WHEN** lint 一个 type = "UnknownType" 的 concept
- **THEN** 不返回 Error（最多 Warning 提示）

#### Scenario: 未知 frontmatter key

- **WHEN** concept 含自定义字段 `custom_field: value`
- **THEN** 不返回 Error
