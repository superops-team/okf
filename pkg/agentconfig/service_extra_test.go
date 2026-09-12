package agentconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdapterStatusStates covers missing|installed|drifted|conflict per client.
func TestAdapterStatusStates(t *testing.T) {
	for _, client := range []string{"cursor", "claude-code", "codex"} {
		t.Run(client, func(t *testing.T) {
			root := newRepo(t)
			svc := NewService(root, nil)

			// missing on a fresh repo.
			rep := mustStatus(t, svc, client)
			for _, f := range rep.Files {
				if f.Status != string(StatusMissing) {
					t.Errorf("fresh: want missing, got %s for %s", f.Status, f.Path)
				}
			}

			if err := svc.Apply(client, true); err != nil {
				t.Fatal(err)
			}
			rep = mustStatus(t, svc, client)
			for _, f := range rep.Files {
				if f.Status != string(StatusInstalled) {
					t.Errorf("installed: want installed, got %s for %s", f.Status, f.Path)
				}
			}

			// Drift: hand-edit the owned artifact.
			switch client {
			case "cursor":
				p := filepath.Join(root, ".cursor/rules/okf.md")
				b, _ := os.ReadFile(p)
				os.WriteFile(p, append(b, []byte("\n// manual edit")...), 0o644)
			case "claude-code":
				p := filepath.Join(root, ".claude/skills/okf/SKILL.md")
				b, _ := os.ReadFile(p)
				os.WriteFile(p, append(b, []byte("\n// manual edit")...), 0o644)
			case "codex":
				p := filepath.Join(root, "AGENTS.md")
				b, _ := os.ReadFile(p)
				edited := strings.Replace(string(b), "OKF project knowledge workflow", "OKF CHANGED WORKFLOW", 1)
				os.WriteFile(p, []byte(edited), 0o644)
			}
			rep = mustStatus(t, svc, client)
			var drifted bool
			for _, f := range rep.Files {
				if f.Path == "AGENTS.md" || strings.HasSuffix(f.Path, "okf.md") ||
					strings.HasSuffix(f.Path, "SKILL.md") {
					if f.Status == string(StatusDrifted) {
						drifted = true
					}
				}
			}
			if !drifted {
				t.Errorf("expected a drifted owned artifact, got %+v", rep.Files)
			}
		})
	}
}

func mustStatus(t *testing.T, svc *Service, client string) ClientReport {
	t.Helper()
	reps, err := svc.Status(client)
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 1 {
		t.Fatalf("expected one report, got %d", len(reps))
	}
	return reps[0]
}

// TestAdapterConflicts covers malformed JSON, unowned TOML table, unbalanced
// markers and unowned whole-file, asserting inputs stay byte-identical.
func TestAdapterConflicts(t *testing.T) {
	t.Run("malformed json", func(t *testing.T) {
		root := newRepo(t)
		writeFile(t, root, ".cursor/mcp.json", "{ not json")
		before := readFile(t, root, ".cursor/mcp.json")
		svc := NewService(root, nil)
		reps, err := svc.Plan("cursor")
		if err != nil {
			t.Fatal(err)
		}
		if reps[0].Status != "conflict" {
			t.Fatalf("expected conflict report, got %+v", reps[0])
		}
		// Apply must also refuse and write nothing.
		if aerr := svc.Apply("cursor", true); aerr == nil {
			t.Fatal("apply must refuse malformed json")
		}
		if after := readFile(t, root, ".cursor/mcp.json"); after != before {
			t.Fatal("malformed json modified")
		}
	})

	t.Run("unowned toml table", func(t *testing.T) {
		root := newRepo(t)
		writeFile(t, root, ".codex/config.toml", "[mcp_servers.okf]\ncommand = \"something-else\"\n")
		before := readFile(t, root, ".codex/config.toml")
		svc := NewService(root, nil)
		reps, err := svc.Plan("codex")
		if err != nil {
			t.Fatal(err)
		}
		if reps[0].Status != "conflict" {
			t.Fatalf("expected conflict report, got %+v", reps[0])
		}
		if aerr := svc.Apply("codex", true); aerr == nil {
			t.Fatal("apply must refuse unowned toml table")
		}
		if after := readFile(t, root, ".codex/config.toml"); after != before {
			t.Fatal("unowned toml table modified")
		}
	})

	t.Run("unbalanced markers", func(t *testing.T) {
		root := newRepo(t)
		writeFile(t, root, "AGENTS.md", agBegin+"\nsome block\n")
		svc := NewService(root, nil)
		reps, err := svc.Plan("codex")
		if err != nil {
			t.Fatal(err)
		}
		if reps[0].Status != "conflict" {
			t.Fatalf("expected conflict report, got %+v", reps[0])
		}
	})

	t.Run("unowned whole file", func(t *testing.T) {
		root := newRepo(t)
		writeFile(t, root, ".cursor/rules/okf.md", "# user rule, no OKF header\n")
		svc := NewService(root, nil)
		reps, err := svc.Plan("cursor")
		if err != nil {
			t.Fatal(err)
		}
		if reps[0].Status != "conflict" {
			t.Fatalf("expected conflict report, got %+v", reps[0])
		}
		// And remove must also refuse.
		if rerr := svc.Remove("cursor", true); rerr == nil {
			t.Fatal("remove must refuse unowned whole file")
		}
	})
}

