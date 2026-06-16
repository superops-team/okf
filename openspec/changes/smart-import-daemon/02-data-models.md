# 数据模型设计

## 1. 核心数据结构

### 1.1 FileMetadata（文件元数据）

记录每个已导入文件的完整元数据，用于变更检测和重复文件判断。

```go
type FileMetadata struct {
    SourcePath      string    `json:"source_path"`       // 源文件绝对路径 (去重键 1)
    TargetPath      string    `json:"target_path"`       // 知识库中相对路径 (去重键 2)
    ContentHash      string    `json:"content_hash"`       // 文件内容 SHA256 (完整内容)
    FrontmatterHash string    `json:"frontmatter_hash"`  // 仅 frontmatter 的 SHA256 (用于 merge 策略)
    LastModified   time.Time `json:"last_modified"`    // 源文件 mtime
    LastImported   time.Time `json:"last_imported"`    // 最后一次成功导入时间
    FileSize       int64     `json:"file_size"`          // 源文件大小 bytes
    ImportStrategy   string  `json:"import_strategy"`   // skip|overwrite|merge|patch
}
```

**设计意图**：
- `SourcePath + TargetPath` 联合作为唯一键，避免重复导入
- `ContentHash` 用于精确内容变更检测
- `LastModified + FileSize` 用于 FAST PATH 快速检测
- `ImportStrategy` 记录该文件使用的导入策略

### 1.2 MetadataIndex（元数据索引）

知识库级别的完整索引文件。

```go
type MetadataIndex struct {
    Version    string                  `json:"version"`    // 索引格式版本，用于升级检测
    LastSynced time.Time                `json:"last_synced"` // 最后同步时间
    Files      map[string]*FileMetadata `json:"files"`  // key: target_path (相对知识库路径)
    BySource   map[string]string        `json:"by_source"` // source_path -> target_path 反向索引
}
```

**设计意图**：
- `Files` map 的 key 使用目标路径（相对知识库路径）
- `BySource` 反向索引：通过源文件路径快速查找是否已导入
- 原子化读写保证并发安全

### 1.3 DetectResult（检测结果）

检测器返回的结果，指导后续操作。

```go
type DetectResult int

const (
    DetectNoChange     DetectResult = iota // mtime+size+hash 都无变化 → FAST PATH
    DetectMetadataChanged                 // mtime/size 变了但 hash 不变 → 元数据更新
    DetectContentChanged                // hash 变了 → 需要导入
    DetectNewFile                      // 新文件 → 正常导入
    DetectSourceMissing                  // 源文件不存在
)
```

### 1.4 ImportStrategy（导入策略）

```go
type ImportStrategy int

const (
    StrategySkip       ImportStrategy = iota // 跳过已存在的文件
    StrategyOverwrite                        // 直接覆盖
    StrategyMerge                           // 合并 frontmatter
    StrategyPatch                         // 仅更新指定字段
)
```

### 1.5 MergeResult（合并结果）

```go
type MergeResult struct {
    Strategy     ImportStrategy `json:"strategy"`
    MergedContent string       `json:"merged_content"`
    Changed      bool          `json:"changed"`
    PatchData     string       `json:"patch_data,omitempty"` // diff 输出
}
```

### 1.6 WatchRule（Watch 规则）

```go
type WatchRule struct {
    Name       string            `yaml:"name"`
    Source     string            `yaml:"source"`         // 源路径 (绝对或相对)
    Target     string            `yaml:"target"`         // 知识库目标路径
    Strategy   string            `yaml:"strategy"`       // skip|overwrite|merge|patch
    DebounceMs int               `yaml:"debounce_ms"`     // 去抖时间窗口，默认500
    FilePatterns []string       `yaml:"file_patterns"` // glob 模式，默认 ["*.md"]
    PatchFields []string         `yaml:"patch_fields,omitempty"` // patch 模式下更新字段
    Recursive  bool              `yaml:"recursive"`     // 是否递归监听子目录
}
```

**File Watch Config（文件监听器配置）

