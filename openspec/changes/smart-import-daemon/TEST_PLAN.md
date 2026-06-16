# 测试规划

## 测试分层策略

```
┌─────────────────────────────────────────────────────┐
│                  E2E 测试 (manual/ci)                │
│   真实工作流: okf add → watch → edit → verify       │
├─────────────────────────────────────────────────────┤
│               集成测试 (pkg/okf + cmd/okf)          │
│   批量导入、并发安全、daemon 运行、崩溃恢复          │
├─────────────────────────────────────────────────────┤
│                单元测试 (每个模块)                   │
│   Mock 文件系统/时间/锁，确保快速反馈               │
└─────────────────────────────────────────────────────┘
```

---

## Phase 1 单元测试（已完成 ✅）

**文件**：`pkg/okf/detect_test.go`

| 测试用例 | 覆盖场景 |
|---------|---------|
| `TestFileMetadata_Basic` | 结构体字段赋值 |
| `TestComputeFileHash` | 相同内容相同 hash，不同内容不同 hash |
| `TestComputeFileHash_NotExist` | 不存在文件返回错误 |
| `TestMetadataIndex_New` | 空索引初始化 |
| `TestMetadataIndex_AddAndGet` | 按 TargetPath 和 SourcePath 增删查 |
| `TestMetadataIndex_AddDuplicateTarget` | 重复 TargetPath 拒绝写入 |
| `TestMetadataIndex_Update` | 更新现有记录 |
| `TestMetadataIndex_Delete` | 按 TargetPath 删除，同时清理 BySource |
| `TestMetadataIndex_SaveAndLoad` | 完整持久化 + 加载验证 |
| `TestMetadataIndex_LoadEmpty` | 文件不存在返回空索引 |
| `TestMetadataIndex_LoadCorrupted` | 损坏 JSON 返回错误 |
| `TestMetadataIndex_SaveCreatesDir` | 自动创建嵌套目录 |
| `TestDetectChanges_FastPath_NoChange` | mtime+size 一致 → DetectNoChange |
| `TestDetectChanges_SlowPath_ContentChanged` | mtime+size 变化且 hash 变化 |
| `TestDetectChanges_SlowPath_MetaChangedOnly` | mtime 变化但 hash 不变 |
| `TestDetectChanges_NewFile` | 未在索引中的文件 |
| `TestDetectChanges_SourceMissing` | 在索引中但磁盘不存在 |
| `TestDefaultKnowledgeDir` | 跨平台路径 |
| `TestAtomicWriteFile` | 原子写入 + 无 .tmp 残留 |
| `TestAtomicWriteFile_Overwrite` | 覆盖已有文件 |

**覆盖率**：`detect.go` 函数覆盖率 ≥ 90%

---

## Phase 2 单元测试（MergeEngine）

**文件**：`pkg/okf/merge_test.go`（待创建）

| 测试用例 | 覆盖场景 |
|---------|---------|
| `TestMergeEngine_DecideStrategy` | CLI > 已记录 > 默认 的优先级 |
| `TestDoSkip_TargetExists` | 目标存在 → Changed=false |
| `TestDoSkip_TargetNotExists` | 目标不存在 → 导入新文件 |
| `TestDoOverwrite_Basic` | 覆盖写入 + hash 更新 |
| `TestDoMerge_ConflictingFields` | type/title 保留目标，tags 并集 |
| `TestDoMerge_NoConflict` | 源有额外字段时复制 |
| `TestDoMerge_BodyPreserved` | 正文内容始终保留目标 |
| `TestDoMerge_NoFrontmatter` | 无 frontmatter 的源/目标文件 |
| `TestDoPatch_SpecificFields` | 仅更新指定字段，其他不变 |
| `TestDoPatch_EmptyFields` | patchFields 为空使用默认值 |
| `TestDoPatch_BodyPreserved` | patch 不影响正文 |
| `TestFrontmatterParser_Basic` | 标准 YAML frontmatter |
| `TestFrontmatterParser_NoFrontmatter` | 无 frontmatter 的文件 |
| `TestFrontmatterParser_Multiline` | 多行字符串和特殊字符 |

**覆盖率**：`merge.go` 函数覆盖率 ≥ 95%

---

## Phase 3 单元测试（WatchDaemon）

**文件**：`pkg/okf/watch_test.go`（待创建）

