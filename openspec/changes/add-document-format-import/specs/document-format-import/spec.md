# Specification: Document Format Import

## Overview

本规范定义 okf 知识库对丰富文档格式（PDF、DOCX、XLSX、PPTX、HTML、CSV、TXT 等）的导入支持。通过纯 Go 转换层（downmark）将文档转换为 Markdown，接入 `okf add` 导入入口。所有转换均在本地完成，不依赖 Python、CGO 或外部二进制。转换产物为标准 `.md`，直接复用既有 `SmartImportSource` 管道，知识库核心模型零改动。

## ADDED Requirements

### Requirement: Go 版本升级到 1.26

系统 SHALL 将 Go 版本升级到 1.26.0，以满足 downmark v0.10.0 的 `go >= 1.26.0` 硬性要求。

#### Scenario: 全项目在 Go 1.26 下构建通过

- **WHEN** 在 `go 1.26.0`（toolchain go1.26.7）环境下执行 `go build ./...`
- **THEN** 系统 MUST 编译通过，无错误
- **AND** 既有所有单元测试 MUST 通过（`go test ./...`）

#### Scenario: 引入并锁定 downmark 依赖

- **WHEN** 在 go.mod 添加 `github.com/giraffesyo/downmark v0.10.0`
- **THEN** 依赖 MUST 为 MIT 宽松许可
- **AND** 传递依赖 MUST 全部为纯 Go 实现（无 CGO）
- **AND** 版本 MUST 精确锁定（非浮动版本），以保证转换输出稳定

---

### Requirement: 提供统一文档转换层

系统 SHALL 提供 `pkg/convert` 包作为统一文档转 Markdown 入口。

#### Scenario: 支持的核心格式

- **WHEN** 调用 `ConvertToMarkdown(ctx, path, opts)` 处理以下扩展名
- **THEN** 系统 MUST 成功转换并返回结构化 Markdown：
  - `.pdf` — 文本流提取
  - `.docx` — 标题/列表/表格/加粗等结构化 Markdown
  - `.xlsx` — 每个 sheet 一个 `##` 标题 + Markdown 表格
  - `.pptx` — 每页带 `<!-- Slide number: N -->` 标记
  - `.html` / `.htm` — 标题/链接/表格/列表
  - `.csv` — Markdown 表格（字符集感知）
  - `.txt` / `.text` — 原样透传

#### Scenario: 转换结果包含标题与警告摘要

- **WHEN** 文档包含可提取的标题（如 DOCX 标题、HTML `<title>`、PPTX 首页标题）
- **THEN** 返回的 `Result.Title` MUST 包含该标题
- **AND** 当无法提取时 `Result.Title` MUST 为空字符串（由调用方回退到文件名）
- **AND** `Result.Warnings` MUST 为字符串摘要列表（由 downmark 结构化 Warning 收敛），用于展示转换损失

#### Scenario: 转换产物可被 okf parser 解析

- **WHEN** 将转换后的 Markdown 包裹 OKF frontmatter 后调用 `parser.ParseConceptBytes`
- **THEN** 解析 MUST 成功
- **AND** 产生的 Concept 的 Content MUST 包含完整的转换正文

#### Scenario: 不支持的格式返回明确错误

- **WHEN** 调用 `ConvertToMarkdown` 处理无法识别的扩展名（如 `.exe`、`.bin`）
- **THEN** 系统 MUST 返回 `ErrUnsupportedFormat`
- **AND** MUST NOT 静默跳过或产出空内容

---

### Requirement: 格式识别与路由

系统 SHALL 提供统一的文档格式判断能力，并明确识别优先级。

#### Scenario: 识别优先级为 归档 > 文档 > Markdown

- **WHEN** 处理输入路径
- **THEN** 系统 MUST 按以下优先级判定：
  1. 归档格式（`.zip`/`.tar`/`.tar.gz` 等）→ 走归档提取路径
  2. 受支持文档扩展名（`.pdf`/`.docx`/`.xlsx`/`.pptx`/`.html`/`.csv`/`.txt`/`.doc`）→ 走转换路径
  3. `.md`/`.markdown` → 直接走既有导入管道（不转换）
  4. 其他 → 报 `ErrUnsupportedFormat`

