# Proposal: Vector Semantic Search (本地向量化 + 自然语言搜索)

## Summary

为 okf 知识库增加**本地语义检索**能力：通过 `go:embed` 将 ONNX Runtime 动态库与 MiniLM 量化模型**内嵌进 okf 二进制**，运行时解包到缓存目录后加载，实现完全离线、零联网、无 CGO 的文本向量化；配合纯 Go HNSW 近似近邻索引（`github.com/coder/hnsw`，CC0-1.0），为 `okf search` 增加自然语言/语义检索（`--semantic`），并将语义召回与既有子串/正则召回做混合排序。二进制单文件分发，**运行时无需安装任何外部运行时或模型**。

## Motivation

- **补齐语义检索能力**：`pkg/query` 现有 `Search`/`SearchWithMatches` 仅支持子串（`strings.Contains`）与正则匹配，无法理解"概念等价"（如查 "efficient indexing" 命中不到 "fast lookup structure"）。与主流知识库工具（OpenKB、Logseq 向量检索等）相比，语义检索是主要功能差距
- **本地化与隐私**：知识库内容常含未公开代码/文档，远程 embedding API 存在数据外泄与联网依赖；本地推理确保内容不出机器
- **离线可用**：用户明确否决"首次联网 bootstrap 下载"；采用内嵌静态资源，运行时零联网
- **纯 Go / 无 CGO**：延续 okf「纯 Go、不依赖 Python/CGO/外部二进制」约束栈；编译产物单一自包含（`CGO_ENABLED=0`）

## 已核实的依赖事实（引入前证据，避免凭印象）

| 依赖 | 用途 | 许可 | 关键事实 |
|---|---|---|---|
| `github.com/amikos-tech/pure-onnx` | purego 绑定 ONNX Runtime，无 CGO | MIT | `ort.SetSharedLibraryPath` + `ort.InitializeEnvironment`（显式路径模式）；`embeddings/minilm` 提供 `NewEmbedder/EmbedQuery/EmbedDocuments`；跨 Linux/macOS/Windows；`CGO_ENABLED=0` 已由外部 spike 验证（Go 1.26/macOS arm64） |
| `github.com/coder/hnsw` | 纯 Go HNSW 近似近邻 | CC0-1.0 | `NewGraph/Add/Search/Delete`、`SavedGraph` 文件持久化、`CosineDistance`；纯 Go 零 CGO；已有生产项目（floop）用作 tiered ANN 索引 |
| ONNX Runtime CPU 库 | 推理引擎 | MIT | 官方 release 各平台 CPU 动态库约 7–20 MB（linux Release 约 7.6 MB、osx-arm64 约 13.7 MB） |
| MiniLM-L6-v2 ONNX | 文本向量模型（384 维） | Apache-2.0（模型） | `model.onnx` fp32 约 90 MB；`model_quantized.onnx` int8 约 25 MB（`sentence-transformers/all-MiniLM-L6-v2`） |

## Requirements

### MUST

- 引入 `pure-onnx`（MIT）与 `coder/hnsw`（CC0-1.0），全部依赖为宽松许可、无 CGO
- 提供 `//go:embed` 静态资源层：内嵌**当前平台**的 ORT 动态库 + MiniLM **int8 量化**模型（`model_quantized.onnx`）与 tokenizer；按 `GOOS`/`GOARCH` build tag 只编译对应平台文件，实现"按 OS 按需"
- 运行时将内嵌资源**解包到用户缓存目录**（`os.UserCacheDir()/okf/ort/` 与 `.../okf/models/`），SHA256 校验一致则复用缓存，**全程不联网**
- 显式路径模式初始化 ORT（`ort.SetSharedLibraryPath` + `ort.InitializeEnvironment`）；**不使用** `InitializeEnvironmentWithBootstrap`（bootstrap 属被否决的联网路径）
- 提供 `Embedder` 抽象接口（`EmbedQuery`/`EmbedDocuments`/`Dimension`/`Close`），MiniLM 为默认实现，为后续中文模型（BGE-M3 等）/远程 API 预留替换点
- 提供向量索引层（封装 `coder/hnsw`），支持构建/增量更新/持久化（`.okf/vector/index.bin`）
- CLI 集成：`okf vector index|status|rebuild` 子命令组 + `okf search -q "<query>" -semantic` 开关（`-q` 与既有搜索一致，`-semantic` 为布尔 flag）
- 混合检索：语义召回（HNSW topK）与既有词法召回（子串/正则）经 RRF 融合后输出统一结果；`--semantic` 显式开关，默认仍走既有检索路径（**不改变现有命令默认行为**）
- 错误处理明确：未建索引 / ORT 初始化失败 / 平台不支持 / 资源损坏，均返回可读错误与处置建议，不 panic、不静默降级
- 全链路 `CGO_ENABLED=0 go build ./...` 通过；`tools/gauntlet.sh` 全绿

### SHOULD

- MCP 服务器暴露语义搜索工具（`okf_semantic_search`），复用同一检索核心
- 索引支持增量更新（`okf sync`/`add` 后自动或按需刷新），避免全量重建
- README 增加语义搜索用法、离线/体积说明、模型与库的许可与来源标注（含运行时动态加载声明）

### 明确不做（Scope 外）

- **不引入远程 API 作为默认路径**；仅通过 `Embedder` 接口预留（后续变更另行设计）
- **不引入 chunk 级检索**：P0 为概念级（每概念一个向量，MiniLM 默认 256 token 截断）；chunk 化作为后续变更
- **不修改** `pkg/okf` 核心模型（Concept/Bundle）与 OKF v0.2 规范；检索能力以独立包 + 前置接线方式接入
- **不引入** Python/CGO/外部二进制；ORT 库以内嵌资源随二进制分发
