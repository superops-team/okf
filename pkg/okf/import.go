package okf

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// MaxArchiveSize is the maximum allowed archive size (50MB)
	MaxArchiveSize = 50 * 1024 * 1024

	// MaxFileSize is the maximum allowed single file size (10MB)
	MaxFileSize = 10 * 1024 * 1024
)

// =============================================================================
// Phase 2: File Import - Data Structures
// =============================================================================

// ImportOptions contains options for import operations.
type ImportOptions struct {
	DryRun bool // Preview mode, don't make changes
	Force  bool // Overwrite existing files
	Silent bool // Suppress informational output
}

// DefaultImportOptions returns the default import options.
func DefaultImportOptions() *ImportOptions {
	return &ImportOptions{
		DryRun: false,
		Force:  false,
		Silent: false,
	}
}

// ImportResult contains the result of an import operation.
type ImportResult struct {
	TotalFiles    int
	ImportedFiles int
	SkippedFiles  int
	FailedFiles   int
	Errors        []ImportError
}

// ImportError represents an error that occurred during import.
type ImportError struct {
	FilePath string
	Message  string
	Err      error
}

func (e *ImportError) Error() string {
	return fmt.Sprintf("%s: %s", e.FilePath, e.Message)
}

func SmartImportSource(srcPath, knowledgeDir string, idx *MetadataIndex, opts *SmartImportOptions) (*ImportResult, error) {
	if opts == nil {
		opts = DefaultSmartImportOptions()
	}
	if idx == nil {
		idx = NewMetadataIndex()
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("source path not accessible: %w", err)
	}

	root := srcPath
	archiveSourcePrefix := ""
	cleanup := func() {}
	if IsArchive(srcPath) {
		canonicalArchivePath, err := canonicalArchiveSourcePath(srcPath)
		if err != nil {
			return nil, err
		}
		tmpDir, err := os.MkdirTemp("", "okf-import-*")
		if err != nil {
			return nil, fmt.Errorf("create temp extraction dir: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(tmpDir) }
		defer cleanup()
		if _, err := ExtractArchive(srcPath, tmpDir); err != nil {
			return nil, err
		}
		root = tmpDir
		archiveSourcePrefix = canonicalArchivePath
		info = nil
	}

	var sourceFiles []string
	rootIsDir := info == nil || info.IsDir()
	if rootIsDir {
		sourceFiles, err = CollectFiles(root)
		if err != nil {
			return nil, fmt.Errorf("collect markdown files: %w", err)
		}
	} else if strings.HasSuffix(strings.ToLower(srcPath), ".md") {
		sourceFiles = []string{srcPath}
	}

	result := &ImportResult{}
	importer := NewSmartImporter(idx, knowledgeDir)
	for _, source := range sourceFiles {
		result.TotalFiles++
		target := smartImportTargetPath(source, root, rootIsDir)
		detectSource := source
		if archiveSourcePrefix != "" {
			detectSource = archiveStableSourcePath(archiveSourcePrefix, source, root)
		}

		if opts.DetectOnly || opts.HashOnly {
			detectResult := importer.detectChangeForIdentity(source, detectSource)
			if detectResult == DetectNoChange {
				result.SkippedFiles++
			} else {
				result.ImportedFiles++
			}
			continue
		}

		mergeResult, err := importer.ImportFileWithIdentity(source, detectSource, target, opts)
		if err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, ImportError{FilePath: source, Message: err.Error(), Err: err})
			continue
		}
		if mergeResult != nil && mergeResult.Changed {
			result.ImportedFiles++
		} else {
			result.SkippedFiles++
		}
	}

	return result, nil
}

func smartImportTargetPath(src, srcRoot string, rootIsDir bool) string {
	if !rootIsDir {
		return filepath.Base(src)
	}
	rel, err := filepath.Rel(srcRoot, src)
	if err != nil {
		return filepath.Base(src)
	}
	return rel
}

