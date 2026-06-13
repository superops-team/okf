package okf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConcept(t *testing.T) {
	c := NewConcept("table", "users")
	assert.Equal(t, "table", c.Type)
	assert.Equal(t, "users", c.Title)
	assert.NotEmpty(t, c.Timestamp)
	assert.NotNil(t, c.Tags)
}

func TestNewBundle(t *testing.T) {
	b := NewBundle("test-bundle")
	assert.Equal(t, "test-bundle", b.Name)
	assert.Equal(t, "1.0", b.Version)
	assert.NotNil(t, b.Concepts)
	assert.Empty(t, b.Concepts)
}

func TestBundle_AddConcept(t *testing.T) {
	b := NewBundle("test")
	c := NewConcept("metric", "active_users")
	c.Resource = "bigquery.project.dataset.active_users"

	b.AddConcept(c)

	assert.Len(t, b.Concepts, 1)
	assert.Equal(t, c, b.Concepts[0])
}

func TestBundle_RemoveConcept(t *testing.T) {
	b := NewBundle("test")
	c1 := NewConcept("metric", "users")
	c2 := NewConcept("metric", "sessions")
	b.AddConcept(c1)
	b.AddConcept(c2)

	removed := b.RemoveConcept("users")
	assert.True(t, removed)
	assert.Len(t, b.Concepts, 1)
	assert.Equal(t, c2, b.Concepts[0])

	// Remove non-existent
	removed = b.RemoveConcept("nonexistent")
	assert.False(t, removed)
}

func TestBundle_GetConcept(t *testing.T) {
	b := NewBundle("test")
	c := NewConcept("api", "user-endpoint")
	b.AddConcept(c)

	found := b.GetConcept("user-endpoint")
	assert.Equal(t, c, found)

	notFound := b.GetConcept("nonexistent")
	assert.Nil(t, notFound)
}

func TestBundle_FilterByType(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(NewConcept("table", "users"))
	b.AddConcept(NewConcept("table", "orders"))
	b.AddConcept(NewConcept("api", "checkout"))

	tables := b.FilterByType("table")
	assert.Len(t, tables, 2)

	apis := b.FilterByType("api")
	assert.Len(t, apis, 1)
}

func TestBundle_FilterByTag(t *testing.T) {
	b := NewBundle("test")
	c1 := NewConcept("table", "users")
	c1.Tags = []string{"pii", "production"}
	b.AddConcept(c1)

	c2 := NewConcept("table", "logs")
	c2.Tags = []string{"logs", "production"}
	b.AddConcept(c2)

	c3 := NewConcept("table", "staging_data")
	c3.Tags = []string{"staging"}
	b.AddConcept(c3)

	production := b.FilterByTag("production")
	assert.Len(t, production, 2)

	pii := b.FilterByTag("pii")
	assert.Len(t, pii, 1)
}

func TestBundle_Search(t *testing.T) {
	b := NewBundle("test")
	c1 := NewConcept("metric", "active_users")
	c1.Description = "Count of users who logged in within 7 days"
	b.AddConcept(c1)

	c2 := NewConcept("metric", "revenue")
	c2.Description = "Total revenue in USD"
	b.AddConcept(c2)

	c3 := NewConcept("metric", "user_sessions")
	c3.Description = "Number of sessions per user"
	b.AddConcept(c3)

	// "users" appears in c1 description, "revenue" in c2
	results := b.Search("users")
	assert.Len(t, results, 1)

	results = b.Search("revenue")
	assert.Len(t, results, 1)

	results = b.Search("session")
	assert.Len(t, results, 1)

	results = b.Search("nonexistent")
	assert.Len(t, results, 0)
}

func TestBundle_Stats(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(NewConcept("table", "users"))
	b.AddConcept(NewConcept("table", "orders"))
	b.AddConcept(NewConcept("api", "checkout"))

	stats := b.Stats()
	assert.Equal(t, 3, stats.TotalConcepts)
	assert.Equal(t, 2, stats.UniqueTypes)
	assert.Equal(t, 2, stats.TypeCounts["table"])
	assert.Equal(t, 1, stats.TypeCounts["api"])
}

func TestParseConcept(t *testing.T) {
	content := `---
type: table
title: users
description: User accounts table
resource: bigquery.project.dataset.users
tags:
  - pii
  - production
timestamp: "2024-01-15T10:30:00Z"
---

This table contains all registered user accounts.

## Schema

- id: INTEGER
- email: STRING
- created_at: TIMESTAMP
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "users.md")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	concept, err := ParseConcept(tmpFile)
	require.NoError(t, err)

	assert.Equal(t, "table", concept.Type)
	assert.Equal(t, "users", concept.Title)
	assert.Equal(t, "User accounts table", concept.Description)
	assert.Equal(t, "bigquery.project.dataset.users", concept.Resource)
	assert.Equal(t, []string{"pii", "production"}, concept.Tags)
	assert.Equal(t, "2024-01-15T10:30:00Z", concept.Timestamp)
	assert.Contains(t, concept.Content, "This table contains all registered user accounts")
}

func TestParseConcept_NoFrontmatter(t *testing.T) {
	content := `# Just a title

Some content without frontmatter.
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "simple.md")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	concept, err := ParseConcept(tmpFile)
	require.NoError(t, err)

	// When no frontmatter, title defaults to filename without extension
	assert.Equal(t, "concept", concept.Type)
	assert.Equal(t, "simple", concept.Title)
	assert.Contains(t, concept.Content, "Some content without frontmatter")
}

