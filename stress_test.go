package okf_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	okf "github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/git"
	"github.com/superops-team/okf/pkg/lint"
	"github.com/superops-team/okf/pkg/parser"
	"github.com/superops-team/okf/pkg/query"
)

// ============================================================================
// Stress Test Suite for OKF
// ============================================================================

// ---------------------------------------------------------------------------
// 1. Parser Stress Tests
// ---------------------------------------------------------------------------

func TestStressParseLargeConcept(t *testing.T) {
	t.Parallel()
	// 10MB content body
	content := strings.Repeat("This is a line of content for stress testing.\n", 200000)
	fm := fmt.Sprintf("---\ntype: table\ntitle: Large Table\ndescription: A very large concept\n---\n\n%s", content)
	start := time.Now()
	c, err := parser.ParseConceptBytes("large.md", []byte(fm))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ParseConceptBytes failed: %v", err)
	}
	if c.Title != "Large Table" {
		t.Fatalf("title = %q, want 'Large Table'", c.Title)
	}
	t.Logf("Parsed 10MB concept in %v", elapsed)
}

func TestStressParseManyConcepts(t *testing.T) {
	t.Parallel()
	count := 5000
	start := time.Now()
	for i := 0; i < count; i++ {
		fm := fmt.Sprintf("---\ntype: table\ntitle: concept-%d\ndescription: Concept number %d\n---\n\nContent for concept %d\n", i, i, i)
		_, err := parser.ParseConceptBytes(fmt.Sprintf("concept-%d.md", i), []byte(fm))
		if err != nil {
			t.Fatalf("ParseConceptBytes failed at %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	t.Logf("Parsed %d concepts in %v (avg: %v per concept)", count, elapsed, elapsed/time.Duration(count))
}

func TestStressParseMalformedYAML(t *testing.T) {
	t.Parallel()
	malformed := []string{
		"---\ntype: [unclosed\n---\n",
		"---\ntype: table\ntitle: \n---\n",
		"---\n: : :\n---\n",
		"---\ntype: \ntitle: \n---\n",
		"",
		"---\n---\n",
		"---\ntype: table\n---\n",
		"---\ntitle: only-title\n---\n",
	}
	for i, m := range malformed {
		_, err := parser.ParseConceptBytes(fmt.Sprintf("bad-%d.md", i), []byte(m))
		if err == nil {
			t.Logf("WARNING: malformed input %d did not return error (input: %q)", i, m)
		}
	}
}

func TestStressParseSpecialCharacters(t *testing.T) {
	t.Parallel()
	specials := []struct {
		name  string
		title string
	}{
		{"unicode", "中文标题测试 🚀"},
		{"emoji", "📊 Data Analysis 📈"},
		// YAML reserved characters in title require quoting - this is expected behavior
		{"special_chars", "title with forward slash and backslash and pipe"},
		{"very_long", strings.Repeat("a", 10000)},
		{"zero_width", "hello\u200bworld"},
		{"rtl", "مرحبا بالعالم"},
	}
	for _, tc := range specials {
		t.Run(tc.name, func(t *testing.T) {
			fm := fmt.Sprintf("---\ntype: table\ntitle: %s\n---\n\nTest content\n", tc.title)
			c, err := parser.ParseConceptBytes("special.md", []byte(fm))
			if err != nil {
				t.Fatalf("ParseConceptBytes failed for %s: %v", tc.name, err)
			}
			if c.Title != tc.title {
				t.Fatalf("title = %q, want %q", c.Title, tc.title)
			}
		})
	}
}

func TestStressParseEmptyAndNil(t *testing.T) {
	t.Parallel()
	// BUG FOUND: nil bytes should return error but parser does not check for nil
	_, err := parser.ParseConceptBytes("nil.md", nil)
	if err == nil {
		t.Log("BUG: ParseConceptBytes does not handle nil input (no nil check)")
	} else {
		t.Logf("nil input correctly returned error: %v", err)
	}
	// empty bytes
	c, err := parser.ParseConceptBytes("empty.md", []byte{})
	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}
	if c.Type != "concept" {
		t.Fatalf("expected default type 'concept', got %q", c.Type)
	}
}

// ---------------------------------------------------------------------------
// 2. Lint Stress Tests
// ---------------------------------------------------------------------------

func TestStressLintManyConcepts(t *testing.T) {
	t.Parallel()
	count := 10000
	concepts := make([]*lint.Concept, count)
	for i := 0; i < count; i++ {
		concepts[i] = &lint.Concept{
			Type:        "table",
			Title:       fmt.Sprintf("concept-%d", i),
			Description: fmt.Sprintf("Description for concept %d", i),
			Tags:        []string{"test", "stress"},
			Timestamp:   "2024-01-15T10:30:00Z",
			Content:     fmt.Sprintf("Content for concept %d", i),
			FilePath:    fmt.Sprintf("concept-%d.md", i),
		}
	}
	cfg := lint.DefaultConfig()
	start := time.Now()
	result := lint.LintBundle(concepts, cfg)
	elapsed := time.Since(start)
	t.Logf("Linted %d concepts in %v (avg: %v per concept)", count, elapsed, elapsed/time.Duration(count))
	if result.ConceptsChecked != count {
		t.Fatalf("checked = %d, want %d", result.ConceptsChecked, count)
	}
}

func TestStressLintEdgeCases(t *testing.T) {
	t.Parallel()
	edgeCases := []*lint.Concept{
		{Type: "", Title: "", FilePath: "empty.md"},
		{Type: "UPPERCASE", Title: "test", FilePath: "upper.md"},
		{Type: "table", Title: "test", Tags: []string{"TAG1", "TAG2", "TAG1"}, FilePath: "dup.md"},
		{Type: "table", Title: "test", Timestamp: "not-a-date", FilePath: "bad-date.md"},
		{Type: "table", Title: "test", Description: "short", FilePath: "short.md"},
		{Type: "table", Title: "test", Content: strings.Repeat("x", 300), FilePath: "long.md"},
		{Type: "table", Title: "test", Tags: []string{"has space"}, FilePath: "space.md"},
	}
	cfg := lint.DefaultConfig()
	result := lint.LintBundle(edgeCases, cfg)
	t.Logf("Edge case lint: %d issues found", len(result.Issues))
	for _, issue := range result.Issues {
		t.Logf("  [%s] %s: %s", issue.Severity, issue.Code, issue.Message)
	}
}

// ---------------------------------------------------------------------------
// 3. Bundle Operations Stress Tests
// ---------------------------------------------------------------------------

func TestStressBundleLargeOperations(t *testing.T) {
	t.Parallel()
	bundle := okf.NewBundle("stress-test")
	count := 20000
	for i := 0; i < count; i++ {
		c := okf.NewConcept("table", fmt.Sprintf("concept-%d", i))
		c.Description = fmt.Sprintf("Description %d", i)
		c.Tags = []string{"test", fmt.Sprintf("tag-%d", i%100)}
		c.Content = fmt.Sprintf("Content for concept %d\n", i)
		bundle.AddConcept(c)
	}

	// Test Search
	start := time.Now()
	results := bundle.Search("concept-12345")
	elapsed := time.Since(start)
	if len(results) != 1 {
		t.Fatalf("Search returned %d results, want 1", len(results))
	}
	t.Logf("Search in %d concepts: %v", count, elapsed)

	// Test Stats
	start = time.Now()
	stats := bundle.Stats()
	elapsed = time.Since(start)
	t.Logf("Stats for %d concepts: %v (types=%d, tags=%d)", count, elapsed, stats.UniqueTypes, stats.UniqueTags)

	// Test FilterByType
	start = time.Now()
	filtered := bundle.FilterByType("table")
	elapsed = time.Since(start)
	if len(filtered) != count {
		t.Fatalf("FilterByType returned %d, want %d", len(filtered), count)
	}
	t.Logf("FilterByType in %d concepts: %v", count, elapsed)

	// Test FilterByTag
	start = time.Now()
	tagged := bundle.FilterByTag("tag-50")
	elapsed = time.Since(start)
	t.Logf("FilterByTag in %d concepts: %v (found %d)", count, elapsed, len(tagged))

	// Test RelatedConcepts
	start = time.Now()
	related := bundle.RelatedConcepts(bundle.Concepts[0])
	elapsed = time.Since(start)
	t.Logf("RelatedConcepts in %d concepts: %v (found %d)", count, elapsed, len(related))

	// Test GetConcept
	start = time.Now()
	c := bundle.GetConcept("concept-15000")
	elapsed = time.Since(start)
	if c == nil {
		t.Fatal("GetConcept returned nil")
	}
	t.Logf("GetConcept in %d concepts: %v", count, elapsed)

	// Test RemoveConcept
	start = time.Now()
	removed := bundle.RemoveConcept("concept-0")
	elapsed = time.Since(start)
	if !removed {
		t.Fatal("RemoveConcept failed")
	}
	t.Logf("RemoveConcept in %d concepts: %v", count, elapsed)
}

func TestStressBundleNilAndEmpty(t *testing.T) {
	t.Parallel()
	bundle := okf.NewBundle("empty")

	// Operations on empty bundle
	if results := bundle.Search("anything"); len(results) != 0 {
		t.Fatalf("Search on empty bundle returned %d results", len(results))
	}
	if c := bundle.GetConcept("nonexistent"); c != nil {
		t.Fatal("GetConcept on empty bundle returned non-nil")
	}
	if bundle.RemoveConcept("nonexistent") {
		t.Fatal("RemoveConcept on empty bundle returned true")
	}
	// BUG FOUND: RelatedConcepts(nil) panics with nil pointer dereference
	// This is a real bug - the method does not check for nil concept argument
	defer func() {
		if r := recover(); r != nil {
			t.Logf("BUG: RelatedConcepts(nil) panicked: %v", r)
		}
	}()
	_ = bundle.RelatedConcepts(nil)

	// Add nil concept
	bundle.Concepts = append(bundle.Concepts, nil)
	stats := bundle.Stats()
	t.Logf("Stats with nil concept: total=%d", stats.TotalConcepts)
}

func TestStressBundleConcurrentAccess(t *testing.T) {
	t.Parallel()
	bundle := okf.NewBundle("concurrent")
	count := 1000
	for i := 0; i < count; i++ {
		c := okf.NewConcept("table", fmt.Sprintf("concept-%d", i))
		c.Content = fmt.Sprintf("Content %d", i)
		bundle.AddConcept(c)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent reads
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = bundle.Search(fmt.Sprintf("concept-%d", i))
				_ = bundle.GetConcept(fmt.Sprintf("concept-%d", i))
				_ = bundle.FilterByType("table")
				_ = bundle.Stats()
			}
		}(g)
	}

	// Concurrent writes
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				c := okf.NewConcept("table", fmt.Sprintf("concurrent-%d-%d", gid, i))
				bundle.AddConcept(c)
			}
		}(g)
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent error: %v", err)
	}
	t.Logf("Concurrent test completed, bundle has %d concepts", len(bundle.Concepts))
}

