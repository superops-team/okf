package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	toolsvc "github.com/superops-team/okf/pkg/tool"
)

func TestAgentFacingToolsAreRegistered(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))

	got := make(map[string]bool)
	for _, definition := range registry.List() {
		got[definition.Name] = true
	}
	for _, name := range []string{
		"okf_status",
		"okf_init",
		"okf_refresh",
		"okf_query",
		"okf_context",
		"okf_resolve",
		"okf_ask",
	} {
		if !got[name] {
			t.Errorf("agent-facing tool %q is not registered", name)
		}
	}
}

func TestMCPInitKnowledgeDirResolution(t *testing.T) {
	tests := []struct {
		name         string
		knowledgeDir func(t *testing.T, repo string) string
		wantDir      func(t *testing.T, repo, configured string) string
	}{
		{
			name: "absolute",
			knowledgeDir: func(t *testing.T, _ string) string {
				return filepath.Join(t.TempDir(), "absolute-knowledge")
			},
			wantDir: func(_ *testing.T, _, configured string) string { return configured },
		},
		{
			name:         "relative",
			knowledgeDir: func(_ *testing.T, _ string) string { return filepath.Join("custom", "knowledge") },
			wantDir:      func(_ *testing.T, repoRoot, configured string) string { return filepath.Join(repoRoot, configured) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initMCPToolTestRepo(t)
			configured := tt.knowledgeDir(t, repo)
			registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo, KnowledgeDir: configured}))
			result, err := registry.Call("okf_init", map[string]interface{}{})
			if err != nil {
				t.Fatal(err)
			}
			var envelope toolsvc.ToolEnvelope
			if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
				t.Fatal(err)
			}
			if !envelope.OK {
				t.Fatalf("init failed: %s", result.Content[0].Text)
			}
			want := filepath.Clean(tt.wantDir(t, envelope.RepoRoot, configured))
			if filepath.Clean(envelope.KnowledgeDir) != want {
				t.Fatalf("knowledge dir = %q, want %q", envelope.KnowledgeDir, want)
			}
			if _, err := os.Stat(want); err != nil {
				t.Fatalf("stat knowledge dir: %v", err)
			}
			if filepath.IsAbs(configured) {
				wrong := filepath.Join(repo, strings.TrimPrefix(configured, string(filepath.Separator)))
				if _, err := os.Stat(wrong); !os.IsNotExist(err) {
					t.Fatalf("absolute dir was also incorrectly joined under repo: %s", wrong)
				}
			}
		})
	}
}

func TestAgentFacingToolAnnotationsDeclareSemantics(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	tools := make(map[string]Tool)
	for _, definition := range registry.List() {
		tools[definition.Name] = definition
	}

	for _, name := range []string{"okf_status", "okf_query", "okf_context", "okf_ask"} {
		annotations := tools[name].Annotations
		if annotations == nil || !annotations.ReadOnlyHint || !annotations.IdempotentHint || annotations.OpenWorldHint {
			t.Errorf("%s annotations = %#v, want read-only/idempotent/closed-world", name, annotations)
		}
	}
	for _, name := range []string{"okf_init", "okf_refresh", "okf_note", "okf_log", "okf_feedback"} {
		annotations := tools[name].Annotations
		if annotations == nil || annotations.ReadOnlyHint || !annotations.IdempotentHint || annotations.DestructiveHint || annotations.OpenWorldHint {
			t.Errorf("%s annotations = %#v, want mutating/non-destructive/idempotent/closed-world", name, annotations)
		}
	}
}

func TestServerConfigWiresAgentFacingServiceTools(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	server := NewServer(ServerConfig{RepoPath: repo, KnowledgeDir: ".okf/knowledge"})

	got := make(map[string]bool)
	for _, definition := range server.tools.List() {
		got[definition.Name] = true
	}
	for _, name := range []string{"okf_status", "okf_init", "okf_refresh", "okf_query", "okf_context", "okf_ask"} {
		if !got[name] {
			t.Errorf("server tool %q is not registered", name)
		}
	}
}

