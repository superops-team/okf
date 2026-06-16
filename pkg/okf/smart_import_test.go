package okf

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// SmartImportFile 测试
// =============================================================================

func TestSmartImportFile_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(kbDir, 0755)

	source := filepath.Join(tmpDir, "src.md")
	target := "api/new.md"
	os.WriteFile(source, []byte("---\ntype: api\ntitle: New\n---\n# Body\n"), 0644)

	idx := NewMetadataIndex()
	importer := NewSmartImporter(idx, kbDir)

	result, err := importer.ImportFile(source, target, &SmartImportOptions{
		ForceStrategy: StrategyOverwrite,
	})
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true for new file")
	}

	// 验证文件被创建
	dstPath := filepath.Join(kbDir, target)
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Error("target file should be created")
	}

	// 验证元数据被记录
	if !idx.IsTracked(target) {
		t.Error("metadata should track the new file")
	}
}

func TestSmartImportFile_NoChange(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(kbDir, 0755)

	source := filepath.Join(tmpDir, "src.md")
	target := "api/unchanged.md"
	targetPath := filepath.Join(kbDir, target)

	content := []byte("---\ntype: api\ntitle: Same\n---\n# Body\n")
	os.WriteFile(source, content, 0644)
	os.WriteFile(targetPath, content, 0644)

	// 预先建立元数据记录（hash 一致 → FAST PATH）
	idx := NewMetadataIndex()
	hash, _ := ComputeFileHash(source)
	idx.Add(&FileMetadata{
		SourcePath:   source,
		TargetPath:   target,
		ContentHash:  hash,
		LastModified: mustStat(t, source).ModTime(),
		FileSize:     mustStat(t, source).Size(),
		Strategy:     StrategyOverwrite,
	})

	importer := NewSmartImporter(idx, kbDir)
	result, err := importer.ImportFile(source, target, &SmartImportOptions{
		ForceStrategy: StrategyOverwrite,
	})
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if result.Changed {
		t.Error("expected Changed=false when content unchanged")
	}
}

func TestSmartImportFile_ContentChanged_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(kbDir, 0755)

	source := filepath.Join(tmpDir, "src.md")
	target := "api/changed.md"
	targetPath := filepath.Join(kbDir, target)

	oldContent := []byte("---\ntype: api\ntitle: Old\n---\n# Old\n")
	newContent := []byte("---\ntype: api\ntitle: New\n---\n# New\n")
	os.WriteFile(source, oldContent, 0644)
	os.WriteFile(targetPath, oldContent, 0644)

	// 元数据记录旧 hash 和过去的 mtime（强制走 SLOW PATH）
	oldHash, _ := ComputeFileHash(source)
	idx := NewMetadataIndex()
	idx.Add(&FileMetadata{
		SourcePath:   source,
		TargetPath:   target,
		ContentHash:  oldHash,
		LastModified: timeMustParse(t, "2020-01-01T00:00:00Z"),
		FileSize:     int64(len(oldContent)),
		Strategy:     StrategyOverwrite,
	})

	// 修改源文件
	os.WriteFile(source, newContent, 0644)

	importer := NewSmartImporter(idx, kbDir)
	result, err := importer.ImportFile(source, target, nil)
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true for content change")
	}

	// 验证目标文件被覆盖
	data, _ := os.ReadFile(targetPath)
	if string(data) != string(newContent) {
		t.Errorf("target not updated: %s", string(data))
	}

	// 验证元数据更新
	meta, _ := idx.GetByTarget(target)
	newHash, _ := ComputeFileHash(source)
	if meta.ContentHash != newHash {
		t.Error("metadata hash should be updated")
	}
}

func TestSmartImportFile_ContentChanged_Skip(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(filepath.Join(kbDir, "api"), 0755)

	source := filepath.Join(tmpDir, "src.md")
	target := "api/skip.md"
	targetPath := filepath.Join(kbDir, target)

	oldContent := []byte("old")
	newContent := []byte("new")
	os.WriteFile(source, oldContent, 0644)
	os.WriteFile(targetPath, oldContent, 0644)

	oldHash, _ := ComputeFileHash(source)
	idx := NewMetadataIndex()
	idx.Add(&FileMetadata{
		SourcePath:   source,
		TargetPath:   target,
		ContentHash:  oldHash,
		LastModified: timeMustParse(t, "2020-01-01T00:00:00Z"),
		FileSize:     int64(len(oldContent)),
		Strategy:     StrategySkip,
	})

	os.WriteFile(source, newContent, 0644)

	importer := NewSmartImporter(idx, kbDir)
	result, err := importer.ImportFile(source, target, nil)
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	t.Logf("Changed=%v Strategy=%v", result.Changed, result.Strategy)
	if result.Changed {
		t.Error("expected Changed=false with skip strategy")
	}

	data, _ := os.ReadFile(targetPath)
	t.Logf("target content=%q", string(data))
	if string(data) != "old" {
		t.Errorf("target should be unchanged: %s", string(data))
	}
}

