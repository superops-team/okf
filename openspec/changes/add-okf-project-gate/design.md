# Design: OKF Project Admission Gate

## 1. 文档状态

| 字段 | 内容 |
|---|---|
| 状态 | proposed |
| 类型 | tooling / gate |
| 日期 | 2026-10-21 |
| 关联项 | 无远程 issue；本地变更 `add-okf-project-gate` |
| 历史关系 | 首次设计；补充现有 `tools/gauntlet.sh`，不替代 |

## 2. 背景、问题与证据

### 2.1 当前问题

OKF 的重型 gauntlet 已经覆盖大量运行时验证，但 SDD 形状、Go/downmark 版本锁定、gauntlet 层存在性目前主要依赖人工记忆和文档阅读。对于 agent 执行的仓库变更，缺少一个足够快、足够项目化的 L0 门禁在昂贵测试前阻断“spec/conformance 不完整”或“关键门禁层被削弱”。

### 2.2 已验证事实

| ID | 类型 | 结论 | 精确证据 | 设计影响 | 验证/关闭方式 |
|---|---|---|---|---|---|
| FACT-1 | 已验证事实 | OKF 是 Go 实现的知识库工具链，核心模块包括 CLI、parser/lint/query/okf/git/mcp。 | `AGENTS.md:5` | 门禁必须按 OKF 仓库形态设计，不使用通用 Web/Node 流程。 | 已验证 |
| FACT-2 | 已验证事实 | 需求变更必须走 `openspec/changes/<change-id>/`，并要求 proposal/design/spec/tasks/conformance。 | `AGENTS.md:22`, `AGENTS.md:25` | 门禁必须检查当前变更的 OpenSpec 形状和一致性对照。 | 已验证 |
| FACT-3 | 已验证事实 | Go 版本为 `1.26.0`，toolchain 为 `go1.26.7`。 | `go.mod:3`, `go.mod:5` | 门禁必须检查版本锁定。 | 已验证 |
| FACT-4 | 已验证事实 | downmark 依赖精确锁定为 `v0.10.0`。 | `go.mod:16`, `AGENTS.md:15`, `AGENTS.md:69` | 门禁必须阻断 downmark 版本漂移。 | 已验证 |
| FACT-5 | 已验证事实 | `tools/gauntlet.sh` 当前包含 build、vet、gofmt、staticcheck、test、race、coverage、shuffle、mutation、real execution。 | `tools/gauntlet.sh:29`, `tools/gauntlet.sh:32`, `tools/gauntlet.sh:35`, `tools/gauntlet.sh:42`, `tools/gauntlet.sh:45`, `tools/gauntlet.sh:51`, `tools/gauntlet.sh:54`, `tools/gauntlet.sh:63`, `tools/gauntlet.sh:129`, `tools/gauntlet.sh:141` | L0 门禁应检查这些关键层仍可定位，但不重复执行它们。 | 已验证 |
| FACT-6 | 已验证事实 | 项目已有高完整度 conformance 矩阵，将 Scenario 映射到实现路径、测试、命令和状态。 | `openspec/changes/add-agent-knowledge-discovery/conformance.md:1` | 新门禁的 conformance 也应提供 scenario-to-verification 映射。 | 已验证 |
| FACT-7 | 已验证事实 | README 的贡献流程要求 SDD、TDD、conformance 和 gauntlet。 | `README.md:561`-`README.md:568` | 门禁语义应与贡献流程一致。 | 已验证 |

### 2.3 假设与待确认项

| ID | 类型 | 结论/假设 | 精确证据 | 设计影响 | 验证/关闭方式 |
|---|---|---|---|---|---|
| ASM-1 | 假设 | 本项目暂不需要解析全部历史 OpenSpec draft 并强制补齐。 | 历史 `openspec/changes/*` 完整度不一致 | 默认只检查当前编辑或显式指定的 change，避免误伤历史草稿。 | 若用户要求全仓治理，再新增 archive/draft 策略。 |

## 3. 目标、非目标与范围

### 3.1 目标

- G1：提供一个只读 L0 门禁，在重型 gauntlet 前快速验证 OKF 仓库核心约束。
- G2：提供 `--change-id` 检查，确保当前 OpenSpec 变更具备 proposal/design/spec/tasks/conformance 和基本可验证结构。
- G3：提供负控验证，证明门禁不仅能通过当前仓库，也能捕捉关键漂移。
- G4：将 L0 门禁接入 `tools/gauntlet.sh`，但不削弱任何既有层。

