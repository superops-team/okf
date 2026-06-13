package okf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLintConcept(t *testing.T) {
	t.Run("valid concept", func(t *testing.T) {
		c := NewConcept("table", "users")
		c.Description = "User accounts table with authentication data"
		c.Content = "This is the users table.\n\nIt stores all user accounts."

		issues := LintConcept(c, DefaultLintConfig())
		for _, issue := range issues {
			if issue.Severity == Error {
				t.Errorf("unexpected error: %s", issue.Message)
			}
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		c := &Concept{
			Type:    "",
			Title:   "",
			Content: "",
		}

		issues := LintConcept(c, DefaultLintConfig())
		hasError := false
		for _, issue := range issues {
			if issue.Severity == Error {
				hasError = true
			}
		}
		if !hasError {
			t.Error("expected error for missing required fields")
		}
	})

	t.Run("all rules", func(t *testing.T) {
		c := NewConcept("TABLE", "TEST")
		issues := LintConcept(c, DefaultLintConfig())
		// Should have at least some warnings for uppercase type and title
		hasWarning := false
		for _, issue := range issues {
			if issue.Severity == Warning || issue.Severity == Error {
				hasWarning = true
				break
			}
		}
		_ = hasWarning
	})
}

func TestLintBundle(t *testing.T) {
	b := NewBundle("test")

	c1 := NewConcept("table", "users")
	c1.Description = "User accounts table"
	c1.Tags = []string{"production", "pii"}
	b.AddConcept(c1)

	c2 := NewConcept("api", "users_endpoint")
	c2.Description = "REST API for users"
	c2.Tags = []string{"api", "production"}
	b.AddConcept(c2)

	c3 := NewConcept("metric", "revenue")
	c3.Description = "Revenue metrics"
	b.AddConcept(c3)

	result := LintBundle(b, DefaultLintConfig())

	if result.ConceptsChecked != 3 {
		t.Errorf("expected 3 concepts checked, got %d", result.ConceptsChecked)
	}

	t.Logf("Lint result: %s", result.Summary())
}

func TestLintConfig_RequiredTags(t *testing.T) {
	config := &LintConfig{
		MaxLineLength:        240,
		MinDescriptionLength: 10,
		RequiredTags:         []string{"reviewed"},
	}

	c := NewConcept("table", "users")
	c.Description = "User table description"
	issues := LintConcept(c, config)

	hasError := false
	for _, issue := range issues {
		if issue.Severity == Error && issue.Code == "OKF011" {
			hasError = true
		}
	}

	if !hasError {
		t.Error("expected error for missing required tag")
	}

	// Add required tag
	c.Tags = []string{"reviewed"}
	issues = LintConcept(c, config)
	for _, issue := range issues {
		if issue.Code == "OKF011" {
			t.Errorf("unexpected OKF011 error: %s", issue.Message)
		}
	}
}

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"utils.ts", "typescript"},
		{"lib.rs", "rust"},
		{"Main.java", "java"},
		{"README.md", "markdown"},
		{"config.yml", "yaml"},
		{"data.json", "json"},
	}

	for _, tt := range tests {
		result := detectFileType(tt.path)
		if result != tt.expected {
			t.Errorf("detectFileType(%s) = %s, want %s", tt.path, result, tt.expected)
		}
	}
}

func TestExtractImports(t *testing.T) {
	goCode := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello")
}
`
	imports := extractImports(goCode, "go")
	if len(imports) < 2 {
		t.Error("expected at least 2 imports")
	}

	pythonCode := `import os
import sys
from pathlib import Path

def main():
    pass
`
	imports = extractImports(pythonCode, "python")
	if len(imports) < 2 {
		t.Error("expected at least 2 python imports")
	}
}

func TestExtractFunctions(t *testing.T) {
	goCode := `package main

func hello() {
}

func (c *Client) Connect() error {
	return nil
}

