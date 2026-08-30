# Tasks: Vector Semantic Search

> P0–P4 仅表依赖顺序，**全部完成才算完成**（AGENTS.md 维度 18）；每个 P 级完成后需对应测试绿 + spec 覆盖矩阵打勾。

## P0 — 依赖引入与链路 spike（TDD red 起步）

- [ ] 引入 `github.com/amikos-tech/pure-onnx`（MIT）与 `github.com/coder/hnsw`（CC0-1.0），go.mod 精确锁定；`TestDependencyPinned` 断言
- [ ] 本地最小 spike：临时脚本验证「解包 ORT → `SetSharedLibraryPath` → `InitializeEnvironment` → `minilm.NewEmbedder` → `EmbedQuery` 得 384 维」在 `CGO_ENABLED=0` 下可跑通（失败即暴露，不进入下一步）
- [ ] 确认各平台 ORT 库获取脚本（`scripts/fetch-ort.sh`：从官方 release 下载对应平台 Release 包，校验 SHA256，写入 embed 目录）
- [ ] MiniLM 模型与 tokenizer 下载脚本（`scripts/fetch-model.sh`：HuggingFace `sentence-transformers/all-MiniLM-L6-v2`，int8 `model_quantized.onnx`，记录 SHA256）
- [ ] 覆盖 spec：S1.1（构建期打包）、S7.1（依赖锁定）

## P1 — 内嵌资源层 + 解包缓存

- [ ] `internal/embeddings/assets/`：各平台 `ort_<goos>_<goarch>.go` + `model.go` + `tokenizer.go` + `sha256.go`；`.gitignore` 排除大二进制（保留清单）
- [ ] `assets.Ensure()`：缓存目录解包、原子写、SHA256 复用、`OKF_ORT_DIR` 覆盖
- [ ] 测试：`TestEnsureAssets`（首次解包/缓存命中跳过/损坏重写/OKF_ORT_DIR/零网络断言）；属性测试：SHA256 幂等
- [ ] 覆盖 spec：S1.2（按平台）、S1.3（解包校验）、S1.4（损坏处置）

## P2 — Embedder 接口 + 向量索引 + 混合检索

- [ ] `pkg/embeddings.Embedder` 接口 + MiniLM 默认实现（`WithMeanPooling`/`WithL2Normalization`，seq 256）；`sync.Mutex` 串行化
- [ ] 测试：`EmbedQuery` 384 维/L2 归一化 golden、`EmbedDocuments` 批量、超长截断不 panic、`Close` 后报错、并发 `-race`
- [ ] `pkg/vectorindex`：`VectorIndex` 接口 + HNSW 封装（`Add`/`Search`/`Save`/`Load`/`Len`）+ `index.meta.json` 版本校验 + 指纹去重
- [ ] 测试：构建/搜索排序、持久化 round-trip、元信息不匹配提示 rebuild、指纹去重、`-race`
- [ ] `pkg/query.SemanticSearch`：RRF 融合（k=60）+ 来源标注 + 失败回退语义
- [ ] 测试：RRF 排序正确性、双通道优先、语义失败回退、与既有 `Search` 行为不冲突
- [ ] 覆盖 spec：S2.1/S2.2/S2.3、S3.1/S3.2/S3.3、S4.1/S4.2

## P3 — CLI 接线 + MCP 扩展

- [ ] `cmd/okf/cmd_vector.go`：`vector index|status|rebuild`；`main.go` 注册 `vector` 命令
- [ ] `cmdSearch` 支持 `-semantic` / `-k`（`-q` 复用既有参数）；`-verbose` 输出两通道命中与融合分；帮助文本与 usage 更新
- [ ] `pkg/mcp`：`okf_semantic_search` 工具（复用 `SemanticSearch` 核心）
- [ ] 测试：CLI 端到端（建索引→语义搜索→结果含来源标注；未建索引提示；无 `--semantic` 默认词法）；MCP 工具调用；`python3 test_mcp.py`
- [ ] 覆盖 spec：S5.1/S5.2/S5.3、S6.1

## P4 — 文档、回归、conformance

- [ ] README + README.zh-CN.md：用法、离线/体积、256 token 与中文限制、许可来源、动态加载声明
- [ ] 全量回归：`tools/gauntlet.sh` 全绿 + `CGO_ENABLED=0 go build ./...` + `python3 test_mcp.py`
- [ ] 输出 `conformance.md`（spec↔实现↔测试 对齐度逐条打勾）
- [ ] 性能冒烟：建索引吞吐、单查询延迟基准记录（供 README 标注）

## 验收门槛（合入前置）

- [ ] spec 全部 Scenario 有实现入口 + 测试用例（覆盖矩阵逐项打勾）
- [ ] `tools/gauntlet.sh` 全绿（含变异 4/4）
- [ ] `conformance.md` 无 `gap`；`partial` 需注明原因与后续任务
- [ ] `okf lint`（`docs/knowledge`）0 errors
