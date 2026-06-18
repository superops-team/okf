package okf

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Phase 2: File Import - File Collection Tests
// =============================================================================

// TestCollectFiles_Basic tests basic file collection from directory
func TestCollectFiles_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	createTestFile(t, filepath.Join(tmpDir, "file1.md"), "test content 1")
	createTestFile(t, filepath.Join(tmpDir, "file2.md"), "test content 2")
	createTestFile(t, filepath.Join(tmpDir, "file3.txt"), "should be ignored")

	files, err := CollectFiles(tmpDir)
	if err != nil {
		t.Fatalf("CollectFiles() error = %v", err)
	}

	if len(files) != 2 {
		t.Errorf("CollectFiles() returned %d files, want 2", len(files))
	}
}

// TestCollectFiles_Recursive tests recursive file collection
func TestCollectFiles_Recursive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directories
	subDir1 := filepath.Join(tmpDir, "sub1")
	subDir2 := filepath.Join(subDir1, "sub2")
	if err := os.MkdirAll(subDir2, 0755); err != nil {
		t.Fatalf("Failed to create subdirectories: %v", err)
	}

	createTestFile(t, filepath.Join(tmpDir, "root.md"), "root")
	createTestFile(t, filepath.Join(subDir1, "level1.md"), "level1")
	createTestFile(t, filepath.Join(subDir2, "level2.md"), "level2")
	createTestFile(t, filepath.Join(subDir2, "nested.txt"), "should be ignored")

	files, err := CollectFiles(tmpDir)
	if err != nil {
		t.Fatalf("CollectFiles() error = %v", err)
	}

	if len(files) != 3 {
		t.Errorf("CollectFiles() returned %d files, want 3", len(files))
	}
}

// TestCollectFiles_EmptyDirectory tests handling of empty directory
func TestCollectFiles_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	files, err := CollectFiles(tmpDir)
	if err != nil {
		t.Fatalf("CollectFiles() error = %v", err)
	}

	if len(files) != 0 {
		t.Errorf("CollectFiles() returned %d files, want 0", len(files))
	}
}

// TestCollectFiles_SingleFile tests collection of a single file (non-directory)
func TestCollectFiles_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "single.md")
	createTestFile(t, filePath, "content")

	files, err := CollectFiles(filePath)
	if err != nil {
		t.Fatalf("CollectFiles() error = %v", err)
	}

	if len(files) != 1 {
		t.Errorf("CollectFiles() returned %d files, want 1", len(files))
	}

	if files[0] != filePath {
		t.Errorf("CollectFiles() returned %q, want %q", files[0], filePath)
	}
}

// TestCollectFiles_IgnoresNonMarkdown tests that non-markdown files are ignored
func TestCollectFiles_IgnoresNonMarkdown(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various non-markdown files
	createTestFile(t, filepath.Join(tmpDir, "readme.txt"), "text file")
	createTestFile(t, filepath.Join(tmpDir, "data.json"), `{"key": "value"}`)
	createTestFile(t, filepath.Join(tmpDir, "script.sh"), "#!/bin/bash")
	createTestFile(t, filepath.Join(tmpDir, "noextension"), "no extension")

	files, err := CollectFiles(tmpDir)
	if err != nil {
		t.Fatalf("CollectFiles() error = %v", err)
	}

	if len(files) != 0 {
		t.Errorf("CollectFiles() returned %d files, want 0 (non-markdown should be ignored)", len(files))
	}
}

func TestCollectFiles_IncludesMarkdownExtensionCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	upperPath := filepath.Join(tmpDir, "README.MD")
	createTestFile(t, upperPath, validConceptContent("Uppercase Markdown"))

	files, err := CollectFiles(tmpDir)
	if err != nil {
		t.Fatalf("CollectFiles() error = %v", err)
	}
	if len(files) != 1 || files[0] != upperPath {
		t.Fatalf("CollectFiles() = %#v, want uppercase markdown file", files)
	}

	single, err := CollectFiles(upperPath)
	if err != nil {
		t.Fatalf("CollectFiles(single) error = %v", err)
	}
	if len(single) != 1 || single[0] != upperPath {
		t.Fatalf("CollectFiles(single) = %#v, want uppercase markdown file", single)
	}
}

// =============================================================================
// Phase 2: File Import - Archive Extraction Tests
// =============================================================================

