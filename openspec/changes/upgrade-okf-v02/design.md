# Design: OKF v0.2 兼容支持升级

## 1. 类型系统设计

### 1.1 Concept 扩展

在 `pkg/okf/types.go` 中扩展 `Concept` 结构体，新增 v0.2 字段：

```go
type Concept struct {
    // v0.1 字段（保留）
    Type        string `yaml:"type" json:"type"`
    Title       string `yaml:"title,omitempty" json:"title,omitempty"` // v0.2: optional
    Description string `yaml:"description,omitempty" json:"description,omitempty"`
    Resource    string `yaml:"resource,omitempty" json:"resource,omitempty"`
    Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
    Timestamp   string `yaml:"timestamp,omitempty" json:"timestamp,omitempty"` // legacy v0.1

    // v0.2 新增字段
    Sources     []Source          `yaml:"sources,omitempty" json:"sources,omitempty"`
    UsageWindow *UsageWindow      `yaml:"usage_window,omitempty" json:"usage_window,omitempty"`
    Generated   *GeneratedInfo    `yaml:"generated,omitempty" json:"generated,omitempty"`
    Verified    []VerificationEvent `yaml:"verified,omitempty" json:"verified,omitempty"`
    Status      ConceptStatus     `yaml:"status,omitempty" json:"status,omitempty"`
    StaleAfter  string            `yaml:"stale_after,omitempty" json:"stale_after,omitempty"` // YYYY-MM-DD

    // Attested Computation 字段（仅 type == "Attested Computation" 时使用）
    Runtime     string            `yaml:"runtime,omitempty" json:"runtime,omitempty"`
    Parameters  []Parameter       `yaml:"parameters,omitempty" json:"parameters,omitempty"`
    Computation string            `yaml:"computation,omitempty" json:"computation,omitempty"` // path to computation file
    Executor    *ExecutorRef      `yaml:"executor,omitempty" json:"executor,omitempty"`
    Attester    *AttesterRef      `yaml:"attester,omitempty" json:"attester,omitempty"`

    // 内部字段
    Content     string                 `yaml:"-" json:"-"`
    FilePath    string                 `yaml:"-" json:"filePath,omitempty"`
    CustomFields map[string]interface{} `yaml:",inline" json:"-"`
}
```

### 1.2 新增类型

```go
type Source struct {
    ID          string `yaml:"id,omitempty" json:"id,omitempty"`
    Resource    string `yaml:"resource" json:"resource"` // REQUIRED within entry
    Title       string `yaml:"title,omitempty" json:"title,omitempty"`
    Author      string `yaml:"author,omitempty" json:"author,omitempty"` // actor convention
    UsageCount  int    `yaml:"usage_count,omitempty" json:"usage_count,omitempty"`
    LastModified string `yaml:"last_modified,omitempty" json:"last_modified,omitempty"` // YYYY-MM-DD
}

type UsageWindow struct {
    From string `yaml:"from" json:"from"` // YYYY-MM-DD
    To   string `yaml:"to" json:"to"`     // YYYY-MM-DD
}

type GeneratedInfo struct {
    By string `yaml:"by" json:"by"`         // actor convention, REQUIRED
    At string `yaml:"at,omitempty" json:"at,omitempty"` // ISO 8601
}

type VerificationEvent struct {
    By string `yaml:"by" json:"by"`         // actor convention
    At string `yaml:"at,omitempty" json:"at,omitempty"` // ISO 8601
}

type Parameter struct {
    Name     string `yaml:"name" json:"name"`
    Type     string `yaml:"type" json:"type"`
    Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

type ExecutorRef struct {
    Resource string   `yaml:"resource" json:"resource"`
    Receipt  []string `yaml:"receipt,omitempty" json:"receipt,omitempty"`
}

type AttesterRef struct {
    Resource string `yaml:"resource" json:"resource"`
}

type TrustTier int
const (
    TrustUnverified TrustTier = iota
    TrustMachineConfirmed
    TrustHumanReviewed
)

type ConceptStatus string
const (
    StatusDraft      ConceptStatus = "draft"
    StatusStable     ConceptStatus = "stable"
    StatusDeprecated ConceptStatus = "deprecated"
)
```

