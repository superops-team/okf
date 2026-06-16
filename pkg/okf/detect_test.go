package okf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// ============================================================================
// Phase 1: 元数据与变更检测 - Tests
// ============================================================================

// ----------------------------------------------------------------------------
// FileMetadata 测试
// ----------------------------------------------------------------------------

func TestFileMetadata_Basic(t *testing.T) {
	meta := FileMetadata{
		SourcePath:   "/home/user/docs/api.md",
		TargetPath:   "api/user.md",
		ContentHash:  "abc123",
		LastModified: time.Now(),
		LastImported: time.Now(),
		FileSize:     1024,
		Strategy:     "overwrite",
	}

	if meta.SourcePath != "/home/user/docs/api.md" {
		t.Errorf("SourcePath mismatch")
	}
	if meta.TargetPath != "api/user.md" {
		t.Errorf("TargetPath mismatch")
	}
	if meta.ContentHash != "abc123" {
		t.Errorf("ContentHash mismatch")
	}
}

// ----------------------------------------------------------------------------
// Hash 计算测试
// ----------------------------------------------------------------------------

func TestComputeFileHash(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	hash, err := ComputeFileHash(testFile)
	if err != nil {
		t.Fatalf("ComputeFileHash failed: %v", err)
	}

	if hash == "" {
		t.Errorf("ComputeFileHash returned empty string")
	}

	// 相同内容应产生相同 hash
	hash2, err := ComputeFileHash(testFile)
	if err != nil {
		t.Fatalf("ComputeFileHash second call failed: %v", err)
	}

	if hash != hash2 {
		t.Errorf("Same content produced different hashes: %s vs %s", hash, hash2)
	}

	// 不同内容应产生不同 hash
	otherFile := filepath.Join(tmpDir, "other.md")
	if err := os.WriteFile(otherFile, []byte("different content"), 0644); err != nil {
		t.Fatalf("Failed to write second test file: %v", err)
	}

	hash3, err := ComputeFileHash(otherFile)
	if err != nil {
		t.Fatalf("ComputeFileHash failed: %v", err)
	}

	if hash == hash3 {
		t.Errorf("Different content produced same hash")
	}
}

func TestComputeFileHash_NotExist(t *testing.T) {
	_, err := ComputeFileHash("/non/existent/path/file.md")
	if err == nil {
		t.Error("ComputeFileHash on non-existent file should return error")
	}
}

// ----------------------------------------------------------------------------
// MetadataIndex 测试
// ----------------------------------------------------------------------------

func TestMetadataIndex_New(t *testing.T) {
	index := NewMetadataIndex()

	if index.Files == nil {
		t.Error("Files map should be initialized")
	}
	if index.BySource == nil {
		t.Error("BySource map should be initialized")
	}
	if len(index.Files) != 0 {
		t.Error("New index should have 0 files")
	}
}

func TestMetadataIndex_AddAndGet(t *testing.T) {
	index := NewMetadataIndex()

	meta := &FileMetadata{
		SourcePath: "/source/docs/api.md",
		TargetPath: "api/docs.md",
		ContentHash: "abc123",
	}

	err := index.Add(meta)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// 通过 TargetPath 获取
	m, found := index.GetByTarget("api/docs.md")
	if !found || m.ContentHash != "abc123" {
		t.Errorf("GetByTarget failed: %+v", m)
	}

	// 通过 SourcePath 获取
	m, found = index.GetBySource("/source/docs/api.md")
	if !found || m.ContentHash != "abc123" {
		t.Errorf("GetBySource failed: %+v", m)
	}

	if !index.IsTracked("api/docs.md") {
		t.Error("IsTracked returned false for tracked file")
	}
}

func TestMetadataIndex_AddDuplicateTarget(t *testing.T) {
	index := NewMetadataIndex()

	meta1 := &FileMetadata{
		SourcePath: "/source/docs/api.md",
		TargetPath: "api/docs.md",
		ContentHash: "abc123",
	}

	meta2 := &FileMetadata{
		SourcePath: "/other/docs/api.md",
		TargetPath: "api/docs.md",
		ContentHash: "def456",
	}

	err := index.Add(meta1)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	err = index.Add(meta2)
	if err == nil {
		t.Error("Adding duplicate target path should fail")
	}
}