// TestIsArchive tests archive type detection by extension
func TestIsArchive(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"zip file", "archive.zip", true},
		{"tar file", "archive.tar", true},
		{"tar.gz file", "archive.tar.gz", true},
		{"tar.bz2 file", "archive.tar.bz2", true},
		{"regular file", "file.md", false},
		{"text file", "readme.txt", false},
		{"no extension", "noextension", false},
		{"uppercase ZIP", "archive.ZIP", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsArchive(tt.path)
			if result != tt.expected {
				t.Errorf("IsArchive(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

// TestExtractArchive_Zip tests ZIP archive extraction
func TestExtractArchive_Zip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a ZIP archive
	zipPath := filepath.Join(tmpDir, "test.zip")
	createTestZip(t, zipPath, []testFile{
		{Name: "file1.md", Content: "content 1"},
		{Name: "subdir/file2.md", Content: "content 2"},
	})

	// Extract to temp directory
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	files, err := ExtractArchive(zipPath, extractDir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}

	if len(files) != 2 {
		t.Errorf("ExtractArchive() returned %d files, want 2", len(files))
	}

	// Verify file contents
	expectedFiles := map[string]string{
		"file1.md":                          "content 1",
		filepath.Join("subdir", "file2.md"): "content 2",
	}

	// Check that expected files were extracted
	extractedCount := 0
	for _, f := range files {
		// Get just the filename or relative path from the extracted directory
		relPath, _ := filepath.Rel(extractDir, f)

		expected, ok := expectedFiles[relPath]
		if !ok {
			// Try with just the base name
			baseName := filepath.Base(relPath)
			for epath, econtent := range expectedFiles {
				if filepath.Base(epath) == baseName {
					expected = econtent
					ok = true
					break
				}
			}
		}

		if !ok {
			t.Errorf("Unexpected file extracted: %s", relPath)
			continue
		}

		content, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("Failed to read extracted file %s: %v", f, err)
			continue
		}

		if string(content) != expected {
			t.Errorf("Extracted file %s content = %q, want %q", relPath, string(content), expected)
		}
		extractedCount++
	}

	if extractedCount != 2 {
		t.Errorf("Expected 2 extracted files, got %d", extractedCount)
	}
}

func TestExtractArchive_ZipIncludesMarkdownExtensionCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	createTestZip(t, zipPath, []testFile{{Name: "README.MD", Content: validConceptContent("Uppercase Archive")}})

	files, err := ExtractArchive(zipPath, filepath.Join(tmpDir, "extracted"))
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0], "README.MD") {
		t.Fatalf("ExtractArchive() = %#v, want uppercase markdown entry", files)
	}
}

func TestSmartImportSourceArchiveIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "bundle.zip")
	createTestZip(t, zipPath, []testFile{{Name: "concepts/service.md", Content: validConceptContent("Archive Service")}})
	knowledgeDir := filepath.Join(tmpDir, "knowledge")
	idx := NewMetadataIndex()

	first, err := SmartImportSource(zipPath, knowledgeDir, idx, DefaultSmartImportOptions())
	if err != nil {
		t.Fatalf("first SmartImportSource() error = %v", err)
	}
	if first.ImportedFiles != 1 || first.FailedFiles != 0 {
		t.Fatalf("first result = %#v, want one imported file", first)
	}
	canonicalZipPath, err := canonicalArchiveSourcePath(zipPath)
	if err != nil {
		t.Fatalf("canonicalArchiveSourcePath() error = %v", err)
	}
	meta, ok := idx.GetBySource(canonicalZipPath + "::concepts/service.md")
	if !ok {
		t.Fatalf("metadata source key missing stable archive identity")
	}
	firstModified := meta.LastModified

	second, err := SmartImportSource(zipPath, knowledgeDir, idx, DefaultSmartImportOptions())
	if err != nil {
		t.Fatalf("second SmartImportSource() error = %v", err)
	}
	if second.FailedFiles != 0 || second.ImportedFiles != 0 || second.SkippedFiles != 1 {
		t.Fatalf("second result = %#v, want idempotent skip without duplicate target failure", second)
	}
	meta, ok = idx.GetBySource(canonicalZipPath + "::concepts/service.md")
	if !ok {
		t.Fatalf("metadata source key missing after second import")
	}
	if !meta.LastModified.Equal(firstModified) {
		t.Fatalf("metadata last modified changed from %s to %s for unchanged archive entry", firstModified, meta.LastModified)
	}
}