func TestMCPWriteToolsPersistAndAskByProject(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))

	writeCases := []struct {
		tool string
		args map[string]interface{}
	}{
		{tool: "okf_note", args: map[string]interface{}{
			"content": "Remember the Alpha deployment rule.", "project": "alpha", "tags": []interface{}{"ops"}, "idempotency_key": "note-alpha-v1",
		}},
		{tool: "okf_log", args: map[string]interface{}{
			"content": "Beta deployment completed.", "project": "beta", "idempotency_key": "event-beta-v1",
		}},
		{tool: "okf_feedback", args: map[string]interface{}{
			"principle": "Alpha prefers fail-closed parsing.", "category": "verification", "project": "alpha", "evidence_refs": []interface{}{"spec.md#decision"}, "idempotency_key": "feedback-alpha-v1",
		}},
	}
	for _, tt := range writeCases {
		result, err := registry.Call(tt.tool, tt.args)
		if err != nil {
			t.Fatalf("%s: %v", tt.tool, err)
		}
		var envelope toolsvc.ToolEnvelope
		if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
			t.Fatal(err)
		}
		if !envelope.OK || !envelope.Mutating {
			t.Fatalf("%s envelope = %#v", tt.tool, envelope)
		}
	}

	restarted := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := restarted.Call("okf_ask", map[string]interface{}{"query": "Alpha", "project": "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK     bool `json:"ok"`
		Result struct {
			Results []struct {
				Type string `json:"type"`
			} `json:"results"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || len(envelope.Result.Results) != 2 {
		t.Fatalf("ask envelope = %s, want alpha note and feedback", result.Content[0].Text)
	}
	for _, hit := range envelope.Result.Results {
		if hit.Type != "note" && hit.Type != "feedback" {
			t.Fatalf("unexpected ask hit type %q", hit.Type)
		}
	}
}

func TestMCPWriteRejectsUnknownFields(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := registry.Call("okf_note", map[string]interface{}{
		"content": "valid", "idempotency_key": "note-v1", "repo": "/tmp/escape",
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope toolsvc.ToolEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.Code != toolsvc.ErrInvalidRequest {
		t.Fatalf("envelope = %#v, want invalid_request", envelope)
	}
	if _, err := os.Stat(filepath.Join(repo, ".okf", "knowledge")); !os.IsNotExist(err) {
		t.Fatalf("invalid write created knowledge dir: %v", err)
	}
}

func TestMCPWriteRejectsInvalidFieldTypes(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]interface{}
	}{
		{name: "content", tool: "okf_note", args: map[string]interface{}{"content": float64(1), "idempotency_key": "k"}},
		{name: "project", tool: "okf_note", args: map[string]interface{}{"content": "valid", "project": true, "idempotency_key": "k"}},
		{name: "tags non array", tool: "okf_note", args: map[string]interface{}{"content": "valid", "tags": "ops", "idempotency_key": "k"}},
		{name: "tags non string item", tool: "okf_note", args: map[string]interface{}{"content": "valid", "tags": []interface{}{"ops", float64(1)}, "idempotency_key": "k"}},
		{name: "metadata", tool: "okf_note", args: map[string]interface{}{"content": "valid", "metadata": []interface{}{}, "idempotency_key": "k"}},
		{name: "idempotency key", tool: "okf_note", args: map[string]interface{}{"content": "valid", "idempotency_key": true}},
		{name: "principle", tool: "okf_feedback", args: map[string]interface{}{"principle": float64(1), "category": "quality", "idempotency_key": "k"}},
		{name: "category", tool: "okf_feedback", args: map[string]interface{}{"principle": "valid", "category": float64(1), "idempotency_key": "k"}},
		{name: "evidence refs", tool: "okf_feedback", args: map[string]interface{}{"principle": "valid", "category": "quality", "evidence_refs": []interface{}{"spec", true}, "idempotency_key": "k"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initMCPToolTestRepo(t)
			registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
			result, err := registry.Call(tt.tool, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			var envelope toolsvc.ToolEnvelope
			if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.OK || envelope.Error == nil || envelope.Error.Code != toolsvc.ErrInvalidRequest {
				t.Fatalf("envelope = %#v, want invalid_request", envelope)
			}
			if _, err := os.Stat(filepath.Join(repo, ".okf", "knowledge")); !os.IsNotExist(err) {
				t.Fatalf("invalid write created knowledge dir: %v", err)
			}
		})
	}
}

func TestMCPWriteRejectsSymlinkKnowledgeRoot(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	outside := t.TempDir()
	link := filepath.Join(repo, "linked-knowledge")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo, KnowledgeDir: link}))
	result, err := registry.Call("okf_note", map[string]interface{}{
		"content": "must not escape", "idempotency_key": "escape-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope toolsvc.ToolEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.Code != toolsvc.ErrKnowledgePathOutside {
		t.Fatalf("envelope = %#v, want path_outside_root", envelope)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory contains %d entries, want none", len(entries))
	}
}

func TestMCPQueryMatchesServiceEnvelope(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	mustWriteMCPConcept(t, repo, "concepts/alpha.md", `---
type: concept
title: Alpha Service
description: AlphaSymbol routing
---
AlphaSymbol owns routing.
`)
	service := toolsvc.NewService(toolsvc.Config{RepoPath: repo})
	registry := NewToolRegistryWithService(service)
	args := map[string]interface{}{"query": "AlphaSymbol", "limit": float64(5), "include_trace": true}

	result, err := registry.Call("okf_query", args)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(service.Query(t.Context(), toolsvc.QueryRequest{Query: "AlphaSymbol", Limit: 5, IncludeTrace: true}))
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Content[0].Text; got != string(want) {
		t.Fatalf("MCP query envelope differs from Service.Query\n got: %s\nwant: %s", got, want)
	}
}

func TestMCPContextHonorsBudget(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	mustWriteMCPConcept(t, repo, "concepts/alpha.md", `---
type: concept
title: Alpha Service
description: AlphaSymbol routing
---
AlphaSymbol owns routing and provides deterministic context.
`)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := registry.Call("okf_context", map[string]interface{}{"query": "AlphaSymbol", "budget_tokens": float64(12)})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK     bool `json:"ok"`
		Result struct {
			BudgetTokens int `json:"budget_tokens"`
			UsedTokens   int `json:"used_tokens"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("context failed: %s", result.Content[0].Text)
	}
	if envelope.Result.UsedTokens > envelope.Result.BudgetTokens {
		t.Fatalf("used tokens %d exceed budget %d", envelope.Result.UsedTokens, envelope.Result.BudgetTokens)
	}
}

func TestMCPAskReturnsOnlyDurableKnowledgeTypes(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	mustWriteMCPConcept(t, repo, "concepts/code.md", `---
type: code_symbol
title: Alpha implementation
description: shared needle
---
shared needle
`)
	mustWriteMCPConcept(t, repo, "notes/note.md", `---
type: note
title: Alpha note
description: shared needle
---
shared needle
`)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := registry.Call("okf_ask", map[string]interface{}{"query": "shared needle"})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Result struct {
			Results []struct {
				Type string `json:"type"`
			} `json:"results"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Result.Results) != 1 || envelope.Result.Results[0].Type != "note" {
		t.Fatalf("ask results = %#v, want only note/event/feedback", envelope.Result.Results)
	}
}

func TestMCPAskHonorsLimitAndDeterministicOrder(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	for _, fixture := range []struct {
		path    string
		title   string
		project string
	}{
		{path: "notes/zulu.md", title: "Zulu", project: "alpha"},
		{path: "notes/alpha.md", title: "Alpha", project: "alpha"},
		{path: "notes/mike.md", title: "Mike", project: "alpha"},
	} {
		mustWriteMCPConcept(t, repo, fixture.path, fmt.Sprintf(`---
type: note
title: %s
description: shared deterministic needle
project: %s
---
shared deterministic needle
`, fixture.title, fixture.project))
	}
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	call := func() []string {
		t.Helper()
		result, err := registry.Call("okf_ask", map[string]interface{}{
			"query": "shared deterministic needle", "project": "alpha", "limit": float64(2),
		})
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			OK     bool `json:"ok"`
			Result struct {
				Results []struct {
					Ref string `json:"ref"`
				} `json:"results"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
			t.Fatal(err)
		}
		if !envelope.OK {
			t.Fatalf("ask failed: %s", result.Content[0].Text)
		}
		refs := make([]string, 0, len(envelope.Result.Results))
		for _, hit := range envelope.Result.Results {
			refs = append(refs, hit.Ref)
		}
		return refs
	}

	first := call()
	second := call()
	if len(first) != 2 {
		t.Fatalf("ask returned %d results, want limit 2", len(first))
	}
	if !slices.Equal(first, second) {
		t.Fatalf("ask order is not deterministic: first=%v second=%v", first, second)
	}
}

