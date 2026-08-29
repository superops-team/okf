# Implementation Tasks

任务按 **SDD（Spec-Driven Development）→ TDD（Test-Driven Development）** 组织：每阶段先对齐 spec 场景，再以测试先行（red → green → refactor）实现，最后全量回归。

## 完成门槛（所有需求必须全量落地）

> **P0-P4 仅表示依赖关系与先后顺序，任何阶段都不构成可独立交付的边界。只有全部阶段落地、全部测试绿、一致性对照通过后，本变更才算完成。**

1. **全量落地**：P0-P4 共 15 个 Task 全部实施，不得只开发 P0/P1
2. **全需求接线**：每个 Requirement 必须有实现函数/入口（接线点）与对应测试用例；**未接线的需求不计入完成**
3. **全覆盖测试**：spec 全部 28 个 Scenario 均被覆盖矩阵中对应测试执行（单元/集成/命令级/文档断言）；**没有接线和测试的开发不算完成**
4. **一致性对照**：Task 4.4 输出 `conformance.md`，spec 与落地代码逐条对齐（fully/aligned/partial/gap），作为合入 PR 的前置门槛

## 改动面（最小化）

`go.mod`、`pkg/convert/convert.go`（新增）、`cmd/okf/cmd_add.go`、`pkg/mcp/tools.go` + 3 个测试文件。**不改** `CollectFiles`/`SmartImportSource`/`ImportResult`/watch 配置。

---

## Phase 0: Go 版本升级与依赖引入（0.5 人日）

> 对齐 spec: `Requirement: Go 版本升级到 1.26`（场景 S1, S2）

### Task 0.1: 升级 Go 版本
- File: `go.mod`
- Description: `go 1.22` → `go 1.26.0`，加 `toolchain go1.26.7`
- **接线点**: 命令级 `go build ./...` + `go test ./...`（CI/冒烟入口）↔ 全量回归
- Acceptance Criteria:
  - `go build ./...` 全项目编译通过
  - `go test ./...` 既有测试全部通过（验证升级无破坏）
- Tests: 全量回归（S1）
- Priority: High

### Task 0.2: 引入并锁定 downmark
- File: `go.mod` / `go.sum`
- Description: `go get github.com/giraffesyo/downmark@v0.10.0`（**精确版本，不浮动**）
- **接线点**: `go.mod` 版本断言 ↔ `TestDependencyPinned`（新增）
- Acceptance Criteria:
  - go.mod 记录 downmark 及纯 Go 传递依赖
  - `go mod tidy` 无残留
  - 依赖清单核对：全部 MIT/BSD（无 AGPL）
- Tests（S2）:
  - `TestDependencyPinned`（断言 go.mod 锁定 downmark v0.10.0，非浮动）
  - `go list -m all` 版本核对
- Priority: High

---

## Phase 1: pkg/convert 统一转换层（2 人日，TDD 核心）

> 对齐 spec: `Requirement: 提供统一文档转换层`（S3-S6）、`Requirement: 格式识别与路由`（S7-S9）

### Task 1.1: 生成并入库 testdata fixtures
- File: `pkg/convert/testdata/`（.pdf/.docx/.xlsx/.pptx/.html/.csv/.txt）
- Description: 用 Python（python-docx/openpyxl/python-pptx/reportlab，环境已有）生成固定样本；HTML/CSV/TXT 手写；git 入库
- **接线点**: fixtures 目录 ↔ `TestFixturesPresent`（样本存在且非空自检）
- Acceptance Criteria: 每个受支持格式至少 1 个固定样本，内容确定（便于 golden 断言）
- Tests: `TestFixturesPresent`
- Priority: High

### Task 1.2: 定义 Result / Options / 错误类型
- File: `pkg/convert/convert.go`
- Description:
  - `Result{Markdown, Title, Warnings []string}`
  - `Options{MaxInputBytes=64MiB, Timeout=60s}` + `DefaultOptions()`
  - `ErrUnsupportedFormat`（唯一自定义错误）
- **接线点**: `Result`/`Options`/`DefaultOptions`/`ErrUnsupportedFormat` ↔ `TestDefaultOptions`
- Acceptance Criteria: 与 design.md 数据模型一致
- Tests: `TestDefaultOptions`
- Priority: High