```go
type WatchConfig struct {
    Version    string       `yaml:"version"`
    Rules     []WatchRule `yaml:"rules"`
    GlobalDebounceMs int `yaml:"global_debounce_ms"` // 全局去抖时间
}
```

### 1.7 PatchRecord（Patch 记录）

```go
type PatchRecord struct {
    Timestamp     time.Time `json:"timestamp"`
    SourcePath    string `json:"source_path"`
    TargetPath    string `json:"target_path"`
    Strategy      string `json:"strategy"`
    OldContentHash string `json:"old_content_hash"`
    NewContentHash string `json:"new_content_hash"`
    Patch         string `json:"patch,omitempty"` // unified diff
}
```

### 1.8 SmartImportOptions（智能导入选项）

扩展自原始 ImportOptions

```go
type SmartImportOptions struct {
    Strategy       ImportStrategy // 导入策略
    HashOnly      bool          // 仅计算 hash 不实际导入，用于 dry-run
    ForcePatchFields   []string      // patch 模式要更新的字段
    DetectOnly    bool           // 仅检测变更，不执行 (不同
}
```

## 2. 存储格式

### 2.1 .metadata.json

存储在知识库根目录，格式示例：

```json
{
  "version": "1.0",
  "last_synced": "2024-01-15T10:30:00Z",
  "files": {
    "api/user.md": {
      "source_path": "/home/user/docs/api/user.md",
      "content_hash": "a1b2c3d4e5f6...",
      "frontmatter_hash": "x7y8z9...",
      "last_modified": "2024-01-15T09:00:00Z",
      "last_imported": "2024-01-15T10:30:00Z",
      "file_size": 1024,
      "import_strategy": "merge"
    },
    "concepts/design.md": {
      "source_path": "/home/user/docs/concepts/design.md",
      "target_path": "concepts/design.md",
      "content_hash": "f1e2d3c4b5a6...",
      "frontmatter_hash": "m1n2o3...",
      "last_modified": "2024-01-14T16:00:00Z",
      "last_imported": "2024-01-15T08:00:00Z",
      "file_size": 2048,
      "import_strategy": "overwrite"
    }
  },
  "by_source": {
    "/home/user/docs/api/user.md": "api/user.md",
    "/home/user/docs/concepts/design.md": "concepts/design.md"
  }
}
```

### 2.2 .watch.yaml

Watch 规则配置，支持动态修改。

```yaml
version: "1.0"
global_debounce_ms: 500

rules:
  - name: "API Documentation"
    source: "/home/user/docs/api"
    target: "api"
    strategy: merge
    debounce_ms: 500
    file_patterns:
      - "*.md"
    recursive: true

  - name: "Design Concepts"
    source: "/home/user/docs/concepts"
    target: "concepts"
    strategy: patch
    debounce_ms: 1000
    patch_fields:
      - title
      - description
      - tags
    recursive: true

  - name: "Readme Files"
    source: "/home/user/docs"
    target: ""
    strategy: overwrite
    file_patterns:
      - "README.md"
    recursive: false
```

### 2.3 .patches/ 目录结构

每次更新时记录变更（可选，通过配置开启）：

```
knowledge/.patches/
├── 2024-01-15T10:30:00Z/
│   ├── summary.json
│   ├── api_user.md.patch
│   └── concepts_design.md.patch
└── 2024-01-15T11:00:00Z/
    └── ...
```

