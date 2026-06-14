## 1. P0 模型与兼容边界

- [x] 1.1 定义 `CodeEntity`、`CodeRelation`、`CodeKnowledgeIndex`、`CodeGraphMapping` 内部模型，覆盖 repository/file/package/module/symbol/relation/context view。
- [x] 1.2 定义 stable identity 生成函数，并添加测试覆盖 file、package、function、method、type、无 line range 和路径特殊字符。
- [ ] 1.3 定义 concept type 与 frontmatter custom field 约定：`code_repository`、`code_file`、`code_package`、`code_symbol`、`code_relation_index`、`code_context_view`。
- [x] 1.4 添加 CodeGraph Node/Edge/FileRecord 到 OKF 模型的纯函数 mapping 测试，不依赖 CodeGraph runtime。
- [ ] 1.5 明确 generated vs human-authored concept 判定规则，确保删除/重建只影响 generated artifacts。

## 2. P1 生成代码维度知识库

- [x] 2.1 将现有 `FileSummary`、imports、symbols、parse warnings 转换为 `CodeEntity` 与 `CodeRelation`。
- [x] 2.2 更新 repository generation，生成 `code_file` concepts，包含 source、imports、symbols、relationships、warnings、provenance 稳定章节。
- [x] 2.3 生成 `code_repository` overview，汇总语言分布、文件数量、包/模块列表、符号数量、入口文件候选和 generated artifact manifest。
- [x] 2.4 生成 `code_relation_index`，至少包含 contains/imports/file-owns-symbol/package-owns-symbol 关系，并保证排序确定。
- [ ] 2.5 添加生成测试：小型多文件仓库、Go package、多语言 fallback、空仓库、解析 warning、deterministic output。

## 3. P1 增量更新与派生视图

- [x] 3.1 扩展 incremental update，使 added/modified/deleted 文件同步更新 code entities、relations 和 generated concepts。
- [x] 3.2 删除文件时清理 stale generated symbol/relation records，但保留 human-authored concepts。
- [x] 3.3 每次 update 后重建 repository overview 和 relation index，避免派生视图 stale。
- [ ] 3.4 添加 full regeneration 与多次 incremental update 的代码维度收敛测试。
- [ ] 3.5 CLI verbose 输出新增 code dimension counts：entities created/updated/deleted、relations rebuilt、views regenerated。

## 4. P2 查询与 Agent 上下文

- [x] 4.1 扩展 query index，支持按 `code.language`、`code.file_path`、`code.symbol_kind`、`code.qualified_name`、`code.relation_kind` 过滤。
- [ ] 4.2 更新 `okf search` 输出，符号和关系结果必须显示 `file_path:start_line-end_line` 导航位置。
- [ ] 4.3 增加 context view 构建器：按文件、符号、package、最近变更、relation neighborhood 生成 Agent 可读上下文。
- [ ] 4.4 添加查询测试：符号名、限定名、文件路径、语言过滤、关系过滤、上下文视图排序和截断。

## 5. P2 CodeGraph 兼容导入

- [x] 5.1 定义 CodeGraph-compatible JSON fixture 格式，覆盖 nodes、edges、files、subgraph/context metadata。
- [x] 5.2 实现从 fixture records 到 OKF code dimension 的导入路径，并保留 `external_ids.codegraph`。
- [x] 5.3 支持 CodeGraph richer edges（calls/extends/implements/references/type_of/returns）的保留和展示；OKF 自身无法解析时标记 provenance 为 `codegraph`。
- [x] 5.4 添加兼容测试：unknown node kind、unknown edge kind、unresolved target、重复 node、同名不同文件 symbol、CodeGraph id 改变但 OKF identity 稳定。

## 6. P3 性能、文档与验证

- [ ] 6.1 Benchmark 代码维度生成和 query：约 1k 文件、约 10k symbols、关系索引重建、CodeGraph fixture import。
- [ ] 6.2 增加配置项控制 standalone `code_symbol` 生成策略：none/exported/all。
- [ ] 6.3 更新 README 或 CLI help，说明代码维度生成、查询、增量更新和 CodeGraph 兼容边界。
- [ ] 6.4 跑 `go test ./...`、`go test -race ./...`、`go test -bench=. -benchmem ./...`。
- [ ] 6.5 跑 `openspec validate add-code-knowledge-dimension --strict` 并修复所有规范错误。