### Task 1.3: 格式识别 IsSupportedDocument / DocumentType
- File: `pkg/convert/convert.go`
- Description: 按扩展名（小写化）识别 pdf/docx/doc/xlsx/pptx/html/htm/csv/txt；`.md` 返回 false；识别优先级注释说明（归档>文档>md）
- **接线点**: `IsSupportedDocument`/`DocumentType` ↔ `TestIsSupportedDocument*`/`TestDocumentType`
- Acceptance Criteria: 对齐 spec S7/S8/S9
- Tests（TDD 先行）:
  - `TestIsSupportedDocument`（S8 正例）
  - `TestIsSupportedDocument_CaseInsensitive`（S9）
  - `TestIsSupportedDocument_NotMarkdown`（S8 `.md`=false）
  - `TestFormatRoutingPriority`（S7 归档>文档>md>其他）
  - `TestDocumentType`（S8 映射）
- Priority: High

### Task 1.4: ConvertToMarkdown 核心实现
- File: `pkg/convert/convert.go`
- Description: `all.New(all.Options{KeepDataURIs:false})` + `ConvertFile`；`os.Stat` 前置输入检查；`context.WithTimeout` + `downmark.ResultLimit`；Warnings 收敛 `[]Warning`→`[]string`
- **接线点**: `ConvertToMarkdown` ↔ `TestConvert*` 全系列
- Acceptance Criteria: 对齐 spec S3/S4/S6/S16/S17/S18
- Tests（TDD 先行，用 testdata fixtures + golden files）:
  - `TestConvertPDF` / `TestConvertDOCX` / `TestConvertXLSX` / `TestConvertPPTX` / `TestConvertHTML` / `TestConvertCSV` / `TestConvertTXT`（S3）
  - `TestConvertExtractsTitle`（S4 标题提取）
  - `TestConvertWarningsSummarized`（S4 Warnings 字符串摘要）
  - `TestConvertUnsupportedFormat`（S6 `.exe` → `ErrUnsupportedFormat`）
  - `TestConvertInputTooLarge`（S16 超限输入被拒）
  - `TestConvertTimeout`（S17 超时中止）
  - `TestConvertNoTextPDF`（S18 扫描 PDF 透传 downmark no-text 错误，`errors.Is` 匹配）
- Golden 策略: `testdata/golden/*.golden` 记录转换输出；文件头注释锁定 downmark v0.10.0；升级依赖后需重新生成并人工确认 diff
- Priority: High

### Task 1.5: 转换产物可被 okf parser 解析
- File: `pkg/convert/convert_test.go`
- Description: 转换产物包裹 frontmatter 后走 `parser.ParseConceptBytes`
- **接线点**: `ConvertToMarkdown` → `wrapFrontmatter`（cmd 层） ↔ `TestConvertedOutputParsableByOKF`
- Acceptance Criteria: 对齐 spec S5
- Tests: `TestConvertedOutputParsableByOKF`（转换 → 包 frontmatter → 解析 → Content 非空含正文）
- Priority: High

---

## Phase 2: cmd_add 前置集成（1.5 人日）

> 对齐 spec: `Requirement: okf add 命令自动转换文档`（S10-S15）、`Requirement: 压缩包内文档自动转换`（S26）、`Requirement: 文档导入身份与重导入语义`（S20-S21）、`Requirement: 导入摘要报告转换统计`（S27）、MODIFIED `归档文件导入`（S28）

### Task 2.1: 前置编排 convertAndStageDocuments
- File: `cmd/okf/cmd_add.go`（新增函数，不侵入既有 cmdAdd 主流程）
- Description:
  - 归档 → 提取临时目录（复用 `ExtractArchive`）
  - 遍历收集 `.md` + 受支持文档（新增收集，不改 `CollectFiles`）
  - 文档 → `ConvertToMarkdown` → `wrapFrontmatter` → 写入 staging 临时目录
  - 产物命名 `<原名>.md`
  - `defer cleanup` 清理 staging 与解压目录
- **接线点**: `convertAndStageDocuments`/`wrapFrontmatter` ↔ `TestConvertAndStage_*`
- Acceptance Criteria: 对齐 spec S10/S11/S12/S15/S19/S26/S28
- Tests（TDD 先行）:
  - `TestConvertAndStage_SingleDoc`（S10 单个 PDF → staging .md）
  - `TestConvertAndStage_MixedDir`（S11 md + pdf + xlsx 混合）
  - `TestConvertAndStage_Archive`（S26/S28 ZIP 内 pdf/docx）
  - `TestConvertAndStage_Naming`（S12 命名 `<原名>.md`）
  - `TestConvertAndStage_Cleanup`（S19 失败/完成后 staging 被清理）
  - `TestConvertAndStage_Unsupported`（S15 失败不中断，返回失败计数）
