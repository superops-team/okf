package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
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

func TestCLIConfigGetKnowledgeDirReportsResolvedSource(t *testing.T) {
	bin := buildOKF(t)
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configuredDir := filepath.Join(tmpDir, "configured", "knowledge")
	if err := okf.SaveConfig(&okf.Config{KnowledgeDir: configuredDir}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	out := runOKFWithEnv(t, bin, []string{"OKF_CONFIG_PATH=" + configPath}, "config", "get", "knowledge_dir")
	if !strings.Contains(out, configuredDir) {
		t.Fatalf("config get output = %q, want resolved path %q", out, configuredDir)
	}
	if !strings.Contains(out, "(source: config)") {
		t.Fatalf("config get output = %q, want explicit source metadata '(source: config)'", out)
	}
}

func TestCLIAddImportsArchiveThroughDispatcher(t *testing.T) {
	bin := buildOKF(t)
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "bundle.zip")
	writeCLITestZip(t, archivePath, map[string]string{
		"concepts/service.md": `---
type: concept
title: Archive Service
---
Archive content.
`,
	})
	knowledgeDir := filepath.Join(tmpDir, "knowledge")

	out := runOKF(t, bin, "add", "-dir", knowledgeDir, archivePath)
	if !strings.Contains(out, "Imported: 1") {
		t.Fatalf("add archive output = %q, want Imported: 1", out)
	}
	if _, err := os.Stat(filepath.Join(knowledgeDir, "concepts", "service.md")); err != nil {
		t.Fatalf("archive markdown was not imported to expected target: %v", err)
	}
}

func TestCLIToolStatusJSONEnvelope(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
tags:
  - alpha
---
Alpha content.
`)

	out := runOKF(t, bin, "tool", "status", "--repo", repo, "--json")
	env := decodeToolEnvelope(t, out)
	if env["schema_version"] != "okf.tool.v1" || env["operation"] != "status" || env["ok"] != true {
		t.Fatalf("tool status envelope = %#v", env)
	}
	if env["mutating"] != false {
		t.Fatalf("mutating = %#v, want false", env["mutating"])
	}
	result, ok := env["result"].(map[string]interface{})
	if !ok || result["ready"] != true || result["concept_count"].(float64) != 1 {
		t.Fatalf("result = %#v, want ready concept_count=1", env["result"])
	}
	pathMeta, ok := result["knowledge_path"].(map[string]interface{})
	if !ok || pathMeta["write_path"] == "" {
		t.Fatalf("knowledge_path = %#v, want write_path metadata", result["knowledge_path"])
	}
	readPaths, ok := pathMeta["read_paths"].([]interface{})
	if !ok || len(readPaths) != 1 {
		t.Fatalf("read_paths = %#v, want single legacy read path", pathMeta["read_paths"])
	}
}

func TestCLIToolInitJSONEnvelopeIsMutating(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	mustWriteCLIFile(t, filepath.Join(repo, "main.go"), "package main\nfunc main() {}\n")
	runCLIGit(t, repo, "add", "main.go")
	runCLIGit(t, repo, "commit", "-m", "add main")

	out := runOKF(t, bin, "tool", "init", "--repo", repo, "--json")
	env := decodeToolEnvelope(t, out)
	if env["schema_version"] != "okf.tool.v1" || env["operation"] != "init" || env["ok"] != true {
		t.Fatalf("tool init envelope = %#v", env)
	}
	if env["mutating"] != true {
		t.Fatalf("mutating = %#v, want true", env["mutating"])
	}
}

func TestCLIToolQueryStructuredFlags(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: RouteAlphaSymbol
description: Exact symbol for alpha routing
resource: code://repo/internal/alpha/service.go
tags:
  - code
source_path: internal/alpha/service.go
language: go
symbol_kind: function
qualified_name: alpha.RouteAlphaSymbol
start_line: 12
end_line: 14
---
RouteAlphaSymbol handles alpha routing.
`)

	out := runOKF(t, bin, "tool", "query", "--repo", repo, "--q", "RouteAlphaSymbol", "--type", "code_symbol", "--language", "go", "--symbol-kind", "function", "--qualified-name", "alpha.RouteAlphaSymbol", "--limit", "5", "--json")
	env := decodeToolEnvelope(t, out)
	if env["operation"] != "query" || env["ok"] != true || env["mutating"] != false {
		t.Fatalf("tool query envelope = %#v", env)
	}
	result := env["result"].(map[string]interface{})
	results := result["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	hit := results[0].(map[string]interface{})
	if hit["source_path"] != "internal/alpha/service.go" || hit["qualified_name"] != "alpha.RouteAlphaSymbol" || hit["location"] != "internal/alpha/service.go:12-14" {
		t.Fatalf("hit = %#v, want source navigation", hit)
	}
}

