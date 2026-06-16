package okf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// SourceExists 字段测试
// =============================================================================

func TestFileMetadata_SourceExists_Default(t *testing.T) {
	// Go zero value 是 false；MetadataIndex.Add 会显式设为 true
	meta := &FileMetadata{SourcePath: "/test/file.md", TargetPath: "file.md"}
	if meta.SourceExists {
		t.Error("Go zero value of bool should be false")
	}

	// Add 后默认 true
	idx := NewMetadataIndex()
	if err := idx.Add(meta); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if !meta.SourceExists {
		t.Error("Add should default SourceExists to true")
	}
}

func TestMetadataIndex_DetectChange_MarksSourceMissing(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	os.WriteFile(source, []byte("content"), 0644)

	hash, _ := ComputeFileHash(source)
	info, _ := os.Stat(source)

	idx := NewMetadataIndex()
	idx.Add(&FileMetadata{
		SourcePath:   source,
		TargetPath:   "test/file.md",
		ContentHash:  hash,
		LastModified: info.ModTime(),
		FileSize:     info.Size(),
	})

	// 模拟源文件被删除
	os.Remove(source)

	// SmartImporter 应该标记 SourceExists=false
	importer := NewSmartImporter(idx, tmpDir)
	_, err := importer.ImportFile(source, "test/file.md", nil)
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}

	meta, _ := idx.GetBySource(source)
	if meta == nil {
		t.Fatal("meta should still exist after DetectSourceMissing")
	}
	if meta.SourceExists {
		t.Error("SourceExists should be false after DetectSourceMissing")
	}
}

func TestMetadataIndex_SourceExists_PersistedAsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".metadata.json")

	idx := NewMetadataIndex()
	meta := &FileMetadata{
		SourcePath:   "/test/path.md",
		TargetPath:   "test/path.md",
		ContentHash:  "abc",
		SourceExists: false, // 显式设为 false
	}
	// 直接绕过 Add 写入（避免 Add 自动设 true）
	idx.Files[meta.TargetPath] = meta
	idx.BySource[meta.SourcePath] = meta.TargetPath

	if err := idx.Save(indexPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 读取 JSON 验证字段被持久化
	data, _ := os.ReadFile(indexPath)
	// MarshalIndent 输出带空格，所以匹配 "sourceExists": false 形式
	if !contains(string(data), `"sourceExists": false`) {
		t.Errorf("sourceExists=false should be in JSON, got: %s", string(data))
	}

	// Load 后字段保持
	loaded := NewMetadataIndex()
	loaded.Load(indexPath)
	loadedMeta, _ := loaded.GetByTarget("test/path.md")
	if loadedMeta.SourceExists {
		t.Error("SourceExists should remain false after Load")
	}
}

// =============================================================================
// MigrateIndex 测试
// =============================================================================

func TestMigrateIndex_AlreadyCurrentVersion(t *testing.T) {
	idx := &MetadataIndex{
		Version: MetadataVersion,
		Files:   make(map[string]*FileMetadata),
		BySource: make(map[string]string),
	}

	if err := MigrateIndex(idx); err != nil {
		t.Errorf("MigrateIndex on current version should not error: %v", err)
	}
	if idx.Version != MetadataVersion {
		t.Error("Version should remain unchanged")
	}
}

func TestMigrateIndex_RebuildsBySource(t *testing.T) {
	idx := &MetadataIndex{
		Version: "0.9",
		Files: map[string]*FileMetadata{
			"api/a.md": {SourcePath: "/source/a.md", TargetPath: "api/a.md", SourceExists: false},
			"api/b.md": {SourcePath: "/source/b.md", TargetPath: "api/b.md"},
		},
		BySource: nil, // 旧版本没有反向索引
	}

	if err := MigrateIndex(idx); err != nil {
		t.Fatalf("MigrateIndex failed: %v", err)
	}

	if idx.Version != MetadataVersion {
		t.Errorf("Version should be updated to %s, got %s", MetadataVersion, idx.Version)
	}
	if len(idx.BySource) != 2 {
		t.Errorf("BySource should have 2 entries, got %d", len(idx.BySource))
	}
	if idx.BySource["/source/a.md"] != "api/a.md" {
		t.Error("BySource mapping for a.md incorrect")
	}
	if !idx.Files["api/a.md"].SourceExists {
		t.Error("SourceExists should default to true after migration")
	}
}

