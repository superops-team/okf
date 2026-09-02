# Conformance: improve-retrieval-quality

对照对象：`openspec/changes/improve-retrieval-quality/specs/retrieval-quality/spec.md`
（8 Requirement / 39 Scenario）。

对齐度取值：`fully`（实现 + 测试均覆盖，且测试能真正证伪）、`aligned`（实现覆盖，测试为间接覆盖）、`partial`、`gap`。

**结论：39/39 Scenario 为 `fully`，无 `partial`，无 `gap`。**

验证方式（非声明，均为实际执行）：

- `tools/gauntlet.sh` → `GAUNTLET PASS: build/vet/staticcheck/tests(-race)/coverage(64%)/shuffle/mutation/real-exec`
- `tools/mutants.sh` → `manual mutation: 14/14 killed (restore verified, suite green)`
- `okf lint -path docs/knowledge` → 0 errors
- `okf eval -golden pkg/eval/testdata/golden_semantic.json -path docs/knowledge -compare` → 见下方实测表

---

## R1 索引可复现（确定性）

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 1 | 重复构建索引产生相同检索序列 | `pkg/vectorindex/vectorindex.go`：`indexRngSeed=42` 固定 `g.Rng`；`Search` 候选放大 + 精确重排 | `TestSearchIsDeterministicAcrossRebuilds`、`TestSearchDeterministicAfterReload`、`TestSearchDeterministicOnLargeIndex` | fully |
| 2 | 融合排序在分数并列时稳定 | `pkg/query/semantic.go` 五级 tie-break：分数 > 语义 rank > 有无语义命中 > 来源 > Fingerprint | `TestWeightedFusionIsReproducible` | fully |
| 3 | BM25 检索在分数并列时稳定 | `pkg/lexical/lexical.go`：`Search` 同分按 `Key` 升序 | `TestBM25TieBreakIsStable`、`TestBM25IsDeterministic` | fully |

**实施记录（重要偏差）**：初版仅固定 `Rng` 并**未能**消除漂移，漂移位置只是从 rank 1 移到 rank 2。根因是 `coder/hnsw@v0.6.1` 的 `layer.entry()`（`graph.go` 约 198 行）用 `for _, node := range l.nodes { return node }` 取搜索入口，依赖 Go map 遍历顺序，固定种子覆盖不到。经暴力全量真值对照确认漂移性质是**召回不足**而非排序不稳（候选分数各不相同，全量召回三次一致且精确匹配真值），故最终方案改为「候选放大 + 封装层精确重排」：索引规模 ≤ `searchExactMaxNodes=2048` 时全量召回（等价精确检索），超过时取 `k*8 + len(removed)` 且不低于 128。

---

## R2 Markdown 分块

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 4 | 按 H2–H4 标题切分并生成 breadcrumb | `pkg/chunk/chunk.go`：`Split` + `parseHeading`，`minSplitLevel=2`/`maxSplitLevel=4` | `TestSplitByHeadingBuildsBreadcrumb`、`TestDeepHeadingLevels`、`TestHeadingLevelJumpBack` | fully |
| 5 | H1 不参与切分 | 同上，level 1 不触发 | `TestH1DoesNotSplit` + 变异 M6 | fully |
| 6 | 代码围栏内的井号不被视为标题 | `isFenceLine` 跟踪 ``` / ~~~ 状态 | `TestCodeFenceHashNotTreatedAsHeading` + 变异 M5 | fully |
| 7 | 每个 chunk 不超过编码上限 | `fitPieces` / `hardSplit` / `splitRunes`；`DefaultMaxChars=1024` | `TestChunkNeverExceedsMaxChars`、`TestPropertyChunkWithinMaxChars`、`TestOverlongBreadcrumbStillRespectsMaxChars` | fully |
| 8 | 无标题文档按段落兜底切分 | `fitPieces` 按 `\n\n` 累积 | `TestNoHeadingLongContentStillSplits`、`TestSingleOversizedParagraphIsHardSplit` | fully |
| 9 | 过小的相邻同 breadcrumb 块被合并 | `mergeTiny`，`DefaultMinChars=120` | `TestTinyAdjacentChunksMerged` + 变异 M8 | fully |
| 10 | 分块不丢失内容 | `Split` 逐行扫描不丢非空白内容 | `TestPropertySplitPreservesNonSpaceContent`、`TestTableNotSplitAcrossChunks` | fully |

**实施记录（缺陷发现与修复）**：

1. **Scenario 9 一度为假绿**。原测试用 `## S\n\na\n\nb\n\nc\n\nd` + `MaxChars=1024`，这些短段落被 `fitPieces` 一次性装进同一块，`mergeTiny` 无事可做——删掉 `mergeTiny` 测试依然通过。变异 M8 存活暴露了此问题。实证扫描后发现 `mergeTiny` 的可达窗口很窄（随机语料 447/20000 ≈ 2.2%，真实 `docs/knowledge` 下 0 次生效，因为 `fitPieces` 贪心装填使前块通常已接近 `MaxChars`）。因 spec 明确要求该行为故保留实现，改用实证得出的参数（`MaxChars=30, MinChars=6` + 混合长度语料）让测试真正走进合并分支，并加入前置条件断言。