func TestParseConcept_MissingTitle(t *testing.T) {
	content := `---
type: table
---

Content without title.
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.md")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	_, err = ParseConcept(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestSerializeConcept(t *testing.T) {
	c := &Concept{
		Type:        "metric",
		Title:       "active_users",
		Description: "Count of active users",
		Resource:    "bigquery.project.metrics.active_users",
		Tags:        []string{"analytics", "kpi"},
		Timestamp:   "2024-01-15T10:30:00Z",
		Content:     "This metric tracks **active users**.\n\nCalculation: Count of users with activity in last 7 days.",
	}

	data, err := SerializeConcept(c, true)
	require.NoError(t, err)

	output := string(data)
	assert.Contains(t, output, "---")
	assert.Contains(t, output, "type: metric")
	assert.Contains(t, output, "title: active_users")
	assert.Contains(t, output, "This metric tracks")
	assert.Contains(t, output, "**active users**")
}

func TestLoadBundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create bundle structure
	tablesDir := filepath.Join(tmpDir, "tables")
	os.MkdirAll(tablesDir, 0755)

	table1 := `---
type: table
title: users
description: User accounts
---

Users table content.
`
	err := os.WriteFile(filepath.Join(tablesDir, "users.md"), []byte(table1), 0644)
	require.NoError(t, err)

	table2 := `---
type: table
title: orders
description: Customer orders
---

Orders table content.
`
	err = os.WriteFile(filepath.Join(tablesDir, "orders.md"), []byte(table2), 0644)
	require.NoError(t, err)

	bundle, err := LoadBundle(tmpDir, &LoadOptions{Recursive: true})
	require.NoError(t, err)

	assert.Equal(t, 2, len(bundle.Concepts))
}

func TestSaveAndLoadBundle(t *testing.T) {
	original := NewBundle("test-bundle")
	c1 := NewConcept("table", "users")
	c1.Description = "User accounts table"
	c1.Tags = []string{"production"}
	original.AddConcept(c1)

	c2 := NewConcept("api", "users_endpoint")
	c2.Description = "REST API for users"
	c2.Resource = "https://api.example.com/v1/users"
	original.AddConcept(c2)

	tmpDir := t.TempDir()
	err := SaveBundle(original, tmpDir, &SaveOptions{PrettyPrint: true})
	require.NoError(t, err)

	// Load it back - Name will be the directory name since we don't save bundle metadata
	loaded, err := LoadBundle(tmpDir, &LoadOptions{Recursive: true})
	require.NoError(t, err)

	// Name is derived from directory path on load
	assert.Equal(t, filepath.Base(tmpDir), loaded.Name)
	assert.Equal(t, 2, len(loaded.Concepts))

	found := loaded.GetConcept("users")
	assert.NotNil(t, found)
	assert.Equal(t, "table", found.Type)
	assert.Equal(t, []string{"production"}, found.Tags)
}

func TestQuery_BuildAndExecute(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(NewConcept("table", "users"))
	b.AddConcept(NewConcept("table", "orders"))
	b.AddConcept(NewConcept("api", "checkout"))

	q := NewQuery().
		WithType("table").
		WithText("order").
		Build()

	results := q.Execute(b)
	assert.Len(t, results, 1)
	assert.Equal(t, "orders", results[0].Title)
}

func TestQuery_TagsFilter(t *testing.T) {
	b := NewBundle("test")

	c1 := NewConcept("table", "users")
	c1.Tags = []string{"pii", "production"}
	b.AddConcept(c1)

	c2 := NewConcept("table", "logs")
	c2.Tags = []string{"logs", "production"}
	b.AddConcept(c2)

	c3 := NewConcept("table", "staging_users")
	c3.Tags = []string{"pii", "staging"}
	b.AddConcept(c3)

	// Must have both pii AND production
	q := NewQuery().WithTags("pii", "production").Build()
	results := q.Execute(b)
	assert.Len(t, results, 1)
	assert.Equal(t, "users", results[0].Title)
}

func TestQuery_Regex(t *testing.T) {
	b := NewBundle("test")
	b.AddConcept(NewConcept("table", "users_v1"))
	b.AddConcept(NewConcept("table", "users_v2"))
	b.AddConcept(NewConcept("table", "orders"))

	q := NewQuery().WithTitleRegex(`users_v\d+`).Build()
	results := q.Execute(b)
	assert.Len(t, results, 2)
}

func TestKnowledgeBundle_RelatedConcepts(t *testing.T) {
	b := NewBundle("test")

	c1 := NewConcept("table", "users")
	c1.Tags = []string{"pii", "production"}
	c1.Resource = "bigquery.project.dataset.users"
	b.AddConcept(c1)

	c2 := NewConcept("table", "user_events")
	c2.Tags = []string{"analytics", "production"}
	c2.Resource = "bigquery.project.dataset.user_events"
	b.AddConcept(c2)

	c3 := NewConcept("api", "users_endpoint")
	c3.Tags = []string{"api", "production"}
	c3.Resource = "https://api.example.com/users"
	b.AddConcept(c3)

	c4 := NewConcept("metric", "daily_revenue")
	c4.Tags = []string{"finance"}
	b.AddConcept(c4)

	related := b.RelatedConcepts(c1)
	assert.Len(t, related, 2) // users_v2 and users_endpoint (share tags or resource)
}

func TestSanitizeFilename2(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", "users"},
		{"user accounts", "user accounts"},
		{"path/to/file", "path_to_file"},
		{"file:name", "file_name"},
		{"file*name", "file_name"},
	}

	for _, tt := range tests {
		result := sanitizeFilename(tt.input)
		assert.NotContains(t, result, "/")
		assert.NotContains(t, result, "\\")
		assert.NotContains(t, result, ":")
		assert.NotContains(t, result, "*")
	}
}

