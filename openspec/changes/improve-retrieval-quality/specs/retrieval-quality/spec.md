# Specification: Retrieval Quality

## Overview

本规范定义 okf 检索质量的三项改进：(1) 分块级向量索引，消除 MiniLM 256 token 截断导致的内容丢失；(2) BM25 词法通道（CJK bigram + 代码标识符子词分词），替换布尔子串匹配；(3) 可配加权 RRF 与可复现索引。

约束：不改动 OKF v0.2 核心模型（`Concept` / `KnowledgeBundle`），不新增第三方依赖，不引入外部服务与网络请求，`CGO_ENABLED=0` 可构建。`SearchResult` 对外形态不变——分块是索引层内部实现，调用方无感。

## ADDED Requirements

### Requirement: 索引可复现（确定性）

向量索引的构建与检索 SHALL 是确定性的：相同输入产生逐位相同的索引结构与检索序列。

系统 SHALL 显式设置 `hnsw.Graph.Rng` 为固定种子的随机源，不得依赖 `coder/hnsw` 默认的 `rand.NewSource(time.Now().UnixNano())`。

所有参与排序的比较函数 SHALL 具备稳定 tie-break：分数相等时按 key 升序，不得依赖 Go map 遍历顺序。

#### Scenario: 重复构建索引产生相同检索序列

- **WHEN** 对同一 bundle 连续执行两次索引构建，并用同一 query 检索
- **THEN** 两次返回的结果 key 序列 MUST 逐位相同
- **AND** 两次返回的分数 MUST 逐位相同

#### Scenario: 融合排序在分数并列时稳定

- **GIVEN** 两个概念在加权 RRF 后得到完全相同的分数
- **WHEN** 执行检索
- **THEN** 二者的相对顺序 MUST 由 key 升序确定
- **AND** 多次运行 MUST 得到相同顺序

#### Scenario: BM25 检索在分数并列时稳定

- **GIVEN** 两个 chunk 的 BM25 分数完全相同
- **WHEN** 执行 BM25 检索
- **THEN** 二者的相对顺序 MUST 由 key 升序确定

---

### Requirement: Markdown 分块

系统 SHALL 提供 `pkg/chunk`，将概念内容按 Markdown 标题切分为 chunk，每个 chunk 携带 breadcrumb（标题路径）。

分块 SHALL 为纯函数：无 I/O、无全局状态、相同输入产生相同输出。

#### Scenario: 按 H2–H4 标题切分并生成 breadcrumb

- **GIVEN** 概念标题为 `CLI`，内容含 `## Commands` 与其下的 `### search`
- **WHEN** 执行分块
- **THEN** `### search` 所属 chunk 的 breadcrumb MUST 为 `CLI > Commands > search`
- **AND** breadcrumb MUST 包含在该 chunk 的编码文本中

#### Scenario: H1 不参与切分

- **GIVEN** 内容以 `# Title` 开头
- **WHEN** 执行分块
- **THEN** `#`（level 1）MUST NOT 触发切分
- **AND** 仅 level 2–4 标题 MUST 触发切分

#### Scenario: 代码围栏内的井号不被视为标题

