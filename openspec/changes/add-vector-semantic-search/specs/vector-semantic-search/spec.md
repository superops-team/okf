# Specification: Vector Semantic Search

## Overview

本规范定义 okf 的本地语义检索能力：通过 `go:embed` 内嵌 ONNX Runtime + MiniLM 量化模型，纯 Go（`CGO_ENABLED=0`）、运行时零联网，将文本向量化并构建 HNSW 索引，为 `okf search` 提供自然语言/语义检索（`-semantic`），与既有子串/正则检索做 RRF 混合排序。既有 `pkg/okf` 核心模型与 `okf search` 默认行为不变。

## ADDED Requirements

### Requirement: 内嵌资源层（embed + 按平台解包）

系统 SHALL 提供 `internal/embeddings/assets`（或等价内部包），将当前平台的 ONNX Runtime 动态库、MiniLM int8 量化模型（`model_quantized.onnx`）与 tokenizer 通过 `//go:embed` 内嵌，并按平台 build tag 只编译匹配文件。

#### Scenario: 内嵌资源在构建期打包进二进制

- **WHEN** 执行 `CGO_ENABLED=0 go build ./...`
- **THEN** 构建 MUST 成功
- **AND** 产物二进制 MUST 包含 ORT 动态库 + 模型 + tokenizer 的内嵌数据（通过 `//go:embed` 变量可读）
- **AND** 产物 MUST NOT 依赖系统已安装的 ONNX Runtime 或任何外部二进制

#### Scenario: 按平台裁剪（按 OS 按需）

- **WHEN** 分别以 `linux/amd64`、`darwin/arm64`、`windows/amd64` 交叉编译
- **THEN** 每个产物 MUST 仅包含其对应平台的 ORT 动态库
- **AND** 未覆盖的平台组合 MUST 通过 `ort_unsupported.go`（`//go:build !linux && !darwin && !windows`）在编译期报出明确的"平台不支持"错误（文件内 `compile error` 触发 fail），而非未定义符号

#### Scenario: 运行时解包到缓存目录并校验

- **WHEN** 首次调用 embedding 初始化
- **THEN** 系统 MUST 将内嵌资源解包到 `os.UserCacheDir()/okf/{ort,models}/`
- **AND** 解包 MUST 原子写（临时文件 + rename），写盘后校验 SHA256 与内嵌清单一致
- **AND** 再次调用时若缓存文件存在且 SHA256 一致，MUST 复用缓存、跳过写盘
- **AND** 全过程 MUST NOT 发起任何网络请求（测试方法：临时 `OKF_ORT_DIR` + 网络不可达环境跑通，且断言不调用 `EnsureOnnxRuntimeSharedLibrary`/bootstrap 路径）
- **AND** 环境变量 `OKF_ORT_DIR` 可覆盖缓存根目录；显式 `SetSharedLibraryPath` 以内嵌/`OKF_ORT_DIR` 为准，不受外部 `ONNXRUNTIME_LIB_PATH` 影响（README 声明）

#### Scenario: 资源损坏的处置

- **WHEN** 缓存文件存在但 SHA256 与清单不一致
- **THEN** 系统 MUST 返回明确错误（含缓存路径与"删除缓存后重试/okf vector rebuild"建议）
- **AND** MUST NOT 用损坏资源继续初始化或静默跳过

### Requirement: Embedder 抽象与 MiniLM 默认实现

系统 SHALL 提供 `pkg/embeddings.Embedder` 接口（`EmbedQuery`/`EmbedDocuments`/`Dimension`/`Close`），并以内嵌 MiniLM 模型为默认实现（基于 `pure-onnx/embeddings/minilm`，mean pooling + L2 归一化，384 维）。该接口亦作为测试注入点（见"测试分层"Requirement）。

#### Scenario: 查询文本编码为 384 维向量

- **WHEN** 调用 `EmbedQuery("hello world")`
- **THEN** 返回的向量 MUST 长度为 384（`Dimension()==384`）
- **AND** 向量 MUST 为 L2 归一化后的 `[]float32`

#### Scenario: 批量文档编码