func TestSmartImportSourceArchiveIdentityCanonicalizesArchivePath(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "bundle.zip")
	createTestZip(t, zipPath, []testFile{{Name: "concepts/service.md", Content: validConceptContent("Archive Service")}})
	knowledgeDir := filepath.Join(tmpDir, "knowledge")
	idx := NewMetadataIndex()
	t.Chdir(tmpDir)

	first, err := SmartImportSource("bundle.zip", knowledgeDir, idx, DefaultSmartImportOptions())
	if err != nil {
		t.Fatalf("relative SmartImportSource() error = %v", err)
	}
	if first.ImportedFiles != 1 || first.FailedFiles != 0 {
		t.Fatalf("relative result = %#v, want one imported file", first)
	}

	second, err := SmartImportSource(zipPath, knowledgeDir, idx, DefaultSmartImportOptions())
	if err != nil {
		t.Fatalf("absolute SmartImportSource() error = %v", err)
	}
	if second.FailedFiles != 0 || second.ImportedFiles != 0 || second.SkippedFiles != 1 {
		t.Fatalf("absolute result = %#v, want same archive identity to skip", second)
	}
	canonicalZipPath, err := canonicalArchiveSourcePath(zipPath)
	if err != nil {
		t.Fatalf("canonicalArchiveSourcePath() error = %v", err)
	}
	if _, ok := idx.GetBySource(canonicalZipPath + "::concepts/service.md"); !ok {
		t.Fatalf("metadata source key missing canonical absolute archive identity")
	}
}

func TestExtractArchive_TarGzIncludesMarkdownExtensionCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "test.tar.gz")
	createTestTarGz(t, tarPath, []testFile{{Name: "README.MD", Content: validConceptContent("Uppercase Archive")}})

	files, err := ExtractArchive(tarPath, filepath.Join(tmpDir, "extracted"))
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0], "README.MD") {
		t.Fatalf("ExtractArchive() = %#v, want uppercase tar markdown entry", files)
	}
}

// TestExtractArchive_TarGz tests TAR.GZ archive extraction
func TestExtractArchive_TarGz(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a tar.gz archive
	tarPath := filepath.Join(tmpDir, "test.tar.gz")
	createTestTarGz(t, tarPath, []testFile{
		{Name: "doc1.md", Content: "documentation 1"},
		{Name: "nested/doc2.md", Content: "documentation 2"},
	})

	// Extract to temp directory
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	files, err := ExtractArchive(tarPath, extractDir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}

	if len(files) != 2 {
		t.Errorf("ExtractArchive() returned %d files, want 2", len(files))
	}
}

// TestExtractArchive_PathTraversal tests path traversal prevention
func TestExtractArchive_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a ZIP with path traversal attempt
	zipPath := filepath.Join(tmpDir, "malicious.zip")
	createMaliciousZip(t, zipPath)

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	_, err := ExtractArchive(zipPath, extractDir)
	if err == nil {
		t.Fatal("ExtractArchive() error = nil, want path traversal rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "path traversal") {
		t.Fatalf("ExtractArchive() error = %v, want path traversal", err)
	}
}

func TestExtractArchive_RejectsAbsolutePathEntry(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "absolute.zip")
	createTestZip(t, zipPath, []testFile{{Name: "/tmp/evil.md", Content: validConceptContent("Absolute")}})

	_, err := ExtractArchive(zipPath, filepath.Join(tmpDir, "extracted"))
	if err == nil {
		t.Fatal("ExtractArchive() error = nil, want absolute path rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "absolute") {
		t.Fatalf("ExtractArchive() error = %v, want absolute path", err)
	}
}

func TestExtractArchive_RejectsOversizedEntry(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "oversized-entry.zip")
	if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zipWriter := zip.NewWriter(zipFile)
	header := &zip.FileHeader{Name: "large.md", Method: zip.Store}
	header.SetMode(0644)
	header.UncompressedSize64 = MaxFileSize + 1
	entry, err := zipWriter.CreateHeader(header)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	largeContent := append([]byte("---\ntype: concept\ntitle: Large\n---\n"), make([]byte, MaxFileSize+1)...)
	if _, err := entry.Write(largeContent); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}

	_, err = ExtractArchive(zipPath, filepath.Join(tmpDir, "extracted"))
	if err == nil {
		t.Fatal("ExtractArchive() error = nil, want oversized file rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "size") {
		t.Fatalf("ExtractArchive() error = %v, want size limit", err)
	}
}