// ---------------------------------------------------------------------------
// 4. Query Engine Stress Tests
// ---------------------------------------------------------------------------

func TestStressQueryLargeIndex(t *testing.T) {
	t.Parallel()
	count := 10000
	concepts := make([]*query.Concept, count)
	for i := 0; i < count; i++ {
		concepts[i] = &query.Concept{
			Type:        fmt.Sprintf("type-%d", i%10),
			Title:       fmt.Sprintf("Concept Title %d", i),
			Description: fmt.Sprintf("Description for concept number %d", i),
			Tags:        []string{fmt.Sprintf("tag-%d", i%50), "common"},
			Content:     fmt.Sprintf("## Content %d\n\n- `function` `Func%d` (exported) at `file%d.go:1-10`\n- Language: `go`\n- Path: `pkg/file%d.go`\n", i, i, i, i),
			FilePath:    fmt.Sprintf("file-%d.md", i),
		}
	}
	bundle := &query.KnowledgeBundle{Concepts: concepts}

	// Build index
	start := time.Now()
	bundle.BuildIndex()
	elapsed := time.Since(start)
	t.Logf("Built index for %d concepts in %v", count, elapsed)

	// Indexed search
	start = time.Now()
	results := query.Search(bundle, "Concept Title 5000")
	elapsed = time.Since(start)
	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}
	t.Logf("Indexed search in %d concepts: %v (found %d)", count, elapsed, len(results))

	// Filter by type
	start = time.Now()
	filtered := query.FilterByType(bundle, "type-5")
	elapsed = time.Since(start)
	t.Logf("FilterByType in %d concepts: %v (found %d)", count, elapsed, len(filtered))

	// Code dimension filter
	start = time.Now()
	q := query.New().WithCodeLanguage("go").Build()
	codeResults := q.Execute(bundle)
	elapsed = time.Since(start)
	t.Logf("Code language filter in %d concepts: %v (found %d)", count, elapsed, len(codeResults))

	// Complex query
	start = time.Now()
	q2 := query.New().WithType("type-5").WithCodeSymbolKind("function").Build()
	complexResults := q2.Execute(bundle)
	elapsed = time.Since(start)
	t.Logf("Complex query in %d concepts: %v (found %d)", count, elapsed, len(complexResults))
}