// TestAdapterRemoveOwnership asserts remove only touches OKF-owned content and
// conflicts on ambiguous ownership (no force).
func TestAdapterRemoveOwnership(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, ".mcp.json", `{"mcpServers":{"okf":{"command":"x"}}}`)
	svc := NewService(root, nil)
	// Same-name entry without OKF marker -> conflict on apply AND remove.
	if err := svc.Apply("claude-code", true); err == nil {
		t.Fatal("apply should conflict on unowned entry")
	}
	if err := svc.Remove("claude-code", true); err == nil {
		t.Fatal("remove should conflict on unowned entry")
	}
}

// TestAgentSecretRedaction asserts literal credential values never appear in
// plan/status output and generated config has no credential fields.
func TestAgentSecretRedaction(t *testing.T) {
	root := newRepo(t)
	const secret = "ghp-literal-supersecret-value"
	writeFile(t, root, ".cursor/mcp.json",
		`{"mcpServers":{"other":{"command":"x","env":{"GITHUB_TOKEN":"`+secret+`","HOME_REF":"$HOME"}}}}`)
	svc := NewService(root, nil)
	reps, err := svc.Plan("cursor")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reps {
		data, _ := json.Marshal(r)
		if strings.Contains(string(data), secret) {
			t.Fatalf("secret leaked into plan output: %s", data)
		}
	}
	// Generated OKF config must not introduce credential fields. The user's
	// own secret in a non-OKF entry is preserved semantically (and must not
	// appear in our emitted output, which only uses hashes).
	if err := svc.Apply("cursor", true); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, root, ".cursor/mcp.json")), &doc); err != nil {
		t.Fatal(err)
	}
	servers := doc["mcpServers"].(map[string]any)
	okfEntry := servers["okf"].(map[string]any)
	okfEnv := okfEntry["env"].(map[string]any)
	if len(okfEnv) != 1 {
		t.Fatalf("OKF env must carry only the ownership marker, got %v", okfEnv)
	}
	if _, has := okfEnv["GITHUB_TOKEN"]; has {
		t.Fatal("generated OKF config introduced a credential field")
	}
	// The user's unrelated env reference style is preserved verbatim.
	other := servers["other"].(map[string]any)
	otherEnv := other["env"].(map[string]any)
	if otherEnv["HOME_REF"] != "$HOME" {
		t.Errorf("env var reference not preserved: %v", otherEnv["HOME_REF"])
	}
}