func TestMigrateIndex_SetsUpdatedAtFromLastImported(t *testing.T) {
	now := time.Now()
	idx := &MetadataIndex{
		Version: "0.9",
		Files: map[string]*FileMetadata{
			"a.md": {SourcePath: "/a", TargetPath: "a.md", LastImported: now},
		},
		BySource: nil,
	}

	if err := MigrateIndex(idx); err != nil {
		t.Fatalf("MigrateIndex failed: %v", err)
	}

	if idx.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after migration")
	}
}

// =============================================================================
// 兼容性测试：JSON 字段名
// =============================================================================

func TestMetadata_JSONFieldNames_CamelCase(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".metadata.json")

	idx := NewMetadataIndex()
	idx.Add(&FileMetadata{
		SourcePath:  "/source/test.md",
		TargetPath:  "test.md",
		ContentHash: "abc123",
	})

	if err := idx.Save(indexPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, _ := os.ReadFile(indexPath)
	content := string(data)

	// 验证 camelCase 字段名
	expectedFields := []string{
		`"sourcePath"`,
		`"targetPath"`,
		`"contentHash"`,
		`"lastModified"`,
		`"lastImported"`,
		`"fileSize"`,
		`"sourceExists"`,
		`"bySource"`,
		`"updatedAt"`,
	}
	for _, field := range expectedFields {
		if !contains(content, field) {
			t.Errorf("expected JSON to contain %s, got: %s", field, content)
		}
	}

	// 验证 snake_case 字段名不存在
	notExpectedFields := []string{
		`"source_path"`,
		`"target_path"`,
		`"content_hash"`,
		`"last_modified"`,
	}
	for _, field := range notExpectedFields {
		if contains(content, field) {
			t.Errorf("JSON should NOT contain %s, got: %s", field, content)
		}
	}
}

func TestMetadata_Load_SupportsCamelCase(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, ".metadata.json")

	// 手工写 camelCase JSON
	rawJSON := `{
  "version": "1.0",
  "updatedAt": "2024-01-15T10:30:00Z",
  "files": {
    "api/test.md": {
      "sourcePath": "/source/test.md",
      "targetPath": "api/test.md",
      "contentHash": "abc123",
      "lastModified": "2024-01-15T09:00:00Z",
      "lastImported": "2024-01-15T10:30:00Z",
      "fileSize": 100,
      "strategy": "overwrite",
      "sourceExists": true
    }
  },
  "bySource": {
    "/source/test.md": "api/test.md"
  }
}`
	if err := os.WriteFile(indexPath, []byte(rawJSON), 0644); err != nil {
		t.Fatalf("write json: %v", err)
	}

	idx := NewMetadataIndex()
	if err := idx.Load(indexPath); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !idx.IsTracked("api/test.md") {
		t.Error("tracked file missing after load")
	}

	meta, _ := idx.GetByTarget("api/test.md")
	if meta == nil {
		t.Fatal("meta should be loaded")
	}
	if meta.ContentHash != "abc123" {
		t.Errorf("ContentHash mismatch: %s", meta.ContentHash)
	}
	if !meta.SourceExists {
		t.Error("SourceExists should be true")
	}
	if meta.Strategy != StrategyOverwrite {
		t.Errorf("Strategy mismatch: %s", meta.Strategy)
	}
}

// =============================================================================
// ParseTime 兼容性测试（json unmarshal of time.Time）
// =============================================================================

func TestFileMetadata_UnmarshalTime(t *testing.T) {
	rawJSON := `{
  "sourcePath": "/a",
  "targetPath": "a.md",
  "contentHash": "h",
  "lastModified": "2024-01-15T09:00:00Z",
  "lastImported": "2024-01-15T10:30:00Z",
  "fileSize": 100,
  "sourceExists": true
}`
	var meta FileMetadata
	if err := json.Unmarshal([]byte(rawJSON), &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if meta.LastModified.IsZero() {
		t.Error("LastModified should be parsed")
	}
	if meta.LastImported.IsZero() {
		t.Error("LastImported should be parsed")
	}
}
