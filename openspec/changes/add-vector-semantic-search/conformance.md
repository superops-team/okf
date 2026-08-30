# Conformance: spec ↔ Implementation ↔ Tests

> 依据 OKF 项目 AGENTS.md 维度 19：落地后逐条核对 spec Scenario 与实现/测试对齐度。
> 对齐度：**fully**（实现+测试均满足）/ aligned（实现满足、测试部分覆盖）/ partial（部分满足）/ gap（未实现）。
> 本报告基于实现完成后全量回归实测结果（go build/vet/test + test_mcp.py 全绿 + 四平台交叉编译）。

## 汇总

- 24 / 24 场景有实现与测试/实测记录
- 对齐度：**fully 22，aligned 2，partial 0，gap 0**
- 未对齐项：S14 / S15 为 CLI 层行为（依赖真实模型，无单测），以端到端实测 + gauntlet L10 覆盖
- 依赖锁定：pure-onnx **v0.0.1**（MIT）、coder/hnsw **v0.6.1**（CC0-1.0）精确锁定（`TestVectorDepsPinned` 断言）；renameio 本地 fork replace（修复 Windows 编译）
- 版本规划：0.3.x → **0.4.0**（`cmd/okf/main.go` + `pkg/mcp/server.go` 同步）

## 逐条对照

| # | Scenario | 实现位置 | 测试用例 | 实测 | 对齐 |
|---|----------|----------|----------|------|------|
| S1 | 内嵌资源构建期打包进二进制 | `internal/embeddings/assets`（`//go:embed` + build tag 文件） | `CGO_ENABLED=0 go build ./...`（L1）+ 四平台交叉编译 | PASS | fully |
| S2 | 按平台裁剪（按 OS 按需） | `ort_{linux_amd64,linux_arm64,darwin_amd64,darwin_arm64,windows_amd64}.go` + `ort_unsupported.go` | 手动交叉编译 `GOOS=linux/darwin/windows × GOARCH=amd64/arm64` | 全 PASS | fully |
| S3 | 运行时解包到缓存目录并校验 | `assets.Ensure`（原子写 + SHA256 + 复用 + `OKF_ORT_DIR` 覆盖） | `TestEnsureExtractsToCache` / `TestEnsureReusesCacheWithoutRewrite` / `TestDefaultDirEnvOverride` | PASS | fully |
| S4 | 资源损坏的处置 | `assets.Ensure`（SHA256 不一致 → 明确错误含缓存路径与建议） | `TestEnsureRewritesCorruptedFile` | PASS | fully |
| S5 | 查询文本编码为 384 维 L2 向量 | `pkg/embeddings` MiniLM（`minilm.NewEmbedder`，mean pooling + L2） | `TestIntegrationMiniLMNewAndBatch`（-tags integration） | PASS | fully |
| S6 | 批量文档编码 + 256 token 截断 | `EmbedDocuments`（tokenizer 右截断，默认 seq 256） | `TestIntegrationMiniLMNewAndBatch` | PASS | fully |
| S7 | Embedder 生命周期与并发安全 | ORT 环境 `sync.Once` 单例 + 推理 `sync.Mutex` 串行化 | 集成并发测试 + `go test -race` | PASS | fully |
| S8 | 构建与搜索（HNSW topK） | `pkg/vectorindex`（EfSearch=64、M=16，显式余弦降序重排） | `TestAddSearchOrder` / `TestSemanticSearchTopK` | PASS | fully |
| S9 | 持久化与加载（含 meta 不匹配提示 rebuild） | `index.bin`（hnsw Export/Import）+ `index.meta.json` | `TestSaveLoadRoundTrip` / `TestSaveLoadPersistedFiles` / `TestLoadMetaMismatch` | PASS | fully |
| S10 | 概念级粒度与指纹去重 | `query.Fingerprint` + Add 幂等（Lookup 存在则跳过） | `TestAddSameKeyIsIdempotent` / `TestFingerprintNormalization` | PASS | fully |
| S11 | 增量与重建语义（P0） | `cmdVectorIndex`（增量 append+去重）/ `cmdVectorRebuild`（删旧全量重建） | 端到端实测（二次 index 幂等 + rebuild 全量）+ gauntlet L10 | PASS | fully |
| S12 | RRF 融合排序 + 来源标注 | `pkg/query.SemanticSearch`（k=60，`score=Σ1/(60+rank)`，Source 标注） | `TestSemanticSearchRRFAndSource` | PASS | fully |
| S13 | 结果类型向后兼容 | `SearchResult` 扩展可选 `Source`/`SemanticScore`；既有调用方不传字段行为不变 | 既有 cmdSearch/MCP 测试全绿 | PASS | fully |
| S14 | 索引未构建的处置 | `runSemanticSearch` 返回明确错误（提示 `okf vector index`） | 端到端实测（未建索引报错） | PASS | aligned |
| S15 | 语义失败回退（warning 不静默） | `cmdSearch` warning + 回退词法；`-verbose` 输出详情 | 端到端实测（回退正常）+ `TestSemanticSearchNilBackendIsLexicalOnly` / `TestSemanticSearchEmbedError` | PASS | aligned |
| S16 | okf vector status | `cmd_vector.go`（存在性/条目数/维度/模型/平台/缓存路径；未构建提示） | 端到端实测 | PASS | fully |
| S17 | okf vector index 与 rebuild | `cmd_vector.go`（首次建索引 + P0 增量 + rebuild 全量） | 端到端实测（7 概念索引 161ms）+ gauntlet L10 | PASS | fully |
| S18 | okf search -semantic（含来源标注） | `cmdSearch` `-semantic`/`-k` flag；MCP `okf_semantic_search` | CLI 端到端（OKF Lint Rules 命中）+ `test_mcp.py` Test 7.5 + gauntlet L10 | PASS | fully |
| S19 | 单测不依赖真实模型 | `Embedder` 接口注入 fake/mock；确定性向量 | `semantic_test.go` fakeBackend / `vectorindex_test.go` 确定性向量；`go test ./...` 默认路径全绿（无模型资源） | PASS | fully |
| S20 | 真实模型冒烟（集成） | `-tags integration` 测试 + gauntlet L10 语义冒烟 | `TestIntegrationInt8Embedding` / `TestIntegrationMiniLMNewAndBatch` / gauntlet L10 | PASS | fully |
| S21 | 依赖精确锁定 + 许可 + 无 CGO | `go.mod` 精确版本 + renameio 本地 fork replace | `TestVectorDepsPinned`（pure-onnx/coder-hnsw pin + renameio replace 断言） | PASS | fully |
| S22 | 功能变更版本规划（0.3.x → 0.4.0） | `cmd/okf/main.go` Version + `pkg/mcp/server.go` serverInfo | 构建 + `test_mcp.py` initialize 断言 | PASS | fully |
| S23 | 文档声明 | `README.md` + `README.zh-CN.md`「Semantic Search / 语义搜索」章节 | 人工核对（用法/离线体积/256 token/中文限制/许可来源/动态加载/OKF_ORT_DIR） | PASS | fully |
| S24 | 全量约束栈通过 | `tools/gauntlet.sh`（L1-L10 + 语义冒烟） | 全量回归 | PASS | fully |

