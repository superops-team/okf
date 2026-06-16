# API 接口设计

## 1. 核心 API 接口

### 1.1 MetadataManager API

```go
type MetadataManager interface {
    // 读取/写入元数据索引
    LoadIndex(path string) (*MetadataIndex, error)
    SaveIndex(index *MetadataIndex, path string) error

    // 查询操作 (RLock)
    GetBySource(sourcePath string) (*FileMetadata, bool)
    GetByTarget(targetPath string) (*FileMetadata, bool)
    IsImported(sourcePath string) bool

    // 写操作 (Lock)
    Update(meta *FileMetadata) error
    DeleteBySource(sourcePath string) error
    Upsert(meta *FileMetadata) error  // Insert or Update
}
```

### 1.2 ChangeDetector API

```go
type ChangeDetector interface {
    // 检测单个文件是否需要更新
    Detect(sourcePath string, knownMeta *FileMetadata) (DetectResult, error)

    // 批量检测目录
    DetectDirectory(sourceDir string) (map[string]DetectResult, error)
}
```

### 1.3 MergeEngine API

```go
type MergeEngine interface {
    // 决定使用哪种策略
    DecideStrategy(meta *FileMetadata, userOpts SmartImportOptions) ImportStrategy

    // 执行策略
    ApplyStrategy(sourcePath, targetPath string,
                  strategy ImportStrategy,
                  meta *FileMetadata) (*MergeResult, error)

    // 四种具体策略实现
    DoSkip(source, target string, meta *FileMetadata) (*MergeResult, error)
    DoOverwrite(source, target string, meta *FileMetadata) (*MergeResult, error)
    DoMerge(source, target string, meta *FileMetadata) (*MergeResult, error)
    DoPatch(source, target string, meta *FileMetadata, fields []string) (*MergeResult, error)
}
```

### 1.4 SmartImporter API

```go
type SmartImporter interface {
    // 导入单个文件 (智能检测)
    ImportFile(source, target string, opts SmartImportOptions) (*ImportResult, error)

    // 批量导入目录
    ImportDirectory(sourceDir, targetDir string, opts SmartImportOptions) (*ImportResult, error)

    // 检测变更 (不执行，返回检测报告)
    DetectChanges(sourceDir, targetDir string) (*ImportResult, error)
}
```

### 1.5 WatchDaemon API

```go
type WatchDaemon interface {
    // 启动监听器
    Start(config WatchConfig, knowledgeDir string) error

    // 停止监听器
    Stop() error

    // 重新加载配置 (SIGHUP 触发)
    ReloadConfig() error

    // 查询当前状态
    Status() DaemonStatus
}

type DaemonStatus struct {
    Running     bool
    Uptime     time.Duration
    RulesActive int
    FilesWatched int
    LastEvent   time.Time
    LastImport  time.Time
}
```

## 2. CLI 接口设计

### 2.1 新增命令

```bash
# 1. 智能导入命令 (对现有 okf add 的增强)
okf add <source> --dir ./knowledge [options]
    --strategy=skip|overwrite|merge|patch   # 导入策略
    --patch-fields=title,description         # patch 模式下更新的字段
    --detect-only                            # 仅检测变更，不执行
    --save-patch                            # 保存 patch 记录

# 2. 同步命令 (基于元数据的全量同步)
okf sync --dir ./knowledge
    # 检查知识库中所有已记录文件的源路径
    # 自动检测变更并应用策略

# 3. 守护进程命令
okf watch --dir ./knowledge [options]
    --config=./.watch.yaml                  # 指定 watch 规则配置
    --debounce=500                          # 全局去抖时间 (ms)
    --pidfile=/var/run/okf.pid             # 进程 pid 文件

# 4. 元数据管理命令
okf metadata --dir ./knowledge [options]
    --list                                  # 列出所有已知文件
    --check                                 # 检查源文件存在性和变更
    --prune                                 # 清理源文件已不存在的记录
    --rebuild                               # 从现有知识库重建索引
    --update <source>                       # 手动更新某个文件的元数据
```

### 2.2 示例输出

```bash
$ okf add ./docs --dir ./knowledge --strategy=merge

[okf] Scanning ./docs for markdown files...
[okf] Found 12 files.
[okf] Loading metadata...
[okf] Detecting changes...
  ✓ concepts/architecture.md   (无变更)
  ↑ api/user.md                (内容变更 → merge 策略)
  + api/orders.md              (新文件)
  ✓ concepts/design.md         (无变更)
  - old-notes.md              (已存在，skip 策略)

[okf] Importing files...
  ↑ 更新 api/user.md
  + 新增 api/orders.md

[okf] Import complete: 2 imported, 2 skipped, 12 total
[okf] Metadata saved: ./knowledge/.metadata.json
```