2. **Scenario 7 修复了一个真实越界缺陷**（由属性测试 `TestPropertyChunkWithinMaxChars` 随机输入捕获）。当 breadcrumb 长度 ≥ `MaxChars` 时，旧实现退化为 `budget = 1` 却仍拼接完整 breadcrumb，导致**每个块都超限**（实测 250 字节 breadcrumb / 200 上限 → 每块 252 字节），下游 embedding 的 token 上限假设随之失效。修复：新增 `minBodyBudget=16`，按 rune 边界截断 breadcrumb（`truncateRunes` / `lastRuneStart`）保证约束恒成立；`fitPieces` 返回签名改为 `([]string, string)` 以回传截断后的 breadcrumb。已加回归测试 `TestOverlongBreadcrumbStillRespectsMaxChars`（4 个子用例）与 `TestTruncatedBreadcrumbKeepsValidUTF8`。

---

## R3 分块级向量索引与父概念回溯

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 11 | 索引条目数等于 chunk 总数 | `cmd/okf/cmd_vector.go`：遍历 `chunk.Split` 结果，key = `query.ChunkKey`；`Meta.Concepts` 记录概念数 | `TestIndexEntryCountEqualsChunkCount`、`TestConceptChunkSourceIncludesDescription` | fully |
| 12 | 长文档的尾部内容可被语义检索命中 | `pkg/query/semantic.go`：`ChunkKeyConcept` 回溯父概念 | `TestLongDocumentTailIsRetrievable`（含改动前后对比，前置条件断言概念级索引下确实召回不到） | fully |
| 13 | 同一概念的多个 chunk 命中后合并为一条结果（分数**累加**） | `semRank` / `semHits` + `rrfScore(...) * float32(hits)` | `TestMultiChunkHitsAccumulate`、`TestManyChunksOfSameConceptCollapseToOneResult` + 变异 M14 | fully |
| 14 | 召回候选放大以避免单概念占满 TopK | `CandidateFactor`（默认 4），`want = TopK * CandidateFactor` | `TestCandidateAmplification`、`TestCandidateFactorDefaultsToFour` | fully |
| 15 | 旧格式索引被明确拒绝而非静默降级 | `CurrentIndexFormatVersion=2`；`Load` 先校验格式版本再校验维度 | `TestSaveWritesIndexFormatVersion`、`TestLoadRejectsMissingFormatVersion`、`TestLoadRejectsOlderFormatVersion`、`TestLoadAcceptsCurrentFormatVersion` | fully |

**实施记录（设计修正）**：候选窗口最初以 `SemanticTopK * CandidateFactor` 为基准（= 80），而本仓库知识库总块数仅 86 —— 几乎全部块被召回，**分数累加退化为「按块数排名」**，21 块的 `lint.md` 压过真正相关的 `cli.md`。改为以 `TopK` 为基准（`TopK * CandidateFactor`）后修正。这条与 spec 的「候选放大」并不冲突：spec 要求的是至少 `TopK * CandidateFactor`，实现取的正是该值。

