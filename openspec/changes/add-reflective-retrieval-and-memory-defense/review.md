# Spec Review: Reflective Retrieval and Memory Defense

实现前做一次 spec review，记录发现的歧义/不可测/过度设计/兼容问题及修复。

## 发现与修复

### R1: redact 与 payload_hash 交互——歧义已消除
- **问题**：PRD 说"先 redact 再算 hash"，但未明确 retry 时第二次 Screen 是否仍对原始 content 操作。
- **修复**：spec MD-08 明确——每次调用都对原始 `req.Content` 做确定性 Screen；同一输入同一输出；retry 时 payload.Content 被替换为相同的 `[REDACTED:...]`，hash 一致。测试 T1.8 端到端验证。

### R2: 文档导入路径是否经过 defense——不阻塞本期但记录
- **问题**：`pkg/convert` → SmartImportSource 是另一条管道，不经过 `WriteKnowledge`。
- **修复判断**：本期 Memory Defense 只覆盖 `WriteKnowledge`（note/event/feedback）。文档导入（PDF/DOCX）是受控的本地导入路径，不是 agent 自动写入；v1 不强制扫描。spec T5.12 记录此行为并测试验证当前不经过 defense。后续 change 可扩展。

### R3: poison gate 规则——deterministic 不依赖 LLM
- **问题**：PRD 开放问题"poison 词表硬编码还是 config"。
- **修复判断**：v1 两层：(a) proposed 状态天然不进 current 视图（已有 temporal 层保证）；(b) 额外可配置 `memory_defense.trap_phrases` 词表，默认空。不引入 LLM judge。spec RF-04 / TE-05 覆盖。

### R4: 中文 PII——明确不支持
- **问题**：v1 是否覆盖中文手机号/身份证？
- **修复判断**：不覆盖。假阳率高。spec T5.13 测试验证正常中文笔记不误杀。后续独立 change。

### R5: extends 递归深度——明确 1 跳
- **问题**：PRD 非目标说"不拉邻居的邻居"。
- **修复**：spec RR-05 明确 depth=1，A extends B, B extends C 时 C 不返回。

### R6: RRF 同分排序——确定性
- **问题**：RRF 融合后同分怎么办？
- **修复**：同分时按 okf_id 字典序排序，保证可重放。spec RF-02 + T5.18。

### R7: block 错误消息不泄露原文——已明确
- **问题**：block 时 remediation 要不要给上下文？
- **修复**：只给 detector ID，不给命中文本。spec MD-09。

### R8: 配置位置——repo-local vs global
- **问题**：`.okf/config.yaml` 是 repo-local 还是 `~/.okf/config.yaml`？
- **修复判断**：repo-local（`<repoRoot>/.okf/config.yaml`），因为策略是 per-repo 的。全局 config 只控 knowledge_dir。design.md 已写明。

### R9: 过度设计检查
- 不引入接口抽象层（Screen 直接是函数）；不引入插件机制；不引入新存储；不引入 worker。
- Trap golden loader 只支持 JSON 一种格式，不做 YAML/TOML。
- RRF 只支持 k=60 常数，不做可配置。

### R10: 兼容性检查
- 未配置 memory_defense 时行为字节级不变（MD-01）。
- 新增 MCP 工具不影响现有工具 schema。
- WriteKnowledge envelope 新增 `redactions` 字段是 omitempty，旧客户端忽略。

## 结论
spec 无阻塞性歧义。所有 Scenario 可测、无过度设计、兼容向后。开始 TDD。
