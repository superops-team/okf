## Why

OKF 官方规范已发布 v0.2（GoogleCloudPlatform/knowledge-catalog/okf/SPEC.md），在 v0.1 基础上引入了 provenance（`sources`）、trust（`generated`/`verified`）、lifecycle（`status`/`stale_after`）、Attested Computation 等一等公民字段，并明确了 conformance 规则：只有 `type` 是必填字段，`title` 降级为 recommended，消费者必须容忍未知 type/字段/断链。

当前 okf 实现仍停留在 v0.1 模型：`Concept` 只有 `type/title/description/resource/tags/timestamp`，`title` 被当作必填，`timestamp` 是唯一时间字段，parser 和 lint 都强制 v0.1 规则。这导致：
- 无法解析/生成 v0.2 规范的 concept（`sources`/`generated`/`verified`/`status`/`stale_after`/Attested Computation 字段全部丢失）
- `title` 必填与 v0.2 conformance 冲突（v0.2 只要求 `type`）
- `timestamp` 已被 `generated.at` 取代，缺少 v0.1→v0.2 回退逻辑
- 无法消费官方 Appendix A 的 income statement 示例（含 Attested Computation、trust tiers、stale_after）

## What Changes

- **核心类型升级**：`Concept` 新增 `Sources`/`Generated`/`Verified`/`Status`/`StaleAfter`/`Runtime`/`Parameters`/`Computation`/`Executor`/`Attester` 字段；新增 `Source`/`GeneratedInfo`/`VerificationEvent`/`Parameter`/`Executor`/`Attester` 类型；`Title` 从必填降级为可选；`Timestamp` 标记为 legacy v0.1 回退字段。
- **Trust tier 派生**：从 `verified` 派生 `unverified`/`machine-confirmed`/`human-reviewed` 三级；bare `verified` mapping 必须当作一元列表。
- **Staleness 判断**：`stale_after` 绝对日期比较，`today >= stale_after` 即为 stale。
- **解析器升级**：frontmatter 支持所有 v0.2 字段；`verified` 支持 list 和 single mapping 两种形式；识别保留文件名 `index.md`/`log.md`；bundle-root `index.md` 支持 `okf_version`；v0.1 回退：`timestamp`→`generated.at`，body `# Citations`→`sources`。
- **Lint 规则对齐 v0.2 conformance**：OKF002（title required）从 Error 降级为 Warning；OKF005 从 `timestamp` 改为 `generated.at`；新增 `sources`/`generated`/`verified`/`status`/`stale_after`/Attested Computation 字段校验规则；OKF004 不再强制 type 全小写（v0.2 示例含 "BigQuery Table"）。
- **查询引擎升级**：支持按 trust tier、status、stale 状态过滤；`Search` 扩展到 `sources[].title`/`description`。
- **Attested Computation 支持**：`type: Attested Computation` 的完整 frontmatter 模型；body `# Computation` 约定 heading 提取；`computation` 文件路径回退。
- **index.md / log.md 支持**：保留文件名识别；bundle-root index 的 `okf_version` 解析；index 自动生成（从 concept frontmatter 聚合 title/description）。
- **v0.1 向后兼容**：加载 v0.1 bundle 时自动迁移 `timestamp`→`generated.at`、`# Citations`→`sources`；保存时默认输出 v0.2 字段但保留 legacy `timestamp` 作为回退（可配置）。
- **官方示例验证**：内置 Appendix A income statement 示例的 fixture 测试，确保 round-trip 无损。

## Capabilities

### New Capabilities
- `okf-v02-core-types`: v0.2 核心类型系统（provenance/trust/lifecycle/Attested Computation）
- `okf-v02-attested-computation`: Attested Computation concept 完整支持
- `okf-v02-index-log`: `index.md`/`log.md` 保留文件名与 `okf_version` 支持

### Modified Capabilities
- `okf-v02-parser`: 解析器从 v0.1 升级到 v0.2，含 v0.1 回退
- `okf-v02-lint`: lint 规则对齐 v0.2 conformance
- `okf-v02-query`: 查询引擎支持 trust tier/status/stale 过滤
- `okf-v02-compatibility`: v0.1→v0.2 自动迁移与双向兼容

## Impact
- 主要影响 Go 包：`pkg/okf`（types.go 核心类型扩展、api.go 加载保存、helpers.go 新增 trust/stale 方法）、`pkg/parser`（frontmatter 结构体扩展、保留文件名识别、v0.1 回退）、`pkg/lint`（规则重写对齐 conformance）、`pkg/query`（过滤维度扩展）、`pkg/git`（generator 输出 v0.2 字段）、`cmd/okf`（命令输出适配）。
- 新增类型：`Source`、`GeneratedInfo`、`VerificationEvent`、`Parameter`、`ExecutorRef`、`AttesterRef`、`TrustTier`、`ConceptStatus`、`IndexFile`、`LogFile`、`LogEntry`。
- 破坏性变更：`Concept.Title` 不再是必填（parser 不再因缺少 title 报错）；`Concept.Timestamp` 标记为 legacy。这两项是 v0.2 spec 的 deliberate breaking changes，通过 v0.1 回退逻辑保证旧 bundle 可加载。
- 不新增外部依赖；继续使用 `gopkg.in/yaml.v3`。
- 性能：新增字段解析开销 O(frontmatter size)，benchmark 确保 1k concept 加载不超过 v0.1 的 1.2 倍。
