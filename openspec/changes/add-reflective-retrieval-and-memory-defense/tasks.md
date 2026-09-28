# Tasks: Reflective Retrieval and Memory Defense

P0/P1/P2 仅表依赖顺序，全部完成才算完成。

## P0: Memory Defense（写前扫描）
- [x] T1.1 创建 `pkg/memorydefense/catalog.go`：16 detectors，每个 ID/label/severity/regex 编译期合法
- [x] T1.2 TDD RED：catalog 数量≥16、每 detector 正例命中、正常文本无假阳
- [x] T1.3 TDD GREEN：Screen(content, policy) → (redacted, hits, action)
- [x] T1.4 信用卡 Luhn 校验（spike bug #1 回归测试）
- [x] T1.5 `LoadPolicy(repoRoot)` 读 `.okf/config.yaml` memory_defense 段；缺失/非法测试
- [x] T1.6 WriteKnowledge 接线：normalize 后 Screen，redact 改 payload.Content，block 提前返回
- [x] T1.7 redact 后空内容 → ErrRedactionEmpty
- [x] T1.8 redaction × payload_hash/idempotency_key 端到端测试（同 key 重试命中已有记录）
- [x] T1.9 block 零字节落盘测试（临时文件/目标文件均不存在）
- [x] T1.10 错误不泄露原文测试

## P1: Relation Retrieval + Reflect Workflow
- [x] T2.1 TDD RED：extends 双向邻居召回（spike bug #2 回归：incoming 方向）
- [x] T2.2 TDD GREEN：`pkg/relationrecall`：遍历 view.Entries 收集双向 approved extends
- [x] T2.3 updates 链 head：复用 `TemporalView.History`，标 is_chain_head
- [x] T2.4 proposed/declined/invalid 不进结果测试
- [x] T2.5 cycle/fork/dangling 降级为 warning 测试
- [x] T2.6 depth=1 不递归测试
- [x] T2.7 TDD RED：RRF(k=60) 融合，重叠项排第一，稳定排序
- [x] T2.8 TDD GREEN：`pkg/reflect`：round1 query → poison gate → round2 relation → RRF
- [x] T2.9 need_clarify 弃权（证据 < min_evidence）
- [x] T2.10 max_rounds 硬截断 3
- [x] T2.11 reflect 只读（不写文件）测试

## P2: Trap Evaluation
- [x] T3.1 `pkg/trapeval`：golden case JSON loader
- [x] T3.2 三层打分：answer graded / evidence refs+support / abstention
- [x] T3.3 poison trap：预埋 proposed 恶意记忆，验证不进证据
- [x] T3.4 五类 case_type 分组聚合
- [x] T3.5 poison_blocked < 1.0 → 退出码非零
- [x] T3.6 `okf eval trap` CLI 接线

## Wiring
- [x] T4.1 Service.Reflect / Service.RelationRecall 方法
- [x] T4.2 CLI `okf tool reflect` / `okf tool relation`
- [x] T4.3 MCP `okf_reflect` / `okf_relation_recall`（modern + legacy 双时代）
- [x] T4.4 write 三工具（note/log/feedback）走 defense 的 parity 测试
- [x] T4.5 CLI/MCP parity 测试

## 准出门禁（hindsight 10 类 + spike 17 项 + 6 条新风险）
- [x] T5.1 模式数量下限 ≥16（CI 强制）
- [x] T5.2 每 detector 正例命中 + 正常文本无假阳
- [x] T5.3 redact 原串不落盘
- [x] T5.4 block 零字节落盘
- [x] T5.5 RRF 重叠项排第一
- [x] T5.6 trap 候选被丢弃
- [x] T5.7 proposed 不泄漏
- [x] T5.8 更新链头正确
- [x] T5.9 证据不足弃权
- [x] T5.10 case type 分项计分
- [x] T5.11 redaction × payload_hash/idempotency
- [x] T5.12 文档导入入口行为验证（记录当前是否经过 defense）
- [x] T5.13 中文 PII：v1 不覆盖，测试验证不误杀正常中文
- [x] T5.14 10k 概念延迟/内存 benchmark（真实数字）
- [x] T5.15 配置缺失/非法处理
- [x] T5.16 错误消息不泄密
- [x] T5.17 关系 cycle/fork/dangling/proposed/declined/current-head 全覆盖
- [x] T5.18 RRF 稳定排序/去重
- [x] T5.19 need_clarify 弃权
- [x] T5.20 trap refs/forbidden facts
- [x] T5.21 CLI/MCP parity
- [x] T5.22 真实 Agent E2E（reflect + defense 安全写入）

## 验证
- [x] T6.1 go build / vet / test
- [x] T6.2 gofmt -l 空
- [x] T6.3 staticcheck（可用时）零告警
- [x] T6.4 go test -race
- [x] T6.5 go test -shuffle=on
- [x] T6.6 覆盖率门禁（changed lines ≥80%，全仓 ≥60%）
- [x] T6.7 fuzz/property：regex 解析、redaction 不变量、关系图遍历不变量
- [x] T6.8 mutation 测试（核心逻辑注入 4 缺陷 4/4 杀死）
- [x] T6.9 secret scan / supply-chain
- [x] T6.10 真实 CLI E2E
- [x] T6.11 MCP E2E（python test_mcp.py 扩展）
- [x] T6.12 真实 Codex 多轮 Reflect + Defense 安全写入验证

## 交付物
- [x] T7.1 两轮代码审查并修复
- [x] T7.2 evidence.md（fresh run 真实数字）
- [x] T7.3 conformance.md（Spec↔实现↔测试逐条）
- [x] T7.4 release-notes.md
- [x] T7.5 清理临时文件/worktree 外脏文件
- [x] T7.6 本地 commits 合理拆分，不 push/PR/merge
