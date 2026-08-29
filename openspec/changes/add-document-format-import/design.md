# Design: Document Format Import

## Overview

在 okf 现有导入链路（`okf add`）**入口之前**插入一层纯 Go 文档转 Markdown 转换层（基于 downmark v0.10.0）。转换产物是标准 Markdown（`.md`），**直接喂给既有 `SmartImportSource` 管道，知识库核心模型零改动**。本设计已通过本地实测：downmark 在 Go 1.26.7 下编译、7 种格式转换、okf parser 解析全部验证通过。

## 核心原则（最小实现）

- **转换是前置处理器，不是核心管道的一部分**：不改 `CollectFiles`、`SmartImportSource`、`ImportResult`、watch 配置
- **产物即标准 `.md`**：转换后的文档与用户手写 Markdown 在知识库中地位一致，复用全部既有能力（校验/lint/查询/变更检测）
- **改动面收敛到 4 个文件**：`go.mod`、`pkg/convert/convert.go`、`cmd/okf/cmd_add.go`、`pkg/mcp/tools.go`（+ 测试）

## Architecture

### 整体数据流（前置转换层）

```
                         ┌─────────────────────────────┐
   okf add <path> ──▶     │      cmd_add 前置编排          │
                         │  ┌─────────────────────────┐ │
                         │  │ 1. 归档? → 提取到临时目录  │ │  (复用 IsArchive/ExtractArchive)
                         │  │ 2. 收集文件（.md + 文档）   │ │  (新增 CollectImportFiles)
                         │  │ 3. 文档 → pkg/convert 转换 │ │  (新增, downmark)
                         │  │ 4. 补 frontmatter → .md    │ │  (新增 wrapFrontmatter)
                         │  └─────────────────────────┘ │
                         └──────────────┬──────────────┘
                                        ▼
                       临时目录（全部为 .md，含转换产物）
                                        │
                                        ▼
                    ┌─────────────────────────────────────┐
                    │ 既有 SmartImportSource（零改动）       │
                    │  变更检测/策略/元数据/写入              │
                    └─────────────────────────────────────┘
                                        │
                                        ▼
                                  知识库（.okf/knowledge）
```

### pkg/convert 包设计

```
pkg/convert/
├── convert.go      # 统一入口 ConvertToMarkdown、格式识别、防护
├── convert_test.go # 各格式 fixtures 测试 + golden 锁定
└── testdata/       # 固定测试样本（.pdf/.docx/.xlsx/.pptx/.html/.csv/.txt）
```

**核心接口：**

```go
// Result 是单次转换的产物
type Result struct {
    Markdown string   // 转换后的 Markdown 正文（无 frontmatter）
    Title    string   // 从文档提取的标题（可能为空）
    Warnings []string // 转换警告摘要（由 downmark []Warning 收敛为可读字符串）
}

// Options 控制转换行为（pkg/convert 自管，不直接透传 downmark 内部结构）
type Options struct {
    MaxInputBytes int64 // 输入文件大小上限（默认 64 MiB，os.Stat 前置检查）
    Timeout       time.Duration // 转换超时（默认 60s，经 context 传给 downmark）
}

// ConvertToMarkdown 将任意受支持文档转换为 Markdown。
// 输入超限/扫描 PDF 无文本/转换失败均返回可识别的错误（errors.Is 可匹配）。
func ConvertToMarkdown(ctx context.Context, path string, opts *Options) (*Result, error)

// IsSupportedDocument 报告路径是否为"需经转换层的非 md 文档"。
// 注意：.md 返回 false（Markdown 不经转换层，走既有管道）。
func IsSupportedDocument(path string) bool

// DocumentType 返回格式类型（pdf/docx/xlsx/pptx/html/csv/txt/doc）。
func DocumentType(path string) string
```

**内部机制说明（与 downmark 的实际对接）：**

| pkg/convert 关注点 | 实现 |
|--------------------|------|
| 格式路由 | 按扩展名（小写化）映射到 downmark 各注册格式 |
| 引擎 | `all.New(all.Options{KeepDataURIs: false})` 注册全部格式 |
| 输出上限 | 经 `context.WithValue` 设置 `downmark.ResultLimit`（不自定义 Options 字段） |
| 输入上限 | `os.Stat(path).Size()` 前置检查 `MaxInputBytes` |
| 扫描 PDF 无文本 | 透传 downmark 的 `errNoText`（`errors.Is` 匹配，不发明新类型） |
| Warnings | 由 downmark `[]Warning` 收敛为 `[]string`（`Converter: Location: err` 摘要） |

**格式路由表：**

| 扩展名 | 产物形态 |
|--------|----------|
| `.pdf` | 文本流（含 Form XObject/ToUnicode 复合字体） |
| `.docx` | 标题/列表/表格/加粗等结构化 Markdown |
| `.doc` | Word 97-2003 二进制文本 |
| `.xlsx` | 每 sheet 一个 `##` 标题 + Markdown 表格 |
| `.pptx` | 按页 `<!-- Slide number: N -->` + 内容 + 备注 |
| `.html` / `.htm` | 标题/链接/表格/列表（DOM 清理后） |
| `.csv` | Markdown 表格（字符集感知、分隔符嗅探） |
| `.txt` / `.text` | 原样透传（字符集检测） |
| `.md` / `.markdown` | **不转换**，直接走既有管道 |

### cmd_add 前置编排（新增函数，最小侵入）

在 `cmd_add.go` 增加 `convertAndStageDocuments(srcPath) (stagingDir string, convertedCount int, cleanup func(), err error)`：