### 1.3 方法设计

```go
// TrustTier 从 verified 派生信任等级
func (c *Concept) TrustTier() TrustTier

// IsStale 判断是否过期（today >= stale_after）
func (c *Concept) IsStale(referenceTime time.Time) bool

// IsAttestedComputation 判断是否为 Attested Computation 类型
func (c *Concept) IsAttestedComputation() bool

// GetComputationBody 从 body 提取 # Computation 章节内容
func (c *Concept) GetComputationBody() (string, bool)

// NormalizeVerified 将 bare mapping 归一化为列表
func (c *Concept) NormalizeVerified()
```

## 2. 解析器设计

### 2.1 frontmatter 结构体扩展

在 `pkg/parser/parser.go` 中扩展 `frontmatter` 结构体，使用 `yaml.Node` 实现 `verified` 字段的灵活解析（支持 list 和 single mapping）：

```go
type frontmatter struct {
    // v0.1
    Type        string                 `yaml:"type"`
    Title       string                 `yaml:"title,omitempty"`
    Description string                 `yaml:"description,omitempty"`
    Resource    string                 `yaml:"resource,omitempty"`
    Tags        []string               `yaml:"tags,omitempty"`
    Timestamp   string                 `yaml:"timestamp,omitempty"` // legacy

    // v0.2
    Sources     []Source               `yaml:"sources,omitempty"`
    UsageWindow *UsageWindow           `yaml:"usage_window,omitempty"`
    Generated   *GeneratedInfo         `yaml:"generated,omitempty"`
    Verified    yaml.Node              `yaml:"verified,omitempty"` // 灵活解析
    Status      string                 `yaml:"status,omitempty"`
    StaleAfter  string                 `yaml:"stale_after,omitempty"`
    Runtime     string                 `yaml:"runtime,omitempty"`
    Parameters  []Parameter            `yaml:"parameters,omitempty"`
    Computation string                 `yaml:"computation,omitempty"`
    Executor    *ExecutorRef           `yaml:"executor,omitempty"`
    Attester    *AttesterRef           `yaml:"attester,omitempty"`

    CustomFields map[string]interface{} `yaml:",inline"`
}
```

### 2.2 verified 灵活解析逻辑

`verified` 字段在 v0.2 中可以是：
- 列表：`verified: [{by: ..., at: ...}, ...]`
- 单个 mapping：`verified: {by: ..., at: ...}`

使用 `yaml.Node` 后，在解析后判断 `Node.Kind`：
- `yaml.SequenceNode` → 解码为 `[]VerificationEvent`
- `yaml.MappingNode` → 解码为单个 `VerificationEvent`，包装为一元列表

### 2.3 保留文件名识别

新增 `IsReservedFilename(path string) bool` 函数，识别 `index.md` 和 `log.md`。解析时：
- `index.md`：不解析为 Concept（除了 bundle-root 可以有 `okf_version` frontmatter）
- `log.md`：不解析为 Concept

### 2.4 v0.1 回退逻辑

在 `ParseConceptBytes` 中，解析完成后执行回退：
1. 如果 `Generated == nil && Timestamp != ""` → 设置 `Generated = {By: "unknown", At: Timestamp}`
2. 如果 body 包含 `# Citations` 章节且 `Sources` 为空 → 从 Citations 列表提取 sources

### 2.5 title 不再必填

移除 `if concept.Title == "" { return error }`，改为：如果 title 为空，从文件名派生（`titleFromPath`），与 v0.2 spec §4.1 一致。

## 3. Lint 规则设计

### 3.1 规则变更映射

