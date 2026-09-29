package agentconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLifecycle_W09W10_Strict verifies Apply/Status/Remove lifecycle
// with strict content, semantics, idempotency, and cleanup assertions.
func TestLifecycle_W09W10_Strict(t *testing.T) {
	for _, client := range []string{"cursor", "claude-code", "codex"} {
		t.Run(client, func(t *testing.T) {
			root := newRepo(t)
			svc := NewService(root, nil)

			// Pre-populate user content for codex (verify it's preserved).
			var userContent string
			if client == "codex" {
				userContent = "# My AGENTS.md\nMy custom rules here.\n"
				os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(userContent), 0o644)
			}

			// 1. Plan: should return non-empty changes.
			plan, err := svc.Plan(client)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			if len(plan) == 0 {
				t.Fatalf("Plan returned empty for %s", client)
			}

			// 2. Apply.
			if err := svc.Apply(client, true); err != nil {
				t.Fatalf("Apply failed: %v", err)
			}

			// Read installed file.
			var path, content string
			switch client {
			case "cursor":
				path = filepath.Join(root, ".cursor/rules/okf.md")
			case "claude-code":
				path = filepath.Join(root, ".claude/skills/okf/SKILL.md")
			case "codex":
				path = filepath.Join(root, "AGENTS.md")
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content = string(b)

			// 3b. Codex must also install the managed MCP block in
			// .codex/config.toml (not just AGENTS.md). This is how Codex
			// discovers the okf MCP server for persisted-skill E2E.
			if client == "codex" {
				tomlPath := filepath.Join(root, ".codex", "config.toml")
				tb, err := os.ReadFile(tomlPath)
				if err != nil {
					t.Fatalf("read %s after Apply: %v", tomlPath, err)
				}
				tomlContent := string(tb)
				if !strings.Contains(tomlContent, "BEGIN OKF MANAGED MCP v1") {
					t.Errorf("codex: .codex/config.toml missing managed MCP block:\n%s", tomlContent)
				}
				if !strings.Contains(tomlContent, "mcp_servers.okf") {
					t.Errorf("codex: .codex/config.toml missing mcp_servers.okf:\n%s", tomlContent)
				}
			}

			// 3. Strict W09/W10 content + semantics.
			semanticChecks := map[string]string{
				"W09":                 "W09",
				"W10":                 "W10",
				"okf_reflect":         "okf_reflect",
				"okf_relation_recall": "okf_relation_recall",
				"need_clarify":        "need_clarify",
				"Memory Defense":      "Memory Defense",
				"[REDACTED":           "[REDACTED",
				"read-only":           "read-only",
				"proposed":            "proposed",
				"declined":            "declined",
			}
			for name, want := range semanticChecks {
				if !strings.Contains(content, want) {
					t.Errorf("%s: missing %q (%s)", client, want, name)
				}
			}

			// 4. Apply idempotent: second Apply should not error.
			if err := svc.Apply(client, true); err != nil {
				t.Fatalf("Apply idempotent: %v", err)
			}

			// 5. Status: non-empty report, all files installed.
			rep, err := svc.Status(client)
			if err != nil {
				t.Fatalf("Status failed: %v", err)
			}
			if len(rep) == 0 {
				t.Fatalf("Status returned empty for %s", client)
			}
			for _, cr := range rep {
				for _, f := range cr.Files {
					if f.Status != "installed" && f.Status != "no_change" {
						t.Errorf("%s: file %s status=%s", client, f.Path, f.Status)
					}
				}
			}

			// 6. Remove.
			if err := svc.Remove(client, true); err != nil {
				t.Fatalf("Remove failed: %v", err)
			}

			// 7. Post-Remove strict assertions.
			switch client {
			case "cursor":
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("cursor: %s must not exist after Remove", path)
				}
			case "claude-code":
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("claude: %s must not exist after Remove", path)
				}
			case "codex":
				b2, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("codex AGENTS.md should still exist: %v", err)
				}
				after := string(b2)
				if strings.Contains(after, "W09") || strings.Contains(after, "W10") {
					t.Errorf("codex: W09/W10 must be removed from AGENTS.md")
				}
				if strings.Contains(after, "OKF MANAGED") {
					t.Errorf("codex: OKF MANAGED markers must be removed")
				}
				// Managed MCP block must also be removed from .codex/config.toml.
				tomlPath := filepath.Join(root, ".codex", "config.toml")
				if tb, err := os.ReadFile(tomlPath); err == nil {
					if strings.Contains(string(tb), "OKF MANAGED MCP") || strings.Contains(string(tb), "mcp_servers.okf") {
						t.Errorf("codex: managed MCP block must be removed from .codex/config.toml:\n%s", string(tb))
					}
				}
				// User content preserved.
				if !strings.Contains(after, "My custom rules") {
					t.Errorf("codex: user content must be preserved after Remove")
				}
			}

			// 8. Remove idempotent (cursor/claude), codex may error on second
			// Remove because managed block is already gone (acceptable).
			if client != "codex" {
				if err := svc.Remove(client, true); err != nil {
					t.Fatalf("Remove idempotent: %v", err)
				}
			}
		})
	}
}
