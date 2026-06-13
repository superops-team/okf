package okf

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/superops-team/okf/pkg/parser"
	"github.com/superops-team/okf/pkg/query"
)

// LoadBundle reads a knowledge bundle from a directory.
// The directory should contain markdown files with YAML frontmatter.
func LoadBundle(path string, opts *LoadOptions) (*KnowledgeBundle, error) {
	if opts == nil {
		opts = DefaultLoadOptions()
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, &ParseError{FilePath: path, Message: "failed to stat path: " + err.Error()}
	}

	if !info.IsDir() {
		return nil, &ParseError{FilePath: path, Message: "path is not a directory"}
	}

	bundle := &KnowledgeBundle{
		RootPath: path,
		Name:     filepath.Base(path),
	}

	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if strings.HasPrefix(filepath.Base(filePath), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(filePath, ".md") {
			return nil
		}

		if opts.FilterFunc != nil && !opts.FilterFunc(filePath, info) {
			return nil
		}

		if !opts.Recursive {
			relPath, _ := filepath.Rel(path, filePath)
			if strings.Contains(relPath, string(filepath.Separator)) {
				return nil
			}
		}

		pc, err := parser.ParseConcept(filePath)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(path, filePath)

		// Convert parser.Concept to okf.Concept
		concept := &Concept{
			Type:        pc.Type,
			Title:       pc.Title,
			Description: pc.Description,
			Resource:    pc.Resource,
			Tags:        pc.Tags,
			Timestamp:   pc.Timestamp,
			Content:     pc.Content,
			FilePath:    relPath,
		}

		bundle.Concepts = append(bundle.Concepts, concept)
		return nil
	})

	if err != nil {
		return nil, &ParseError{Message: "failed to walk directory: " + err.Error()}
	}

	return bundle, nil
}

// SaveBundle writes a knowledge bundle to a directory.
// Each concept is saved as a separate markdown file.
func SaveBundle(b *KnowledgeBundle, path string, opts *SaveOptions) error {
	if opts == nil {
		opts = DefaultSaveOptions()
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return &ParseError{FilePath: path, Message: "failed to create directory: " + err.Error()}
	}

	writtenPaths := make(map[string]bool)

	for _, concept := range b.Concepts {
		filePath := concept.FilePath
		if filePath == "" {
			filename := sanitizeFilename(concept.Title) + ".md"
			if concept.Type != "" {
				filePath = filepath.Join(concept.Type+"s", filename)
			} else {
				filePath = filepath.Join("concepts", filename)
			}
		}

		if filepath.IsAbs(filePath) {
			relPath, _ := filepath.Rel(path, filePath)
			filePath = relPath
		}

		baseFilePath := filePath
		counter := 1
		for writtenPaths[filePath] {
			ext := filepath.Ext(baseFilePath)
			name := strings.TrimSuffix(baseFilePath, ext)
			filePath = filepath.Join(filepath.Dir(name), filepath.Base(name)+"_"+string(rune('0'+counter))+ext)
			counter++
		}
		writtenPaths[filePath] = true

		fullPath := filepath.Join(path, filePath)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			continue
		}

		// Convert okf.Concept to parser.Concept for serialization
		pc := &parser.Concept{
			Type:        concept.Type,
			Title:       concept.Title,
			Description: concept.Description,
			Resource:    concept.Resource,
			Tags:        concept.Tags,
			Timestamp:   concept.Timestamp,
			Content:     concept.Content,
		}

		data, err := parser.SerializeConcept(pc, opts.PrettyPrint)
		if err != nil {
			continue
		}

		os.WriteFile(fullPath, data, 0644)
	}

	b.RootPath = path
	return nil
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

// NewQuery creates a new query builder.
func NewQuery() *query.Builder {
	return query.New()
}
