package okf

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// =============================================================================
// 变更报告
// =============================================================================

// ChangeReport 单个文件的变更检测报告
type ChangeReport struct {
	SourcePath string
	TargetPath string
	Result     DetectResult
}

// =============================================================================
// SmartImporter
// =============================================================================

// SmartImporter 智能导入器：编排检测 + 决策 + 合并 + 元数据更新
type SmartImporter struct {
	idx          *MetadataIndex
	knowledgeDir string
}

// NewSmartImporter 创建 SmartImporter
func NewSmartImporter(idx *MetadataIndex, knowledgeDir string) *SmartImporter {
	return &SmartImporter{
		idx:          idx,
		knowledgeDir: knowledgeDir,
	}
}

// ImportFile 智能导入单个文件
// 流程：
//  1. 变更检测 → DetectResult
//  2. 根据检测结果分派：
//     - DetectNewFile → 正常导入
//     - DetectNoChange → 跳过
//     - DetectMetaChanged → 静默更新元数据
//     - DetectContentChanged → 调用 Merge Engine
//     - DetectSourceMissing → 标记 SourceExists=false
//  3. 持久化元数据
func (s *SmartImporter) ImportFile(source, target string, opts *SmartImportOptions) (*MergeResult, error) {
	if opts == nil {
		opts = DefaultSmartImportOptions()
	}

	// 1. 变更检测
	result := s.idx.DetectChange(source)

	// 解析目标绝对路径
	targetPath := s.resolveTargetPath(target)

	// 2. 分派
	switch result {
	case DetectSourceMissing:
		// 标记 SourceExists=false
		if meta, ok := s.idx.GetBySource(source); ok {
			meta.SourceExists = false
			s.idx.Update(meta)
		}
		return &MergeResult{Changed: false}, nil

	case DetectNoChange:
		return &MergeResult{Changed: false}, nil

	case DetectMetaChanged:
		// 静默更新元数据中的 mtime/size
		if meta, ok := s.idx.GetBySource(source); ok {
			if info, err := os.Stat(source); err == nil {
				meta.LastModified = info.ModTime()
				meta.FileSize = info.Size()
				meta.SourceExists = true
				s.idx.Update(meta)
			}
		}
		return &MergeResult{Changed: false}, nil

	case DetectNewFile:
		// 新文件：直接导入（默认 overwrite）
		return s.importNewFile(source, target, targetPath, opts)

	case DetectContentChanged:
		// 有变更：决策策略 → 执行
		meta, _ := s.idx.GetBySource(source)
		strategy := DecideStrategy(meta, opts)
		return s.applyStrategy(strategy, source, target, targetPath, meta, opts)
	}

	return &MergeResult{Changed: false}, nil
}

// importNewFile 导入新文件
func (s *SmartImporter) importNewFile(source, target, targetPath string, opts *SmartImportOptions) (*MergeResult, error) {
	content, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}

	if err := AtomicWriteFile(targetPath, content, 0644); err != nil {
		return nil, fmt.Errorf("write target: %w", err)
	}

	// 计算 hash 并记录元数据
	hash, _ := ComputeFileHash(source)
	info, _ := os.Stat(source)
	meta := &FileMetadata{
		SourcePath:   source,
		TargetPath:   target,
		ContentHash:  hash,
		LastModified: info.ModTime(),
		LastImported: time.Now().UTC(),
		FileSize:     info.Size(),
		Strategy:     opts.ForceStrategy,
		SourceExists: true,
	}
	if err := s.idx.Add(meta); err != nil {
		// 重复 TargetPath → 报错
		return nil, fmt.Errorf("record metadata: %w", err)
	}

	return &MergeResult{
		Strategy: opts.ForceStrategy,
		Content:  content,
		Changed:  true,
	}, nil
}

// applyStrategy 应用具体合并策略
func (s *SmartImporter) applyStrategy(strategy MergeStrategy, source, target, targetPath string, meta *FileMetadata, opts *SmartImportOptions) (*MergeResult, error) {
	var (
		result *MergeResult
		err    error
	)

	switch strategy {
	case StrategySkip:
		result, err = DoSkip(source, targetPath, meta)
	case StrategyOverwrite:
		result, err = DoOverwrite(source, targetPath, meta)
	case StrategyMerge:
		result, err = DoMerge(source, targetPath, meta)
	case StrategyPatch:
		result, err = DoPatchWithOpts(source, targetPath, meta, opts)
	default:
		return nil, fmt.Errorf("unknown strategy: %s", strategy)
	}

	if err != nil {
		return nil, err
	}

	// 更新元数据
	if meta != nil && result != nil {
		hash, _ := ComputeFileHash(source)
		info, _ := os.Stat(source)
		meta.ContentHash = hash
		meta.LastModified = info.ModTime()
		meta.LastImported = time.Now().UTC()
		meta.FileSize = info.Size()
		meta.Strategy = strategy
		meta.SourceExists = true
		if err := s.idx.Update(meta); err != nil {
			return result, fmt.Errorf("update metadata: %w", err)
		}
	}

	return result, nil
}

// resolveTargetPath 解析目标文件的绝对路径
func (s *SmartImporter) resolveTargetPath(target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(s.knowledgeDir, target)
}

// =============================================================================
// DetectChanges（仅检测不导入）
// =============================================================================

// DetectChanges 对给定的 source/target 列表执行变更检测，不修改任何文件
func (s *SmartImporter) DetectChanges(sources, targets []string) ([]ChangeReport, error) {
	// 简化：用 source 列表
	report := make([]ChangeReport, 0, len(sources))
	for _, src := range sources {
		var target string
		if meta, ok := s.idx.GetBySource(src); ok {
			target = meta.TargetPath
		}
		report = append(report, ChangeReport{
			SourcePath: src,
			TargetPath: target,
			Result:     s.idx.DetectChange(src),
		})
	}
	_ = targets
	return report, nil
}

// =============================================================================
// Sync（基于索引全量同步）
// =============================================================================

// Sync 同步所有已索引文件（检测变更并应用策略）
func (s *SmartImporter) Sync(pruneMissing bool) (synced, skipped, errors int, err error) {
	// 收集所有已索引文件（不持锁，仅快照）
	idx := s.idx
	idx.mu.RLock()
	sources := make([]string, 0, len(idx.BySource))
	for src := range idx.BySource {
		sources = append(sources, src)
	}
	idx.mu.RUnlock()

	for _, src := range sources {
		meta, ok := s.idx.GetBySource(src)
		if !ok {
			continue
		}
		target := meta.TargetPath
		result, e := s.ImportFile(src, target, nil)
		if e != nil {
			errors++
			continue
		}
		if result != nil && result.Changed {
			synced++
		} else {
			skipped++
		}
	}

	if pruneMissing {
		if p, e := s.Prune(); e == nil {
			_ = p
		}
	}
	return
}

// Prune 清理 SourceExists=false 的记录
// 返回被清理的记录数
func (s *SmartImporter) Prune() (int, error) {
	pruned := 0
	s.idx.mu.Lock()
	defer s.idx.mu.Unlock()

	toDelete := make([]string, 0)
	for target, meta := range s.idx.Files {
		if meta != nil && !meta.SourceExists {
			toDelete = append(toDelete, target)
		}
	}
	for _, target := range toDelete {
		// 已在外部持锁，直接操作 map（DeleteByTarget 内部会再次加锁）
		if meta, exists := s.idx.Files[target]; exists {
			delete(s.idx.Files, target)
			if meta != nil && meta.SourcePath != "" {
				delete(s.idx.BySource, meta.SourcePath)
			}
			pruned++
		}
	}
	return pruned, nil
}
