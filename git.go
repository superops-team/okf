package okf

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// GitConfig Git 操作配置
type GitConfig struct {
	// RepoPath 仓库路径，默认为当前目录
	RepoPath string

	// KnowledgeDir 知识库目录，默认为 .okf/knowledge
	KnowledgeDir string

	// IncludeFiles 包含的文件模式
	IncludeFiles []string

	// ExcludeDirs 排除的目录
	ExcludeDirs []string

	// Author 作者信息，从 git 自动获取
	Author string
	Email  string

	// MaxFileSizeKB 最大文件大小（KB），超过跳过
	MaxFileSizeKB int64
}

// DefaultGitConfig 默认 git 配置
func DefaultGitConfig() *GitConfig {
	wd, _ := os.Getwd()
	return &GitConfig{
		RepoPath:      wd,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go", "*.py", "*.js", "*.ts", "*.rs", "*.java", "*.c", "*.cpp", "*.h", "*.tsx", "*.jsx", "*.rb", "*.sh", "*.yml", "*.yaml", "*.json", "*.toml", "*.md"},
		ExcludeDirs:   []string{".git", "node_modules", "vendor", "dist", "build", "target", ".okf", ".venv", "__pycache__", ".idea", ".vscode", ".next"},
		MaxFileSizeKB: 100, // 100KB 限制，太大的文件跳过
	}
}

// GitCommit Git 提交信息
type GitCommit struct {
	Hash        string
	ShortHash   string
	Author      string
	Email       string
	Date        time.Time
	Subject     string
	Body        string
	Files       []string
	AddedFiles  []string
	ModifiedFiles []string
	DeletedFiles []string
}

// -----------------------------------------------------------------------------
// Git 命令执行
// -----------------------------------------------------------------------------

// IsGitRepo 检查是否为 git 仓库
func IsGitRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path
	return cmd.Run() == nil
}