单个 patch 文件示例 (api_user.md.patch:

```diff
--- old/api/user.md
+++ new/api/user.md
@@ -1,6 +1,6 @@
 ---
 type: api
-title: User API V1
-description: 用户管理接口
+title: User API V2
+description: 用户管理接口 (支持 OAuth)
 tags:
   - users
   - auth
```

## 3. 状态机

文件变更处理状态机：

```
                    ┌────────────────────┐
                    │  Read Metadata    │
                    │  Index           │
                    └────────┬──────────┘
                             │
                    ┌────────▼────────┐
                    │  Detect    │ 检查源文件
                    │  Changes  │
                    └────┬──┬────┘
                         │  │
          ┌────────────┘  │  └──────────┐
          │                │                 │
    ┌────▼────┐      ┌──▼─────────┐      ┌──▼─────────┐
    │ New    │      │  No      │      │  Changed  │
    │ File   │      │  Change  │      │          │
    └────┬──┘      └────┬────┘      └────┬──────┘
         │                │                │
    ┌────▼────┐      ┌──▼─────────┐     ┌──▼──────────┐
    │ Import   │      │ Update    │     │  Merge    │
    │ File     │      │ Metadata │     │  Engine  │
    └──────────┘      └────────────┘     └────┬──────┘
                                           │
                              ┌────────────┴────────────┐
                              │            │           │
                          ┌──▼──┐     ┌──▼──┐    ┌──▼────┐
                          │ Skip │     │Over│    │Merge │
                          │      │     │write│    │       │
                          └──────┘     └──────┘    └────┬──┘
                                                         │
                                                    ┌──▼────┐
                                                    │ Update│
                                                    │ Index │
                                                    └──────┘
```

## 4. 变更流程

### 4.1 单文件导入流程

```go
// 伪代码

func SmartImportFile(source, target string, opts Options) error {
    // 1. 检查元数据
    meta := metadataIndex.GetBySource(source)

    // 2. 快速检测
    result, err := detectFile(meta)
    if err != nil { return err }

    // 3. 根据检测结果处理
    switch result {
    case DetectNewFile:
        // 新文件 → 直接导入
    case DetectNoChange, DetectMetadataChanged:
        // 无变更或仅元数据变更 → 跳过
    case DetectContentChanged:
        // 有内容变更 → 合并
        // 3.1 决策使用的策略
        strategy := decideStrategy(meta, opts)
        // 3.2 执行策略
        applyStrategy(strategy, source, target, meta)
    }

    // 4. 更新元数据
    metadataIndex.Update(meta)
}
```

### 4.2 哈希计算

```go
func computeHashes(filePath string) (contentHash, frontmatterHash string, err error) {
    data, err := os.ReadFile(filePath)
    if err != nil {
        return "", "", err
    }

    // 完整内容 hash
    contentHash = sha256hex(data)

    // 仅 frontmatter hash (如果存在)
    if frontmatter, ok := extractFrontmatter(data); ok {
        frontmatterHash = sha256hex(frontmatter)
    }
    return
}
```

### 4.3 FAST PATH 算法

```go
func detectFast(meta FileMetadata, sourceInfo os.FileInfo) DetectResult {
    // FAST PATH: 比较 mtime 和 file size
    // - 若都相同 → 无变更
    // - 若不同 → 需要 hash 比较
    if meta.LastModified == sourceInfo.ModTime() &&
       meta.FileSize == sourceInfo.Size() {
        return DetectNoChange
    }
    return DetectContentChanged
}
```

## 5. 性能考量

### 5.1 Hash 策略

- **mtime + size** 快速路径 → O(1) 系统调用，无文件 IO
- **SHA256** → 完整内容 hash → 精确但需要读文件
- **frontmatter hash** → 仅读 frontmatter 部分，用于 merge 策略

### 5.2 元数据存储

- 使用 JSON 格式 (单文件 ~10KB for 1000 个文件)
- 原子化读写：先写 .tmp 文件，再 rename
- 内存缓存：进程内 cache，定期回写

### 5.3 Watch daemon 性能

- **fsnotify** 内核级监听 → O(1) 事件响应
- **Debouncer** → 合并多次事件 → O(1) 去抖
- **串行任务队列** → 避免并发导入导致的资源竞争
- **动态规则热加载** → SIGHUP 信号重新加载配置

## 6. 并发安全设计

### 6.1 元数据读写锁

```go
type SafeMetadataIndex struct {
    mu    sync.RWMutex
    index MetadataIndex
}
```

- 读操作（查询、检测） → RLock
- 写操作（导入、更新） → Lock

### 6.2 文件原子化写入

```
源文件 → 临时文件 target.md.tmp → 校验完整性 → rename → target.md
```

### 6.3 Watch daemon 单例设计

- 使用进程锁文件（pidfile) 防止多实例启动
