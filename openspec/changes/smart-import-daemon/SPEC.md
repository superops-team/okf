# Smart Import & Watch Daemon - 功能规格

> **版本**：v1.1（修订版）
> **状态**：待评审
> **变更说明**：基于 v1.0 进行了精简（移除 `.patches/` 过度设计）、歧义修正（Merge/Patch 正文处理、DetectMetadataChanged 流程）、并发安全补充（文件锁）、兼容隐患修复。

## 功能概述

扩展现有文件导入能力，支持智能检测、内容合并和后台守护模式自动更新。

### 核心能力

1. **智能文件检测**：基于内容 hash + 文件元数据判断是否需要更新
2. **四向合并策略**：支持 `skip` / `overwrite` / `merge` / `patch` 四种策略
3. **后台 Watch daemon**：持续监听源文件变化，自动更新知识库
4. **动态 Watch 规则**：支持运行时配置监听规则和导入策略

### 设计原则

- **零配置可用**：开箱即用，默认使用保守的安全策略
- **高性能**：避免重复扫描和全量比较，使用增量检测
- **稳定性**：原子化写入，失败自动回滚，保证知识库一致性
- **可观测**：完善的日志，便于排障
- **最小化实现**：优先小而高效的改动，不做过度设计

---

## 架构设计

### 核心组件

| 组件 | 职责 |
|------|------|
| MetadataManager | 维护文件元数据索引，提供并发安全的 CRUD |
| ChangeDetector | 双层变更检测（FAST PATH mtime+size / SLOW PATH SHA-256） |
| MergeEngine | 执行四种合并策略 |
| WatchDaemon | fsnotify 监听 + 去抖 + 任务队列 |
| AtomicWriter | 先写 .tmp 再 rename，保证写入原子性 |

### 知识库目录结构

```
knowledge/
├── files/                      # 知识文件（现有）
├── .metadata.json              # 文件元数据索引（原子读写）
└── .watch.yaml                 # Watch 规则配置（可动态修改）
```

> **注意**：第一期不实现 `.patches/` 目录。版本历史由 git 管理，不额外存储 patch diff。

---

## 数据模型

### MetadataIndex（元数据索引）

```go
type MetadataIndex struct {
    Version   string                  `json:"version"`    // 格式版本，当前 "1.0"
    UpdatedAt time.Time               `json:"updatedAt"`  // 索引最后更新时间
    Files     map[string]*FileMetadata `json:"files"`      // key: TargetPath（相对路径）
    BySource  map[string]string       `json:"bySource"`    // SourcePath → TargetPath 反向索引
}
```

### FileMetadata（文件元数据）

```go
type FileMetadata struct {
    SourcePath    string        `json:"sourcePath"`    // 源文件绝对路径
    TargetPath    string        `json:"targetPath"`    // 知识库相对路径
    ContentHash   string        `json:"contentHash"`   // 文件内容 SHA-256（hex 小写）
    LastModified  time.Time     `json:"lastModified"`  // 源文件 mtime
    LastImported  time.Time     `json:"lastImported"`  // 最后一次成功导入时间
    FileSize      int64         `json:"fileSize"`      // 源文件大小（字节）
    Strategy      MergeStrategy `json:"strategy"`      // skip|overwrite|merge|patch
    PatchFields   []string      `json:"patchFields,omitempty"` // patch 策略下更新的字段
    SourceExists  bool          `json:"sourceExists"`  // 源文件是否存在于磁盘
}
```

> **字段说明**：
> - 第一期**不包含** `frontmatterHash`：merge/patch 策略基于内容 hash 决策，不需要单独的 frontmatter hash。
> - `SourceExists`：当 `DetectSourceMissing` 时设为 false，由 `okf sync --prune` 清理。

### DetectResult（变更检测结果）

```go
type DetectResult int

const (
    DetectNoChange       DetectResult = iota // mtime+size 一致 → 无操作
    DetectNewFile                            // 未在索引中 → 新文件导入
    DetectMetaChanged                        // mtime/size 变但 hash 不变 → 静默更新元数据
    DetectContentChanged                      // hash 不同 → 交给 Merge Engine
    DetectSourceMissing                      // 源文件不存在 → 标记 SourceExists=false
)
```

### MergeStrategy（合并策略，string 类型）

```go
type MergeStrategy string

const (
    StrategySkip       MergeStrategy = "skip"       // 跳过已存在的目标文件
    StrategyOverwrite  MergeStrategy = "overwrite"  // 用源文件覆盖目标
    StrategyMerge      MergeStrategy = "merge"      // 合并 frontmatter，保留正文
    StrategyPatch      MergeStrategy = "patch"      // 仅更新指定的 frontmatter 字段
)
```

### MergeResult（合并结果）

```go
type MergeResult struct {
    Strategy    MergeStrategy `json:"strategy"`
    Content     []byte        `json:"-"`             // 合并后内容（不序列化到 JSON）
    Changed     bool          `json:"changed"`        // 内容是否实际变化
}
```

