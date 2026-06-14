package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

var benchmarkStringsSink []string

func TestShouldIncludeHonorsIncludeFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main\n")
	mustWriteFile(t, filepath.Join(dir, "notes.txt"), "ignore me\n")
	mustWriteFile(t, filepath.Join(dir, "vendor", "dep.go"), "package dep\n")
	mustWriteFile(t, filepath.Join(dir, "large.go"), strings.Repeat("x", 11*1024))

	cfg := &Config{
		RepoPath:      dir,
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{"vendor"},
		MaxFileSizeKB: 10,
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "included go file", path: "main.go", want: true},
		{name: "unsupported extension", path: "notes.txt", want: false},
		{name: "excluded directory", path: "vendor/dep.go", want: false},
		{name: "file too large", path: "large.go", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldInclude(tt.path, cfg); got != tt.want {
				t.Fatalf("ShouldInclude(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestAnalyzeFileExtractsImportsAndFunctions(t *testing.T) {
	dir := initTestRepo(t)
	mustWriteFile(t, filepath.Join(dir, "main.go"), `package main

import (
	"fmt"
	"strings"
)

type Runner interface { Run() error }
type Config struct { Name string }
type Count = int

func (c *Config) Run() error { return nil }
func main() { fmt.Println(strings.TrimSpace(" ok ")) }
func helper() {}
`)
	runGit(t, dir, "add", "main.go")
	runGit(t, dir, "commit", "-m", "add main")

	summary, err := AnalyzeFile(dir, "main.go")
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	assertContains(t, summary.Imports, "fmt")
	assertContains(t, summary.Imports, "strings")
	assertContains(t, summary.Functions, "main")
	assertContains(t, summary.Functions, "helper")
	assertContains(t, summary.Functions, "(*Config).Run")
	assertContains(t, summary.Functions, "interface Runner")
	assertContains(t, summary.Functions, "struct Config")
	assertContains(t, summary.Functions, "type Count")
	assertSymbol(t, summary.Symbols, "method", "Run", "Config", 12, 12)
	assertSymbol(t, summary.Symbols, "function", "main", "", 13, 13)
	assertSymbol(t, summary.Symbols, "interface", "Runner", "", 8, 8)
	assertSymbol(t, summary.Symbols, "struct", "Config", "", 9, 9)
	assertSymbol(t, summary.Symbols, "type", "Count", "", 10, 10)
	assertSymbolDetails(t, summary.Symbols, "Runner", "main", "main.go", true)
	assertSymbolDetails(t, summary.Symbols, "main", "main", "main.go", false)
	if summary.LastAuthor == "" || summary.LastCommit == "" {
		t.Fatalf("expected git metadata to be populated, got author=%q commit=%q", summary.LastAuthor, summary.LastCommit)
	}
}

func TestAnalyzeFileFallsBackOnMalformedGoAndRecordsWarning(t *testing.T) {
	dir := initTestRepo(t)
	mustWriteFile(t, filepath.Join(dir, "broken.go"), "package main\nfunc broken(\nfunc fallback() {}\n")
	runGit(t, dir, "add", "broken.go")
	runGit(t, dir, "commit", "-m", "add broken")

	summary, err := AnalyzeFile(dir, "broken.go")
	if err != nil {
		t.Fatalf("AnalyzeFile returned error: %v", err)
	}

	assertContains(t, summary.Functions, "fallback")
	if len(summary.ParseWarnings) == 0 {
		t.Fatal("expected malformed Go file to record parse warning")
	}
	if !strings.Contains(summary.ParseWarnings[0], "failed to parse Go AST") {
		t.Fatalf("parse warning = %q, want failed Go AST parse message", summary.ParseWarnings[0])
	}
}

func TestBatchGitMetadataPopulatesMultipleFiles(t *testing.T) {
	dir := initTestRepo(t)
	mustWriteFile(t, filepath.Join(dir, "one.go"), "package main\nfunc one() {}\n")
	mustWriteFile(t, filepath.Join(dir, "two.go"), "package main\nfunc two() {}\n")
	runGit(t, dir, "add", "one.go", "two.go")
	runGit(t, dir, "commit", "-m", "initial")
	mustWriteFile(t, filepath.Join(dir, "one.go"), "package main\nfunc one() {}\nfunc changed() {}\n")
	runGit(t, dir, "add", "one.go")
	runGit(t, dir, "commit", "-m", "change one")

	metadata, err := BatchGitMetadata(dir, []string{"one.go", "two.go"})
	if err != nil {
		t.Fatalf("BatchGitMetadata returned error: %v", err)
	}

	one := metadata["one.go"]
	if one.LastAuthor != "Test User" || one.LastCommit != currentShortCommit(t, dir) || one.CommitCount != 2 {
		t.Fatalf("one.go metadata = %#v, want last author, current short commit, count 2", one)
	}
	two := metadata["two.go"]
	if two.LastAuthor != "Test User" || two.LastCommit == "" || two.CommitCount != 1 {
		t.Fatalf("two.go metadata = %#v, want last author, non-empty commit, count 1", two)
	}
}

func TestAnalyzeFileUsesProvidedGitMetadata(t *testing.T) {
	dir := initTestRepo(t)
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, dir, "add", "main.go")
	runGit(t, dir, "commit", "-m", "initial")

	summary, err := AnalyzeFileWithMetadata(dir, "main.go", &GitMetadata{
		LastAuthor:  "Batch Author",
		LastCommit:  "abc1234",
		CommitCount: 42,
	})
	if err != nil {
		t.Fatalf("AnalyzeFileWithMetadata returned error: %v", err)
	}
	if summary.LastAuthor != "Batch Author" || summary.LastCommit != "abc1234" || summary.CommitCount != 42 {
		t.Fatalf("summary metadata = author %q commit %q count %d", summary.LastAuthor, summary.LastCommit, summary.CommitCount)
	}
}

func TestAnalyzeFilesWithMetadataReturnsSortedSummaries(t *testing.T) {
	dir := initTestRepo(t)
	mustWriteFile(t, filepath.Join(dir, "zeta.go"), "package main\nfunc zeta() {}\n")
	mustWriteFile(t, filepath.Join(dir, "alpha.go"), "package main\nfunc alpha() {}\n")
	mustWriteFile(t, filepath.Join(dir, "middle.go"), "package main\nfunc middle() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	metadata, err := BatchGitMetadata(dir, []string{"zeta.go", "alpha.go", "middle.go"})
	if err != nil {
		t.Fatalf("BatchGitMetadata returned error: %v", err)
	}
	summaries, err := AnalyzeFilesWithMetadata(dir, []string{"zeta.go", "alpha.go", "middle.go"}, metadata, 2)
	if err != nil {
		t.Fatalf("AnalyzeFilesWithMetadata returned error: %v", err)
	}

	got := summaryPaths(summaries)
	want := []string{"alpha.go", "middle.go", "zeta.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("summary paths = %v, want sorted %v", got, want)
	}
	for _, summary := range summaries {
		if summary.LastAuthor != "Test User" || summary.LastCommit == "" || summary.CommitCount != 1 {
			t.Fatalf("summary %s metadata = author %q commit %q count %d", summary.RelativePath, summary.LastAuthor, summary.LastCommit, summary.CommitCount)
		}
	}
}

func TestDefaultConfigSetsBoundedWorkers(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Workers <= 0 {
		t.Fatalf("DefaultConfig Workers = %d, want positive worker limit", cfg.Workers)
	}
}

func TestUpdateFromLastCommitIncludesChangedFiles(t *testing.T) {
	dir := initTestRepo(t)
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main\nfunc main() {}\n")
	runGit(t, dir, "add", "main.go")
	runGit(t, dir, "commit", "-m", "add main")

	bundle, updated, err := UpdateFromLastCommit(&Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	})
	if err != nil {
		t.Fatalf("UpdateFromLastCommit returned error: %v", err)
	}
	if len(updated) != 1 || updated[0] != "main.go" {
		t.Fatalf("updated = %v, want [main.go]", updated)
	}
	if bundle == nil || len(bundle.Concepts) != 1 {
		t.Fatalf("expected one updated concept, got bundle=%#v", bundle)
	}
}

func TestApplyIncrementalUpdatePersistsAddedModifiedAndDeletedFiles(t *testing.T) {
	dir := initTestRepo(t)
	cfg := &Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	}

	mustWriteFile(t, filepath.Join(dir, "old.go"), "package main\nfunc old() {}\n")
	mustWriteFile(t, filepath.Join(dir, "stay.go"), "package main\nfunc stay() {}\n")
	runGit(t, dir, "add", "old.go", "stay.go")
	runGit(t, dir, "commit", "-m", "initial")

	initial, err := GenerateBundle(cfg, true)
	if err != nil {
		t.Fatalf("GenerateBundle returned error: %v", err)
	}
	if _, err := SaveKnowledgeBase(initial, cfg); err != nil {
		t.Fatalf("SaveKnowledgeBase returned error: %v", err)
	}
	state, err := ReadState(cfg)
	if err != nil {
		t.Fatalf("ReadState after SaveKnowledgeBase returned error: %v", err)
	}
	if state.LastIndexedCommit != currentCommit(t, dir) {
		t.Fatalf("state LastIndexedCommit = %q, want current HEAD", state.LastIndexedCommit)
	}

	mustWriteFile(t, filepath.Join(dir, "stay.go"), "package main\nfunc stay() {}\nfunc changed() {}\n")
	mustWriteFile(t, filepath.Join(dir, "new.go"), "package main\nfunc fresh() {}\n")
	if err := os.Remove(filepath.Join(dir, "old.go")); err != nil {
		t.Fatalf("remove old.go: %v", err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "update files")

	bundle, updated, err := UpdateFromLastCommit(cfg)
	if err != nil {
		t.Fatalf("UpdateFromLastCommit returned error: %v", err)
	}
	if err := ApplyIncrementalUpdate(cfg, bundle); err != nil {
		t.Fatalf("ApplyIncrementalUpdate returned error: %v", err)
	}

	loaded, err := okf.LoadBundle(filepath.Join(dir, cfg.KnowledgeDir), okf.DefaultLoadOptions())
	if err != nil {
		t.Fatalf("LoadBundle returned error: %v", err)
	}

	if conceptByResource(loaded, "old.go") != nil {
		t.Fatal("expected deleted source concept old.go to be removed")
	}
	if conceptByResource(loaded, "new.go") == nil {
		t.Fatal("expected added source concept new.go to be saved")
	}
	stay := conceptByResource(loaded, "stay.go")
	if stay == nil || !strings.Contains(stay.Content, "changed") {
		t.Fatalf("expected modified stay.go concept to include changed function, got %#v", stay)
	}
	assertContains(t, updated, "new.go")
	assertContains(t, updated, "stay.go")
	assertContains(t, updated, "old.go (deleted)")
}

func TestUpdateSinceLastIndexedCommitProcessesMultipleCommits(t *testing.T) {
	dir := initTestRepo(t)
	cfg := &Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	}

	mustWriteFile(t, filepath.Join(dir, "base.go"), "package main\nfunc base() {}\n")
	runGit(t, dir, "add", "base.go")
	runGit(t, dir, "commit", "-m", "base")
	baseCommit := currentCommit(t, dir)

	mustWriteFile(t, filepath.Join(dir, "one.go"), "package main\nfunc one() {}\n")
	runGit(t, dir, "add", "one.go")
	runGit(t, dir, "commit", "-m", "one")
	mustWriteFile(t, filepath.Join(dir, "two.go"), "package main\nfunc two() {}\n")
	runGit(t, dir, "add", "two.go")
	runGit(t, dir, "commit", "-m", "two")

	if err := WriteState(cfg, &State{LastIndexedCommit: baseCommit}); err != nil {
		t.Fatalf("WriteState returned error: %v", err)
	}
	bundle, updated, err := UpdateSinceLastIndexedCommit(cfg)
	if err != nil {
		t.Fatalf("UpdateSinceLastIndexedCommit returned error: %v", err)
	}
	assertContains(t, updated, "one.go")
	assertContains(t, updated, "two.go")
	if len(bundle.Concepts) != 2 {
		t.Fatalf("updated bundle has %d concepts, want 2", len(bundle.Concepts))
	}
	state, err := ReadState(cfg)
	if err != nil {
		t.Fatalf("ReadState returned error: %v", err)
	}
	if state.LastIndexedCommit != baseCommit {
		t.Fatalf("state LastIndexedCommit = %q, want old checkpoint until save succeeds", state.LastIndexedCommit)
	}
}

