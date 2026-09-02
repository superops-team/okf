# Design: Improve Retrieval Quality

## 决策总览（无 Open Question）

| 决策点 | 定案 | 依据 |
|---|---|---|
| chunk 上限 | **1024 字符**（≈256 token，MiniLM 硬上限） | 实测 88 块 0 超限；主流建议 512 token 不适用，MiniLM 上限即 256 |
| chunk 下限（合并阈值） | **120 字符** | 低于此值与相邻同 breadcrumb 块合并，避免碎片稀释 IDF |
| 切分依据 | Markdown `##`~`####`（level 2–4），`#` 归入概念标题 | 天然语义边界，okf 已有 Markdown 解析 |
| 代码围栏 | ` ``` ` 内的 `#` **不视为标题** | 防止 shell 注释 / Python 注释被误切 |
| breadcrumb | `概念标题 > H2 > H3` 前缀写入 chunk 编码文本 | 实测保留层级语义；调研中的通用做法 |
| chunk key | `<概念指纹>#<块序号>`（序号从 0，按文档顺序） | 与既有 `Fingerprint()` 兼容，可回溯父概念 |
| 多块命中同概念 | **RRF 分数累加**（非取最佳块） | 实测：累加 Recall@5 +8.3%；max-pooling 退化到 +0.0% |
| BM25 参数 | `k1=1.2`, `b=0.75` | IR 领域标准默认 |
| 分词：拉丁 | 整词小写 + snake/camel/kebab 子词（**同时保留整词**） | 实测 `okf_semantic_search` → `[okf_semantic_search, okf, semantic, search]` |
| 分词：CJK | 重叠 bigram（单字则 unigram） | 无需词典、零维护；实测 `如何构建向量索引` → 7 个 bigram |
| RRF k 默认 | **60** | 业界一致默认：Elasticsearch `rank_constant=60`、Milvus `RRFRanker(k=60)`、WeKnora `GetEffectiveRRFK()→60`；okf 现状亦为 60 |
| 加权范式 | **范式 A（RRF，基于排名）**，不采用 alpha 线性加权 | okf 已实现 RRF；范式 A 免分数归一化，BM25 与余弦分数不可比也不影响 |
| RRF 权重默认 | 向量 `0.7` / 词法 `0.3`（比例 2.33:1），**待扩充 golden set 复测确认** | 与 WeKnora 源码默认一致；okf 实测最优 `1.0/0.5`（比例 2.0:1）与之相邻，落在业界 1:1~3:1 区间正中 |
| 权重归一化 | 权重和 SHOULD 为 `1.0`（便于理解），但**不强制** | 实测验证：RRF 加权对绝对尺度不变，同比例缩放不改变排序；`1.0/0.5` ≡ `0.667/0.333` |

### 加权范式辨析（避免误搬默认值）

业界存在两套**语义不同、不可混用**的混合检索加权范式：

| | 范式 A：RRF（排名） | 范式 B：alpha 线性加权（分数） |
|---|---|---|
| 公式 | `wv/(k+rank_v) + wk/(k+rank_l)` | `alpha*norm(vec) + (1-alpha)*norm(bm25)` |
| 代表实现 | Elasticsearch RRF retriever（`rank_constant=60`，子检索器等权）、Milvus `RRFRanker(k=60)`、WeKnora（`0.7/0.3`） | Weaviate `hybrid`（`alpha` 默认 `0.75`）、Milvus `WeightedRanker`（需 `norm_score`） |
| 是否需归一化 | 不需要（只用排名） | **必须**，否则量纲不同直接失效 |
| 权衡 | 对异常分数鲁棒；丢失分数间距信息 | 保留分数间距；对分数分布敏感 |

**okf 属于范式 A**，因此权重语义是「两路**排名**贡献的相对比例」，而非「分数占比」。**不得**把 Weaviate 的 `alpha=0.75` 直接搬成 okf 的 `0.75/0.25`——二者作用对象不同（归一化分数 vs `1/(k+rank)`）。