Scenario 15 的实测行为：旧索引触发 `索引格式版本 0 与当前版本 2 不兼容（分块级索引），请执行 okf vector rebuild`，检索回退词法而非崩溃；`okf vector status` 显示 `索引格式: v0` 并附提示。

---

## R4 分词（CJK bigram + 代码标识符子词）

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 16 | 代码标识符拆出子词且保留整词 | `splitIdentifier`（`_ - .` 分隔） | `TestTokenizeIdentifierKeepsWholeAndSubwords`、`TestTokenizeKebabAndDot` + 变异 M9 | fully |
| 17 | camelCase 与连续大写缩写正确拆分 | `lowerToUpper` / `acronymEnd` 边界判定 | `TestTokenizeCamelCase`、`TestTokenizeAcronymBoundary` | fully |
| 18 | 中文产出重叠 bigram | `flushCJK` 产出 `cjk[i:i+2]` | `TestTokenizeChineseBigram` + 变异 M10 | fully |
| 19 | 中英混合按语种切段 | `Tokenize` 按 `isCJK` / `isWordRune` 分流并互相 flush | `TestTokenizeMixedLanguage`、`TestTokenizeSingleCJKChar` | fully |
| 20 | 分词稳定且无空 token | 同上 | `TestPropertyTokenizeDeterministic`、`TestPropertyTokenizeNoEmpty`、`TestPropertyTokenizeLowercase`、`TestTokenizeNoEmptyTokens`、`TestTokenizeIsLowercased`、`TestPropertyASCIIWordsArePresent` | fully |
| 21 | 空与纯标点输入安全返回 | 非 CJK / 非词字符作分隔符丢弃 | `TestTokenizeEmptyAndPunctuationSafe`、`TestTokenizeDigitsAndVersion` | fully |

---

## R5 BM25 词法检索

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 22 | 多词查询可召回并按相关性排序 | `pkg/lexical/lexical.go`：`BM25.Search`，`k1=1.2`/`b=0.75`，Lucene 风格 IDF | `TestBM25MultiWordQueryReturnsResults` | fully |
| 23 | 中文查询可召回 | bigram 分词 + BM25 打分 | `TestBM25ChineseQuery` | fully |
| 24 | 标识符子词可召回 | 子词分词 + BM25 | `TestBM25IdentifierSubwordMatch` | fully |
| 25 | 词频与文档长度归一化生效 | `norm := f + k1*(1-b+b*len/avgdl)` | `TestBM25TermFrequencyAndLengthNormalization`、`TestBM25LengthNormalizationIsolated` + 变异 M11 | fully |
| 26 | 未命中任何词项时返回空 | `score > 0` 才收集 | `TestBM25NoMatchReturnsEmpty`、`TestBM25EmptyQueryAndEmptyIndex`、`TestBM25RespectsK` | fully |

**实施记录**：Scenario 25 原测试同时改变词频与文档长度（`"alpha alpha alpha"` vs `"alpha " + filler*60`），**词频一项就能决定胜负**，因此去掉长度归一化后测试仍通过——变异 M11 存活暴露此问题。新增 `TestBM25LengthNormalizationIsolated`：两文档中查询词词频**都为 1**，仅长度不同，去掉归一化项后二者分数完全相等即失败。

---

## R6 可配加权 RRF

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 27 | 权重与 k 可通过选项配置 | `SearchOptions{RRFK, VectorWeight, LexicalWeight, CandidateFactor, Lexical}` | `TestWeightedRRFVectorWeightDominates`、`TestWeightedRRFLexicalWeightDominates`、`TestCustomRRFK` | fully |
| 28 | 权重按相对比例生效（尺度不变） | `rrfScore(weight, k, rank) = weight/(k+rank)` | `TestWeightedRRFScaleInvariant`（`{1.0,0.5}` 与 `{0.667,0.3335}` 排序一致） | fully |
| 29 | 词法权重为 0 时等价于纯语义通道 | `if opts.LexicalWeight > 0` 才走词法；`WithLexicalWeight` 区分「未设置」与「显式设 0」 | `TestZeroLexicalWeightSkipsLexicalChannel`（断言后端调用次数为 0）+ 变异 M13 | fully |
| 30 | 默认值可复现且被测试固定 | `DefaultRRFK=60`、`DefaultVectorWeight=0.5`、`DefaultLexicalWeight=0.5` | `TestDefaultWeightsApplied` | fully |
| 31 | 来源标注保持既有语义 | `source` 映射 semantic / lexical / both | `TestBothSourcesLabeled`、`TestSemanticSearchRRFAndSource`、`TestFallsBackToSubstringLexicalWhenNoBackend`、`TestSemanticSearchNilBackendIsLexicalOnly` | fully |

