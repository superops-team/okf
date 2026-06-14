package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

const relationshipGraphResource = "okf://relationships"

var (
	packageLinePattern     = regexp.MustCompile("\\*\\*Package:\\*\\* `([^`]+)`")
	importLinePattern      = regexp.MustCompile("^- `([^`]+)`$")
	generatedSymbolPattern = regexp.MustCompile("^- `([^`]+)` `([^`]+)` \\(([^)]+)\\) at `([^`]+)`$")
)

// Relationship records one lightweight code relationship discovered during indexing.
type Relationship struct {
	Kind   string
	Source string
	Target string
	Meta   string
}

// RelationshipGraph contains deterministic relationship records for the knowledge base.
type RelationshipGraph struct {
	Relationships []Relationship
}

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
	metadataByFile, err := BatchGitMetadata(repoRoot, relevantFiles)
	if err != nil {
		return nil, err
	}
	summaries, err := AnalyzeFilesWithMetadata(repoRoot, relevantFiles, metadataByFile, cfg.Workers)
	if err != nil {
		return nil, err
	}

	for _, summary := range summaries {
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

	// Add lightweight relationship graph
	relationshipConcept := createRelationshipGraphConcept(BuildRelationshipGraphFromSummaries(summaries))
	bundle.Concepts = append(bundle.Concepts, relationshipConcept)

	// Add contributors
	if len(authorCounts) > 0 {
		contribConcept := createContributors(cfg, authorCounts)
		bundle.Concepts = append(bundle.Concepts, contribConcept)
	}

	return bundle, nil
}

// BuildRelationshipGraphFromSummaries builds file/package import and ownership relationships.
func BuildRelationshipGraphFromSummaries(summaries []*FileSummary) RelationshipGraph {
	seen := make(map[string]bool)
	var relationships []Relationship
	add := func(kind, source, target, meta string) {
		if source == "" || target == "" {
			return
		}
		key := kind + "\x00" + source + "\x00" + target + "\x00" + meta
		if seen[key] {
			return
		}
		seen[key] = true
		relationships = append(relationships, Relationship{Kind: kind, Source: source, Target: target, Meta: meta})
	}

	for _, summary := range summaries {
		pkg := packageName(summary)
		for _, imp := range summary.Imports {
			add("file_import", summary.RelativePath, imp, "")
			add("package_import", pkg, imp, "")
		}
		for _, symbol := range summary.Symbols {
			qualified := qualifiedSymbolName(symbol)
			add("file_owns_symbol", summary.RelativePath, qualified, symbol.Kind)
			add("package_owns_symbol", symbol.Package, displaySymbolName(symbol), symbol.Kind)
		}
	}

	sortRelationships(relationships)
	return RelationshipGraph{Relationships: relationships}
}

func packageName(summary *FileSummary) string {
	for _, symbol := range summary.Symbols {
		if symbol.Package != "" {
			return symbol.Package
		}
	}
	return summary.Type
}

func qualifiedSymbolName(symbol Symbol) string {
	name := displaySymbolName(symbol)
	if symbol.Package == "" {
		return name
	}
	return symbol.Package + "." + name
}

func displaySymbolName(symbol Symbol) string {
	if symbol.Receiver != "" {
		return symbol.Receiver + "." + symbol.Name
	}
	return symbol.Name
}

func sortRelationships(relationships []Relationship) {
	sort.Slice(relationships, func(i, j int) bool {
		if relationships[i].Kind != relationships[j].Kind {
			return relationships[i].Kind < relationships[j].Kind
		}
		if relationships[i].Source != relationships[j].Source {
			return relationships[i].Source < relationships[j].Source
		}
		if relationships[i].Target != relationships[j].Target {
			return relationships[i].Target < relationships[j].Target
		}
		return relationships[i].Meta < relationships[j].Meta
	})
}