func TestUpdateSinceLastIndexedCommitDoesNotWriteStateBeforePersistence(t *testing.T) {
	dir := initTestRepo(t)
	cfg := &Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	}

	mustWriteFile(t, filepath.Join(dir, "base.go"), "package main\nfunc base() {}\n")
	runGit(t, dir, "add", "base.go")
	runGit(t, dir, "commit", "-m", "base")
	baseCommit := currentCommit(t, dir)
	if err := WriteState(cfg, &State{LastIndexedCommit: baseCommit}); err != nil {
		t.Fatalf("WriteState returned error: %v", err)
	}

	mustWriteFile(t, filepath.Join(dir, "next.go"), "package main\nfunc next() {}\n")
	runGit(t, dir, "add", "next.go")
	runGit(t, dir, "commit", "-m", "next")

	if _, _, err := UpdateSinceLastIndexedCommit(cfg); err != nil {
		t.Fatalf("UpdateSinceLastIndexedCommit returned error: %v", err)
	}
	state, err := ReadState(cfg)
	if err != nil {
		t.Fatalf("ReadState returned error: %v", err)
	}
	if state.LastIndexedCommit != baseCommit {
		t.Fatalf("state advanced to %q before persistence, want %q", state.LastIndexedCommit, baseCommit)
	}
}