跨实现的比例参考（仅比例有意义）：

| 来源 | 向量:词法 |
|---|---|
| Weaviate `alpha=0.75` 的等效比例 | 3.00 : 1 |
| **WeKnora `0.7/0.3`（源码默认）** | **2.33 : 1** |
| **okf 实测最优 `1.0/0.5`** | **2.00 : 1** |
| Elasticsearch RRF 默认（子检索器等权） | 1.00 : 1 |

### 权重取舍原则

1. **向量权重 > 词法权重是常态**：语义通道覆盖面更广（能处理同义、改写、跨语言），词法通道精度高但召回窄。业界默认普遍落在 2:1 ~ 3:1。
2. **提高词法权重的场景**：精确标识符/错误码/API 名检索、专有名词多、语料含大量代码——okf 作为代码知识库偏向此侧，故取比例区间偏低端（2.33:1 而非 3:1）。
3. **提高向量权重的场景**：自然语言提问式查询、跨语言检索、语料为叙述性长文。
4. **不追求"最优单点"**：RRF 对权重不敏感（分母 `k+rank` 中 `k=60` 起强平滑作用），2:1 与 2.33:1 的实测差异在小语料上处于噪声量级。因此默认值应取**业界共识值**而非过拟合当前 golden set 的实测峰值。
5. **必须可关闭**：`LexicalWeight=0` 退化为纯语义，作为 BM25 在特定语料上产生噪声时的逃生阀。
| HNSW 种子 | 固定 `Rng = rand.New(rand.NewSource(42))` | `coder/hnsw` 显式支持（`graph.go:222` 注释）；不固定则结果不可复现 |
| 评测文档口径 | `Concept.FilePath` | 实测 `Concept.Resource` 恒为空，现基线恒为 0 |

## 架构与最小化实现

遵循 AGENTS.md「最小化实现」「架构统一」：新增能力走**独立包 + 索引层内部实现**，不侵入核心模型。

```
新增：
  pkg/chunk/          分块（纯函数：概念 → []Chunk）
  pkg/lexical/        分词 + BM25（纯函数 + 内存索引）

修改（受控、必要）：
  pkg/vectorindex/    固定 Rng；index.meta.json 增 index_format_version
  pkg/query/semantic.go  RRF 权重/k 可配；chunk key 回溯父概念；稳定 tie-break
  pkg/eval/benchmark.go  Resource → FilePath；支持注入检索函数（对比多策略）
  cmd/okf/cmd_vector.go  索引构建改为遍历 chunk
  cmd/okf/cmd_search.go  暴露权重/k flag
  cmd/okf/main.go        新增 eval 子命令

不改（硬约束）：
  pkg/okf/            核心模型 Concept / KnowledgeBundle
  pkg/parser/ pkg/lint/ pkg/convert/ pkg/git/ pkg/tool/
  SmartImportSource / CollectFiles / watch 配置
```

### pkg/chunk

```go
type Chunk struct {
    Breadcrumb string // "概念标题 > H2 > H3"
    Body       string
    Ordinal    int    // 块序号，用于组装 key
}

// Split 把概念内容按标题切分；超长块按空行段落二次切分；过小块与相邻同 breadcrumb 块合并。
func Split(title, content string, opts Options) []Chunk

type Options struct {
    MaxChars int // 默认 1024
    MinChars int // 默认 120
}

// Text 返回用于 embedding / BM25 的完整文本（breadcrumb + body）。
func (c Chunk) Text() string
```

纯函数、无状态、无 I/O，便于 TDD 与 `testing/quick` 属性测试。

### pkg/lexical