### WatchRule（Watch 规则）

```go
type WatchRule struct {
    Name          string   `yaml:"name"`
    Source        string   `yaml:"source"`           // 源路径（绝对或相对）
    Target        string   `yaml:"target"`           // 知识库目标路径前缀
    Strategy      string   `yaml:"strategy"`         // skip|overwrite|merge|patch
    DebounceMs    int      `yaml:"debounceMs"`       // 去抖窗口（默认 500ms）
    FilePatterns  []string `yaml:"filePatterns"`     // glob 模式（默认 ["*.md"]）
    PatchFields   []string `yaml:"patchFields,omitempty"`
    Recursive     bool     `yaml:"recursive"`        // 是否递归监听子目录
    FollowSymlinks bool    `yaml:"followSymlinks"`   // 是否跟随符号链接（默认 false）
}
```

---

## 变更检测算法

### 双层检测流程

```
输入: sourcePath
输出: DetectResult

1. 文件 stat 失败?
   → 不存在 && 在索引中 → DetectSourceMissing
   → 不存在 && 不在索引中 → DetectNewFile
   → 其他错误 → DetectContentChanged（保守处理）

2. 文件在索引中?
   → 否 → DetectNewFile

3. FAST PATH: size == meta.FileSize && mtime == meta.LastModified
   → 是 → DetectNoChange

4. SLOW PATH: hash = ComputeSHA256(sourcePath)
   → hash == meta.ContentHash → DetectMetaChanged（仅更新元数据）
   → hash != meta.ContentHash → DetectContentChanged
```

**DetectMetaChanged 的处理**：不调用 MergeEngine，静默更新 `FileMetadata.LastModified` 和 `FileSize`，返回成功。

---

## 四种合并策略

### StrategySkip（默认）

```
行为：
  - 目标文件存在 → 跳过
  - 目标文件不存在 → 导入为新文件

适用：保守导入，防止覆盖用户手动修改
```

### StrategyOverwrite

```
行为：
  1. 读取源文件全部内容
  2. 原子化写入目标路径（先写 .tmp，再 rename）
  3. 更新元数据

适用：源文件为唯一真实来源（如自动文档生成工具输出）
```

### StrategyMerge

```
行为：
  1. 读取源文件和目标文件
  2. 解析两者的 YAML frontmatter
  3. frontmatter 合并规则：
     - 已有字段（type/title/description/resource）：保留目标文件值
     - 新增字段（不在目标中的字段）：从源文件复制
     - tags 字段：取源+目标的并集（去重）
  4. 正文内容：保留目标文件的正文（不受源文件影响）
  5. 生成合并后的文件，原子化写入

适用：目标文件有自定义标签，源文件有新字段需要引入
```

### StrategyPatch

```
行为：
  1. 读取源文件和目标文件
  2. 解析两者的 frontmatter
  3. 仅更新 patchFields 中指定的字段（从源取值写入目标）
  4. 保留目标文件的其他所有字段和正文
  5. 生成文件，原子化写入

示例：patchFields = ["title", "description", "tags"]
     → 仅更新这 3 个字段，其他字段和正文不变

适用：仅同步元数据变更，正文内容始终保留
```

**策略决策优先级**（从高到低）：
1. `SmartImportOptions.ForceStrategy`（CLI 传入）
2. `MetadataIndex.Files[target].Strategy`（已记录的策略）
3. `StrategySkip`（默认）

**PatchFields 优先级**：
1. `WatchRule.PatchFields`（watch 规则指定）
2. `MetadataIndex.Files[target].PatchFields`（已记录的字段）
3. `["title", "description", "tags"]`（默认值）

---

## 并发安全设计

### 元数据读写

- **进程内**：`MetadataIndex` 内置 `sync.RWMutex`
  - 读操作（GetBySource/GetByTarget/DetectChange）：`RLock`
  - 写操作（Add/Update/Delete）：`Lock`

- **进程间**：文件锁（`flock` on Unix，`LockFileEx` on Windows）
  - `SaveIndex` 前获取写锁，保存后释放
  - `LoadIndex` 前尝试获取读锁（非阻塞），失败则返回错误
  - 同时使用版本号 CAS 校验：写入前读取当前版本号，写入时携带版本号，若版本号已变化则重试

### 文件原子化写入

```
临时文件 target.md.tmp → 写入内容 → rename → target.md
```

- rename 在 POSIX 上原子（同一文件系统内）
- Windows rename 不可跨盘，若检测到跨盘则使用 copy+delete 方案
- rename 失败时清理 .tmp 文件

---

## Watch Daemon 设计

### 事件处理流程

```
文件系统事件 (CREATE/WRITE/RENAME/REMOVE)
    ↓
Debouncer（500ms 冷却窗口，同一文件的多次事件合并）
    ↓
规则匹配器（遍历 WatchRule，检查 source 路径前缀 + filePatterns glob）
    ↓
任务队列（单 worker 串行执行，避免并发写入冲突）
    ↓
Smart Import Engine（执行变更检测和合并策略）
    ↓
元数据持久化
```

