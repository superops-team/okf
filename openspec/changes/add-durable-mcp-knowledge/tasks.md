# Tasks

## 完成门槛

P0–P4 仅表示依赖顺序，全部任务、全部 Scenario 测试、全量门禁和 `conformance.md` 均为本次交付范围。任一 Requirement 无实现入口、任一 Scenario 无执行测试、或 conformance 出现 `partial/gap` 时，本变更不得标记完成。

## Scenario 覆盖矩阵

| ID | Scenario | 计划测试 | 接线点 |
|---|---|---|---|
| S1 | query 使用统一 service | `TestMCPQueryMatchesServiceEnvelope` | `pkg/mcp` query handler → `tool.Service.Query` |
| S2 | 未初始化 status 无副作用 | `TestMCPStatusUsesServiceWithoutInitializingKnowledge` | status handler |
| S3 | context budget | `TestMCPContextHonorsBudget` | context handler |
| S4 | 绝对 dir | `TestMCPInitKnowledgeDirResolution/absolute` | server config + Service.Init |
| S5 | 相对 dir | `TestMCPInitKnowledgeDirResolution/relative` | server config + Service.Init |
| S6 | note 跨进程 | `test_mcp.py::test_note_survives_restart` | WriteKnowledge + ask |
| S7 | 幂等同内容 | `TestWriteKnowledgeIdempotentAndConflict` | WriteKnowledge |
| S8 | 幂等冲突 | `TestWriteKnowledgeIdempotentAndConflict` | WriteKnowledge |
| S9 | feedback evidence | `TestWriteFeedbackEvidenceRefs` | feedback handler |
| S10 | 并发同 key | `TestWriteKnowledgeConcurrentSameKey` | atomic upsert |
| S11 | 保存失败原子性 | `TestWriteKnowledgeCommitFailureLeavesNoHalfWrite` | persistence seam |
| S12 | ask project/limit/顺序 | `TestMCPWriteToolsPersistAndAskByProject` + `TestMCPAskHonorsLimitAndDeterministicOrder` | Service.Query projection |
| S13 | 路径逃逸 | `TestMCPWriteRejectsSymlinkKnowledgeRoot` | resolver |
| S14 | 非法输入 | `TestMCPWriteRejectsInvalidFieldTypes` + `TestWriteKnowledgeValidationMatrix` | strict decoder/validator |
| S15 | 旧 MCP 工具回归 | 既有 Go tests + `test_mcp.py` | legacy registry |

## 1. P0 规范与测试设计

- [x] 1.1 完成十九维 spec review，阻塞项清零。
- [x] 1.2 固化 MCP tool schema、error code、mutating annotation 与 capability mapping。
- [x] 1.3 建立 Scenario→Go/Python 测试覆盖矩阵。

## 2. P1 Service-backed MCP（TDD）

- [x] 2.1 RED：为 status/init/refresh/query/context 添加 MCP handler 失败测试。
- [x] 2.2 GREEN：`ServerConfig` 接收 repo/dir，server 持有共享 `tool.Service`。
- [x] 2.3 GREEN：handlers 仅做 decode→Service→encode。
- [x] 2.4 REFACTOR：统一 schema helper、error envelope 和 tool annotation。
- [x] 2.5 验证：`go test ./pkg/mcp ./pkg/tool ./cmd/okf -count=1`。

## 3. P2 持久知识写入（TDD）

- [x] 3.1 RED：note/log/feedback 正常写入、幂等、冲突、大小上限、未知字段、credential 拒绝测试。
- [x] 3.2 RED：并发同 key、保存失败原子性与跨进程回读测试。
- [x] 3.3 GREEN：实现 `pkg/tool` 的窄 WriteKnowledge 服务与稳定 identity。
- [x] 3.4 GREEN：实现原子 bundle upsert 和回读校验。
- [x] 3.5 GREEN：实现 `okf_note`、`okf_log`、`okf_feedback` handlers。
- [x] 3.6 REFACTOR：复用 Concept validation、tag normalization 和 path resolver。

## 4. P3 Ask 与迁移能力（TDD）

- [x] 4.1 RED：`okf_ask` type/project/limit 与 deterministic order 测试。
- [x] 4.2 GREEN：ask 投影到 `Service.Query`，不调用 legacy contains search。
- [x] 4.3 增加通用 durable capture、普通 Markdown 输入和隔离目录示例。

## 5. P3 CLI 与 E2E

- [x] 5.1 `okf mcp` 增加 `--repo`、`--dir` 并保留 `--bundle`。
- [x] 5.2 RED/GREEN：绝对/相对 dir 的 CLI、tool、MCP 三路径测试。
- [x] 5.3 扩展 `test_mcp.py`，覆盖 initialize→tools/list→init→note→ask→context→restart→ask。
- [x] 5.4 验证旧 MCP 工具不回归。

## 6. P4 文档与一致性

- [x] 6.1 更新 README、MCP tool catalog 和安全边界。
- [x] 6.2 更新相关 canonical spec/architecture 文档。
- [x] 6.3 运行十九维 conformance，生成 `conformance.md`。

## 7. P4 最终门禁

- [x] 7.1 `go test ./... -count=1`。
- [x] 7.2 `go test -race ./pkg/tool ./pkg/mcp`。
- [x] 7.3 `python3 test_mcp.py`。
- [x] 7.4 `bash tools/gauntlet.sh`。
- [x] 7.5 真实 stdio MCP 双进程验证并保存证据。
- [x] 7.6 secret scan、无临时调试输出检查。
