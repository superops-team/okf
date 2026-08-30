# Review: Vector Semantic Search Change

## 评审范围与依据

- 评审对象：`openspec/changes/add-vector-semantic-search/` 全部 4 个文件（proposal/design/specs/spec/tasks）
- 核查方式：逐文件对照 okf 实际源码（`cmd/okf/main.go` 的 `cmdSearch`/`main` 命令分发、`pkg/query/query.go` 的 `Search`/`SearchWithMatches`/`SearchResult`、`pkg/mcp/tools.go` 工具命名 `okf_*`）与已核实的依赖事实（pure-onnx MIT、coder/hnsw CC0-1.0、ORT/MiniLM 体积来源）交叉核对
- 结论分级：🔴 必须修正（事实错误/设计缺陷/歧义未定案）、🟡 建议优化（可改进）、🟢 通过（符合）

---

## 维度 1：上下文逻辑连贯统一 —— 🔴 1 处已修正

| # | 级别 | 问题 | 证据 | 处置 |
|---|---|---|---|---|
| R1 | 🔴 | spec/design 中 `okf search --semantic "<query>"` 用位置参数写法，与现状不符——`cmdSearch` 实际用 `-q` flag（`fs.String("q", ...)`），且 `-q` 必填校验在 `cmd/okf/main.go:357` | `cmd/okf/main.go:339-360` | 已修正为 `okf search -q "<query>" -semantic`（proposal/design/tasks/spec 同步）；`-semantic` 为布尔 flag，`-q` 沿用既有必填逻辑 |

🟢 通过项：MCP 工具命名 `okf_semantic_search` 与既有 `okf_search`/`okf_query` 风格一致（`pkg/mcp/tools.go`）；`Search`/`SearchWithMatches` 签名核对无误，`SemanticSearch` 作为新函数与其并列，不破坏既有调用。

---

## 维度 3：消除歧义 —— 🔴 6 处已修正

| # | 级别 | 问题 | 处置 |
|---|---|---|---|
| R2 | 🔴 | "content 前 256 token 编码"歧义：是 Go 侧先按字符/词截断，还是交给 tokenizer？pure-onnx 的 minilm 默认 seq 256 会**右截断**，两种实现结果不同 | 定案：拼接 title+description+content 后由 tokenizer 截断至 256 token；spec S3.3、design D4 已统一 |
| R3 | 🔴 | 语义失败处置矛盾：design D8 旧版"非致命错误显式返回"，spec S4.2 旧版"返回可读错误"，且 design 风险表曾提"回退纯词法"——三处语义不一致 | 定案：索引未构建→明确报错（含 `okf vector index` 建议）；语义初始化/检索失败→warning + 回退纯词法（`-verbose` 说明，非静默）；spec S4.2 与 design D8 已统一 |
| R4 | 🔴 | 语义结果类型未定案：扩展既有 `query.SearchResult`（`{Concept, SymbolMatches}`）还是新建类型？ | 定案：扩展 `SearchResult` 增加可选字段 `Source string` / `SemanticScore float32`，向后兼容（spec S4.2 新增 Scenario） |
| R5 | 🔴 | 概念指纹"归一化字符串"未定义归一化规则 | 定案：`lower(type) + ":" + lower(title) + ":" + 绝对化且规范化分隔符的 filepath`（spec S3.3） |
| R6 | 🔴 | 未支持平台"编译期报错"实现方式未定（靠未定义符号报错不可读） | 定案：`ort_unsupported.go` 带 `//go:build !linux && !darwin && !windows`，内用 `compile error` 触发显式 fail（spec S1.2） |
| R7 | 🔴 | `OKF_ORT_DIR` 与 pure-onnx 内置 env（`ONNXRUNTIME_LIB_PATH` 会触发其显式路径模式）交互未说明 | 定案：显式 `SetSharedLibraryPath` 以内嵌/`OKF_ORT_DIR` 为准，README 声明不受外部 `ONNXRUNTIME_LIB_PATH` 影响（spec S1.3） |

---

## 维度 5（测试可先行）/ 维度 17（接线与覆盖）—— 🔴 1 处已修正、🟡 1 处建议

| # | 级别 | 问题 | 处置 |
|---|---|---|---|
| R8 | 🔴 | 测试分层缺位：语义搜索单测若依赖真实 ORT/模型（约 35–40 MB），单测变重、CI 网络依赖强 | 新增「测试分层」Requirement：单测注入 fake/mock `Embedder`（确定性向量）不碰模型；真实模型走 `-tags integration` 与 gauntlet L10（CI 预置资源） |
| R9 | 🟡 | 零网络断言的方法未明确 | spec S1.3 已补：临时 `OKF_ORT_DIR` + 网络不可达环境跑通 + 断言不调用 bootstrap 路径；design 可在实现期补充测试基建细节 |

---

## 维度 7/8：兼容性与破坏性影响 —— 🔴 1 处已修正

| # | 级别 | 问题 | 处置 |
|---|---|---|---|
| R10 | 🔴 | 版本规划缺失：`cmd/okf/main.go` 现为 `Version = "0.3.0"`，功能变更未规划版本号 | 新增 spec Scenario：minor 递增至 **0.4.0**，`const Version` 与 README 同步；此前的版本规则（README-only 不递增）不适用于功能变更 |

🟢 通过项：`-semantic` 为显式开关，不传时 `cmdSearch` 分支不变，默认行为与既有版本完全一致；新增 MCP 工具为增量，不影响既有 `okf_search` 契约。

---

## 维度 6（最小实现）/ 维度 18（全量落地）—— 🟡 2 处建议

| # | 级别 | 问题 | 建议 |
|---|---|---|---|
| R11 | 🟡 | design 风险表"删除触发 rebuild 标记，sync 时统一重建"与 spec P0 增量语义（追加+去重）需对齐 | spec S3.4 已定案 P0=追加+去重、删除/变更走 rebuild；P1 变更检测留后续变更；design 已同步措辞 |
| R12 | 🟡 | MCP `okf_semantic_search` 输出格式未与 `okf_search` 对齐 | 建议复用扩展后 `SearchResult` 的 JSON 结构（含来源标注），实现期在 `pkg/mcp` 接线时保持字段一致 |

---

## 汇总

- 评审发现：🔴 必须修正 9 项（R1–R8、R10），🟡 建议优化 3 项（R9、R11、R12）
- 处置状态：🔴 全部已修正（spec 重写、design/proposal/tasks 同步）；🟡 已纳入 spec 或标注为实现期事项
- 评审后 spec 场景数：8 个 Requirement / 24 个 Scenario（含新增：结果类型向后兼容、测试分层、版本规划）

## 仍留待实现期验证的假设

- pure-onnx v0.0.1 的 `ort`/`minilm` API 以本地 spike 为准（P0 首步验证，失败即暴露）
- MiniLM int8 量化模型在 pure-onnx 下的输出与 fp32 的语义质量差异（P4 性能冒烟记录）
- HNSW（coder/hnsw）对"全量重建为主"的索引更新成本（P4 冒烟）