```go
// Tokenize：拉丁段 → 整词 + 标识符子词；CJK 段 → 重叠 bigram。
func Tokenize(s string) []string

type BM25 struct { /* df, tf, dl, avgdl */ }

func NewBM25() *BM25
func (b *BM25) Add(key, text string)   // 建索引
func (b *BM25) Finalize()              // 计算 avgdl（Add 完成后调用一次）
func (b *BM25) Search(query string, k int) []Hit  // 按分数降序，同分按 key 升序（稳定）
```

`Search` 必须做 tie-break（同分按 key 升序），否则结果不确定。

### 检索链路（分块透明化）

```
query
  ├─ 向量通道：EmbedQuery → HNSW.Search(k*4)  → chunk keys
  ├─ 词法通道：BM25.Search(k*4)                → chunk keys
  └─ 加权 RRF：score[concept] += w/(k+rank)    ← chunk key 回溯父概念，分数累加
     └─ 排序（同分 tie-break）→ TopK → []SearchResult（对外形态不变）
```

**召回放大**：块级检索需取 `k*4` 候选再回溯去重，否则同一概念的多个块会占满 TopK。该系数写入 `SearchOptions`（`CandidateFactor`，默认 4）。

## 关键风险与处置

| 风险 | 处置 |
|---|---|
| 旧索引不兼容导致静默错误结果 | `index.meta.json` 写 `index_format_version`；加载旧版本**明确报错**并提示 `okf vector rebuild`，不静默降级 |
| 索引体积/耗时上升（实测约 12 倍向量） | 在 spec 中给出实测体积与耗时门槛；README 声明；`okf vector status` 显示块数与概念数 |
| BM25 在小语料拖低 MRR（已实测） | 默认取业界共识 `0.7/0.3`（RRF 的 `k=60` 本身有强平滑作用，对权重不敏感）；扩充 golden set 复测把关，若 MRR 低于纯语义则默认改 `0.0`；`-lexical-weight 0` 提供逃生阀 |
| bigram 使 IDF 稀释 | 由评测把关；不引入词典（避免依赖与维护成本） |
| 代码围栏内 `#` 被误切 | 切分时跟踪 ` ``` ` 状态；配 fixture 测试 |
| 无标题文档退化为单块（仍被截断） | 无标题时按段落切分兜底，保证任何输入都不超 MaxChars |
| chunk 序号漂移导致索引 key 不稳定 | 序号按文档顺序确定；`LoadBundle` 概念顺序已实测稳定 |
| 分块后中文语义变稀疏（实测 2 条中文 MRR 下降） | 由 BM25 中文 bigram 通道补偿；以评测验证而非假设 |

## 测试策略（TDD，测试先行）

- **单元**：`pkg/chunk` 覆盖 —— 无标题、仅 H1、H2/H3/H4 嵌套、代码围栏含 `#`、超长段落、过小块合并、表格不被切断。
- **单元**：`pkg/lexical` 覆盖 —— 纯英文、纯中文、中英混合、snake/camel/kebab/连续大写缩写、数字、标点、空串。
- **属性测试**（`testing/quick`，不新增依赖，遵循 AGENTS.md）：
  - `Split` 结果拼接后不丢失非空白字符；
  - 每个 chunk 的 `Text()` 长度 ≤ MaxChars；
  - `Tokenize` 幂等（对同一输入稳定）且不返回空 token；
  - `Tokenize` 对纯 ASCII 输入与 `strings.Fields` + 小写化在整词集合上一致。
- **确定性回归**（针对已发现缺陷）：同一 bundle 连续构建索引 2 次，同一 query 返回序列**必须逐位相同**。
- **评测**：扩充 golden set，新增语义化 / 多词 / 中文 / 标识符查询；`okf eval` 输出各策略对比。
- **门槛**：`tools/gauntlet.sh` 全绿（build / vet+gofmt+staticcheck / test -race / 覆盖率 ≥60% / shuffle / 变异 4-4 / 真实执行）；变异脚本需扩展覆盖 `pkg/chunk` 与 `pkg/lexical` 核心逻辑。