#### Scenario: IsSupportedDocument 语义

- **WHEN** 传入受支持文档路径（`.pdf`/`.docx`/`.xlsx`/`.pptx`/`.html`/`.csv`/`.txt`）
- **THEN** `IsSupportedDocument(path)` MUST 返回 `true`
- **AND** 传入 `.md` 时 MUST 返回 `false`（仅标识"需经转换层的非 md 文档"）

#### Scenario: 大小写不敏感的扩展名识别

- **WHEN** 传入 `.PDF`、`.DOCX`、`.XLSX` 等大写扩展名
- **THEN** 系统 MUST 识别为对应格式并正常转换

---

### Requirement: okf add 命令自动转换文档

系统 SHALL 让 `okf add` 对非 Markdown 文档自动执行转换后导入。

#### Scenario: 添加单个文档文件

- **WHEN** 运行 `okf add /path/to/report.pdf`
- **THEN** 系统 MUST 将 PDF 转换为 Markdown
- **AND** 自动补全 frontmatter（`type: source`、`title`、`description`）
- **AND** 将转换产物写入知识库，命名为 `<原名>.md`（如 `report.pdf.md`）
- **AND** 原文件保持不变（只读输入）

#### Scenario: 添加目录含多种文档

- **WHEN** 运行 `okf add /path/to/documents/`（内含 `.md`、`.pdf`、`.xlsx`、`.pptx`）
- **THEN** 系统 MUST 保留相对目录结构
- **AND** `.md` 文件直接导入
- **AND** 文档格式文件经转换后导入
- **AND** 不支持的格式跳过并报告错误，不影响其他文件导入

#### Scenario: 转换产物命名规则

- **WHEN** 将文档 `sample.pdf` 转换后导入
- **THEN** 知识库中产物 MUST 命名为 `sample.pdf.md`
- **AND** 命名 MUST 保留原扩展名，避免与同名 `.md` 冲突并便于溯源

#### Scenario: 转换产物参与智能变更检测

- **WHEN** 同一文档文件被再次 `okf add`
- **THEN** 系统 MUST 走既有 `smart_import` 变更检测（元数据比对）
- **AND** 依据 `-strategy` 决定 skip/overwrite/merge/patch

#### Scenario: Dry-run 展示转换预览

- **WHEN** 运行 `okf add /path/to/doc.pdf --dry-run`
- **THEN** 系统 MUST 展示将被转换并导入的文件清单
- **AND** MUST NOT 对知识库做任何写入

#### Scenario: 转换失败不中断批量导入

- **WHEN** 目录中有单个文档转换失败（格式不支持或损坏）
- **THEN** 系统 MUST 记录该文件错误并继续处理其他文件
- **AND** 最终汇总中 MUST 报告失败文件数与原因

---

### Requirement: 转换安全防护

系统 SHALL 对文档转换实施输入输出防护。

#### Scenario: 超大输入文件被拒绝

- **WHEN** 输入文件超过 `MaxInputBytes`（默认 64 MiB）
- **THEN** 系统 MUST 拒绝该文件并报告错误
- **AND** MUST NOT 触发实际转换

#### Scenario: 转换超时被中止

- **WHEN** 单次转换超过超时阈值（默认 60s）
- **THEN** 系统 MUST 中止该转换并报告错误

#### Scenario: 扫描版 PDF 无文本时明确报错

- **WHEN** 转换扫描版（无文本层）PDF
- **THEN** 系统 MUST 返回 downmark 的 no-text 错误（经 `errors.Is` 可匹配）
- **AND** 错误信息 MUST 提示该 PDF 可能需要 OCR 或替代文件
- **AND** MUST NOT 导入空内容概念

#### Scenario: 临时文件被清理

