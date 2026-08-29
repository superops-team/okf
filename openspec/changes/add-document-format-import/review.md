# Review: Document Format Import Change

## 评审范围与依据

- 评审对象：`openspec/changes/add-document-format-import/` 全部 5 个文件
- 核查方式：逐文件对照 okf 实际源码（`pkg/okf/import.go`、`pkg/okf/smart_import.go`、`pkg/okf/merge.go`、`pkg/okf/watch_config.go`、`cmd/okf/cmd_add.go`、`pkg/mcp/tools.go`）与 downmark v0.10.0 源码（`gocache2/pkg/mod/github.com/giraffesyo/downmark@v0.10.0/`）交叉核对
- 结论分级：🔴 必须修正（事实错误/设计缺陷）、🟡 建议优化（歧义/精简）、🟢 通过（符合）

---

## 维度 1：上下文逻辑连贯统一 —— 🔴 有 4 处断层

| # | 级别 | 问题 | 证据 |
|---|------|------|------|
| 1-1 | 🔴 | `tasks.md` Task 2.1 声称改 `CollectFiles`+`Import/ImportDirectory`，但 `okf add` 实际主链路是 `SmartImportSource`（只认 `.md`），`Import*` 系列是另一条路径。两条路径改造点未分清，实现会做错 | `cmd_add.go:115` 调 `okf.SmartImportSource`；`import.go:67` 的 `SmartImportSource` 内 `CollectFiles` 仅收 `.md` |
| 1-2 | 🔴 | `design.md` 的 `pkg/convert.Options{MaxInputBytes, MaxOutputBytes}` 与 downmark 实际机制不符：输出上限走 `downmark.ResultLimit(ctx)` context，输入上限需自定义 | `downmark/convert/pdf/pdf.go:144` `limit, _ := downmark.ResultLimit(ctx)` |
| 1-3 | 🔴 | `Result.Warnings` 写 `[]string`，downmark 实际为 `[]Warning`（含 Converter/Code/Location/Err 结构化字段） | `downmark/warnings.go:41` |
| 1-4 | 🔴 | 变更检测身份语义缺失：smart_import 的 `ContentHash` 基于 source 文件计算，文档场景 source 是"临时转换 .md"还是"原文档"未定义，直接决定升级后是否全量重导入 | `smart_import.go:152` `ComputeFileHash(source)` |

**整改**：统一主链路为「cmd_add 前置转换 → 既有 SmartImportSource」，不侵入核心；`pkg/convert` 明确"输入 os.Stat 检查 + 输出 ResultLimit context"双机制；`Warnings` 收敛为字符串摘要；新增身份稳定性 Requirement（见维度 7/9）。

---

## 维度 2：内容空洞、无落地依据 —— 🟡 2 处

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 2-1 | 🟡 | `Requirement: watch 守护进程支持文档格式（可选）` 引入新配置开关 `watch_documents`，但 `.watch.yaml` 已有 `Patterns`（默认 `**/*.md`），用户加 `**/*.pdf` 即可实现，新开关属空转 | 删除新开关；watch 支持降级为「文档说明：扩展 Patterns 即可监听文档」，不作为 MUST 场景 |
| 2-2 | 🟡 | `Requirement: MCP 提供文档导入工具` 未定义"写入哪个知识库"（当前加载 bundle？额外路径参数？），落地无法执行 | 定案：写入当前已加载 bundle 路径；未加载返回明确错误 |

---

## 维度 3：模糊/多义 —— 🟡 5 处

| # | 级别 | 问题 | 整改（定案） |
|---|------|------|-------------|
| 3-1 | 🟡 | 转换产物 `type` 默认值悬而未决（design Open Questions） | 定案 `source`，与既有 source 类型一致，不改 lint/查询语义 |
| 3-2 | 🟡 | 转换产物命名规则未定：`sample.pdf` → `sample.md`? `sample.pdf.md`? | 定案 `<原名>.md`（如 `sample.pdf.md`），保留扩展名防冲突、可溯源 |
| 3-3 | 🟡 | `description` 硬编码文案是否合适 | 改为模板 `Converted from <filename> (via <format>)`，格式可读 |
| 3-4 | 🟡 | `IsSupportedDocument(.md)` 返回 false 语义易误读为"md 不受支持" | doc 注释明确：仅标识"需经转换层的非 md 文档"；`.md` 走既有管道 |
| 3-5 | 🟡 | 格式识别优先级（归档 vs 文档 vs md）未声明 | 补场景：`IsArchive` → 归档路径；文档扩展名 → 转换；`.md` → 直通 |

