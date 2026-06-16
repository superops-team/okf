# 开发排期规划

## 总体估算

| Phase | 任务块 | 估算工作量 | 累计 |
|-------|--------|-----------|------|
| Phase 1 | MetadataIndex 增强（文件锁 + DetectMetaChanged） | 3 PD | 3 PD |
| Phase 2 | MergeEngine（4 种策略） | 3 PD | 6 PD |
| Phase 3 | SmartImporter + CLI 增强 | 2 PD | 8 PD |
| Phase 4 | WatchDaemon 守护进程 | 4 PD | 12 PD |
| Phase 5 | 版本迁移 + 集成测试 | 2 PD | 14 PD |

> **估算假设**：单个人力，Go 中级开发者，熟悉 fsnotify。总计约 **14 个工作日**（约 2.8 周）。

---

## 详细排期

### 第 1 周（Day 1-5）

| Day | 上午 | 下午 |
|-----|------|------|
| Day 1 | **T-1.1-1.3**：文件锁实现（flock/LockFileEx + CAS） | **T-1.5**：DetectMetaChanged 修复 |
| Day 2 | **T-1.4**：`MigrateIndex` 版本迁移 | **T-1.6-1.7**：tmp 清理 + Windows fallback |
| Day 3 | **T-2.1-2.3**：MergeEngine 接口 + Skip + Overwrite | **T-2.7**：frontmatter 解析器扩展 |
| Day 4 | **T-2.5**：Merge 策略（frontmatter 合并 + 正文保留） | **T-2.6**：Patch 策略（指定字段更新） |
| Day 5 | **T-2.8**：PatchFields 优先级 | **Review + 测试**：补充 `merge_test.go` 覆盖 |

**里程碑**：Phase 1 + Phase 2 完成，MergeEngine 测试全部通过

---

### 第 2 周（Day 6-10）

| Day | 上午 | 下午 |
|-----|------|------|
| Day 6 | **T-3.1**：`SmartImportFile` 编排 | **T-3.2**：ImportDirectory 批量导入 |
| Day 7 | **T-3.3**：DetectChanges | **T-3.4-3.5**：Sync + --prune |
| Day 8 | **T-4.1-4.2**：CLI 增强 okf add + okf sync | **T-4.3**：okf metadata 命令 |
| Day 9 | **T-4.4**：WatchConfig YAML 解析 | **T-4.5**：CLI 帮助文档优化 |
| Day 10 | **Review + 集成测试**：SmartImporter 集成测试 | **Bug Fix + 收尾** |

**里程碑**：Phase 3 完成，所有 CLI 命令可用

---

### 第 3 周（Day 11-15）

| Day | 上午 | 下午 |
|-----|------|------|
| Day 11 | **T-5.1**：`Debouncer` 接口（fake timer） | **T-5.2**：WatchRuleMatcher |
| Day 12 | **T-5.3**：`TaskQueue` 单 worker | **T-5.4-5.6**：WatchDaemon 事件循环 + 信号 |
| Day 13 | **T-5.7**：pidfile 管理 | **T-5.8**：inotify 限制检测 + --no-watch fallback |
| Day 14 | **T-5.9**：okf watch CLI | **T-5.10**：symlink 安全 |
| Day 15 | **Review**：WatchDaemon 集成测试 | **Bug Fix** |

**里程碑**：Phase 4 完成，WatchDaemon 可用

---

### 第 4 周（Day 16-20）

| Day | 上午 | 下午 |
|-----|------|------|
| Day 16 | **T-6.1**：MigrateIndex v0.x → v1.0 | **T-6.2**：RebuildFromKnowledgeBase |
| Day 17 | **T-6.3-6.4**：--config 路径 + symlink 字段 | |
| Day 18 | **T-7.3**：集成测试 1000 文件 | **T-7.4**：内存泄漏测试 |
| Day 19 | **T-7.5-7.6**：并发一致性 + E2E 测试 | **T-7.7**：性能基准测试 |
| Day 20 | **最终 Review**：覆盖率检查 + 全量测试 | **文档 + 收尾** |

**里程碑**：Phase 5 完成，所有测试通过，版本可发布

---

## 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 文件锁跨平台实现复杂度超预期 | 中 | 高 | Unix 用 `flock`，Windows 用 `golang.org/x/sys/windows`；有成熟库可参考 |
| fsnotify 在 macOS 上行为不一致 | 低 | 中 | macOS 使用 FSEvents 包装；充分集成测试覆盖 |
| Merge frontmatter 解析复杂（YAML 边界情况） | 中 | 中 | 复用现有 `import.go` 解析器，先覆盖 90% 场景 |
| Watch daemon 集成测试不稳定（timing 依赖） | 高 | 中 | 使用 fake timer + 确定性事件注入，不依赖真实文件系统 timing |
| 性能目标未达标（5000 文件 < 25s） | 低 | 中 | 第一期使用串行检测；后续按需引入并行优化 |

---

## 交付物清单

| 交付物 | 负责人 | 截止日期 |
|--------|--------|---------|
| `detect.go`（增强版） | - | Day 2 |
| `merge.go` | - | Day 5 |
| `smart_import.go` | - | Day 10 |
| `watch.go` + `daemon.go` | - | Day 15 |
| CLI 命令（add/sync/metadata/watch） | - | Day 15 |
| 单元测试（`*_test.go`） | - | 随每阶段 |
| 集成测试 + E2E | - | Day 20 |
| `SPEC.md` + `TASKS.md` + `TEST_PLAN.md` | - | **已完成** |