### 3.2 非目标

- 不运行 Go 全量测试、MCP E2E、real agent E2E 或 mutation；这些仍归现有 gauntlet 和专题 verify 脚本。
- 不强制所有历史 OpenSpec draft 一次性合规。
- 不引入第三方依赖或非标准 Python 包。
- 不修改 OKF 产品运行行为。

### 3.3 范围

范围内：`tools/okf-project-gate.sh`、`tools/verify-okf-project-gate.sh`、`tools/gauntlet.sh` 的 L0 调用、本 change 的 OpenSpec 文档。

范围外：Go 业务包、MCP 协议、Agent 配置逻辑、远程 CI、发布流程。

## 4. 用户、调用方与场景

| ID | 角色 | Given | When | Then |
|---|---|---|---|---|
| SCN-1 | 本地开发者/agent | 正在实现一个 OpenSpec 变更 | 运行 `tools/okf-project-gate.sh --change-id <id>` | 如果 SDD 结构完整且核心约束未漂移，命令退出 0 |
| SCN-2 | 本地开发者/agent | 忘记创建 `specs/*/spec.md` | 运行 change gate | 命令非 0，并指出缺失 spec |
| SCN-3 | 维护者 | 即将运行完整 gauntlet | 运行 `tools/gauntlet.sh` | L0 先执行项目门禁，失败时停止在昂贵测试之前 |
| SCN-4 | 维护者 | 怀疑门禁只会通过不会失败 | 运行 `tools/verify-okf-project-gate.sh` | 正向 fixture 通过，负控漂移失败 |

## 5. 需求与约束

### 5.1 功能需求

| ID | 来源 | 优先级 | 触发/适用范围 | 必须行为或约束 | 可观察结果 |
|---|---|---|---|---|---|
| FR-1 | FACT-3, FACT-4 | 必须 | 每次运行项目门禁 | 校验 Go/toolchain/downmark 精确版本 | 版本漂移时非 0 |
| FR-2 | FACT-5 | 必须 | 每次运行项目门禁 | 校验 gauntlet 关键层仍存在 | 关键层字符串缺失时非 0 |
| FR-3 | FACT-2 | 必须 | `--change-id <id>` 或推断到当前 change | 校验 proposal/design/spec/tasks/conformance 文件存在 | 缺失文件时非 0 |
| FR-4 | FACT-2 | 必须 | change gate | 校验 spec 包含 Requirement 和 Scenario | 缺失时非 0 |
| FR-5 | FACT-2, FACT-6 | 必须 | change gate | 校验 tasks P0-P4 和 conformance 的场景/验证/状态语义 | 缺失时非 0 |
| FR-6 | FACT-5 | 必须 | gauntlet 执行 | gauntlet 在重型层前调用项目门禁 | gauntlet 输出 L0 项目门禁步骤 |
| FR-7 | 用户原始请求 | 必须 | 验证门禁 | 提供正向和负向自验证脚本 | `tools/verify-okf-project-gate.sh` 退出 0 |

### 5.2 非功能约束

| ID | 来源 | 优先级 | 触发/适用范围 | 必须行为或约束 | 可观察结果/度量 |
|---|---|---|---|---|---|
| NFR-1 | 用户原始请求, FACT-5 | 必须 | 门禁脚本 | 只读，不写当前仓库，不访问网络 | 脚本逻辑不包含仓库写入；验证脚本只写临时目录 |
| NFR-2 | FACT-5 | 必须 | 门禁脚本 | 不依赖非标准 Python 包或外部服务 | 可在本机 bash 环境执行 |
| NFR-3 | FACT-5 | 必须 | 门禁输出 | fail-closed，错误定位到文件/约束 | 失败输出 `OKF-GATE-FAILED` 或 `VERIFY-FAILED` |

## 6. 方案设计

### 6.1 方案总览

新增 `tools/okf-project-gate.sh` 作为轻量文本/结构门禁：先检查核心仓库约束，再检查显式或当前编辑的 OpenSpec change。新增 `tools/verify-okf-project-gate.sh` 构造临时 fixture 和负控，证明门禁有失败敏感性。最后在 `tools/gauntlet.sh` 前置调用项目门禁，形成 L0 → L1+ 的执行顺序。