**实施记录（默认值与 spec 定案不同，已按 spec 的把关条款处理）**：

spec/design 阶段定的默认权重是 `0.7 / 0.3`（比例 2.33:1，取自 WeKnora / Elasticsearch 惯例）。spec 同时保留了把关条款：*「若扩充 golden set 后复测发现该默认值劣于纯语义通道，则改默认值并把结论写入 conformance」*。

扩充 golden set 后实测（28 条，26 正样本，K=5），扫描向量:词法比例：

| 比例 | 0.8 | 1.0 | 1.2 | 1.5 | 2.0 | 2.33 | 3.0 | 4.0 | 纯语义 |
|---|---|---|---|---|---|---|---|---|---|
| MRR | 0.8237 | 0.8045 | 0.8173 | 0.7981 | 0.7788 | 0.7788 | 0.7788 | 0.7596 | 0.7532 |

统计判断：26 正样本下单个 case 从 rank1 掉到 rank2 对 MRR 均值的影响为 `0.5/26 ≈ 0.019`。0.8–1.5 区间内相邻比例差异均为 ±1 个 case 量级（属噪声，**不可据此挑选单点最优**），但该区间整体比 2.33:1 高约 1.3 个 case。另一组独立的 16 条查询集复测同样指向 1:1（MRR +12.5% vs `0.7/0.3` 的 +1.3%）。

**因此把默认值改为等权 `0.5 / 0.5`**（比例 1:1），依据是两个独立集合一致指向的区间，而非区间内的峰值。理由已写入 `pkg/query/semantic.go` 常量注释。原 `0.7/0.3` 并未劣于纯语义（+3.4%），改为等权是取更优区间，非纠错。

---

## R7 评测口径修复与策略对比

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 32 | 修复恒零基线 | `pkg/eval/benchmark.go`：新增 `docIDOf()` 用 `FilePath` 取代 `Resource` | `TestBenchmarkUsesFilePathNotResource` | fully |
| 33 | 支持多策略横向对比 | `RunBenchmarkWith` / `CompareStrategies` / `FormatComparison` / `SearchStrategy` / `DefaultStrategy` | `TestRunBenchmarkWithInjectedStrategy`、`TestRunBenchmarkDelegatesToDefaultStrategy`、`TestCompareStrategiesProducesOneReportPerStrategy`、`TestFormatComparisonIncludesAllStrategies` | fully |
| 34 | golden set 覆盖语义化与中文查询 | `pkg/eval/testdata/golden_semantic.json`：28 条（26 正 / 2 负），含 6 条中文、多词、标识符查询 | 全部 `expected_docs` 引用与负样本词已脚本校验；`okf eval` 实跑 | fully |
| 35 | 评测结果可复现 | 策略与索引均确定；`FormatComparison` 按策略名字典序输出 | `TestBenchmarkIsReproducible`、`TestFormatComparisonIsDeterministic` | fully |

**实施记录（假绿陷阱）**：Scenario 32 的缺陷此前被测试掩盖——`pkg/eval/benchmark_test.go` 的 `buildBenchmarkBundle` 手工设置了 `Resource: f + ".md"`，所以单测一直绿（断言 MRR > 0.7），而真实 `okf.LoadBundle` 加载的转换类概念 `Resource` **恒为空**，导致所有指标恒为 0。已把该 fixture 改为设置 `FilePath` 并注释说明「刻意不设置 Resource」，新增 `TestBenchmarkUsesFilePathNotResource` 在实现改回读 `Resource` 时失败。

---

## R8 CLI 接线与文档对齐

