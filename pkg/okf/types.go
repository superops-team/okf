package okf

import (
	"os"
	"path/filepath"
	"time"
)

// Concept represents a single unit of knowledge within a bundle.
// It corresponds to one markdown file with YAML frontmatter.
type Concept struct {
	// Type is the category of the concept (e.g., "table", "metric", "api", "concept").
	// This is a required field per OKF specification.
	Type string `yaml:"type" json:"type"`

	// Title is a human-readable name for the concept.
	// This is a required field per OKF specification.
	Title string `yaml:"title" json:"title"`

	// Description provides a brief summary of the concept.
	// This is an optional but recommended field.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Resource references the actual resource this knowledge describes
	// (e.g., "bigquery.project.dataset.table", "https://api.example.com/v1/users").
	// This is an optional but recommended field for linking to actual data sources.
	Resource string `yaml:"resource,omitempty" json:"resource,omitempty"`

	// Tags are arbitrary labels for categorization and discovery.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// Timestamp records when this knowledge was created or last updated.
	// ISO 8601 format is recommended.
	Timestamp string `yaml:"timestamp,omitempty" json:"timestamp,omitempty"`

	// Content is the markdown body of the document.
	// This contains the detailed knowledge that humans and agents read.
	Content string `yaml:"-" json:"-"`

	// FilePath is the relative path to the source file.
	// This is set during parsing and not serialized.
	FilePath string `yaml:"-" json:"filePath,omitempty"`

	// CustomFields holds additional fields not defined in the OKF spec.
	// Agents can use this for domain-specific extensions.
	CustomFields map[string]interface{} `yaml:",inline" json:"-"`
}

// KnowledgeBundle represents a self-contained collection of knowledge documents.
// It is the unit of distribution in OKF - typically a directory structure.
type KnowledgeBundle struct {
	// Name is a short identifier for the bundle.
	Name string `yaml:"name" json:"name"`

	// Description provides an overview of what this bundle contains.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Version is the bundle format version (e.g., "1.0", "2.1").
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// Owner identifies who or what is responsible for this bundle.
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty"`

	// Concepts is the list of all concepts in this bundle.
	Concepts []*Concept `yaml:"concepts,omitempty" json:"concepts,omitempty"`

	// RootPath is the filesystem path to the bundle root.
	// This is set during parsing and not serialized.
	RootPath string `yaml:"-" json:"rootPath,omitempty"`
}

// LoadOptions contains configuration for loading bundles.
type LoadOptions struct {
	// Recursive controls whether subdirectories are scanned for concepts.
	Recursive bool

	// FilterFunc optionally filters which files are loaded.
	FilterFunc func(path string, info os.FileInfo) bool
}

// SaveOptions contains configuration for saving bundles.
type SaveOptions struct {
	// PrettyPrint controls whether YAML is formatted with indentation.
	PrettyPrint bool
}

// DefaultLoadOptions returns the recommended default load configuration.
func DefaultLoadOptions() *LoadOptions {
	return &LoadOptions{
		Recursive: true,
	}
}

// DefaultSaveOptions returns the recommended default save configuration.
func DefaultSaveOptions() *SaveOptions {
	return &SaveOptions{
		PrettyPrint: true,
	}
}

// NewConcept creates a new concept with required fields set.
func NewConcept(conceptType, title string) *Concept {
	return &Concept{
		Type:      conceptType,
		Title:     title,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Tags:      []string{},
	}
}

// NewBundle creates a new knowledge bundle.
func NewBundle(name string) *KnowledgeBundle {
	return &KnowledgeBundle{
		Name:      name,
		Version:   "1.0",
		Concepts:  []*Concept{},
	}
}

// AddConcept adds a concept to the bundle and returns the concept.
func (b *KnowledgeBundle) AddConcept(c *Concept) *Concept {
	if c.FilePath == "" {
		filename := sanitizeFilename(c.Title) + ".md"
		if c.Type != "" {
			c.FilePath = filepath.Join(c.Type+"s", filename)
		} else {
			c.FilePath = filepath.Join("concepts", filename)
		}
	}
	b.Concepts = append(b.Concepts, c)
	return c
}

// RemoveConcept removes a concept from the bundle by title.
// Returns true if the concept was found and removed.
func (b *KnowledgeBundle) RemoveConcept(title string) bool {
	for i, c := range b.Concepts {
		if c.Title == title {
			b.Concepts = append(b.Concepts[:i], b.Concepts[i+1:]...)
			return true
		}
	}
	return false
}

// GetConcept returns a concept by title, or nil if not found.
func (b *KnowledgeBundle) GetConcept(title string) *Concept {
	for _, c := range b.Concepts {
		if c.Title == title {
			return c
		}
	}
	return nil
}

// FilterConcepts returns all concepts matching the given predicate.
func (b *KnowledgeBundle) FilterConcepts(pred func(*Concept) bool) []*Concept {
	var result []*Concept
	for _, c := range b.Concepts {
		if pred(c) {
			result = append(result, c)
		}
	}
	return result
}

// FilterByType returns all concepts of the given type.
func (b *KnowledgeBundle) FilterByType(conceptType string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return c.Type == conceptType
	})
}

// FilterByTag returns all concepts containing the given tag.
func (b *KnowledgeBundle) FilterByTag(tag string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		for _, t := range c.Tags {
			if t == tag {
				return true
			}
		}
		return false
	})
}

// FilterByResource returns all concepts referencing the given resource.
func (b *KnowledgeBundle) FilterByResource(resource string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return c.Resource == resource
	})
}

// Search performs a full-text search across concept titles, descriptions,
// and content. Returns matching concepts.
func (b *KnowledgeBundle) Search(query string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return containsFold(c.Title, query) ||
			containsFold(c.Description, query) ||
			containsFold(c.Content, query)
	})
}

// Stats returns statistics about the bundle.
func (b *KnowledgeBundle) Stats() BundleStats {
	stats := BundleStats{
		TotalConcepts: len(b.Concepts),
		TypeCounts:   make(map[string]int),
		TagCounts:    make(map[string]int),
	}

	typeSet := make(map[string]struct{})
	tagSet := make(map[string]struct{})

	for _, c := range b.Concepts {
		if c.Type != "" {
			typeSet[c.Type] = struct{}{}
			stats.TypeCounts[c.Type]++
		}
		for _, tag := range c.Tags {
			tagSet[tag] = struct{}{}
			stats.TagCounts[tag]++
		}
	}

	stats.UniqueTypes = len(typeSet)
	stats.UniqueTags = len(tagSet)

	return stats
}

// BundleStats contains aggregate information about a bundle.
type BundleStats struct {
	TotalConcepts int            `json:"totalConcepts"`
	UniqueTypes   int            `json:"uniqueTypes"`
	UniqueTags    int            `json:"uniqueTags"`
	TypeCounts    map[string]int `json:"typeCounts"`
	TagCounts     map[string]int `json:"tagCounts"`
}

// RelatedConcepts finds concepts that share tags or resources with the given concept.
func (b *KnowledgeBundle) RelatedConcepts(c *Concept) []*Concept {
	tagSet := make(map[string]bool)
	for _, t := range c.Tags {
		tagSet[t] = true
	}

	return b.FilterConcepts(func(other *Concept) bool {
		if other == c {
			return false
		}

		for _, t := range other.Tags {
			if tagSet[t] {
				return true
			}
		}

		if c.Resource != "" && other.Resource == c.Resource {
			return true
		}

		return false
	})
}
