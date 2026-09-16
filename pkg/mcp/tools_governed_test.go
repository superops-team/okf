package mcp

import (
	"encoding/json"
	"testing"

	toolsvc "github.com/superops-team/okf/pkg/tool"
)

// S39 (P1): okf_manifest schema exposes the governed parameters in snake_case.
func TestMCPManifestGovernedSchema(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	var found *Tool
	for _, definition := range registry.List() {
		if definition.Name == "okf_manifest" {
			found = &definition
		}
	}
	if found == nil {
		t.Fatal("okf_manifest not registered")
	}
	props, _ := found.InputSchema["properties"].(map[string]interface{})
	for _, key := range []string{"for_path", "governance", "mode", "max_tokens", "stale_refs"} {
		if _, ok := props[key]; !ok {
			t.Errorf("okf_manifest schema missing property %q", key)
		}
	}
}

// S39 (P3): okf_context schema exposes refs.
func TestMCPContextRefsSchema(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	var found *Tool
	for _, definition := range registry.List() {
		if definition.Name == "okf_context" {
			found = &definition
		}
	}
	if found == nil {
		t.Fatal("okf_context not registered")
	}
	props, _ := found.InputSchema["properties"].(map[string]interface{})
	if _, ok := props["refs"]; !ok {
		t.Fatal("okf_context schema missing refs")
	}
}

// P1: okf_manifest honors for_path/mode through the MCP handler.
func TestMCPManifestGovernedHandler(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	mustWriteMCPConcept(t, repo, "concepts/h.md", `---
type: decision
title: Hold Rule
governance: hold
code_refs:
  - pkg/x.go
---
hold body
`)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := registry.Call("okf_manifest", map[string]interface{}{
		"for_path":   "pkg/x.go",
		"mode":       "summary",
		"max_tokens": float64(1000),
		"stale_refs": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope toolsvc.ToolEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("manifest not ok: %s", result.Content[0].Text)
	}
	data, err := json.Marshal(envelope.Result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	items, _ := decoded["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1: %s", len(items), data)
	}
	first, _ := items[0].(map[string]any)
	if first["governance"] != "hold" {
		t.Fatalf("governance = %v", first["governance"])
	}
	if decoded["governance_warning"] != true {
		t.Fatalf("governance_warning = %v", decoded["governance_warning"])
	}
}

// P3: okf_context resolves refs through the MCP handler.
func TestMCPContextRefsHandler(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	mustWriteMCPConcept(t, repo, "concepts/note.md", `---
type: note
title: Ref Note
okf_id: `+id+`
---
Ref note body content here.
`)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := registry.Call("okf_context", map[string]interface{}{
		"refs": []interface{}{id},
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope toolsvc.ToolEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("context not ok: %s", result.Content[0].Text)
	}
	data, _ := json.Marshal(envelope.Result)
	var decoded map[string]any
	json.Unmarshal(data, &decoded)
	items, _ := decoded["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1: %s", len(items), data)
	}
}