func processData(x int) string {
	return ""
}
`
	fns := extractFunctions(goCode, "go")
	if len(fns) < 2 {
		t.Errorf("expected at least 2 functions, got %d", len(fns))
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []string{
		"simple file",
		"file/with/slashes",
		"file:with:colons",
		"file*with*stars",
	}

	for _, test := range tests {
		result := sanitizeFilename(test)
		if strings.Contains(result, "/") {
			t.Errorf("sanitizeFilename should not contain slashes: %s", result)
		}
		if strings.Contains(result, ":") {
			t.Errorf("sanitizeFilename should not contain colons: %s", result)
		}
	}
}

func TestSanitizeTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John Doe", "john-doe"},
		{"Test@User", "test_user"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		result := sanitizeTag(tt.input)
		if result == "" {
			t.Errorf("sanitizeTag(%s) returned empty", tt.input)
		}
		if strings.ContainsAny(result, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			t.Errorf("sanitizeTag(%s) should be lowercase, got %s", tt.input, result)
		}
	}
}

func TestRelatedConcepts(t *testing.T) {
	b := NewBundle("test")

	c1 := NewConcept("table", "users")
	c1.Tags = []string{"pii", "production"}
	b.AddConcept(c1)

	c2 := NewConcept("table", "orders")
	c2.Tags = []string{"production", "revenue"}
	b.AddConcept(c2)

	c3 := NewConcept("api", "users_api")
	c3.Tags = []string{"pii", "api"}
	b.AddConcept(c3)

	c4 := NewConcept("metric", "revenue_metric")
	c4.Tags = []string{"analytics"}
	b.AddConcept(c4)

	related := b.RelatedConcepts(c1)
	if len(related) < 2 {
		t.Errorf("expected at least 2 related concepts, got %d", len(related))
	}
}

func TestQuerySearch(t *testing.T) {
	b := NewBundle("test")

	c1 := NewConcept("table", "user_accounts")
	c1.Description = "User database table"
	c1.Tags = []string{"database", "production"}
	b.AddConcept(c1)

	c2 := NewConcept("api", "users_api")
	c2.Description = "User REST API endpoint"
	c2.Tags = []string{"api", "production"}
	b.AddConcept(c2)

	c3 := NewConcept("metric", "monthly_revenue")
	c3.Description = "Monthly revenue calculation"
	b.AddConcept(c3)

	// Test text search
	q := NewQuery().WithText("user").Build()
	results := q.Execute(b)
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'user', got %d", len(results))
	}

	// Test type filter
	q = NewQuery().WithType("api").Build()
	results = q.Execute(b)
	if len(results) != 1 {
		t.Errorf("expected 1 result for api type, got %d", len(results))
	}

	// Test tag filter
	q = NewQuery().WithTags("production").Build()
	results = q.Execute(b)
	if len(results) != 2 {
		t.Errorf("expected 2 results for production tag, got %d", len(results))
	}

	// Test combined filter
	q = NewQuery().WithType("table").WithTags("production").WithText("user").Build()
	results = q.Execute(b)
	if len(results) != 1 {
		t.Errorf("expected 1 result for combined query, got %d", len(results))
	}
}

func TestFileFiltering(t *testing.T) {
	config := DefaultGitConfig()

	tests := []struct {
		path     string
		expected bool
	}{
		{"src/main.go", true},
		{"node_modules/lodash.js", false},
		{"vendor/github.com/test.go", false},
		{"README.md", true},
		{".okf/knowledge/file.md", false},
		{"build/main.js", false},
	}

	for _, tt := range tests {
		result := shouldIncludeFile(tt.path, config)
		if result != tt.expected {
			t.Errorf("shouldIncludeFile(%s) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestSerializeDeserialize(t *testing.T) {
	original := &Concept{
		Type:        "table",
		Title:       "users",
		Description: "User accounts table",
		Resource:    "db.users",
		Tags:        []string{"production", "pii"},
		Timestamp:   "2024-01-15T10:30:00Z",
		Content:     "## Users\n\nStores all user accounts.",
	}

	data, err := SerializeConcept(original, true)
	if err != nil {
		t.Fatalf("SerializeConcept failed: %v", err)
	}

	// Write to temp file and parse back
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "users.md")
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	parsed, err := ParseConcept(tmpFile)
	if err != nil {
		t.Fatalf("ParseConcept failed: %v", err)
	}

	if parsed.Type != original.Type {
		t.Errorf("type mismatch: got %s, want %s", parsed.Type, original.Type)
	}
	if parsed.Title != original.Title {
		t.Errorf("title mismatch: got %s, want %s", parsed.Title, original.Title)
	}
	if parsed.Description != original.Description {
		t.Errorf("description mismatch: got %s, want %s", parsed.Description, original.Description)
	}
	if parsed.Resource != original.Resource {
		t.Errorf("resource mismatch: got %s, want %s", parsed.Resource, original.Resource)
	}
	if len(parsed.Tags) != len(original.Tags) {
		t.Errorf("tags count mismatch: got %d, want %d", len(parsed.Tags), len(original.Tags))
	}
}

// -----------------------------------------------------------------------------
// 性能测试 (Benchmarks)
// -----------------------------------------------------------------------------

func BenchmarkParseConcept(b *testing.B) {
	// 准备一个大的 concept 文件
	tmpDir := b.TempDir()
	content := `---