### 6.2 关键流程与不变量

1. 解析参数：`--repo <path>` 默认为当前目录，`--change-id <id>` 可选。
2. 核心检查：
   - `go.mod` 存在；
   - Go/toolchain/downmark 行精确匹配；
   - `tools/gauntlet.sh` 存在，且关键命令字符串可定位。
3. 变更检查：
   - 如果显式传入 `--change-id`，只检查该 ID；
   - 如果未传入，从 `git status --porcelain --untracked-files=all` 推断当前修改中的 `openspec/changes/<id>/`；
   - 推断不到时跳过变更检查，不把历史 draft 当作当前任务。
4. 对每个目标 change 检查：
   - 必需文件存在；
   - 至少一个 `specs/*/spec.md`；
   - spec 包含 Requirement 和 Scenario；
   - tasks 包含 completion contract 与 P0-P4；
   - conformance 包含 Scenario/Verification 和状态枚举；
   - 关键文档不含明显占位符。

不变量：

- 项目门禁不写当前仓库。
- 项目门禁不运行重型测试。
- gauntlet 既有层只增加，不删除或降级。

### 6.3 接口契约

```bash
tools/okf-project-gate.sh [--repo <path>] [--change-id <id>]
tools/verify-okf-project-gate.sh
```

退出码：0 表示通过；非 0 表示 fail-closed。失败输出以 `OKF-GATE-FAILED:` 或 `VERIFY-FAILED:` 开头。

### 6.4 备选方案与取舍

| 方案 | 优点 | 缺点 | 结论 |
|---|---|---|---|
| A. 只写设计文档，不写脚本 | 最小改动 | 无可执行证明，不能成为门禁 | 放弃 |
| B. 将所有检查写入 gauntlet | 执行入口单一 | gauntlet 已重且昂贵，结构失败反馈慢 | 放弃 |
| C. 独立轻量门禁 + gauntlet L0 调用 | 快速失败，可单独运行，有负控 | 只能做结构/文本检查，不替代运行验证 | 采用 |
| D. 全仓扫描所有 OpenSpec draft | 治理彻底 | 历史草稿完整度不一致，容易误伤 | 作为后续治理 |

## 7. 边界、失败与恢复

| 场景 | 处理 | 状态 | 信号 | 恢复 |
|---|---|---|---|---|
| 不在 git 仓库或无 git status | 核心检查继续，变更推断跳过 | 不写入 | 输出 no active change targets | 显式传 `--change-id` |
| 指定 change 不存在 | fail-closed | 不写入 | `missing change directory` | 创建或修正 change ID |
| spec 缺失 Requirement/Scenario | fail-closed | 不写入 | 指出 spec 缺失语义 | 补充 spec |
| 历史 draft 不完整 | 默认不扫描 | 不写入 | 无失败 | 显式指定时才检查 |
| 负控未失败 | verify 脚本 fail | 临时目录清理 | `VERIFY-FAILED` | 修复门禁敏感性 |

## 8. 兼容、迁移与回滚

本变更不修改产品输出、数据格式或 Go API。回滚方式是移除 `tools/gauntlet.sh` 的 L0 调用和新增脚本/文档。由于脚本只读，不产生迁移状态。

## 9. 安全、隐私与权限

门禁只读取仓库内公开源码和临时 fixture，不读取凭证、不输出环境秘密、不访问网络。验证脚本仅写 `mktemp` 目录，并通过 trap 清理。

## 10. 可观测性、发布与运维

本地输出就是主要信号：每个阶段输出 `== ... ==`，失败输出稳定前缀。无生产发布、无灰度、无外部监控。

## 11. 实施计划与影响范围

| 阶段 | 变更边界 | 前置条件 | 完成证据 | 安全停止点 |
|---|---|---|---|---|
| P0 | 项目事实与提示词深化 | 读取 AGENTS/go.mod/gauntlet/OpenSpec | `prompt-rewrite.md` | 可只交付设计，不改脚本 |
| P1 | OpenSpec 文档 | P0 | proposal/design/spec/tasks | 文档可评审 |
| P2 | 门禁脚本 | P1 | `tools/okf-project-gate.sh --change-id add-okf-project-gate` | 脚本未接 gauntlet 前可回滚 |
| P3 | 自验证脚本 | P2 | 正负控通过 | 负控失败前不接入 gauntlet |
| P4 | gauntlet 接入与 conformance | P3 | `bash -n` + conformance | 完整 gauntlet 可按时间单独运行 |

