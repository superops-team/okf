package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureAgentStdout swaps os.Stdout for the duration of fn and returns what was
// written. os.Stderr is left untouched.
func captureAgentStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	return string(data)
}

func TestCmdAgentE2E(t *testing.T) {
	root := t.TempDir()
	for _, client := range []string{"cursor", "claude-code", "codex"} {
		t.Run(client, func(t *testing.T) {
			// Plan (read-only) must succeed and emit JSON.
			out := captureAgentStdout(t, func() {
				code := CmdAgent([]string{"plan", "--client", client, "--format", "json", "--repo", root})
				if code != 0 {
					t.Fatalf("plan exit %d", code)
				}
			})
			if !strings.Contains(out, `"client": "`+client+`"`) {
				t.Fatalf("plan json missing client: %s", out)
			}

			// Apply twice via the CLI (non-interactive, --yes).
			if code := CmdAgent([]string{"apply", "--client", client, "--yes", "--repo", root}); code != 0 {
				t.Fatalf("apply exit %d", code)
			}
			if code := CmdAgent([]string{"apply", "--client", client, "--yes", "--repo", root}); code != 0 {
				t.Fatalf("second apply exit %d", code)
			}

			// Status.
			out = captureAgentStdout(t, func() {
				if code := CmdAgent([]string{"status", "--client", client, "--format", "json", "--repo", root}); code != 0 {
					t.Fatalf("status exit %d", code)
				}
			})
			if !strings.Contains(out, `"status": "installed"`) {
				t.Fatalf("status not installed: %s", out)
			}

			// Remove.
			if code := CmdAgent([]string{"remove", "--client", client, "--yes", "--repo", root}); code != 0 {
				t.Fatalf("remove exit %d", code)
			}
		})
	}
}

func TestCmdAgentMutationRequiresConfirmationViaCLI(t *testing.T) {
	root := t.TempDir()
	out := captureAgentStdout(t, func() {
		code := CmdAgent([]string{"apply", "--client", "cursor", "--repo", root})
		if code == 0 {
			t.Fatal("apply without --yes should exit non-zero")
		}
	})
	if !strings.Contains(out, "agent_config_confirmation_required") {
		// confirmation error is printed to stderr, not stdout; just assert no file
		if _, err := os.Stat(filepath.Join(root, ".cursor", "mcp.json")); !os.IsNotExist(err) {
			t.Fatal("apply without --yes wrote files")
		}
	}
}

func TestCmdAgentUnsupportedClientViaCLI(t *testing.T) {
	root := t.TempDir()
	code := CmdAgent([]string{"plan", "--client", "nope", "--format", "json", "--repo", root})
	if code == 0 {
		t.Fatal("unknown client should exit non-zero")
	}
}