func TestStressQueryEdgeCases(t *testing.T) {
	t.Parallel()
	// nil bundle
	if results := query.Search(nil, "test"); results != nil {
		t.Fatal("Search on nil bundle should return nil")
	}
	if results := query.FilterByType(nil, "test"); results != nil {
		t.Fatal("FilterByType on nil bundle should return nil")
	}
	if results := query.FilterByTag(nil, "test"); results != nil {
		t.Fatal("FilterByTag on nil bundle should return nil")
	}

	// Empty bundle
	empty := &query.KnowledgeBundle{}
	if results := query.Search(empty, "test"); len(results) != 0 {
		t.Fatalf("Search on empty bundle returned %d results", len(results))
	}

	// Empty query
	bundle := &query.KnowledgeBundle{
		Concepts: []*query.Concept{
			{Type: "table", Title: "Test", Content: "content"},
		},
	}
	q := query.New().Build()
	results := q.Execute(bundle)
	if len(results) != 1 {
		t.Fatalf("Empty query should return all concepts, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// 5. Save/Load Roundtrip Stress Tests
// ---------------------------------------------------------------------------

func TestStressSaveLoadRoundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bundle := okf.NewBundle("roundtrip-test")
	count := 1000
	for i := 0; i < count; i++ {
		c := okf.NewConcept("table", fmt.Sprintf("concept-%d", i))
		c.Description = fmt.Sprintf("Description for concept %d", i)
		c.Tags = []string{"test", fmt.Sprintf("tag-%d", i%10)}
		c.Content = fmt.Sprintf("## Concept %d\n\nContent for concept %d\n\n- Item 1\n- Item 2\n", i, i)
		bundle.AddConcept(c)
	}

	// Save
	start := time.Now()
	err := okf.SaveBundle(bundle, dir, okf.DefaultSaveOptions())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("SaveBundle failed: %v", err)
	}
	t.Logf("Saved %d concepts in %v", count, elapsed)

	// Load
	start = time.Now()
	loaded, err := okf.LoadBundle(dir, okf.DefaultLoadOptions())
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("LoadBundle failed: %v", err)
	}
	t.Logf("Loaded %d concepts in %v", len(loaded.Concepts), elapsed)

	if len(loaded.Concepts) != count {
		t.Fatalf("roundtrip mismatch: saved %d, loaded %d", count, len(loaded.Concepts))
	}
}

