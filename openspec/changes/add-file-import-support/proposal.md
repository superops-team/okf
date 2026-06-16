## Why

当前 OKF 知识库主要通过 `okf init` 从 Git 仓库生成，缺少从任意文件、文件夹和压缩包导入知识的能力。用户需要：

1. **灵活的知识库路径配置**：不同操作系统应有不同默认路径，同时支持用户自定义
2. **多源知识导入**：从任意文件、文件夹导入知识，而不仅仅是 Git 仓库
3. **压缩包自动提取**：支持 ZIP/TAR 等压缩格式，自动解压并索引其中的知识文件

这些能力使 OKF 能作为通用知识库管理工具，支持手动添加各类文档、导入现有知识库备份、迁移知识等场景。

## What Changes

- **新增知识库路径配置**：定义系统级默认路径（Linux: `~/.okf/knowledge`, macOS: `~/Library/Application Support/okf/knowledge`, Windows: `%APPDATA%\okf\knowledge`），支持通过 CLI 参数和环境变量覆盖
- **新增文件导入能力**：`okf add` 命令支持添加单个文件或整个文件夹
- **新增压缩包提取能力**：自动识别 ZIP/TAR(.gz/.bz2) 压缩包，提取后索引内部知识文件
- **新增知识库管理能力**：`okf config` 命令管理默认知识库路径

## Capabilities

### New Capabilities

- `knowledge-path-configuration`: 支持多平台默认路径、CLI 参数、环境变量三层配置
- `file-import`: 从文件系统导入单个文件或文件夹作为知识库内容
- `archive-extraction`: 自动识别并提取 ZIP/TAR 压缩包中的知识文件
- `knowledge-base-management`: 配置管理、路径查询、状态查看

### Modified Capabilities

- `knowledge-bundle`: 支持从非 Git 源加载和保存知识包

## Impact

- 影响 `pkg/okf`：新增配置管理、路径解析、文件导入逻辑
- 影响 `cmd/okf`：新增 `add`、`config` 子命令
- 新增内部模型：`Config`、`ConfigManager`
- 不新增外部依赖（使用标准库处理 ZIP/TAR）