# Depth diagnosis

## Expert delta

- 原始任务只说“学习当前项目”和“设计专用门禁”，但没有要求先重建当前项目已经存在的门禁事实。OKF 已有 `tools/gauntlet.sh`，直接新增一个抽象门禁会和现有合入门槛重叠，因此深版任务必须先区分“已有重型运行门禁”和“本次应补的轻量准入门禁”。
- 原始任务没有要求识别当前仓库的硬约束来源。OKF 的门禁不能只用通用 Go 项目清单，必须保留 AGENTS 中的 SDD/TDD/conformance、Go 1.26、downmark v0.10.0、OpenSpec 目录和真实执行层。
- 原始任务没有要求可失败证明。项目门禁若只在当前仓库上跑通，无法证明能捕捉漂移；深版任务应增加负控，例如 downmark 版本漂移、缺失 spec、缺失 Scenario 必须失败。
- 原始任务没有要求执行后留下 spec-to-implementation 对照。AGENTS 明确要求 `conformance.md`，所以本任务的执行结果也必须有 scenario 到脚本/验证命令的映射。

## Primary materials

- 仓库级开发约束：`AGENTS.md` 中的项目概述、常用命令、SDD/TDD 流程、19 维规范、GAUNTLET、Go/downmark 约束。
- 当前门禁实现：`tools/gauntlet.sh`、`tools/mutants.sh`、`tools/mutants-agent-discovery.sh`、`tools/verify-agent-discovery.sh`、`tools/verify-real-agent-e2e.sh`。
- 当前项目结构与依赖：`go.mod`、`README.md` 的 Project Structure / Module Reference / Build & Test / Contributing 段落。
- 既有 OpenSpec 正本：`openspec/config.yaml`、`openspec/changes/*/{proposal.md,design.md,tasks.md,conformance.md,specs/*/spec.md}`，优先参考已完成度高的 `add-agent-knowledge-discovery`。
- 当前 Git 工作区事实：`git status --porcelain` 和当前变更文件，而不是历史记忆或聊天推断。

## Methods and tools

- 仓库事实 -> `rg`/`sed`/`nl` 定位 -> 模块、门禁层、约束来源表 -> 决定新门禁只补缺口、不替代 gauntlet。
- 原始任务 -> deepen-task-prompt 四项深度测试 -> 自包含执行提示词 -> 约束本次交付物、证据、负控和拒绝伪造规则。
- OpenSpec 正本 -> 结构抽取 -> `proposal/design/spec/tasks/conformance` -> 保证任务符合仓库 SDD，而不是只给聊天方案。
- 门禁设计 -> Bash 只读脚本 -> 可观察 pass/fail 输出 -> 供 `tools/gauntlet.sh` L0 调用。
- 门禁脚本 -> 临时 fixture + 负控 -> `tools/verify-okf-project-gate.sh` -> 证明门禁能失败，而不仅能通过。

## Independent proof

- `tools/okf-project-gate.sh --change-id add-okf-project-gate` 必须在当前变更上通过。
- `tools/verify-okf-project-gate.sh` 必须通过，并且其中至少包含：有效 fixture 通过、downmark 版本漂移失败、缺失 spec 失败、缺失 Scenario 失败。
- `bash -n` 必须验证新增和修改的 shell 脚本语法。
- `tools/gauntlet.sh` 的 L0 必须能调用项目门禁；如未运行完整 gauntlet，必须报告未运行原因和残余风险，不得宣称完整合入门禁通过。

# Rewritten prompt

```text
在 /Users/bytedance/orca/okf 中执行一个仓库变更任务：学习 OKF 当前项目，然后设计并落地一个 OKF 专用的轻量准入门禁。

目标：
- 不替代现有 `tools/gauntlet.sh` 的重型运行门禁，而是在其前面补一个 OKF 专用、只读、fail-closed 的 L0 门禁。
- 门禁必须验证当前仓库硬约束和当前 OpenSpec 变更完整性：Go/toolchain/downmark 版本、gauntlet 关键层、OpenSpec change 的 proposal/design/spec/tasks/conformance、Requirement+Scenario、任务 P0-P4、conformance 的 scenario-to-verification 映射。
- 产物必须遵循 OKF 的 SDD 流程，新增 `openspec/changes/<change-id>/` 下的 proposal/design/spec/tasks/conformance，并提供实现脚本与验证证据。

范围：
- 只修改本任务需要的 OpenSpec 文档和 tools 脚本。
- 不修改 OKF 核心模型、parser/query/MCP/convert 业务代码。
- 不删除、清理、移动或 unregister 任何 worktree。
- 不创建远程 PR、不 push、不发布、不修改外部系统。

必须先读取和引用的 primary materials：
- `AGENTS.md`
- `go.mod`
- `tools/gauntlet.sh`
- `tools/mutants.sh`
- 至少一个高完整度 OpenSpec 变更，例如 `openspec/changes/add-agent-knowledge-discovery/`
- `README.md` 中项目结构、模块和贡献门禁相关段落

方法：
1. 用 `rg`/`sed`/`nl` 收集当前事实，区分已验证事实、假设和本次非目标。
2. 写出 OpenSpec：`proposal.md`、`design.md`、`specs/okf-project-gate/spec.md`、`tasks.md`、`conformance.md`。
3. 实现 `tools/okf-project-gate.sh`：
   - 默认只检查仓库核心约束；如果传入 `--change-id <id>`，检查该变更的 SDD 完整性。
   - 如果未传 `--change-id`，可从当前 git status 推断正在编辑的 `openspec/changes/<id>/` 并检查；推断不到时只执行核心约束检查。
   - 不写入仓库，不读外部网络，不依赖非标准 Python 包。
4. 实现 `tools/verify-okf-project-gate.sh`：
   - 运行当前变更的正向门禁。
   - 构造临时 fixture，并证明有效 fixture 通过。
   - 构造负控，证明 downmark 版本漂移、缺失 spec、缺失 Scenario 都会失败。
5. 将项目门禁作为 L0 接入 `tools/gauntlet.sh`，但不得削弱任何既有 gauntlet 层。

独立验证：
- 运行 `bash -n tools/okf-project-gate.sh tools/verify-okf-project-gate.sh tools/gauntlet.sh`。
- 运行 `tools/okf-project-gate.sh --change-id add-okf-project-gate`。
- 运行 `tools/verify-okf-project-gate.sh`。
- 如完整 `tools/gauntlet.sh` 未运行或失败，必须报告真实状态和残余风险，不得把局部门禁说成完整合入通过。

输出：
- 最终说明变更文件、门禁覆盖范围、验证命令和结果、未运行项与风险。
- 不得伪造源文件、行号、命令结果、测试结果或验证证据。
```
