# 分层任务拆解

## 任务依赖关系

```
Phase 1 (必须先完成)
└── T-1: MetadataIndex 文件锁与并发安全
    └── T-2: MergeEngine（合并策略核心）

Phase 2 (依赖 Phase 1)
├── T-3: SmartImporter 整合（智能导入编排）
└── T-4: CLI 命令增强（okf add / okf sync）

Phase 3 (依赖 Phase 2)
└── T-5: WatchDaemon 守护进程

Phase 4 (依赖 Phase 3)
└── T-6: 版本迁移与兼容性

Phase 5 (贯穿始终)
└── T-7: 集成测试 + 性能基准测试
```

---

## T-1: MetadataIndex 文件锁与并发安全

**目标**：在 `detect.go` 现有代码基础上，补充文件锁和 SourceExists 字段。

### 子任务

| ID | 任务 | 复杂度 | 依赖 |
|----|------|--------|------|
| T-1.1 | 补充 `SourceExists` 字段到 `FileMetadata` | 低 | 无 |
| T-1.2 | 实现 `LoadIndex` 的文件锁读（`flock`/`LockFileEx`） | 中 | 无 |
| T-1.3 | 实现 `SaveIndex` 的文件锁写 + 版本 CAS 校验 | 高 | T-1.2 |
| T-1.4 | 实现 `MigrateIndex` 版本迁移函数（v0.x → v1.0） | 中 | 无 |
| T-1.5 | 修复 `DetectChange` 中 `DetectMetaChanged` 的处理逻辑（静默更新元数据） | 低 | 无 |
| T-1.6 | 清理 `.metadata.json.tmp` 残留文件的启动检查 | 低 | 无 |
| T-1.7 | Windows 跨盘原子写入 fallback（copy+delete） | 中 | 无 |

**产出**：`detect.go`（增强版）、`detect_lock.go`（文件锁实现）

**验收标准**：
- 多进程同时调用 `SaveIndex` 不发生静默数据覆盖（通过 mock 文件锁测试）
- `DetectMetaChanged` 场景下不调用 MergeEngine，元数据正确更新

---

## T-2: MergeEngine（合并策略核心）

**目标**：实现四种合并策略的完整逻辑。

### 子任务

| ID | 任务 | 复杂度 | 依赖 |
|----|------|--------|------|
| T-2.1 | 实现 `MergeEngine` 接口和 `NewMergeEngine` 构造函数 | 低 | 无 |
| T-2.2 | 实现 `DecideStrategy`（优先级：CLI > 已记录 > 默认 skip） | 低 | 无 |
| T-2.3 | 实现 `DoSkip`：检查目标存在性，返回 Changed=false | 低 | 无 |
| T-2.4 | 实现 `DoOverwrite`：读取源文件 → 原子写入 → 更新元数据 | 低 | 无 |
| T-2.5 | 实现 `DoMerge`：frontmatter 合并（保留目标字段、tags 并集） + 正文保留目标 | 高 | T-2.4 |
| T-2.6 | 实现 `DoPatch`：仅更新指定字段（从源取值） + 正文保留目标 | 高 | T-2.5 |
| T-2.7 | 实现 frontmatter 解析/序列化（复用或扩展 `import.go` 中的解析器） | 中 | 无 |
| T-2.8 | PatchFields 决策优先级实现（WatchRule > 已记录 > 默认） | 低 | 无 |

**产出**：`merge.go`

**验收标准**：
- `DoMerge` 保留目标文件的 type/title/description/resource，正文不变，tags 取并集
- `DoPatch` 仅更新指定字段，其他字段和正文不变
- patchFields 为空时使用默认值 `["title", "description", "tags"]`

---

## T-3: SmartImporter（智能导入编排）

**目标**：将 MetadataIndex、ChangeDetector、MergeEngine 组装为统一的 `SmartImporter`。

### 子任务

| ID | 任务 | 复杂度 | 依赖 |
|----|------|--------|------|
| T-3.1 | 实现 `SmartImportFile` 编排函数（检测 → 策略决策 → 合并 → 元数据更新） | 中 | T-1, T-2 |
| T-3.2 | 实现 `ImportDirectory` 批量导入（串行，逐文件调用 SmartImportFile） | 中 | T-3.1 |
| T-3.3 | 实现 `DetectChanges`（仅检测，返回变更报告，不执行导入） | 低 | T-1 |
| T-3.4 | 实现 `Sync`（扫描索引中所有记录，检测变更并处理） | 中 | T-3.1 |
| T-3.5 | 实现 `--prune`（清理 SourceExists=false 的记录） | 低 | T-3.4 |

**产出**：`smart_import.go`

**验收标准**：
- 单文件导入：检测 → 决策 → 合并 → 持久化元数据，全链路正确
- 目录批量导入：重复文件（不同 source，同一 target）正确报错
- `DetectChanges` 不修改任何文件，只返回报告

---

## T-4: CLI 命令增强

**目标**：将 SmartImporter 集成到 CLI，新增 `okf sync`、`okf watch`（不含 daemon）、`okf metadata`。

