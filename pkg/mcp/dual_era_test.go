package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"
)

// newTestServer creates a Server with in-memory writer for testing.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(ServerConfig{
		RepoPath: t.TempDir(),
		Logger:   log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.writer = &bytes.Buffer{}
	return s
}

func (s *Server) feed(data string) {
	s.handleMessage([]byte(data))
}

func (s *Server) output() string {
	return s.writer.(*bytes.Buffer).String()
}

func (s *Server) lastResponse() map[string]any {
	out := s.output()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := lines[len(lines)-1]
	var resp map[string]any
	json.Unmarshal([]byte(last), &resp)
	return resp
}

func modernMeta() string {
	return `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`
}

func TestModernDiscover(t *testing.T) {
	s := newTestServer(t)
	s.feed(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":` + modernMeta() + `}`)
	resp := s.lastResponse()
	if resp["error"] != nil {
		t.Fatalf("unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatal("no result")
	}
	if result["resultType"] != "complete" {
		t.Errorf("expected resultType=complete, got %v", result["resultType"])
	}
	versions, ok := result["supportedVersions"].([]any)
	if !ok || len(versions) != 1 || versions[0] != "2026-07-28" {
		t.Errorf("expected supportedVersions=[2026-07-28], got %v", result["supportedVersions"])
	}
	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatal("no _meta in result")
	}
	serverInfo, ok := meta["io.modelcontextprotocol/serverInfo"].(map[string]any)
	if !ok || serverInfo["name"] != "okf-mcp-server" {
		t.Errorf("expected serverInfo, got %v", meta)
	}
}

func TestModernToolsListHas11Tools(t *testing.T) {
	s := newTestServer(t)
	s.feed(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":` + modernMeta() + `}`)
	resp := s.lastResponse()
	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 11 {
		t.Errorf("expected 11 modern tools, got %d", len(tools))
	}
	// Verify sorted and no legacy-only tools
	names := make([]string, len(tools))
	for i, tl := range tools {
		names[i] = tl.(map[string]any)["name"].(string)
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("tools not sorted: %s before %s", names[i-1], names[i])
		}
	}
	for _, name := range names {
		if name == "okf_load_bundle" || name == "okf_search" || name == "okf_semantic_search" {
			t.Errorf("legacy tool %s should not be in modern catalog", name)
		}
	}
}

func TestLegacyInitialize(t *testing.T) {
	s := newTestServer(t)
	s.feed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`)
	resp := s.lastResponse()
	result := resp["result"].(map[string]any)
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected legacy protocol version, got %v", result["protocolVersion"])
	}
	// Legacy result should NOT have resultType or _meta
	if _, ok := result["resultType"]; ok {
		t.Error("legacy result should not have resultType")
	}
}

func TestMixedEraRejected(t *testing.T) {
	s := newTestServer(t)
	// First select legacy
	s.feed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}`)
	// Then try modern method
	s.feed(`{"jsonrpc":"2.0","id":2,"method":"server/discover","params":` + modernMeta() + `}`)
	resp := s.lastResponse()
	if resp["error"] == nil {
		t.Error("expected mixed-era rejection error")
	}
}

func TestModernUnsupportedVersion(t *testing.T) {
	s := newTestServer(t)
	s.feed(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2024-11-05","io.modelcontextprotocol/clientCapabilities":{}}}}`)
	resp := s.lastResponse()
	err, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error")
	}
	if int(err["code"].(float64)) != -32022 {
		t.Errorf("expected -32022, got %v", err["code"])
	}
}

func TestModernSkillsRequireCapability(t *testing.T) {
	s := newTestServer(t)
	// Without skills capability
	s.feed(`{"jsonrpc":"2.0","id":1,"method":"skills/list","params":` + modernMeta() + `}`)
	resp := s.lastResponse()
	err, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected missing capability error")
	}
	if int(err["code"].(float64)) != -32021 {
		t.Errorf("expected -32021, got %v", err["code"])
	}
}

func TestModernPromptsNotImplemented(t *testing.T) {
	s := newTestServer(t)
	s.feed(`{"jsonrpc":"2.0","id":1,"method":"prompts/list","params":` + modernMeta() + `}`)
	resp := s.lastResponse()
	err, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected method not found error")
	}
	if int(err["code"].(float64)) != -32601 {
		t.Errorf("expected -32601, got %v", err["code"])
	}
}

func TestEraResetOnNewServer(t *testing.T) {
	s1 := newTestServer(t)
	s1.feed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{}}}`)
	if s1.era != eraLegacy {
		t.Error("expected legacy era")
	}
	// New server instance should start unselected
	s2 := newTestServer(t)
	if s2.era != eraUnselected {
		t.Error("new server should start unselected")
	}
}
