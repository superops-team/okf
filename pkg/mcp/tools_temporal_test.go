package mcp

import (
	"encoding/json"
	"reflect"
	"testing"

	toolsvc "github.com/superops-team/okf/pkg/tool"
)

// temporalQueryKeys are the additive temporal knobs that okf_query must expose in
// every era (S37).
var temporalQueryKeys = []string{"memory_view", "refs", "memory_review_queue"}

// temporalWriteKeys are the additive temporal fields that the durable write tools
// (note/log/feedback) must expose in every era (S38). evidence_refs already
// existed on feedback; it is added to note/log here.
var temporalWriteKeys = []string{
	"evidence_refs", "memory_state", "memory_confidence", "memory_relation_kind", "memory_relation_targets",
}

// toolSchemaFromEra runs tools/list against one era and returns the inputSchema
// for the named tool. modern=true uses the 2026-07-28 discover era; false uses
// the 2024-11-05 legacy era.
func toolSchemaFromEra(t *testing.T, modern bool, name string) map[string]interface{} {
	t.Helper()
	s := newTestServer(t)
	if modern {
		s.feed(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":` + modernMeta() + `}`)
	} else {
		s.feed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}`)
		s.feed(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	}
	resp := s.lastResponse()
	if resp["error"] != nil {
		t.Fatalf("tools/list (modern=%v) error: %v", modern, resp["error"])
	}
	return toolSchemaFromList(t, resp, name)
}

// S37/S38: the temporal query/context/write schema is identical across the modern
// and legacy eras (one registration, two transports).
func TestMCPTemporalSchemaSharedAcrossEras(t *testing.T) {
	cases := []struct {
		tool string
		keys []string
	}{
		{tool: "okf_query", keys: temporalQueryKeys},
		{tool: "okf_context", keys: []string{"memory_view"}},
		{tool: "okf_note", keys: temporalWriteKeys},
		{tool: "okf_log", keys: temporalWriteKeys},
		{tool: "okf_feedback", keys: temporalWriteKeys},
	}
	for _, tc := range cases {
		modern := toolSchemaFromEra(t, true, tc.tool)
		legacy := toolSchemaFromEra(t, false, tc.tool)
		if !reflect.DeepEqual(modern, legacy) {
			t.Fatalf("%s schema differs across eras:\nmodern=%v\nlegacy=%v", tc.tool, modern, legacy)
		}
		props, _ := modern["properties"].(map[string]interface{})
		for _, key := range tc.keys {
			if _, ok := props[key]; !ok {
				t.Errorf("%s shared schema missing temporal key %q", tc.tool, key)
			}
		}
	}
}

// S37: okf_memory_review is registered in BOTH eras and is flagged mutating +
// destructive + non-idempotent (design §8.2), never read-only.
func TestMCPMemoryReviewToolPresentBothEras(t *testing.T) {
	for _, modern := range []bool{true, false} {
		s := newTestServer(t)
		if modern {
			s.feed(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":` + modernMeta() + `}`)
		} else {
			s.feed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}`)
			s.feed(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
		}
		resp := s.lastResponse()
		tools, _ := resp["result"].(map[string]any)["tools"].([]any)
		var found map[string]any
		for _, tl := range tools {
			tm, _ := tl.(map[string]any)
			if tm["name"] == "okf_memory_review" {
				found = tm
			}
		}
		if found == nil {
			t.Fatalf("okf_memory_review missing from era modern=%v: %v", modern, tools)
		}
		ann, _ := found["annotations"].(map[string]any)
		if ann == nil {
			t.Fatalf("okf_memory_review has no annotations in era modern=%v", modern)
		}
		if ro, _ := ann["readOnlyHint"].(bool); ro {
			t.Errorf("okf_memory_review must not be read-only (era modern=%v)", modern)
		}
		if dest, _ := ann["destructiveHint"].(bool); !dest {
			t.Errorf("okf_memory_review must be destructive (era modern=%v)", modern)
		}
		if idem, _ := ann["idempotentHint"].(bool); idem {
			t.Errorf("okf_memory_review must be non-idempotent (era modern=%v)", modern)
		}
		schema, _ := found["inputSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		for _, key := range []string{"ref", "action", "expected_state"} {
			if _, ok := props[key]; !ok {
				t.Errorf("okf_memory_review schema missing %q", key)
			}
		}
		required, _ := schema["required"].([]any)
		wantRequired := map[string]bool{"ref": true, "action": true, "expected_state": true}
		for _, r := range required {
			delete(wantRequired, r.(string))
		}
		if len(wantRequired) != 0 {
			t.Errorf("okf_memory_review required missing %v", wantRequired)
		}
	}
}

// S38: the new temporal write fields are accepted on okf_note and flow through
// the service; the resulting proposal then appears in the review queue.
func TestMCPTemporalWriteFieldsAccepted(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	initRegistryOK(t, registry)

	callWrite := registryCall(t, registry, "okf_note", map[string]interface{}{
		"content":           "Proposed reusable caching heuristic",
		"idempotency_key":   "temporal-note-1",
		"memory_state":      "proposed",
		"memory_confidence": float64(0.75),
		"evidence_refs":     []interface{}{"spec/caching.md"},
	})
	if !callWrite.OK {
		t.Fatalf("proposed write rejected: %v", callWrite.Error)
	}

	// The proposal is now listable through the review queue (read-only).
	queue := registryCall(t, registry, "okf_query", map[string]interface{}{
		"memory_review_queue": true,
	})
	if !queue.OK {
		t.Fatalf("review queue failed: %v", queue.Error)
	}
	data, _ := json.Marshal(queue.Result)
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	items, _ := decoded["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("review queue items = %d, want 1: %s", len(items), data)
	}
}

// S38: an unknown temporal-ish field is still rejected on a durable write.
func TestMCPTemporalUnknownFieldRejected(t *testing.T) {
	repo := initMCPToolTestRepo(t)
	registry := NewToolRegistryWithService(toolsvc.NewService(toolsvc.Config{RepoPath: repo}))
	env := registryCall(t, registry, "okf_note", map[string]interface{}{
		"content": "valid", "idempotency_key": "unknown-field-1",
		"memory_relation_kind_typo": "updates",
	})
	if env.OK || env.Error == nil || env.Error.Code != toolsvc.ErrInvalidRequest {
		t.Fatalf("envelope = %#v, want invalid_request", env)
	}
}

// S38: memory_confidence maps to *float64 (nil when absent, set when present).
func TestMCPTemporalWriteRequestMapping(t *testing.T) {
	// Absent confidence stays nil.
	absent := writeRequestFromArgs("note", map[string]interface{}{
		"content": "c", "idempotency_key": "k",
	})
	if absent.MemoryConfidence != nil {
		t.Fatalf("absent memory_confidence must map to nil, got %v", *absent.MemoryConfidence)
	}
	if absent.MemoryState != "" || absent.MemoryRelationKind != "" || len(absent.MemoryRelationTargets) != 0 {
		t.Fatalf("absent temporal fields must be empty: %#v", absent)
	}

	// Present confidence and relation map through.
	withConf := writeRequestFromArgs("note", map[string]interface{}{
		"content":                 "c",
		"idempotency_key":         "k",
		"memory_state":            "proposed",
		"memory_confidence":       float64(0.42),
		"memory_relation_kind":    "updates",
		"memory_relation_targets": []interface{}{"okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"evidence_refs":           []interface{}{"spec/x.md"},
	})
	if withConf.MemoryConfidence == nil || *withConf.MemoryConfidence != 0.42 {
		t.Fatalf("memory_confidence = %v, want 0.42", withConf.MemoryConfidence)
	}
	if withConf.MemoryState != "proposed" || withConf.MemoryRelationKind != "updates" {
		t.Fatalf("temporal fields not mapped: %#v", withConf)
	}
	if len(withConf.MemoryRelationTargets) != 1 || withConf.MemoryRelationTargets[0] != "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("relation targets = %v", withConf.MemoryRelationTargets)
	}
	if len(withConf.EvidenceRefs) != 1 || withConf.EvidenceRefs[0] != "spec/x.md" {
		t.Fatalf("evidence_refs = %v", withConf.EvidenceRefs)
	}
}

// S37: queryRequestFromArgs maps the temporal query knobs.
func TestMCPTemporalQueryRequestMapping(t *testing.T) {
	req := queryRequestFromArgs(map[string]interface{}{
		"query":               "routing",
		"memory_view":         "history",
		"refs":                []interface{}{"okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		"memory_review_queue": true,
	})
	if req.MemoryView != "history" {
		t.Fatalf("MemoryView = %q", req.MemoryView)
	}
	if len(req.Refs) != 1 || req.Refs[0] != "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("Refs = %v", req.Refs)
	}
	if !req.MemoryReviewQueue {
		t.Fatalf("MemoryReviewQueue must be true")
	}
}

// initRegistryOK calls okf_init so the service-backed registry has a knowledge
// tree to write into.
func initRegistryOK(t *testing.T, registry *ToolRegistry) {
	t.Helper()
	env := registryCall(t, registry, "okf_init", map[string]interface{}{})
	if !env.OK {
		t.Fatalf("okf_init failed: %v", env.Error)
	}
}

// registryCall invokes one registered tool and decodes the service envelope.
func registryCall(t *testing.T, registry *ToolRegistry, name string, args map[string]interface{}) toolsvc.ToolEnvelope {
	t.Helper()
	result, err := registry.Call(name, args)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	var env toolsvc.ToolEnvelope
	if err := json.Unmarshal([]byte(result.Content[0].Text), &env); err != nil {
		t.Fatalf("decode %s envelope: %v\n%s", name, err, result.Content[0].Text)
	}
	return env
}
