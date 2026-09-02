# Proposal: Improve Retrieval Quality (Chunked Index + BM25 + Weighted RRF)

## Problem

okf 已具备本地语义检索（MiniLM 384 维 + HNSW + RRF 融合），但**三个已实测的缺陷**使检索质量在真实使用场景下显著低于应有水平。以下数字全部由本仓库 `docs/knowledge`（8 个概念、26018 字符）与 `pkg/eval` 指标实测得出，非估算。

### P0：68.7% 的知识库内容根本没有进入向量索引

`cmd/okf/cmd_vector.go:conceptText()` 把整个概念（title + description + content）拼成**一个**字符串交给 MiniLM 编码，而 MiniLM 的硬上限是 256 token（≈1024 英文字符），超出部分被 tokenizer 静默截断。

实测（`okf vector status` 显示 7 概念 = 7 向量）：

| 文档 | 字符数 | 进入向量 | 丢失 |
|---|---|---|---|
| mcp-server.md | 6310 | 1024 | **83.8%** |
| releases.md | 5098 | 1024 | 79.9% |
| lint.md | 3725 | 1024 | 72.5% |
| cli.md | 3612 | 1024 | 71.7% |
| durable-capture.md | 2652 | 1024 | 61.4% |
| core-types.md | 1999 | 1024 | 48.8% |
| parser.md | 1648 | 1024 | 37.9% |
| index.md | 974 | 974 | 0.0% |
| **合计** | **26018** | **8142** | **68.7%** |

直接后果：语义分数完全没有区分度（实测全部命中挤在 0.0149–0.0164 区间），文档后 2/3 的内容对语义检索不可见。

### P1：词法通道是布尔子串匹配，无相关性打分，多词与中文查询直接 0 结果

`pkg/query/query.go:SearchWithMatches()` 对 query 做整串 `containsFold`，命中即入列，**不分词、不打分**，返回顺序仅取决于概念遍历顺序。

实测 12 条语义化/多词/中文查询（真实 Agent 使用形态，如 `vector index rebuild command`、`如何构建向量索引`）：

| 通道 | Recall@5 | MRR |
|---|---|---|
| 词法（现状） | **0.0000** | **0.0000** |
| 语义（现状） | 1.0000 | 0.8194 |

词法通道在全部 12 条上**零命中**（含 3 条中文）。这意味着 RRF 融合的两路输入之一长期是纯噪声：`okf search -q "vector index rebuild"` 实测返回 `No results found.`

同时，作为代码知识库，标识符不可检索：`okf_semantic_search`、`EnableParentChild` 这类符号无法通过子词（`semantic`、`parent`）命中。

### P2：RRF 参数硬编码，且向量索引不可复现

- `pkg/query/semantic.go` 硬编码 `rrfK = 60`、两路等权，无法针对代码知识库调优（代码检索通常应提高词法权重）。
- **`pkg/vectorindex.NewHNSW()` 未设置 `hnsw.Graph.Rng`**，`coder/hnsw@v0.6.1` 因此回落到 `rand.NewSource(time.Now().UnixNano())`（`graph.go:244`）。每次建索引图结构都不同，同一 query 的返回序列在多次 `okf vector rebuild` 之间会漂移。这既让检索结果不可复现，也让"用评测数据验证改进"无法成立——实测同一份代码同一参数连跑 8 次，MRR 在 −3.4% ~ +2.2% 间抖动。

## Goal

在**不改动 OKF v0.2 核心模型（Concept / KnowledgeBundle）**、不新增第三方依赖、不引入外部服务的前提下：