| 规则 | v0.1 行为 | v0.2 行为 |
|------|-----------|-----------|
| OKF001 | type required (Error) | 不变 |
| OKF002 | title required (Error) | title recommended (Warning)，缺失时建议从文件名派生 |
| OKF003 | description 太短 (Warning) | 不变 |
| OKF004 | type 必须全小写 (Warning) | 移除小写强制（v0.2 示例含 "BigQuery Table"），改为建议性提示 |
| OKF005 | timestamp ISO 8601 (Warning) | 改为检查 `generated.at` ISO 8601；`timestamp` 标记为 legacy |
| OKF006 | tags 小写无空格 (Warning) | 不变 |
| OKF007 | content 非空 (Warning) | 不变 |
| OKF009 | 长行 (Warning) | 不变 |
| OKF010 | 重复 tags (Warning) | 不变 |
| OKF011 | required tags (Warning) | 不变 |
| OKF012 (新) | - | `sources[].resource` 必填校验 (Warning) |
| OKF013 (新) | - | `generated.by` 必填校验（如果 generated 存在）(Warning) |
| OKF014 (新) | - | `verified[].by` actor 格式校验 (Warning) |
| OKF015 (新) | - | `status` 枚举校验 (Warning) |
| OKF016 (新) | - | `stale_after` 日期格式校验 (Warning) |
| OKF017 (新) | - | Attested Computation: `runtime` 必填 (Error) |
| OKF018 (新) | - | Attested Computation: `parameters[].name/type` 必填 (Warning) |
| OKF019 (新) | - | `executor.resource` / `attester.resource` 路径校验 (Warning) |
| OKF020 (新) | - | legacy `timestamp` 使用提示（建议迁移到 `generated.at`）(Info) |

### 3.2 Conformance 对齐

v0.2 §11 明确消费者 MUST NOT reject：
- 缺少可选 frontmatter 字段
- 未知 type 值
- 未知额外 frontmatter key
- 断链
- 缺少 index.md

因此 lint 中所有 v0.2 新字段规则均为 Warning/Info，不使用 Error（除了 Attested Computation 的 `runtime` 必填，因为这是该 type 的契约字段）。

## 4. 查询引擎设计

### 4.1 新增过滤维度

在 `pkg/query/query.go` 中扩展 `Query` 结构体：

```go
type Query struct {
    // 现有
    Type     string
    Tags     []string
    Resource string
    FullText string

    // v0.2 新增
    TrustTiers  []TrustTier    // 按信任等级过滤
    Statuses    []ConceptStatus // 按状态过滤
    ExcludeStale bool           // 排除过期 concept
    Sources     []string        // 按 source id/resource 过滤
}
```

### 4.2 Search 扩展

`KnowledgeBundle.Search` 的全文搜索范围扩展到：
- title, description, content（现有）
- `sources[].title`, `sources[].author`（新增）

## 5. Attested Computation 设计

### 5.1 类型识别

`Concept.IsAttestedComputation()` 通过 `Type == "Attested Computation"` 判断（大小写不敏感？spec 示例用的是 "Attested Computation"，保持精确匹配但提供归一化）。

### 5.2 Computation 提取

`GetComputationBody()` 从 markdown body 中提取 `# Computation` 章节下的第一个 fenced code block。如果 `Computation` 字段（文件路径）非空，则优先从文件读取。

### 5.3 验证

Attested Computation 的 lint 规则：
- `runtime` 必填（OKF017, Error）
- 如果 `parameters` 存在，每个 `name` 和 `type` 必填
- `executor.receipt` 应该是字符串列表

## 6. index.md / log.md 设计

### 6.1 IndexFile 类型

```go
type IndexFile struct {
    OKFVersion string       `yaml:"okf_version,omitempty"` // 仅 bundle-root
    Sections   []IndexSection `yaml:"-"`
    FilePath   string       `yaml:"-"`
}

type IndexSection struct {
    Heading string
    Entries []IndexEntry
}

type IndexEntry struct {
    Title       string
    URL         string // relative path
    Description string
}
```

