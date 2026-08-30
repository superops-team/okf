# Design: Vector Semantic Search

## Overview

在 okf **既有检索能力之外**新增一条语义检索通道：内嵌 ONNX Runtime + MiniLM 量化模型，纯 Go、零 CGO、运行时零联网；HNSW 索引提供近似近邻召回；语义与词法结果经 RRF 融合。**检索核心模型（`pkg/okf` Concept/Bundle）与 OKF v0.2 规范零改动**；`pkg/query` 既有 `Search`/`SearchWithMatches` 保持默认行为不变，语义搜索作为显式开关（`--semantic`）接入。

## 核心原则（最小实现）

- **内嵌资源即"自带运行时"**：ORT 动态库 + 模型 + tokenizer 全部 `go:embed` 进二进制，按平台 build tag 裁剪，产物单文件自包含
- **解包到缓存、SHA256 复用**：运行时把内嵌 bytes 写盘到用户缓存目录，校验一致则跳过，避免每次解包（资源共约 35–40 MB）
- **接口隔离、实现可换**：`Embedder` 接口解耦模型实现（MiniLM 默认 → 后续 BGE-M3 / 远程 API）
- **不侵入主链路**：不改 `CollectFiles`/`SmartImportSource`/`ImportResult`/`pkg/okf`；新增独立包 `pkg/embeddings`、`pkg/vectorindex`，检索接线收敛到 `pkg/query` + `cmd/okf` + `pkg/mcp`
- **默认行为不变**：未显式 `--semantic` 时 `okf search` 走既有词法路径

## Architecture

### 包结构与数据流

```
                         ┌────────────────────────────────────────────┐
   okf vector index ──▶  │ pkg/embeddings/assets (go:embed + 解包)      │
                         │  · ortembed/: 按平台 embed ORT 库           │
                         │  · model/:  embed model_quantized.onnx      │
                         │            + tokenizer.json                 │
                         │  · 解包→ ~/.cache/okf/{ort,models}/ + SHA256│
                         └──────────────────┬─────────────────────────┘
                                            ▼
                         pkg/embeddings · Embedder 接口
                                            │  (pure-onnx embeddings/minilm)
                                            ▼
                         pkg/vectorindex · HNSW 封装（coder/hnsw）
                                            │  .okf/vector/index.bin 持久化
                                            ▼
                         pkg/query · SemanticSearch（RRF 融合语义+词法）
                                            │
              ┌─────────────────────────────┴──────────────────────────┐
              ▼                                                        ▼
        cmd/okf search --semantic                               pkg/mcp okf_semantic_search
```

### 关键决策（定案）

| # | 决策点 | 定案 | 理由 |
|---|---|---|---|
| D1 | 内嵌范围 | ORT CPU 库 + MiniLM **int8** 模型（约 25 MB）+ tokenizer | 与用户确认：int8 对检索场景可忽略精度损失，二进制增量约 35–40 MB |
| D2 | 资源获取 | **CI 构建时**从官方 release / HuggingFace 拉取一次写入 embed 目录（构建期联网）；git 不存大文件 | `go:embed` 要求文件编译时在 module 内；产物运行时零联网 |
| D3 | ORT 初始化 | 显式路径：`ort.SetSharedLibraryPath(解包路径)` + `ort.InitializeEnvironment()` | 用户否决 bootstrap 联网下载 |
| D4 | 索引粒度 | 概念级：每概念一个向量（title+description+content 拼接文本，由 tokenizer 截断至模型最大序列长度 256 token） | P0 最小实现；chunk 化留后续变更 |
| D5 | 融合策略 | RRF（Reciprocal Rank Fusion）：`score = Σ 1/(k+rank)`，k=60 | 无标定参数、对两通道结果公平、可解释 |
| D6 | 并发模型 | `Embedder` 串行化（内部 `sync.Mutex`）；HNSW 读多写少，写操作加锁、读走快照 | 避免 pure-onnx session 并发未定义行为；索引一致性优先 |
| D7 | 平台裁剪 | `internal/embeddings/assets/` 下 `ort_<goos>_<goarch>.go` + build tag | 构建某平台仅编译对应资源文件 |
| D8 | 失败语义 | 索引未构建→明确报错（含处置建议）；语义初始化/检索失败→打印 warning 并回退纯词法结果（`-verbose` 输出原因，不静默） | 维度 9 风险预判 |

### 内嵌资源目录组织（按平台按需）

```
internal/embeddings/assets/
├── ort_linux_amd64.go    //go:build linux && amd64   //go:embed libs/linux/amd64/libonnxruntime.so
├── ort_darwin_arm64.go   //go:build darwin && arm64  //go:embed libs/darwin/arm64/libonnxruntime.dylib
├── ort_darwin_amd64.go   //go:build darwin && amd64
├── ort_windows_amd64.go  //go:build windows && amd64 //go:embed libs/windows/amd64/onnxruntime.dll
├── model.go              //go:embed models/model_quantized.onnx
├── tokenizer.go          //go:embed models/tokenizer.json
└── sha256.go             // 内嵌资源 SHA256 清单（常量）
```

`go build` 某平台时仅编译匹配的 `ort_*.go`，二进制只含该平台库——实现"按 OS 按需"。未覆盖的平台组合（如 `linux/arm64`）编译时明确报错（build tag 无匹配文件），文档声明支持矩阵。

### 解包与缓存（核心流程）

```
首次调用:
  ensureAssets()
    1. dir = os.UserCacheDir()/okf/{ort,models}
    2. 对每个内嵌资源：目标文件存在 且 SHA256==清单值 → 复用（跳过）
       否则 WriteFile(0644)（原子写：临时文件 + rename），并校验
    3. 返回 ort 库路径、model 路径、tokenizer 路径
之后调用:
    命中缓存，跳过写盘
```

