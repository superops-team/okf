# Tasks: OKF v0.2 兼容支持升级

> 所有任务遵循 TDD：先写测试（红），再实现（绿），最后重构（蓝）。
> 每个任务完成后必须跑 `go test ./...` 和 `go vet ./...`。

## 1. P0 核心类型系统（TDD）

- [ ] 1.1 定义 `Source`、`UsageWindow`、`GeneratedInfo`、`VerificationEvent`、`Parameter`、`ExecutorRef`、`AttesterRef`、`TrustTier`、`ConceptStatus` 类型，添加单元测试覆盖零值和序列化。
- [ ] 1.2 扩展 `Concept` 结构体，新增所有 v0.2 字段（Sources/UsageWindow/Generated/Verified/Status/StaleAfter/Runtime/Parameters/Computation/Executor/Attester），`Title` 标记为 optional，`Timestamp` 标记为 legacy。添加 YAML round-trip 测试。
- [ ] 1.3 实现 `TrustTier()` 方法：无 verified→unverified，仅非 human→machine-confirmed，含 human→human-reviewed。添加表驱动测试。
- [ ] 1.4 实现 `IsStale(referenceTime)` 方法：`today >= stale_after` 即 stale，空 stale_after→false，无效日期→false。添加测试。
- [ ] 1.5 实现 `IsAttestedComputation()` 和 `GetComputationBody()`（从 body 提取 # Computation 章节的第一个 fenced code block）。添加测试。
- [ ] 1.6 实现 `NormalizeVerified()`：将 bare mapping 归一化为一元列表。添加测试（list 形式不变、mapping 形式转一元列表、空值不变）。
- [ ] 1.7 扩展 `KnowledgeBundle` 新增 `OKFVersion` 字段。添加测试。

## 2. P0 解析器升级（TDD）

- [ ] 2.1 扩展 parser `frontmatter` 结构体，新增所有 v0.2 字段映射。添加 v0.2 concept 解析测试（含 sources/generated/verified/status/stale_after）。
- [ ] 2.2 实现 `verified` 字段灵活解析：使用 `yaml.Node`，支持 list 和 single mapping 两种形式。添加测试（列表形式、单个 mapping 形式、空值、无效格式降级）。
- [ ] 2.3 移除 `title` 必填校验：title 为空时从文件名派生（`titleFromPath`），不再返回 error。添加测试（无 title 的 concept 可正常解析）。
- [ ] 2.4 实现保留文件名识别：`IsReservedFilename(path)` 识别 `index.md`/`log.md`；解析时跳过保留文件（不返回 Concept）。添加测试。
- [ ] 2.5 实现 bundle-root `index.md` 的 `okf_version` 解析：加载时读取并设置到 `KnowledgeBundle.OKFVersion`。添加测试。
- [ ] 2.6 实现 v0.1 回退：`timestamp`→`generated.at`（generated 为空时），body `# Citations`→`sources`（sources 为空时）。添加 v0.1 fixture 测试。
- [ ] 2.7 扩展 `SerializeConcept` 支持所有 v0.2 字段序列化，`omitempty` 零值不输出。添加 round-trip 测试（v0.2 concept 保存再加载字段无损）。

## 3. P0 Lint 规则对齐 v0.2 conformance（TDD）

- [ ] 3.1 OKF002 降级：从 Error 改为 Warning，消息改为 "title is recommended"。添加测试（无 title 不再是 error）。
- [ ] 3.2 OKF004 修改：移除 type 全小写强制（v0.2 示例含 "BigQuery Table"），改为仅在 type 为空时提示。添加测试（"BigQuery Table" 不再报警）。
- [ ] 3.3 OKF005 修改：从检查 `timestamp` 改为检查 `generated.at` ISO 8601；`timestamp` 存在但 `generated` 不存在时触发 OKF020（legacy 提示）。添加测试。
- [ ] 3.4 新增 OKF012：`sources[].resource` 必填校验（如果 sources 存在）。添加测试。
- [ ] 3.5 新增 OKF013：`generated.by` 必填校验（如果 generated 存在）。添加测试。
- [ ] 3.6 新增 OKF014：`verified[].by` actor 格式校验（匹配 `human:`/`process:`/`<producer>/<version>` 模式）。添加测试。
- [ ] 3.7 新增 OKF015：`status` 枚举校验（draft/stable/deprecated，空值默认 stable）。添加测试。
- [ ] 3.8 新增 OKF016：`stale_after` 日期格式校验（YYYY-MM-DD）。添加测试。
- [ ] 3.9 新增 OKF017：Attested Computation 的 `runtime` 必填（Error，因为是该 type 的契约字段）。添加测试。
- [ ] 3.10 新增 OKF018：Attested Computation 的 `parameters[].name/type` 必填校验。添加测试。
- [ ] 3.11 新增 OKF019：`executor.resource`/`attester.resource` 非空校验（如果 executor/attester 存在）。添加测试。
- [ ] 3.12 新增 OKF020：legacy `timestamp` 使用提示（Info，建议迁移到 `generated.at`）。添加测试。
- [ ] 3.13 更新 lint `Concept` 接口结构体，新增所有 v0.2 字段。确保 `LintBundle` 兼容新字段。

## 4. P1 查询引擎升级（TDD）

- [ ] 4.1 扩展 `Query` 结构体，新增 `TrustTiers`、`Statuses`、`ExcludeStale`、`Sources` 过滤维度。添加测试。
- [ ] 4.2 实现 trust tier 过滤：`FilterByTrustTier(tier)` 和 `FilterByTrustTiers(tiers)`。添加测试。
- [ ] 4.3 实现 status 过滤：`FilterByStatus(status)` 和 `FilterByStatuses(statuses)`。添加测试。
- [ ] 4.4 实现 stale 过滤：`FilterFresh()`（排除 stale concept）。添加测试。
- [ ] 4.5 扩展 `Search` 全文搜索范围到 `sources[].title` 和 `sources[].author`。添加测试。
- [ ] 4.6 扩展 `Stats()` 新增 trust tier 统计和 status 统计。添加测试。

