# Specification: okf-v02-attested-computation

## Description

OKF v0.2 Attested Computation concept 完整支持，包括 type 识别、runtime/parameters/computation/executor/attester 字段、body # Computation 章节提取、computation 文件路径回退，以及 Appendix A 官方示例验证。

## Requirements

### Requirement: Attested Computation 类型识别

`Concept.IsAttestedComputation()` 方法 MUST 通过 `Type == "Attested Computation"` 识别（spec §10.1）。

#### Scenario: Attested Computation type

- **WHEN** Concept.Type = "Attested Computation"
- **THEN** IsAttestedComputation() 返回 true

#### Scenario: 其他 type

- **WHEN** Concept.Type = "Metric"
- **THEN** IsAttestedComputation() 返回 false

### Requirement: runtime 字段

Attested Computation concept 的 `runtime` 字段 MUST 被正确解析和序列化（spec §10.2：runtime REQUIRED for this type）。

#### Scenario: bigquery runtime

- **WHEN** frontmatter 含 `runtime: bigquery`
- **THEN** Concept.Runtime = "bigquery"

#### Scenario: dbt runtime

- **WHEN** frontmatter 含 `runtime: dbt`
- **THEN** Concept.Runtime = "dbt"

#### Scenario: python runtime

- **WHEN** frontmatter 含 `runtime: python`
- **THEN** Concept.Runtime = "python"

### Requirement: parameters 字段

Attested Computation concept 的 `parameters` 字段 MUST 被正确解析为 `[]Parameter`，每个 Parameter 含 name、type、required（spec §10.2）。

#### Scenario: 单个 parameter

- **WHEN** frontmatter 含 `parameters: [{name: year, type: integer, required: true}]`
- **THEN** Concept.Parameters 长度为 1
- **AND** Parameters[0].Name = "year", Type = "integer", Required = true

#### Scenario: 多个 parameters

- **WHEN** frontmatter 含 2 个 parameters（year 和 segment）
- **THEN** Concept.Parameters 长度为 2，顺序正确

#### Scenario: optional required 字段

- **WHEN** parameter 不含 required 字段
- **THEN** Parameter.Required = false（零值）

### Requirement: computation 文件路径字段

Attested Computation concept 的 `computation` 字段 MUST 被正确解析为路径（spec §10.2：可选，用于替代 body # Computation fence）。

#### Scenario: computation 路径

- **WHEN** frontmatter 含 `computation: references/computations/revenue.sql`
- **THEN** Concept.Computation = "references/computations/revenue.sql"

#### Scenario: 无 computation 字段

- **WHEN** frontmatter 无 computation
- **THEN** Concept.Computation 为空，使用 body # Computation 章节

### Requirement: executor 字段

Attested Computation concept 的 `executor` 字段 MUST 被正确解析为 `*ExecutorRef`，含 resource 和 receipt（spec §10.2）。

#### Scenario: executor 含 resource 和 receipt

- **WHEN** frontmatter 含 `executor: {resource: references/skills/run-on-bq.md, receipt: [job_id, executed_sql, result]}`
- **THEN** Concept.Executor.Resource = "references/skills/run-on-bq.md"
- **AND** Concept.Executor.Receipt = ["job_id", "executed_sql", "result"]

#### Scenario: executor 仅含 resource

- **WHEN** executor 不含 receipt
- **THEN** Concept.Executor.Receipt 为 nil

### Requirement: attester 字段

Attested Computation concept 的 `attester` 字段 MUST 被正确解析为 `*AttesterRef`，含 resource（spec §10.2）。

#### Scenario: attester 含 resource

- **WHEN** frontmatter 含 `attester: {resource: references/attesters/sql-equality.py}`
- **THEN** Concept.Attester.Resource = "references/attesters/sql-equality.py"

### Requirement: body # Computation 章节提取

`Concept.GetComputationBody()` 方法 MUST 从 markdown body 中提取 `# Computation` 章节下的第一个 fenced code block 内容（spec §4.2 约定 heading、§10.3 inline 方式）。

#### Scenario: body 含 # Computation + fenced code

- **WHEN** body 含 `# Computation` 章节，其下有一个 ```sql fenced code block
- **THEN** GetComputationBody() 返回 (codeContent, true)

#### Scenario: body 无 # Computation

- **WHEN** body 不含 `# Computation` 章节
- **THEN** GetComputationBody() 返回 ("", false)

#### Scenario: # Computation 下无 fenced code

- **WHEN** body 含 `# Computation` 章节但没有 fenced code block
- **THEN** GetComputationBody() 返回 ("", false)

#### Scenario: computation 文件路径优先

- **WHEN** Concept.Computation（文件路径）非空
- **THEN** GetComputationBody() 应优先从文件路径读取（如果文件存在），或返回路径提示

### Requirement: Appendix A revenue.md 官方示例

内置 Appendix A 的 revenue.md 示例 MUST 被正确解析（spec Appendix A）：
- type: Attested Computation
- runtime: bigquery
- parameters: 1 个（year, integer, required）
- executor: resource + receipt [job_id, executed_sql, result]
- attester: resource
- generated: by + at
- verified: human:ahormati
- stale_after: 2026-12-31
- sources: 2 个（rev-policy, exec-rev-dash），含 credibility signals
- usage_window: {from: 2026-06-01, to: 2026-06-30}
- body 含 # Computation + SQL code block

#### Scenario: revenue.md 完整解析

- **WHEN** 解析 revenue.md fixture
- **THEN** 所有上述字段正确解析
- **AND** TrustTier() = TrustHumanReviewed
- **AND** IsStale(2026-08-25) = false
- **AND** GetComputationBody() 返回 SQL 代码

### Requirement: Appendix A profit.md 官方示例

内置 Appendix A 的 profit.md 示例 MUST 被正确解析：
- type: Attested Computation
- runtime: dbt
- parameters: 2 个（year, segment）
- verified: process:finance-nightly
- stale_after: 2026-06-15

#### Scenario: profit.md 完整解析

- **WHEN** 解析 profit.md fixture
- **THEN** 所有上述字段正确解析
- **AND** TrustTier() = TrustMachineConfirmed
- **AND** IsStale(2026-08-25) = true（stale_after=2026-06-15 已过）
- **AND** GetComputationBody() 返回 dbt SQL 代码

### Requirement: Appendix A income-statement.md 官方示例

内置 Appendix A 的 income-statement.md 示例 MUST 被正确解析：
- type: Metric
- 链接到 revenue.md 和 profit.md（标准 markdown 链接）
- sources: 1 个（fpa-handbook）
- body 含 footnote 引用 [^fpa-handbook]

#### Scenario: income-statement.md 完整解析

- **WHEN** 解析 income-statement.md fixture
- **THEN** type = "Metric"，sources 正确解析
- **AND** body 含指向 ../computations/revenue.md 和 ../computations/profit.md 的链接
