# Tasks: Improve Retrieval Quality

> **P0–P4 仅表依赖顺序，不构成交付边界。全部完成 + 全测试绿 + `conformance.md` 通过，才算完成。**

## P0 — 确定性前置（必须最先落地）

不先修好非确定性，后续任何评测数字都不可信。

- [x] **P0.1** 测试先行：写确定性回归测试 —— 同一 bundle 连续构建索引 2 次，同一 query 返回 key 序列与分数逐位相同（red）
- [x] **P0.2** `pkg/vectorindex`：`NewHNSW` 显式设置 `Rng = rand.New(rand.NewSource(42))`，消除 `coder/hnsw` 默认时间种子（green）
- [x] **P0.3** `pkg/query/semantic.go`：融合排序补稳定 tie-break（同分按 key 升序），消除 map 遍历顺序影响
- [x] **P0.4** 修复 `pkg/eval/benchmark.go`：`Concept.Resource` → `Concept.FilePath`，并加断言防止基线恒零回归
- [x] **P0.5** 用修复后的评测记录**当前基线数字**（写入 `conformance.md`，作为后续对比锚点）

## P1 — 分块（核心收益，MRR 实测 +10.2%）

- [x] **P1.1** 测试先行：`pkg/chunk` 单元测试 —— 无标题 / 仅 H1 / H2–H4 嵌套 / 代码围栏含 `#` / 超长段落 / 过小块合并 / 表格不被切断（red）
- [x] **P1.2** 属性测试（`testing/quick`）：拼接后不丢非空白字符；每 chunk `Text()` ≤ MaxChars
- [x] **P1.3** 实现 `pkg/chunk.Split` / `Chunk.Text`（纯函数，默认 MaxChars=1024、MinChars=120）
- [x] **P1.4** `pkg/vectorindex`：`index.meta.json` 增 `index_format_version`；加载旧版本明确报错并提示 `okf vector rebuild`（含测试）
- [x] **P1.5** `cmd/okf/cmd_vector.go`：索引构建改为遍历 chunk，key 为 `<概念指纹>#<序号>`
- [x] **P1.6** `pkg/query/semantic.go`：chunk key 回溯父概念、分数**累加**去重；新增 `CandidateFactor`（默认 4）
- [x] **P1.7** 回归测试：长文档尾部 H2 小节的唯一短语可被语义命中（固定「改动前不可命中」这一对比）
- [x] **P1.8** `okf vector status` 显示概念数 / chunk 数 / 索引格式版本
- [x] **P1.9** 用评测复测并记录分块后指标（对比 P0.5 基线）

## P2 — 词法通道（补齐多词 / 中文 / 标识符查询）

- [x] **P2.1** 测试先行：`pkg/lexical.Tokenize` 单元测试 —— 纯英文 / 纯中文 / 中英混合 / snake / camel / kebab / 连续大写缩写 / 数字 / 空与纯标点（red）
- [x] **P2.2** 属性测试：`Tokenize` 幂等、无空 token；纯 ASCII 输入与 `strings.Fields`+小写化在整词集合上一致
- [x] **P2.3** 实现 `Tokenize`（拉丁整词 + 标识符子词；CJK 重叠 bigram）
- [x] **P2.4** 测试先行：BM25 单元测试 —— 词频/长度归一化、未命中返回空、同分 tie-break 稳定（red）
- [x] **P2.5** 实现 `pkg/lexical.BM25`（`k1=1.2`、`b=0.75`，chunk 为索引单位）
- [x] **P2.6** 接入词法通道替换布尔子串匹配；`LexicalWeight=0` 时短路不执行 BM25
- [x] **P2.7** 验收测试：`vector index rebuild command`、`如何构建向量索引`、`semantic search` 均返回非空

## P3 — 可配加权 RRF 与默认值定案

- [x] **P3.1** `SearchOptions` 增 `RRFK` / `VectorWeight` / `LexicalWeight`，实现加权 RRF 公式（不强制权重和为 1.0）
- [x] **P3.2** 扩充 golden set：新增多词 / 中文 / 标识符查询，保留 ≥2 条负例
- [x] **P3.3** `pkg/eval`：支持注入检索策略，一次报告输出「词法 / 语义 / 混合」三路对比
- [x] **P3.4** 以业界共识值 `k=60`、`0.7/0.3`（比例 2.33:1，同 WeKnora 源码默认）作为默认值实现
- [x] **P3.5** 在扩充 golden set 上复测该默认值；**仅当 MRR 低于纯语义通道时**才改默认词法权重为 `0.0`，结论写入 `conformance.md`
- [x] **P3.6** 测试固定默认值（`k=60`、向量 `0.7`、词法 `0.3`）
- [x] **P3.7** 测试权重尺度不变性：`{1.0,0.5}` 与 `{0.667,0.333}` 返回顺序完全相同

## P4 — 接线、文档与门槛

