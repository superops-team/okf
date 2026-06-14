## 1. P0 测试安全网

- [x] 1.1 为 `pkg/parser` 添加表驱动测试：frontmatter、无 frontmatter、YAML 错误、缺 title/type、CRLF 分隔符。
- [x] 1.2 为 `pkg/lint` 添加表驱动测试：必填字段、描述长度、type 格式、timestamp、tags、长行、重复 tags、重复 title。
- [x] 1.3 为 `pkg/query` 添加测试：text/type/tag/resource/regex/filter 组合，以及空 bundle 行为。
- [x] 1.4 为 `pkg/okf` 添加测试：NewConcept、Add/Remove/Get、RelatedConcepts、SaveBundle/LoadBundle round trip、重复文件名处理。
- [x] 1.5 为 `pkg/git` 添加测试夹具：临时 Git repo、tracked/untracked 文件、include/exclude/max-size、deleted file 场景。
- [x] 1.6 为 CLI 添加 smoke test：在临时 repo 中运行 `init`、`lint`、`search`、`update` 并断言退出码和关键输出。

## 2. P0 正确性修复

- [x] 2.1 修复 `ShouldInclude`，让 `Config.IncludeFiles` 按 glob 生效，同时保留 exclude dirs 和 max file size 规则。
- [x] 2.2 统一 `SaveKnowledgeBase` 序列化路径，删除 `pkg/git/generator.go` 内重复 `serialize` 或改为调用 `parser.SerializeConcept`。
- [x] 2.3 修正 Git 命令执行目录，确保 `AnalyzeFile` 中的 `git log` 等命令使用 repo root 作为 `cmd.Dir`。
- [x] 2.4 让 `AnalyzeFile` 读取文件内容并填充 `Imports`、`Functions`，先复用现有 fallback extractor。
- [x] 2.5 更新 `conceptFromSummary`，在 concept body 中输出 imports/functions 结构化章节。
- [x] 2.6 跑 `go test ./...`，确保 P0 测试和正确性修复通过。

## 3. P1 增量更新正确性

- [x] 3.1 设计并实现 `.okf/state.json`，记录 last indexed commit、schema version、updated time。
- [x] 3.2 实现从 last indexed commit 到 HEAD 的 changed files 计算，覆盖多 commit added/modified/deleted。
- [x] 3.3 修改 `UpdateBundle` 或新增 update service：加载现有 bundle，以 source file path 为键 upsert modified/added concepts。
- [x] 3.4 实现 deleted source file 的 stale generated concept 删除，避免误删用户手写 concept。
- [x] 3.5 修改 `cmdUpdate`，让 `okf update` 真正保存 bundle 和 state，并输出 created/updated/deleted/skipped 数量。
- [x] 3.6 添加增量收敛测试：full regeneration 与多次 incremental update 的结果等价。

## 4. P2 Go AST 符号索引

- [x] 4.1 定义 symbol model：kind、name、package、receiver、exported、file path、start/end line、parse warnings。
- [x] 4.2 实现 Go AST analyzer，提取 package、imports、functions、methods、structs、interfaces、type aliases。
- [x] 4.3 将 Go analyzer 接入 `AnalyzeFile`，`.go` 文件优先使用 AST，解析失败时记录 warning 并安全降级。
- [x] 4.4 更新 concept body，输出 package、imports、symbols、warnings 和 `file:start-end` 导航位置。
- [x] 4.5 添加 symbol search 或 query 适配，使搜索符号名时能返回 kind 与源码位置。
- [x] 4.6 添加 Go AST 测试：普通函数、方法、指针 receiver、struct、interface、type alias、exported/unexported、语法错误。

## 5. P2 轻量关系图

- [x] 5.1 定义 relationship model：file imports、package imports、file owns symbol、package owns symbol。
- [x] 5.2 在 generation/update 过程中构建关系记录，并保证输出排序确定。
- [x] 5.3 将关系图写入知识库的 project/system concept，或写入独立 generated concept 文件。
- [x] 5.4 添加关系图测试：多 package Go repo、相对文件路径、重复 imports、删除文件后的关系清理。

## 6. P3 高效索引

- [x] 6.1 将静态 regex patterns 预编译，移出 per-line hot loop。
- [x] 6.2 实现批量 Git metadata 获取，替代每文件多次 `git log` 子进程。
- [x] 6.3 为文件分析引入 bounded worker pool，并确保结果按 path 排序后写入。
- [x] 6.4 为 bundle/query 构建 type/tag/resource/title/symbol 内存索引，保持 free-text 搜索语义不变。
- [x] 6.5 添加 benchmark：小型 repo、约 1k 文件 repo、query indexed path、query free-text path、save/load round trip。
- [x] 6.6 跑 `go test -race ./...` 和 `go test -bench=. -benchmem ./...`，记录 baseline 输出供后续 benchstat 比较。

## 7. 收尾验证

- [x] 7.1 跑 `go test ./...`，所有包必须通过且不再显示 0 测试文件。
- [x] 7.2 跑 `go build ./...`，确保 CLI 和 library 编译通过。
- [ ] 7.3 在临时 Git repo 手工验证 `okf init`、修改/新增/删除文件、`okf update -verbose`、`okf search`。
- [ ] 7.4 更新 README 中与增量更新、符号索引、测试/benchmark 相关的用户说明。
- [x] 7.5 运行 `openspec validate improve-indexing-correctness --strict` 并修复所有规范错误。