### 6.2 自动生成

`GenerateIndex(bundle *KnowledgeBundle, dir string) (*IndexFile, error)` 从指定目录的 concept frontmatter 聚合 title/description，按子目录分组生成 sections。

### 6.3 LogFile 类型

```go
type LogFile struct {
    Entries  []LogEntry `yaml:"-"`
    FilePath string     `yaml:"-"`
}

type LogEntry struct {
    Date    string // YYYY-MM-DD
    Action  string // Update/Creation/Deprecation
    Message string
}
```

## 7. v0.1 兼容性设计

### 7.1 加载时自动迁移

`LoadBundle` 内部对每个 concept 执行 `migrateV01ToV02(c)`：
1. `timestamp` → `generated.at`（如果 generated 不存在）
2. body `# Citations` → `sources`（如果 sources 为空）
3. `title` 为空时从文件名派生

### 7.2 保存时策略

`SaveBundle` 默认输出 v0.2 字段。提供 `SaveOptions.LegacyTimestamp bool` 配置：
- `false`（默认）：不输出 `timestamp`，只输出 `generated.at`
- `true`：同时输出 `timestamp`（值为 `generated.at`），用于 v0.1 消费者回退

### 7.3 版本声明

bundle-root `index.md` 的 `okf_version` 字段：
- 加载时读取，记录到 `KnowledgeBundle.OKFVersion`
- 保存时如果 `OKFVersion == "0.2"`，写入到 bundle-root index.md frontmatter

## 8. 性能设计

- frontmatter 解析：使用 `yaml.Node` 仅对 `verified` 字段做灵活解析，其余字段直接映射，避免全量 `map[string]interface{}` 开销
- 新增字段均为 `omitempty`，零值不序列化
- benchmark 基线：1000 个 concept 的 v0.2 bundle 加载时间 ≤ v0.1 加载时间 × 1.2
- `TrustTier()` 和 `IsStale()` 为纯计算，无 IO

## 9. 稳定性设计

- 所有新增字段均为 optional，缺失不影响解析
- `verified` 灵活解析使用 `yaml.Node`，遇到无法识别的格式时降级为空列表而非报错
- v0.1 回退逻辑有明确的触发条件（generated 为空才回退到 timestamp），不会覆盖 v0.2 数据
- CustomFields `yaml:",inline"` 保留未知字段，round-trip 无损
- parser 对保留文件名（index.md/log.md）跳过 Concept 解析，避免误报

## 10. 协议兼容性设计

- YAML frontmatter 格式不变，仅新增字段
- v0.1 bundle 可被 v0.2 消费者加载（自动迁移）
- v0.2 bundle 可被 v0.1 消费者加载（新字段被忽略，`timestamp` 可选回退）
- `okf_version` 声明在 bundle-root index.md，不影响 concept 文件格式
- Attested Computation 作为新 type，v0.1 消费者将其视为普通 concept（容忍未知 type）

## 11. 官方示例验证

内置 Appendix A income statement 示例作为 fixture：
- `metrics/income-statement.md`（Metric, 链接两个 computation）
- `computations/revenue.md`（Attested Computation, bigquery, human-verified, fresh）
- `computations/profit.md`（Attested Computation, dbt, process-verified, stale）
- `references/skills/run-on-bq.md`, `run-dbt.md`
- `references/attesters/sql-equality.py`, `dbt-binding.py`

测试覆盖：
- 完整加载 → 所有字段正确解析
- round-trip 保存 → 字段无损
- trust tier 派生 → revenue=human-reviewed, profit=machine-confirmed
- staleness 判断 → profit stale（stale_after=2026-06-15）, revenue fresh
- Attested Computation 字段 → runtime/parameters/executor/attester 正确
- sources 解析 → 含 credibility signals 和 usage_window
