package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/okf"
	"gopkg.in/yaml.v3"
)

// GenerateBundle creates a complete knowledge bundle from a Git repository.
func GenerateBundle(cfg *Config, force bool) (*okf.KnowledgeBundle, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	repoRoot, err := GetRepoRoot(cfg.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	cfg.RepoPath = repoRoot

	if cfg.Author == "" {
		if out, err := exec.Command("git", "config", "--get", "user.name").CombinedOutput(); err == nil {
			cfg.Author = strings.TrimSpace(string(out))
		}
	}
	if cfg.Email == "" {
		if out, err := exec.Command("git", "config", "--get", "user.email").CombinedOutput(); err == nil {
			cfg.Email = strings.TrimSpace(string(out))
		}
	}

	bundle := &okf.KnowledgeBundle{
		Name:     filepath.Base(repoRoot),
		RootPath: filepath.Join(repoRoot, cfg.KnowledgeDir),
	}

	files, err := ListTrackedFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	var relevantFiles []string
	for _, f := range files {
		if ShouldInclude(f, cfg) {
			relevantFiles = append(relevantFiles, f)
		}
	}

	generated := 0
	typeCounts := make(map[string]int)
	authorCounts := make(map[string]int)

	for _, filePath := range relevantFiles {
		summary, err := AnalyzeFile(repoRoot, filePath)
		if err != nil {
			continue
		}

		concept := conceptFromSummary(summary, cfg, bundle)
		bundle.Concepts = append(bundle.Concepts, concept)
		typeCounts[summary.Type]++
		if summary.LastAuthor != "" {
			authorCounts[summary.LastAuthor]++
		}
		generated++
	}

	// Add project overview
	projectConcept := createProjectOverview(cfg, repoRoot, files, typeCounts, authorCounts)
	bundle.Concepts = append([]*okf.Concept{projectConcept}, bundle.Concepts...)

	// Add directory structure
	dirConcept := createDirectoryStructure(cfg, repoRoot, relevantFiles)
	bundle.Concepts = append(bundle.Concepts, dirConcept)

	// Add contributors
	if len(authorCounts) > 0 {
		contribConcept := createContributors(cfg, authorCounts)
		bundle.Concepts = append(bundle.Concepts, contribConcept)
	}

	return bundle, nil
}

func conceptFromSummary(s *FileSummary, cfg *Config, bundle *okf.KnowledgeBundle) *okf.Concept {
	c := okf.NewConcept("component", filepath.Base(s.RelativePath))
	c.Description = fmt.Sprintf("%s file: %s (%d lines)", strings.ToUpper(s.Type), s.RelativePath, s.LineCount)
	c.Resource = s.RelativePath
	c.Timestamp = s.LastModified.Format(time.RFC3339)
	c.FilePath = s.RelativePath

	tags := []string{s.Type}
	if s.LastAuthor != "" {
		tags = append(tags, sanitizeTag(s.LastAuthor))
	}
	if s.CommitCount > 10 {
		tags = append(tags, "frequently-modified")
	}
	c.Tags = tags

	var content strings.Builder
	fmt.Fprintf(&content, "## File: `%s`\n\n", s.RelativePath)
	fmt.Fprintf(&content, "**Type:** %s\n\n", strings.ToUpper(s.Type))
	fmt.Fprintf(&content, "**Size:** %d bytes / %d lines\n\n", s.Size, s.LineCount)
	if s.LastCommit != "" {
		fmt.Fprintf(&content, "**Last Commit:** `%s`\n\n", s.LastCommit)
	}
	if s.LastAuthor != "" {
		fmt.Fprintf(&content, "**Last Modified By:** %s\n\n", s.LastAuthor)
	}
	fmt.Fprintf(&content, "**Commit Count:** %d\n\n", s.CommitCount)
	fmt.Fprintf(&content, "**Last Modified:** %s\n\n", s.LastModified.Format("2006-01-02 15:04:05 MST"))

	c.Content = content.String()
	return c
}

func createProjectOverview(cfg *Config, repoRoot string, allFiles []string, typeCounts map[string]int, authorCounts map[string]int) *okf.Concept {
	c := okf.NewConcept("project", "Project Overview")
	c.FilePath = "project/project_overview.md"

	branch, _ := GetCurrentBranch(repoRoot)
	commit, _ := GetCurrentCommit(repoRoot)

	var typeList []string
	for t, count := range typeCounts {
		typeList = append(typeList, fmt.Sprintf("**%s:** %d files", t, count))
	}
	sort.Strings(typeList)

	var topAuthors []string
	type kv struct {
		Key   string
		Value int
	}
	var sortedAuthors []kv
	for k, v := range authorCounts {
		sortedAuthors = append(sortedAuthors, kv{k, v})
	}
	sort.Slice(sortedAuthors, func(i, j int) bool {
		return sortedAuthors[i].Value > sortedAuthors[j].Value
	})
	for i, a := range sortedAuthors {
		if i >= 10 {
			break
		}
		topAuthors = append(topAuthors, fmt.Sprintf("- %s (%d files)", a.Key, a.Value))
	}

	c.Description = fmt.Sprintf("Project %s on branch %s with %d tracked files", filepath.Base(repoRoot), branch, len(allFiles))

	var content strings.Builder
	fmt.Fprintf(&content, "## Project: `%s`\n\n", filepath.Base(repoRoot))
	fmt.Fprintf(&content, "**Branch:** `%s`\n\n", branch)
	if commit != "" {
		fmt.Fprintf(&content, "**Current Commit:** `%s`\n\n", commit[:min(len(commit), 12)])
	}
	fmt.Fprintf(&content, "**Total Files:** %d\n\n", len(allFiles))

	if len(typeList) > 0 {
		fmt.Fprintf(&content, "### File Types\n\n")
		for _, t := range typeList {
			fmt.Fprintf(&content, "- %s\n", t)
		}
		fmt.Fprintf(&content, "\n")
	}

	if len(topAuthors) > 0 {
		fmt.Fprintf(&content, "### Top Contributors\n\n")
		for _, a := range topAuthors {
			fmt.Fprintf(&content, "%s\n", a)
		}
		fmt.Fprintf(&content, "\n")
	}

	fmt.Fprintf(&content, "### Generated At\n\n%s\n\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	c.Content = content.String()
	c.Resource = repoRoot
	if cfg.Author != "" {
		c.Tags = []string{sanitizeTag(cfg.Author), "project"}
	} else {
		c.Tags = []string{"project"}
	}

	return c
}

func createDirectoryStructure(cfg *Config, repoRoot string, relevantFiles []string) *okf.Concept {
	c := okf.NewConcept("system", "Project Structure")
	c.FilePath = "project/structure.md"

	dirTree := make(map[string][]string)
	for _, f := range relevantFiles {
		dir := filepath.Dir(f)
		dirTree[dir] = append(dirTree[dir], filepath.Base(f))
	}

	var dirs []string
	for d := range dirTree {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	var content strings.Builder
	fmt.Fprintf(&content, "## Project Structure\n\n")
	fmt.Fprintf(&content, "**Total Directories:** %d\n\n", len(dirs))
	fmt.Fprintf(&content, "```\n%s/\n", filepath.Base(repoRoot))

	for _, dir := range dirs {
		if dir == "." || dir == "" {
			continue
		}
		depth := strings.Count(dir, "/")
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(&content, "%s%s/ (%d files)\n", indent, filepath.Base(dir), len(dirTree[dir]))
	}
	fmt.Fprintf(&content, "```\n\n")

	c.Description = fmt.Sprintf("Directory structure for %s", filepath.Base(repoRoot))
	c.Content = content.String()
	c.Tags = []string{"structure", "project"}

	return c
}

func createContributors(cfg *Config, authorCounts map[string]int) *okf.Concept {
	c := okf.NewConcept("people", "Contributors")
	c.FilePath = "project/contributors.md"

	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range authorCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	total := 0
	for _, v := range authorCounts {
		total += v
	}

	var content strings.Builder
	fmt.Fprintf(&content, "## Contributors\n\n")
	fmt.Fprintf(&content, "**Total Contributors:** %d\n\n", len(authorCounts))
	fmt.Fprintf(&content, "**Total File Ownership:** %d entries\n\n", total)

	fmt.Fprintf(&content, "### Top Contributors\n\n")
	for i, author := range sorted {
		percent := float64(author.Value) / float64(total) * 100
		fmt.Fprintf(&content, "%d. **%s** - %d files (%.1f%%)\n", i+1, author.Key, author.Value, percent)
	}

	c.Description = "Contributors and their file involvement"
	c.Content = content.String()
	c.Tags = []string{"contributors", "people"}

	return c
}

func sanitizeFilename(name string) string {
	result := name
	for _, r := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		result = strings.ReplaceAll(result, r, "_")
	}
	return result
}

func sanitizeTag(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = regexp.MustCompile(`[^a-z0-9_\-]`).ReplaceAllString(s, "_")
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// UpdateBundle updates bundle based on changed files.
func UpdateBundle(cfg *Config, changedFiles []string) (*okf.KnowledgeBundle, []string, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	repoRoot, err := GetRepoRoot(cfg.RepoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("not a git repository: %w", err)
	}
	cfg.RepoPath = repoRoot

	bundle := &okf.KnowledgeBundle{
		Name:     filepath.Base(repoRoot) + "_incremental",
		RootPath: filepath.Join(repoRoot, cfg.KnowledgeDir),
	}

	var updatedPaths []string

	for _, file := range changedFiles {
		if !ShouldInclude(file, cfg) {
			continue
		}

		fullPath := filepath.Join(repoRoot, file)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			concept := okf.NewConcept("deleted", filepath.Base(file))
			concept.Description = fmt.Sprintf("File %s was deleted", file)
			concept.Resource = file
			concept.Timestamp = time.Now().UTC().Format(time.RFC3339)
			concept.FilePath = file
			concept.Content = fmt.Sprintf("## Deleted File: `%s`\n\nRemoved at %s\n", file, time.Now().Format("2006-01-02 15:04:05 MST"))
			bundle.Concepts = append(bundle.Concepts, concept)
			updatedPaths = append(updatedPaths, file+" (deleted)")
			continue
		}

		summary, err := AnalyzeFile(repoRoot, file)
		if err != nil {
			continue
		}

		concept := conceptFromSummary(summary, cfg, bundle)
		bundle.Concepts = append(bundle.Concepts, concept)
		updatedPaths = append(updatedPaths, file)
	}

	return bundle, updatedPaths, nil
}

// UpdateFromLastCommit updates based on the last commit.
func UpdateFromLastCommit(cfg *Config) (*okf.KnowledgeBundle, []string, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	repoRoot, err := GetRepoRoot(cfg.RepoPath)
	if err != nil {
		return nil, nil, err
	}

	commits, err := GetLastCommits(repoRoot, 1)
	if err != nil || len(commits) == 0 {
		return nil, nil, fmt.Errorf("no commits found: %w", err)
	}

	commit := commits[0]
	var changedFiles []string
	changedFiles = append(changedFiles, commit.AddedFiles...)
	changedFiles = append(changedFiles, commit.ModifiedFiles...)
	changedFiles = append(changedFiles, commit.DeletedFiles...)

	if len(changedFiles) == 0 {
		return nil, nil, nil
	}

	return UpdateBundle(cfg, changedFiles)
}

// SaveKnowledgeBase saves bundle to disk.
func SaveKnowledgeBase(bundle *okf.KnowledgeBundle, cfg *Config) (int, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	outputDir := filepath.Join(cfg.RepoPath, cfg.KnowledgeDir)
	os.MkdirAll(outputDir, 0755)

	saved := 0
	for _, concept := range bundle.Concepts {
		relPath := concept.FilePath
		if relPath == "" {
			relPath = filepath.Join(sanitizeFilename(concept.Type)+"s", sanitizeFilename(concept.Title)+".md")
		}
		if !strings.HasSuffix(relPath, ".md") {
			relPath = relPath + ".md"
		}

		fullPath := filepath.Join(outputDir, relPath)
		os.MkdirAll(filepath.Dir(fullPath), 0755)

		// Use parser to serialize
		content, err := serialize(concept, true)
		if err != nil {
			continue
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			continue
		}
		saved++
	}

	return saved, nil
}

func serialize(c *okf.Concept, prettyPrint bool) ([]byte, error) {
	type fm struct {
		Type        string   `yaml:"type"`
		Title       string   `yaml:"title"`
		Description string   `yaml:"description,omitempty"`
		Resource    string   `yaml:"resource,omitempty"`
		Tags        []string `yaml:"tags,omitempty"`
		Timestamp   string   `yaml:"timestamp,omitempty"`
	}

	f := fm{
		Type:        c.Type,
		Title:       c.Title,
		Description: c.Description,
		Resource:    c.Resource,
		Tags:        c.Tags,
		Timestamp:   c.Timestamp,
	}

	yamlData, _ := yaml.Marshal(&f)

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlData)
	buf.WriteString("---\n")
	buf.WriteString(c.Content)

	return buf.Bytes(), nil
}