## 5. P1 Attested Computation 支持（TDD）

- [ ] 5.1 完整 Attested Computation concept 解析测试：runtime/parameters/computation/executor/attester 全部字段。
- [ ] 5.2 `GetComputationBody()` 测试：body 有 # Computation + fenced code block → 返回代码；无 # Computation → 返回 false；computation 文件路径优先。
- [ ] 5.3 Attested Computation lint 规则集成测试：runtime 缺失→Error，parameters 字段校验。
- [ ] 5.4 Appendix A revenue.md fixture 测试：bigquery runtime，1 个 parameter，executor/attester 引用，human-verified，fresh。
- [ ] 5.5 Appendix A profit.md fixture 测试：dbt runtime，2 个 parameters，process-verified，stale（stale_after=2026-06-15）。

## 6. P1 index.md / log.md 支持（TDD）

- [ ] 6.1 定义 `IndexFile`、`IndexSection`、`IndexEntry`、`LogFile`、`LogEntry` 类型。添加测试。
- [ ] 6.2 实现 `ParseIndexFile(path)`：解析 index.md，bundle-root 时读取 `okf_version` frontmatter。添加测试。
- [ ] 6.3 实现 `GenerateIndex(bundle, dir)`：从 concept frontmatter 聚合生成 index。添加测试（空目录、单 concept、多子目录分组）。
- [ ] 6.4 实现 `ParseLogFile(path)`：解析 log.md，按日期分组。添加测试。
- [ ] 6.5 `LoadBundle` 集成：跳过 index.md/log.md，bundle-root index 的 okf_version 写入 bundle。添加集成测试。

## 7. P1 v0.1 向后兼容（TDD）

- [ ] 7.1 v0.1 fixture 加载测试：仅含 type/title/timestamp 的 concept → 自动迁移 generated.at，timestamp 保留。
- [ ] 7.2 v0.1 body `# Citations` 迁移测试：Citations 列表 → sources 列表（resource=URL，id 自动生成）。
- [ ] 7.3 v0.2 bundle 被 v0.1 消费者加载测试：新字段被忽略，concept 仍可解析（模拟 v0.1 parser 只识别旧字段）。
- [ ] 7.4 `SaveOptions.LegacyTimestamp` 测试：true→同时输出 timestamp 和 generated.at；false→仅输出 generated.at。
- [ ] 7.5 混合 bundle 测试：部分 v0.1 concept + 部分 v0.2 concept → 全部正确加载和迁移。

## 8. P2 API 层与 CLI 适配（TDD）

- [ ] 8.1 `api.go` `LoadBundle`/`SaveBundle` 集成 v0.2 字段和 okf_version。添加集成测试。
- [ ] 8.2 `git/generator.go` 生成 v0.2 兼容 concept：设置 `generated` 字段（by=generator/version, at=now），`status=stable`。添加测试。
- [ ] 8.3 CLI `show` 命令输出新增 trust tier、status、stale 状态、sources 数量。添加测试。
- [ ] 8.4 CLI `search` 命令支持 `--trust`、`--status`、`--exclude-stale` 过滤参数。添加测试。
- [ ] 8.5 CLI `lint` 命令输出新规则（OKF012-OKF020）。添加测试。

## 9. P2 官方示例与端到端验证（TDD）

- [ ] 9.1 创建 Appendix A 完整 fixture 目录（metrics/income-statement.md, computations/revenue.md, computations/profit.md, references/...）。
- [ ] 9.2 端到端测试：加载完整 income statement bundle → 所有 concept 正确解析 → trust tier/stale 状态正确 → round-trip 保存无损。
- [ ] 9.3 income-statement.md 链接解析测试：两个 Attested Computation 链接可被识别和追踪。
- [ ] 9.4 revenue.md 完整字段验证：sources 含 credibility signals（author/usage_count/last_modified）和 usage_window。
- [ ] 9.5 profit.md stale 验证：stale_after=2026-06-15，referenceTime=2026-08-25 → IsStale=true。

## 10. P2 性能、稳定性、协议兼容性验证

- [ ] 10.1 Benchmark：1000 个 v0.2 concept 加载时间 ≤ v0.1 加载时间 × 1.2。记录基准数据。
- [ ] 10.2 Benchmark：1000 个 concept 保存时间 ≤ v0.1 × 1.2。
- [ ] 10.3 稳定性测试：随机生成 100 个 v0.2 concept（含各种字段组合）→ round-trip 无损。
- [ ] 10.4 协议兼容性测试：v0.1 bundle → v0.2 加载 → v0.2 保存 → v0.1 加载（新字段被忽略），全链路无错误。
- [ ] 10.5 模糊测试：无效 YAML、缺失字段、未知 type、断链 → parser 不 panic，返回合理错误或降级。
- [ ] 10.6 跑 `go test ./...`、`go test -race ./...`、`go vet ./...` 全部通过。
- [ ] 10.7 跑 `openspec validate upgrade-okf-v02 --strict` 并修复所有规范错误。

## 11. P2 文档与示例

- [ ] 11.1 更新 README.md：新增 v0.2 字段说明、Attested Computation 示例、trust tier 说明。
- [ ] 11.2 更新 README.zh-CN.md：同步中文版。
- [ ] 11.3 新增 `examples/v0.2/` 目录：包含 income statement 完整示例。
- [ ] 11.4 更新 `STRESS_TEST_REPORT.md`：新增 v0.2 性能基准数据。