- Priority: High

### Task 2.2: cmdAdd 接入 staging 结果
- File: `cmd/okf/cmd_add.go`
- Description: cmdAdd 将 staging 目录（或单 .md）交给既有 `SmartImportSource`；`--dry-run` 时只预览不写；摘要追加 `Converted (documents): N`
- **接线点**: `cmdAdd` 主流程（新增前置分支）↔ `TestCmdAdd*`
- Acceptance Criteria: 对齐 spec S13/S14/S27
- Tests:
  - `TestCmdAddSingleDocument`（S10/S11 端到端 `okf add report.pdf` → 知识库含 `report.pdf.md`）
  - `TestCmdAddDryRunNoWrite`（S14 dry-run 不写知识库）
  - `TestCmdAddMixedReport`（S27 混合目录导入摘要正确）
- Priority: High

### Task 2.3: 身份与重导入语义确认
- File: `cmd/okf/cmd_add.go`（+ 文档说明）
- Description: 确认 staging 中 `.md` 的 `SourcePath` 对应原文档路径（smart_import 元数据用原路径）；downmark 版本锁定由 go.mod 保证
- **接线点**: `SmartImportSource` 元数据（原路径身份）↔ `TestCmdAddReimportNoChange` + `TestReleaseNotesDeclaresReimport`（文档断言）
- Acceptance Criteria: 对齐 spec S20/S21
- Tests:
  - `TestCmdAddReimportNoChange`（S20 二次导入同文档 → no-change，身份稳定）
  - `TestReleaseNotesDeclaresReimport`（S21 文档断言：Release Notes 声明 downmark 升级触发受控重导入 + 规避方式）
- Priority: Medium

---

## Phase 3: MCP 集成（1 人日）

> 对齐 spec: `Requirement: MCP 提供文档导入工具`（S22-S25）

### Task 3.1: 注册 okf_import_document 工具
- File: `pkg/mcp/tools.go`
- Description: 新增工具（`path` 必填，`title`/`type` 可选默认 `source`）；handler 转换 + 写当前已加载 bundle 路径；未加载 bundle 返回错误
- **接线点**: `okf_import_document` 工具注册 + handler ↔ `TestMCP*`
- Acceptance Criteria: 对齐 spec S22/S23/S24/S25
- Tests（TDD 先行）:
  - `TestMCPSchemaHasImportDocument`（S22 tools/list 含新工具且 schema 正确）
  - `TestMCPImportRequiresBundle`（S23 未加载 bundle → IsError）
  - `TestMCPImportSuccess`（S22 转换+写入成功返回）
  - `TestMCPImportWithOverrides`（S24 title/type 覆盖）
  - `TestMCPImportError`（S25 转换失败 → IsError）
- Priority: High

### Task 3.2: MCP 端到端测试更新
- File: `test_mcp.py`
- Description: 新增 `okf_import_document` 用例；测试序列：先 `okf_load_bundle` 再 `okf_import_document`；用 testdata 的 DOCX/PDF 样本
- **接线点**: MCP stdio 协议 ↔ `test_import_document`（Python 端到端）
- Acceptance Criteria: 对真实样本调用工具并断言导入成功（S22）
- Tests: `test_import_document`（新增 Test Case）
- Priority: Medium

---

## Phase 4: 回归、一致性对照、文档与发布（1.5 人日）

### Task 4.1: 全量回归
- Command: `go build ./... && go vet ./... && go test ./... && python3 test_mcp.py`
- **接线点**: 全量命令 ↔ 覆盖率矩阵终态（所有 ✅）
- Acceptance Criteria:
  - 编译/vet/测试全部通过
  - golden fixtures 锁定转换行为
  - MCP 端到端含新工具全部通过
- Priority: High

### Task 4.2: CLI 帮助与 README
- File: `cmd/okf/main.go`（usage）、`README.md`、`README.zh-CN.md`
- Description: 声明新支持格式、`okf add report.pdf` 示例、`okf add <非md>` 行为变更说明、最低 Go 版本
- Acceptance Criteria: 帮助文本与 README 覆盖新格式与行为变更
- Priority: Medium

### Task 4.3: 知识库文档与 Release Notes
- File: `docs/knowledge/cli.md`
- Description: 更新 CLI 知识库条目；生成发布说明（含 downmark 版本锁定、重导入语义声明）
- **接线点**: Release Notes 声明 ↔ `TestReleaseNotesDeclaresReimport`（S21）
- Acceptance Criteria: 知识库 lint 通过（`okf lint` 0 errors）；Release Notes 含 S21 声明
- Priority: Medium