- **WHEN** 调用 `EmbedDocuments([]string{...})` 传入 N 条文本
- **THEN** 返回 `N` 个 384 维向量
- **AND** 超长文本 MUST 被 tokenizer 截断到模型最大序列长度（默认 256 token，右截断），MUST NOT panic

#### Scenario: Embedder 实例生命周期与并发安全

- **WHEN** 多个 goroutine 同时调用同一 `Embedder` 的 `EmbedQuery`
- **THEN** 结果 MUST 正确（内部串行化），经 `go test -race` 无数据竞争
- **AND** `Close()` 后再次调用 MUST 返回错误，不 panic

### Requirement: 向量索引层（HNSW + 持久化）

系统 SHALL 提供 `pkg/vectorindex`，基于 `coder/hnsw`（CC0-1.0）构建、更新、搜索概念向量，并持久化到 `.okf/vector/`。

#### Scenario: 构建与搜索

- **WHEN** 对一组概念构建索引（`okf vector index`）后，用语义向量查询
- **THEN** 返回按余弦距离排序的 topK 概念，`topK` 默认 10
- **AND** 每个命中可回溯到对应 Concept（filepath/type/title）

#### Scenario: 持久化与加载

- **WHEN** 索引保存为 `.okf/vector/index.bin` + `index.meta.json` 后重新加载
- **THEN** 加载 MUST 成功且搜索结果与保存前一致
- **AND** 元信息（维度/模型/版本）不匹配时 MUST 提示 `okf vector rebuild`

#### Scenario: 概念级粒度与指纹去重

- **WHEN** 同一概念（指纹 = `lower(type) + ":" + lower(title) + ":" + 绝对化且规范化分隔符的 filepath`）重复入图
- **THEN** 图内 MUST 只保留一份（Add 覆盖语义）
- **AND** 每概念一个向量，编码文本 = title+description+content 拼接，由 tokenizer 截断至 256 token

#### Scenario: 增量与重建语义（P0）

- **WHEN** 索引已存在再次执行 `okf vector index`
- **THEN** P0 语义 MUST 为：追加新指纹向量 + 已存在指纹跳过（幂等）
- **AND** 概念删除/内容变更在 P0 MUST 通过 `okf vector rebuild` 全量重建（`index.meta.json` 记录指纹快照便于校验）；变更检测增量留待 P1（后续变更）

### Requirement: 混合检索（RRF 融合）

系统 SHALL 在 `pkg/query` 提供 `SemanticSearch(bundle, query, opts)`，将语义召回（HNSW topK）与既有词法召回（子串/正则）经 RRF 融合后返回统一结果。

#### Scenario: RRF 融合排序

- **WHEN** 对同时被语义与词法命中的查询执行 `SemanticSearch`
- **THEN** 结果按 `score = Σ 1/(60+rank)` 降序排列
- **AND** 每条结果 MUST 标注来源（`semantic`/`lexical`/`both`）
- **AND** 双通道命中的概念 MUST 排在单通道命中的概念之前（同 rank 情形）

#### Scenario: 结果类型向后兼容

- **WHEN** 扩展 `query.SearchResult` 以承载语义结果
- **THEN** 新增字段 MUST 为可选（`Source string`、`SemanticScore float32`），既有调用方（`cmdSearch`、MCP `okf_search`）不传该字段时行为 MUST 与现状一致
- **AND** 扩展后 `go build ./...` 与既有测试 MUST 全绿

#### Scenario: 索引未构建的处置

- **WHEN** 执行 `SemanticSearch`/`okf search -semantic` 但索引尚未构建
- **THEN** 系统 MUST 返回明确错误（含处置建议 `okf vector index`）
- **AND** MUST NOT panic、MUST NOT 静默返回空结果

#### Scenario: 语义通道初始化/检索失败的处置

- **WHEN** 索引已构建但语义初始化失败（如 ORT 库损坏）或检索运行时报错
- **THEN** 系统 MUST 打印 warning（含原因），并回退输出纯词法结果
- **AND** `-verbose` 时 MUST 输出失败详情与回退说明；该回退 MUST NOT 被视为静默降级（warning 显式可见）

### Requirement: CLI 集成（vector 子命令组 + search -semantic）