## 3. 错误处理 API

```go
type ImportError struct {
    Path    string
    Code    ErrorCode
    Message string
    Err     error
}

type ErrorCode int

const (
    ErrFileNotFound     ErrorCode = iota
    ErrPermissionDenied
    ErrInvalidFormat
    ErrContentEmpty
    ErrMetadataCorrupted
    ErrStrategyFailed
    ErrConcurrentAccess
)

// 友好的错误信息
func (e *ImportError) Error() string {
    switch e.Code {
    case ErrFileNotFound:
        return fmt.Sprintf("%s: source file not found", e.Path)
    case ErrPermissionDenied:
        return fmt.Sprintf("%s: permission denied (check write access)", e.Path)
    case ErrInvalidFormat:
        return fmt.Sprintf("%s: invalid markdown format (missing frontmatter?)", e.Path)
    default:
        return fmt.Sprintf("%s: %s", e.Path, e.Message)
    }
}
```

## 4. 合并策略具体实现

### 4.1 Skip 策略

```
流程:
  1. 检查目标文件是否存在
  2. 存在 → 跳过，记录到 skipped 计数
  3. 不存在 → 导入为新文件

用例:
  - 保守导入，避免覆盖人工修改
  - 首次批量导入后，后续增量导入
```

### 4.2 Overwrite 策略

```
流程:
  1. 读取源文件内容
  2. 原子化写入目标路径 (target.md.tmp → target.md)
  3. 更新元数据

用例:
  - 源文件为唯一真源 (single source of truth)
  - 自动文档生成工具输出
```

### 4.3 Merge 策略

```
流程:
  1. 读取源文件 (Source) 和目标文件 (Target)
  2. 解析两者的 frontmatter
  3. 合并规则:
     - 已有字段: 保留 Target 的值
     - 新增字段: 从 Source 复制
     - tags 字段: 取并集 (去重)
  4. 正文内容: 保留 Target 正文
  5. 生成合并后的文件

用例:
  - 目标文件有自定义标签和描述
  - 源文件有新增字段需要引入
```

### 4.4 Patch 策略

```
流程:
  1. 读取源文件和目标文件
  2. 解析两者 frontmatter
  3. 仅更新 patch_fields 中指定的字段:
     - 从 Source 读取字段值
     - 写入 Target 对应字段
  4. 保留 Target 的其他字段和正文

用例:
  - 仅更新 title/description/tags
  - 保持其他自定义字段不变
```

## 5. 原子化写入流程

```go
func atomicWrite(targetPath string, content []byte) error {
    // 步骤1: 写入临时文件
    tmpPath := targetPath + ".tmp"
    if err := os.WriteFile(tmpPath, content, 0644); err != nil {
        return fmt.Errorf("write temp file: %w", err)
    }

    // 步骤2: 校验临时文件完整性 (SHA256)
    data, err := os.ReadFile(tmpPath)
    if err != nil {
        os.Remove(tmpPath)  // 清理
        return err
    }
    if !bytes.Equal(data, content) {
        os.Remove(tmpPath)  // 清理
        return fmt.Errorf("integrity check failed")
    }

    // 步骤3: 原子化 rename (跨平台)
    if err := os.Rename(tmpPath, targetPath); err != nil {
        os.Remove(tmpPath)  // 清理
        return err
    }

    return nil
}
```

## 6. 元数据持久化流程

```go
func (m *MetadataIndex) Save(path string) error {
    // 1. 序列化
    data, err := json.MarshalIndent(m, "", "  ")
    if err != nil { return err }

    // 2. 写入临时文件
    tmpPath := path + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return err
    }

    // 3. 校验
    if _, err := LoadIndex(tmpPath); err != nil {
        os.Remove(tmpPath)
        return err
    }

    // 4. 原子化 rename
    return os.Rename(tmpPath, path)
}
```

## 7. Watch 事件处理流程

```go
func (d *daemon) handleEvent(event fsnotify.Event) {
    // 1. Debounce
    debounceKey := d.debounceKey(event.Name)
    d.debouncer.Add(debounceKey, 500ms, func() {
        // 2. 规则匹配
        rule, targetDir := d.matchRule(event.Name)
        if rule == nil { return }

        // 3. 文件模式过滤
        if !d.matchPattern(event.Name, rule.FilePatterns) {
            return
        }

        // 4. 加入任务队列
        d.taskQueue <- importTask{
            source: event.Name,
            target: targetDir,
            rule: rule,
        }
    })
}

// 单 worker 处理任务队列
func (d *daemon) runWorker() {
    for task := range d.taskQueue {
        // 串行执行，避免并发写入
        if err := d.importer.ImportFile(task.source, task.target, task.opts); err != nil {
            log.Printf("import failed: %v", err)
        }
    }
}
```