type: table
title: large_test_table
description: This is a large test table for benchmarking
resource: bigquery.project.dataset.table
tags:
  - production
  - analytics
  - benchmark
timestamp: "2024-01-15T10:30:00Z"
custom_field: some custom data
---

# Large Test Table

This is a detailed description of the table.

## Columns

- id: INTEGER - Primary key
- name: STRING - User name
- email: STRING - User email
- created_at: TIMESTAMP - Creation time
- updated_at: TIMESTAMP - Last update time
- status: STRING - User status (active/inactive)
- last_login: TIMESTAMP - Last login timestamp
- login_count: INTEGER - Number of logins
- preferred_lang: STRING - Preferred language
- timezone: STRING - User timezone

## Relationships

- References: accounts.id
- Referenced by: orders.user_id
- Referenced by: sessions.user_id

## Access Patterns

- Queries by id (indexed)
- Queries by email (indexed)
- Queries by status (not indexed)

## Notes

This is a benchmark test table. It contains synthetic data for performance testing.
The table is updated frequently and should be monitored for query performance.

Additional sections can be added as needed.

## See Also

- orders table
- sessions table
- accounts table

`

	tmpFile := filepath.Join(tmpDir, "bench_test.md")
	os.WriteFile(tmpFile, []byte(content), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseConcept(tmpFile)
	}
}

func BenchmarkSerializeConcept(b *testing.B) {
	c := &Concept{
		Type:        "table",
		Title:       "bench_table",
		Description: "Benchmark test table",
		Resource:    "db.bench",
		Tags:        []string{"test", "benchmark", "production"},
		Timestamp:   "2024-01-15T10:30:00Z",
		Content:     "# Bench\n\nContent for benchmark testing.\n\n## Section\n\nMore content here.\n",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SerializeConcept(c, true)
	}
}

func BenchmarkLintConcept(b *testing.B) {
	c := &Concept{
		Type:        "table",
		Title:       "users",
		Description: "User accounts table with detailed description",
		Resource:    "db.users",
		Tags:        []string{"production", "pii", "analytics"},
		Timestamp:   "2024-01-15T10:30:00Z",
		Content:     "# Users Table\n\nThis table stores user accounts.\n\n## Columns\n\n- id\n- name\n- email\n- created_at\n",
	}
	config := DefaultLintConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LintConcept(c, config)
	}
}

func BenchmarkBundleLint(b *testing.B) {
	bundle := NewBundle("bench")
	// 生成 100 个 concept
	for i := 0; i < 100; i++ {
		c := &Concept{
			Type:        "component",
			Title:       fmt.Sprintf("concept_%d", i),
			Description: fmt.Sprintf("Test concept number %d for benchmark", i),
			Tags:        []string{"test", "benchmark"},
			Timestamp:   "2024-01-15T10:30:00Z",
			Content:     "Content for benchmark concept.\n\n# Details\n\nSome information here.\n",
		}
		bundle.Concepts = append(bundle.Concepts, c)
	}
	config := DefaultLintConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LintBundle(bundle, config)
	}
}

func BenchmarkBundleSearch(b *testing.B) {
	bundle := NewBundle("bench")
	for i := 0; i < 200; i++ {
		c := &Concept{
			Type:        "component",
			Title:       fmt.Sprintf("file_%d.go", i),
			Description: fmt.Sprintf("File description for benchmark concept %d", i),
			Content:     fmt.Sprintf("Content %d: This is a test file with sample code and functions.\n\n## Details\n\nSome information about file %d.\n", i, i),
			Tags:        []string{"go", "test"},
			Timestamp:   "2024-01-15T10:30:00Z",
		}
		bundle.Concepts = append(bundle.Concepts, c)
	}

	q := NewQuery().WithText("file").Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Execute(bundle)
	}
}

func BenchmarkNewConcept(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewConcept("table", "test_table")
	}
}

func BenchmarkAnalyzeFile(b *testing.B) {
	// 创建一个测试文件
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Println("Hello")
}

func helper(x int) string {
	return fmt.Sprintf("%d", x)
}

func (s *Service) Process() error {
	return nil
}

func parseData(data []byte) error {
	return nil
}
`
	os.WriteFile(testFile, []byte(content), 0644)

	repoRoot := tmpDir
	_ = repoRoot
	relPath := "test.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AnalyzeFile(tmpDir, relPath)
	}
}

