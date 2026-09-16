package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// S05/S39: new manifest flags parse and flow through the JSON envelope.
func TestToolManifestGovernedFlags(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	mustWriteKBFile(t, repo, "concepts/h.md", `---
type: decision
title: Hold Rule
governance: hold
code_refs:
  - pkg/x.go
---
hold body
`)
	mustWriteKBFile(t, repo, "concepts/c.md", `---
type: decision
title: Constraint Rule
governance: constraint
code_refs:
  - pkg/x.go
---
constraint body
`)

	var code int
	var parsed map[string]any
	out := captureStdout(t, func() {
		code = cmdTool([]string{"manifest", "--repo", repo,
			"--for-path", "pkg/x.go", "--governance", "hold,constraint",
			"--mode", "summary", "--max-tokens", "1000", "--stale-refs", "--json"})
	})
	if code != 0 {
		t.Fatalf("exit = %d, output = %s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	result, _ := parsed["result"].(map[string]any)
	items, _ := result["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2: %s", len(items), out)
	}
	first, _ := items[0].(map[string]any)
	if first["governance"] != "hold" {
		t.Fatalf("first item governance = %v, want hold", first["governance"])
	}
	if result["governance_warning"] != true {
		t.Fatalf("governance_warning = %v", result["governance_warning"])
	}
	// Summary projection carries no extra data fields beyond okf_id/title/type/
	// governance/description (S28).
	if _, ok := first["tags"]; ok {
		t.Fatalf("summary mode must drop tags: %v", first)
	}
	if _, ok := first["code_refs"]; ok {
		t.Fatalf("summary mode must drop code_refs: %v", first)
	}
	if _, ok := first["ref"]; ok {
		t.Fatalf("summary mode must drop ref: %v", first)
	}
	if p, _ := first["path"].(string); p != "" {
		t.Fatalf("summary mode must not surface path data: %q", p)
	}
}

// S39: okf tool context --refs parses comma-separated refs.
func TestToolContextRefsFlag(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	mustWriteKBFile(t, repo, "concepts/note.md", `---
type: note
title: Ref Note
okf_id: `+id+`
---
Ref note body content.
`)
	var code int
	var parsed map[string]any
	out := captureStdout(t, func() {
		code = cmdTool([]string{"context", "--repo", repo, "--refs", id, "--json"})
	})
	if code != 0 {
		t.Fatalf("exit = %d, output = %s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	result, _ := parsed["result"].(map[string]any)
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1: %s", len(items), out)
	}
	first, _ := items[0].(map[string]any)
	if body, _ := first["snippet"].(string); body == "" {
		t.Fatalf("ref item snippet empty: %v", first)
	}
}

func mustWriteKBFile(t *testing.T, repo, relPath, body string) {
	t.Helper()
	path := filepath.Join(repo, ".okf", "knowledge", filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
