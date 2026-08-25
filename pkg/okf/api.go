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
			Type:         pc.Type,
			Title:        pc.Title,
			Description:  pc.Description,
			Resource:     pc.Resource,
			Tags:         pc.Tags,
			Timestamp:    pc.Timestamp,
			Content:      pc.Content,
			FilePath:     relPath,
			CustomFields: pc.CustomFields,
			// v0.2 fields
			Status:      ConceptStatus(pc.Status),
			StaleAfter:  pc.StaleAfter,
			Runtime:     pc.Runtime,
			Computation: pc.Computation,
		}
		// Convert v0.2 nested types
		if len(pc.Sources) > 0 {
			concept.Sources = make([]Source, len(pc.Sources))
			for i, s := range pc.Sources {
				concept.Sources[i] = Source{
					ID:           s.ID,
					Resource:     s.Resource,
					Title:        s.Title,
					Author:       s.Author,
					UsageCount:   s.UsageCount,
					LastModified: s.LastModified,
				}
			}
		}
		if pc.UsageWindow != nil {
			concept.UsageWindow = &UsageWindow{From: pc.UsageWindow.From, To: pc.UsageWindow.To}
		}
		if pc.Generated != nil {
			concept.Generated = &GeneratedInfo{By: pc.Generated.By, At: pc.Generated.At}
		}
		if len(pc.Verified) > 0 {
			concept.Verified = make([]VerificationEvent, len(pc.Verified))
			for i, v := range pc.Verified {
				concept.Verified[i] = VerificationEvent{By: v.By, At: v.At}
			}
		}
		if len(pc.Parameters) > 0 {
			concept.Parameters = make([]Parameter, len(pc.Parameters))
			for i, p := range pc.Parameters {
				concept.Parameters[i] = Parameter{Name: p.Name, Type: p.Type, Required: p.Required}
			}
		}
		if pc.Executor != nil {
			concept.Executor = &ExecutorRef{Resource: pc.Executor.Resource, Receipt: pc.Executor.Receipt}
		}
		if pc.Attester != nil {
			concept.Attester = &AttesterRef{Resource: pc.Attester.Resource}
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
			Type:         concept.Type,
			Title:        concept.Title,
			Description:  concept.Description,
			Resource:     concept.Resource,
			Tags:         concept.Tags,
			Timestamp:    concept.Timestamp,
			Content:      concept.Content,
			CustomFields: concept.CustomFields,
			// v0.2 fields
			Status:      string(concept.Status),
			StaleAfter:  concept.StaleAfter,
			Runtime:     concept.Runtime,
			Computation: concept.Computation,
		}
		// Convert v0.2 nested types
		if len(concept.Sources) > 0 {
			pc.Sources = make([]parser.Source, len(concept.Sources))
			for i, s := range concept.Sources {
				pc.Sources[i] = parser.Source{
					ID:           s.ID,
					Resource:     s.Resource,
					Title:        s.Title,
					Author:       s.Author,
					UsageCount:   s.UsageCount,
					LastModified: s.LastModified,
				}
			}
		}
		if concept.UsageWindow != nil {
			pc.UsageWindow = &parser.UsageWindow{From: concept.UsageWindow.From, To: concept.UsageWindow.To}
		}
		if concept.Generated != nil {
			pc.Generated = &parser.GeneratedInfo{By: concept.Generated.By, At: concept.Generated.At}
		}
		if len(concept.Verified) > 0 {
			pc.Verified = make([]parser.VerificationEvent, len(concept.Verified))
			for i, v := range concept.Verified {
				pc.Verified[i] = parser.VerificationEvent{By: v.By, At: v.At}
			}
		}
		if len(concept.Parameters) > 0 {
			pc.Parameters = make([]parser.Parameter, len(concept.Parameters))
			for i, p := range concept.Parameters {
				pc.Parameters[i] = parser.Parameter{Name: p.Name, Type: p.Type, Required: p.Required}
			}
		}
		if concept.Executor != nil {
			pc.Executor = &parser.ExecutorRef{Resource: concept.Executor.Resource, Receipt: concept.Executor.Receipt}
		}
		if concept.Attester != nil {
			pc.Attester = &parser.AttesterRef{Resource: concept.Attester.Resource}
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