func TestUpdateSinceLastIndexedCommitAdvancesStateWhenOnlyExcludedFilesChanged(t *testing.T) {
	dir := initTestRepo(t)
	cfg := &Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	}

	mustWriteFile(t, filepath.Join(dir, "base.go"), "package main\nfunc base() {}\n")
	runGit(t, dir, "add", "base.go")
	runGit(t, dir, "commit", "-m", "base")
	baseCommit := currentCommit(t, dir)
	if err := WriteState(cfg, &State{LastIndexedCommit: baseCommit}); err != nil {
		t.Fatalf("WriteState returned error: %v", err)
	}

	mustWriteFile(t, filepath.Join(dir, "notes.txt"), "not indexed\n")
	runGit(t, dir, "add", "notes.txt")
	runGit(t, dir, "commit", "-m", "excluded change")
	head := currentCommit(t, dir)

	bundle, updated, err := UpdateSinceLastIndexedCommit(cfg)
	if err != nil {
		t.Fatalf("UpdateSinceLastIndexedCommit returned error: %v", err)
	}
	if bundle != nil || len(updated) != 0 {
		t.Fatalf("bundle=%#v updated=%v, want no indexable changes", bundle, updated)
	}
	state, err := ReadState(cfg)
	if err != nil {
		t.Fatalf("ReadState returned error: %v", err)
	}
	if state.LastIndexedCommit != head {
		t.Fatalf("state LastIndexedCommit = %q, want %q", state.LastIndexedCommit, head)
	}
}