- **GIVEN** chunk 内容含代码块，块内有行 `## not a heading`（位于一对 ` ``` ` 之间）
- **WHEN** 执行分块
- **THEN** 该行 MUST NOT 触发切分
- **AND** 代码块 MUST 保持在同一 chunk 内不被拆开

#### Scenario: 每个 chunk 不超过编码上限

- **WHEN** 对任意输入执行分块，MaxChars 默认为 1024
- **THEN** 每个 chunk 的 `Text()`（breadcrumb + body）长度 MUST NOT 超过 MaxChars
- **AND** 单个标题下内容超限时 MUST 按空行段落二次切分
- **AND** 二次切分后仍超限的单段 MUST 被硬切分，保证不超上限

#### Scenario: 无标题文档按段落兜底切分

- **GIVEN** 内容长度 3000 字符且不含任何 Markdown 标题
- **WHEN** 执行分块
- **THEN** MUST 产生多个 chunk（不得退化为单个超限 chunk）
- **AND** 每个 chunk MUST NOT 超过 MaxChars

#### Scenario: 过小的相邻同 breadcrumb 块被合并

- **GIVEN** 相邻两个 chunk 具有相同 breadcrumb，且前一个 body 长度小于 MinChars（默认 120）
- **WHEN** 执行分块
- **THEN** 二者 MUST 被合并为一个 chunk
- **AND** 合并后 MUST NOT 超过 MaxChars

#### Scenario: 分块不丢失内容

- **WHEN** 对任意概念内容执行分块
- **THEN** 所有 chunk body 拼接后 MUST 包含原内容的全部非空白字符
- **AND** 该性质 MUST 由 `testing/quick` 属性测试覆盖

---

### Requirement: 分块级向量索引与父概念回溯

向量索引 SHALL 以 chunk 为单位建立，索引 key 格式为 `<概念指纹>#<块序号>`；检索结果 SHALL 回溯到父概念后去重，对外返回概念级 `SearchResult`。

#### Scenario: 索引条目数等于 chunk 总数

- **GIVEN** 一个含 8 个概念、分块后产生 N 个 chunk 的知识库
- **WHEN** 执行 `okf vector index`
- **THEN** 索引条目数 MUST 等于 N（而非概念数）
- **AND** `okf vector status` MUST 同时显示概念数与 chunk 数

#### Scenario: 长文档的尾部内容可被语义检索命中

- **GIVEN** 一个 6000 字符的概念，其**最后一个** H2 小节含唯一短语 `zzz-tail-marker-phrase`
- **WHEN** 以该短语的语义近似表述执行 `okf search -semantic`
- **THEN** 该概念 MUST 出现在结果中
- **AND** 在概念级索引（改动前）下该短语因 256 token 截断而不可命中——此对比 MUST 由回归测试固定

#### Scenario: 同一概念的多个 chunk 命中后合并为一条结果

- **GIVEN** 某 query 命中同一概念下的 3 个 chunk
- **WHEN** 执行语义检索
- **THEN** 结果中该概念 MUST 只出现一次
- **AND** 其 RRF 分数 MUST 为各命中 chunk 分数之**累加**（不得取最佳块单值）

#### Scenario: 召回候选放大以避免单概念占满 TopK

- **GIVEN** `TopK=5`、`CandidateFactor=4`
- **WHEN** 执行块级检索
- **THEN** 底层 MUST 取至少 `TopK * CandidateFactor` 个 chunk 候选再回溯去重
- **AND** 最终返回 MUST NOT 超过 TopK 个概念

#### Scenario: 旧格式索引被明确拒绝而非静默降级

- **GIVEN** `.okf/vector/index.meta.json` 中 `index_format_version` 缺失或低于当前版本
- **WHEN** 加载索引
- **THEN** MUST 返回明确错误，提示执行 `okf vector rebuild`
- **AND** MUST NOT 静默使用旧索引产生错误结果
- **AND** MUST NOT 自动触发重建（避免隐式长耗时操作）

---

### Requirement: 分词（CJK bigram + 代码标识符子词）

系统 SHALL 提供 `pkg/lexical.Tokenize`，对拉丁段产出整词与标识符子词，对 CJK 段产出重叠 bigram。

#### Scenario: 代码标识符拆出子词且保留整词

- **WHEN** 对 `okf_semantic_search` 分词
- **THEN** 结果 MUST 同时包含整词 `okf_semantic_search` 与子词 `okf`、`semantic`、`search`

#### Scenario: camelCase 与连续大写缩写正确拆分

- **WHEN** 对 `EnableParentChild` 分词
- **THEN** 结果 MUST 包含 `enable`、`parent`、`child`
- **WHEN** 对 `HTTPServer` 分词
- **THEN** 结果 MUST 包含 `http`、`server`

