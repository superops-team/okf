## Why

OKF 当前已经能把 Git 仓库生成 Markdown 知识库，并在 `improve-indexing-correctness` 中补齐增量更新、Go AST 符号索引、轻量关系图和性能基础。但这仍主要是“文件摘要 + 轻量符号列表”：它没有明确把代码仓库作为一个独立知识维度建模，也没有定义与 CodeGraph 这类细粒度代码图系统的兼容边界。

CodeGraph 的核心价值是本地优先的代码知识图：`CodeGraph` facade 负责 init/open/index/search/traverse/context 等能力（`/Users/bytedance/workspace/github/codegraph/src/index.ts:131`），它的 `Node`/`Edge` 类型能表达符号和关系（`/Users/bytedance/workspace/github/codegraph/src/types.ts:110`、`/Users/bytedance/workspace/github/codegraph/src/types.ts:184`），SQLite schema 也显式持久化 nodes、edges、files（`/Users/bytedance/workspace/github/codegraph/src/db/schema.sql:20`、`/Users/bytedance/workspace/github/codegraph/src/db/schema.sql:45`、`/Users/bytedance/workspace/github/codegraph/src/db/schema.sql:59`）。OKF 则以 `Concept` 和 `KnowledgeBundle` 为稳定分发单位（`/Users/bytedance/workspace/github/okf/pkg/okf/types.go:11`、`/Users/bytedance/workspace/github/okf/pkg/okf/types.go:51`），更适合作为人类和 Agent 共享、可 Git diff、可长期沉淀的知识层。

因此需要一个代码知识维度扩展，把 CodeGraph 的细粒度图语义映射进 OKF，而不是把 OKF 改造成数据库或后台索引服务。

## What Changes

- 新增 `code-knowledge-dimension` 能力：把仓库、文件、模块/包、符号、关系和上下文视图作为 OKF 可表达的代码知识维度。
- 新增 CodeGraph 兼容模型：定义 CodeGraph `NodeKind`/`EdgeKind` 到 OKF concept type、frontmatter custom fields、Markdown 章节和关系记录的映射。
- 增加稳定身份规则：代码实体 MUST 使用 repository-relative path、language、kind、qualified name、line range 和 optional content hash 组成可重复生成的 identity。
- 增加机器可读 metadata：在不破坏 OKF 现有 frontmatter 基础字段的前提下，通过 `CustomFields` 或专用 generated manifest 暴露 code entity、code relation、provenance、schema version。
- 增加仓库层视图：生成 package/module index、symbol index、relationship index、hotspot/entrypoint/context slices 等可供 Agent 快速定位的 Markdown 页面。
- 保持源码按需读取：OKF 默认存导航、摘要、签名、docstring、关系和 line range，不默认复制完整源码。

## Capabilities

### New Capabilities

- `code-knowledge-dimension`: 将代码仓库表达为 OKF 的代码维度知识层，覆盖 repository/file/package/symbol/relation/context view。
- `codegraph-compatibility`: 与 CodeGraph 的 Node/Edge/FileRecord/Subgraph/Context 语义兼容，允许未来从 CodeGraph 输出导入或与 CodeGraph 共同使用。

### Modified Capabilities

- `code-symbol-index`: 从“文件内符号提取”扩展为“可被仓库级代码维度引用的符号实体”。该能力的基础需求仍由 `improve-indexing-correctness` 定义，本 change 补充跨文件关系、身份、视图和兼容性。
- `incremental-knowledge-index`: 增量更新需要同步维护代码实体、关系索引和仓库级 generated views，不能只 upsert file concepts。

## Impact

- 主要影响 Go 包：`pkg/git`（提取/关系/增量）、`pkg/okf`（concept custom metadata 使用约定）、`pkg/query`（代码维度搜索和过滤）、CLI 命令（生成/更新/搜索输出）。
- 可能新增内部模型：`CodeEntity`、`CodeRelation`、`CodeKnowledgeIndex`、`CodeGraphMapping`、`GeneratedView`。
- 可能新增 generated concept 类型：`code_repository`、`code_file`、`code_package`、`code_symbol`、`code_relation_index`、`code_context_view`。
- 不要求第一版引入 CodeGraph npm 包、SQLite 依赖或 tree-sitter；兼容性先以数据模型和可导入 schema 为边界。真正直接读取 `.codegraph/` 或 CodeGraph DB 可作为后续 task 实现。
- 不新增后台 daemon、远程服务或强制数据库。

