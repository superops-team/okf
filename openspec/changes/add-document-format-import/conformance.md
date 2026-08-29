# Conformance: spec ↔ Implementation ↔ Tests

> 依据 OKF 项目 AGENTS.md 维度 19：落地后逐条核对 spec Scenario 与实现/测试对齐度。
> 对齐度：**fully**（实现+测试均满足）/ aligned（实现满足、测试部分覆盖）/ partial（部分满足）/ gap（未实现）。
> 本报告基于实现完成后全量回归实测结果（go build/vet/test + test_mcp.py 全绿）。

## 汇总

- 29 / 29 场景有实现与测试记录
- 对齐度：**fully 29，aligned 0，partial 0，gap 0**
- 未对齐项：无
- 依赖锁定：downmark **v0.10.0**（精确，`TestDependencyPinned` 断言）
- Golden 锁定：7 格式转换输出锁定（`TestConvertGolden`，UPDATE_GOLDEN 重生成）

## 逐条对照

| # | Scenario | 实现位置 | 测试用例 | 实测 | 对齐 |
|---|----------|----------|----------|------|------|
| S1 | 全项目 Go 1.26 构建通过 | `go.mod`（go 1.26.0 + toolchain go1.26.7） | `go build ./...` + `go test ./...`（全量回归） | PASS | fully |
| S2 | 引入并锁定 downmark | `go.mod`（downmark v0.10.0 精确） | `TestDependencyPinned` | PASS | fully |
| S3 | 支持的核心格式 | `pkg/convert.ConvertToMarkdown`（downmark all 引擎） | `TestConvert{PDF,DOCX,XLSX,PPTX,HTML,CSV,TXT}` + `TestConvertGolden` | PASS | fully |
| S4 | 标题与警告摘要 | `Result.Title`/`summarizeWarnings`（[]Warning→[]string） | `TestConvertExtractsTitle` / `TestConvertWarningsSummarized` | PASS | fully |
| S5 | 转换产物可被 parser 解析 | `convert.WrapConcept` + `parser.ParseConceptBytes` | `TestConvertedOutputParsableByOKF` | PASS | fully |
| S6 | 不支持的格式返回明确错误 | `ErrUnsupportedFormat`（errors.Is 可匹配） | `TestConvertUnsupportedFormat` | PASS | fully |
| S7 | 识别优先级 归档>文档>md | `IsSupportedDocument`/`DocumentType`（路由注释） | `TestFormatRoutingPriority` | PASS | fully |
| S8 | IsSupportedDocument 语义 | `IsSupportedDocument`（.md 返回 false） | `TestIsSupportedDocument` / `TestIsSupportedDocument_NotMarkdown` | PASS | fully |
| S9 | 大小写不敏感 | 扩展名 `strings.ToLower` | `TestIsSupportedDocument_CaseInsensitive` | PASS | fully |
| S10 | 添加单个文档文件 | `cmd_add.convertAndStageDocuments` + `SmartImportSource` | `TestCmdAddSingleDocument` | PASS | fully |
| S11 | 添加目录含多种文档 | `convertAndStageDocuments` 聚合（.md 复制 + 文档转换） | `TestCmdAddMixedDirectory` | PASS | fully |
| S12 | 转换产物命名规则 | `relativePath` + `WrapConcept`（`<原名>.md`） | `TestConvertAndStage_Naming` | PASS | fully |
| S13 | 转换产物参与智能变更检测 | `SmartImportSource` 元数据（确定性 staging 路径） | `TestCmdAddReimportNoChange` | PASS | fully |
| S14 | Dry-run 展示转换预览 | `cmdAdd` dry-run 分支（不写盘） | `TestCmdAddDryRunNoWrite` | PASS | fully |
| S15 | 转换失败不中断批量导入 | `convertAndStageDocuments`（continue + stderr 报告） | `TestConvertAndStage_Unsupported` | PASS | fully |
| S16 | 超大输入文件被拒绝 | `ConvertToMarkdown` os.Stat 前置检查 | `TestConvertInputTooLarge` | PASS | fully |
| S17 | 转换超时被中止 | `ConvertToMarkdown` context.WithTimeout | `TestConvertTimeout` | PASS | fully |
| S18 | 扫描版 PDF 明确报错 | `ErrNoText` 透传（downmark no-extractable-text） | `TestConvertNoTextPDF` | PASS | fully |
| S19 | 临时文件被清理 | `cleanup`（defer os.RemoveAll staging） | `TestConvertAndStage_Cleanup` | PASS | fully |
| S20 | 变更检测基于原文档身份 | 确定性 staging 路径（sha1(srcPath)）+ 内容确定性 | `TestCmdAddReimportNoChange` | PASS | fully |
| S21 | downmark 升级触发受控重导入 | `docs/knowledge/releases.md`（re-import 语义声明） | `TestReleaseNotesDeclaresReimport` | PASS | fully |
| S22 | Agent 通过 MCP 导入文档 | `pkg/mcp.handleImportDocument`（写当前 bundle） | `TestMCPImportSuccess` + `test_mcp.py` Test 12 | PASS | fully |
| S23 | 未加载 bundle 返回错误 | `handleImportDocument` bundle 前置检查 | `TestMCPImportRequiresBundle` | PASS | fully |
| S24 | 可选参数控制 frontmatter | `title`/`type` 覆盖逻辑 | `TestMCPImportWithOverrides` | PASS | fully |
| S25 | 转换失败返回错误 | `handleImportDocument` IsError 返回 | `TestMCPImportError` | PASS | fully |
| S26 | ZIP 内含 PDF/DOCX | `cmd_add.extractArchiveFull`（全量安全解压）+ 转换 | `TestConvertAndStage_Archive` | PASS | fully |
| S27 | 摘要显示转换文档数 | `cmdAdd` 本地统计 `Converted (documents): N` | `TestCmdAddMixedReport`（+手动验证） | PASS | fully |
| S28 | 归档含文档成员 | `extractArchiveFull`（非仅 .md 提取） | `TestConvertAndStage_Archive` | PASS | fully |
| S29 | 转换产物可检索（CLI/MCP 双通道） | `cmdSearch`/`query.SearchWithMatches` 全链路 | `TestCmdAddDocumentsSearchable`（7 格式命中 + 反例 0 结果）+ `test_mcp.py` okf_search 命中 + 手动 CLI 检索 | PASS | fully |