func archiveStableSourcePath(archivePath, extractedPath, extractRoot string) string {
	rel, err := filepath.Rel(extractRoot, extractedPath)
	if err != nil {
		rel = filepath.Base(extractedPath)
	}
	return archivePath + "::" + filepath.ToSlash(rel)
}

func canonicalArchiveSourcePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize archive path: %w", err)
	}
	if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
		abs = evaluated
	}
	return filepath.Clean(abs), nil
}

// =============================================================================
// Phase 2: File Import - File Collection
// =============================================================================

// CollectFiles recursively collects all markdown files from a directory or returns the file itself if it's a single file.
func CollectFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		// Single file - check if it's markdown
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return []string{}, nil
		}
		return []string{path}, nil
	}

	var files []string

	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only collect markdown files
		if strings.HasSuffix(strings.ToLower(filePath), ".md") {
			files = append(files, filePath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}

// =============================================================================
// Phase 2: File Import - Archive Detection and Extraction
// =============================================================================

// IsArchive returns true if the file is a supported archive format.
// Detection is case-insensitive for compound extensions (.tar.gz, etc.)
// but case-sensitive for simple extensions (.zip, .tar).
func IsArchive(path string) bool {
	lower := strings.ToLower(path)

	// Check for compound extensions (case-insensitive)
	if strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tar.bz2") ||
		strings.HasSuffix(lower, ".tar.xz") {
		return true
	}

	// Check for simple extensions (case-sensitive)
	ext := filepath.Ext(path)
	switch ext {
	case ".zip", ".tar":
		return true
	default:
		return false
	}
}

// ExtractArchive extracts an archive to the destination directory and returns the list of extracted markdown files.
func ExtractArchive(archivePath, destDir string) ([]string, error) {
	// Check file size
	info, err := os.Stat(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat archive: %w", err)
	}

	if info.Size() > MaxArchiveSize {
		return nil, fmt.Errorf("archive exceeds maximum size limit of %d bytes", MaxArchiveSize)
	}

	lower := strings.ToLower(archivePath)

	// Detect archive type
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(archivePath, destDir)
	case strings.HasSuffix(lower, ".tar.gz"):
		return extractTarGz(archivePath, destDir)
	case strings.HasSuffix(lower, ".tar.bz2"):
		return extractTarBz2(archivePath, destDir)
	case strings.HasSuffix(lower, ".tar"):
		return extractTarGz(archivePath, destDir)
	default:
		return nil, fmt.Errorf("unsupported archive format")
	}
}

func extractZip(archivePath, destDir string) ([]string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer reader.Close()

	var extractedFiles []string

	for _, file := range reader.File {
		// Check for path traversal
		cleanName := filepath.Clean(file.Name)
		if err := validateArchiveEntryName(file.Name); err != nil {
			return nil, err
		}
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink entry rejected: %s", file.Name)
		}
		if file.UncompressedSize64 > MaxFileSize {
			return nil, fmt.Errorf("file size exceeds maximum size limit: %s", file.Name)
		}

		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Only extract markdown files
		if !strings.HasSuffix(strings.ToLower(file.Name), ".md") {
			continue
		}

		// Extract the file
		src, err := file.Open()
		if err != nil {
			continue
		}

		targetPath := filepath.Join(destDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			src.Close()
			continue
		}

		targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			src.Close()
			continue
		}

		_, err = io.Copy(targetFile, src)
		src.Close()
		targetFile.Close()

		if err != nil {
			continue
		}

		extractedFiles = append(extractedFiles, targetPath)
	}

	return extractedFiles, nil
}