func TestChangedFilesBetweenHandlesSpacesAndRenames(t *testing.T) {
	dir := initTestRepo(t)
	mustWriteFile(t, filepath.Join(dir, "old name.go"), "package main\nfunc oldName() {}\n")
	runGit(t, dir, "add", "old name.go")
	runGit(t, dir, "commit", "-m", "add old name")
	baseCommit := currentCommit(t, dir)

	if err := os.Rename(filepath.Join(dir, "old name.go"), filepath.Join(dir, "new name.go")); err != nil {
		t.Fatalf("rename file: %v", err)
	}
	mustWriteFile(t, filepath.Join(dir, "new name.go"), "package main\nfunc newName() {}\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "rename with spaces")

	files, err := changedFilesBetween(dir, baseCommit, currentCommit(t, dir))
	if err != nil {
		t.Fatalf("changedFilesBetween returned error: %v", err)
	}
	assertContains(t, files, "old name.go")
	assertContains(t, files, "new name.go")
}

func TestGenerateBundleCreatesDeterministicRelationshipGraph(t *testing.T) {
	dir := initTestRepo(t)
	cfg := &Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	}

	mustWriteFile(t, filepath.Join(dir, "cmd", "app", "main.go"), `package main

import "example.com/app/pkg/lib"

func main() { lib.Run() }
`)
	mustWriteFile(t, filepath.Join(dir, "pkg", "lib", "lib.go"), `package lib

import "fmt"

type Service struct{}
func Run() { fmt.Println("ok") }
`)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add packages")

	bundle, err := GenerateBundle(cfg, true)
	if err != nil {
		t.Fatalf("GenerateBundle returned error: %v", err)
	}
	relationships := conceptByResource(bundle, relationshipGraphResource)
	if relationships == nil {
		t.Fatal("expected relationship graph concept to be generated")
	}

	assertContentContains(t, relationships.Content, "### File Imports")
	assertContentContains(t, relationships.Content, "- `cmd/app/main.go` imports `example.com/app/pkg/lib`")
	assertContentContains(t, relationships.Content, "- `pkg/lib/lib.go` imports `fmt`")
	assertContentContains(t, relationships.Content, "### Package Imports")
	assertContentContains(t, relationships.Content, "- `main` imports `example.com/app/pkg/lib`")
	assertContentContains(t, relationships.Content, "- `lib` imports `fmt`")
	assertContentContains(t, relationships.Content, "### File Owns Symbols")
	assertContentContains(t, relationships.Content, "- `cmd/app/main.go` owns `main.main` (function)")
	assertContentContains(t, relationships.Content, "- `pkg/lib/lib.go` owns `lib.Service` (struct)")
	assertContentContains(t, relationships.Content, "### Package Owns Symbols")
	assertContentContains(t, relationships.Content, "- `lib` owns `Service` (struct)")
	assertInOrder(t, relationships.Content,
		"- `cmd/app/main.go` imports `example.com/app/pkg/lib`",
		"- `pkg/lib/lib.go` imports `fmt`",
	)
}