| 测试用例 | 覆盖场景 |
|---------|---------|
| `TestDebouncer_Debounce` | 500ms 内的多次调用只触发一次 |
| `TestDebouncer_DifferentKeys` | 不同 key 独立去抖 |
| `TestDebouncer_FakeTimer` | 使用 fake timer 推进时间 |
| `TestRuleMatcher_Prefix` | 路径前缀匹配 |
| `TestRuleMatcher_Glob` | glob 模式过滤 |
| `TestRuleMatcher_Recursive` | 递归规则 |
| `TestRuleMatcher_NoMatch` | 不匹配时返回 nil |
| `TestTaskQueue_Serial` | 任务串行执行 |
| `TestTaskQueue_PanicRecover` | 任务 panic 不影响队列 |

**覆盖率**：`watch.go`/`daemon.go` 函数覆盖率 ≥ 85%

---

## Phase 4 单元测试（SmartImporter + CLI）

**文件**：`pkg/okf/smart_import_test.go`（待创建）、`cmd/okf/cmd_add_test.go`（扩展）

| 测试用例 | 覆盖场景 |
|---------|---------|
| `TestSmartImportFile_NewFile` | 新文件导入 |
| `TestSmartImportFile_NoChange` | DetectNoChange 跳过 |
| `TestSmartImportFile_MetaChanged` | DetectMetaChanged 静默更新元数据 |
| `TestSmartImportFile_ContentChanged_Skip` | 策略 skip → 不覆盖 |
| `TestSmartImportFile_ContentChanged_Overwrite` | 策略 overwrite → 覆盖 |
| `TestSmartImportFile_ContentChanged_Merge` | 策略 merge → frontmatter 合并 |
| `TestSmartImportFile_ContentChanged_Patch` | 策略 patch → 指定字段更新 |
| `TestSmartImportFile_SourceMissing` | SourceExists=false |
| `TestImportDirectory_DuplicateTarget` | 不同 source 同 target → 报错 |
| `TestImportDirectory_Batch` | 目录批量导入统计正确 |
| `TestDetectChanges_OnlyReports` | 不修改任何文件 |
| `TestSync_PruneMissing` | --prune 清理 SourceExists=false |
| `TestRebuild_EmptyBySource` | 重建索引 bySource 为空 + 警告 |

**CLI 测试扩展**：

| 测试用例 | 覆盖场景 |
|---------|---------|
| `TestAddCommand_Strategy` | 各策略参数正确传递 |
| `TestAddCommand_DetectOnly` | --detect-only 不写文件 |
| `TestAddCommand_InvalidStrategy` | 无效策略给出友好错误 |
| `TestSyncCommand_Prune` | --prune 参数 |
| `TestMetadataCommand_List` | --list 输出格式 |
| `TestMetadataCommand_Rebuild` | --rebuild 流程 |

---

## 集成测试

**文件**：`pkg/okf/integration_test.go`（待创建）

| 场景 | 验证目标 |
|------|---------|
| `TestIntegration_1000Files` | 内存 < 50MB，时间 < 5s |
| `TestIntegration_WatchDaemon_Memory` | 运行 1h 无内存泄漏 |
| `TestIntegration_ConcurrentAddSync` | 并发写入数据一致性 |
| `TestIntegration_DiskFull` | 磁盘满时优雅降级 |
| `TestIntegration_PermissionDenied` | 权限错误友好提示 |
| `TestIntegration_WindowsCrossVolume` | Windows 跨盘写入 |
| `TestIntegration_PidfileCleanup` | 崩溃后 pidfile 清理 |

---

## E2E 测试

**文件**：`scripts/e2e-smart-import.sh`（待创建）

```bash
# 场景: 真实开发工作流
setup() { ... }

test "初始导入" okf add ./docs --dir /tmp/kb
test "无变更跳过" okf add ./docs --dir /tmp/kb
test "内容变更 overwrite" echo "new" >> docs/api.md && okf add ./docs --dir /tmp/kb
test "检测模式" okf add ./docs --dir /tmp/kb --detect-only
test "Watch daemon 监听" okf watch --dir /tmp/kb &
test "文件变更触发 watch" echo "watched" >> docs/new.md && sleep 2 && verify new.md in /tmp/kb
cleanup
```

---

## 覆盖率目标

| 文件 | 行覆盖率目标 | 函数覆盖率目标 |
|------|------------|-------------|
| `detect.go` | ≥ 90% | ≥ 95% |
| `merge.go` | ≥ 95% | ≥ 98% |
| `smart_import.go` | ≥ 90% | ≥ 95% |
| `watch.go` | ≥ 85% | ≥ 90% |
| `daemon.go` | ≥ 80% | ≥ 85% |
| CLI (`cmd/okf/`) | ≥ 85% | ≥ 90% |