func TestMetadataIndex_Update(t *testing.T) {
	index := NewMetadataIndex()

	meta := &FileMetadata{
		SourcePath: "/source/docs/api.md",
		TargetPath: "api/docs.md",
		ContentHash: "abc123",
		FileSize: 100,
	}

	index.Add(meta)

	// Update with new hash
	updatedMeta := &FileMetadata{
		SourcePath: "/source/docs/api.md",
		TargetPath: "api/docs.md",
		ContentHash: "def456",
		FileSize: 200,
	}

	err := index.Update(updatedMeta)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	m, _ := index.GetByTarget("api/docs.md")
	if m.ContentHash != "def456" {
		t.Errorf("Update did not update hash: got %s", m.ContentHash)
	}
	if m.FileSize != 200 {
		t.Errorf("Update did not update size: got %d", m.FileSize)
	}
}

func TestMetadataIndex_Delete(t *testing.T) {
	index := NewMetadataIndex()

	meta := &FileMetadata{
		SourcePath: "/source/docs/api.md",
		TargetPath: "api/docs.md",
		ContentHash: "abc123",
	}

	index.Add(meta)

	err := index.DeleteByTarget("api/docs.md")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if index.IsTracked("api/docs.md") {
		t.Error("File should not be tracked after deletion")
	}

	if _, found := index.GetBySource("/source/docs/api.md"); found {
		t.Error("Source should not be found after deletion")
	}
}

// ----------------------------------------------------------------------------
// 持久化测试
// ----------------------------------------------------------------------------

func TestMetadataIndex_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".metadata.json")

	// Create and populate
	index := NewMetadataIndex()

	meta1 := &FileMetadata{
		SourcePath: "/source/docs/api.md",
		TargetPath: "api/docs.md",
		ContentHash: "abc123",
		LastModified: time.Now(),
		LastImported: time.Now(),
		FileSize: 100,
		Strategy: "overwrite",
	}

	meta2 := &FileMetadata{
		SourcePath: "/source/docs/concept.md",
		TargetPath: "concepts/design.md",
		ContentHash: "def456",
		LastModified: time.Now(),
		LastImported: time.Now(),
		FileSize: 200,
		Strategy: "merge",
	}

	index.Add(meta1)
	index.Add(meta2)

	// Save
	err := index.Save(indexPath)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file created
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatalf("Save did not create metadata file")
	}

	// Load
	loaded := NewMetadataIndex()
	err = loaded.Load(indexPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify content
	if !loaded.IsTracked("api/docs.md") {
		t.Error("Loaded index missing api/docs.md")
	}

	if !loaded.IsTracked("concepts/design.md") {
		t.Error("Loaded index missing concepts/design.md")
	}

	m, found := loaded.GetBySource("/source/docs/api.md")
	if !found || m.ContentHash != "abc123" {
		t.Errorf("Loaded index has wrong content: %+v", m)
	}

	if loaded.Version != MetadataVersion {
		t.Errorf("Loaded index has wrong version: %s", loaded.Version)
	}
}

func TestMetadataIndex_LoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".metadata.json")

	// Load non-existent file should not error and return empty index
	index := NewMetadataIndex()
	err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("Load non-existent file should not error: %v", err)
	}

	if len(index.Files) != 0 {
		t.Error("Loaded non-existent file should return empty index")
	}
}

func TestMetadataIndex_LoadCorrupted(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".metadata.json")

	// Write corrupted json
	err := os.WriteFile(indexPath, []byte("this is not json"), 0644)
	if err != nil {
		t.Fatalf("Failed to write corrupt file: %v", err)
	}

	index := NewMetadataIndex()
	err = index.Load(indexPath)
	if err == nil {
		t.Error("Load corrupted file should fail")
	}
}

func TestMetadataIndex_SaveCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Deep nested path
	indexPath := filepath.Join(tmpDir, "a", "b", "c", ".metadata.json")

	index := NewMetadataIndex()
	meta := &FileMetadata{
		SourcePath: "/source/api.md",
		TargetPath: "api.md",
		ContentHash: "hash",
	}
	index.Add(meta)

	err := index.Save(indexPath)
	if err != nil {
		t.Fatalf("Save with directory creation failed: %v", err)
	}

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("Metadata file not created")
	}
}

// ----------------------------------------------------------------------------
// 变更检测测试
// ----------------------------------------------------------------------------

func TestDetectChanges_FastPath_NoChange(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "api.md")

	content := []byte("test content")
	err := os.WriteFile(source, content, 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Get current file info
	info, _ := os.Stat(source)
	hash, _ := ComputeFileHash(source)

	// Create index with matching metadata
	index := NewMetadataIndex()
	meta := &FileMetadata{
		SourcePath: source,
		TargetPath: "api.md",
		ContentHash: hash,
		LastModified: info.ModTime(),
		FileSize: info.Size(),
	}
	index.Add(meta)

	// Detect - should detect no change (FAST PATH)
	result := index.DetectChange(source)
	if result != DetectNoChange {
		t.Errorf("Expected DetectNoChange, got %v", result)
	}
}