// TestAgentSymlinkEscape asserts path boundaries reject escape attempts.
func TestAgentSymlinkEscape(t *testing.T) {
	root := newRepo(t)

	// Direct lexical escape.
	if _, err := safeJoin(root, "../outside"); err == nil {
		t.Fatal("expected unsafe path for ..")
	} else if ace := err.(*AgentConfigError); ace.Code != ErrUnsafeAgentConfigPath {
		t.Fatalf("expected unsafe code, got %v", ace)
	}
	// Absolute path.
	if _, err := safeJoin(root, "/etc/passwd"); err == nil {
		t.Fatal("expected unsafe path for absolute")
	}

	// Symlink escape: link inside root pointing outside.
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("nope"), 0o644)
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlink unsupported")
	}
	if _, err := safeJoin(root, "link/secret.txt"); err == nil {
		t.Fatal("expected unsafe path for symlink escape")
	} else if ace := err.(*AgentConfigError); ace.Code != ErrUnsafeAgentConfigPath {
		t.Fatalf("expected unsafe code, got %v", ace)
	}
}

// TestUnsupportedAgentClient asserts unknown client fails explicitly and writes
// nothing.
func TestUnsupportedAgentClient(t *testing.T) {
	root := newRepo(t)
	svc := NewService(root, nil)
	if _, err := svc.Plan("nope"); err == nil {
		t.Fatal("expected unsupported")
	} else if ace := err.(*AgentConfigError); ace.Code != ErrUnsupportedAgentClient {
		t.Fatalf("expected unsupported code, got %v", ace)
	}
	if err := svc.Apply("nope", true); err == nil {
		t.Fatal("apply should fail unsupported")
	}
	// "all" must cover the three known clients.
	reps, err := svc.Plan("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 3 {
		t.Fatalf("expected 3 clients for all, got %d", len(reps))
	}
}

// TestAgentRollback asserts a mid-apply write failure restores already-changed
// files from in-process original bytes.
func TestAgentRollback(t *testing.T) {
	root := newRepo(t)
	svc := NewService(root, nil)

	// File 1 will be created (changed). File 2 writes onto an existing
	// directory, which makes os.Rename fail deterministically.
	dirAsFile := filepath.Join(root, "adir")
	os.MkdirAll(dirAsFile, 0o755)
	op1 := fileOp{rel: "one.txt", abs: filepath.Join(root, "one.txt"), current: nil, proposed: []byte("v1"), action: ActionCreate}
	op2 := fileOp{rel: "adir", abs: dirAsFile, current: nil, proposed: []byte("v2"), action: ActionCreate}

	err := svc.commit(cursorAdapter{}, []fileOp{op1, op2})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if exists(root, "one.txt") {
		t.Fatal("rollback did not remove created file")
	}
}

// TestClaudeSkillFrontmatter asserts the rendered SKILL.md has valid skill
// frontmatter.
func TestClaudeSkillFrontmatter(t *testing.T) {
	text := RenderClaudeSkill()
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("skill must start with frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		t.Fatal("skill frontmatter not closed")
	}
	front := text[4 : 4+end]
	for _, want := range []string{"name: okf", "description:"} {
		if !strings.Contains(front, want) {
			t.Errorf("skill frontmatter missing %q", want)
		}
	}
	if !strings.Contains(text, PendingFileHeader) {
		t.Error("skill lacks OKF ownership header")
	}
}

// TestCodexTOMLCommentsPreserved asserts comments and unknown TOML survive.
func TestCodexTOMLCommentsPreserved(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, ".codex/config.toml", "# keep me\n[unknown_table]\nx=1\n")
	svc := NewService(root, nil)
	if err := svc.Apply("codex", true); err != nil {
		t.Fatal(err)
	}
	cfg := readFile(t, root, ".codex/config.toml")
	if !strings.Contains(cfg, "# keep me") {
		t.Error("comment lost")
	}
	if !strings.Contains(cfg, "[unknown_table]") {
		t.Error("unknown toml table lost")
	}
	if !strings.Contains(cfg, tomlBegin) || !strings.Contains(cfg, tomlEnd) {
		t.Error("managed block markers missing")
	}
	// Idempotent.
	before := cfg
	if err := svc.Apply("codex", true); err != nil {
		t.Fatal(err)
	}
	if readFile(t, root, ".codex/config.toml") != before {
		t.Error("codex toml not idempotent")
	}
}