#### Scenario: 中文产出重叠 bigram

- **WHEN** 对 `如何构建向量索引` 分词
- **THEN** 结果 MUST 为 `如何`、`何构`、`构建`、`建向`、`向量`、`量索`、`索引`
- **AND** 单个汉字独立成段时 MUST 产出该字的 unigram

#### Scenario: 中英混合按语种切段

- **WHEN** 对 `MCP 服务器有哪些工具` 分词
- **THEN** 结果 MUST 包含拉丁词 `mcp`
- **AND** MUST 包含中文 bigram `服务`、`务器`

#### Scenario: 分词稳定且无空 token

- **WHEN** 对任意字符串分词
- **THEN** 结果 MUST NOT 含空字符串
- **AND** 对同一输入重复调用 MUST 返回相同结果
- **AND** 该性质 MUST 由 `testing/quick` 属性测试覆盖

#### Scenario: 空与纯标点输入安全返回

- **WHEN** 对空串、纯空白、纯标点分词
- **THEN** MUST 返回空切片且不 panic

---

### Requirement: BM25 词法检索

系统 SHALL 以 `pkg/lexical.BM25` 实现 BM25 打分（`k1=1.2`、`b=0.75`），以 chunk 为索引单位，替换布尔子串匹配作为词法通道。

#### Scenario: 多词查询可召回并按相关性排序

- **GIVEN** 知识库中存在描述 `okf vector rebuild` 的 chunk
- **WHEN** 以 `vector index rebuild command` 执行词法检索
- **THEN** MUST 返回非空结果（改动前实测为 `No results found.`）
- **AND** 结果 MUST 按 BM25 分数降序排列

#### Scenario: 中文查询可召回

- **WHEN** 以 `如何构建向量索引` 执行词法检索
- **THEN** MUST 返回非空结果（改动前实测 Recall=0.0000）

#### Scenario: 标识符子词可召回

- **GIVEN** 某 chunk 含 `okf_semantic_search`
- **WHEN** 以 `semantic search` 执行词法检索
- **THEN** 该 chunk MUST 出现在结果中

#### Scenario: 词频与文档长度归一化生效

- **GIVEN** 两个 chunk 均含查询词，A 的词频更高且长度更短
- **WHEN** 执行 BM25 检索
- **THEN** A 的分数 MUST 高于 B

#### Scenario: 未命中任何词项时返回空

- **WHEN** 查询词在全部 chunk 中均不出现
- **THEN** MUST 返回空结果且不 panic

---

### Requirement: 可配加权 RRF

融合排序 SHALL 支持可配置的 RRF 常数 `k` 与两路权重。默认值 SHALL 为 `k=60`、向量权重 `0.7`、词法权重 `0.3`（比例 2.33:1）。

该默认值取自业界共识而非单点实测峰值：`k=60` 与 Elasticsearch `rank_constant`、Milvus `RRFRanker`、WeKnora `GetEffectiveRRFK()` 一致；`0.7/0.3` 与 WeKnora 源码默认一致，且与 okf 实测最优比例（2.0:1）相邻。

权重 SHALL 按相对比例生效（RRF 加权对绝对尺度不变），实现 MUST NOT 要求权重和为 1.0。

若扩充后的 golden set 实测显示该默认值下 MRR 低于纯语义通道（词法权重 0），则默认词法权重 SHALL 改为 `0.0`，且该结论 MUST 记录在 `conformance.md` 中。

#### Scenario: 权重与 k 可通过选项配置

- **WHEN** 以 `SearchOptions{RRFK: 30, VectorWeight: 1.0, LexicalWeight: 0.5}` 检索
- **THEN** 融合分数 MUST 按 `VectorWeight/(RRFK+rank_v) + LexicalWeight/(RRFK+rank_l)` 计算

#### Scenario: 权重按相对比例生效（尺度不变）