---

## 维度 4：大模型语义理解偏差 —— 🟡 2 处

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 4-1 | 🟡 | `ErrUnsupportedFormat` 触发条件"无法识别的扩展名"与 `IsArchive` 的关系未说明，易误解为 zip 也报此错 | spec 补"识别优先级 归档>文档>md>其他"，`.zip` 归归档不归文档 |
| 4-2 | 🟡 | `ErrEmptyPDF` 命名暗示新增错误类型，实际 downmark 已提供 `errNoText`（含扫描/字体提示文案） | 直接透传 downmark 错误（`errors.Is` 匹配），不发明新类型；spec 措辞改为"映射/透传" |

---

## 维度 5：SDD/TDD 落地适配 —— 🟡 3 处

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 5-1 | 🟡 | Task 1.3 需要 testdata fixtures（真实 pdf/docx/xlsx/pptx），来源未定义，TDD red 阶段无法启动 | 定案：用 Python（环境已有 python-docx/openpyxl/python-pptx/reportlab）生成固定样本存 `pkg/convert/testdata/`，git 入库 |
| 5-2 | 🟡 | golden 测试对 downmark 版本敏感，升级即失效 | go.mod 精确锁 v0.10.0；golden 文件头注释锁定版本 |
| 5-3 | 🟡 | MCP 集成测试依赖 bundle 前置加载，测试顺序未定义 | 测试序列明确：先 `okf_load_bundle` 再 `okf_import_document` |

---

## 维度 6：最小化实现原则 —— 🔴 2 处（核心精简）

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 6-1 | 🔴 | 原方案同时改 `CollectFiles`/`Import*`/`SmartImportSource`/`ImportResult`/watch，破坏面大且多数非必需 | **最小路径**：仅新增 `pkg/convert` + `cmd_add` 前置转换 + MCP 工具 + go.mod。`CollectFiles`/`SmartImportSource`/`ImportResult` **全部不改**（转换产物是 .md，喂给既有管道即通） |
| 6-2 | 🔴 | `ImportResult.ConvertedFiles` 新字段：转换统计只需在 cmd_add 本地累加，不必侵入 ImportResult 结构 | 删除该字段改动，cmd_add 本地统计并打印 |

**精简后改动面**：4 个文件（go.mod、pkg/convert/convert.go、cmd/okf/cmd_add.go、pkg/mcp/tools.go）+ 3 个测试文件，比原方案减少约一半。

---

## 维度 7：向下兼容隐患 —— 🟡 2 处

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 7-1 | 🟡 | Go 1.22→1.26 升级：存量用户 `go install`/源码构建需 Go 1.26，旧环境可能不可用 | README/Release Notes 标注最低 Go 版本；保留 `go.mod` 的 `go 1.26.0` + `toolchain go1.26.7` |
| 7-2 | 🔴 | **身份/重导入语义**：文档 hash 若基于转换产物，downmark 升级→产物变化→全量重导入（数据风暴） | 方案：go.mod 锁死 downmark 版本 + spec 新增 Requirement「文档导入身份语义」：`SourcePath` 用原文档路径、变更检测基于原文档 mtime/size 前置判断（复用 `detectChangeForIdentity` 思路），downmark 升级属受控行为变更，触发一次性重导入可接受并文档化 |

---

## 维度 8：存量业务破坏性 —— 🟡 2 处

| # | 级别 | 影响 | 说明 |
|---|------|------|------|
| 8-1 | 🟡 | `okf add foo.pdf` 行为变更：原「静默跳过（No markdown files found）」→ 新「自动转换导入」 | 期望变更，但需在 README/CLI 帮助中声明，避免用户困惑 |
| 8-2 | 🟡 | 二进制体积 +~10MB（引入 downmark 全格式） | 记录；如体积敏感可改为按需 import 子包（downmark 支持） |

---

