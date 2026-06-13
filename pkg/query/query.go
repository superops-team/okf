// Package query provides advanced search and filtering for OKF concepts.
package query

import (
	"regexp"
	"strings"
)

// Concept represents the minimal concept interface needed for querying.
type Concept struct {
	Type        string
	Title       string
	Description string
	Resource    string
	Tags        []string
	Content     string
	FilePath    string
}

// KnowledgeBundle represents a collection of concepts for querying.
type KnowledgeBundle struct {
	Concepts []*Concept
}

// Query represents a search query with multiple criteria.
type Query struct {
	Type             string
	Tags             []string
	Resource         string
	Text             string
	TitleRegex       string
	DescriptionRegex string
	ContentRegex     string
}

// Builder helps construct complex queries fluently.
type Builder struct {
	q Query
}

// New creates a new query builder.
func New() *Builder {
	return &Builder{}
}

// WithType adds a type filter.
func (b *Builder) WithType(conceptType string) *Builder {
	b.q.Type = conceptType
	return b
}

// WithTags adds tag filters (concept must have all these tags).
func (b *Builder) WithTags(tags ...string) *Builder {
	b.q.Tags = tags
	return b
}

// WithResource adds a resource filter.
func (b *Builder) WithResource(resource string) *Builder {
	b.q.Resource = resource
	return b
}

// WithText adds a full-text search.
func (b *Builder) WithText(text string) *Builder {
	b.q.Text = text
	return b
}

// WithTitleRegex adds a title regex filter.
func (b *Builder) WithTitleRegex(pattern string) *Builder {
	b.q.TitleRegex = pattern
	return b
}

// WithDescriptionRegex adds a description regex filter.
func (b *Builder) WithDescriptionRegex(pattern string) *Builder {
	b.q.DescriptionRegex = pattern
	return b
}

// WithContentRegex adds a content regex filter.
func (b *Builder) WithContentRegex(pattern string) *Builder {
	b.q.ContentRegex = pattern
	return b
}

// Build returns the final query.
func (b *Builder) Build() *Query {
	return &b.q
}

// Execute runs the query against a bundle and returns matching concepts.
func (q *Query) Execute(bundle *KnowledgeBundle) []*Concept {
	return FilterConcepts(bundle.Concepts, q.Matches)
}

// Matches checks if a concept matches the query.
func (q *Query) Matches(c *Concept) bool {
	if q.Type != "" && c.Type != q.Type {
		return false
	}

	if len(q.Tags) > 0 {
		tagSet := make(map[string]bool)
		for _, t := range c.Tags {
			tagSet[t] = true
		}
		for _, requiredTag := range q.Tags {
			if !tagSet[requiredTag] {
				return false
			}
		}
	}

	if q.Resource != "" && !containsFold(c.Resource, q.Resource) {
		return false
	}

	if q.Text != "" {
		textLower := strings.ToLower(q.Text)
		found := containsFold(c.Title, textLower) ||
			containsFold(c.Description, textLower) ||
			containsFold(c.Content, textLower)
		if !found {
			return false
		}
	}

	if q.TitleRegex != "" {
		matched, _ := regexp.MatchString(q.TitleRegex, c.Title)
		if !matched {
			return false
		}
	}

	if q.DescriptionRegex != "" {
		matched, _ := regexp.MatchString(q.DescriptionRegex, c.Description)
		if !matched {
			return false
		}
	}

	if q.ContentRegex != "" {
		matched, _ := regexp.MatchString(q.ContentRegex, c.Content)
		if !matched {
			return false
		}
	}

	return true
}

// FilterConcepts filters a slice of concepts by predicate.
func FilterConcepts(concepts []*Concept, pred func(*Concept) bool) []*Concept {
	var result []*Concept
	for _, c := range concepts {
		if pred(c) {
			result = append(result, c)
		}
	}
	return result
}

// Search performs a full-text search across concept titles, descriptions, and content.
func Search(bundle *KnowledgeBundle, text string) []*Concept {
	return FilterConcepts(bundle.Concepts, func(c *Concept) bool {
		return containsFold(c.Title, text) ||
			containsFold(c.Description, text) ||
			containsFold(c.Content, text)
	})
}

// FilterByType returns all concepts of the given type.
func FilterByType(bundle *KnowledgeBundle, conceptType string) []*Concept {
	return FilterConcepts(bundle.Concepts, func(c *Concept) bool {
		return c.Type == conceptType
	})
}

// FilterByTag returns all concepts containing the given tag.
func FilterByTag(bundle *KnowledgeBundle, tag string) []*Concept {
	return FilterConcepts(bundle.Concepts, func(c *Concept) bool {
		for _, t := range c.Tags {
			if t == tag {
				return true
			}
		}
		return false
	})
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