### Task 4.4: spec-实现一致性对照审计（新增）
- File: `openspec/changes/add-document-format-import/conformance.md`（交付物）
- Description: 逐条 spec Scenario（28 条）对照：实现位置（函数/命令）→ 测试用例 → 实测结果 → 对齐度（fully/aligned/partial/gap）。以覆盖矩阵为骨架生成待核对清单；openspec CLI 可用时叠加 `openspec validate`；人工复核
- **接线点**: spec ↔ 落地代码 ↔ 测试 ↔ `conformance.md`
- Acceptance Criteria:
  - 28 条 Scenario 全部有实现与测试记录，无 gap
  - `partial`/`gap` 项 MUST 给出原因与后续任务
  - 作为合入 PR 的前置门槛（未通过不予合入）
- Priority: High（合入门槛）

---

## 覆盖率矩阵（spec Scenario ↔ 测试 ↔ 接线函数）

> 共 28 个 Scenario，全部映射到可执行测试。**验收时逐项打勾，缺一不可。**

| # | Spec Scenario | 测试用例 | 测试文件 | 接线函数/入口 |
|---|---------------|----------|----------|----------------|
| S1 | 全项目 Go 1.26 构建通过 | 全量回归（build+test） | —（CI） | `go build` / `go test` |
| S2 | 引入并锁定 downmark | `TestDependencyPinned` | convert_test.go | go.mod 断言 |
| S3 | 支持的核心格式 | `TestConvert{PDF,DOCX,XLSX,PPTX,HTML,CSV,TXT}` | convert_test.go | `ConvertToMarkdown` |
| S4 | 标题与警告摘要 | `TestConvertExtractsTitle` / `TestConvertWarningsSummarized` | convert_test.go | `ConvertToMarkdown` |
| S5 | 转换产物可被 parser 解析 | `TestConvertedOutputParsableByOKF` | convert_test.go | `ConvertToMarkdown`+`wrapFrontmatter` |
| S6 | 不支持的格式返回明确错误 | `TestConvertUnsupportedFormat` | convert_test.go | `ConvertToMarkdown`→`ErrUnsupportedFormat` |
| S7 | 识别优先级 归档>文档>md | `TestFormatRoutingPriority` | convert_test.go | `IsSupportedDocument`/`DocumentType` |
| S8 | IsSupportedDocument 语义 | `TestIsSupportedDocument` / `TestIsSupportedDocument_NotMarkdown` | convert_test.go | `IsSupportedDocument` |
| S9 | 大小写不敏感 | `TestIsSupportedDocument_CaseInsensitive` | convert_test.go | `IsSupportedDocument` |
| S10 | 添加单个文档文件 | `TestCmdAddSingleDocument` | cmd_add_test.go | `cmdAdd`→`convertAndStageDocuments` |
| S11 | 添加目录含多种文档 | `TestCmdAddSingleDocument` / `TestConvertAndStage_MixedDir` | cmd_add_test.go | `convertAndStageDocuments` |
| S12 | 转换产物命名规则 | `TestConvertAndStage_Naming` | cmd_add_test.go | `wrapFrontmatter` |
| S13 | 转换产物参与智能变更检测 | `TestCmdAddReimportNoChange` | cmd_add_test.go | `SmartImportSource`（元数据） |
| S14 | Dry-run 展示转换预览 | `TestCmdAddDryRunNoWrite` | cmd_add_test.go | `cmdAdd` dry-run 分支 |
| S15 | 转换失败不中断批量导入 | `TestConvertAndStage_Unsupported` | cmd_add_test.go | `convertAndStageDocuments` |
| S16 | 超大输入文件被拒绝 | `TestConvertInputTooLarge` | convert_test.go | `ConvertToMarkdown`（os.Stat） |
| S17 | 转换超时被中止 | `TestConvertTimeout` | convert_test.go | `ConvertToMarkdown`（context） |
| S18 | 扫描版 PDF 明确报错 | `TestConvertNoTextPDF` | convert_test.go | `ConvertToMarkdown`（错误透传） |
| S19 | 临时文件被清理 | `TestConvertAndStage_Cleanup` | cmd_add_test.go | `cleanup`（defer RemoveAll） |
| S20 | 变更检测基于原文档身份 | `TestCmdAddReimportNoChange` | cmd_add_test.go | `SmartImportSource` 元数据 |
| S21 | downmark 升级触发受控重导入 | `TestReleaseNotesDeclaresReimport` | cmd_add_test.go | Release Notes 文档断言 |
| S22 | Agent 通过 MCP 导入文档 | `TestMCPImportSuccess` / `test_import_document` | tools_test.go / test_mcp.py | `okf_import_document` handler |
| S23 | 未加载 bundle 返回错误 | `TestMCPImportRequiresBundle` | tools_test.go | `okf_import_document` handler |
| S24 | 可选参数控制 frontmatter | `TestMCPImportWithOverrides` | tools_test.go | `okf_import_document` handler |
| S25 | 转换失败返回错误 | `TestMCPImportError` | tools_test.go | `okf_import_document` handler |
| S26 | ZIP 内含 PDF/DOCX | `TestConvertAndStage_Archive` | cmd_add_test.go | `convertAndStageDocuments`（ExtractArchive） |
| S27 | 摘要显示转换文档数 | `TestCmdAddMixedReport` | cmd_add_test.go | `cmdAdd` 本地统计 |
| S28 | 归档含文档成员 | `TestConvertAndStage_Archive` | cmd_add_test.go | `convertAndStageDocuments` |