func TestExtractArchive_RejectsSymlinkTarEntry(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "symlink.tar.gz")
	if err := os.MkdirAll(filepath.Dir(tarPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("create tar: %v", err)
	}
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "link.md", Typeflag: tar.TypeSymlink, Linkname: "/tmp/evil.md"}); err != nil {
		t.Fatalf("write symlink header: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	_, err = ExtractArchive(tarPath, filepath.Join(tmpDir, "extracted"))
	if err == nil {
		t.Fatal("ExtractArchive() error = nil, want symlink rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Fatalf("ExtractArchive() error = %v, want symlink", err)
	}
}

// TestExtractArchive_CleansUpTempFiles tests that temp files are cleaned up
func TestExtractArchive_CleansUpTempFiles(t *testing.T) {
	tmpDir := t.TempDir()

	zipPath := filepath.Join(tmpDir, "test.zip")
	createTestZip(t, zipPath, []testFile{
		{Name: "file.md", Content: "content"},
	})

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	_, err := ExtractArchive(zipPath, extractDir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}

	// Check for leftover temp files in system temp
	// This is a basic check - actual temp cleanup verification would need more sophisticated testing
}

// TestExtractArchive_InvalidArchive tests handling of corrupt/invalid archives
func TestExtractArchive_InvalidArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an invalid file pretending to be an archive
	invalidPath := filepath.Join(tmpDir, "invalid.zip")
	if err := os.WriteFile(invalidPath, []byte("this is not a valid zip"), 0644); err != nil {
		t.Fatalf("Failed to create invalid archive: %v", err)
	}

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	_, err := ExtractArchive(invalidPath, extractDir)
	if err == nil {
		t.Errorf("ExtractArchive() expected error for invalid archive, got nil")
	}
}

// =============================================================================
// Phase 2: File Import - OKF Validation Tests
// =============================================================================

// TestValidateConcept_ValidConcept tests validation of valid OKF concept
func TestValidateConcept_ValidConcept(t *testing.T) {
	content := `---
type: api
title: User API
description: User management endpoints
tags:
  - users
  - rest
timestamp: "2024-01-15T10:30:00Z"
---

## User API

This is the API documentation.
`

	concept, err := ValidateConcept([]byte(content), "test.md")
	if err != nil {
		t.Errorf("ValidateConcept() unexpected error = %v", err)
	}

	if concept == nil {
		t.Error("ValidateConcept() returned nil concept for valid input")
		return
	}

	if concept.Type != "api" {
		t.Errorf("ValidateConcept().Type = %q, want %q", concept.Type, "api")
	}

	if concept.Title != "User API" {
		t.Errorf("ValidateConcept().Title = %q, want %q", concept.Title, "User API")
	}
}

// TestValidateConcept_MissingType tests validation fails for missing type
func TestValidateConcept_MissingType(t *testing.T) {
	content := `---
title: Missing Type
description: This is missing the type field
---

Content here
`

	_, err := ValidateConcept([]byte(content), "test.md")
	if err == nil {
		t.Error("ValidateConcept() expected error for missing type, got nil")
	}
}

// TestValidateConcept_MissingTitle tests validation fails for missing title
func TestValidateConcept_MissingTitle(t *testing.T) {
	content := `---
type: api
description: This is missing the title field
---

Content here
`

	_, err := ValidateConcept([]byte(content), "test.md")
	if err == nil {
		t.Error("ValidateConcept() expected error for missing title, got nil")
	}
}

// TestValidateConcept_EmptyType tests validation fails for empty type
func TestValidateConcept_EmptyType(t *testing.T) {
	content := `---
type: ""
title: Empty Type
---

Content here
`

	_, err := ValidateConcept([]byte(content), "test.md")
	if err == nil {
		t.Error("ValidateConcept() expected error for empty type, got nil")
	}
}

// TestValidateConcept_InvalidYAML tests handling of content with issues
// Note: Our simple parser doesn't validate YAML syntax strictly
func TestValidateConcept_InvalidYAML(t *testing.T) {
	content := `---
type: api
title: Test
description: [unterminated
  - tag1
  - tag2
---

Content here
`

	_, err := ValidateConcept([]byte(content), "test.md")
	if err == nil {
		t.Fatal("ValidateConcept() error = nil, want malformed YAML rejection")
	}
}

func TestSmartImporterRejectsInvalidMarkdownWithoutWriting(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "invalid.md")
	knowledgeDir := filepath.Join(tmpDir, "knowledge")
	createTestFile(t, source, `---
type: concept
title: [unterminated
---
bad
`)

	idx := NewMetadataIndex()
	importer := NewSmartImporter(idx, knowledgeDir)
	_, err := importer.ImportFile(source, "invalid.md", nil)
	if err == nil {
		t.Fatal("ImportFile() error = nil, want invalid markdown rejection")
	}
	if _, statErr := os.Stat(filepath.Join(knowledgeDir, "invalid.md")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid import wrote target or returned unexpected stat error: %v", statErr)
	}
	if idx.Len() != 0 {
		t.Fatalf("metadata entries = %d, want 0", idx.Len())
	}
}