## 12. 验收标准与验证计划

| ID | Given | When | Then | 验证方法/命令 | 环境 |
|---|---|---|---|---|---|
| AC-1 | 当前仓库 | 运行项目门禁 | 核心约束和当前 change 通过 | `tools/okf-project-gate.sh --change-id add-okf-project-gate` | 本地 |
| AC-2 | 新增/修改 shell 脚本 | 运行语法检查 | Bash 语法通过 | `bash -n tools/okf-project-gate.sh tools/verify-okf-project-gate.sh tools/gauntlet.sh` | 本地 |
| AC-3 | 临时有效 fixture | 运行 verify 脚本 | 正向 fixture 通过 | `tools/verify-okf-project-gate.sh` | 本地 |
| AC-4 | 临时 fixture 中 downmark 版本漂移 | 运行 verify 脚本 | 负控被门禁拒绝 | `tools/verify-okf-project-gate.sh` | 本地 |
| AC-5 | 临时 fixture 缺失 spec 或 Scenario | 运行 verify 脚本 | 负控被门禁拒绝 | `tools/verify-okf-project-gate.sh` | 本地 |
| AC-6 | gauntlet 脚本 | 检查文件内容或运行 gauntlet | L0 调用项目门禁且既有层保留 | `tools/okf-project-gate.sh` + `rg "L0 OKF project gate" tools/gauntlet.sh` | 本地 |

## 13. 风险与待确认项

| ID | 触发条件 | 概率/影响 | 缓解措施 | 监测信号 | 兜底/回滚 |
|---|---|---|---|---|---|
| RISK-1 | 历史 draft 被误判为当前变更 | 中/中 | 默认只看 git status 或显式 ID | 门禁输出目标 change 列表 | 放宽推断或显式传 ID |
| RISK-2 | 关键 gauntlet 层改写导致字符串匹配误报 | 低/中 | 匹配命令语义字符串而非行号 | `OKF-GATE-FAILED` 指出缺失命令 | 更新门禁模式并保留负控 |
| RISK-3 | 门禁结构检查过浅 | 中/低 | conformance 记录为 L0 结构门禁，运行行为仍由 gauntlet 证明 | 发现漏检时新增负控 | 扩展 verify 脚本 |

| ID | 问题 | 为什么重要 | 影响的决策 | 确认方式 | 最晚确认阶段 |
|---|---|---|---|---|---|
| OPEN-1 | 是否未来要求所有历史 OpenSpec draft 全量合规 | 会改变默认扫描范围，可能需要批量修旧文档 | 全仓治理策略 | 用户明确授权后另立 change | 后续治理前 |

## 14. 追踪矩阵

| 需求 | 方案位置 | 验收标准 | 验证方法 | 状态 |
|---|---|---|---|---|
| FR-1 | 6.2 核心检查 | AC-1, AC-4 | project gate + verify 负控 | 已覆盖 |
| FR-2 | 6.2 核心检查 | AC-1, AC-6 | project gate + gauntlet 文本检查 | 已覆盖 |
| FR-3 | 6.2 变更检查 | AC-1, AC-5 | project gate + verify 负控 | 已覆盖 |
| FR-4 | 6.2 变更检查 | AC-1, AC-5 | project gate + verify 负控 | 已覆盖 |
| FR-5 | 6.2 变更检查 | AC-1 | project gate | 已覆盖 |
| FR-6 | 6.1/6.2 gauntlet L0 | AC-6 | gauntlet 文件检查；完整 gauntlet 可选运行 | 已覆盖 |
| FR-7 | 6.1/12 验证脚本 | AC-3, AC-4, AC-5 | verify 脚本 | 已覆盖 |
| NFR-1 | 6.2 不变量 / 9 安全 | AC-1, AC-3 | 脚本审查 + verify 临时目录 | 已覆盖 |
| NFR-2 | 6.3 接口契约 | AC-1, AC-3 | 本地 bash 执行 | 已覆盖 |
| NFR-3 | 7 失败与恢复 | AC-4, AC-5 | verify 负控 | 已覆盖 |