- **WHEN** 一次文档导入完成或失败退出
- **THEN** 系统 MUST 清理转换产生的临时 staging 目录与解压临时目录
- **AND** MUST NOT 在知识库或系统临时目录残留中间产物

---

### Requirement: 文档导入身份与重导入语义

系统 SHALL 保证文档导入的变更检测身份稳定，避免意外全量重导入。

#### Scenario: 变更检测基于原文档身份

- **WHEN** 文档已导入后再次执行 `okf add`
- **THEN** 元数据记录的源身份 MUST 对应原文档路径
- **AND** 未变化的文档 MUST 被判定为 no-change 并跳过

#### Scenario: downmark 升级触发受控重导入

- **WHEN** downmark 依赖版本升级导致转换输出变化
- **THEN** 已导入文档 MAY 触发一次性重导入（作为受控行为变更）
- **AND** 该行为 MUST 在 Release Notes 中声明
- **AND** 提供规避方式（清空 `.metadata.json` 后重导）

---

### Requirement: MCP 提供文档导入工具

系统 SHALL 在 MCP 服务器中提供 `okf_import_document` 工具。

#### Scenario: Agent 通过 MCP 导入文档

- **WHEN** 当前已加载 bundle 后调用 MCP 工具 `okf_import_document`，参数 `{"path": "/path/to/report.docx"}`
- **THEN** 服务器 MUST 将文档转换并写入当前已加载 bundle 的路径
- **AND** 返回结构化结果（导入状态、生成的产物路径、转换警告）

#### Scenario: 未加载 bundle 时返回明确错误

- **WHEN** 调用 `okf_import_document` 但未先加载 bundle
- **THEN** 工具 MUST 返回 `IsError: true` 的结果
- **AND** 错误信息 MUST 提示先调用 `okf_load_bundle`

#### Scenario: 可选参数控制 frontmatter

- **WHEN** 调用 `okf_import_document` 携带 `title` 或 `type` 参数
- **THEN** `type` MUST 覆盖默认的 `source`
- **AND** `title` MUST 覆盖转换提取的标题

#### Scenario: 转换失败返回错误

- **WHEN** 文档无法转换（格式不支持或损坏）
- **THEN** 工具 MUST 返回 `IsError: true` 的结果
- **AND** 错误信息 MUST 包含具体原因

---

### Requirement: 压缩包内文档自动转换

系统 SHALL 支持将压缩包内的受支持文档成员转换后导入。

#### Scenario: ZIP 内含 PDF/DOCX

- **WHEN** 运行 `okf add /path/to/archive.zip`（内含 `.pdf`、`.docx` 成员）
- **THEN** 系统 MUST 提取压缩包到临时目录
- **AND** 对文档成员执行转换后导入
- **AND** `.md` 成员直接导入
- **AND** 复用既有解压安全防护（路径清洗、成员上限）
- **AND** 导入完成后清理解压临时目录

---

### Requirement: 导入摘要报告转换统计

系统 SHALL 在 `okf add` 摘要中报告转换统计。

#### Scenario: 摘要显示转换文档数

- **WHEN** 一次导入涉及文档转换
- **THEN** 摘要 MUST 显示转换并导入的文档数（`Converted (documents): N`）
- **AND** 该统计由 `cmd_add` 本地累加，不修改 `ImportResult` 结构

---

## MODIFIED Requirements

### Requirement: 归档文件导入（扩展）

原归档导入能力 SHALL 扩展为：提取后的非 Markdown 文档成员同样经过转换层处理。

#### Scenario: 归档含文档成员

- **WHEN** 提取 ZIP/TAR 归档后扫描到 `.pdf`/`.xlsx` 等文档
- **THEN** 系统 MUST 将其转换后导入，而非仅收集 `.md`

---

## 说明（非 Requirement）

### watch 守护进程的文档监听（现有能力，非新增）

`.watch.yaml` 的 `Patterns` 已支持 glob（默认 `**/*.md`）。用户如需 watch 监听文档，可在配置中追加 `**/*.pdf` 等模式。本变更不新增 watch 配置开关、不改 watch 默认行为。