## 维度 9：功能失效/异常风险 —— 🔴 2 处

| # | 级别 | 风险 | 缓解 |
|---|------|------|------|
| 9-1 | 🔴 | 预转换临时目录生命周期：转换失败/进程中断→临时 .md 泄漏或残留 | cmd_add 用 `os.MkdirTemp` + `defer os.RemoveAll`；转换失败即清理 |
| 9-2 | 🔴 | 大 PDF 内存/CPU 峰值，watch 触发时可能阻塞 | 前置 `MaxInputBytes`（os.Stat）检查 + `ResultLimit` context 带超时；watch 复用既有 debouncer 防重入 |

---

## 维度 10：整体可行性 —— 🟢 高

核心链路已实测跑通（downmark 编译 + 7 格式转换 + okf parser 解析）。主要阻碍为维度 1/6/7/9 的整改项，均为设计修正而非技术不可行。预计总工期 5-6 人日（见排期）。

---

## 维度 11：分层任务/测试/排期 —— 见 tasks.md（已重写）

---

## 维度 12：可扩展性 —— 🟢 结构良好，1 处需前置决策

- 格式扩展：`pkg/convert` 集中 `DocumentType` 映射表，加格式只增映射 + downmark 包，扩展成本 O(1)
- **前置决策**：文档「身份/重导入语义」若现在不定，后续加格式时每个都要处理，放大成本（维度 7-2）。本方案已在 spec 定案
- OCR：downmark `Options.PDF` 已留 OCR 字段，pkg/convert 预留透传即可，不阻塞

---

## 维度 13：过度设计 —— 🟡 收敛 3 处

- `watch_documents` 新开关 → 删除（用现有 Patterns）
- 自定义错误体系（ErrEmptyPDF/ErrInputTooLarge/ErrOutputTooLarge）→ 精简为透传 downmark 错误 + `ErrUnsupportedFormat` 单一包装
- `ImportResult.ConvertedFiles` → 删除（cmd_add 本地统计）

---

## 维度 14：小而高效 —— 整改后达成

核心改动收敛为：`pkg/convert`（~200 行）+ `cmd_add` 前置转换（~40 行）+ MCP 工具（~50 行）+ go.mod。转换作为「前置处理器」，知识库核心模型零改动。

---

## 维度 15：持续优化写法 —— 🟢

`pkg/convert` 独立包、错误透传、不侵入核心导入逻辑，保持 okf「parser/lint/query 各司其职」的分层风格；后续可平滑演进（OCR/更多格式）。

---

## 维度 16：架构风格统一 —— 🟢 但 1 处需坚持

- 转换层必须放在「导入入口之前」（前置管道），而非塞进 `import.go` 内部，否则造成架构割裂 —— 整改后已保证
- `pkg/convert` 返回标准 Markdown，与 okf 既有「一切皆 .md 概念」的心智模型一致，无二义

---

## 维度 17：全需求接线与覆盖率测试 —— 🔴 当前 3 处缺口

> 要求：**所有规划的需求必须接线并针对性设计覆盖率测试用例**；代码流程被覆盖和执行；**没有接线和测试的开发不算完成**。

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 17-1 | 🔴 | 原 tasks.md 每个 Task 的 `Tests` 只是"列测试名"，**未与 spec 的 28 个 Scenario 建立一对一追踪**，无法证明"每个需求都有对应测试被接线" | tasks.md 新增「spec Scenario → 测试用例 → 测试文件 → 接线函数」**覆盖矩阵**（28 场景全覆盖），并给每个 Task 增加 `接线点` 字段（实现函数 ↔ 测试用例） |
| 17-2 | 🔴 | 缺「未接线需求不计入完成」的**验收门槛**定义，存在只写 spec 不接线的风险 | tasks.md「完成门槛」明确：Requirement 未接线（无实现函数）或无测试用例（未执行）→ 该需求不算完成；验收以覆盖率矩阵逐项打勾为准 |
| 17-3 | 🟡 | 命令级/文档级断言（如"依赖锁定""Release Notes 声明"）无对应可执行检查 | 补 `TestDependencyPinned`（go.mod 断言）、`TestReleaseNotesDeclaresReimport`（文档断言）等，把非函数类需求也纳入可执行覆盖 |

