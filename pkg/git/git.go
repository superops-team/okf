// Package git provides Git repository integration for OKF knowledge base generation.
package git

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
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
	Workers       int
}

var importPatterns = map[string][]*regexp.Regexp{
	"go":         {regexp.MustCompile(`^\s*import\s+"([^"]+)"`), regexp.MustCompile(`^\s*"([^"]+)"`)},
	"python":     {regexp.MustCompile(`^\s*import\s+([\w.]+)`), regexp.MustCompile(`^\s*from\s+([\w.]+)\s+import`)},
	"javascript": {regexp.MustCompile(`^\s*(?:import|from)\s+['"]([^'"]+)['"]`), regexp.MustCompile(`^\s*require\(['"]([^'"]+)['"]\)`)},
	"typescript": {regexp.MustCompile(`^\s*(?:import|from)\s+['"]([^'"]+)['"]`), regexp.MustCompile(`^\s*require\(['"]([^'"]+)['"]\)`)},
}

var functionPatterns = map[string][]*regexp.Regexp{
	"go":         {regexp.MustCompile(`^\s*func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)},
	"python":     {regexp.MustCompile(`^\s*def\s+(\w+)\s*\(`), regexp.MustCompile(`^\s*class\s+(\w+)\s*[:\(]`)},
	"javascript": {regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`), regexp.MustCompile(`^\s*const\s+(\w+)\s*=\s*(?:async\s+)?\(`)},
	"typescript": {regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`), regexp.MustCompile(`^\s*(?:export\s+)?(?:class|interface|type)\s+(\w+)`)},
	"java":       {regexp.MustCompile(`^\s*(?:public|private|protected)?\s*(?:static\s+)?(?:[\w<>]+\s+)?(\w+)\s*\(`)},
	"rust":       {regexp.MustCompile(`^\s*fn\s+(\w+)\s*\(`), regexp.MustCompile(`^\s*(?:pub\s+)?struct\s+(\w+)`)},
	"c":          {regexp.MustCompile(`^\s*(?:[\w*\s]+)\s+(\w+)\s*\([^;]*$`)},
	"cpp":        {regexp.MustCompile(`^\s*(?:[\w*\s]+)\s+(\w+)\s*\([^;]*$`), regexp.MustCompile(`^\s*(?:class|struct)\s+(\w+)`)},
	"ruby":       {regexp.MustCompile(`^\s*def\s+(\w+)`), regexp.MustCompile(`^\s*class\s+(\w+)`)},
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
		Workers:       runtime.NumCPU(),
	}
}

// Commit represents a Git commit.
type Commit struct {
	Hash          string
	ShortHash     string
	Author        string
	Email         string
	Date          time.Time
	Subject       string
	Body          string
	Files         []string
	AddedFiles    []string
	ModifiedFiles []string
	DeletedFiles  []string
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
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
		populateCommitFiles(path, commit)
		commits = append(commits, commit)
	}
	return commits, nil
}

func populateCommitFiles(path string, commit *Commit) {
	cmd := exec.Command("git", "show", "--name-status", "-z", "--format=", commit.Hash)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	addFile := func(status byte, file string) {
		if file == "" {
			return
		}
		commit.Files = append(commit.Files, file)
		switch status {
		case 'A':
			commit.AddedFiles = append(commit.AddedFiles, file)
		case 'M', 'C':
			commit.ModifiedFiles = append(commit.ModifiedFiles, file)
		case 'D':
			commit.DeletedFiles = append(commit.DeletedFiles, file)
		}
	}

	entries := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	for i := 0; i < len(entries); {
		status := entries[i]
		i++
		if status == "" || i >= len(entries) {
			continue
		}
		switch status[0] {
		case 'R':
			oldPath := entries[i]
			i++
			if i >= len(entries) {
				addFile('D', oldPath)
				continue
			}
			newPath := entries[i]
			i++
			addFile('D', oldPath)
			addFile('M', newPath)
		case 'C':
			i++ // source path
			if i >= len(entries) {
				continue
			}
			newPath := entries[i]
			i++
			addFile('C', newPath)
		default:
			file := entries[i]
			i++
			addFile(status[0], file)
		}
	}
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
	Path          string
	RelativePath  string
	Size          int64
	LineCount     int
	LastModified  time.Time
	LastCommit    string
	LastAuthor    string
	FirstAuthor   string
	CommitCount   int
	Type          string
	Imports       []string
	Functions     []string
	Symbols       []Symbol
	ParseWarnings []string
}

// GitMetadata contains per-file Git history metadata used by file analysis.
type GitMetadata struct {
	LastCommit  string
	LastAuthor  string
	CommitCount int
}

// Symbol describes a top-level code symbol discovered during source analysis.
type Symbol struct {
	Kind      string
	Name      string
	Receiver  string
	Package   string
	FilePath  string
	Exported  bool
	StartLine int
	EndLine   int
}

// AnalyzeFile analyzes a single file.
func AnalyzeFile(repoPath, filePath string) (*FileSummary, error) {
	metadataByFile, err := BatchGitMetadata(repoPath, []string{filePath})
	if err != nil {
		return nil, err
	}
	metadata := metadataByFile[filePath]
	return AnalyzeFileWithMetadata(repoPath, filePath, metadata)
}

// AnalyzeFileWithMetadata analyzes a single file and uses pre-fetched Git metadata when provided.
func AnalyzeFileWithMetadata(repoPath, filePath string, metadata *GitMetadata) (*FileSummary, error) {
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

	if data, err := os.ReadFile(fullPath); err == nil {
		content := string(data)
		if summary.Type == "go" {
			summary.Imports, summary.Functions, summary.Symbols, summary.ParseWarnings = ExtractGoSymbolDetails(content)
			for i := range summary.Symbols {
				summary.Symbols[i].FilePath = filePath
			}
		} else {
			summary.Imports = ExtractImports(content, summary.Type)
			summary.Functions = ExtractFunctions(content, summary.Type)
		}
	}

	if metadata != nil {
		summary.LastAuthor = metadata.LastAuthor
		summary.LastCommit = metadata.LastCommit
		summary.CommitCount = metadata.CommitCount
	}

	return summary, nil
}

type fileAnalysisResult struct {
	path    string
	summary *FileSummary
	err     error
}

// AnalyzeFilesWithMetadata analyzes files with a bounded worker pool and returns summaries sorted by path.
func AnalyzeFilesWithMetadata(repoPath string, files []string, metadataByFile map[string]*GitMetadata, workers int) ([]*FileSummary, error) {
	if len(files) == 0 {
		return nil, nil
	}
	workers = boundedWorkerCount(workers, len(files))

	sem := make(chan struct{}, workers)
	results := make(chan fileAnalysisResult, len(files))
	var wg sync.WaitGroup

	for _, file := range files {
		file := file
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			summary, err := AnalyzeFileWithMetadata(repoPath, file, metadataByFile[file])
			results <- fileAnalysisResult{path: file, summary: summary, err: err}
		}()
	}

	wg.Wait()
	close(results)

	var summaries []*FileSummary
	var firstErr error
	for result := range results {
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		if result.summary != nil {
			summaries = append(summaries, result.summary)
		}
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].RelativePath < summaries[j].RelativePath
	})
	return summaries, firstErr
}

func boundedWorkerCount(workers, fileCount int) int {
	if workers <= 0 {
		workers = 1
	}
	if fileCount > 0 && workers > fileCount {
		return fileCount
	}
	return workers
}

// BatchGitMetadata fetches Git metadata for many files with one git log process.
func BatchGitMetadata(repoPath string, files []string) (map[string]*GitMetadata, error) {
	metadata := make(map[string]*GitMetadata, len(files))
	if len(files) == 0 {
		return metadata, nil
	}

	wanted := make(map[string]bool, len(files))
	for _, file := range files {
		if file == "" {
			continue
		}
		wanted[file] = true
		metadata[file] = &GitMetadata{}
	}
	if len(wanted) == 0 {
		return metadata, nil
	}

	args := []string{"log", "--format=%h%x1e%an", "--name-only", "--"}
	args = append(args, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log metadata failed: %w (output: %s)", err, string(out))
	}

	var commitHash, author string
	for _, rawLine := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.Contains(line, "\x1e") {
			parts := strings.SplitN(line, "\x1e", 2)
			commitHash = strings.TrimSpace(parts[0])
			author = ""
			if len(parts) > 1 {
				author = strings.TrimSpace(parts[1])
			}
			continue
		}
		if !wanted[line] {
			continue
		}
		entry := metadata[line]
		if entry == nil {
			entry = &GitMetadata{}
			metadata[line] = entry
		}
		if entry.CommitCount == 0 {
			entry.LastCommit = commitHash
			entry.LastAuthor = author
		}
		entry.CommitCount++
	}

	return metadata, nil
}

// ExtractGoSymbols extracts imports and top-level symbols from Go source using AST parsing.
func ExtractGoSymbols(content string) ([]string, []string) {
	imports, functions, _, _ := ExtractGoSymbolDetails(content)
	return imports, functions
}

// ExtractGoSymbolDetails extracts imports, display names, and structured top-level symbols from Go source.
func ExtractGoSymbolDetails(content string) ([]string, []string, []Symbol, []string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.SkipObjectResolution)
	if err != nil || file == nil {
		return ExtractImports(content, "go"), ExtractFunctions(content, "go"), nil, []string{fmt.Sprintf("failed to parse Go AST: %v", err)}
	}

	seenImports := make(map[string]bool)
	var imports []string
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, "\"")
		if path != "" && !seenImports[path] {
			seenImports[path] = true
			imports = append(imports, path)
		}
	}

	seenSymbols := make(map[string]bool)
	var functions []string
	var symbols []Symbol
	addFunction := func(symbol string) {
		if symbol == "" || seenSymbols[symbol] {
			return
		}
		seenSymbols[symbol] = true
		functions = append(functions, symbol)
	}
	addStructuredSymbol := func(symbol Symbol, displayName string) {
		addFunction(displayName)
		symbol.Package = file.Name.Name
		symbol.Exported = ast.IsExported(symbol.Name)
		symbols = append(symbols, symbol)
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			kind := "function"
			receiver := ""
			displayName := name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
				receiver = receiverBaseName(d.Recv.List[0].Type)
				displayName = receiverName(d.Recv.List[0].Type) + "." + name
			}
			addStructuredSymbol(Symbol{
				Kind:      kind,
				Name:      name,
				Receiver:  receiver,
				StartLine: fset.Position(d.Pos()).Line,
				EndLine:   fset.Position(d.End()).Line,
			}, displayName)
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				kind := "type"
				displayName := "type " + typeSpec.Name.Name
				switch typeSpec.Type.(type) {
				case *ast.StructType:
					kind = "struct"
					displayName = "struct " + typeSpec.Name.Name
				case *ast.InterfaceType:
					kind = "interface"
					displayName = "interface " + typeSpec.Name.Name
				}
				addStructuredSymbol(Symbol{
					Kind:      kind,
					Name:      typeSpec.Name.Name,
					StartLine: fset.Position(typeSpec.Pos()).Line,
					EndLine:   fset.Position(typeSpec.End()).Line,
				}, displayName)
			}
		}
	}

	return imports, functions, symbols, nil
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "(*" + receiverName(t.X) + ")"
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	default:
		return "receiver"
	}
}

func receiverBaseName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverBaseName(t.X)
	case *ast.IndexExpr:
		return receiverBaseName(t.X)
	case *ast.IndexListExpr:
		return receiverBaseName(t.X)
	default:
		return "receiver"
	}
}

// ShouldInclude determines if a file should be included in knowledge base.
func ShouldInclude(filePath string, cfg *Config) bool {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	for _, excludeDir := range cfg.ExcludeDirs {
		if strings.HasPrefix(filePath, excludeDir+"/") || filePath == excludeDir {
			return false
		}
	}

	if len(cfg.IncludeFiles) > 0 {
		matchedInclude := false
		base := filepath.Base(filePath)
		for _, pattern := range cfg.IncludeFiles {
			matchedPath, _ := filepath.Match(pattern, filePath)
			matchedBase, _ := filepath.Match(pattern, base)
			if matchedPath || matchedBase {
				matchedInclude = true
				break
			}
		}
		if !matchedInclude {
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
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

// ExtractImports extracts import statements from code.
func ExtractImports(content, fileType string) []string {
	seen := make(map[string]bool)
	var imports []string

	lines := strings.Split(content, "\n")
	if patList, ok := importPatterns[fileType]; ok {
		for _, line := range lines {
			for _, pattern := range patList {
				if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
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

// ExtractFunctions extracts function definitions from code.
func ExtractFunctions(content, fileType string) []string {
	seen := make(map[string]bool)
	var fns []string

	lines := strings.Split(content, "\n")
	if patList, ok := functionPatterns[fileType]; ok {
		for _, line := range lines {
			for _, pattern := range patList {
				if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
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