func TestApplyIncrementalUpdateRebuildsRelationshipGraphAfterDelete(t *testing.T) {
	dir := initTestRepo(t)
	cfg := &Config{
		RepoPath:      dir,
		KnowledgeDir:  ".okf/knowledge",
		IncludeFiles:  []string{"*.go"},
		ExcludeDirs:   []string{".git", ".okf"},
		MaxFileSizeKB: 100,
	}

	mustWriteFile(t, filepath.Join(dir, "old.go"), "package main\nfunc old() {}\n")
	mustWriteFile(t, filepath.Join(dir, "stay.go"), "package main\nfunc stay() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	initial, err := GenerateBundle(cfg, true)
	if err != nil {
		t.Fatalf("GenerateBundle returned error: %v", err)
	}
	if _, err := SaveKnowledgeBase(initial, cfg); err != nil {
		t.Fatalf("SaveKnowledgeBase returned error: %v", err)
	}

	if err := os.Remove(filepath.Join(dir, "old.go")); err != nil {
		t.Fatalf("remove old.go: %v", err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "delete old")

	incremental, _, err := UpdateFromLastCommit(cfg)
	if err != nil {
		t.Fatalf("UpdateFromLastCommit returned error: %v", err)
	}
	if err := ApplyIncrementalUpdate(cfg, incremental); err != nil {
		t.Fatalf("ApplyIncrementalUpdate returned error: %v", err)
	}

	loaded, err := okf.LoadBundle(filepath.Join(dir, cfg.KnowledgeDir), okf.DefaultLoadOptions())
	if err != nil {
		t.Fatalf("LoadBundle returned error: %v", err)
	}
	relationships := conceptByResource(loaded, relationshipGraphResource)
	if relationships == nil {
		t.Fatal("expected relationship graph concept after incremental update")
	}
	if strings.Contains(relationships.Content, "old.go") {
		t.Fatalf("relationship graph still references deleted file: %s", relationships.Content)
	}
	assertContentContains(t, relationships.Content, "- `stay.go` owns `main.stay` (function)")
}

func TestSaveKnowledgeBaseReturnsWriteErrors(t *testing.T) {
	dir := initTestRepo(t)
	cfg := &Config{RepoPath: dir, KnowledgeDir: ".okf/knowledge"}
	bundle := okf.NewBundle("bad")
	concept := okf.NewConcept("component", "Bad")
	concept.FilePath = "bad\x00path.md"
	bundle.Concepts = append(bundle.Concepts, concept)

	if _, err := SaveKnowledgeBase(bundle, cfg); err == nil {
		t.Fatal("SaveKnowledgeBase returned nil error for invalid output path")
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func currentCommit(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func currentShortCommit(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse --short HEAD failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%v does not contain %q", values, want)
}

func assertContentContains(t *testing.T, content, want string) {
	t.Helper()
	if !strings.Contains(content, want) {
		t.Fatalf("content does not contain %q\ncontent:\n%s", want, content)
	}
}

func assertInOrder(t *testing.T, content string, ordered ...string) {
	t.Helper()
	last := -1
	for _, want := range ordered {
		idx := strings.Index(content, want)
		if idx == -1 {
			t.Fatalf("content does not contain %q\ncontent:\n%s", want, content)
		}
		if idx < last {
			t.Fatalf("%q appeared before previous expected content\ncontent:\n%s", want, content)
		}
		last = idx
	}
}

func conceptByResource(bundle *okf.KnowledgeBundle, resource string) *okf.Concept {
	for _, concept := range bundle.Concepts {
		if concept.Resource == resource {
			return concept
		}
	}
	return nil
}

func summaryPaths(summaries []*FileSummary) []string {
	paths := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		paths = append(paths, summary.RelativePath)
	}
	return paths
}

func assertSymbol(t *testing.T, symbols []Symbol, kind, name, receiver string, startLine, endLine int) {
	t.Helper()
	for _, symbol := range symbols {
		if symbol.Kind == kind && symbol.Name == name && symbol.Receiver == receiver && symbol.StartLine == startLine && symbol.EndLine == endLine {
			return
		}
	}
	t.Fatalf("symbols %#v do not contain %s %s receiver=%q lines=%d-%d", symbols, kind, name, receiver, startLine, endLine)
}

func assertSymbolDetails(t *testing.T, symbols []Symbol, name, pkg, filePath string, exported bool) {
	t.Helper()
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Package == pkg && symbol.FilePath == filePath && symbol.Exported == exported {
			return
		}
	}
	t.Fatalf("symbols %#v do not contain %s package=%q file=%q exported=%v", symbols, name, pkg, filePath, exported)
}

func BenchmarkExtractImports(b *testing.B) {
	content := repeatedSource(500, `import "fmt"\nimport "strings"\nfmt.Println(strings.TrimSpace("ok"))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkStringsSink = ExtractImports(content, "go")
	}
}

func BenchmarkExtractFunctions(b *testing.B) {
	content := repeatedSource(500, `func handler%d() {}\ntype Service%d struct{}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkStringsSink = ExtractFunctions(content, "go")
	}
}

func BenchmarkGitMetadata(b *testing.B) {
	dir, files := initBenchmarkRepo(b, 20)

	b.Run("batch", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metadata, err := BatchGitMetadata(dir, files)
			if err != nil {
				b.Fatal(err)
			}
			if len(metadata) != len(files) {
				b.Fatalf("metadata size = %d, want %d", len(metadata), len(files))
			}
		}
	})

	b.Run("per-file-legacy", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metadata := make(map[string]*GitMetadata, len(files))
			for _, file := range files {
				metadata[file] = legacyGitMetadata(b, dir, file)
			}
			if len(metadata) != len(files) {
				b.Fatalf("metadata size = %d, want %d", len(metadata), len(files))
			}
		}
	})
}