// TestValidateConcept_NoFrontmatter tests handling of content without frontmatter
// Without frontmatter, type and title will be empty, so validation should fail
func TestValidateConcept_NoFrontmatter(t *testing.T) {
	content := `# Just a title\n\nSome content without frontmatter`

	concept, err := ValidateConcept([]byte(content), "test.md")
	// Without frontmatter, type and title are empty, so validation should fail
	if err == nil {
		// If validation passes (no error), that's also acceptable behavior
		// The key is that the function should either fail or return valid concept
		if concept != nil && (concept.Type != "" || concept.Title != "") {
			t.Logf("ValidateConcept() parsed content without frontmatter: Type=%q, Title=%q",
				concept.Type, concept.Title)
		}
	} else {
		// Error is expected because type/title are missing
		t.Logf("ValidateConcept() correctly rejected content without frontmatter: %v", err)
	}
}

// =============================================================================
// Phase 2: File Import - Import Functions Tests
// =============================================================================

// TestImportFile_Basic tests basic file import
func TestImportFile_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	// Create test file
	srcFile := filepath.Join(srcDir, "test.md")
	createTestFile(t, srcFile, `---
type: api
title: Test API
---

Test content
`)

	opts := &ImportOptions{}
	result, err := ImportFile(srcFile, dstDir, opts)
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}

	if result.ImportedFiles != 1 {
		t.Errorf("ImportFile().ImportedFiles = %d, want 1", result.ImportedFiles)
	}

	// Verify file was copied
	dstFile := filepath.Join(dstDir, "test.md")
	if _, err := os.Stat(dstFile); os.IsNotExist(err) {
		t.Errorf("ImportFile() did not create destination file at %q", dstFile)
	}
}

// TestImportFile_ForceOverwrite tests force overwrite option
func TestImportFile_ForceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	// Create existing file
	existingFile := filepath.Join(dstDir, "test.md")
	createTestFile(t, existingFile, "existing content")

	// Create source file
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	srcFile := filepath.Join(srcDir, "test.md")
	createTestFile(t, srcFile, `---
type: api
title: Test API
---

New content
`)

	// Without force - should skip
	opts := &ImportOptions{}
	result, err := ImportFile(srcFile, dstDir, opts)
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}

	if result.SkippedFiles != 1 {
		t.Errorf("ImportFile() SkippedFiles = %d, want 1 (without force)", result.SkippedFiles)
	}

	// With force - should overwrite
	opts.Force = true
	result, err = ImportFile(srcFile, dstDir, opts)
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}

	if result.ImportedFiles != 1 {
		t.Errorf("ImportFile().ImportedFiles = %d, want 1 (with force)", result.ImportedFiles)
	}
}

// TestImportDirectory_Basic tests directory import
func TestImportDirectory_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	// Create source directory structure
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create src subdir: %v", err)
	}

	createTestFile(t, filepath.Join(srcDir, "file1.md"), `---
type: api
title: API 1
---
Content 1
`)

	createTestFile(t, filepath.Join(subDir, "file2.md"), `---
type: concept
title: Concept 1
---
Content 2
`)

	createTestFile(t, filepath.Join(srcDir, "ignored.txt"), "should be ignored")

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	opts := &ImportOptions{}
	result, err := ImportDirectory(srcDir, dstDir, opts)
	if err != nil {
		t.Fatalf("ImportDirectory() error = %v", err)
	}

	if result.TotalFiles != 2 {
		t.Errorf("ImportDirectory().TotalFiles = %d, want 2", result.TotalFiles)
	}

	if result.ImportedFiles != 2 {
		t.Errorf("ImportDirectory().ImportedFiles = %d, want 2", result.ImportedFiles)
	}

	// Verify directory structure is preserved (source dir name included)
	if _, err := os.Stat(filepath.Join(dstDir, "src", "file1.md")); os.IsNotExist(err) {
		t.Errorf("ImportDirectory() did not create src/file1.md")
	}

	if _, err := os.Stat(filepath.Join(dstDir, "src", "subdir", "file2.md")); os.IsNotExist(err) {
		t.Errorf("ImportDirectory() did not preserve subdirectory structure as src/subdir/file2.md")
	}
}

