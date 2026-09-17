package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// S20-S27: okf_query schema exposes the governed memory_check knobs and keeps
// the single-string "type" filter (no plural "types" on the query tool).
func TestMCPQueryMemoryCheckSchema(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	var found *Tool
	for _, definition := range registry.List() {
		if definition.Name == "okf_query" {
			found = &definition
		}
	}
	if found == nil {
		t.Fatal("okf_query not registered")
	}
	props, _ := found.InputSchema["properties"].(map[string]interface{})
	for _, key := range []string{"memory_check", "dup_threshold"} {
		if _, ok := props[key]; !ok {
			t.Errorf("okf_query schema missing property %q", key)
		}
	}
	if _, ok := props["types"]; ok {
		t.Errorf("okf_query schema must not expose a plural 'types' property")
	}
	if _, ok := props["type"]; !ok {
		t.Errorf("okf_query schema should keep the singular 'type' filter")
	}
}

// knowledgeSizes snapshots the knowledge tree as relative-path -> size so a
// read-only tool call can be proven to touch nothing on disk.
func knowledgeSizes(t *testing.T, root string) map[string]int64 {
	t.Helper()
	sizes := map[string]int64{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		info, _ := d.Info()
		sizes[filepath.ToSlash(rel)] = info.Size()
		return nil
	})
	return sizes
}

// S20-S27: okf_query with memory_check=true returns the dedicated envelope and
// is strictly read-only (Mutating=false, zero on-disk side effects).
func TestMCPQueryMemoryCheckHandler(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	mustWriteMCPConcept(t, repo, "concepts/note.md", `---
type: note
title: Durable Note
---
Body describing caching eviction policy in detail.
`)
	knowledgeRoot := filepath.Join(repo, ".okf", "knowledge")
	before := knowledgeSizes(t, knowledgeRoot)

	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := registry.Call("okf_query", map[string]interface{}{
		"query":        "caching eviction policy",
		"memory_check": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope toolsvc.ToolEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("memory_check not ok: %s", result.Content[0].Text)
	}
	if envelope.Mutating {
		t.Fatal("memory_check must be read-only (Mutating=false)")
	}
	data, err := json.Marshal(envelope.Result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["status"]; !ok {
		t.Fatalf("dedicated memory_check result missing status: %v", decoded)
	}
	if _, ok := decoded["candidates"]; !ok {
		t.Fatalf("dedicated memory_check result missing candidates: %v", decoded)
	}
	if _, ok := decoded["results"]; ok {
		t.Fatalf("memory_check must not mix in ranked 'results': %v", decoded)
	}

	after := knowledgeSizes(t, knowledgeRoot)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("memory_check modified the knowledge tree:\nbefore=%v\nafter=%v", before, after)
	}
}

// toolNamesFromList extracts the tool names from a tools/list response.
func toolNamesFromList(t *testing.T, resp map[string]any) []string {
	t.Helper()
	result, _ := resp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		tm, _ := tl.(map[string]any)
		if name, ok := tm["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

// toolSchemaFromList extracts the inputSchema of a named tool from a tools/list
// response, failing the test when the tool is absent.
func toolSchemaFromList(t *testing.T, resp map[string]any, name string) map[string]any {
	t.Helper()
	result, _ := resp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	for _, tl := range tools {
		tm, _ := tl.(map[string]any)
		if tm["name"] == name {
			schema, _ := tm["inputSchema"].(map[string]any)
			return schema
		}
	}
	t.Fatalf("tool %q not present in tools/list response", name)
	return nil
}

// S16/S20: okf_query is registered once and shared by both the modern and
// legacy eras; its schema (including the memory_check knobs) is identical.
func TestMCPQuerySharedAcrossEras(t *testing.T) {
	// Modern era: tools/list with _meta.
	mod := newTestServer(t)
	mod.feed(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":` + modernMeta() + `}`)
	modResp := mod.lastResponse()
	if modResp["error"] != nil {
		t.Fatalf("modern tools/list error: %v", modResp["error"])
	}
	modNames := toolNamesFromList(t, modResp)
	if !slices.Contains(modNames, "okf_query") {
		t.Fatalf("modern era tools/list missing okf_query: %v", modNames)
	}
	modSchema := toolSchemaFromList(t, modResp, "okf_query")

	// Legacy era: initialize first, then tools/list.
	leg := newTestServer(t)
	leg.feed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}`)
	leg.feed(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	legResp := leg.lastResponse()
	if legResp["error"] != nil {
		t.Fatalf("legacy tools/list error: %v", legResp["error"])
	}
	legNames := toolNamesFromList(t, legResp)
	if !slices.Contains(legNames, "okf_query") {
		t.Fatalf("legacy era tools/list missing okf_query: %v", legNames)
	}
	legSchema := toolSchemaFromList(t, legResp, "okf_query")

	// Same single registration => identical schema in both eras.
	if !reflect.DeepEqual(modSchema, legSchema) {
		t.Fatalf("okf_query schema differs across eras:\nmodern=%v\nlegacy=%v", modSchema, legSchema)
	}
	props, _ := modSchema["properties"].(map[string]any)
	for _, key := range []string{"memory_check", "dup_threshold"} {
		if _, ok := props[key]; !ok {
			t.Errorf("shared okf_query schema missing %q", key)
		}
	}
}
