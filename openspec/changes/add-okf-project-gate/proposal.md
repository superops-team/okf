# Proposal: OKF Project Admission Gate

## Summary

为 OKF 增加一个轻量、只读、fail-closed 的项目专用准入门禁 `tools/okf-project-gate.sh`，并用 `tools/verify-okf-project-gate.sh` 提供正向和负向证明。该门禁不替代现有 `tools/gauntlet.sh` 的重型运行验证，而是在 gauntlet 前作为 L0 检查：

1. 当前仓库硬约束是否仍成立：Go/toolchain/downmark 版本、gauntlet 关键层仍存在。
2. 当前正在编辑的 OpenSpec 变更是否满足 OKF 的 SDD 最小形状：`proposal.md`、`design.md`、`specs/*/spec.md`、`tasks.md`、`conformance.md`。
3. spec 是否使用 `Requirement` + `Scenario`，任务是否按 P0-P4 表达依赖顺序，conformance 是否提供场景到验证的映射。

## Motivation

OKF 已经有很强的运行门禁：`tools/gauntlet.sh` 执行 build/vet/gofmt/staticcheck/test/race/coverage/shuffle/property/secret/mod-verify/mutation/real-execution。它擅长证明“代码运行行为”正确，但没有一个轻量入口在重型测试前快速阻断以下项目特有漂移：

- Go 1.26 与 downmark v0.10.0 锁定被无意改动；
- gauntlet 脚本关键层被删弱；
- 当前 OpenSpec 变更缺少 spec、conformance 或可执行验证映射；
- agent 只写聊天方案或单个脚本，绕过 OKF 的 SDD 完成边界。

本变更把这些仓库约束前置成可执行门禁，减少“跑了很多测试才发现 SDD 形状不完整”的反馈延迟。

## Requirements

### MUST

- 门禁必须只读，不修改仓库、不创建持久文件、不访问网络。
- 门禁必须验证 `go.mod` 中 `go 1.26.0`、`toolchain go1.26.7`、`github.com/giraffesyo/downmark v0.10.0`。
- 门禁必须验证 `tools/gauntlet.sh` 仍包含 build/vet/gofmt/staticcheck/test/race/coverage/shuffle/mutation/real execution 的关键命令。
- 门禁必须支持 `--change-id <id>`，并检查 `openspec/changes/<id>/` 下的 `proposal.md`、`design.md`、`tasks.md`、`conformance.md` 与至少一个 `specs/*/spec.md`。
- 门禁必须要求 spec 中出现 `## Requirement:` 和 `### Scenario`。
- 门禁必须要求 `tasks.md` 包含 P0-P4 依赖阶段和 completion contract。
- 门禁必须要求 `conformance.md` 包含 Scenario/Verification 语义和 `fully|aligned|partial|gap` 状态。
- 门禁必须提供验证脚本，证明有效 fixture 通过，且 downmark 版本漂移、缺失 spec、缺失 Scenario 会失败。
- `tools/gauntlet.sh` 必须在重型层之前调用该项目门禁，且不得移除或弱化既有层。

### SHOULD

- 未传 `--change-id` 时，门禁应从 `git status --porcelain` 推断当前正在编辑的 `openspec/changes/<id>/` 并检查；推断不到时只执行仓库核心约束。
- 门禁输出应短小、定位明确，便于 agent 和人类快速修复。
- 验证脚本应使用临时目录构造 fixture，避免污染当前仓库。

### MAY

- 后续可扩展到更严格的 scenario-to-test 解析、OpenSpec YAML schema 校验、README/中文文档同步检查。
- 后续可把完整门禁结果记录到 change evidence 文件，但本变更不引入证据写入机制。

## Non-Goals

- 不替代 `tools/gauntlet.sh`。
- 不运行 Go 全量测试、mutation、真实 MCP/Agent E2E；这些仍由 gauntlet 和专用 verify 脚本承担。
- 不修改 OKF 核心模型、parser/query/convert/MCP 业务代码。
- 不扫描或强制修复历史上所有 OpenSpec draft；只检查显式指定或当前正在编辑的变更。
- 不创建远程 CI 配置、提交、推送或发布。

## Dependency Order and Completion Boundary

`P0 项目事实与提示词深化 → P1 SDD 文档 → P2 项目门禁脚本 → P3 自验证与负控 → P4 gauntlet 接入与一致性对照`。

P0-P4 仅表示依赖顺序。只有 OpenSpec 文档、门禁脚本、自验证脚本、gauntlet L0 接入、conformance 和本地验证全部完成后，本变更才可宣称完成。

## Expected Impact

- 新增 `tools/okf-project-gate.sh`。
- 新增 `tools/verify-okf-project-gate.sh`。
- 修改 `tools/gauntlet.sh`，增加 L0 项目门禁调用。
- 新增 `openspec/changes/add-okf-project-gate/` 设计与一致性材料。

## Risks

- **误伤历史 draft**：默认只检查 git status 中正在编辑的变更或显式 `--change-id`，不扫描全部历史 draft。
- **门禁过弱**：本变更用负控证明关键失败可被捕捉；更深的 semantic scenario-to-test 覆盖作为后续扩展。
- **门禁过慢**：脚本只做文本和结构检查，不运行 Go 测试，保持在重型 gauntlet 前快速失败。
- **本地/CI 基线差异**：脚本不依赖非标准 Python 包、不访问网络，降低环境漂移。