func TestCLIToolQueryRelationSourceAndTargetFlags(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "alpha-beta.md"), `---
type: code_relation
title: alpha imports beta
description: alpha imports beta
resource: okf://relations/alpha-beta
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/beta/service.go
start_line: 10
end_line: 10
---
alpha imports beta
`)
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "relations", "alpha-gamma.md"), `---
type: code_relation
title: alpha imports gamma
description: alpha imports gamma
resource: okf://relations/alpha-gamma
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/gamma/service.go
start_line: 11
end_line: 11
---
alpha imports gamma
`)

	out := runOKF(t, bin, "tool", "query", "--repo", repo, "--q", "imports", "--type", "code_relation", "--relation-kind", "file_import", "--relation-source", "internal/alpha/service.go", "--relation-target", "internal/beta/service.go", "--limit", "5", "--json")
	env := decodeToolEnvelope(t, out)
	if env["operation"] != "query" || env["ok"] != true {
		t.Fatalf("tool query envelope = %#v", env)
	}
	result := env["result"].(map[string]interface{})
	results := result["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	hit := results[0].(map[string]interface{})
	if hit["relation_source"] != "internal/alpha/service.go" || hit["relation_target"] != "internal/beta/service.go" {
		t.Fatalf("hit = %#v, want additive relation source+target filter result", hit)
	}
}

