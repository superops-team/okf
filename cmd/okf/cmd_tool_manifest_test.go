package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// S26: `okf tool manifest --json` emits the stable okf.tool.v1 envelope and the
// same semantic fields as the Service/ MCP contract.
func TestToolManifestJSON(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	writeCLIKBConcept(t, repo, "concepts/alpha.md", "Alpha")
	writeCLIKBConcept(t, repo, "concepts/beta.md", "Beta")

	var code int
	var parsed map[string]any
	out := captureStdout(t, func() {
		code = cmdTool([]string{"manifest", "--repo", repo, "--json"})
	})
	if code != 0 {
		t.Fatalf("exit = %d, output = %s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if parsed["schema_version"] != "okf.tool.v1" {
		t.Fatalf("schema_version = %v", parsed["schema_version"])
	}
	if parsed["operation"] != "manifest" {
		t.Fatalf("operation = %v", parsed["operation"])
	}
	if parsed["ok"] != true {
		t.Fatalf("ok = %v", parsed["ok"])
	}
	result, _ := parsed["result"].(map[string]any)
	items, _ := result["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["path"] != "concepts/alpha.md" {
		t.Fatalf("first path = %v, want concepts/alpha.md (ordering)", first["path"])
	}
	if _, ok := first["body"]; ok {
		t.Fatal("manifest item must never carry a body field")
	}
	if result["index_status"] != "missing" {
		t.Fatalf("index_status = %v", result["index_status"])
	}
}

// Explicit limit must be honored via JSON; an out-of-range limit fails closed.
func TestToolManifestJSONLimitValidation(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	writeCLIKBConcept(t, repo, "a.md", "A")

	var code int
	out := captureStdout(t, func() {
		code = cmdTool([]string{"manifest", "--repo", repo, "--limit", "999", "--json"})
	})
	if code == 0 {
		t.Fatalf("out-of-range limit should exit non-zero, output = %s", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("error output not JSON: %v\n%s", err, out)
	}
	errObj, _ := parsed["error"].(map[string]any)
	if errObj["code"] != "invalid_request" {
		t.Fatalf("error code = %v, want invalid_request", errObj["code"])
	}
}

func writeCLIKBConcept(t *testing.T, repo, relPath, title string) {
	t.Helper()
	path := filepath.Join(repo, ".okf", "knowledge", filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ntype: concept\ntitle: " + title + "\n---\nbody\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
