// Package git provides Git repository integration for OKF knowledge base generation.
package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Config contains Git integration configuration.
type Config struct {
	RepoPath      string
	KnowledgeDir  string
	IncludeFiles  []string
	ExcludeDirs   []string
	Author        string
	Email         string
	MaxFileSizeKB int64
}

// DefaultConfig returns the default Git configuration.
func DefaultConfig() *Config {
	wd, _ := os.Getwd()
	return &Config{
		RepoPath:      wd,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go", "*.py", "*.js", "*.ts", "*.rs", "*.java", "*.c", "*.cpp", "*.h", "*.tsx", "*.jsx", "*.rb", "*.sh", "*.yml", "*.yaml", "*.json", "*.toml", "*.md"},
		ExcludeDirs:   []string{".git", "node_modules", "vendor", "dist", "build", "target", ".okf", ".venv", "__pycache__", ".idea", ".vscode", ".next"},
		MaxFileSizeKB: 100,
	}
}

// Commit represents a Git commit.
type Commit struct {
	Hash         string
	ShortHash    string
	Author       string
	Email        string
	Date         time.Time
	Subject      string
	Body         string
	Files        []string
	AddedFiles   []string
	ModifiedFiles []string
	DeletedFiles []string
}

// IsRepo checks if path is a Git repository.
func IsRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path
	return cmd.Run() == nil
}

// GetRepoRoot returns the Git repository root.
func GetRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w (output: %s)", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// GetCurrentBranch returns the current branch name.
func GetCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// GetCurrentCommit returns the current commit hash.
func GetCurrentCommit(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// GetLastCommits returns the last n commits.
func GetLastCommits(path string, count int) ([]*Commit, error) {
	format := "%H\x1e%h\x1e%an\x1e%ae\x1e%aI\x1e%s\x1e%b\x1f"
	cmd := exec.Command("git", "log", fmt.Sprintf("-%d", count), fmt.Sprintf("--format=%s", format))
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var commits []*Commit
	entries := strings.Split(string(out), "\x1f")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "\x1e")
		if len(parts) < 6 {
			continue
		}
		date, _ := time.Parse(time.RFC3339, parts[4])
		commit := &Commit{
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
		commits = append(commits, commit)
	}
	return commits, nil
}

// ListTrackedFiles lists all tracked files.
func ListTrackedFiles(path string) ([]string, error) {
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(out)) == "" {
		return []string{}, nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}

// FileSummary contains file analysis data.
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
}

// AnalyzeFile analyzes a single file.
func AnalyzeFile(repoPath, filePath string) (*FileSummary, error) {
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

	if f, err := os.Open(fullPath); err == nil {
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

	if out, err := exec.Command("git", "log", "-1", "--format=%an", filePath).CombinedOutput(); err == nil {
		summary.LastAuthor = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "log", "-1", "--format=%h", filePath).CombinedOutput(); err == nil {
		summary.LastCommit = strings.TrimSpace(string(out))
	}

	return summary, nil
}

// ShouldInclude determines if a file should be included in knowledge base.
func ShouldInclude(filePath string, cfg *Config) bool {
	for _, excludeDir := range cfg.ExcludeDirs {
		if strings.HasPrefix(filePath, excludeDir+"/") || filePath == excludeDir {
			return false
		}
	}

	fullPath := filepath.Join(cfg.RepoPath, filePath)
	if info, err := os.Stat(fullPath); err == nil {
		if info.Size() > cfg.MaxFileSizeKB*1024 {
			return false
		}
	}

	return true
}

func detectFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go": return "go"
	case ".py": return "python"
	case ".js", ".jsx": return "javascript"
	case ".ts", ".tsx": return "typescript"
	case ".rs": return "rust"
	case ".java": return "java"
	case ".c", ".h": return "c"
	case ".cpp", ".cc", ".hpp": return "cpp"
	case ".rb": return "ruby"
	case ".sh", ".bash": return "shell"
	case ".md", ".markdown": return "markdown"
	case ".yml", ".yaml": return "yaml"
	case ".json": return "json"
	case ".toml": return "toml"
	default: return strings.TrimPrefix(ext, ".")
	}
}

// ExtractImports extracts import statements from code.
func ExtractImports(content, fileType string) []string {
	seen := make(map[string]bool)
	var imports []string

	lines := strings.Split(content, "\n")
	patterns := map[string][]string{
		"go":         {`^\s*import\s+"([^"]+)"`, `^\s*\"([^"]+)\"`},
		"python":     {`^\s*import\s+([\w.]+)`, `^\s*from\s+([\w.]+)\s+import`},
		"javascript": {`^\s*(?:import|from)\s+['"]([^'"]+)['"]`, `^\s*require\(['"]([^'"]+)['"]\)`},
		"typescript": {`^\s*(?:import|from)\s+['"]([^'"]+)['"]`, `^\s*require\(['"]([^'"]+)['"]\)`},
	}

	if patList, ok := patterns[fileType]; ok {
		for _, line := range lines {
			for _, pattern := range patList {
				if re, err := regexp.Compile(pattern); err == nil {
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
	}
	return imports
}

// ExtractFunctions extracts function definitions from code.
func ExtractFunctions(content, fileType string) []string {
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
				if re, err := regexp.Compile(pattern); err == nil {
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
	}
	return fns
}
