package okf

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// =============================================================================
// 常量定义
// =============================================================================

const (
	// MetadataVersion 是当前 .metadata.json 的格式版本
	MetadataVersion = "1.0"

	// DefaultMetadataFilename 是元数据索引文件的默认名称
	DefaultMetadataFilename = ".metadata.json"

	// DefaultWatchConfigFilename 是 watch 配置文件的默认名称
	DefaultWatchConfigFilename = ".watch.yaml"
)

// =============================================================================
// 变更检测结果枚举
// =============================================================================

// DetectResult 表示文件变更检测的结果
type DetectResult int

const (
	// DetectNoChange 文件没有变化（FAST PATH 通过或 hash 一致）
	DetectNoChange DetectResult = iota

	// DetectNewFile 文件尚未被索引记录
	DetectNewFile

	// DetectMetaChanged 文件的 mtime/size 变化，但内容 hash 未变
	DetectMetaChanged

	// DetectContentChanged 文件内容发生了变化（hash 不同）
	DetectContentChanged

	// DetectSourceMissing 被索引的源文件在磁盘上找不到
	DetectSourceMissing
)

// String 返回可读的检测结果
func (r DetectResult) String() string {
	switch r {
	case DetectNoChange:
		return "no_change"
	case DetectNewFile:
		return "new_file"
	case DetectMetaChanged:
		return "meta_changed"
	case DetectContentChanged:
		return "content_changed"
	case DetectSourceMissing:
		return "source_missing"
	default:
		return "unknown"
	}
}

// =============================================================================
// 合并策略枚举
// =============================================================================

// MergeStrategy 表示当目标文件已存在时的合并策略
type MergeStrategy string

const (
	// StrategySkip 跳过已存在的文件（默认）
	StrategySkip MergeStrategy = "skip"

	// StrategyOverwrite 直接覆盖目标文件
	StrategyOverwrite MergeStrategy = "overwrite"

	// StrategyMerge 合并 frontmatter（保留自定义字段，合并 tags）
	StrategyMerge MergeStrategy = "merge"

	// StrategyPatch 仅更新指定的 frontmatter 字段
	StrategyPatch MergeStrategy = "patch"
)

// =============================================================================
// 核心数据结构
// =============================================================================

// FileMetadata 记录单个源文件的元数据
type FileMetadata struct {
	// SourcePath 源文件的绝对路径
	SourcePath string `json:"sourcePath"`

	// TargetPath 知识库内的相对路径
	TargetPath string `json:"targetPath"`

	// ContentHash SHA-256 hash（hex 编码）
	ContentHash string `json:"contentHash"`

	// LastModified 源文件上次修改时间
	LastModified time.Time `json:"lastModified"`

	// LastImported 上次导入时间
	LastImported time.Time `json:"lastImported"`

	// FileSize 文件大小（字节）
	FileSize int64 `json:"fileSize"`

	// Strategy 合并策略
	Strategy MergeStrategy `json:"strategy,omitempty"`

	// PatchFields 在 patch 策略下需要更新的字段列表
	PatchFields []string `json:"patchFields,omitempty"`

	// SourceExists 源文件当前是否存在于磁盘
	SourceExists bool `json:"sourceExists"`
}

// MetadataIndex 是知识库整体的文件元数据索引
type MetadataIndex struct {
	// Version 元数据格式版本
	Version string `json:"version"`

	// UpdatedAt 索引最后更新时间
	UpdatedAt time.Time `json:"updatedAt"`

	// Files 以 TargetPath 为 key 的文件元数据映射
	Files map[string]*FileMetadata `json:"files"`

	// BySource 以 SourcePath 为 key 的反向索引
	BySource map[string]string `json:"bySource"`

	// mu 并发读写保护
	mu sync.RWMutex `json:"-"`
}

// =============================================================================
// 构造函数
// =============================================================================

// NewMetadataIndex 创建一个空的元数据索引
func NewMetadataIndex() *MetadataIndex {
	return &MetadataIndex{
		Version:  MetadataVersion,
		Files:    make(map[string]*FileMetadata),
		BySource: make(map[string]string),
	}
}

// =============================================================================
// MetadataIndex - 基本 CRUD
// =============================================================================

// Add 添加一条文件元数据记录。如果 target path 已存在，返回错误。
// 新添加的记录默认 SourceExists=true（导入时源文件已存在）
func (idx *MetadataIndex) Add(meta *FileMetadata) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if meta == nil {
		return fmt.Errorf("metadata cannot be nil")
	}
	if meta.TargetPath == "" {
		return fmt.Errorf("target path cannot be empty")
	}
	if _, exists := idx.Files[meta.TargetPath]; exists {
		return fmt.Errorf("target path already tracked: %s", meta.TargetPath)
	}

	// 默认 SourceExists=true（导入时源文件必定存在）
	meta.SourceExists = true
	idx.Files[meta.TargetPath] = meta
	if meta.SourcePath != "" {
		idx.BySource[meta.SourcePath] = meta.TargetPath
	}
	return nil
}