**整改后**：28 个 spec Scenario 全部映射到测试用例（单元/集成/命令级/文档断言），覆盖矩阵作为验收唯一依据。

---

## 维度 18：排期仅表依赖关系，不得只开发 P0 —— 🔴 需显式约束

> 要求：p0/p1/p2 **仅仅表示依赖关系与先后顺序**，不能只开发 p0。

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 18-1 | 🔴 | 原 tasks.md 以 Phase/排期呈现，未显式声明"阶段仅表依赖顺序、交付必须全量完成"，易被误解为可分阶段交付 | tasks.md 顶部加**「完成门槛」**：P0-P4 全部落地、全部测试绿、一致性对照通过后，整体变更才算完成；任何阶段都不构成可独立交付的边界 |
| 18-2 | 🟡 | 未定义"部分完成"的判定，无法阻止"只做完 P0 就宣称完成" | 完成门槛 + 覆盖率矩阵双把关：验收清单（10 项）逐项打勾，缺任何一项即未完成 |

**整改后**：排期表仅表达依赖先后（P0 是 P1 的前置等），完成定义与排期解耦。

---

## 维度 19：落地后 spec-代码一致性对照 —— 🔴 缺专项审计任务

> 要求：需求完成落地后，需设计 **spec 与落地代码一致性对照分析任务**，评估实现与 spec 是否对齐。

| # | 级别 | 问题 | 整改 |
|---|------|------|------|
| 19-1 | 🔴 | 原 tasks.md 无「spec-实现一致性对照审计」任务，落地后无法系统证明实现与 spec 对齐 | Phase 4 新增 **Task 4.4「spec-实现一致性对照审计」**：逐条 spec Scenario → 实现位置（函数/命令）→ 测试用例 → 实测结果，输出 `conformance.md` 对照报告，作为合入 PR 的前置门槛 |
| 19-2 | 🟡 | 一致性对照缺少自动化辅助（依赖 openspec CLI） | 对照审计以覆盖矩阵为骨架自动生成待核对清单；openspec CLI 可用时叠加 `openspec validate`；两者结合人工复核 |

**整改后**：新增 `conformance.md` 交付物（一致性对照报告），对齐度（fully/aligned/partial/gap）逐项记录，未对齐项必须给出原因与后续任务。

---

## 维度 16 之后补充结论

本轮新增 3 个维度（17/18/19）核查结果：**3 处 🔴 缺口（17-1, 17-2, 18-1, 19-1）+ 3 处 🟡 优化（17-3, 18-2, 19-2）**，已全部落实到 tasks.md 的「完成门槛」「接线点」「覆盖矩阵」「一致性审计」四项机制中。

---

## 问题清单汇总（31 项 → 整改后）

| 分级 | 数量 | 处置 |
|------|------|------|
| 🔴 必须修正 | 13 项（1-1,1-2,1-3,1-4,6-1,6-2,7-2,9-1,9-2 + 17-1,17-2,18-1,19-1） | 已在 spec 文件修订中全部落实；其中 1-1 与 6-1 为同一主链路修正、7-2 与 9 系列合并，**独立修正点 10 项** |
| 🟡 建议优化 | 18 项（2-1,2-2,3-1~3-5,4-1,4-2,5-1~5-3,7-1,8-1,8-2 + 17-3,18-2,19-2） | 已在 spec 文件修订中逐一定案/收敛；另有维度 13 的三处设计收敛（watch 开关/错误体系/ConvertedFiles）并入 |

## 整改后 spec 文件更新清单

| 文件 | 变更 |
|------|------|
| `proposal.md` | 收敛 Requirements（watch 降级为说明）、风险更新（Go 升级/身份语义/二进制体积） |
| `design.md` | 架构改为「前置转换层」；修正 Warnings/ResultLimit 机制；定案命名/type/description；删除过度设计；关闭 Open Questions |
| `specs/.../spec.md` | 修正 Warnings 类型、透传错误；新增「格式识别优先级」「转换产物命名」「文档身份/重导入语义」「MCP 写入目标」场景 |
| `tasks.md` | 基于最小路径重写：精简任务、补 fixture 策略与 golden 策略、附测试规划与排期 |