- **GIVEN** 两组权重 `{1.0, 0.5}` 与 `{0.667, 0.333}`（同为 2:1 比例）
- **WHEN** 分别检索同一 query
- **THEN** 两组返回的结果**顺序** MUST 完全相同
- **AND** 权重和不为 1.0 MUST NOT 被拒绝或告警

#### Scenario: 词法权重为 0 时等价于纯语义通道

- **WHEN** 以 `LexicalWeight: 0` 检索
- **THEN** 结果排序 MUST 与仅使用向量通道一致
- **AND** MUST NOT 执行 BM25 检索（避免无谓开销）

#### Scenario: 默认值可复现且被测试固定

- **WHEN** 不指定任何权重选项
- **THEN** MUST 使用 `k=60`、向量权重 `0.7`、词法权重 `0.3`
- **AND** 这些默认值 MUST 被测试断言固定，变更须同步更新 spec 与 `conformance.md`

#### Scenario: 来源标注保持既有语义

- **WHEN** 某概念同时被向量与词法通道命中
- **THEN** 其 `Source` MUST 为 `both`
- **AND** 仅向量命中为 `semantic`、仅词法命中为 `lexical`

---

### Requirement: 评测口径修复与策略对比

`pkg/eval` SHALL 以 `Concept.FilePath` 作为文档标识与 golden set 的 `expected_docs` 对齐，并支持注入不同检索策略以横向对比。

#### Scenario: 修复恒零基线

- **GIVEN** golden set 的 `expected_docs` 为文件名（如 `sample.xlsx.md`）
- **WHEN** 对由 `pkg/convert/testdata` 构建的 bundle 运行基准
- **THEN** 词法策略的 Recall@5 MUST 大于 0（改动前因使用恒为空的 `Concept.Resource` 而恒为 0）

#### Scenario: 支持多策略横向对比

- **WHEN** 运行基准
- **THEN** MUST 能分别评测「词法」「语义」「混合」三种策略并在同一报告中对比
- **AND** 报告 MUST 包含 Recall@K、Precision@K、MRR、NDCG@K

#### Scenario: golden set 覆盖语义化与中文查询

- **WHEN** 加载 golden set
- **THEN** 用例 MUST 包含多词查询、中文查询与代码标识符查询
- **AND** MUST 保留至少 2 条负例（`expected_docs` 为空）用于验证不误召回

#### Scenario: 评测结果可复现

- **WHEN** 连续两次运行同一基准
- **THEN** 所有指标数值 MUST 完全相同

---

### Requirement: CLI 接线与文档对齐

系统 SHALL 提供 `okf eval` 命令运行基准，并在 `okf search` 暴露融合参数；行为变更 SHALL 在 CLI 帮助与 README 中声明。

#### Scenario: okf eval 可运行并输出报告

- **WHEN** 执行 `okf eval -path <知识库> -golden <golden.json>`
- **THEN** MUST 输出各策略的 Recall@K / Precision@K / MRR / NDCG@K
- **AND** 退出码为 0；golden 文件缺失或格式错误时 MUST 返回非 0 并给出明确错误

#### Scenario: search 暴露融合参数

- **WHEN** 执行 `okf search -semantic -lexical-weight 0.5 -rrf-k 60 -q <query>`
- **THEN** 参数 MUST 生效
- **AND** `okf help` MUST 列出这些参数及其默认值

#### Scenario: 索引状态显示分块信息

- **WHEN** 执行 `okf vector status`
- **THEN** MUST 显示概念数、chunk 数、索引格式版本
- **AND** 索引格式过旧时 MUST 提示 `okf vector rebuild`

#### Scenario: 文档声明破坏性变更

- **WHEN** 变更落地
- **THEN** README（中英双语）MUST 说明索引格式不兼容需 `okf vector rebuild`
- **AND** MUST 给出实测的索引体积与构建耗时变化
- **AND** Release Notes MUST 声明 `okf search -semantic` 排序变化与此前无结果查询开始返回结果