func mustWriteMCPConcept(t *testing.T, repo, relativePath, content string) {
	t.Helper()
	path := filepath.Join(repo, ".okf", "knowledge", relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMCPStatusUsesServiceWithoutInitializingKnowledge(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	knowledgeDir := filepath.Join(repo, ".okf", "knowledge")
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))

	result, err := registry.Call("okf_status", map[string]interface{}{})
	if err != nil {
		t.Fatalf("call okf_status: %v", err)
	}
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("result = %#v, want one JSON content item", result)
	}
	if result.IsError {
		t.Fatalf("transport result is error: %s", result.Content[0].Text)
	}

	var envelope toolsvc.ToolEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatalf("decode service envelope: %v\n%s", err, result.Content[0].Text)
	}
	if envelope.SchemaVersion != toolsvc.SchemaVersion {
		t.Fatalf("schema version = %q, want %q", envelope.SchemaVersion, toolsvc.SchemaVersion)
	}
	if envelope.Operation != toolsvc.OperationStatus {
		t.Fatalf("operation = %q, want %q", envelope.Operation, toolsvc.OperationStatus)
	}
	if envelope.OK {
		t.Fatal("status OK = true, want false before initialization")
	}
	if envelope.Mutating {
		t.Fatal("status mutating = true, want false")
	}
	if envelope.Error == nil || envelope.Error.Code != toolsvc.ErrKnowledgeNotInitialized {
		t.Fatalf("error = %#v, want %q", envelope.Error, toolsvc.ErrKnowledgeNotInitialized)
	}
	if _, err := os.Stat(knowledgeDir); !os.IsNotExist(err) {
		t.Fatalf("status created knowledge dir %q: %v", knowledgeDir, err)
	}
}

func initMCPToolTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	cmd := exec.Command("git", "init", "-q", repo)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	cmd = exec.Command("git", "-C", repo, "config", "user.email", "okf-test@example.invalid")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email: %v: %s", err, output)
	}
	cmd = exec.Command("git", "-C", repo, "config", "user.name", "OKF Test")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name: %v: %s", err, output)
	}
	readme := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readme, []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", repo, "add", "README.md")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	cmd = exec.Command("git", "-C", repo, "commit", "-qm", "fixture")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}
	return repo
}

// S11: the MCP okf_resolve handler resolves a stable okf://concept/<id> ref to
// the concept's current path after a file move.
func TestMCPResolveSurvivesMove(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	const okfID = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	oldRel := "notes/original.md"
	newRel := "notes/renamed.md"
	mustWriteMCPConcept(t, repo, oldRel, "---\ntype: note\ntitle: Original\nokf_id: "+okfID+"\n---\nbody\n")

	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	uri := "okf://concept/" + okfID

	callResolve := func(ref string) (map[string]any, *ToolCallResult) {
		result, err := registry.Call("okf_resolve", map[string]interface{}{"ref": ref})
		if err != nil {
			t.Fatalf("call okf_resolve: %v", err)
		}
		var env map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].Text), &env); err != nil {
			t.Fatalf("unmarshal envelope: %v\n%s", err, result.Content[0].Text)
		}
		return env, result
	}

	// Before move.
	env, _ := callResolve(uri)
	if ok, _ := env["ok"].(bool); !ok {
		t.Fatalf("resolve before move not ok: %v", env)
	}
	result, _ := env["result"].(map[string]any)
	if got, _ := result["path"].(string); got != oldRel {
		t.Fatalf("before move: path = %q, want %q", got, oldRel)
	}

	// Move the file.
	kbDir := filepath.Join(repo, ".okf", "knowledge")
	if err := os.Rename(filepath.Join(kbDir, filepath.FromSlash(oldRel)), filepath.Join(kbDir, filepath.FromSlash(newRel))); err != nil {
		t.Fatal(err)
	}
	mustWriteMCPConcept(t, repo, newRel, "---\ntype: note\ntitle: Renamed\nokf_id: "+okfID+"\n---\nbody\n")

	// After move: same ref, new path.
	env, _ = callResolve(uri)
	if ok, _ := env["ok"].(bool); !ok {
		t.Fatalf("resolve after move not ok: %v", env)
	}
	result, _ = env["result"].(map[string]any)
	if got, _ := result["path"].(string); got != newRel {
		t.Fatalf("after move: path = %q, want %q", got, newRel)
	}

	// Unknown valid-syntax ref -> concept_ref_not_found.
	env, _ = callResolve("okf://concept/okf_ffffffffffffffffffffffffffffffff")
	if ok, _ := env["ok"].(bool); ok {
		t.Fatal("unknown ref should not be ok")
	}
	errObj, _ := env["error"].(map[string]any)
	if code, _ := errObj["code"].(string); code != "concept_ref_not_found" {
		t.Fatalf("unknown ref code = %q, want concept_ref_not_found", code)
	}
}