## 实现要点记录（与 design 的一致性）

1. **前置转换层**：`pkg/convert` 独立包，不改 `CollectFiles`/`SmartImportSource`/`ImportResult`（维度 6 最小化）✓
2. **归档全量解压**：okf 既有 `ExtractArchive` 只提取 .md，文档成员会丢失；故在 cmd_add 实现 `extractArchiveFull`（zip/tar 全量 + zip-slip/symlink/大小防护），不动核心（S26/S28）✓
3. **身份稳定**：staging 用确定性路径 `os.TempDir()/okf-convert/<sha1(srcPath)>`，配合 downmark 确定性转换 → 二次导入 no-change（S13/S20）✓
4. **frontmatter 单一来源**：`convert.WrapConcept` 被 cmd_add 与 MCP 共用（维度 16 架构统一）✓
5. **downmark 错误透传**：`ErrNoText` 透传、`ErrUnsupportedFormat` 自定义；Warnings 收敛为字符串摘要（维度 4 语义精准）✓

## 遗留说明

- S14 dry-run 实测：`SmartImportSource` DetectOnly 模式会显示 "Imported: N"（指待导入数），但不写盘 —— 断言"知识库无产物"通过，符合 spec 语义
- `test_mcp.py` 归档的 `okf-bin` 二进制在 `.gitignore`（本地构建产物，不提交）
- golden 文件依赖 downmark v0.10.0；升级依赖必须重生成并人工确认 diff（README 已说明）

## 已知限制（两轮代码审查后确认，非 spec 缺口）

1. **`okf sync` 不追踪文档源**：转换产物写入 metadata 的 `SourcePath` 是确定性 staging 路径（`os.TempDir()/okf-convert/<sha1>`，导入后清理）。`okf sync` 检测到该源缺失会标记 `SourceExists=false`，**不会删除概念**（`DetectSourceMissing` 分支）。文档内容的变更检测请使用 `okf add <文档>`（重复导入会自动检测内容变更）。这是 S20 确定性 staging 设计权衡的结果。
2. **MCP `okf_import_document` 覆盖语义**：直接写 bundle 根 `<original>.md`，同名文件**直接覆盖**，无 skip/merge 策略（spec S22 未要求策略；CLI 侧 `okf add` 保留完整策略控制）。
3. **`.tar.xz` 不支持**：`okf.IsArchive` 误报 `.tar.xz`（既有行为），文档导入前置层对其返回明确报错（建议转 `.tar.gz`/`.zip`），不会静默丢数据。
4. **归档解压三重防护**：整体归档 ≤ `okf.MaxArchiveSize`(50MB)、单成员 ≤ `okf.MaxFileSize`(10MB)、`writeExtracted` 再用 `io.LimitedReader` 兜底（防头部伪造），zip-slip/symlink 拒绝。