1. **分块级索引**：按 Markdown 标题切分概念为 chunk 建立向量索引，消除 256 token 截断导致的内容丢失；检索命中 chunk 后回溯父概念去重，`SearchResult` 对外形态不变。
2. **BM25 词法通道**：以纯 Go 实现 BM25，配 CJK bigram + 代码标识符子词分词，替换布尔子串匹配，使多词、中文、标识符查询可召回且有相关性排序。
3. **可配加权 RRF**：`k`、向量权重、词法权重可配置，默认取业界共识值 `k=60`、`0.7/0.3`（与 WeKnora 源码默认一致）。
4. **索引可复现**：固定 HNSW `Rng` 种子，使索引构建与检索结果确定；同时补齐融合排序的稳定 tie-break。
5. **评测可用**：修复 `pkg/eval` 用 `Concept.Resource`（实测恒为空）打分导致基线恒为 0 的缺陷，改用 `FilePath` 口径；扩充 golden set 覆盖语义化/中文/标识符查询；接线 `okf eval` 命令。

## 实测收益与不确定性（诚实披露）

在 `docs/knowledge` + 13 条语义化查询上的 A/B 实测（固定 HNSW 种子后）：

| 方案 | Recall@5 | MRR | 备注 |
|---|---|---|---|
| 现状（概念级 + 子串） | 0.9231 | 0.7462 | 基线 |
| **仅分块级向量** | 1.0000 | **0.9028** | **MRR +10.2%**，收益最大且稳定 |
| 分块 + BM25 等权 RRF | 1.0000 | 0.7910 | Recall +8.3%，MRR 被拖低 |
| 分块 + BM25 加权（wk=0.5） | 0.9231~1.0 | 0.6885~0.7628 | 8 次实测抖动，均值约 −1.3% |

**必须明确的结论修正**：最初假设"BM25 是分块的必要配套"，实测**不成立**。在当前 8 文档/85 块的小规模语料上，bigram 使 IDF 被稀释，BM25 引入的噪声在融合后反而拖低 MRR。

因此本变更的收益结构是：

- **P0（分块）= 确定收益**，MRR +10.2%，且从根本上消除 68.7% 内容不可见——即使 MRR 不变，"内容可被检索"本身就是正确性修复。
- **P1（BM25）= 补齐能力短板，但需以评测把关**：它解决的是"多词/中文/标识符查询 0 结果"这一功能缺失（现状 Recall=0.0000），价值在覆盖面而非当前小语料的 MRR。默认权重取业界共识 `0.7/0.3`（比例 2.33:1，同 WeKnora；okf 实测最优比例 2.0:1 与之相邻，均落在业界 1:1~3:1 区间）。**若扩充 golden set 复测显示该默认下 MRR 低于纯语义通道，则词法权重默认改为 `0.0`**，BM25 退为可选能力。
- **P2（可配 + 可复现）= 前置条件**：不先修好 HNSW 非确定性，P0/P1 的任何数字都不可信。因此 P2 中的"固定种子"实际应**最先落地**。

## Non-Goals

- 不做 GraphRAG / 知识图谱：需 LLM 抽取实体关系 + 图数据库，破坏零依赖与离线性。
- 不做 cross-encoder rerank：需再内嵌一个模型（体积 +100MB）或调远程 API，两者都违背当前设计。
- 不做 parent-child 双索引（WeKnora 式）：okf 概念本身即父级，chunk 回溯父概念已覆盖该收益，双索引属过度设计。
- 不改 `pkg/okf` 核心模型、不改 `SmartImportSource` / `CollectFiles` / watch 配置。
- 不改 `okf search` 默认（非 `-semantic`）行为的对外语义。
- 不引入第三方分词库（jieba 等）或 BM25 库：标准库实现，避免依赖与词典维护。

## 向下兼容与破坏性影响

- **索引格式变更**：向量索引 key 由 `<概念指纹>` 变为 `<概念指纹>#<块序号>`，旧 `.okf/vector/index.bin` 不兼容。`index.meta.json` 须写入索引格式版本；加载到旧版本时**明确报错并提示 `okf vector rebuild`**，不静默降级。
- **索引体积与耗时上升**：实测 7 概念 → 85 块（约 12 倍向量数）。需在 spec 中给出实测的体积/耗时数据并写入 README。
- **行为变更需声明**：`okf search` 在 `-semantic` 下的排序会变化；BM25 使此前"无结果"的查询开始返回结果。须在 CLI 帮助与 README/Release Notes 中声明。