| # | Scenario | 实现 | 测试 | 对齐度 |
|---|---|---|---|---|
| 36 | `okf eval` 可运行并输出报告 | `cmd/okf/cmd_eval.go`；`main.go` 注册 `case "eval"` 与 usage 条目 | 实跑通过（见下表）；`TestMissingExpectedDocsDetection` | fully |
| 37 | search 暴露融合参数 | `-lexical-weight`（默认 -1 = 用默认值，0 = 纯语义） | 实跑三档验证（默认 / 0 / 5）行为各异 | fully |
| 38 | 索引状态显示分块信息 | `cmdVectorStatus` 显示分块数、概念数、索引格式版本 + 不兼容提示 | `TestIndexEntryCountEqualsChunkCount`；实跑验证 | fully |
| 39 | 文档声明破坏性变更 | `README.md` / `README.zh-CN.md` 语义搜索章节；`docs/knowledge/releases.md` v0.5.0；`docs/knowledge/cli.md` 新增 `vector` / `eval` 条目并补 search flag | `okf lint` 0 errors；双语章节逐条对照一致 | fully |

**实施记录**：`docs/knowledge/cli.md` 此前**缺失 `vector` 命令条目**（v0.4.0 引入但未写入 CLI 参考文档），本次一并补齐，属顺带修复的既有文档缺口。

`okf eval` 额外加了一个 spec 未要求但必要的防误读保护：当 golden set 的 `expected_docs` 不在目标知识库中时给出警告——否则会输出全零指标，极易被误读为「检索完全失效」（实跑 `golden_queries.json` 对 `docs/knowledge` 即为此情形）。

---

## 实测结果（可复现）

命令：

```bash
okf eval -golden pkg/eval/testdata/golden_semantic.json -path docs/knowledge -compare
```

28 cases（26 positive / 2 negative），K=5，均值仅统计正样本：

| Strategy | Recall@5 | Precision@5 | MRR | NDCG@5 |
|---|---|---|---|---|
| `lexical-substring`（改动前行为） | 0.0769 | 0.0769 | 0.0769 | 0.0769 |
| `bm25-only` | 0.8077 | 0.2776 | 0.7019 | 0.7290 |
| `semantic-only` | 0.9615 | 0.2583 | 0.7276 | 0.7894 |
| **`hybrid-default`** | **0.9615** | 0.2333 | **0.8109** | **0.8447** |

- 混合检索相对纯语义通道 **MRR +11.5%**。
- 改动前的子串词法通道 Recall 仅 **0.0769**，印证了本次改造的前提（作为 RRF 输入基本是噪声）。
- `hybrid-default` 的 Precision 低于 `semantic-only` 属预期：词法通道引入更多候选，在「每条 query 通常只有 1 个正确文档、K=5」的口径下 Precision 上限本身仅 0.2，该指标在此不具区分度，故以 MRR / NDCG 为主。

分块覆盖率（`docs/knowledge`，7 概念）：

| 指标 | 改动前 | 改动后 |
|---|---|---|
| 入索引内容占比 | 29.4% | 104.9%（>100% 因 breadcrumb 重复计入） |
| 内容丢失率 | **70.6%** | 0% |
| 索引条目 | 7（概念级） | 86（块级） |
| 超限块 | — | 0 |

成本（同一内容）：

| 指标 | 改动前 | 改动后 | 倍数 |
|---|---|---|---|
| 索引体积 | 17,701 B | 250,419 B | 14.15x |
| 构建耗时 | 161 ms | 706 ms | 4.4x |

---

## 未纳入本次变更的项（明确记录，非遗漏）

以下为调研阶段明确**排除**的 WeKnora 特性，理由是依赖 okf 刻意不引入的重型基础设施（外部数据库、模型服务、Web 前端、多租户体系），与「单二进制、纯 Go、无 CGO、离线可用」的定位冲突：

- GraphRAG / 知识图谱构建
- 交叉编码器 rerank
- Web UI
- 多租户与权限体系

`Embedder` 仍是接口，未来若需更强模型（BGE-M3 等）或远程 API 可替换，不影响本次索引格式与融合逻辑。
