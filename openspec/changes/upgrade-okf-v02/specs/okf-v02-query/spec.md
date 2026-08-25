# Specification: okf-v02-query

## Description

OKF v0.2 查询引擎升级，支持按 trust tier、status、stale 状态、sources 过滤，全文搜索范围扩展到 sources 字段，Stats 新增 trust tier 和 status 统计。

## Requirements

### Requirement: Query 结构体扩展

`Query` 结构体 MUST 新增以下过滤维度：
- `TrustTiers []TrustTier` — 按信任等级过滤
- `Statuses []ConceptStatus` — 按状态过滤
- `ExcludeStale bool` — 排除过期 concept
- `Sources []string` — 按 source id 或 resource 过滤

#### Scenario: Query 含新字段

- **WHEN** 创建一个含 TrustTiers、Statuses、ExcludeStale 的 Query
- **THEN** 所有字段正确设置

### Requirement: 按 trust tier 过滤

`KnowledgeBundle` MUST 提供按 trust tier 过滤 concept 的能力。

#### Scenario: 过滤 human-reviewed

- **WHEN** bundle 含 unverified、machine-confirmed、human-reviewed 三种 concept
- **AND** 按 TrustHumanReviewed 过滤
- **THEN** 仅返回含 human: verifier 的 concept

#### Scenario: 过滤多个 trust tier

- **WHEN** 按 [TrustMachineConfirmed, TrustHumanReviewed] 过滤
- **THEN** 返回 machine-confirmed 和 human-reviewed 的 concept，排除 unverified

#### Scenario: 空 TrustTiers 不过滤

- **WHEN** TrustTiers 为空
- **THEN** 不按 trust tier 过滤，返回所有 concept

### Requirement: 按 status 过滤

`KnowledgeBundle` MUST 提供按 status 过滤 concept 的能力。

#### Scenario: 过滤 draft

- **WHEN** bundle 含 draft、stable、deprecated 三种 concept
- **AND** 按 StatusDraft 过滤
- **THEN** 仅返回 status = draft 的 concept

#### Scenario: 空 status 视为 stable

- **WHEN** concept 的 status 字段为空
- **AND** 按 StatusStable 过滤
- **THEN** 该 concept 被包含（spec §5.4：absent status ⇒ stable）

### Requirement: 排除 stale concept

`KnowledgeBundle` MUST 提供排除 stale concept 的能力（`ExcludeStale`）。

#### Scenario: 排除 stale

- **WHEN** bundle 含 fresh 和 stale 两种 concept
- **AND** ExcludeStale = true，referenceTime = today
- **THEN** 仅返回 IsStale(referenceTime) = false 的 concept

#### Scenario: 不排除 stale

- **WHEN** ExcludeStale = false
- **THEN** 返回所有 concept，包括 stale

### Requirement: 按 sources 过滤

`KnowledgeBundle` MUST 提供按 source id 或 resource 过滤 concept 的能力。

#### Scenario: 按 source id 过滤

- **WHEN** concept 的 sources 含 `{id: "rev-policy", resource: "..."}`
- **AND** Sources 过滤条件含 "rev-policy"
- **THEN** 该 concept 被返回

#### Scenario: 按 source resource 过滤

- **WHEN** concept 的 sources 含 `{resource: "https://wiki.acme/..."}`
- **AND** Sources 过滤条件含该 URL
- **THEN** 该 concept 被返回

### Requirement: Search 全文搜索扩展

`KnowledgeBundle.Search(query)` 的全文搜索范围 MUST 扩展到：
- title、description、content（现有）
- sources[].title（新增）
- sources[].author（新增）

#### Scenario: 搜索 source title

- **WHEN** concept 的 sources 含 `{title: "Revenue recognition policy"}`
- **AND** Search("revenue recognition")
- **THEN** 该 concept 被返回

#### Scenario: 搜索 source author

- **WHEN** concept 的 sources 含 `{author: "team:finance-fpa"}`
- **AND** Search("finance-fpa")
- **THEN** 该 concept 被返回

### Requirement: Stats 扩展

`BundleStats` MUST 新增以下统计：
- `TrustTierCounts map[TrustTier]int` — 各 trust tier 的 concept 数量
- `StatusCounts map[ConceptStatus]int` — 各 status 的 concept 数量
- `StaleCount int` — stale concept 数量（基于当前时间）
- `AttestedComputationCount int` — Attested Computation 类型的 concept 数量

#### Scenario: Stats 含 trust tier 统计

- **WHEN** bundle 含 2 个 unverified、1 个 machine-confirmed、1 个 human-reviewed
- **THEN** Stats.TrustTierCounts 正确反映各 tier 数量

#### Scenario: Stats 含 status 统计

- **WHEN** bundle 含 1 个 draft、2 个 stable、1 个 deprecated
- **THEN** Stats.StatusCounts 正确反映各 status 数量

#### Scenario: Stats 含 stale 统计

- **WHEN** bundle 含 1 个 stale concept（stale_after 已过）
- **THEN** Stats.StaleCount = 1