## 实现要点记录（与 design 的一致性）

1. **内嵌资源**：`internal/embeddings/assets/` 按平台 build tag 编译，资源在 CI/构建期经 `scripts/fetch-ort.sh` / `scripts/fetch-model.sh` 下载（gitignore 排除，不入库），运行时经 `assets.Ensure()` 解包到缓存并 SHA256 校验——运行时零联网（S1-S4）✓
2. **交叉平台修正**：上游 `renameio v1.0.1` 的 tempfile.go 带 `!windows` build tag，导致 `coder/hnsw` 在 Windows 编译失败；以最小本地 fork（`internal/third_party/renameio`，标准库实现、去掉 build tag）`replace` 解决，四平台交叉编译全绿（S2）✓
3. **coder/hnsw 缺陷规避**：重复 Add panic → 幂等跳过；Delete 破坏图层 → tombstone 过滤不物理删除；Search 不保证排序 → 封装层显式余弦降序重排（S8-S10）✓
4. **指纹稳定性**：`Fingerprint` 使用归一化相对路径（不做 `filepath.Abs`），避免依赖进程 cwd 导致 CLI 与 MCP 索引 key 不一致（实测修复了 MCP 语义检索 0 结果问题，S10/S18）✓
5. **失败语义定案**：索引未建 → 报错提示 `okf vector index`；语义初始化/检索失败 → warning + 回退词法（不静默），`-verbose` 输出详情（S14-S15）✓
6. **版本与文档**：minor 递增 0.4.0（CLI + MCP server 同步）；README 双语声明运行时动态加载、许可来源、OKF_ORT_DIR（S22-S23）✓

## 遗留说明

- S14/S15 标注 aligned：CLI 层索引未建/语义失败路径依赖真实模型，无单测（`pkg/query` 层已有 `TestSemanticSearchNilBackendIsLexicalOnly`/`TestSemanticSearchEmbedError` 覆盖核心逻辑），CLI 行为经端到端实测与 gauntlet L10 冒烟验证
- `internal/embeddings/assets/libs|models/` 资源文件被 `.gitignore` 排除，CI 需先执行两个 fetch 脚本；本地已预置 8 个资源（4 平台 ORT + arm64/x86 模型 + tokenizer）
- darwin/amd64 平台 ORT 官方自 1.24.1 起不再发布 x86_64 包，脚本固定回退 1.23.1（README 已声明）
- `okf-bin` 为本地构建产物（.gitignore，供 `test_mcp.py` 使用）

## 已知限制（非 spec 缺口）

1. **MiniLM 英文语义为主**：中文语义质量有限（词法检索仍可用）；`Embedder` 接口为 BGE-M3/远程 API 预留升级路径（README 已声明）
2. **P0 增量语义**：概念删除/内容变更需 `okf vector rebuild` 全量重建；变更检测增量（P1）留待后续变更
3. **模型体积**：单平台内嵌约 33-38 MB（ORT ~10-15MB + 模型 ~23MB）；构建产物按平台裁剪，仅含当前平台资源