// S26: okf_manifest is registered with the additive okf.tool.v1 envelope schema.
func TestMCPManifestSchema(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	var found *Tool
	for _, definition := range registry.List() {
		if definition.Name == "okf_manifest" {
			found = &definition
		}
	}
	if found == nil {
		t.Fatal("okf_manifest is not registered")
	}
	props, _ := found.InputSchema["properties"].(map[string]interface{})
	for _, key := range []string{"offset", "limit", "types", "tags", "statuses", "stale", "folder_prefix", "include_trace"} {
		if _, ok := props[key]; !ok {
			t.Errorf("okf_manifest schema missing property %q", key)
		}
	}
}

// S26: MCP okf_manifest and Service.Manifest produce byte-identical envelopes
// for the same repo and request.
func TestMCPManifestParity(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	mustWriteMCPConcept(t, repo, "concepts/alpha.md", `---
type: concept
title: Alpha
okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e
---
Alpha body.
`)
	mustWriteMCPConcept(t, repo, "concepts/beta.md", `---
type: source
title: Beta
tags: [x]
---
Beta body.
`)
	service := toolsvc.NewService(toolsvc.Config{RepoPath: repo})
	registry := NewToolRegistryWithService(service)

	limit := 5
	args := map[string]interface{}{"limit": float64(limit), "types": []interface{}{"concept"}}
	result, err := registry.Call("okf_manifest", args)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(service.Manifest(t.Context(), toolsvc.ManifestRequest{Limit: &limit, Types: []string{"concept"}}))
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Content[0].Text; got != string(want) {
		t.Fatalf("MCP manifest envelope differs from Service.Manifest\n got: %s\nwant: %s", got, want)
	}
}

// groupingMCPFixture mirrors the Service grouping fixture: two concepts share
// src/doc1.md and a third lives in src/doc2.md.
func groupingMCPFixture(t *testing.T) string {
	t.Helper()
	repo := initMCPToolTestRepo(t)
	mustWriteMCPConcept(t, repo, "concepts/one.md", "---\ntype: concept\ntitle: Alpha Doc One\ndescription: ParityToken first half of doc1\nsource_path: src/doc1.md\n---\nParityToken body alpha one.\n")
	mustWriteMCPConcept(t, repo, "concepts/two.md", "---\ntype: concept\ntitle: Alpha Doc Two\ndescription: ParityToken second half of doc1\nsource_path: src/doc1.md\n---\nParityToken body alpha two.\n")
	mustWriteMCPConcept(t, repo, "concepts/three.md", "---\ntype: concept\ntitle: Beta Doc\ndescription: ParityToken lives in doc2\nsource_path: src/doc2.md\n---\nParityToken body beta.\n")
	return repo
}

// TestMCPGroupingOmittedHasNoGroups (S27): without group_by the okf_query
// envelope carries no groups field.
func TestMCPGroupingOmittedHasNoGroups(t *testing.T) {
	repo := groupingMCPFixture(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	result, err := registry.Call("okf_query", map[string]interface{}{"query": "ParityToken", "limit": float64(5)})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Result struct {
			Groups json.RawMessage `json:"groups"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Result.Groups) != 0 {
		t.Fatalf("groups present when omitted: %s", envelope.Result.Groups)
	}
}

// TestMCPGroupingMatchesService (S34): okf_query with group_by produces an
// envelope byte-identical to the direct Service.Query result, proving MCP and
// Service share the grouped projection semantics.
func TestMCPGroupingMatchesService(t *testing.T) {
	repo := groupingMCPFixture(t)
	service := toolsvc.NewService(toolsvc.Config{RepoPath: repo})
	registry := NewToolRegistryWithService(service)

	args := map[string]interface{}{
		"query": "ParityToken", "limit": float64(5),
		"group_by": "source", "include_group_members": true,
	}
	result, err := registry.Call("okf_query", args)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(service.Query(t.Context(), toolsvc.QueryRequest{
		Query: "ParityToken", Limit: 5, GroupBy: "source", IncludeGroupMembers: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Content[0].Text; got != string(want) {
		t.Fatalf("MCP grouped envelope differs from Service\n got: %s\nwant: %s", got, want)
	}

	// And the payload actually contains two groups with the expected counts.
	var envelope struct {
		Result struct {
			Groups []struct {
				GroupKey    string `json:"group_key"`
				HitCount    int    `json:"hit_count"`
				SourceCount int    `json:"source_count"`
			} `json:"groups"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Result.Groups) != 2 {
		t.Fatalf("MCP groups = %d, want 2", len(envelope.Result.Groups))
	}
	if envelope.Result.Groups[0].GroupKey != "src:src/doc1.md" || envelope.Result.Groups[0].HitCount != 2 {
		t.Fatalf("MCP group0 = %+v", envelope.Result.Groups[0])
	}
}