func extractTarGz(archivePath, destDir string) ([]string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	// Check if it's a .tar file (no gzip)
	info, _ := file.Stat()
	reader := io.Reader(file)

	// Try to detect gzip compression
	if strings.HasSuffix(archivePath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		reader = gzReader
		defer gzReader.Close()
	}

	tarReader := tar.NewReader(reader)
	var extractedFiles []string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Check for path traversal
		cleanName := filepath.Clean(header.Name)
		if err := validateArchiveEntryName(header.Name); err != nil {
			return nil, err
		}
		if err := validateTarEntry(header); err != nil {
			return nil, err
		}

		// Skip directories
		if header.FileInfo().IsDir() {
			continue
		}

		// Only extract markdown files
		if !strings.HasSuffix(strings.ToLower(header.Name), ".md") {
			continue
		}

		targetPath := filepath.Join(destDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			continue
		}

		targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			continue
		}

		_, err = io.Copy(targetFile, tarReader)
		targetFile.Close()

		if err != nil {
			os.Remove(targetPath)
			continue
		}

		extractedFiles = append(extractedFiles, targetPath)
	}

	_ = info // silence unused variable

	return extractedFiles, nil
}

func extractTarBz2(archivePath, destDir string) ([]string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	bz2Reader := bzip2.NewReader(file)
	tarReader := tar.NewReader(bz2Reader)
	var extractedFiles []string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Check for path traversal
		cleanName := filepath.Clean(header.Name)
		if err := validateArchiveEntryName(header.Name); err != nil {
			return nil, err
		}
		if err := validateTarEntry(header); err != nil {
			return nil, err
		}

		// Skip directories
		if header.FileInfo().IsDir() {
			continue
		}

		// Only extract markdown files
		if !strings.HasSuffix(strings.ToLower(header.Name), ".md") {
			continue
		}

		targetPath := filepath.Join(destDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			continue
		}

		targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			continue
		}

		_, err = io.Copy(targetFile, tarReader)
		targetFile.Close()

		if err != nil {
			os.Remove(targetPath)
			continue
		}

		extractedFiles = append(extractedFiles, targetPath)
	}

	return extractedFiles, nil
}

func validateArchiveEntryName(name string) error {
	cleanName := filepath.Clean(name)
	if filepath.IsAbs(name) || filepath.IsAbs(cleanName) {
		return fmt.Errorf("absolute path entry rejected: %s", name)
	}
	if cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) || strings.Contains(cleanName, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return fmt.Errorf("path traversal entry rejected: %s", name)
	}
	return nil
}

func validateTarEntry(header *tar.Header) error {
	if header == nil {
		return fmt.Errorf("nil tar entry")
	}
	if header.Typeflag == tar.TypeSymlink {
		return fmt.Errorf("symlink entry rejected: %s", header.Name)
	}
	if header.Typeflag == tar.TypeLink {
		return fmt.Errorf("hardlink entry rejected: %s", header.Name)
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
		return fmt.Errorf("special file entry rejected: %s", header.Name)
	}
	if header.Size > MaxFileSize {
		return fmt.Errorf("file size exceeds maximum size limit: %s", header.Name)
	}
	return nil
}

// =============================================================================
// Phase 2: File Import - OKF Validation
// =============================================================================

// ValidateConcept validates markdown content as an OKF concept and returns the parsed concept.
func ValidateConcept(content []byte, filePath string) (*Concept, error) {
	concept, err := parseConceptFromBytes(content)
	if err != nil {
		return nil, fmt.Errorf("invalid OKF concept: %w", err)
	}

	// Validate required fields
	if concept.Type == "" {
		return nil, fmt.Errorf("missing required field: type")
	}

	if concept.Title == "" {
		return nil, fmt.Errorf("missing required field: title")
	}

	return concept, nil
}

func ValidateConceptFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read concept file: %w", err)
	}
	_, err = ValidateConcept(content, filePath)
	return err
}

func ValidateOKFMarkdownCandidateFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read concept file: %w", err)
	}
	return ValidateMarkdownFrontmatter(content)
}

func ValidateMarkdownFrontmatter(content []byte) error {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil
	}
	frontmatterLines := bytes.SplitN(content[3:], []byte("\n---"), 2)
	if len(frontmatterLines) < 2 {
		return fmt.Errorf("unterminated frontmatter")
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(frontmatterLines[0], &raw); err != nil {
		return fmt.Errorf("invalid frontmatter: %w", err)
	}
	return nil
}