### 子任务

| ID | 任务 | 复杂度 | 依赖 |
|----|------|--------|------|
| T-4.1 | 增强 `okf add` 命令：支持 `--strategy`/`--patch-fields`/`--detect-only` | 中 | T-3 |
| T-4.2 | 实现 `okf sync` 命令（基础 sync + --prune） | 中 | T-3.4 |
| T-4.3 | 实现 `okf metadata` 命令（--list/--check/--rebuild/--update） | 中 | T-1, T-3 |
| T-4.4 | 实现 YAML watch 配置解析（`WatchRule`、`WatchConfig`） | 中 | 无 |
| T-4.5 | 命令行帮助文档和错误提示优化 | 低 | 无 |

**产出**：`cmd/okf/cmd_add.go`（增强）、`cmd_sync.go`、`cmd_metadata.go`、`watch_config.go`

**验收标准**：
- `okf add --detect-only` 不修改任何文件
- `okf metadata --rebuild` 生成正确的索引（bySource 为空，带警告）
- `--strategy` 参数验证（非法值给出友好提示）

---

## T-5: WatchDaemon 守护进程

**目标**：实现文件监听、事件去抖和自动导入的 daemon 模式。

### 子任务

| ID | 任务 | 复杂度 | 依赖 |
|----|------|--------|------|
| T-5.1 | 实现 `Debouncer` 接口（基于 timer 的去抖，支持测试注入 fake timer） | 中 | 无 |
| T-5.2 | 实现 `WatchRuleMatcher`（路径前缀匹配 + glob 文件名过滤） | 低 | 无 |
| T-5.3 | 实现单 worker `TaskQueue`（无缓冲 channel，串行处理） | 低 | 无 |
| T-5.4 | 实现 `WatchDaemon.Start`（fsnotify 初始化 + 事件循环 + 信号处理） | 高 | T-5.1, T-5.2, T-5.3 |
| T-5.5 | 实现 `WatchDaemon.ReloadConfig`（SIGHUP 触发） | 低 | T-5.4 |
| T-5.6 | 实现 `WatchDaemon.Stop`（优雅退出 SIGINT/SIGTERM） | 中 | T-5.4 |
| T-5.7 | 实现 pidfile 管理（创建/检测/清理） | 低 | 无 |
| T-5.8 | 实现 inotify 数量检测和 `--no-watch` 轮询 fallback | 中 | 无 |
| T-5.9 | 实现 `okf watch` CLI 命令 | 中 | T-5.7 |
| T-5.10 | symlink 安全策略（默认不跟随） | 低 | 无 |

**产出**：`watch.go`、`daemon.go`、`cmd_watch.go`

**验收标准**：
- 同一文件 500ms 内的多次保存只触发一次导入
- SIGHUP 触发配置热重载，不重启 daemon
- daemon 崩溃后 pidfile 残留能正确清理并重启

---

## T-6: 版本迁移与兼容性

**目标**：确保与旧知识库的兼容，以及未来格式升级的平滑过渡。

### 子任务

| ID | 任务 | 复杂度 | 依赖 |
|----|------|--------|------|
| T-6.1 | 实现 `MigrateIndex(v0.x → v1.0)`：snake_case → camelCase、`last_synced` → `updatedAt`、补全 `bySource` | 中 | T-1.4 |
| T-6.2 | 实现 `RebuildFromKnowledgeBase`：扫描现有知识库文件，计算 hash，重建索引 | 中 | T-3 |
| T-6.3 | 实现 `--config` 指定非默认 `.watch.yaml` 路径 | 低 | T-4.4 |
| T-6.4 | 符号链接安全：`followSymlinks` 字段，默认为 false | 低 | T-5.10 |

**产出**：`migrate.go`

**验收标准**：
- v0.x 格式的 `.metadata.json` 能自动升级到 v1.0
- `okf metadata --rebuild` 正确处理无元数据的旧知识库

---

## T-7: 集成测试 + 性能基准测试

**目标**：确保整体质量达标。

### 子任务

| ID | 任务 | 复杂度 | 依赖 |
|----|------|--------|------|
| T-7.1 | TDD 补充 Phase 2 测试（MergeEngine）：4×10 = 40 个测试用例 | 高 | T-2 |
| T-7.2 | TDD 补充 WatchDaemon 测试（Debouncer fake timer） | 高 | T-5.1 |
| T-7.3 | 集成测试：1000 文件批量导入 | 中 | T-3, T-4 |
| T-7.4 | 集成测试：Watch daemon 运行 1h 内存泄漏检测 | 中 | T-5 |
| T-7.5 | 集成测试：并发 `okf add` + `okf sync` 数据一致性 | 高 | T-4 |
| T-7.6 | E2E 测试：真实开发工作流（编辑 → watch → 验证） | 中 | T-5 |
| T-7.7 | 性能基准测试：5000 文件导入时间验证 | 中 | T-3 |

**验收标准**：见测试规划表格。