func createRelationshipGraphConcept(graph RelationshipGraph) *okf.Concept {
	c := okf.NewConcept("system", "Relationship Graph")
	c.FilePath = "project/relationships.md"
	c.Resource = relationshipGraphResource
	c.Description = "Lightweight relationship graph for file imports, package imports, and symbol ownership"
	c.Tags = []string{"relationship", "graph", "generated"}

	sections := []struct {
		kind  string
		title string
		verb  string
	}{
		{kind: "file_import", title: "File Imports", verb: "imports"},
		{kind: "package_import", title: "Package Imports", verb: "imports"},
		{kind: "file_owns_symbol", title: "File Owns Symbols", verb: "owns"},
		{kind: "package_owns_symbol", title: "Package Owns Symbols", verb: "owns"},
	}

	var content strings.Builder
	fmt.Fprintf(&content, "## Relationship Graph\n\n")
	fmt.Fprintf(&content, "**Total Relationships:** %d\n\n", len(graph.Relationships))
	for _, section := range sections {
		fmt.Fprintf(&content, "### %s\n\n", section.title)
		wrote := false
		for _, relationship := range graph.Relationships {
			if relationship.Kind != section.kind {
				continue
			}
			if relationship.Meta != "" {
				fmt.Fprintf(&content, "- `%s` %s `%s` (%s)\n", relationship.Source, section.verb, relationship.Target, relationship.Meta)
			} else {
				fmt.Fprintf(&content, "- `%s` %s `%s`\n", relationship.Source, section.verb, relationship.Target)
			}
			wrote = true
		}
		if !wrote {
			fmt.Fprintf(&content, "- None\n")
		}
		fmt.Fprintf(&content, "\n")
	}
	c.Content = content.String()
	return c
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
	if len(s.Symbols) > 0 {
		fmt.Fprintf(&content, "**Package:** `%s`\n\n", s.Symbols[0].Package)
	}
	if len(s.ParseWarnings) > 0 {
		fmt.Fprintf(&content, "### Parse Warnings\n\n")
		for _, warning := range s.ParseWarnings {
			fmt.Fprintf(&content, "- %s\n", warning)
		}
		fmt.Fprintf(&content, "\n")
	}
	if len(s.Imports) > 0 {
		fmt.Fprintf(&content, "### Imports\n\n")
		for _, imp := range s.Imports {
			fmt.Fprintf(&content, "- `%s`\n", imp)
		}
		fmt.Fprintf(&content, "\n")
	}
	if len(s.Functions) > 0 {
		fmt.Fprintf(&content, "### Functions\n\n")
		for _, fn := range s.Functions {
			fmt.Fprintf(&content, "- `%s`\n", fn)
		}
		fmt.Fprintf(&content, "\n")
	}
	if len(s.Symbols) > 0 {
		fmt.Fprintf(&content, "### Symbols\n\n")
		for _, symbol := range s.Symbols {
			location := fmt.Sprintf("%s:%d-%d", s.RelativePath, symbol.StartLine, symbol.EndLine)
			exported := "unexported"
			if symbol.Exported {
				exported = "exported"
			}
			if symbol.Receiver != "" {
				fmt.Fprintf(&content, "- `%s` `%s.%s` (%s) at `%s`\n", symbol.Kind, symbol.Receiver, symbol.Name, exported, location)
				continue
			}
			fmt.Fprintf(&content, "- `%s` `%s` (%s) at `%s`\n", symbol.Kind, symbol.Name, exported, location)
		}
		fmt.Fprintf(&content, "\n")
	}

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
	metadataByFile, err := BatchGitMetadata(repoRoot, changedFiles)
	if err != nil {
		return nil, nil, err
	}

	var updatedPaths []string
	var filesToAnalyze []string

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
		filesToAnalyze = append(filesToAnalyze, file)
	}

	summaries, err := AnalyzeFilesWithMetadata(repoRoot, filesToAnalyze, metadataByFile, cfg.Workers)
	if err != nil {
		return nil, nil, err
	}
	for _, summary := range summaries {
		concept := conceptFromSummary(summary, cfg, bundle)
		bundle.Concepts = append(bundle.Concepts, concept)
		updatedPaths = append(updatedPaths, summary.RelativePath)
	}

	return bundle, updatedPaths, nil
}

// ApplyIncrementalUpdate merges an incremental bundle into the on-disk knowledge base.
func ApplyIncrementalUpdate(cfg *Config, incremental *okf.KnowledgeBundle) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if incremental == nil || len(incremental.Concepts) == 0 {
		return nil
	}

	repoRoot, err := GetRepoRoot(cfg.RepoPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	cfg.RepoPath = repoRoot

	knowledgeDir := filepath.Join(repoRoot, cfg.KnowledgeDir)
	existing, err := okf.LoadBundle(knowledgeDir, okf.DefaultLoadOptions())
	if err != nil {
		existing = &okf.KnowledgeBundle{Name: filepath.Base(repoRoot), RootPath: knowledgeDir}
	}

	byResource := make(map[string]*okf.Concept)
	byFilePath := make(map[string]*okf.Concept)
	var unkeyed []*okf.Concept
	for _, concept := range existing.Concepts {
		if isRelationshipGraphConcept(concept) {
			continue
		}
		if concept.Resource != "" {
			byResource[concept.Resource] = concept
			continue
		}
		if concept.FilePath != "" {
			byFilePath[concept.FilePath] = concept
			continue
		}
		unkeyed = append(unkeyed, concept)
	}

	for _, concept := range incremental.Concepts {
		if isRelationshipGraphConcept(concept) {
			continue
		}
		if concept.Type == "deleted" {
			delete(byResource, concept.Resource)
			removeKnowledgeFile(knowledgeDir, concept.Resource)
			continue
		}
		if concept.Resource != "" {
			byResource[concept.Resource] = concept
			continue
		}
		if concept.FilePath != "" {
			byFilePath[concept.FilePath] = concept
			continue
		}
		unkeyed = append(unkeyed, concept)
	}

	var resources []string
	for resource := range byResource {
		resources = append(resources, resource)
	}
	sort.Strings(resources)
	var filePaths []string
	for filePath := range byFilePath {
		filePaths = append(filePaths, filePath)
	}
	sort.Strings(filePaths)

	merged := &okf.KnowledgeBundle{Name: filepath.Base(repoRoot), RootPath: knowledgeDir}
	merged.Concepts = append(merged.Concepts, unkeyed...)
	for _, filePath := range filePaths {
		merged.Concepts = append(merged.Concepts, byFilePath[filePath])
	}
	for _, resource := range resources {
		merged.Concepts = append(merged.Concepts, byResource[resource])
	}
	merged.Concepts = append(merged.Concepts, createRelationshipGraphConcept(BuildRelationshipGraphFromConcepts(merged.Concepts)))

	_, err = SaveKnowledgeBase(merged, cfg)
	return err
}

