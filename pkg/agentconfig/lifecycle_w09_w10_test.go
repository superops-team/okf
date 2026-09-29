package agentconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLifecycle_W09W10Content verifies Apply→Status→Remove lifecycle
// with strict content assertions on installed files.
func TestLifecycle_W09W10Content(t *testing.T) {
	for _, client := range []string{"cursor", "claude-code", "codex"} {
		t.Run(client, func(t *testing.T) {
			root := newRepo(t)
			svc := NewService(root, nil)

			// Apply
			if err := svc.Apply(client, true); err != nil {
				t.Fatalf("Apply failed: %v", err)
			}

			var content string
			var path string
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

			// Strict content assertions
			for _, want := range []string{"W09", "W10", "okf_reflect", "okf_relation_recall", "need_clarify"} {
				if !strings.Contains(content, want) {
					t.Errorf("%s: %q missing from installed %s", client, want, path)
				}
			}

			// Apply idempotent (second Apply should not error)
			if err := svc.Apply(client, true); err != nil {
				t.Fatalf("Apply idempotent failed: %v", err)
			}

			// Status
			rep, err := svc.Status(client)
			if err != nil {
				t.Fatalf("Status failed: %v", err)
			}
			for _, cr := range rep {
				if cr.Status != "installed" && cr.Status != "no_change" {
					t.Errorf("%s: client status=%s", client, cr.Status)
				}
			}

			// Remove
			if err := svc.Remove(client, true); err != nil {
				t.Fatalf("Remove failed: %v", err)
			}

			// Strict post-Remove assertions
			switch client {
			case "cursor":
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("cursor: %s should not exist after Remove", path)
				}
			case "claude-code":
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("claude: %s should not exist after Remove", path)
				}
			case "codex":
				// AGENTS.md may exist but managed block must be gone.
				b2, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("codex AGENTS.md should still exist after Remove: %v", err)
				}
				after := string(b2)
				if strings.Contains(after, "W09") || strings.Contains(after, "W10") {
					t.Errorf("codex: W09/W10 should be removed from AGENTS.md managed block")
				}
			}

			// Remove idempotent
			if err := svc.Remove(client, true); err != nil {
				t.Fatalf("Remove idempotent failed: %v", err)
			}
		})
	}
}
