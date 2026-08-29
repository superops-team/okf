# Proposal: Document Format Import

## Summary

为 okf 知识库导入管道增加丰富的文档格式支持（PDF、DOCX、XLSX、PPTX、HTML、CSV、TXT 等），使 `okf add` 能直接消费用户日常文档，而不局限于 Markdown 文件。方案采用 **纯 Go 转换层（downmark，MIT 许可）**，不依赖 Python、CGO 或任何外部二进制，本地化完成全部转换。转换产物为标准 `.md`，直接喂给既有 `SmartImportSource` 管道，知识库核心模型零改动。

## Motivation

- **补齐最大短板**：与 OpenKB 等同类项目对比，okf 目前仅支持 `.md` 文件与压缩包导入，无法处理 PDF/Word/Excel/PPT/HTML 等人类日常文档，是功能差距最大的维度
- **纯 Go 本地化诉求**：已验证 `downmark`（Microsoft markitdown 的原生 Go 重写）可编译、可运行、可集成，覆盖 7 种格式，性能为 Python 版 30-50 倍，无外部依赖
- **Agent 工作流**：MCP 服务器已就绪，补齐文档导入后，AI Agent 可直接通过 `okf_import_document` 工具把任意文档导入知识库
- **复用既有管道**：转换产物为标准 Markdown，可直接走现有 `add`/`smart_import` 管道，无需改动知识库核心模型

## Requirements

### MUST
- 将 Go 版本升级到 1.26（downmark v0.10.0 硬性要求 `go >= 1.26.0`），并**精确锁定** downmark 版本
- 新增纯 Go 统一转换层 `pkg/convert`，封装 downmark 库，提供 `ConvertToMarkdown` 统一入口
- 支持以下格式转换：PDF、DOCX、XLSX、PPTX、HTML、CSV、纯文本（TXT）
- `okf add` 在导入入口前自动识别并转换非 Markdown 文档（**前置转换层，不改 `CollectFiles`/`SmartImportSource`/`ImportResult`**）
- 转换产物为标准 `.md`，命名 `<原名>.md`，可被 okf parser 正常解析（包裹 frontmatter 后入库）
- 对不支持的格式给出明确错误（`ErrUnsupportedFormat`），不静默跳过；单文件失败不中断批量导入
- 提供安全防护：输入文件大小上限、转换超时、downmark 输出上限、临时目录清理
- 文档导入身份稳定：变更检测基于原文档身份，downmark 升级触发受控重导入并在 Release Notes 声明

### SHOULD
- 支持 DOC（Word 97-2003 二进制格式）
- 支持 ZIP 压缩包内文档成员的自动转换
- MCP 新增 `okf_import_document` 工具，agent 可直接导入文档（写入当前已加载 bundle）
- 转换结果包含标题（title）提取与警告摘要，用于生成 frontmatter 与提示转换损失

### MAY
- 支持 OCR（扫描版 PDF，接入用户提供的 tesseract，默认关闭）
- 支持 EPUB 等其他格式（downmark 后续版本扩展）

## Non-Goals

- 不实现文档渲染、预览或格式美化（转换目标仅为 LLM/知识库友好的 Markdown）
- 不引入 Python markitdown 或任何 CGO 依赖
- 不改变知识库核心模型（Concept/Bundle）与 OKF v0.2 规范
- 不内置 OCR 引擎（仅预留可插拔接口，默认关闭）
- 不新增 watch 配置开关（现有 `Patterns` glob 已可监听文档，本次不改 watch 行为）

## Impact

- **`go.mod`**：Go 版本 1.22 → 1.26；新增 `github.com/giraffesyo/downmark v0.10.0`（精确锁定，及其纯 Go 传递依赖）
- **`pkg/convert`（新增）**：统一转换层，按格式路由到 downmark 各包，输出 `{Markdown, Title, Warnings []string}`
- **`cmd/okf/cmd_add.go`**：新增前置编排 `convertAndStageDocuments`（归档提取 + 文档转换 + staging），接入既有 `SmartImportSource`
- **`pkg/mcp/tools.go`**：新增 `okf_import_document` MCP 工具
- **测试**：新增 `pkg/convert` 单测（fixtures + golden）、cmd_add 前置集成测试、MCP 导入工具测试
- **文档**：README/CLI 帮助/知识库文档声明新格式、行为变更与最低 Go 版本

## Risks

- **Go 版本升级**：1.22 → 1.26 是较大跨度，存量用户需升级 Go 才能构建（本地已验证可构建、测试通过）；README/Release Notes 标注最低版本
- **身份/重导入语义**：downmark 升级可能导致已导入文档全量重导入 —— 通过**精确锁版本** + 受控重导入语义 + Release Notes 声明缓解（见 design.md）
- **downmark 新库稳定性**：2026 年发布、版本 v0.10.0，需通过 fixtures + golden 测试锁定行为
- **扫描版 PDF**：无 OCR 时文本为空，透传 downmark no-text 错误（`errors.Is` 可匹配），不静默产出空内容
- **二进制体积**：引入 downmark 全格式后 okf 二进制增大（验证约 10MB），可接受但需记录；如体积敏感可改按需 import downmark 子包
