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

// memoryCheckEnvelope runs `okf tool query` with the given args and returns the
// parsed envelope plus the decoded "result" map for memory_check assertions.
func memoryCheckEnvelope(t *testing.T, repo string, extra ...string) (int, map[string]any, map[string]any) {
	t.Helper()
	args := append([]string{"query", "--repo", repo, "-q", "durable note body about caching policy", "--memory-check", "--json"}, extra...)
	var code int
	out := captureStdout(t, func() {
		code = cmdTool(args)
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	result, _ := parsed["result"].(map[string]any)
	return code, parsed, result
}

// S20-S27: --memory-check returns a dedicated MemoryCheckResult envelope
// (status/candidates) and never mixes in the normal ranked query output.
func TestToolQueryMemoryCheckFlags(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	mustWriteKBFile(t, repo, "concepts/note.md", `---
type: note
title: Caching Note
okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e
---
A durable note about caching policy and eviction.
`)
	code, _, result := memoryCheckEnvelope(t, repo)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, ok := result["status"]; !ok {
		t.Fatalf("memory_check result missing status: %v", result)
	}
	if _, ok := result["candidates"]; !ok {
		t.Fatalf("memory_check result missing candidates: %v", result)
	}
	// Dedicated envelope: no ranked query shape mixed in.
	if _, ok := result["results"]; ok {
		t.Fatalf("memory_check must not return ranked 'results': %v", result)
	}
	if _, ok := result["hits"]; ok {
		t.Fatalf("memory_check must not return 'hits': %v", result)
	}
	// candidate_types defaults to the durable note/event/feedback set.
	types, _ := result["candidate_types"].([]any)
	if len(types) == 0 {
		t.Fatalf("candidate_types empty: %v", result)
	}
}

// S20: --memory-check still requires a non-empty -q.
func TestToolQueryMemoryCheckEmptyQ(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	var code int
	out := captureStdout(t, func() {
		code = cmdTool([]string{"query", "--repo", repo, "-q", "", "--memory-check", "--json"})
	})
	if code == 0 {
		t.Fatalf("empty -q must fail, output = %s", out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if parsed["ok"] != false {
		t.Fatalf("envelope.ok must be false: %v", parsed)
	}
	errObj, _ := parsed["error"].(map[string]any)
	if errObj["code"] != "invalid_query" {
		t.Fatalf("error.code = %v, want invalid_query", errObj)
	}
}

// S20: --type note restricts the candidate set to a single durable type.
func TestToolQueryMemoryCheckTypeSingular(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	mustWriteKBFile(t, repo, "concepts/note.md", `---
type: note
title: Singular Note
---
Body for the singular type filter test.
`)
	_, _, result := memoryCheckEnvelope(t, repo, "--type", "note")
	types, _ := result["candidate_types"].([]any)
	if len(types) != 1 || types[0] != "note" {
		t.Fatalf("candidate_types = %v, want [note]", types)
	}
}

// S20: --dup-threshold overrides the default Jaccard threshold (0.20).
func TestToolQueryMemoryCheckDupThreshold(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	mustWriteKBFile(t, repo, "concepts/note.md", `---
type: note
title: Threshold Note
---
Body used only to observe the echoed threshold.
`)
	_, _, result := memoryCheckEnvelope(t, repo, "--dup-threshold", "0.5")
	got, _ := result["threshold"].(float64)
	if got != 0.5 {
		t.Fatalf("threshold = %v, want 0.5", result["threshold"])
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