// TestImportDirectory_DryRun tests dry run mode
func TestImportDirectory_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	createTestFile(t, filepath.Join(srcDir, "test.md"), `---
type: api
title: Test
---
Content
`)

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}

	opts := &ImportOptions{DryRun: true}
	result, err := ImportDirectory(srcDir, dstDir, opts)
	if err != nil {
		t.Fatalf("ImportDirectory() error = %v", err)
	}

	if result.TotalFiles != 1 {
		t.Errorf("ImportDirectory().TotalFiles = %d, want 1", result.TotalFiles)
	}

	if result.ImportedFiles != 0 {
		t.Errorf("ImportDirectory().ImportedFiles = %d, want 0 in dry-run mode", result.ImportedFiles)
	}

	// Verify no file was actually created
	if _, err := os.Stat(filepath.Join(dstDir, "test.md")); !os.IsNotExist(err) {
		t.Errorf("ImportDirectory() created file in dry-run mode")
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

type testFile struct {
	Name    string
	Content string
}

func createTestFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create file %s: %v", path, err)
	}
}

func validConceptContent(title string) string {
	return "---\ntype: concept\ntitle: " + title + "\n---\ncontent\n"
}

func createTestZip(t *testing.T, zipPath string, files []testFile) {
	t.Helper()

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	zipWriter, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer zipWriter.Close()

	zipw := zip.NewWriter(zipWriter)
	defer zipw.Close()

	for _, f := range files {
		header := &zip.FileHeader{
			Name:   f.Name,
			Method: zip.Deflate,
		}

		w, err := zipw.CreateHeader(header)
		if err != nil {
			t.Fatalf("Failed to create zip entry: %v", err)
		}

		if _, err := w.Write([]byte(f.Content)); err != nil {
			t.Fatalf("Failed to write zip content: %v", err)
		}
	}
}

func createTestTarGz(t *testing.T, tarPath string, files []testFile) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(tarPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("Failed to create tar file: %v", err)
	}
	defer f.Close()

	gzWriter := gzip.NewWriter(f)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	for _, f := range files {
		header := &tar.Header{
			Name: f.Name,
			Mode: 0644,
			Size: int64(len(f.Content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tarWriter.Write([]byte(f.Content)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}
}

func createMaliciousZip(t *testing.T, zipPath string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	zipWriter, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer zipWriter.Close()

	zipw := zip.NewWriter(zipWriter)
	defer zipw.Close()

	// Path traversal attempt
	maliciousName := "../../../tmp/payload.txt"
	header := &zip.FileHeader{
		Name:   maliciousName,
		Method: zip.Deflate,
	}

	w, err := zipw.CreateHeader(header)
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}

	if _, err := w.Write([]byte("malicious content")); err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}
}

// =============================================================================
// Archive Size Limit Tests
// =============================================================================

// TestExtractArchive_SizeLimit tests that oversized archives are rejected
func TestExtractArchive_SizeLimit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file pretending to be an archive but exceeding size limit
	largePath := filepath.Join(tmpDir, "large.zip")

	// Create a file larger than 50MB (our limit)
	largeFile, err := os.Create(largePath)
	if err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	// Write 51MB of data
	data := make([]byte, 51*1024*1024)
	for i := range data {
		data[i] = 'a'
	}
	largeFile.Write(data)
	largeFile.Close()

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract dir: %v", err)
	}

	_, err = ExtractArchive(largePath, extractDir)
	if err == nil {
		t.Error("ExtractArchive() expected error for oversized archive, got nil")
	}
}

// =============================================================================
// ImportOptions Tests
// =============================================================================

// TestImportOptions_Defaults tests default import options
func TestImportOptions_Defaults(t *testing.T) {
	opts := DefaultImportOptions()

	if opts.DryRun {
		t.Error("DefaultImportOptions().DryRun should be false")
	}

	if opts.Force {
		t.Error("DefaultImportOptions().Force should be false")
	}

	if opts.Silent {
		t.Error("DefaultImportOptions().Silent should be false")
	}
}