// Update 更新一条文件元数据记录。target path 必须已存在。
func (idx *MetadataIndex) Update(meta *FileMetadata) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if meta == nil {
		return fmt.Errorf("metadata cannot be nil")
	}
	if meta.TargetPath == "" {
		return fmt.Errorf("target path cannot be empty")
	}
	old, exists := idx.Files[meta.TargetPath]
	if !exists {
		return fmt.Errorf("target path not tracked: %s", meta.TargetPath)
	}

	// 如果 SourcePath 变化，需要更新反向索引
	if old.SourcePath != meta.SourcePath {
		delete(idx.BySource, old.SourcePath)
	}
	if meta.SourcePath != "" {
		idx.BySource[meta.SourcePath] = meta.TargetPath
	}
	idx.Files[meta.TargetPath] = meta
	return nil
}

// DeleteByTarget 通过 target path 删除记录
func (idx *MetadataIndex) DeleteByTarget(targetPath string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	meta, exists := idx.Files[targetPath]
	if !exists {
		return fmt.Errorf("target path not tracked: %s", targetPath)
	}
	delete(idx.Files, targetPath)
	if meta.SourcePath != "" {
		delete(idx.BySource, meta.SourcePath)
	}
	return nil
}

// GetByTarget 通过 target path 查找
func (idx *MetadataIndex) GetByTarget(targetPath string) (*FileMetadata, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	meta, ok := idx.Files[targetPath]
	return meta, ok
}

// GetBySource 通过 source path 查找
func (idx *MetadataIndex) GetBySource(sourcePath string) (*FileMetadata, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	target, ok := idx.BySource[sourcePath]
	if !ok {
		return nil, false
	}
	meta, ok := idx.Files[target]
	return meta, ok
}

// IsTracked 判断 target path 是否已经被索引
func (idx *MetadataIndex) IsTracked(targetPath string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	_, ok := idx.Files[targetPath]
	return ok
}

// Len 返回索引中文件数量
func (idx *MetadataIndex) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.Files)
}

// RangeFiles 按 TargetPath 遍历所有元数据记录（只读安全）
// callback 返回 false 时停止遍历
func (idx *MetadataIndex) RangeFiles(fn func(target string, meta *FileMetadata) bool) {
	idx.mu.RLock()
	snapshot := make([]struct {
		target string
		meta   *FileMetadata
	}, 0, len(idx.Files))
	for t, m := range idx.Files {
		snapshot = append(snapshot, struct {
			target string
			meta   *FileMetadata
		}{t, m})
	}
	idx.mu.RUnlock()

	for _, item := range snapshot {
		if !fn(item.target, item.meta) {
			return
		}
	}
}

// =============================================================================
// MetadataIndex - 持久化
// =============================================================================

// Save 将索引以原子方式写入磁盘
func (idx *MetadataIndex) Save(path string) error {
	idx.mu.Lock()
	idx.UpdatedAt = time.Now().UTC()
	idx.mu.Unlock()

	idx.mu.RLock()
	data, err := json.MarshalIndent(idx, "", "  ")
	idx.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("failed to marshal metadata index: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create metadata dir: %w", err)
	}

	if err := AtomicWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}
	return nil
}

// Load 从磁盘加载元数据索引
// - 文件不存在视为空索引（不返回错误）
// - 损坏 JSON 会返回错误，允许上层决定是否重建
func (idx *MetadataIndex) Load(path string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 不存在视为空索引
			if idx.Files == nil {
				idx.Files = make(map[string]*FileMetadata)
			}
			if idx.BySource == nil {
				idx.BySource = make(map[string]string)
			}
			if idx.Version == "" {
				idx.Version = MetadataVersion
			}
			return nil
		}
		return fmt.Errorf("failed to read metadata file: %w", err)
	}

	// 解析前保留初始状态
	var loaded MetadataIndex
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("failed to parse metadata file: %w", err)
	}

	// 版本兼容处理
	if loaded.Version == "" {
		loaded.Version = MetadataVersion
	}
	if loaded.Files == nil {
		loaded.Files = make(map[string]*FileMetadata)
	}
	if loaded.BySource == nil {
		loaded.BySource = make(map[string]string)
	}

	// 手动字段赋值，避免覆盖 mu（sync.RWMutex 不可复制）
	idx.Version = loaded.Version
	idx.UpdatedAt = loaded.UpdatedAt
	idx.Files = loaded.Files
	idx.BySource = loaded.BySource
	return nil
}

// =============================================================================
// MetadataIndex - 变更检测
// =============================================================================