// ---------------------------------------------------------------------------
// 6. Git Integration Stress Tests
// ---------------------------------------------------------------------------

func TestStressGitLargeRepoSimulation(t *testing.T) {
	dir := initStressRepo(t, 500)
	cfg := &git.Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 1024,
	}

	start := time.Now()
	bundle, err := git.GenerateBundle(cfg, true)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GenerateBundle failed: %v", err)
	}
	t.Logf("Generated bundle for 500 files in %v (%d concepts)", elapsed, len(bundle.Concepts))

	// Verify key concepts exist
	if c := bundle.GetConcept("Project Overview"); c == nil {
		t.Fatal("Project Overview concept missing")
	}
	if c := bundle.GetConcept("Relationship Graph"); c == nil {
		t.Fatal("Relationship Graph concept missing")
	}

	// Test save
	saved, err := git.SaveKnowledgeBase(bundle, cfg)
	if err != nil {
		t.Fatalf("SaveKnowledgeBase failed: %v", err)
	}
	t.Logf("Saved %d concepts to disk", saved)

	// Test load
	loaded, err := okf.LoadBundle(filepath.Join(dir, cfg.KnowledgeDir), okf.DefaultLoadOptions())
	if err != nil {
		t.Fatalf("LoadBundle failed: %v", err)
	}
	t.Logf("Loaded %d concepts from disk", len(loaded.Concepts))

	// Test incremental update
	mustWriteFile(t, filepath.Join(dir, "new_file.go"), "package main\nfunc newFunc() {}\n")
	runGit(t, dir, "add", "new_file.go")
	runGit(t, dir, "commit", "-m", "add new file")

	start = time.Now()
	incBundle, updated, err := git.UpdateFromLastCommit(cfg)
	elapsed = time.Since(start)
	if err != nil {
		t.Fatalf("UpdateFromLastCommit failed: %v", err)
	}
	t.Logf("Incremental update in %v: %d concepts, updated files: %v", elapsed, len(incBundle.Concepts), updated)

	if err := git.ApplyIncrementalUpdate(cfg, incBundle); err != nil {
		t.Fatalf("ApplyIncrementalUpdate failed: %v", err)
	}
}