- [x] **P4.1** 新增 `okf eval` 子命令（`-path` / `-golden` / `-k`），错误路径返回非 0
- [x] **P4.2** `okf search` 暴露 `-lexical-weight` / `-rrf-k`；`okf help` 列出参数与默认值
- [x] **P4.3** README 中英双语：索引格式不兼容需 rebuild、**实测**索引体积与耗时变化、BM25 能力与默认状态
- [x] **P4.4** Release Notes：声明 `-semantic` 排序变化、此前无结果查询开始返回结果
- [x] **P4.5** 扩展 `tools/mutants.sh`：变异点覆盖 `pkg/chunk` 与 `pkg/lexical` 核心逻辑，4/4 必须被杀死
- [x] **P4.6** `tools/gauntlet.sh` 全绿（build / vet+gofmt+staticcheck / test -race / 覆盖率 ≥60% / shuffle / 变异 / 真实执行）
- [x] **P4.7** 输出 `conformance.md`：逐条对照 spec Scenario ↔ 实现 ↔ 测试，标注 fully/aligned/partial/gap；`partial`/`gap` 给出原因与后续任务

## 覆盖矩阵（spec Scenario ↔ 实现 ↔ 测试）

| Requirement | Scenario | 实现入口 | 测试 | 状态 |
|---|---|---|---|---|
| 索引可复现 | 重复构建产生相同序列 | `vectorindex.NewHNSW` | `vectorindex_test.go` | ☑ |
| 索引可复现 | 融合排序并列稳定 | `query.SemanticSearch` | `semantic_test.go` | ☑ |
| 索引可复现 | BM25 并列稳定 | `lexical.BM25.Search` | `lexical_test.go` | ☑ |
| Markdown 分块 | H2–H4 切分 + breadcrumb | `chunk.Split` | `chunk_test.go` | ☑ |
| Markdown 分块 | H1 不参与切分 | `chunk.Split` | `chunk_test.go` | ☑ |
| Markdown 分块 | 代码围栏内 `#` 不切分 | `chunk.Split` | `chunk_test.go` | ☑ |
| Markdown 分块 | 不超编码上限 | `chunk.Split` | 属性测试 | ☑ |
| Markdown 分块 | 无标题按段落兜底 | `chunk.Split` | `chunk_test.go` | ☑ |
| Markdown 分块 | 过小块合并 | `chunk.Split` | `chunk_test.go` | ☑ |
| Markdown 分块 | 不丢失内容 | `chunk.Split` | 属性测试 | ☑ |
| 分块级索引 | 条目数=chunk 数 | `cmd_vector.go` | `cmd_vector_test.go` | ☑ |
| 分块级索引 | 长文尾部可命中 | `cmd_vector.go` + `semantic.go` | 回归测试 | ☑ |
| 分块级索引 | 多块合并为一条（累加） | `query.SemanticSearch` | `semantic_test.go` | ☑ |
| 分块级索引 | 候选放大 | `SearchOptions.CandidateFactor` | `semantic_test.go` | ☑ |
| 分块级索引 | 旧索引明确拒绝 | `vectorindex.Load` | `vectorindex_test.go` | ☑ |
| 分词 | 标识符子词 + 整词 | `lexical.Tokenize` | `lexical_test.go` | ☑ |
| 分词 | camelCase / 缩写 | `lexical.Tokenize` | `lexical_test.go` | ☑ |
| 分词 | 中文 bigram | `lexical.Tokenize` | `lexical_test.go` | ☑ |
| 分词 | 中英混合 | `lexical.Tokenize` | `lexical_test.go` | ☑ |
| 分词 | 稳定无空 token | `lexical.Tokenize` | 属性测试 | ☑ |
| 分词 | 空 / 纯标点安全 | `lexical.Tokenize` | `lexical_test.go` | ☑ |
| BM25 | 多词可召回排序 | `lexical.BM25` | `lexical_test.go` | ☑ |
| BM25 | 中文可召回 | `lexical.BM25` | `lexical_test.go` | ☑ |
| BM25 | 标识符子词召回 | `lexical.BM25` | `lexical_test.go` | ☑ |
| BM25 | 词频/长度归一化 | `lexical.BM25` | `lexical_test.go` | ☑ |
| BM25 | 未命中返回空 | `lexical.BM25` | `lexical_test.go` | ☑ |
| 加权 RRF | 权重与 k 可配 | `SearchOptions` | `semantic_test.go` | ☑ |
| 加权 RRF | 权重按比例生效（尺度不变） | `query.SemanticSearch` | `semantic_test.go` | ☑ |
| 加权 RRF | 权重 0 短路 | `query.SemanticSearch` | `semantic_test.go` | ☑ |
| 加权 RRF | 默认值被固定（60 / 0.7 / 0.3） | `SearchOptions` 默认 | `semantic_test.go` | ☑ |
| 加权 RRF | 来源标注语义 | `query.SemanticSearch` | `semantic_test.go` | ☑ |
| 评测 | 修复恒零基线 | `eval.RunBenchmark` | `benchmark_test.go` | ☑ |
| 评测 | 多策略对比 | `eval.RunBenchmark` | `benchmark_test.go` | ☑ |
| 评测 | golden 覆盖语义化/中文 | `golden_queries.json` | `benchmark_test.go` | ☑ |
| 评测 | 结果可复现 | `eval.RunBenchmark` | `benchmark_test.go` | ☑ |
| CLI | `okf eval` 可运行 | `cmd_eval.go` | `cmd_eval_test.go` | ☑ |
| CLI | search 暴露参数 | `cmd_search.go` | `cmd_search_test.go` | ☑ |
| CLI | status 显示分块信息 | `cmd_vector.go` | `cmd_vector_test.go` | ☑ |
| CLI | 文档声明破坏性变更 | README / releases.md | 人工核对 + lint | ☑ |