// DetectChange 对单个源文件执行变更检测
// 流程：
//  1. 文件不存在 → DetectSourceMissing
//  2. 未在索引中 → DetectNewFile
//  3. FAST PATH: size 和 mtime 都一致 → DetectNoChange
//  4. SLOW PATH: 计算 SHA-256 hash
//     - hash 一致 → DetectMetaChanged（仅元数据被更新）
//     - hash 不同 → DetectContentChanged
func (idx *MetadataIndex) DetectChange(sourcePath string) DetectResult {
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 检查是否在索引中
			if _, ok := idx.GetBySource(sourcePath); ok {
				return DetectSourceMissing
			}
			return DetectNewFile
		}
		// 其他错误保守处理
		return DetectContentChanged
	}

	meta, ok := idx.GetBySource(sourcePath)
	if !ok {
		return DetectNewFile
	}

	// FAST PATH: mtime + size 快速判断
	// 容忍 1 秒 mtime 差异（跨 FS 复制时常见精度损失）
	if meta.FileSize == info.Size() &&
		mtimeEqualish(meta.LastModified, info.ModTime()) {
		return DetectNoChange
	}

	// SLOW PATH: 计算内容 hash
	hash, err := ComputeFileHash(sourcePath)
	if err != nil {
		return DetectContentChanged
	}
	if hash == meta.ContentHash {
		return DetectMetaChanged
	}
	return DetectContentChanged
}

// =============================================================================
// Hash 计算
// =============================================================================

// ComputeFileHash 流式计算文件的 SHA-256 hash（hex 小写）
func ComputeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash file content: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeHash 从字节切片计算 SHA-256 hash
func ComputeHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// =============================================================================
// 原子写入
// =============================================================================

// AtomicWriteFile 以原子方式写入文件：先写 .tmp 临时文件，再 os.Rename
// 注意：在 Windows 上，如果目标文件已被打开，Rename 可能失败。
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically replace file: %w", err)
	}
	return nil
}

// =============================================================================
// 工具函数：默认知识库目录
// =============================================================================

// DefaultKnowledgeDir 返回当前系统推荐的知识库根目录绝对路径
func DefaultKnowledgeDir() string {
	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("APPDATA")
		if dir == "" {
			dir = os.Getenv("USERPROFILE")
		}
	default:
		dir = os.Getenv("HOME")
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, ".okf", "knowledge")
}

// KnowledgeMetadataPath 返回知识库根目录下的元数据文件绝对路径
func KnowledgeMetadataPath(knowledgeDir string) string {
	return filepath.Join(knowledgeDir, DefaultMetadataFilename)
}

// mtimeEqualish 比较两个 mtime 是否"近似相等"（容忍 1 秒误差）
// 跨文件系统或低精度 FS 上 mtime 可能因取整产生 1 秒差异
func mtimeEqualish(a, b time.Time) bool {
	if a.Equal(b) {
		return true
	}
	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff < time.Second
}

// =============================================================================
// 版本迁移
// =============================================================================

// DefaultPatchFields 是 patch 策略的默认更新字段列表
var DefaultPatchFields = []string{"title", "description", "tags"}

// MigrateIndex 将旧版本格式迁移到当前版本
// 当前只支持 v0.x → v1.0 的迁移
func MigrateIndex(idx *MetadataIndex) error {
	if idx.Version == MetadataVersion {
		return nil // 已是最新版本
	}

	// v0.x → v1.0 迁移
	// 1. 字段重命名 snake_case → camelCase（JSON tag 已处理）
	// 2. bySource 反向索引（若不存在则从 Files 重建）
	// 3. SourceExists 默认设为 true
	// 4. 补全 updatedAt（若缺失则从现有记录中取最大值）
	if idx.BySource == nil {
		idx.BySource = make(map[string]string)
	}
	for _, meta := range idx.Files {
		if meta.SourcePath != "" {
			idx.BySource[meta.SourcePath] = meta.TargetPath
		}
		// 补全 SourceExists（默认为 true）
		if !meta.SourceExists {
			meta.SourceExists = true
		}
	}
	if idx.UpdatedAt.IsZero() {
		var latest time.Time
		for _, meta := range idx.Files {
			if meta.LastImported.After(latest) {
				latest = meta.LastImported
			}
		}
		idx.UpdatedAt = latest
	}
	idx.Version = MetadataVersion
	return nil
}

// =============================================================================
// 文件锁接口（平台相关实现在 detect_lock_*.go 中）
// =============================================================================

// FileLock 封装跨平台文件锁
type FileLock interface {
	Lock() error
	Unlock() error
}

// NewFileLock 创建指定路径的文件锁
// 在 detect_lock_unix.go / detect_lock_windows.go 中实现
func NewFileLock(path string) (FileLock, error) {
	// 延迟加载平台特定实现
	return newFileLock(path)
}