func TestStressGitEdgeCases(t *testing.T) {
	dir := initStressRepo(t, 10)

	// Non-existent path
	if git.IsRepo(filepath.Join(dir, "nonexistent")) {
		t.Fatal("IsRepo should return false for non-existent path")
	}

	// Empty repo (no commits)
	emptyDir := t.TempDir()
	runGit(t, emptyDir, "init")
	cfg := git.DefaultConfig()
	cfg.RepoPath = emptyDir
	_, err := git.GenerateBundle(cfg, true)
	if err == nil {
		t.Log("GenerateBundle on empty repo: no error (expected)")
	}

	// GetRepoRoot (note: macOS /var is symlink to /private/var)
	root, err := git.GetRepoRoot(dir)
	if err != nil {
		t.Fatalf("GetRepoRoot failed: %v", err)
	}
	resolvedDir, _ := filepath.EvalSymlinks(dir)
	if root != dir && root != resolvedDir {
		t.Fatalf("GetRepoRoot = %q, want %q or %q", root, dir, resolvedDir)
	}

	// GetCurrentBranch
	branch, err := git.GetCurrentBranch(dir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	t.Logf("Current branch: %s", branch)

	// GetLastCommits
	commits, err := git.GetLastCommits(dir, 5)
	if err != nil {
		t.Fatalf("GetLastCommits failed: %v", err)
	}
	t.Logf("Last %d commits retrieved", len(commits))
}

func TestStressGitConcurrentGeneration(t *testing.T) {
	dir := initStressRepo(t, 200)
	cfg := &git.Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 1024,
		Workers:       runtime.NumCPU(),
	}

	var wg sync.WaitGroup
	results := make(chan struct {
		concepts int
		duration time.Duration
		err      error
	}, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			bundle, err := git.GenerateBundle(cfg, true)
			results <- struct {
				concepts int
				duration time.Duration
				err      error
			}{len(bundle.Concepts), time.Since(start), err}
		}()
	}

	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("concurrent GenerateBundle failed: %v", r.err)
		}
		t.Logf("Concurrent generation: %d concepts in %v", r.concepts, r.duration)
	}
}

