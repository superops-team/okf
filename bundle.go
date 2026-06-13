package okf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadBundle reads a knowledge bundle from a directory.
// The directory should contain markdown files with YAML frontmatter.
func LoadBundle(path string, opts *LoadOptions) (*KnowledgeBundle, error) {
	if opts == nil {
		opts = DefaultLoadOptions()
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", path)
	}

	bundle := &KnowledgeBundle{
		RootPath: path,
		Name:     filepath.Base(path),
	}

	// Find all markdown files
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(filepath.Base(filePath), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process markdown files
		if !strings.HasSuffix(filePath, ".md") {
			return nil
		}

		// Apply filter if provided
		if opts.FilterFunc != nil && !opts.FilterFunc(filePath, info) {
			return nil
		}

		// Skip non-recursive if in subdirectory
		if !opts.Recursive {
			relPath, _ := filepath.Rel(path, filePath)
			if strings.Contains(relPath, string(filepath.Separator)) {
				return nil
			}
		}

		// Parse the concept
		concept, err := ParseConcept(filePath)
		if err != nil {
			// Log error but continue processing other files
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", filePath, err)
			return nil
		}

		// Set relative path from bundle root
		relPath, _ := filepath.Rel(path, filePath)
		concept.FilePath = relPath

		bundle.Concepts = append(bundle.Concepts, concept)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return bundle, nil
}

// SaveBundle writes a knowledge bundle to a directory.
// Each concept is saved as a separate markdown file.
func SaveBundle(b *KnowledgeBundle, path string, opts *SaveOptions) error {
	if opts == nil {
		opts = DefaultSaveOptions()
	}

	// Create root directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Track which paths have been written to avoid duplicates
	writtenPaths := make(map[string]bool)

	for _, concept := range b.Concepts {
		// Determine file path
		filePath := concept.FilePath
		if filePath == "" {
			// Generate path from title
			filename := sanitizeFilename(concept.Title) + ".md"
			if concept.Type != "" {
				filePath = filepath.Join(concept.Type+"s", filename)
			} else {
				filePath = filepath.Join("concepts", filename)
			}
		}

		// Make path relative if absolute
		if filepath.IsAbs(filePath) {
			relPath, _ := filepath.Rel(path, filePath)
			filePath = relPath
		}

		// Handle duplicate paths
		baseFilePath := filePath
		counter := 1
		for writtenPaths[filePath] {
			ext := filepath.Ext(baseFilePath)
			name := strings.TrimSuffix(baseFilePath, ext)
			filePath = fmt.Sprintf("%s_%d%s", name, counter, ext)
			counter++
		}
		writtenPaths[filePath] = true

		// Full path
		fullPath := filepath.Join(path, filePath)

		// Create parent directories
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Serialize concept
		data, err := SerializeConcept(concept, opts.PrettyPrint)
		if err != nil {
			return fmt.Errorf("failed to serialize concept %s: %w", concept.Title, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}
	}

	// Update bundle root path
	b.RootPath = path

	return nil
}

// LoadBundleFile loads a single concept file and wraps it in a bundle.
func LoadBundleFile(path string) (*KnowledgeBundle, error) {
	concept, err := ParseConcept(path)
	if err != nil {
		return nil, err
	}

	bundle := &KnowledgeBundle{
		Name:      strings.TrimSuffix(filepath.Base(path), ".md"),
		RootPath: filepath.Dir(path),
	}
	bundle.AddConcept(concept)

	return bundle, nil
}

// Exists checks if a path exists and is accessible.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDirectory checks if a path is a directory.
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
