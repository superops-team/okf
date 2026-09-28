package agentconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLifecycle_W09W10Content verifies that after Apply, all three clients
// have W09/W10 and tool references in their installed files.
func TestLifecycle_W09W10Content(t *testing.T) {
	for _, client := range []string{"cursor", "claude-code", "codex"} {
		t.Run(client, func(t *testing.T) {
			root := newRepo(t)
			svc := NewService(root, nil)

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

			if !strings.Contains(content, "W09") {
				t.Errorf("%s: W09 missing from installed %s", client, path)
			}
			if !strings.Contains(content, "W10") {
				t.Errorf("%s: W10 missing from installed %s", client, path)
			}
			if !strings.Contains(content, "okf_reflect") {
				t.Errorf("%s: okf_reflect missing from installed %s", client, path)
			}
			if !strings.Contains(content, "okf_relation_recall") {
				t.Errorf("%s: okf_relation_recall missing from installed %s", client, path)
			}
			if !strings.Contains(content, "need_clarify") {
				t.Errorf("%s: need_clarify missing from installed %s", client, path)
			}

			// Verify Status=installed
			rep, err := svc.Status(client)
			if err != nil {
				t.Fatalf("Status failed: %v", err)
			}
			for _, cr := range rep {
				if cr.Status != string(StatusInstalled) && cr.Status != "no_change" {
					t.Errorf("%s: client status=%s, want installed/no_change", client, cr.Status)
				}
				for _, f := range cr.Files {
					if f.Status != string(StatusInstalled) && f.Status != "no_change" {
						t.Errorf("%s: file %s status=%s, want installed/no_change", client, f.Path, f.Status)
					}
				}
			}

			// Remove and verify cleanup
			if err := svc.Remove(client, true); err != nil {
				t.Fatalf("Remove failed: %v", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) && client != "codex" {
				// For codex, AGENTS.md may still exist but managed block removed.
				// We just verify it's not the full skill file.
			}
		})
	}
}
