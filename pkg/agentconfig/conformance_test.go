package agentconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo creates an isolated temp repository root.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func writeFile(t *testing.T, root, rel string, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(b)
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// shared conformance: every supported client must pass the full
// apply->apply->status->remove flow and preserve unowned content.
func TestAdapterIdempotenceAndPreservation(t *testing.T) {
	cases := map[string]struct {
		seed       func(root string)
		files      []string
		postRemove func(t *testing.T, root string)
	}{
		"cursor": {
			seed: func(root string) {
				writeFile(t, root, ".cursor/mcp.json", `{
  "mcpServers": {
    "other": { "command": "other", "env": {"UNRELATED": "keep-me"} }
  },
  "unknownTopLevel": { "nested": [1, 2, 3] }
}
`)
			},
			files: []string{".cursor/mcp.json", ".cursor/rules/okf.md"},
			postRemove: func(t *testing.T, root string) {
				// mcp.json still exists with the unrelated server preserved.
				if !strings.Contains(readFile(t, root, ".cursor/mcp.json"), `"other"`) {
					t.Errorf("unrelated server removed")
				}
				if strings.Contains(readFile(t, root, ".cursor/mcp.json"), serverName) &&
					strings.Contains(readFile(t, root, ".cursor/mcp.json"), OwnedMarkerKey) {
					t.Errorf("OKF entry not removed")
				}
			},
		},
		"claude-code": {
			seed: func(root string) {
				writeFile(t, root, ".mcp.json", `{
  "mcpServers": {
    "existing": { "command": "x", "args": ["a"] }
  }
}`)
			},
			files: []string{".mcp.json", ".claude/skills/okf/SKILL.md"},
			postRemove: func(t *testing.T, root string) {
				if !strings.Contains(readFile(t, root, ".mcp.json"), `"existing"`) {
					t.Errorf("unrelated server removed")
				}
			},
		},
		"codex": {
			seed: func(root string) {
				writeFile(t, root, ".codex/config.toml", `# user comment that must survive
[some_tool]
path = "/usr/bin/keep"

[mcp_servers.other]
command = "other"
`)
				writeFile(t, root, "AGENTS.md", "# Existing agent instructions\n\nKeep me intact.\n")
			},
			files: []string{".codex/config.toml", "AGENTS.md"},
			postRemove: func(t *testing.T, root string) {
				cfg := readFile(t, root, ".codex/config.toml")
				if !strings.Contains(cfg, "# user comment that must survive") {
					t.Errorf("toml comment not preserved after remove")
				}
				if !strings.Contains(cfg, "[some_tool]") {
					t.Errorf("unrelated toml table not preserved")
				}
				ag := readFile(t, root, "AGENTS.md")
				if !strings.Contains(ag, "# Existing agent instructions") {
					t.Errorf("ag content not preserved after remove")
				}
			},
		},
	}

	for client, tc := range cases {
		t.Run(client, func(t *testing.T) {
			root := newRepo(t)
			tc.seed(root)
			svc := NewService(root, nil)

			// First apply.
			if err := svc.Apply(client, true); err != nil {
				t.Fatalf("first apply: %v", err)
			}
			snap := map[string]string{}
			for _, f := range tc.files {
				snap[f] = readFile(t, root, f)
				if snap[f] == "" {
					t.Fatalf("expected file %s after first apply", f)
				}
			}
			// Unowned content preserved across first apply.
			if client == "codex" {
				if !strings.Contains(readFile(t, root, ".codex/config.toml"), "# user comment that must survive") {
					t.Errorf("toml comment lost on apply")
				}
			}

			// Second apply -> zero diff.
			if err := svc.Apply(client, true); err != nil {
				t.Fatalf("second apply: %v", err)
			}
			for _, f := range tc.files {
				if got := readFile(t, root, f); got != snap[f] {
					t.Errorf("second apply changed %s (not idempotent)", f)
				}
			}

			// Status reports installed.
			reports, err := svc.Status(client)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range reports {
				for _, f := range r.Files {
					if f.Status != string(StatusInstalled) {
						t.Errorf("expected installed for %s, got %s", f.Path, f.Status)
					}
				}
			}

			// Remove.
			if err := svc.Remove(client, true); err != nil {
				t.Fatalf("remove: %v", err)
			}
			tc.postRemove(t, root)
		})
	}
}

// TestAgentPlanReadOnly asserts plan writes nothing and starts no process.
func TestAgentPlanReadOnly(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, ".cursor/mcp.json", `{"mcpServers":{"x":{"command":"x"}}}`)
	svc := NewService(root, nil)

	before := map[string]bool{
		".cursor/mcp.json": exists(root, ".cursor/mcp.json"),
	}
	reports, err := svc.Plan("cursor")
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Client != "cursor" {
		t.Fatalf("unexpected reports %+v", reports)
	}
	// After plan, nothing on disk may have changed.
	if exists(root, ".cursor/rules/okf.md") {
		t.Fatal("plan created a file")
	}
	if got := exists(root, ".cursor/mcp.json"); got != before[".cursor/mcp.json"] {
		t.Fatal("plan modified mcp.json existence")
	}
	// Plan reports hashes.
	var sawAfter bool
	for _, f := range reports[0].Files {
		if f.AfterHash != "" {
			sawAfter = true
		}
	}
	if !sawAfter {
		t.Fatal("plan should report after hashes")
	}
}

// TestAgentMutationRequiresConfirmation asserts apply/remove without --yes fail
// before writing in non-interactive mode.
func TestAgentMutationRequiresConfirmation(t *testing.T) {
	root := newRepo(t)
	svc := NewService(root, nil)

	err := svc.Apply("cursor", false)
	if err == nil || err.(*AgentConfigError).Code != ErrRequiresConfirmation {
		t.Fatalf("apply without yes should require confirmation, got %v", err)
	}
	if exists(root, ".cursor/mcp.json") || exists(root, ".cursor/rules/okf.md") {
		t.Fatal("apply without yes wrote files")
	}
	err = svc.Remove("cursor", false)
	if err == nil || err.(*AgentConfigError).Code != ErrRequiresConfirmation {
		t.Fatalf("remove without yes should require confirmation, got %v", err)
	}
}