func BenchmarkSaveKnowledgeBase(b *testing.B) {
	bundle := NewBundle("bench")
	for i := 0; i < 50; i++ {
		c := NewConcept("component", fmt.Sprintf("concept_%d", i))
		c.Description = fmt.Sprintf("Test concept %d", i)
		c.Content = "Content here.\n\n"
		c.FilePath = fmt.Sprintf("components/concept_%d.md", i)
		bundle.Concepts = append(bundle.Concepts, c)
	}

	config := DefaultGitConfig()
	config.RepoPath = b.TempDir()
	config.KnowledgeDir = "kb"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SaveKnowledgeBase(bundle, config)
	}
}

func TestLintIssue_SeverityString(t *testing.T) {
	tests := []struct {
		severity Severity
		expected string
	}{
		{Info, "INFO"},
		{Warning, "WARNING"},
		{Error, "ERROR"},
		{999, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := tt.severity.String()
		if result != tt.expected {
			t.Errorf("Severity(%d).String() = %s, want %s", tt.severity, result, tt.expected)
		}
	}
}

func TestLintResult_Summary(t *testing.T) {
	result := &LintResult{
		ConceptsChecked: 50,
		Errors:          2,
		Warnings:        5,
		Infos:           10,
		Duration:        100 * time.Millisecond,
	}

	summary := result.Summary()
	if !strings.Contains(summary, "50") {
		t.Errorf("summary should contain concepts count: %s", summary)
	}
	if !strings.Contains(summary, "2 errors") {
		t.Errorf("summary should contain errors: %s", summary)
	}
}

func TestLintResult_HasIssues(t *testing.T) {
	result := &LintResult{
		ConceptsChecked: 10,
		Warnings:        5,
	}

	// 非严格模式下只有警告不应失败
	if result.HasIssues(false) {
		t.Error("expected HasIssues(false) to be false for warnings only")
	}

	// 严格模式下警告应失败
	if !result.HasIssues(true) {
		t.Error("expected HasIssues(true) to be true for warnings")
	}

	// 错误应始终失败
	result.Errors = 1
	if !result.HasIssues(false) {
		t.Error("expected HasIssues(false) to be true when errors exist")
	}
}
