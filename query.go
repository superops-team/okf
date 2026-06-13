package okf

import (
	"regexp"
	"strings"
)

// Query represents a search query with multiple criteria.
type Query struct {
	// Type filters by concept type (exact match).
	Type string

	// Tags filters by tags (concept must have all specified tags).
	Tags []string

	// Resource filters by resource (substring match).
	Resource string

	// Text performs full-text search across title, description, and content.
	Text string

	// TitleRegex is a regex pattern to match against title.
	TitleRegex string

	// DescriptionRegex is a regex pattern to match against description.
	DescriptionRegex string

	// ContentRegex is a regex pattern to match against content.
	ContentRegex string
}

// MatchResult represents the result of matching a concept against a query.
type MatchResult struct {
	Concept      *Concept
	MatchScore   float64
	MatchedOn   []string
	TitleRegex   bool
	DescRegex    bool
	ContentRegex bool
}

// QueryBuilder helps construct complex queries fluently.
type QueryBuilder struct {
	q Query
}

// NewQuery creates a new query builder.
func NewQuery() *QueryBuilder {
	return &QueryBuilder{}
}

// WithType adds a type filter.
func (qb *QueryBuilder) WithType(conceptType string) *QueryBuilder {
	qb.q.Type = conceptType
	return qb
}

// WithTags adds tag filters (concept must have all these tags).
func (qb *QueryBuilder) WithTags(tags ...string) *QueryBuilder {
	qb.q.Tags = tags
	return qb
}

// WithResource adds a resource filter.
func (qb *QueryBuilder) WithResource(resource string) *QueryBuilder {
	qb.q.Resource = resource
	return qb
}

// WithText adds a full-text search.
func (qb *QueryBuilder) WithText(text string) *QueryBuilder {
	qb.q.Text = text
	return qb
}

// WithTitleRegex adds a title regex filter.
func (qb *QueryBuilder) WithTitleRegex(pattern string) *QueryBuilder {
	qb.q.TitleRegex = pattern
	return qb
}

// WithDescriptionRegex adds a description regex filter.
func (qb *QueryBuilder) WithDescriptionRegex(pattern string) *QueryBuilder {
	qb.q.DescriptionRegex = pattern
	return qb
}

// WithContentRegex adds a content regex filter.
func (qb *QueryBuilder) WithContentRegex(pattern string) *QueryBuilder {
	qb.q.ContentRegex = pattern
	return qb
}

// Build returns the final query.
func (qb *QueryBuilder) Build() *Query {
	return &qb.q
}

// Execute runs the query against a bundle and returns matching concepts.
func (q *Query) Execute(b *KnowledgeBundle) []*Concept {
	return b.FilterConcepts(q.Matches)
}

// Matches checks if a concept matches the query.
func (q *Query) Matches(c *Concept) bool {
	// Type filter
	if q.Type != "" && c.Type != q.Type {
		return false
	}

	// Tags filter - concept must have ALL specified tags
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

	// Resource filter
	if q.Resource != "" && !containsFold(c.Resource, q.Resource) {
		return false
	}

	// Text search
	if q.Text != "" {
		textLower := strings.ToLower(q.Text)
		found := containsFold(c.Title, textLower) ||
			containsFold(c.Description, textLower) ||
			containsFold(c.Content, textLower)
		if !found {
			return false
		}
	}

	// Title regex
	if q.TitleRegex != "" {
		matched, _ := regexp.MatchString(q.TitleRegex, c.Title)
		if !matched {
			return false
		}
	}

	// Description regex
	if q.DescriptionRegex != "" {
		matched, _ := regexp.MatchString(q.DescriptionRegex, c.Description)
		if !matched {
			return false
		}
	}

	// Content regex
	if q.ContentRegex != "" {
		matched, _ := regexp.MatchString(q.ContentRegex, c.Content)
		if !matched {
			return false
		}
	}

	return true
}

// ExecuteWithResults runs the query and returns detailed match results.
func (q *Query) ExecuteWithResults(b *KnowledgeBundle) []*MatchResult {
	var results []*MatchResult

	for _, c := range b.Concepts {
		if !q.Matches(c) {
			continue
		}

		result := &MatchResult{
			Concept:    c,
			MatchedOn:  []string{},
			MatchScore: 1.0,
		}

		// Check which criteria matched for scoring
		if q.Type != "" && c.Type == q.Type {
			result.MatchedOn = append(result.MatchedOn, "type")
			result.MatchScore += 0.2
		}

		if len(q.Tags) > 0 {
			allTags := true
			for _, tag := range q.Tags {
				if !containsString(c.Tags, tag) {
					allTags = false
					break
				}
			}
			if allTags {
				result.MatchedOn = append(result.MatchedOn, "tags")
				result.MatchScore += 0.2
			}
		}

		if q.Text != "" {
			textLower := strings.ToLower(q.Text)
			if containsFold(c.Title, textLower) {
				result.MatchedOn = append(result.MatchedOn, "title")
				result.MatchScore += 0.3
			}
			if containsFold(c.Description, textLower) {
				result.MatchedOn = append(result.MatchedOn, "description")
				result.MatchScore += 0.2
			}
			if containsFold(c.Content, textLower) {
				result.MatchedOn = append(result.MatchedOn, "content")
				result.MatchScore += 0.1
			}
		}

		if q.TitleRegex != "" {
			if matched, _ := regexp.MatchString(q.TitleRegex, c.Title); matched {
				result.TitleRegex = true
				result.MatchedOn = append(result.MatchedOn, "title_regex")
				result.MatchScore += 0.2
			}
		}

		if q.DescriptionRegex != "" {
			if matched, _ := regexp.MatchString(q.DescriptionRegex, c.Description); matched {
				result.DescRegex = true
				result.MatchedOn = append(result.MatchedOn, "description_regex")
				result.MatchScore += 0.2
			}
		}

		if q.ContentRegex != "" {
			if matched, _ := regexp.MatchString(q.ContentRegex, c.Content); matched {
				result.ContentRegex = true
				result.MatchedOn = append(result.MatchedOn, "content_regex")
				result.MatchScore += 0.1
			}
		}

		results = append(results, result)
	}

	return results
}

// containsString checks if a string slice contains a value (case-insensitive).
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if equalFold(s, val) {
			return true
		}
	}
	return false
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

		// Share a tag
		for _, t := range other.Tags {
			if tagSet[t] {
				return true
			}
		}

		// Share a resource
		if c.Resource != "" && other.Resource == c.Resource {
			return true
		}

		return false
	})
}