func isRelationshipGraphConcept(concept *okf.Concept) bool {
	return concept != nil && (concept.Resource == relationshipGraphResource || concept.FilePath == "project/relationships.md")
}

// BuildRelationshipGraphFromConcepts rebuilds relationships from generated component concepts.
func BuildRelationshipGraphFromConcepts(concepts []*okf.Concept) RelationshipGraph {
	var summaries []*FileSummary
	for _, concept := range concepts {
		if concept == nil || concept.Resource == "" || isRelationshipGraphConcept(concept) || concept.Type == "deleted" {
			continue
		}
		summary := summaryFromConcept(concept)
		if summary == nil {
			continue
		}
		summaries = append(summaries, summary)
	}
	return BuildRelationshipGraphFromSummaries(summaries)
}

func summaryFromConcept(concept *okf.Concept) *FileSummary {
	summary := &FileSummary{RelativePath: concept.Resource, Type: concept.Type}
	pkg := ""
	inImports := false
	inSymbols := false
	for _, line := range strings.Split(concept.Content, "\n") {
		line = strings.TrimSpace(line)
		if matches := packageLinePattern.FindStringSubmatch(line); len(matches) == 2 {
			pkg = matches[1]
			continue
		}
		switch line {
		case "### Imports":
			inImports = true
			inSymbols = false
			continue
		case "### Symbols":
			inImports = false
			inSymbols = true
			continue
		}
		if strings.HasPrefix(line, "### ") {
			inImports = false
			inSymbols = false
			continue
		}
		if inImports {
			if matches := importLinePattern.FindStringSubmatch(line); len(matches) == 2 {
				summary.Imports = append(summary.Imports, matches[1])
			}
			continue
		}
		if inSymbols {
			if symbol, ok := parseGeneratedSymbol(line, pkg, concept.Resource); ok {
				summary.Symbols = append(summary.Symbols, symbol)
			}
		}
	}
	if len(summary.Imports) == 0 && len(summary.Symbols) == 0 {
		return nil
	}
	return summary
}

func parseGeneratedSymbol(line, pkg, filePath string) (Symbol, bool) {
	matches := generatedSymbolPattern.FindStringSubmatch(line)
	if len(matches) != 5 {
		return Symbol{}, false
	}
	name := matches[2]
	receiver := ""
	if matches[1] == "method" {
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			receiver = parts[0]
			name = parts[1]
		}
	}
	return Symbol{Kind: matches[1], Name: name, Receiver: receiver, Package: pkg, FilePath: filePath, Exported: matches[3] == "exported"}, true
}

func removeKnowledgeFile(knowledgeDir, resource string) {
	if resource == "" {
		return
	}
	path := filepath.Join(knowledgeDir, resource)
	if !strings.HasSuffix(path, ".md") {
		path += ".md"
	}
	_ = os.Remove(path)
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
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return 0, err
	}

	saved := 0
	var failures []string
	for _, concept := range bundle.Concepts {
		relPath := concept.FilePath
		if relPath == "" {
			relPath = filepath.Join(sanitizeFilename(concept.Type)+"s", sanitizeFilename(concept.Title)+".md")
		}
		if !strings.HasSuffix(relPath, ".md") {
			relPath = relPath + ".md"
		}

		fullPath := filepath.Join(outputDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			failures = append(failures, fmt.Sprintf("mkdir %s: %v", filepath.Dir(fullPath), err))
			continue
		}

		content, err := parser.SerializeConcept(&parser.Concept{
			Type:        concept.Type,
			Title:       concept.Title,
			Description: concept.Description,
			Resource:    concept.Resource,
			Tags:        concept.Tags,
			Timestamp:   concept.Timestamp,
			Content:     concept.Content,
		}, true)
		if err != nil {
			failures = append(failures, fmt.Sprintf("serialize %s: %v", relPath, err))
			continue
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			failures = append(failures, fmt.Sprintf("write %s: %v", relPath, err))
			continue
		}
		saved++
	}
	if len(failures) > 0 {
		return saved, fmt.Errorf("failed to save %d concepts: %s", len(failures), strings.Join(failures, "; "))
	}
	if commit, err := GetCurrentCommit(cfg.RepoPath); err == nil && commit != "" {
		if err := WriteState(cfg, &State{LastIndexedCommit: commit}); err != nil {
			return saved, err
		}
	}

	return saved, nil
}