func BenchmarkAnalyzeFilesWithMetadata(b *testing.B) {
	dir, files := initBenchmarkRepo(b, 30)
	metadata, err := BatchGitMetadata(dir, files)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("workers=1", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			summaries, err := AnalyzeFilesWithMetadata(dir, files, metadata, 1)
			if err != nil {
				b.Fatal(err)
			}
			if len(summaries) != len(files) {
				b.Fatalf("summaries size = %d, want %d", len(summaries), len(files))
			}
		}
	})

	b.Run("workers=cpu", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			summaries, err := AnalyzeFilesWithMetadata(dir, files, metadata, runtime.NumCPU())
			if err != nil {
				b.Fatal(err)
			}
			if len(summaries) != len(files) {
				b.Fatalf("summaries size = %d, want %d", len(summaries), len(files))
			}
		}
	})
}

func BenchmarkIndexRepo(b *testing.B) {
	for _, tc := range []struct {
		name      string
		fileCount int
	}{
		{name: "small", fileCount: 30},
		{name: "files=1000", fileCount: 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			dir, _ := initBenchmarkRepo(b, tc.fileCount)
			cfg := &Config{
				RepoPath:      dir,
				KnowledgeDir:  ".okf/knowledge",
				IncludeFiles:  []string{"*.go"},
				ExcludeDirs:   []string{".git", ".okf"},
				MaxFileSizeKB: 100,
				Workers:       runtime.NumCPU(),
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bundle, err := GenerateBundle(cfg, true)
				if err != nil {
					b.Fatal(err)
				}
				if len(bundle.Concepts) == 0 {
					b.Fatal("GenerateBundle returned no concepts")
				}
			}
		})
	}
}