func parseConceptFromBytes(content []byte) (*Concept, error) {
	concept := &Concept{
		Tags: []string{},
	}

	// Check for YAML frontmatter
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		// No frontmatter - just content
		concept.Content = string(content)
		return concept, nil
	}

	// Find end of frontmatter
	lines := bytes.SplitN(content, []byte("\n"), 4)
	if len(lines) < 3 {
		return concept, nil
	}

	// Skip the first "---" line
	frontmatterLines := bytes.SplitN(content[3:], []byte("\n---"), 2)
	if len(frontmatterLines) < 2 {
		return concept, nil
	}

	frontmatter := frontmatterLines[0]
	contentBody := frontmatterLines[1]
	var raw map[string]interface{}
	if err := yaml.Unmarshal(frontmatter, &raw); err != nil {
		return nil, err
	}

	// Remove leading newline from content if present
	if len(contentBody) > 0 && contentBody[0] == '\n' {
		contentBody = contentBody[1:]
	}

	concept.Content = string(contentBody)

	// Parse frontmatter manually (simple key: value parsing)
	scanner := bufio.NewScanner(bytes.NewReader(frontmatter))
	currentList := ""
	inList := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		// Check for list items
		if strings.HasPrefix(line, "- ") {
			value := strings.TrimPrefix(line, "- ")
			if inList && currentList != "" {
				switch currentList {
				case "tags":
					concept.Tags = append(concept.Tags, value)
				}
			}
			continue
		}

		// Check for key: value
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")

		inList = false

		switch key {
		case "type":
			concept.Type = value
		case "title":
			concept.Title = value
		case "description":
			concept.Description = value
		case "resource":
			concept.Resource = value
		case "tags":
			inList = true
			currentList = key
			if value != "" {
				concept.Tags = []string{value}
			} else {
				concept.Tags = []string{}
			}
		case "timestamp":
			concept.Timestamp = value
		}
	}

	return concept, nil
}

// =============================================================================
// Phase 2: File Import - Import Functions
// =============================================================================

// ImportFile imports a single file to the knowledge base.
func ImportFile(srcPath, dstDir string, opts *ImportOptions) (*ImportResult, error) {
	if opts == nil {
		opts = DefaultImportOptions()
	}

	result := &ImportResult{
		TotalFiles: 1,
	}

	// Check if file exists
	if _, err := os.Stat(srcPath); err != nil {
		result.FailedFiles = 1
		result.Errors = append(result.Errors, ImportError{
			FilePath: srcPath,
			Message:  "file not found",
			Err:      err,
		})
		return result, nil
	}

	// Validate the file
	content, err := os.ReadFile(srcPath)
	if err != nil {
		result.FailedFiles = 1
		result.Errors = append(result.Errors, ImportError{
			FilePath: srcPath,
			Message:  "failed to read file",
			Err:      err,
		})
		return result, nil
	}

	_, err = ValidateConcept(content, srcPath)
	if err != nil {
		result.FailedFiles = 1
		result.Errors = append(result.Errors, ImportError{
			FilePath: srcPath,
			Message:  err.Error(),
			Err:      err,
		})
		return result, nil
	}

	// Determine destination path
	fileName := filepath.Base(srcPath)
	dstPath := filepath.Join(dstDir, fileName)

	// Check if file already exists
	if _, err := os.Stat(dstPath); err == nil && !opts.Force {
		result.SkippedFiles = 1
		return result, nil
	}

	// Dry run mode
	if opts.DryRun {
		return result, nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		result.FailedFiles = 1
		result.Errors = append(result.Errors, ImportError{
			FilePath: srcPath,
			Message:  "failed to create directory",
			Err:      err,
		})
		return result, nil
	}

	// Copy the file
	if err := os.WriteFile(dstPath, content, 0644); err != nil {
		result.FailedFiles = 1
		result.Errors = append(result.Errors, ImportError{
			FilePath: srcPath,
			Message:  "failed to write file",
			Err:      err,
		})
		return result, nil
	}

	result.ImportedFiles = 1
	return result, nil
}