func TestCLIToolQueryInvalidEmptyQueryReturnsJSONError(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	out, code := runOKFExpectExit(t, bin, 1, "tool", "query", "--repo", repo, "--json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	env := decodeToolEnvelope(t, out)
	if env["ok"] != false || env["operation"] != "query" {
		t.Fatalf("error envelope = %#v", env)
	}
	errObj := env["error"].(map[string]interface{})
	if errObj["code"] != "invalid_query" {
		t.Fatalf("error = %#v, want invalid_query", errObj)
	}
	assertToolTopLevelFields(t, env)
}

func TestCLIToolQueryFlagParseErrorReturnsJSONEnvelope(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	out, code := runOKFExpectExit(t, bin, 1, "tool", "query", "--repo", repo, "--limit", "nope", "--json", "--q", "Alpha")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	env := decodeToolEnvelope(t, out)
	if env["ok"] != false || env["operation"] != "query" {
		t.Fatalf("error envelope = %#v", env)
	}
	errObj := env["error"].(map[string]interface{})
	if errObj["code"] != "invalid_request" {
		t.Fatalf("error = %#v, want invalid_request", errObj)
	}
	assertToolTopLevelFields(t, env)
}

func TestCLIToolQueryUnknownFlagBeforeJSONStillReturnsJSONEnvelope(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	out, code := runOKFExpectExit(t, bin, 1, "tool", "query", "--repo", repo, "--unknown-flag", "--json", "--q", "Alpha")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	env := decodeToolEnvelope(t, out)
	if env["ok"] != false || env["operation"] != "query" {
		t.Fatalf("error envelope = %#v", env)
	}
	errObj := env["error"].(map[string]interface{})
	if errObj["code"] != "invalid_request" {
		t.Fatalf("error = %#v, want invalid_request", errObj)
	}
	assertToolTopLevelFields(t, env)
}

func TestCLIToolContextPassesRelationsAndTraceFlags(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	mustWriteCLIFile(t, filepath.Join(repo, "internal", "alpha", "service.go"), "package alpha\nfunc AlphaSymbol() {}\n")
	mustWriteCLIFile(t, filepath.Join(repo, "internal", "beta", "service.go"), "package beta\nfunc BetaNeighbor() {}\n")
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha.md"), `---
type: code_symbol
title: AlphaSymbol
description: AlphaSymbol primary
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
symbol_kind: function
qualified_name: alpha.AlphaSymbol
start_line: 2
end_line: 2
---
AlphaSymbol primary.
`)
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "beta.md"), `---
type: code_symbol
title: BetaNeighbor
description: Beta neighbor reached through relation only
resource: code://repo/internal/beta/service.go
source_path: internal/beta/service.go
symbol_kind: function
qualified_name: beta.BetaNeighbor
start_line: 2
end_line: 2
---
Beta neighbor reached through relation only.
`)
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "code", "alpha-imports-beta.md"), `---
type: code_relation
title: Alpha imports Beta
description: Alpha to Beta relation
resource: code://repo/internal/alpha/service.go
source_path: internal/alpha/service.go
relation_kind: file_import
relation_source: internal/alpha/service.go
relation_target: internal/beta/service.go
start_line: 2
end_line: 2
---
Alpha imports Beta.
`)

	out := runOKF(t, bin, "tool", "context", "--repo", repo, "--q", "AlphaSymbol", "--budget-tokens", "200", "--include-relations", "--include-trace", "--json")
	env := decodeToolEnvelope(t, out)
	if env["operation"] != "context" || env["ok"] != true || env["mutating"] != false {
		t.Fatalf("tool context envelope = %#v", env)
	}
	result := env["result"].(map[string]interface{})
	if _, ok := result["trace"].([]interface{}); !ok {
		t.Fatalf("result = %#v, want trace when --include-trace is set", result)
	}
	items := result["items"].([]interface{})
	if !hasCLIContextItem(items, "internal/beta/service.go") {
		t.Fatalf("items = %#v, want relation-expanded beta neighbor", items)
	}
}

func TestCLIToolEnvelopesKeepV1TopLevelFields(t *testing.T) {
	bin := buildOKF(t)
	repo := initCLIRepo(t)
	mustWriteCLIFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "alpha.md"), `---
type: concept
title: Alpha Service
---
AlphaToken content.
`)

	statusEnv := decodeToolEnvelope(t, runOKF(t, bin, "tool", "status", "--repo", repo, "--json"))
	assertToolTopLevelFields(t, statusEnv)

	queryEnv := decodeToolEnvelope(t, runOKF(t, bin, "tool", "query", "--repo", repo, "--q", "AlphaToken", "--json"))
	assertToolTopLevelFields(t, queryEnv)

	contextEnv := decodeToolEnvelope(t, runOKF(t, bin, "tool", "context", "--repo", repo, "--q", "AlphaToken", "--json"))
	assertToolTopLevelFields(t, contextEnv)
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

func runOKFExpectExit(t *testing.T, bin string, wantCode int, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		if wantCode != 0 {
			t.Fatalf("okf %v succeeded, want exit %d\n%s", args, wantCode, out)
		}
		return string(out), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		if code != wantCode {
			t.Fatalf("okf %v exit = %d, want %d\n%s", args, code, wantCode, out)
		}
		return string(out), code
	}
	t.Fatalf("okf %v failed without exit code: %v\n%s", args, err, out)
	return string(out), -1
}

func runOKFWithEnv(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
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

func writeCLITestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
		t.Fatalf("mkdir zip dir: %v", err)
	}
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer zipFile.Close()
	writer := zip.NewWriter(zipFile)
	defer writer.Close()
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
}

func decodeToolEnvelope(t *testing.T, out string) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode JSON %q: %v", out, err)
	}
	return env
}

func hasCLIContextItem(items []interface{}, sourcePath string) bool {
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if ok && item["source_path"] == sourcePath {
			return true
		}
	}
	return false
}

func assertToolTopLevelFields(t *testing.T, env map[string]interface{}) {
	t.Helper()
	for _, key := range []string{"schema_version", "operation", "ok", "mutating", "repo_root", "knowledge_dir", "freshness", "warnings"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("envelope missing top-level field %q: %#v", key, env)
		}
	}
}
