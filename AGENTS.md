# AGENTS.md — OKF 项目开发规范

## 项目概述

okf 是 Go 实现的 OKF（Open Knowledge Format）v0.2 知识库工具链，包含 CLI（`cmd/okf`）、解析/校验/查询核心（`pkg/parser`、`pkg/lint`、`pkg/query`、`pkg/okf`）、Git 集成（`pkg/git`）、MCP 服务器（`pkg/mcp`）。知识库以 Markdown 概念文件 + frontmatter 组织，遵循「一切皆 .md 概念」心智模型。当前开发分支为 `main`，变更管理走 `openspec/changes/<change-id>/`（SDD）。

## 常用命令

- 构建：`go build ./...`
- 静态检查：`go vet ./...`、`staticcheck ./...`
- 全量测试：`go test ./...`、并发检测 `go test ./... -race`、随机序 `go test -shuffle=on ./...`
- 单测：`go test ./pkg/convert -run TestConvertPDF -v`
- 知识库校验：`okf lint`（`docs/knowledge` 应 0 errors）
- MCP 端到端：`python3 test_mcp.py`
- 环境硬约束：Go `1.26.0`（`toolchain go1.26.7`）；文档转换依赖 downmark **v0.10.0 精确锁定**（go.mod 非浮动）
- **约束栈总入口（合入前置门槛）**：`tools/gauntlet.sh`（build/vet/gofmt/staticcheck/test -race/覆盖率≥60%/shuffle/变异 4-4/真实执行，fail-on-first）

## 开发工作流（SDD → TDD → 覆盖 → 一致性）

任何需求变更必须按以下流程，缺一不可：

1. **SDD**：在 `openspec/changes/<change-id>/` 编写 `proposal.md`/`design.md`/`spec.md`（spec 用 Requirement+Scenario 表述）；`tasks.md` 按 P0-P4 拆解；**P0-P4 仅表依赖顺序，全部完成才算完成**
2. **TDD**：先写测试（red）→ 最小实现（green）→ 重构（refactor）
3. **接线与覆盖**：每个 Requirement 必须有实现入口 + 测试用例；spec 每个 Scenario 映射到测试（覆盖矩阵逐项打勾）；**未接线/未测试的需求不计入完成**
4. **一致性对照**：落地后输出 `conformance.md`（spec ↔ 实现 ↔ 测试 对齐度 fully/aligned/partial/gap），作为合入前置门槛
5. **回归**：`go build/vet/test ./...` + `python3 test_mcp.py` 全绿

## 19 维开发规范（验收标准，全部强制）

1. **上下文逻辑连贯**：改动必须与既有代码路径/主链路一致（先核对实际实现，不凭印象），避免误导实现
2. **杜绝空谈**：每个需求/场景有落地依据与可执行验收点；无据可依的内容不得写入 spec
3. **消除歧义**：默认值/命名/识别优先级/错误类型等必须定案并写入 spec，不留 Open Question
4. **语义精准**：术语、错误类型、边界条件表述无多义，避免大模型/实现者理解偏差
5. **SDD/TDD 适配**：测试可先行（fixtures/golden 先备齐、测试顺序明确），TDD red 阶段不被阻塞
6. **最小化实现**：只改必要文件，不侵入核心。示例：文档导入 = cmd_add 前置转换层，**不改** `CollectFiles`/`SmartImportSource`/`ImportResult`/watch 配置
7. **向下兼容**：Go 版本升级、依赖引入必须评估存量影响并文档化（README/Release Notes 标注最低版本、行为变更）
8. **破坏性影响梳理**：行为变更（如 `okf add <非md>` 由静默跳过变自动转换）必须在 CLI 帮助与 README 声明
9. **风险预判**：临时目录清理（`defer RemoveAll`）、输入超限/转换超时/损坏/扫描文件等异常路径必须有明确处理与测试
10. **可行性评估**：先本地验证核心链路（编译/转换/解析），再定方案；技术不可行及时暴露而非绕过
11. **分层任务/测试/排期**：任务依赖清晰、测试规划完整；排期仅表先后顺序，不构成交付边界
12. **可扩展性**：扩展成本低（如加格式只增映射 + downmark 包）；关键前置决策（如身份/重导入语义）在方案阶段定案
13. **拒绝过度设计**：不引入空转开关、冗余抽象、多余字段/错误类型（用既有机制，如 `Patterns` glob、downmark 错误透传）
14. **小而高效**：用最少代码变更实现核心收益；改动面控制在必要文件
15. **持续优化**：代码逻辑优雅直观，避免绕路与重复
16. **架构统一**：风格一致、不割裂；新增能力走前置管道，不塞进核心导入逻辑
17. **全需求接线与覆盖测试**：每个需求必须有实现函数/入口 + 对应测试；**没有接线和测试的开发不算完成**
18. **排期仅表依赖**：不得只开发 P0 就宣称完成；全量落地、全测试绿、一致性对照通过才算完成
19. **spec-实现一致性对照**：落地后逐条核对 spec Scenario 与实现/测试，输出 `conformance.md`；`partial`/`gap` 必须给出原因与后续任务

