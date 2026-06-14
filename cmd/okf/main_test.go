package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/query"
)

func TestCLISmokeInitSearchUpdateLint(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)

	mustWriteCLIFile(t, filepath.Join(repo, "main.go"), "package main\nfunc StartServer() {}\nfunc main() {}\n")
	runCLIGit(t, repo, "add", "main.go")
	runCLIGit(t, repo, "commit", "-m", "initial")

	initOut := runOKF(t, bin, "init", "-repo", repo, "-force")
	if !strings.Contains(initOut, "Generated") {
		t.Fatalf("init output = %q, want Generated", initOut)
	}

	searchOut := runOKF(t, bin, "search", "-path", repo, "-q", "main")
	if !strings.Contains(searchOut, "main.go") {
		t.Fatalf("search output = %q, want main.go", searchOut)
	}

	codeFilterSearches := []struct {
		name string
		args []string
		want string
	}{
		{name: "language", args: []string{"-code-language", "go"}, want: "main.go"},
		{name: "file path", args: []string{"-code-path", "main.go"}, want: "main.go"},
		{name: "symbol kind", args: []string{"-code-symbol-kind", "function"}, want: "main.go"},
		{name: "qualified name", args: []string{"-code-qualified-name", "StartServer"}, want: "main.go"},
		{name: "relation kind", args: []string{"-code-relation-kind", "contains"}, want: "Code Relation Index"},
	}
	for _, tt := range codeFilterSearches {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"search", "-path", repo}, tt.args...)
			out := runOKF(t, bin, args...)
			if !strings.Contains(out, tt.want) {
				t.Fatalf("search output = %q, want %s", out, tt.want)
			}
		})
	}

	symbolSearchOut := runOKF(t, bin, "search", "-path", repo, "-q", "StartServer")
	if !strings.Contains(symbolSearchOut, "symbol matches") || !strings.Contains(symbolSearchOut, "function StartServer") || !strings.Contains(symbolSearchOut, "main.go:") {
		t.Fatalf("symbol search output = %q, want symbol kind, name, and source location", symbolSearchOut)
	}

	mustWriteCLIFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\nfunc changed() {}\n")
	runCLIGit(t, repo, "add", "main.go")
	runCLIGit(t, repo, "commit", "-m", "change main")

	updateOut := runOKF(t, bin, "update", "-repo", repo, "-verbose")
	if !strings.Contains(updateOut, "Updated") || !strings.Contains(updateOut, "main.go") {
		t.Fatalf("update output = %q, want Updated and main.go", updateOut)
	}

	data, err := os.ReadFile(filepath.Join(repo, ".okf", "knowledge", "main.go.md"))
	if err != nil {
		t.Fatalf("read updated concept: %v", err)
	}
	if !strings.Contains(string(data), "changed") {
		t.Fatalf("updated concept = %q, want changed function", data)
	}

	lintOut := runOKF(t, bin, "lint", "-path", repo)
	if !strings.Contains(lintOut, "All checks passed") {
		t.Fatalf("lint output = %q, want all checks passed", lintOut)
	}
}

func TestFilterSearchResultsPreservesSymbolMatchesForDuplicateConceptKeys(t *testing.T) {
	results := []query.SearchResult{
		{
			Concept:       &query.Concept{Type: "component", Title: "duplicate.go", FilePath: "duplicate.go", Tags: []string{"first"}},
			SymbolMatches: []query.SymbolMatch{{Kind: "function", Name: "First", Location: "duplicate.go:1-2"}},
		},
		{
			Concept:       &query.Concept{Type: "component", Title: "duplicate.go", FilePath: "duplicate.go", Tags: []string{"second"}},
			SymbolMatches: []query.SymbolMatch{{Kind: "function", Name: "Second", Location: "duplicate.go:3-4"}},
		},
	}

	filtered := filterSearchResults(results, "", "second")
	if len(filtered) != 1 {
		t.Fatalf("filtered %d results, want 1", len(filtered))
	}
	if len(filtered[0].SymbolMatches) != 1 || filtered[0].SymbolMatches[0].Name != "Second" {
		t.Fatalf("filtered symbol matches = %#v, want Second", filtered[0].SymbolMatches)
	}
}

func buildOKF(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "okf")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build CLI failed: %v\n%s", err, out)
	}
	return bin
}

func initCLIRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runCLIGit(t, dir, "init")
	runCLIGit(t, dir, "config", "user.name", "Test User")
	runCLIGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

func runOKF(t *testing.T, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("okf %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func runCLIGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func mustWriteCLIFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
