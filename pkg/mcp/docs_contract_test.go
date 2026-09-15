package mcp

import (
	"os"
	"strings"
	"testing"
)

// TestDocumentationContract verifies that documentation contains required
// claims and does not contain prohibited claims (S30-S32).
func TestDocumentationContract(t *testing.T) {
	docs := []string{
		"../../docs/knowledge/mcp-server.md",
	}

	requiredClaims := []string{
		"2026-07-28", // modern protocol version
		"2024-11-05", // legacy protocol version
		"skill://okf/SKILL.md",
		"resultType",
		"serverInfo",
	}

	prohibitedClaims := []string{
		"directoryRead: true", // S32: directoryRead must be absent/false
		"proves trust",        // S30: digests do not prove trust
		"proves safety",       // S30: digests do not prove safety
		"approved by",         // S30: no approval claim
		"safe to execute",     // S30: no safety claim
	}

	for _, docPath := range docs {
		t.Run(docPath, func(t *testing.T) {
			data, err := os.ReadFile(docPath)
			if err != nil {
				t.Fatalf("cannot read %s: %v", docPath, err)
			}
			content := string(data)

			for _, claim := range requiredClaims {
				if !strings.Contains(content, claim) {
					t.Errorf("document %s missing required claim: %q", docPath, claim)
				}
			}

			for _, claim := range prohibitedClaims {
				if strings.Contains(content, claim) {
					t.Errorf("document %s contains prohibited claim: %q", docPath, claim)
				}
			}
		})
	}
}

// TestSecurityBoundaryDocs verifies that security documentation assigns
// Host responsibilities and states OKF does not perform them (S31).
func TestSecurityBoundaryDocs(t *testing.T) {
	data, err := os.ReadFile("../../docs/knowledge/mcp-server.md")
	if err != nil {
		t.Fatalf("cannot read mcp-server.md: %v", err)
	}
	content := string(data)

	hostResponsibilities := []string{
		"origin labeling",
		"untrusted-input",
		"approval",
		"permission gating",
	}
	for _, resp := range hostResponsibilities {
		if !strings.Contains(content, resp) {
			t.Errorf("security docs missing Host responsibility: %q", resp)
		}
	}

	// OKF must state it does not perform Host actions
	if !strings.Contains(content, "does not perform") && !strings.Contains(content, "does not execute") {
		t.Error("security docs must state OKF does not perform Host actions")
	}
}