- 环境变量覆盖：`OKF_ORT_DIR`（默认 `os.UserCacheDir()/okf`）便于离线预置与测试隔离
- 原子写 + `defer` 清理临时文件（维度 9 临时目录清理）
- 资源损坏（SHA256 不匹配）→ 报错并提示删除缓存目录后重试，或 `okf vector rebuild`

### Embedder 接口

```go
// pkg/embeddings/embeddings.go
type Embedder interface {
    EmbedQuery(text string) ([]float32, error)      // 单个查询向量
    EmbedDocuments(texts []string) ([][]float32, error) // 批量文档向量
    Dimension() int                                  // 特征维（MiniLM=384）
    Close() error
}
```

默认实现 `minilm.NewEmbedder(modelPath, tokPath, WithMeanPooling(), WithL2Normalization())`（pure-onnx `embeddings/minilm`；默认 seq 256，纯 Go tokenizer 由 pure-tokenizers 提供）。后续 BGE-M3（1024 维，中文强，模型约 1 GB+，只能模型外置或远程 API）通过实现同一接口接入，调用方零改动。

### 向量索引层（pkg/vectorindex）

- 封装 `coder/hnsw`：`VectorIndex` 接口 `Add/Remove/Search/Save/Load/Len`
- 节点 Key：概念指纹（`type:title:filepath` 归一化字符串），避免重复入图
- 持久化：`.okf/vector/index.bin`（`hnsw.SavedGraph` 二进制编码）；维度/版本头写入元信息文件 `index.meta.json`（`dim`、`model`、`okf_version`），加载时不匹配即提示重建
- 增量：`Add` 后追加；`okf vector rebuild` 全量重建；`okf sync` 检测变更后触发增量（P1）

### 混合检索（pkg/query）

```
SemanticSearch(bundle, query, opts)
  1. 词法召回: 既有 SearchWithMatches (子串/正则) → topK_lex (默认 20)
  2. 语义召回: EmbedQuery(query) → HNSW.Search(vec, topK_sem=20) → 映射回 Concept
  3. RRF 融合: score(c) = Σ_{r ∈ 出现该概念的结果集} 1/(k + rank(c)), k=60
  4. 按 score 降序返回 []SearchResult（含 score、来源标注 semantic|lexical|both）
```

- 来源标注让结果可解释、可调试
- `--semantic` 未建索引 → 明确提示 `okf vector index` 先行；语义通道失败（如 ORT 初始化失败）→ 提示原因并**可回退到纯词法结果**（打印 warning，不静默）

### CLI 接线

- `okf vector index [-force]`：构建/增量更新索引（首次需解包资源）
- `okf vector status`：索引存在性、条目数、模型/维/平台、缓存命中
- `okf vector rebuild`：删除并重建
- `okf search -q "<q>" -semantic [-k N]`：混合检索，默认 `-k 10`；`-q` 语义与既有搜索一致
- `-verbose` 时输出语义/词法各通道命中与融合分数

### 并发与锁

- `Embedder` 实例全局单例 + `sync.Mutex` 串行化推理（避免并发 session 未定义行为）
- `VectorIndex` 读（Search）用 RLock 快照；写（Add/Remove）用 Lock；`pkg/query.SemanticSearch` 委托索引层读锁，**不得**在已加锁方法内叠加锁（沿用 `FilterXxx` 委托 `FilterConcepts` 的既有约定）
- 并发改动必须经 `go test -race` 验证

### 依赖与许可

- `github.com/amikos-tech/pure-onnx`（MIT）+ 传递 `pure-tokenizers`（MIT）：纯 Go，`CGO_ENABLED=0` 编译
- `github.com/coder/hnsw`（CC0-1.0）：纯 Go
- ORT CPU 库（MIT）：随官方 release 分发；MiniLM 模型（Apache-2.0）
- 引入前在 go.mod **精确锁定**版本（非浮动）；README 标注模型/库来源与许可、运行时动态加载声明

## 与 AGENTS.md 约束的关系（如实声明）

- **满足**「纯 Go 编译、无 CGO」：`CGO_ENABLED=0 go build ./...` 全绿（pure-onnx 为 purego 实现，外部 spike 已验证）
- **动态加载声明**：运行时通过 `dlopen` 解包加载 ORT 动态库（`SetSharedLibraryPath`），**非静态链接**；虽内嵌自包含，仍需在发布文档如实说明「单文件二进制，运行时解包内置 ORT 动态库到缓存」
- **性能基线**：MiniLM int8 CPU 单查询 <10 ms 量级，建索引约数百 chunks/s（估算，TDD 阶段以 benchmark 实测为准）

## 风险与缓解

| 风险 | 缓解 |
|---|---|
| pure-onnx v0.0.1 尚在演进（文档标注 "in progress"） | TDD 首步即 spike 最小链路（解包→初始化→单次 embedding golden），失败即暴露；接口仅依赖 `ort`/`minilm` 稳定面 |
| `go:embed` 大文件进 git | CI 构建时下载进 embed 目录，git 不存大文件；`.gitignore` 排除 `internal/embeddings/assets/libs|models/` 下的大二进制（保留清单文件） |
| macOS 动态库签名/quarantine | 解包后以 0644 写盘即可 `dlopen`；CI 产物如发布需 ad-hoc 签名（记录为已知项） |
| MiniLM 256 token 截断、中文语义弱 | README 明示限制；`Embedder` 接口预留 BGE-M3 升级路径；中文暂由词法通道兜底 |
| HNSW 更新/删除限制（上游 rebuild workaround） | 概念级索引以「全量重建为主、增量追加为辅」；删除触发 `rebuild` 标记，`okf sync` 时统一重建 |
