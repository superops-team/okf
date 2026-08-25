# Specification: okf-v02-index-log

## Description

OKF v0.2 保留文件名支持：`index.md`（目录列表，spec §8）和 `log.md`（更新历史，spec §9）。包括类型定义、解析、自动生成，以及 bundle-root index.md 的 `okf_version` frontmatter 支持。

## Requirements

### Requirement: IndexFile 类型

`IndexFile` 结构体 MUST 包含：
- `OKFVersion string` — 仅 bundle-root index 可有（spec §12）
- `Sections []IndexSection` — 分组列表
- `FilePath string` — 文件路径

`IndexSection` MUST 包含：
- `Heading string` — 分组标题
- `Entries []IndexEntry` — 条目列表

`IndexEntry` MUST 包含：
- `Title string` — concept 标题
- `URL string` — 相对路径
- `Description string` — concept 描述

#### Scenario: IndexFile 序列化

- **WHEN** 创建一个含 sections 的 IndexFile
- **THEN** 所有字段正确设置

### Requirement: 解析 index.md

`ParseIndexFile(path)` MUST 解析 index.md 文件（spec §8）。

- index.md 通常无 frontmatter
- bundle-root index.md MAY 含 `okf_version` frontmatter（唯一允许 frontmatter 的情况）
- body 使用分组列表格式：`# Heading` 下接 `* [Title](url) - description`

#### Scenario: 解析普通 index.md

- **WHEN** 解析一个无 frontmatter、含 sections 的 index.md
- **THEN** IndexFile.Sections 正确解析所有分组和条目
- **AND** OKFVersion 为空

#### Scenario: 解析 bundle-root index.md 含 okf_version

- **WHEN** 解析 bundle-root 的 index.md，frontmatter 含 `okf_version: "0.2"`
- **THEN** IndexFile.OKFVersion = "0.2"
- **AND** Sections 从 body 正确解析

#### Scenario: index.md 条目解析

- **WHEN** index.md body 含 `* [Customer Orders](orders.md) - One row per order`
- **THEN** IndexEntry.Title = "Customer Orders", URL = "orders.md", Description = "One row per order"

### Requirement: 自动生成 index.md

`GenerateIndex(bundle *KnowledgeBundle, dir string) (*IndexFile, error)` MUST 从指定目录的 concept frontmatter 聚合生成 index（spec §8：producers MAY generate index.md automatically）。

- 条目 SHOULD 包含 concept 的 title 和 description
- 按子目录分组，每个子目录为一个 section
- 排序确定（按文件路径字母序）

#### Scenario: 生成单目录 index

- **WHEN** 目录下有 3 个 concept
- **THEN** GenerateIndex 返回 1 个 section，含 3 个条目
- **AND** 每个条目含对应 concept 的 title 和 description

#### Scenario: 生成多子目录 index

- **WHEN** 目录下有 2 个子目录，各含 concept
- **THEN** GenerateIndex 返回 2 个 sections，分别对应子目录
- **AND** 条目按路径字母序排列

#### Scenario: 生成空目录 index

- **WHEN** 目录下无 concept
- **THEN** GenerateIndex 返回空 sections，不报错

### Requirement: LogFile 类型

`LogFile` 结构体 MUST 包含：
- `Entries []LogEntry` — 日志条目列表
- `FilePath string` — 文件路径

`LogEntry` MUST 包含：
- `Date string` — YYYY-MM-DD（spec §9：date headings MUST use ISO 8601）
- `Action string` — Update/Creation/Deprecation（约定，非强制）
- `Message string` — 描述文本

#### Scenario: LogFile 解析

- **WHEN** 创建一个含 entries 的 LogFile
- **THEN** 所有字段正确设置

### Requirement: 解析 log.md

`ParseLogFile(path)` MUST 解析 log.md 文件（spec §9）。

- 格式为日期分组的扁平列表，最新在前
- 日期标题 MUST 为 `## YYYY-MM-DD`
- 条目格式：`* **Action**: message`

#### Scenario: 解析 log.md

- **WHEN** log.md 含 `## 2026-05-22` 下接 `* **Update**: Added ...`
- **THEN** LogFile.Entries 正确解析
- **AND** LogEntry.Date = "2026-05-22", Action = "Update", Message = "Added ..."

#### Scenario: 多日期 log.md

- **WHEN** log.md 含多个日期分组
- **THEN** Entries 按文件中出现顺序（最新在前）解析

### Requirement: LoadBundle 集成保留文件

`LoadBundle` MUST 在加载时跳过 `index.md` 和 `log.md`，不将它们解析为 Concept（spec §3.1：保留文件名 MUST NOT 用于 concept documents）。

#### Scenario: bundle 含 index.md 和 log.md

- **WHEN** 加载一个含 index.md、log.md 和普通 concept 的 bundle
- **THEN** Concepts 列表仅含普通 concept
- **AND** index.md 和 log.md 不出现在 Concepts 中
- **AND** 不返回解析错误

#### Scenario: bundle-root index 的 okf_version

- **WHEN** bundle-root index.md 含 okf_version: "0.2"
- **THEN** KnowledgeBundle.OKFVersion = "0.2"
- **AND** index.md 不被解析为 Concept