// gitRun 执行 git 命令并返回输出
func gitRun(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// gitMustRun 执行 git 命令，失败返回错误
func gitMustRun(dir string, args ...string) (string, error) {
	out, err := gitRun(dir, args...)
	if err != nil {
		return "", fmt.Errorf("git %v failed: %w (output: %s)", args, err, out)
	}
	return out, nil
}

// GetRepoRoot 获取 git 仓库根目录
func GetRepoRoot(path string) (string, error) {
	return gitMustRun(path, "rev-parse", "--show-toplevel")
}

// GetCurrentBranch 获取当前分支
func GetCurrentBranch(path string) (string, error) {
	return gitMustRun(path, "rev-parse", "--abbrev-ref", "HEAD")
}

// GetCurrentCommit 获取当前 commit hash
func GetCurrentCommit(path string) (string, error) {
	return gitMustRun(path, "rev-parse", "HEAD")
}

// GetGitConfig 获取 git 配置
func GetGitConfig(path string, key string) (string, error) {
	out, err := gitRun(path, "config", "--get", key)
	if err != nil {
		return "", nil
	}
	return out, nil
}

// ListTrackedFiles 列出已追踪的文件
func ListTrackedFiles(path string) ([]string, error) {
	out, err := gitMustRun(path, "ls-files")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// ListModifiedFiles 列出已修改但未提交的文件
func ListModifiedFiles(path string) ([]string, error) {
	out, err := gitMustRun(path, "diff", "--name-only")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// ListStagedFiles 列出已暂存的文件
func ListStagedFiles(path string) ([]string, error) {
	out, err := gitMustRun(path, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// -----------------------------------------------------------------------------
// Commit 信息解析
// -----------------------------------------------------------------------------

// GetLastCommits 获取最近 N 个 commit
func GetLastCommits(path string, count int) ([]*GitCommit, error) {
	// 使用自定义格式输出
	format := "%H\x1e%h\x1e%an\x1e%ae\x1e%aI\x1e%s\x1e%b\x1f"
	out, err := gitMustRun(path, "log", fmt.Sprintf("-%d", count), fmt.Sprintf("--format=%s", format))
	if err != nil {
		return nil, err
	}

	commits := []*GitCommit{}
	entries := strings.Split(out, "\x1f")

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, "\x1e")
		if len(parts) < 6 {
			continue
		}

		date, err := time.Parse(time.RFC3339, parts[4])
		if err != nil {
			date = time.Now()
		}

		commit := &GitCommit{
			Hash:      parts[0],
			ShortHash: parts[1],
			Author:    parts[2],
			Email:     parts[3],
			Date:      date,
			Subject:   parts[5],
		}
		if len(parts) > 6 {
			commit.Body = strings.TrimSpace(parts[6])
		}

		// 获取此 commit 修改的文件
		filesOut, _ := gitRun(path, "diff-tree", "--no-commit-id", "--name-only", "-r", commit.Hash)
		if filesOut != "" {
			commit.Files = strings.Split(filesOut, "\n")
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

// GetCommit 获取单个 commit 信息
func GetCommit(path string, hash string) (*GitCommit, error) {
	format := "%H\x1e%h\x1e%an\x1e%ae\x1e%aI\x1e%s\x1e%b"
	out, err := gitMustRun(path, "show", "--format="+format, "--quiet", hash)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(out, "\x1e")
	if len(parts) < 6 {
		return nil, fmt.Errorf("unexpected commit format")
	}

	date, _ := time.Parse(time.RFC3339, parts[4])

	commit := &GitCommit{
		Hash:      parts[0],
		ShortHash: parts[1],
		Author:    parts[2],
		Email:     parts[3],
		Date:      date,
		Subject:   parts[5],
	}
	if len(parts) > 6 {
		commit.Body = strings.TrimSpace(parts[6])
	}

	// 获取修改的文件
	filesOut, _ := gitRun(path, "diff-tree", "--no-commit-id", "--name-only", "-r", "--diff-filter=M", commit.Hash)
	if filesOut != "" {
		commit.ModifiedFiles = strings.Split(filesOut, "\n")
	}

	filesOut, _ = gitRun(path, "diff-tree", "--no-commit-id", "--name-only", "-r", "--diff-filter=A", commit.Hash)
	if filesOut != "" {
		commit.AddedFiles = strings.Split(filesOut, "\n")
	}

	filesOut, _ = gitRun(path, "diff-tree", "--no-commit-id", "--name-only", "-r", "--diff-filter=D", commit.Hash)
	if filesOut != "" {
		commit.DeletedFiles = strings.Split(filesOut, "\n")
	}

	return commit, nil
}

// -----------------------------------------------------------------------------
// 文件分析
// -----------------------------------------------------------------------------

// FileSummary 文件摘要
type FileSummary struct {
	Path         string
	RelativePath string
	Size         int64
	LineCount    int
	LastModified time.Time
	LastCommit   string
	LastAuthor   string
	FirstAuthor  string
	CommitCount  int
	Type         string
	Imports      []string
	Functions    []string
	References   []string
}

// AnalyzeFile 分析单个文件
func AnalyzeFile(repoPath string, filePath string) (*FileSummary, error) {
	fullPath := filepath.Join(repoPath, filePath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	summary := &FileSummary{
		Path:         fullPath,
		RelativePath: filePath,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		Type:         detectFileType(filePath),
	}

	// 行数
	f, err := os.Open(fullPath)
	if err == nil {
		scanner := bufio.NewScanner(f)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 1024*1024)
		count := 0
		for scanner.Scan() {
			count++
		}
		summary.LineCount = count
		f.Close()
	}

	// Git 历史信息
	if lastAuthor, err := gitRun(repoPath, "log", "-1", "--format=%an", filePath); err == nil && lastAuthor != "" {
		summary.LastAuthor = lastAuthor
	}

	if lastCommit, err := gitRun(repoPath, "log", "-1", "--format=%h", filePath); err == nil && lastCommit != "" {
		summary.LastCommit = lastCommit
	}

	if dateStr, err := gitRun(repoPath, "log", "-1", "--format=%aI", filePath); err == nil && dateStr != "" {
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			summary.LastModified = t
		}
	}

	if countStr, err := gitRun(repoPath, "rev-list", "--count", "HEAD", "--", filePath); err == nil {
		fmt.Sscanf(countStr, "%d", &summary.CommitCount)
	}

	if firstAuthor, err := gitRun(repoPath, "log", "--reverse", "--format=%an", filePath); err == nil {
		lines := strings.Split(firstAuthor, "\n")
		if len(lines) > 0 {
			summary.FirstAuthor = lines[0]
		}
	}

	// 基本代码分析
	content, _ := os.ReadFile(fullPath)
	summary.Imports = extractImports(string(content), summary.Type)
	summary.Functions = extractFunctions(string(content), summary.Type)

	return summary, nil
}

// detectFileType 检测文件类型
func detectFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".sh", ".bash":
		return "shell"
	case ".md", ".markdown":
		return "markdown"
	case ".yml", ".yaml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".ini", ".cfg":
		return "ini"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

// extractImports 从代码中提取 import 语句
func extractImports(content string, fileType string) []string {
	seen := make(map[string]bool)
	var imports []string

	lines := strings.Split(content, "\n")
	patterns := map[string][]string{
		"go":         {`^\s*import\s+"([^"]+)"`, `^\s*\"([^"]+)\"`},
		"python":     {`^\s*import\s+([\w.]+)`, `^\s*from\s+([\w.]+)\s+import`},
		"javascript": {`^\s*(?:import|from)\s+['"]([^'"]+)['"]`, `^\s*require\(['"]([^'"]+)['"]\)`},
		"typescript": {`^\s*(?:import|from)\s+['"]([^'"]+)['"]`, `^\s*require\(['"]([^'"]+)['"]\)`},
		"java":       {`^\s*import\s+(?:static\s+)?([\w.]+)`},
		"rust":       {`^\s*use\s+(?:[\w:]+::)?([\w:]+)`},
		"ruby":       {`^\s*require\s+['"]([^'"]+)['"]`, `^\s*require_relative\s+['"]([^'"]+)['"]`},
	}

	if patList, ok := patterns[fileType]; ok {
		for _, line := range lines {
			for _, pattern := range patList {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				if matches := re.FindStringSubmatch(line); len(matches) > 1 {
					imp := strings.TrimSpace(matches[1])
					if !seen[imp] && imp != "" {
						seen[imp] = true
						imports = append(imports, imp)
					}
				}
			}
		}
	}

	return imports
}

// extractFunctions 提取函数定义
func extractFunctions(content string, fileType string) []string {
	seen := make(map[string]bool)
	var fns []string

	lines := strings.Split(content, "\n")
	patterns := map[string][]string{
		"go":         {`^\s*func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`},
		"python":     {`^\s*def\s+(\w+)\s*\(`, `^\s*class\s+(\w+)\s*[:\(]`},
		"javascript": {`^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`, `^\s*const\s+(\w+)\s*=\s*(?:async\s+)?\(`},
		"typescript": {`^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`, `^\s*(?:export\s+)?(?:class|interface|type)\s+(\w+)`},
		"java":       {`^\s*(?:public|private|protected)?\s*(?:static\s+)?(?:[\w<>]+\s+)?(\w+)\s*\(`},
		"rust":       {`^\s*fn\s+(\w+)\s*\(`, `^\s*(?:pub\s+)?struct\s+(\w+)`},
		"c":          {`^\s*(?:[\w*\s]+)\s+(\w+)\s*\([^;]*$`},
		"cpp":        {`^\s*(?:[\w*\s]+)\s+(\w+)\s*\([^;]*$`, `^\s*(?:class|struct)\s+(\w+)`},
		"ruby":       {`^\s*def\s+(\w+)`, `^\s*class\s+(\w+)`},
	}

	if patList, ok := patterns[fileType]; ok {
		for _, line := range lines {
			for _, pattern := range patList {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				if matches := re.FindStringSubmatch(line); len(matches) > 1 {
					fnName := strings.TrimSpace(matches[1])
					if !seen[fnName] && fnName != "" && len(fnName) < 80 {
						seen[fnName] = true
						fns = append(fns, fnName)
					}
				}
			}
		}
	}

	return fns
}

// shouldIncludeFile 判断文件是否应该被包含
func shouldIncludeFile(filePath string, config *GitConfig) bool {
	// 检查排除目录
	for _, excludeDir := range config.ExcludeDirs {
		if strings.HasPrefix(filePath, excludeDir+"/") || filePath == excludeDir {
			return false
		}
	}

	// 检查文件大小限制
	fullPath := filepath.Join(config.RepoPath, filePath)
	if info, err := os.Stat(fullPath); err == nil {
		if info.Size() > config.MaxFileSizeKB*1024 {
			return false
		}
	}

	// 检查包含模式（只包含代码/配置文件）
	lower := strings.ToLower(filePath)
	if len(config.IncludeFiles) > 0 {
		matched := false
		for _, pattern := range config.IncludeFiles {
			if ok, _ := filepath.Match(strings.ToLower(pattern), filepath.Base(lower)); ok {
				matched = true
				break
			}
		}
		if !matched {
			// 允许某些重要配置文件
			importantFiles := []string{"README.md", "LICENSE", "Makefile", "Dockerfile", ".gitignore", "go.mod", "package.json", "requirements.txt", "pyproject.toml", "Cargo.toml", "pom.xml"}
			for _, imp := range importantFiles {
				if strings.HasSuffix(lower, imp) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}

	return true
}

// -----------------------------------------------------------------------------
// 知识库生成
// -----------------------------------------------------------------------------

// GenerateBundle 从 git 仓库生成完整知识库
func GenerateBundle(config *GitConfig, force bool) (*KnowledgeBundle, error) {
	if config == nil {
		config = DefaultGitConfig()
	}

	repoRoot, err := GetRepoRoot(config.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	config.RepoPath = repoRoot

	// 获取作者信息
	if config.Author == "" {
		config.Author, _ = GetGitConfig(repoRoot, "user.name")
	}
	if config.Email == "" {
		config.Email, _ = GetGitConfig(repoRoot, "user.email")
	}

	bundle := &KnowledgeBundle{
		Name:     filepath.Base(repoRoot),
		RootPath: filepath.Join(repoRoot, config.KnowledgeDir),
	}

	// 获取所有追踪文件
	files, err := ListTrackedFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Found %d tracked files in repository\n", len(files))

	// 过滤文件
	var relevantFiles []string
	for _, f := range files {
		if shouldIncludeFile(f, config) {
			relevantFiles = append(relevantFiles, f)
		}
	}

	fmt.Printf("Analyzing %d relevant files...\n", len(relevantFiles))

	// 分析文件并生成 concept
	generated := 0
	summaryByDir := make(map[string]*FileSummary)
	typeCounts := make(map[string]int)
	authorCounts := make(map[string]int)

	for _, filePath := range relevantFiles {
		summary, err := AnalyzeFile(repoRoot, filePath)
		if err != nil {
			continue
		}

		concept := conceptFromFileSummary(summary, config, bundle)
		bundle.Concepts = append(bundle.Concepts, concept)

		// 统计信息
		if _, ok := summaryByDir[filepath.Dir(filePath)]; !ok {
			summaryByDir[filepath.Dir(filePath)] = summary
		}
		typeCounts[summary.Type]++
		if summary.LastAuthor != "" {
			authorCounts[summary.LastAuthor]++
		}

		generated++
		if generated%50 == 0 {
			fmt.Printf("  Processed %d/%d files...\n", generated, len(relevantFiles))
		}
	}

	// 添加项目概况 concept
	projectConcept := createProjectConcept(config, repoRoot, files, typeCounts, authorCounts)
	bundle.Concepts = append([]*Concept{projectConcept}, bundle.Concepts...)

	// 添加目录结构 concept
	dirConcept := createDirectoryConcept(config, repoRoot, relevantFiles)
	bundle.Concepts = append(bundle.Concepts, dirConcept)

	// 添加贡献者 concept
	if len(authorCounts) > 0 {
		contributorsConcept := createContributorsConcept(config, authorCounts)
		bundle.Concepts = append(bundle.Concepts, contributorsConcept)
	}

	fmt.Printf("Generated %d concepts from %d files\n", len(bundle.Concepts), generated)

	return bundle, nil
}

// conceptFromFileSummary 从文件摘要生成 concept
func conceptFromFileSummary(s *FileSummary, config *GitConfig, bundle *KnowledgeBundle) *Concept {
	c := NewConcept("component", filepath.Base(s.RelativePath))
	c.Description = generateFileDescription(s)
	c.Resource = s.RelativePath
	c.Timestamp = s.LastModified.Format(time.RFC3339)
	c.FilePath = s.RelativePath

	// 标签
	tags := []string{s.Type}
	if s.LastAuthor != "" {
		tags = append(tags, sanitizeTag(s.LastAuthor))
	}
	if s.CommitCount > 10 {
		tags = append(tags, "frequently-modified")
	}
	if s.LineCount > 500 {
		tags = append(tags, "large-file")
	}
	c.Tags = tags

	// 内容
	c.Content = generateFileContent(s)

	return c
}

// generateFileDescription 生成文件描述
func generateFileDescription(s *FileSummary) string {
	desc := fmt.Sprintf("%s file: %s (%d lines)",
		strings.ToUpper(s.Type),
		s.RelativePath,
		s.LineCount)

	if s.LastAuthor != "" {
		desc += fmt.Sprintf(" - last modified by %s", s.LastAuthor)
	}

	return desc
}

// generateFileContent 生成详细的文件内容
func generateFileContent(s *FileSummary) string {
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
	if s.FirstAuthor != "" {
		fmt.Fprintf(&content, "**Original Author:** %s\n\n", s.FirstAuthor)
	}
	fmt.Fprintf(&content, "**Commit Count:** %d\n\n", s.CommitCount)
	fmt.Fprintf(&content, "**Last Modified:** %s\n\n", s.LastModified.Format("2006-01-02 15:04:05 MST"))

	if len(s.Imports) > 0 {
		fmt.Fprintf(&content, "### Imports / Dependencies\n\n")
		for _, imp := range s.Imports {
			fmt.Fprintf(&content, "- `%s`\n", imp)
		}
		fmt.Fprintf(&content, "\n")
	}

	if len(s.Functions) > 0 {
		fmt.Fprintf(&content, "### Functions / Definitions\n\n")
		maxShow := min(len(s.Functions), 20)
		for _, fn := range s.Functions[:maxShow] {
			fmt.Fprintf(&content, "- `%s`\n", fn)
		}
		if len(s.Functions) > maxShow {
			fmt.Fprintf(&content, "- ... and %d more\n", len(s.Functions)-maxShow)
		}
		fmt.Fprintf(&content, "\n")
	}

	// 文件路径相关信息
	fmt.Fprintf(&content, "### Location\n\n")
	fmt.Fprintf(&content, "```\n%s\n```\n\n", s.RelativePath)

	return content.String()
}

// min helper
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// createProjectConcept 生成项目概况
func createProjectConcept(config *GitConfig, repoRoot string, allFiles []string, typeCounts map[string]int, authorCounts map[string]int) *Concept {
	c := NewConcept("project", "Project Overview")
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

	c.Description = fmt.Sprintf("Project %s on branch %s with %d tracked files",
		filepath.Base(repoRoot), branch, len(allFiles))

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

	fmt.Fprintf(&content, "### Generated At\n\n")
	fmt.Fprintf(&content, "%s\n\n", time.Now().Format("2006-01-02 15:04:05 MST"))

	c.Content = content.String()
	c.Resource = repoRoot
	if config.Author != "" {
		c.Tags = []string{sanitizeTag(config.Author), "project"}
	} else {
		c.Tags = []string{"project"}
	}

	return c
}

// createDirectoryConcept 生成目录结构信息
func createDirectoryConcept(config *GitConfig, repoRoot string, relevantFiles []string) *Concept {
	c := NewConcept("system", "Project Structure")
	c.FilePath = "project/structure.md"

	// 统计目录结构
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
	fmt.Fprintf(&content, "```\n")
	fmt.Fprintf(&content, "%s/\n", filepath.Base(repoRoot))

	lastDepth := 0
	_ = lastDepth
	for _, dir := range dirs {
		if dir == "." || dir == "" {
			continue
		}
		depth := strings.Count(dir, "/")
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(&content, "%s%s/ (%d files)\n", indent, filepath.Base(dir), len(dirTree[dir]))
	}
	fmt.Fprintf(&content, "```\n\n")

	fmt.Fprintf(&content, "### Key Directories\n\n")
	for _, dir := range dirs {
		if dir == "." || dir == "" {
			continue
		}
		depth := strings.Count(dir, "/")
		if depth <= 1 { // 只列出一级子目录
			files := dirTree[dir]
			sort.Strings(files)
			fmt.Fprintf(&content, "#### `%s/`\n\n", dir)
			fmt.Fprintf(&content, "- Files: %d\n", len(files))
			maxFiles := min(len(files), 10)
			for i := 0; i < maxFiles; i++ {
				fmt.Fprintf(&content, "  - `%s`\n", files[i])
			}
			if len(files) > maxFiles {
				fmt.Fprintf(&content, "  - ... (%d more)\n", len(files)-maxFiles)
			}
			fmt.Fprintf(&content, "\n")
		}
	}

	c.Description = fmt.Sprintf("Directory structure and organization for %s", filepath.Base(repoRoot))
	c.Content = content.String()
	c.Tags = []string{"structure", "project"}

	return c
}

// createContributorsConcept 生成贡献者信息
func createContributorsConcept(config *GitConfig, authorCounts map[string]int) *Concept {
	c := NewConcept("people", "Contributors")
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
		fmt.Fprintf(&content, "%d. **%s** - %d files (%.1f%%)\n",
			i+1, author.Key, author.Value, percent)
	}

	c.Description = fmt.Sprintf("Contributors and their file involvement in the project")
	c.Content = content.String()
	c.Tags = []string{"contributors", "people"}

	return c
}

// sanitizeTag 清理标签字符串
func sanitizeTag(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = regexp.MustCompile(`[^a-z0-9_\-]`).ReplaceAllString(s, "_")
	return s
}

// -----------------------------------------------------------------------------
// 更新知识库
// -----------------------------------------------------------------------------

// UpdateBundle 基于新的代码变更更新知识库
func UpdateBundle(config *GitConfig, changedFiles []string) (*KnowledgeBundle, []string, error) {
	if config == nil {
		config = DefaultGitConfig()
	}

	repoRoot, err := GetRepoRoot(config.RepoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("not a git repository: %w", err)
	}
	config.RepoPath = repoRoot

	bundle := &KnowledgeBundle{
		Name:     filepath.Base(repoRoot) + "_incremental",
		RootPath: filepath.Join(repoRoot, config.KnowledgeDir),
	}

	var updatedPaths []string

	// 只处理变更的文件
	for _, file := range changedFiles {
		if !shouldIncludeFile(file, config) {
			continue
		}

		// 检查文件是否仍存在
		fullPath := filepath.Join(repoRoot, file)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// 文件已删除，记录删除信息
			concept := NewConcept("deleted", filepath.Base(file))
			concept.Description = fmt.Sprintf("File %s was deleted from the repository", file)
			concept.Resource = file
			concept.Timestamp = time.Now().UTC().Format(time.RFC3339)
			concept.FilePath = file
			concept.Content = fmt.Sprintf("## Deleted File: `%s`\n\nThis file was removed from the repository.\n\n**Removal Time:** %s\n", file, time.Now().Format("2006-01-02 15:04:05 MST"))
			bundle.Concepts = append(bundle.Concepts, concept)
			updatedPaths = append(updatedPaths, file+" (deleted)")
			continue
		}

		// 分析并生成 concept
		summary, err := AnalyzeFile(repoRoot, file)
		if err != nil {
			continue
		}

		concept := conceptFromFileSummary(summary, config, bundle)
		bundle.Concepts = append(bundle.Concepts, concept)
		updatedPaths = append(updatedPaths, file)
	}

	return bundle, updatedPaths, nil
}

// -----------------------------------------------------------------------------
// 基于 commit 历史的增量更新
// -----------------------------------------------------------------------------

// UpdateFromLastCommit 基于最后一个 commit 的变更更新知识库
func UpdateFromLastCommit(config *GitConfig) (*KnowledgeBundle, []string, error) {
	if config == nil {
		config = DefaultGitConfig()
	}

	repoRoot, err := GetRepoRoot(config.RepoPath)
	if err != nil {
		return nil, nil, err
	}

	// 获取最后一个 commit
	commits, err := GetLastCommits(repoRoot, 1)
	if err != nil || len(commits) == 0 {
		return nil, nil, fmt.Errorf("no commits found: %w", err)
	}

	commit := commits[0]

	// 获取此 commit 修改的文件
	var changedFiles []string
	changedFiles = append(changedFiles, commit.AddedFiles...)
	changedFiles = append(changedFiles, commit.ModifiedFiles...)

	// 也处理删除的文件
	for _, del := range commit.DeletedFiles {
		changedFiles = append(changedFiles, del)
	}

	if len(changedFiles) == 0 {
		return nil, nil, nil
	}

	bundle, updated, err := UpdateBundle(config, changedFiles)
	if err != nil {
		return nil, nil, err
	}

	// 添加 commit 信息 concept
	commitConcept := NewConcept("changelog", fmt.Sprintf("Commit: %s", commit.ShortHash))
	commitConcept.FilePath = "changelog/" + commit.ShortHash + ".md"
	commitConcept.Resource = commit.Hash
	commitConcept.Timestamp = commit.Date.Format(time.RFC3339)

	var content strings.Builder
	fmt.Fprintf(&content, "## Commit `%s`\n\n", commit.ShortHash)
	fmt.Fprintf(&content, "**Subject:** %s\n\n", commit.Subject)
	fmt.Fprintf(&content, "**Author:** %s <%s>\n\n", commit.Author, commit.Email)
	fmt.Fprintf(&content, "**Date:** %s\n\n", commit.Date.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(&content, "**Full Hash:** `%s`\n\n", commit.Hash)

	if commit.Body != "" {
		fmt.Fprintf(&content, "### Body\n\n")
		fmt.Fprintf(&content, "%s\n\n", commit.Body)
	}

	fmt.Fprintf(&content, "### Files Changed\n\n")
	fmt.Fprintf(&content, "**Added (%d):**\n\n", len(commit.AddedFiles))
	for _, f := range commit.AddedFiles {
		fmt.Fprintf(&content, "- `%s`\n", f)
	}

	fmt.Fprintf(&content, "\n**Modified (%d):**\n\n", len(commit.ModifiedFiles))
	for _, f := range commit.ModifiedFiles {
		fmt.Fprintf(&content, "- `%s`\n", f)
	}

	if len(commit.DeletedFiles) > 0 {
		fmt.Fprintf(&content, "\n**Deleted (%d):**\n\n", len(commit.DeletedFiles))
		for _, f := range commit.DeletedFiles {
			fmt.Fprintf(&content, "- `%s`\n", f)
		}
	}

	commitConcept.Content = content.String()
	commitConcept.Description = fmt.Sprintf("Changes from commit %s: %s", commit.ShortHash, commit.Subject)
	commitConcept.Tags = []string{"changelog", sanitizeTag(commit.Author)}

	bundle.Concepts = append(bundle.Concepts, commitConcept)

	return bundle, updated, nil
}

// -----------------------------------------------------------------------------
// 保存知识库到磁盘
// -----------------------------------------------------------------------------

// SaveKnowledgeBase 将知识库保存到磁盘
func SaveKnowledgeBase(bundle *KnowledgeBundle, config *GitConfig) (int, error) {
	if config == nil {
		config = DefaultGitConfig()
	}

	outputDir := filepath.Join(config.RepoPath, config.KnowledgeDir)
	os.MkdirAll(outputDir, 0755)

	opts := &SaveOptions{PrettyPrint: true}
	saved := 0

	for _, concept := range bundle.Concepts {
		// 确定输出路径
		relPath := concept.FilePath
		if relPath == "" {
			relPath = filepath.Join(sanitizeFilename(concept.Type)+"s", sanitizeFilename(concept.Title)+".md")
		}
		// 确保路径合理
		if !strings.HasSuffix(relPath, ".md") {
			relPath = relPath + ".md"
		}

		fullPath := filepath.Join(outputDir, relPath)
		dir := filepath.Dir(fullPath)
		os.MkdirAll(dir, 0755)

		// 序列化并保存
		data, err := SerializeConcept(concept, opts.PrettyPrint)
		if err != nil {
			continue
		}

		if err := os.WriteFile(fullPath, []byte(data), 0644); err != nil {
			continue
		}
		saved++
	}

	// 生成 index 文件
	generateIndex(bundle, outputDir)

	// 生成 stats 文件
	generateStats(bundle, outputDir)

	return saved, nil
}

// generateIndex 生成索引文件
func generateIndex(bundle *KnowledgeBundle, outputDir string) {
	// 按类型分组
	byType := make(map[string][]*Concept)
	for _, c := range bundle.Concepts {
		byType[c.Type] = append(byType[c.Type], c)
	}

	var content strings.Builder
	fmt.Fprintf(&content, "# %s - Knowledge Base Index\n\n", bundle.Name)
	fmt.Fprintf(&content, "**Total Concepts:** %d\n\n", len(bundle.Concepts))
	fmt.Fprintf(&content, "**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST"))

	var types []string
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)

	for _, t := range types {
		concepts := byType[t]
		fmt.Fprintf(&content, "## %s (%d)\n\n", strings.ToTitle(t), len(concepts))

		for _, c := range concepts {
			link := c.FilePath
			if !strings.HasSuffix(link, ".md") {
				link = link + ".md"
			}
			fmt.Fprintf(&content, "- [`%s`](%s) - %s\n", c.Title, link, truncate(c.Description, 80))
		}
		fmt.Fprintf(&content, "\n")
	}

	indexPath := filepath.Join(outputDir, "README.md")
	os.WriteFile(indexPath, []byte(content.String()), 0644)
}

// generateStats 生成统计文件
func generateStats(bundle *KnowledgeBundle, outputDir string) {
	stats := bundle.Stats()

	var content strings.Builder
	fmt.Fprintf(&content, "# Knowledge Base Statistics\n\n")
	fmt.Fprintf(&content, "| Metric | Count |\n")
	fmt.Fprintf(&content, "|--------|-------|\n")
	fmt.Fprintf(&content, "| Total Concepts | %d |\n", stats.TotalConcepts)
	fmt.Fprintf(&content, "| Unique Types | %d |\n", stats.UniqueTypes)
	fmt.Fprintf(&content, "| Unique Tags | %d |\n\n", stats.UniqueTags)

	fmt.Fprintf(&content, "## Concepts by Type\n\n")
	var types []string
	for t := range stats.TypeCounts {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		fmt.Fprintf(&content, "- **%s:** %d\n", t, stats.TypeCounts[t])
	}

	fmt.Fprintf(&content, "\n## Top Tags\n\n")
	var tags []string
	for tag := range stats.TagCounts {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return stats.TagCounts[tags[i]] > stats.TagCounts[tags[j]]
	})
	maxTags := min(len(tags), 20)
	for i := 0; i < maxTags; i++ {
		fmt.Fprintf(&content, "- **%s:** %d\n", tags[i], stats.TagCounts[tags[i]])
	}

	statsPath := filepath.Join(outputDir, "STATS.md")
	os.WriteFile(statsPath, []byte(content.String()), 0644)
}

// truncate 截断字符串
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