---

## 测试规划总览

| 层 | 测试文件 | 覆盖 spec 场景 |
|----|----------|----------------|
| 单元 | `pkg/convert/convert_test.go` | S2-S9, S16-S18 |
| 集成 | `cmd/okf/cmd_add_test.go`（扩展） | S10-S15, S19-S21, S26-S28 |
| MCP | `pkg/mcp/tools_test.go`（扩展）+ `test_mcp.py` | S22-S25 |
| 命令/文档 | 全量回归 + `TestDependencyPinned`/`TestReleaseNotesDeclaresReimport` | S1, S2, S21 |
| 一致性 | `conformance.md`（Task 4.4 产出） | 28 场景全量对照 |

**Fixture 策略**：`pkg/convert/testdata/` 由 Python 生成固定样本并 git 入库（可复现、版本可控）。
**Golden 策略**：`testdata/golden/` 记录转换输出，文件头注释锁定 downmark v0.10.0；升级依赖需人工确认 diff 后重新生成。

## 开发排期（仅表依赖顺序，非交付边界）

| 阶段 | 内容 | 工期 | 依赖 |
|------|------|------|------|
| P0 | Go 升级 + downmark 引入 | 0.5 人日 | 无 |
| P1 | pkg/convert 转换层（TDD） | 2 人日 | P0 |
| P2 | cmd_add 前置集成 | 1.5 人日 | P1 |
| P3 | MCP 集成 | 1 人日 | P1 |
| P4 | 回归 + 一致性对照 + 文档 + 发布 | 1.5 人日 | P2/P3 |
| **合计** | | **6.5 人日** | |

> 说明：P0-P4 为依赖先后顺序；**只有 P0-P4 全部完成（含 Task 4.4 一致性对照通过）才视为本变更完成**，任何中间阶段不构成可交付边界。

## 验收清单（对齐 spec 全场景 + 完成门槛）

- [ ] P0-P4 全部 Task 实施完成（不得只开发 P0/P1）（门槛 1）
- [ ] Go 1.26 构建 + 全量测试通过（S1）
- [ ] downmark 版本精确锁定 + `TestDependencyPinned` 通过（S2）
- [ ] 7 种格式转换正确 + golden 锁定 + parser 可解析（S3-S5）
- [ ] 格式识别优先级 + 大小写不敏感 + `.md` 不转换（S7-S9）
- [ ] 输入超限/超时/扫描 PDF 明确报错（S16-S18）
- [ ] `okf add` 单文档/混合目录/归档转换 + 命名 `<原名>.md`（S10-S12, S26, S28）
- [ ] 失败不中断批量 + 临时文件清理 + dry-run 不写（S14-S15, S19）
- [ ] 身份稳定：二次导入 no-change；downmark 升级语义已声明（S13, S20-S21）
- [ ] MCP `okf_import_document` 可用 + bundle 前置 + 参数覆盖（S22-S25）
- [ ] 28 个 Scenario 覆盖率矩阵全部打勾（门槛 2/3）
- [ ] Task 4.4 一致性对照 `conformance.md` 通过、无 gap（门槛 4）
- [ ] README/CLI 帮助/知识库文档更新 + lint 通过