func repeatedSource(count int, pattern string) string {
	var builder strings.Builder
	for i := 0; i < count; i++ {
		if strings.Contains(pattern, "%d") {
			fmt.Fprintf(&builder, pattern, i, i)
		} else {
			builder.WriteString(pattern)
		}
		builder.WriteByte('\n')
	}
	return builder.String()
}

func initBenchmarkRepo(b *testing.B, fileCount int) (string, []string) {
	b.Helper()
	dir := b.TempDir()
	runBenchmarkGit(b, dir, "init")
	runBenchmarkGit(b, dir, "config", "user.name", "Benchmark User")
	runBenchmarkGit(b, dir, "config", "user.email", "bench@example.com")

	files := make([]string, 0, fileCount)
	for i := 0; i < fileCount; i++ {
		file := fmt.Sprintf("file_%02d.go", i)
		files = append(files, file)
		mustWriteBenchmarkFile(b, filepath.Join(dir, file), fmt.Sprintf("package main\nfunc f%d() {}\n", i))
	}
	runBenchmarkGit(b, dir, append([]string{"add"}, files...)...)
	runBenchmarkGit(b, dir, "commit", "-m", "initial")
	for i, file := range files {
		if i%2 == 0 {
			mustWriteBenchmarkFile(b, filepath.Join(dir, file), fmt.Sprintf("package main\nfunc f%d() {}\nfunc changed%d() {}\n", i, i))
		}
	}
	runBenchmarkGit(b, dir, "add", "-A")
	runBenchmarkGit(b, dir, "commit", "-m", "change half")
	return dir, files
}

func legacyGitMetadata(b *testing.B, dir, file string) *GitMetadata {
	b.Helper()
	metadata := &GitMetadata{}
	lastAuthor := exec.Command("git", "log", "-1", "--format=%an", "--", file)
	lastAuthor.Dir = dir
	if out, err := lastAuthor.CombinedOutput(); err == nil {
		metadata.LastAuthor = strings.TrimSpace(string(out))
	} else {
		b.Fatalf("git log author failed: %v\n%s", err, out)
	}
	lastCommit := exec.Command("git", "log", "-1", "--format=%h", "--", file)
	lastCommit.Dir = dir
	if out, err := lastCommit.CombinedOutput(); err == nil {
		metadata.LastCommit = strings.TrimSpace(string(out))
	} else {
		b.Fatalf("git log commit failed: %v\n%s", err, out)
	}
	commitCount := exec.Command("git", "rev-list", "--count", "HEAD", "--", file)
	commitCount.Dir = dir
	if out, err := commitCount.CombinedOutput(); err == nil {
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &metadata.CommitCount)
	} else {
		b.Fatalf("git rev-list failed: %v\n%s", err, out)
	}
	return metadata
}

func runBenchmarkGit(b *testing.B, dir string, args ...string) {
	b.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func mustWriteBenchmarkFile(b *testing.B, path, content string) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		b.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		b.Fatalf("write %s: %v", path, err)
	}
}