## GAUNTLET 约束栈（合入前置门槛，见 `tools/gauntlet.sh`）
所有变更合入前必须通过完整约束栈（一条命令 `tools/gauntlet.sh`，fail-on-first）：
- **L1 类型**：`go build ./...`
- **L2 静态检查**：`go vet ./...` + `gofmt -l`（仅跟踪文件，须空）+ `staticcheck ./...`（零告警）
- **L3 并发**：`go test ./... -race`（零数据竞争）
- **L4 覆盖**：`go test -coverpkg=./... ./...`，全仓语句覆盖 **≥60%**（`COVER_THRESHOLD` 可调），低于阈值即失败
- **L5 套件健康**：`go test -shuffle=on ./...`（无顺序依赖/flaky）
- **L9 变异**：`tools/mutants.sh` 对 `pkg/convert` 核心逻辑注入 4 个缺陷，**4/4 必须被测试杀死**（手动变异；Go 无成熟默认工具）
- **L10 真实执行**：CLI 导入测试文档 + 检索（banana→xlsx、Section One→docx）+ lint 冒烟
## 并发与锁规范
- `KnowledgeBundle` 方法（AddConcept/RemoveConcept/GetConcept/FilterConcepts/Stats 等）**线程安全**（内部 `RWMutex`）；`FilterXxx`/`Search` 等委托 `FilterConcepts` 获得读锁，**不得**在已加锁方法内再叠加锁
- 并发改动必须经 `go test -race` 验证；测试并发场景不得共享可变配置（每 goroutine 独立副本）
## 属性测试（无新依赖）
- 纯函数/序列化不变量用标准库 `testing/quick`（不引入 rapid 等第三方依赖）：`equalFold` 与 `strings.EqualFold` 一致、`sanitizeFilename` 安全且幂等、`WrapConcept` frontmatter 任意标题 round-trip
- 新增文档格式/解析/序列化逻辑时，应配套等价属性测试

## 关键架构约束

- 文档导入（PDF/DOCX/XLSX/PPTX/HTML/CSV/TXT/DOC）走 `pkg/convert` 前置转换层，产物为标准 `.md`（命名 `<原名>.md`），再喂给既有 `SmartImportSource`；纯 Go、不依赖 Python/CGO/外部二进制
- downmark v0.10.0 **精确锁定**（go.mod 非浮动），保证转换输出确定；升级属受控行为变更，需在 Release Notes 声明重导入语义
- 知识库核心模型（Concept/Bundle）与 OKF v0.2 规范不得因功能扩展而改动
- 转换产物 frontmatter 约定：`type: source`，`title` 优先取转换提取标题（空则回退文件名），`description` 用模板 `Converted from <filename> (via <format>)`
- 格式识别优先级：归档 > 文档 > Markdown > 其他；`.md` 不经过转换层