系统 SHALL 提供 `okf vector index|status|rebuild` 子命令，并为 `okf search` 增加 `-semantic` 布尔 flag；`-q` 沿用既有参数且必填；未显式使用 `-semantic` 时行为与既有版本一致。

#### Scenario: okf vector status

- **WHEN** 执行 `okf vector status`
- **THEN** 输出索引存在性、条目数、向量维度、模型与平台、缓存命中路径
- **AND** 未构建索引时明确提示"尚未构建索引"

#### Scenario: okf vector index 与 rebuild

- **WHEN** 执行 `okf vector index`（首次）
- **THEN** 解包资源、编码全部概念并构建 HNSW 索引，输出条目数与耗时
- **AND** 再次执行走 P0 增量语义（追加+去重）；`okf vector rebuild` 删除并全量重建

#### Scenario: okf search -semantic

- **WHEN** 执行 `okf search -q "查询文本" -semantic`（索引已构建）
- **THEN** 输出 RRF 融合后的 topK 结果（默认 10），含来源标注
- **AND** 未构建索引时提示 `okf vector index` 先行
- **AND** 不传 `-semantic` 时输出与既有版本完全一致的词法结果

### Requirement: 测试分层（mock 优先，真实模型隔离）

系统 SHALL 通过 `Embedder` 接口注入使单测不依赖真实模型/ORT；真实模型与 ORT 库仅在集成测试与 gauntlet L10 冒烟中使用。

#### Scenario: 单测不依赖真实模型

- **WHEN** 运行 `go test ./pkg/...`（默认路径）
- **THEN** MUST 通过注入的 fake/mock `Embedder`（确定性向量）完成 `SemanticSearch`、索引、CLI 逻辑测试
- **AND** MUST NOT 下载模型、MUST NOT 依赖 ORT 动态库或网络

#### Scenario: 真实模型冒烟（集成）

- **WHEN** 运行 `go test -tags integration` 或 gauntlet L10（CI 预置 ORT/模型资源）
- **THEN** 端到端验证：解包→初始化→`EmbedQuery` 384 维→建索引→`search -semantic` 返回结果
- **AND** 模型/库资源在 CI 中预下载并校验 SHA256

### Requirement: 依赖锁定、版本规划与许可声明

系统 SHALL 精确锁定新增依赖，规划版本变更，并在 README 声明模型/库来源、许可与运行时动态加载行为。

#### Scenario: 依赖精确锁定

- **WHEN** 在 go.mod 引入 `pure-onnx` 与 `coder/hnsw`
- **THEN** 版本 MUST 精确锁定（非浮动），并有测试断言（沿用 `TestDependencyPinned` 模式）
- **AND** 依赖及传递依赖 MUST 全部为宽松许可（MIT/CC0-1.0/Apache-2.0），无 CGO

#### Scenario: 功能变更版本规划

- **WHEN** 本次功能变更合入并发布
- **THEN** 版本号 MUST 递增 minor（0.3.x → **0.4.0**），因属新功能而非修复
- **AND** `const Version`（`cmd/okf/main.go`）与 README 版本声明同步更新

#### Scenario: 文档声明

- **WHEN** 检查 README（含 README.zh-CN.md）
- **THEN** MUST 包含：语义搜索用法（`vector index` + `search -q ... -semantic`）、离线与体积说明、MiniLM 256 token 与中文语义限制、模型/库许可与来源、运行时动态加载声明、`OKF_ORT_DIR` 与 `ONNXRUNTIME_LIB_PATH` 交互说明

### Requirement: 质量与约束栈

系统 SHALL 通过完整回归与约束栈后合入。

#### Scenario: 全量约束栈通过

- **WHEN** 执行 `tools/gauntlet.sh`（build/vet/gofmt/staticcheck/test -race/覆盖≥60%/shuffle/变异/真实执行）
- **THEN** 全部阶段 MUST 通过（fail-on-first 语义）
- **AND** `CGO_ENABLED=0 go build ./...` 单独验证通过
- **AND** gauntlet L10 真实执行 MUST 包含语义搜索冒烟（建索引 + `search -semantic`）