### 信号处理

| 信号 | 行为 |
|------|------|
| SIGINT / SIGTERM | 优雅退出（处理完当前任务再退出） |
| SIGHUP | 重新加载 `.watch.yaml` 规则 |

### 启动检查

1. 检查 pidfile 是否存在
2. 若存在，检查 pid 是否有效（`kill -0 pid`）
3. 若有效，报错退出（防止重复启动）
4. 若无效，清理 pidfile，正常启动

### inotify 限制处理

- 启动时检测 inotify 可用数量
- 接近 `/proc/sys/fs/inotify/max_user_watches` 限制时记录警告日志
- 提供 `--no-watch` 纯轮询模式作为 fallback（每分钟扫描一次）

---

## 元数据持久化

### .metadata.json 格式

```json
{
  "version": "1.0",
  "updatedAt": "2024-01-15T10:30:00Z",
  "files": {
    "api/user.md": {
      "sourcePath": "/home/user/docs/api/user.md",
      "targetPath": "api/user.md",
      "contentHash": "a1b2c3d4e5f6...",
      "lastModified": "2024-01-15T09:00:00Z",
      "lastImported": "2024-01-15T10:30:00Z",
      "fileSize": 1024,
      "strategy": "merge",
      "patchFields": ["title", "description", "tags"],
      "sourceExists": true
    }
  },
  "bySource": {
    "/home/user/docs/api/user.md": "api/user.md"
  }
}
```

### 版本迁移

当 `version` 低于当前版本时，执行 `MigrateIndex(from, to string)`：

```
v0.x → v1.0:
  - 补全 updatedAt 字段（从 last_synced 改名）
  - 字段重命名 snake_case → camelCase
  - 补全 sourceExists = true
  - 生成 bySource 反向索引
```

### 向后兼容

- **无 .metadata.json**：初始化空索引，正常运行
- **损坏 .metadata.json**：返回错误，允许用 `--rebuild` 从知识库重建
- **无元数据的旧知识库**：`--rebuild` 扫描所有 .md 文件，基于内容计算 hash，`bySource` 设为空，并警告用户无法追踪源文件变更

---

## CLI 命令

### `okf add`（增强版）

```bash
okf add <source> --dir ./knowledge [options]
  --strategy=skip|overwrite|merge|patch   # 导入策略
  --patch-fields=title,description         # patch 模式更新的字段
  --detect-only                            # 仅检测变更，不执行
```

### `okf sync`

```bash
okf sync --dir ./knowledge [options]
  # 检查知识库中所有已记录文件的源路径
  # 自动检测变更并应用策略
  --prune                                  # 清理 SourceExists=false 的记录
```

### `okf watch`

```bash
okf watch --dir ./knowledge [options]
  --config=./.watch.yaml                  # 指定 watch 规则配置
  --debounce=500                          # 全局去抖时间（ms）
  --pidfile=/var/run/okf.pid             # 进程 pid 文件
  --no-watch                              # 纯轮询模式（无 inotify）
```

### `okf metadata`

```bash
okf metadata --dir ./knowledge [options]
  --list                                  # 列出所有已知文件
  --check                                 # 检查源文件存在性和变更
  --rebuild                               # 从现有知识库重建索引
  --update <source>                       # 手动更新某个文件的元数据
```

---

## 性能目标

| 操作 | 100 文件 | 1000 文件 | 5000 文件 |
|------|----------|-----------|-----------|
| FAST PATH | < 50ms | < 200ms | < 1s |
| 完整检测（含 hash） | < 200ms | < 2s | < 10s |
| 完整导入（含写文件） | < 500ms | < 5s | < 25s |

Watch daemon 内存占用 < 50MB（1000 文件），CPU idle < 1%（无事件时）。

---

## 测试规划

| 模块 | 测试重点 | 覆盖率目标 |
|------|---------|-----------|
| MetadataIndex | CRUD、持久化、损坏恢复、版本迁移 | 95% |
| ChangeDetector | FAST PATH 命中、SLOW PATH 精确、边界条件 | 90% |
| MergeEngine | 4 种策略的各种场景 | 95% |
| AtomicWriter | rename 失败恢复、跨盘写入 | 85% |
| WatchDaemon | 去抖逻辑、信号处理（集成测试） | 80% |
| CLI 整合 | 命令参数解析、错误提示 | 85% |

---

## 版本历史

| 版本 | 日期 | 变更 |
|------|------|------|
| v1.0 | - | 初始设计 |
| v1.1 | 当前 | 移除 `.patches/` 和 `frontmatterHash`（精简过度设计）；修正 Merge/Patch 正文处理歧义；明确 DetectMetaChanged 流程；补充文件锁和并发安全设计；修正 JSON 字段名为 camelCase |