func TestSmartImportFile_MetaChangedOnly(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(kbDir, 0755)

	source := filepath.Join(tmpDir, "src.md")
	target := "api/meta.md"
	targetPath := filepath.Join(kbDir, target)

	content := []byte("stable content")
	os.WriteFile(source, content, 0644)
	os.WriteFile(targetPath, content, 0644)

	hash, _ := ComputeFileHash(source)
	idx := NewMetadataIndex()
	idx.Add(&FileMetadata{
		SourcePath:   source,
		TargetPath:   target,
		ContentHash:  hash,
		LastModified: timeMustParse(t, "2020-01-01T00:00:00Z"),
		FileSize:     int64(len(content)),
		Strategy:     StrategyOverwrite,
	})

	// 修改 mtime 但内容不变
	now := mustStat(t, source).ModTime()
	if now.Unix() == 0 {
		now = timeMustParse(t, "2024-01-01T00:00:00Z")
	}
	os.Chtimes(source, now, now)

	importer := NewSmartImporter(idx, kbDir)
	result, err := importer.ImportFile(source, target, &SmartImportOptions{
		ForceStrategy: StrategyOverwrite,
	})
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if result.Changed {
		t.Error("expected Changed=false for meta-only change (no merge engine call)")
	}

	// 元数据应已更新
	meta, _ := idx.GetByTarget(target)
	if !meta.LastModified.Equal(now) {
		t.Errorf("LastModified not updated: %v vs %v", meta.LastModified, now)
	}
}

func TestSmartImportFile_SourceMissing(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(kbDir, 0755)

	source := filepath.Join(tmpDir, "deleted.md")
	target := "api/missing.md"

	idx := NewMetadataIndex()
	idx.Add(&FileMetadata{
		SourcePath:   source,
		TargetPath:   target,
		ContentHash:  "oldhash",
		LastModified: timeMustParse(t, "2024-01-01T00:00:00Z"),
		FileSize:     100,
	})

	importer := NewSmartImporter(idx, kbDir)
	result, err := importer.ImportFile(source, target, nil)
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if result.Changed {
		t.Error("expected Changed=false for missing source")
	}

	meta, _ := idx.GetByTarget(target)
	if meta.SourceExists {
		t.Error("SourceExists should be false")
	}
}

// =============================================================================
// DetectChanges / Sync 测试
// =============================================================================

func TestDetectChanges_ReportsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(filepath.Join(kbDir, "api"), 0755)

	source := filepath.Join(tmpDir, "src.md")
	target := "api/detect.md"
	targetPath := filepath.Join(kbDir, target)

	content := []byte("content")
	os.WriteFile(source, content, 0644)
	os.WriteFile(targetPath, content, 0644)

	// 写入后再 stat，确保 mtime 一致
	srcInfo := mustStat(t, source)
	hash, _ := ComputeFileHash(source)
	idx := NewMetadataIndex()
	idx.Add(&FileMetadata{
		SourcePath:   source,
		TargetPath:   target,
		ContentHash:  hash,
		LastModified: srcInfo.ModTime(),
		FileSize:     srcInfo.Size(),
	})

	importer := NewSmartImporter(idx, kbDir)
	report, err := importer.DetectChanges([]string{source}, []string{target})
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}
	if len(report) != 1 {
		t.Fatalf("expected 1 report entry, got %d", len(report))
	}
	if report[0].Result != DetectNoChange {
		t.Errorf("expected DetectNoChange, got %v", report[0].Result)
	}

	// 验证没有文件被修改
	data, _ := os.ReadFile(targetPath)
	if string(data) != "content" {
		t.Error("DetectChanges should not modify files")
	}
}

func TestSync_PruneMissing(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	os.MkdirAll(kbDir, 0755)

	idx := NewMetadataIndex()
	// 直接写入 map（绕过 Add 避免 SourceExists 自动设 true）
	idx.Files["api/missing1.md"] = &FileMetadata{
		SourcePath:   "/nonexistent/path1.md",
		TargetPath:   "api/missing1.md",
		ContentHash:  "h1",
		SourceExists: false,
	}
	idx.BySource["/nonexistent/path1.md"] = "api/missing1.md"
	idx.Files["api/missing2.md"] = &FileMetadata{
		SourcePath:   "/nonexistent/path2.md",
		TargetPath:   "api/missing2.md",
		ContentHash:  "h2",
		SourceExists: false,
	}
	idx.BySource["/nonexistent/path2.md"] = "api/missing2.md"
	// 一个仍然存在的
	idx.Files["api/keep.md"] = &FileMetadata{
		SourcePath:   "/nonexistent/keep.md",
		TargetPath:   "api/keep.md",
		ContentHash:  "h3",
		SourceExists: true,
	}
	idx.BySource["/nonexistent/keep.md"] = "api/keep.md"

	importer := NewSmartImporter(idx, kbDir)
	pruned, err := importer.Prune()
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", pruned)
	}
	if idx.IsTracked("api/missing1.md") {
		t.Error("missing1 should be pruned")
	}
	if !idx.IsTracked("api/keep.md") {
		t.Error("keep should not be pruned")
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	return info
}

func timeMustParse(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("time parse failed: %v", err)
	}
	return parsed
}