func TestDetectChanges_SlowPath_ContentChanged(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "api.md")

	// Write initial content
	os.WriteFile(source, []byte("initial content"), 0644)

	info, _ := os.Stat(source)
	hash, _ := ComputeFileHash(source)

	// Add to index
	index := NewMetadataIndex()
	meta := &FileMetadata{
		SourcePath: source,
		TargetPath: "api.md",
		ContentHash: hash,
		LastModified: info.ModTime(),
		FileSize: info.Size(),
	}
	index.Add(meta)

	// Modify content (also changes mtime + size)
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(source, []byte("modified content"), 0644)

	// Detect - should detect content changed
	result := index.DetectChange(source)
	if result != DetectContentChanged {
		t.Errorf("Expected DetectContentChanged, got %v", result)
	}
}

func TestDetectChanges_SlowPath_MetaChangedOnly(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "api.md")

	// Write content
	content := []byte("stable content")
	os.WriteFile(source, content, 0644)

	info, _ := os.Stat(source)
	hash, _ := ComputeFileHash(source)

	index := NewMetadataIndex()
	meta := &FileMetadata{
		SourcePath: source,
		TargetPath: "api.md",
		ContentHash: hash,
		LastModified: info.ModTime(),
		FileSize: info.Size(),
	}
	index.Add(meta)

	// Touch file (change mtime but not content)
	// sleep 1.1s 让 mtime 与记录的明显不同（FS 精度通常是秒级）
	time.Sleep(1100 * time.Millisecond)
	now := time.Now()
	os.Chtimes(source, now, now)

	// Detect - should detect metadata changed but content same
	result := index.DetectChange(source)

	// After touch, mtime changed but content hash is same
	// Expected: DetectContentChanged (because we need to re-check)
	// The result depends on our detection logic:
	// If mtime+size differs from record, we compute hash, then:
	//   - hash same → DetectMetaChanged
	//   - hash diff → DetectContentChanged
	if result == DetectNoChange {
		t.Error("Expected DetectMetaChanged or DetectContentChanged after touch")
	}
}

func TestDetectChanges_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "api.md")

	os.WriteFile(source, []byte("new file"), 0644)

	index := NewMetadataIndex()
	// File not in index → new file

	result := index.DetectChange(source)
	if result != DetectNewFile {
		t.Errorf("Expected DetectNewFile, got %v", result)
	}
}

func TestDetectChanges_SourceMissing(t *testing.T) {
	index := NewMetadataIndex()

	// Add record for non-existent file
	meta := &FileMetadata{
		SourcePath: "/non/existent/file.md",
		TargetPath: "file.md",
		ContentHash: "abc123",
	}
	index.Add(meta)

	result := index.DetectChange("/non/existent/file.md")
	if result != DetectSourceMissing {
		t.Errorf("Expected DetectSourceMissing, got %v", result)
	}
}

// ----------------------------------------------------------------------------
// 平台相关测试
// ----------------------------------------------------------------------------

func TestDefaultKnowledgeDir(t *testing.T) {
	dir := DefaultKnowledgeDir()

	if dir == "" {
		t.Fatalf("DefaultKnowledgeDir returned empty string")
	}

	// Verify format by OS
	switch runtime.GOOS {
	case "darwin":
		if !filepath.IsAbs(dir) {
			t.Errorf("DefaultKnowledgeDir on macOS should be absolute path: %s", dir)
		}
	case "linux":
		if !filepath.IsAbs(dir) {
			t.Errorf("DefaultKnowledgeDir on Linux should be absolute path: %s", dir)
		}
	case "windows":
		if !filepath.IsAbs(dir) {
			t.Errorf("DefaultKnowledgeDir on Windows should be absolute path: %s", dir)
		}
	}

	t.Logf("Default knowledge directory: %s (OS: %s)", dir, runtime.GOOS)
}

// ----------------------------------------------------------------------------
// 原子写入测试
// ----------------------------------------------------------------------------

func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "output.md")
	content := []byte("Hello, World!")

	err := AtomicWriteFile(target, content, 0644)
	if err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("Read back failed: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("AtomicWriteFile wrote wrong content")
	}

	// Verify no .tmp file leftover
	tmpFile := target + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("Temp file should be cleaned up")
	}
}

func TestAtomicWriteFile_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "output.md")
	content1 := []byte("Content 1")
	content2 := []byte("Content 2")

	AtomicWriteFile(target, content1, 0644)
	AtomicWriteFile(target, content2, 0644)

	data, _ := os.ReadFile(target)
	if string(data) != "Content 2" {
		t.Errorf("AtomicWriteFile did not overwrite: got %s", string(data))
	}
}