// ImportDirectory imports all markdown files from a directory.
func ImportDirectory(srcDir, dstDir string, opts *ImportOptions) (*ImportResult, error) {
	if opts == nil {
		opts = DefaultImportOptions()
	}

	result := &ImportResult{}

	// Collect files
	files, err := CollectFiles(srcDir)
	if err != nil {
		return nil, fmt.Errorf("failed to collect files: %w", err)
	}

	result.TotalFiles = len(files)

	if len(files) == 0 {
		return result, nil
	}

	// Get absolute path and clean source directory
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	absSrcDir = filepath.Clean(absSrcDir)

	// Determine source directory name for preserving structure
	srcDirName := filepath.Base(absSrcDir)
	if srcDirName == "." || srcDirName == "" {
		srcDirName = filepath.Base(filepath.Dir(absSrcDir))
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	for _, filePath := range files {
		// Calculate relative path from source directory
		absFilePath, err := filepath.Abs(filePath)
		if err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, ImportError{
				FilePath: filePath,
				Message:  "failed to get absolute path",
				Err:      err,
			})
			continue
		}

		relPath, err := filepath.Rel(absSrcDir, absFilePath)
		if err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, ImportError{
				FilePath: filePath,
				Message:  "failed to calculate relative path",
				Err:      err,
			})
			continue
		}

		// Include source directory name to preserve structure
		dstRelPath := filepath.Join(srcDirName, relPath)

		// Read and validate
		content, err := os.ReadFile(filePath)
		if err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, ImportError{
				FilePath: filePath,
				Message:  "failed to read file",
				Err:      err,
			})
			continue
		}

		_, err = ValidateConcept(content, filePath)
		if err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, ImportError{
				FilePath: filePath,
				Message:  err.Error(),
				Err:      err,
			})
			continue
		}

		// Determine destination path
		dstPath := filepath.Join(dstDir, dstRelPath)

		// Check if file already exists
		if _, err := os.Stat(dstPath); err == nil && !opts.Force {
			result.SkippedFiles++
			continue
		}

		// Dry run mode - count as would-be imported but don't actually import
		if opts.DryRun {
			continue
		}

		// Create directory structure
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, ImportError{
				FilePath: filePath,
				Message:  "failed to create directory",
				Err:      err,
			})
			continue
		}

		// Copy the file
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, ImportError{
				FilePath: filePath,
				Message:  "failed to write file",
				Err:      err,
			})
			continue
		}

		result.ImportedFiles++
	}

	return result, nil
}

// Import imports a file, directory, or archive based on the input path.
func Import(srcPath, dstDir string, opts *ImportOptions) (*ImportResult, error) {
	if opts == nil {
		opts = DefaultImportOptions()
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	// If it's an archive, extract first
	if IsArchive(srcPath) {
		return ImportArchive(srcPath, dstDir, opts)
	}

	// If it's a directory
	if info.IsDir() {
		return ImportDirectory(srcPath, dstDir, opts)
	}

	// It's a single file
	return ImportFile(srcPath, dstDir, opts)
}

// ImportArchive extracts an archive and imports its contents.
func ImportArchive(archivePath, dstDir string, opts *ImportOptions) (*ImportResult, error) {
	if opts == nil {
		opts = DefaultImportOptions()
	}

	// Create a temporary directory for extraction
	tmpDir, err := os.MkdirTemp("", "okf-import-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract the archive
	files, err := ExtractArchive(archivePath, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract archive: %w", err)
	}

	if len(files) == 0 {
		result := &ImportResult{}
		result.SkippedFiles = 0
		return result, nil
	}

	// Import the extracted files
	return ImportDirectory(tmpDir, dstDir, opts)
}
