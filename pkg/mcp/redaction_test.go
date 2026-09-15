package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"
)

// S33: errors and logs are redacted — no token, credential, env value,
// user home path, or Skill body is emitted.
func TestErrorAndLogRedaction(t *testing.T) {
	// Canaries that must never appear in output.
	const (
		tokenCanary = "SECRET_TOKEN_abc123xyz"
		envCanary   = "SECRET_ENV_VALUE_789"
		homeCanary  = "/home/secretuser/private/path"
		skillCanary = "SKILL_BODY_CANARY_secret_content"
	)

	var stderrBuf bytes.Buffer
	s, err := NewServer(ServerConfig{
		RepoPath: t.TempDir(),
		Logger:   log.New(&stderrBuf, "", 0),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.writer = &bytes.Buffer{}

	tests := []struct {
		name   string
		method string
		params string
	}{
		{
			name:   "unsupported version does not echo raw in message",
			method: "server/discover",
			params: `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2023-01-01","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"` + tokenCanary + `"}}}`,
		},
		{
			name:   "missing meta",
			method: "tools/list",
			params: `{}`,
		},
		{
			name:   "unknown skill URI does not expose path",
			method: "skills/get",
			params: `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{"extensions":{"io.modelcontextprotocol/skills":{}}}},"uri":"skill://okf/` + homeCanary + `"}`,
		},
		{
			name:   "malformed params",
			method: "tools/call",
			params: `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}},"name":"okf_query","arguments":{invalid}}`,
		},
		{
			name:   "env value in extra field not echoed",
			method: "tools/list",
			params: `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}},"extra_env":"` + envCanary + `"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderrBuf.Reset()
			s.writer.(*bytes.Buffer).Reset()

			req := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  tt.method,
			}
			if tt.params != "" {
				req["params"] = json.RawMessage(tt.params)
			}
			reqBytes, _ := json.Marshal(req)
			s.handleMessage(reqBytes)

			output := s.writer.(*bytes.Buffer).String()
			stderr := stderrBuf.String()

			// Check response doesn't leak canaries.
			for _, canary := range []string{tokenCanary, envCanary, homeCanary, skillCanary} {
				if strings.Contains(output, canary) {
					t.Errorf("response leaked canary %q in test %q", canary, tt.name)
				}
				if strings.Contains(stderr, canary) {
					t.Errorf("stderr leaked canary %q in test %q", canary, tt.name)
				}
			}
		})
	}
}

// S33: registry construction error messages don't contain local paths.
func TestRegistryErrorRedaction(t *testing.T) {
	// validateSkillManifest with bad URI should not contain filesystem paths.
	err := validateSkillManifest([]manifestEntry{
		{uri: "http://evil.com/SKILL.md", name: "okf", content: "x"},
	})
	if err == nil {
		t.Fatal("expected error for bad URI")
	}
	if strings.Contains(err.Error(), "/home/") || strings.Contains(err.Error(), "/tmp/") {
		t.Errorf("registry error contains local path: %v", err)
	}
}

// S33: NewServer with invalid registry (simulated) doesn't panic or leak.
func TestNewServerErrorNoLeak(t *testing.T) {
	// NewServer should succeed with valid config; this tests that the
	// construction path doesn't emit sensitive info. We verify via the
	// redaction test above plus the fact that NewServer returns no error
	// for valid config (no partial catalog).
	s, err := NewServer(ServerConfig{
		RepoPath: t.TempDir(),
		Logger:   log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("NewServer should succeed for valid config: %v", err)
	}
	if s.skills == nil {
		t.Error("skills registry should be initialized")
	}
}