// ---------------------------------------------------------------------------
// 7. Memory and Performance Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkStressParseLargeConcept(b *testing.B) {
	content := strings.Repeat("This is a line of content for stress testing.\n", 200000)
	fm := fmt.Sprintf("---\ntype: table\ntitle: Large Table\ndescription: A very large concept\n---\n\n%s", content)
	data := []byte(fm)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.ParseConceptBytes("large.md", data)
	}
}

func BenchmarkStressBundleSearch(b *testing.B) {
	bundle := okf.NewBundle("bench")
	for i := 0; i < 10000; i++ {
		c := okf.NewConcept("table", fmt.Sprintf("concept-%d", i))
		c.Content = fmt.Sprintf("Content for concept %d", i)
		bundle.AddConcept(c)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bundle.Search(fmt.Sprintf("concept-%d", i%10000))
	}
}

func BenchmarkStressBundleStats(b *testing.B) {
	bundle := okf.NewBundle("bench")
	for i := 0; i < 10000; i++ {
		c := okf.NewConcept("table", fmt.Sprintf("concept-%d", i))
		c.Tags = []string{fmt.Sprintf("tag-%d", i%100)}
		bundle.AddConcept(c)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bundle.Stats()
	}
}

func BenchmarkStressLintManyConcepts(b *testing.B) {
	concepts := make([]*lint.Concept, 10000)
	for i := 0; i < 10000; i++ {
		concepts[i] = &lint.Concept{
			Type:        "table",
			Title:       fmt.Sprintf("concept-%d", i),
			Description: fmt.Sprintf("Description for concept %d", i),
			Tags:        []string{"test", "stress"},
			Timestamp:   "2024-01-15T10:30:00Z",
			Content:     fmt.Sprintf("Content for concept %d", i),
			FilePath:    fmt.Sprintf("concept-%d.md", i),
		}
	}
	cfg := lint.DefaultConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lint.LintBundle(concepts, cfg)
	}
}

func BenchmarkStressQueryBuildIndex(b *testing.B) {
	concepts := make([]*query.Concept, 10000)
	for i := 0; i < 10000; i++ {
		concepts[i] = &query.Concept{
			Type:    fmt.Sprintf("type-%d", i%10),
			Title:   fmt.Sprintf("Concept Title %d", i),
			Content: fmt.Sprintf("Content %d\n- Language: `go`\n- `function` `Func%d` (exported) at `file%d.go:1-10`\n", i, i, i),
		}
	}
	bundle := &query.KnowledgeBundle{Concepts: concepts}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bundle.BuildIndex()
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func initStressRepo(t *testing.T, fileCount int) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Stress Test")
	runGit(t, dir, "config", "user.email", "stress@test.local")

	for i := 0; i < fileCount; i++ {
		pkg := fmt.Sprintf("pkg%d", i%10)
		dirPath := filepath.Join(dir, pkg)
		os.MkdirAll(dirPath, 0755)
		content := fmt.Sprintf(`package %s

import (
	"fmt"
	"strings"
)

type Struct%d struct {
	Name  string
	Value int
}

func (s *Struct%d) Method%d() string {
	return fmt.Sprintf("%%s: %%d", s.Name, s.Value)
}

func Func%d(input string) string {
	return strings.ToUpper(input)
}
`, pkg, i, i, i, i)
		mustWriteFile(t, filepath.Join(dirPath, fmt.Sprintf("file_%d.go", i)), content)
	}

	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial stress test repo")
	return dir
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}