## Why

OKF 已经具备从 Git 仓库生成知识库、增量刷新、code knowledge dimension、智能文件导入、watch daemon，以及面向 agent 的 `pkg/tool` 服务内核。但这些能力目前仍存在几个高 ROI 的产品闭环断点：`knowledge_dir` 配置能写但解析链路未闭环，`okf add` 的 smart import 主路径与 archive/validation import 能力分叉，`pkg/tool` 已实现但缺少稳定 CLI 外壳，agent-facing query/context 没有完全复用 `pkg/query` 的结构化索引能力，context snippet 仍偏关键词截取，且缺少多知识源 overlay 与可解释 retrieval trace。

OpenViking 的优势在于统一上下文 URI、分层检索、session-aware query planning 和异步记忆提取。OKF 不应简单复制重型 RAG 架构，而应利用自身 Markdown/YAML/Git-native 的优势，把现有能力收口为一个可审计、可组合、agent-native、repo-native 的知识管理引擎。V1 的目标不是一次性替换 OpenViking，而是用最少代码变更打通 agent 可稳定调用、可测试、可解释的 OKF 知识闭环。

## What Changes

- 完成知识路径 source-of-truth：让 config file 中的 `knowledge_dir` 真正参与解析，并新增 V1 read-only `knowledge_paths[]` overlay。overlay 只影响 query/context/read path，不改变写入目标。
- 统一导入主链路：`okf add` MUST 走统一 dispatcher，支持 file/dir/archive、OKF frontmatter validation、smart import strategy、metadata index 和现有 watch daemon 的一致语义。V1 不新增 daemon，只要求已有 watch daemon 导入时复用同一套校验语义。
- 产品化 agent-native service：新增独立 `okf tool ... --json` CLI 外壳，作为 `pkg/tool.Service` 的薄 wrapper。稳定 JSON envelope 指 `schema_version`、顶层字段名、错误码和已文档化 result 字段语义保持向下兼容；新增可选字段允许，重命名或删除字段必须升级 schema version。
- 强化结构化 query：agent-facing `query` MUST 复用或等价于 `pkg/query` 的结构化索引能力，V1 最小字段为 type、tag、file path、language、symbol kind、qualified name、relation kind 和 source path 导航。
- 强化 context planner：`context` MUST 从“关键词附近 snippet”升级为 token-budget-aware context pack，V1 先实现 symbol line range 优先、同文件 range 合并、确定性 budget packing 和明确 omissions；relation expansion 必须显式开启。
- 新增可解释 retrieval trace：query/context MUST 在显式 `include_trace` 时返回确定性的 trace steps，说明路径解析、候选过滤、排序、context packing、budget omission 和 freshness warning 的来源。

## Scope Locks

- V1 继续保持 deterministic-first：不引入必需向量库、LLM reranker、新 daemon、SQLite、外部服务或 OpenViking runtime。
- Markdown/YAML knowledge files 仍是 source of truth；所有 index/cache/trace 都是 derived 或 response-time artifact。
- `status/query/context/config get/config list/tool status/tool query/tool context` 默认只读；`init/refresh/add/config set/tool init/tool refresh` 才允许写入。
- `knowledge_paths[]` V1 只做 read/query/context overlay；跨路径写入只能落到单一 resolved write target，避免多源写入歧义。没有 `knowledge_paths[]` 时必须保持旧版单 knowledge dir 行为。
- retrieval trace V1 只要求 deterministic explainability，不要求复刻 OpenViking 的完整 visual thinking trace；trace 默认关闭，只能通过 `include_trace=true` 或 `--include-trace` 开启。
- V1 按 M1→M4 分层交付：M1 路径 resolver，M2 统一导入，M3 tool CLI + structured query，M4 context planner + trace。每一层必须可独立测试并保持上一层兼容。

## Capabilities

### New Capabilities

- `knowledge-path-overlay`: 多知识路径解析、source 标注、overlay merge 和单写入目标能力。
- `unified-import-pipeline`: file/dir/archive/validation/smart strategy/watch 共享一致导入语义。
- `agent-tool-cli`: `okf tool ... --json` 作为 agent-native service 的稳定外部入口。
- `structured-agent-query`: agent-facing structured query 与 deterministic ranking 能力。
- `context-planner`: symbol/relation-aware、budget-aware 的上下文包构建能力。
- `retrieval-trace`: query/context 的确定性可解释 trace 能力。

### Modified Capabilities

- `file-import`: 配置解析和导入入口必须与本 change 的路径与统一导入要求一致。
- `agent-native-knowledge-tool`: 现有 `pkg/tool` service 需要增加 CLI wrapper 和更完整 query/context contract。
- `agent-context-retrieval`: context 返回内容需要从 flat snippets 升级为 context plan + packed items + omissions。
- `code-symbol-index`: symbol/relation metadata 需要成为 agent query/context 的一等输入，而不是只存在于 Markdown 内容中。

## Impact

- 主要影响 Go 包：`pkg/okf/config.go`、`pkg/okf/import.go`、`pkg/okf/smart_import.go`、`cmd/okf/cmd_add.go`、`cmd/okf/cmd_config.go`、`cmd/okf/main.go`、`pkg/tool/service.go`、`pkg/query/*`、`pkg/git/code_dimension.go`。
- 优先扩展现有 package。只有当两个以上调用方需要复用同一 seam 时才新增 package；路径 resolver 可落在 `pkg/okf/paths.go`，trace/context planner V1 可先作为 `pkg/tool` 内部类型和 helper，避免过早拆包。
- 需要新增 CLI/golden/integration tests，覆盖 config source、overlay merge、import dispatcher、tool JSON envelope、structured query、context packing 与 trace 输出。
- 不要求修改 OKF concept 基础 frontmatter 字段；如需新增 generated side index，必须可删除并从 Markdown/source 重新构建。
- 兼容风险集中在导入校验变严格、query 排序 tie-break 增加、context response 增加字段和路径解析 source 改变。实现必须以 additive 字段和兼容 wrapper 优先，避免破坏旧 CLI/API 调用。
