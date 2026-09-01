# Conformance: add-durable-mcp-knowledge

## 结论

- 对齐状态：**fully aligned（S1–S15）**
- 评审范围：`proposal.md`、`design.md`、`specs/mcp-agent-knowledge/spec.md`、`tasks.md`、`pkg/tool`、`pkg/mcp`、`cmd/okf`、`test_mcp.py`
- 边界说明：S10 的正式契约是“两个独立 goroutine 或同一 server request”；实现使用进程内目标路径锁，已通过并发测试和 race detector。跨进程同时写同一 key 未写入本次 Requirement，不据此扩大实现范围。

## Scenario 对齐矩阵

| ID | Scenario | 实现入口 | 自动化证据 | 对齐度 |
|---|---|---|---|---|
| S1 | query 使用统一 service | `pkg/mcp/tools.go` query handler → `tool.Service.Query` | `TestMCPQueryMatchesServiceEnvelope` | aligned |
| S2 | 未初始化 status 无副作用 | `pkg/mcp/tools.go` status handler | `TestMCPStatusUsesServiceWithoutInitializingKnowledge` | aligned |
| S3 | context budget | context handler → `tool.Service.Context` | `TestMCPContextHonorsBudget` | aligned |
| S4 | 绝对 dir | `cmd/okf/main.go` + `tool.Service.resolve` | `TestMCPInitKnowledgeDirResolution/absolute` | aligned |
| S5 | 相对 dir | `cmd/okf/main.go` + `tool.Service.resolve` | `TestMCPInitKnowledgeDirResolution/relative` | aligned |
| S6 | note 跨进程 | `Service.WriteKnowledge` + `okf_ask` | `test_mcp.py` 的双进程 note→restart→ask | aligned |
| S7 | 幂等同内容 | `pkg/tool/write.go` 稳定 identity 与 payload hash | `TestWriteKnowledgeIdempotentAndConflict` | aligned |
| S8 | 幂等冲突 | `pkg/tool/write.go` conflict 分支 | `TestWriteKnowledgeIdempotentAndConflict` | aligned |
| S9 | feedback evidence | feedback handler + provenance fields | `TestWriteFeedbackEvidenceRefs`、`TestMCPWriteToolsPersistAndAskByProject` | aligned |
| S10 | 并发同 key | `knowledgeWriteLock(fullPath)` + 原子 rename | `TestWriteKnowledgeConcurrentSameKey`、`go test -race ./pkg/tool ./pkg/mcp -count=1` | aligned |
| S11 | 保存失败原子性 | persistence seam、原子临时文件、回读校验失败回滚 | `TestWriteKnowledgeCommitFailureLeavesNoHalfWrite`、`TestWriteKnowledgeVerificationFailureRollsBackCommittedFile` | aligned |
| S12 | ask project/limit/顺序 | `okf_ask` → `Service.Query` 类型/project 过滤及确定性排序 | `TestMCPWriteToolsPersistAndAskByProject`、`TestMCPAskHonorsLimitAndDeterministicOrder` | aligned |
| S13 | 路径逃逸 | `validateKnowledgeWriteRoot`、统一 path resolver | `TestMCPWriteRejectsSymlinkKnowledgeRoot`、`TestWriteKnowledgeRejectsSymlinkKnowledgeRoot` | aligned |
| S14 | 非法输入 | MCP strict decode + `normalizeWriteKnowledgeRequest` | `TestMCPWriteRejectsInvalidFieldTypes`、`TestWriteKnowledgeValidationMatrix` | aligned |
| S15 | 旧 MCP 工具回归 | legacy registry 与新 service-backed registry 共存 | `go test ./... -count=1`、`python3 -u test_mcp.py`（17 tools） | aligned |

## Requirement 对齐

| Requirement | 实现状态 | 证据 |
|---|---|---|
| 统一 Service-backed MCP 查询/上下文/状态/刷新/初始化 | fully aligned | handler 复用共享 `*tool.Service`；S1–S5 |
| 持久 note/event/feedback | fully aligned | 稳定 identity、幂等、冲突、provenance、重启查询；S6–S11 |
| ask 复用统一 Query | fully aligned | 类型/project/limit/顺序测试；S12 |
| 写工具 fail closed | fully aligned | strict decode、大小与 credential 校验、symlink 拒绝；S13–S14 |
| 旧 MCP 工具兼容 | fully aligned | 全量 Go 回归与真实 stdio E2E；S15 |
| Wiki 作为普通 Markdown 输入 | fully aligned | 实现未引入 Wiki 特殊状态机；迁移文档声明边界 |

## 新鲜验证证据

2026-08-30 在任务 worktree 执行：

- `go build ./...`：通过。
- `go vet ./...`：通过。
- `go test ./... -count=1`：通过。
- `go test -race ./pkg/tool ./pkg/mcp -count=1`：通过。
- `go build -o okf-bin ./cmd/okf && python3 -u test_mcp.py`：通过；17 个 MCP tools 可见，legacy 工具与 agent-facing 工具均通过，双进程 note 重启后可查询。
- `TestWriteKnowledgeVerificationFailureRollsBackCommittedFile`：RED 时观察到损坏目标文件残留；实现回滚后 GREEN，且查询结果为空。

## 最终仓库级门禁

- `tools/gauntlet.sh`：通过；覆盖 build、vet、gofmt、Go 1.26 兼容 Staticcheck、全仓 race、65% statement coverage（门槛 60%）、shuffle、4/4 mutation killed、真实 CLI import/search。
- framing 修复后 `go test ./pkg/mcp -count=1` 与 `go test -race ./pkg/mcp -count=1`：通过；同时覆盖 newline/Content-Length 回包对称、负 Content-Length 与超过 16 MiB 输入 fail closed。
- UTF-8 派生字段回归：中文长文本标题/描述按 rune 边界安全截断，持久化后保持合法 UTF-8。
- 最终 diff 新增行 secret literal scan：0 命中；临时 `fmt.Print*`/`log.Print*`/`println`/`os.Stdout`/`os.Stderr` scan：0 命中；`git diff --check`：通过。
- 独立 MCP client live probe：在全新 Git repo 中真实注册 17 tools，`okf_note` 创建成功，关闭并重建 Manager 后 `okf_ask` 返回非空结果并命中原正文；最终证据为 独立 client harness 的临时验证证据。

## 判定

S1–S15 的 spec、实现入口和自动化测试均一一对应，无 `partial` 或 `gap`。最终 gauntlet、安全扫描、双 framing 回归和跨独立 MCP client 的 note→restart→ask 均已通过，本 change 达到 fully aligned。