1. 若 `IsArchive(srcPath)`：提取到临时目录（复用 `ExtractArchive`），根目录改为解压目录
2. 遍历目录收集文件（复用 `CollectFiles` 拿 `.md`，新增遍历拿受支持文档扩展名）
3. 对每个文档：`convert.ConvertToMarkdown` → `wrapFrontmatter` → 写入 staging 临时目录，与 `.md` 并列
4. 返回 staging 目录（全部 .md），交给既有 `SmartImportSource`
5. `cleanup` 由 `defer` 调用，`os.RemoveAll` 清理 staging 与解压临时目录

**frontmatter 补全规则（wrapFrontmatter）：**

```yaml
---
type: source                 # 定案：与既有 source 类型一致，不改 lint/查询语义
title: "<Result.Title>"      # 优先转换提取的标题
description: "Converted from <filename> (via <format>)"   # 可读模板
---
<Markdown 正文>
```

- 标题为空 → 回退到 `<原名>`（不含 `.md` 后缀，如 `report.pdf`）
- **转换产物命名：`<原名>.md`**（如 `report.pdf.md`），保留原扩展名防同名冲突、可溯源

### MCP 集成（pkg/mcp/tools.go）

新增工具 `okf_import_document`：

- 参数：`path`（必填）、`title`（可选覆盖）、`type`（可选，默认 `source`）
- **写入目标：当前已加载 bundle 的路径**；未加载 bundle 返回明确错误
- 内部：`ConvertToMarkdown` → `wrapFrontmatter` → 写入 bundle 根目录 → 返回导入结果
- 错误：转换失败返回 `IsError: true`，信息含具体原因

### watch 说明（非新功能）

`.watch.yaml` 的 `Patterns` 已支持 glob（默认 `**/*.md`）。**用户如需 watch 监听文档，直接在配置中追加 `**/*.pdf` 等模式即可**。本次**不新增** `watch_documents` 开关、不改 watch 默认行为，避免误监听二进制变化。

## Data Models

### ConvertResult（pkg/convert）

```go
type Result struct {
    Markdown string
    Title    string
    Warnings []string
}
```

> 说明：`Warnings` 为字符串摘要，源于 downmark `[]Warning`（含 Converter/Code/Location/Err），对外收敛为可读文本，避免泄漏内部结构。

### cmd_add 本地统计（不侵入 ImportResult）

`cmd_add` 在打印导入摘要时追加一行 `Converted (documents): %d`，由前置编排返回的 `convertedCount` 提供，**不改 `ImportResult` 结构**。

## Document Identity & Re-import Semantics（关键设计决策）

**问题**：`SmartImportSource` 的变更检测基于源文件 `ContentHash`。文档场景源文件是"转换产物 .md"，若 downmark 版本升级导致转换输出变化 → 全部已导入文档被判 changed → 全量重导入（数据风暴）。

**决策**：
1. **go.mod 精确锁定 downmark v0.10.0**（`go get` 精确版本，不浮动）
2. 变更检测的 `SourcePath` 使用**原文档路径**（而非临时 .md 路径），`ContentHash` 由既有管道基于 staging 中的 .md 计算 —— **downmark 转换是确定性的**，同一输入+同一版本输出恒定，故 hash 稳定
3. **downmark 升级 = 受控行为变更**：升级后已导入文档可能触发一次性重导入，属可接受（在 release notes 声明）
4. 升级时如要避免重导入，可先清空 `.metadata.json` 再升级

## Security & Safety

- **输入防护**：`os.Stat` 前置检查 `MaxInputBytes`（默认 64 MiB），超限即跳过
- **输出防护**：`downmark.ResultLimit` context 控制输出上限
- **超时**：转换经 `context.WithTimeout`（默认 60s），防大文档挂死
- **临时目录生命周期**：staging 与解压目录统一由 `defer cleanup` 清理，防泄漏
- **压缩包**：复用既有 `ExtractArchive` 的路径清洗与成员上限
- **扫描 PDF**：透传 downmark `errNoText`，明确提示，不静默导入空概念

## Error Handling

| 错误 | 触发条件 | 处理 |
|------|----------|------|
| `ErrUnsupportedFormat`（convert 定义） | 无法识别的扩展名（非归档/非文档/非 md） | `okf add` 报错并跳过，不影响其他文件 |
| downmark `errNoText`（透传，`errors.Is`） | 扫描版 PDF 无文本层 | 报错提示 OCR/替代文件 |
| downmark 结果超限错误（透传） | 输出超过 ResultLimit | 报错并跳过 |
| downmark 其他错误（透传） | 转换失败 | 携带原始错误，计入 FailedFiles |

## Dependencies

- `github.com/giraffesyo/downmark v0.10.0`（MIT，**精确锁定**）
  - 传递依赖：`excelize/v2`、`html-to-markdown/v2`、`mimetype`、`mscfb`、`golang.org/x/net`、`golang.org/x/text` 等（均纯 Go、宽松许可）
- Go 版本：`go 1.26.0` + `toolchain go1.26.7`

## 兼容与破坏性说明

- **Go 版本升级（1.22→1.26）**：存量用户需 Go 1.26 才能源码构建/`go install`；README/Release Notes 标注最低版本
- **`okf add <非md>` 行为变更**：原"静默跳过（No markdown files found）" → 新"自动转换导入"，属期望变更，需在 CLI 帮助与 README 声明
- **二进制体积 +~10MB**：记录；如体积敏感可改按需 import downmark 子包

## Open Questions（已关闭）

1. ~~`type` 默认值~~ → 定案 `source`
2. ~~watch 是否默认监听~~ → 不新增功能，用户自行扩展 `Patterns`
3. ~~转换产物命名~~ → 定案 `<原名>.md`
4. ~~错误类型体系~~ → 精简为 `ErrUnsupportedFormat` + downmark 错误透传
