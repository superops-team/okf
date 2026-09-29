package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func buildOKFBin(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "okf")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/okf")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build okf: %v\n%s", err, out)
	}
	return bin
}

func repoRoot() string {
	wd, _ := os.Getwd()
	for d := wd; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	return wd
}

const (
	idA = "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	idD = "okf_dddddddddddddddddddddddddddddddd"
	idZ = "okf_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
)

func writeParityConcept(t *testing.T, kb, id, title, state, extends string) {
	t.Helper()
	content := fmt.Sprintf("---\nokf_id: %s\ntitle: %q\ntype: note\nmemory_state: %s\n%s---\n# %s\nBody.\n", id, title, state, extends, title)
	os.WriteFile(filepath.Join(kb, id+".md"), []byte(content), 0o644)
}

const extendsRelation = `memory_relation:
  kind: extends
  targets:
    - `

// TestCLI_MCP_RelationParity verifies real CLI subprocess vs MCP stdio subprocess
// return equivalent results for relation recall.
func TestCLI_MCP_RelationParity(t *testing.T) {
	bin := buildOKFBin(t)
	repo := t.TempDir()
	exec.Command("git", "init", "-q", repo).Run()
	exec.Command("git", "-C", repo, "config", "user.email", "t@t").Run()
	exec.Command("git", "-C", repo, "config", "user.name", "T").Run()
	kb := filepath.Join(repo, ".okf", "knowledge")
	os.MkdirAll(kb, 0o755)

	writeParityConcept(t, kb, idA, "A", "approved", extendsRelation+idB+"\n")
	writeParityConcept(t, kb, idB, "B", "approved", "")
	writeParityConcept(t, kb, idD, "D", "approved", extendsRelation+"okf_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n")
	exec.Command("git", "-C", repo, "add", "-A").Run()
	exec.Command("git", "-C", repo, "commit", "-q", "-m", "init").Run()

	// CLI: success (A extends B) — extract hits.
	cliOut, err := exec.Command(bin, "tool", "relation", "--anchor", idA, "--repo", repo, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("CLI A: %v\n%s", err, cliOut)
	}
	var cliEnv map[string]any
	json.Unmarshal(cliOut, &cliEnv)
	if cliEnv["ok"] != true {
		t.Fatalf("CLI A not ok: %s", cliOut)
	}
	cliHits := extractHits(t, cliEnv)
	if len(cliHits) < 1 {
		t.Fatal("CLI A: expected at least self hit")
	}

	// MCP: success (A extends B) — extract full hits.
	mcpEnv := callMCPRelationEnv(t, bin, repo, idA)
	if mcpEnv["ok"] != true {
		t.Fatalf("MCP A not ok: %+v", mcpEnv)
	}
	mcpHits := extractHits(t, mcpEnv)
	if len(mcpHits) < 1 {
		t.Fatal("MCP A: expected at least self hit")
	}

	// Compare full hit structures.
	compareHits(t, "A", cliHits, mcpHits)

	// CLI: unknown anchor → error.
	cliUnknown, _ := exec.Command(bin, "tool", "relation", "--anchor", idZ, "--repo", repo, "--json").CombinedOutput()
	var cliUnknownEnv map[string]any
	json.Unmarshal(cliUnknown, &cliUnknownEnv)
	if cliUnknownEnv["ok"] == true {
		t.Fatal("CLI unknown anchor should error")
	}

	// MCP: unknown anchor → error.
	mcpUnknown := callMCPRelationRaw(t, bin, repo, idZ)
	if mcpUnknown["ok"] == true {
		t.Fatal("MCP unknown anchor should error")
	}

	// CLI: dangling → ok (no panic).
	cliDangling, _ := exec.Command(bin, "tool", "relation", "--anchor", idD, "--repo", repo, "--json").CombinedOutput()
	var cliDanglingEnv map[string]any
	json.Unmarshal(cliDangling, &cliDanglingEnv)
	if cliDanglingEnv["ok"] != true {
		t.Fatalf("CLI dangling should not panic: %s", cliDangling)
	}

	// MCP: dangling → ok (no panic).
	mcpDanglingEnv := callMCPRelationEnv(t, bin, repo, idD)
	if mcpDanglingEnv["ok"] != true {
		t.Fatalf("MCP dangling should not panic: %+v", mcpDanglingEnv)
	}

	// Compare dangling hits (should both return self only).
	compareHits(t, "dangling", extractHits(t, cliDanglingEnv), extractHits(t, mcpDanglingEnv))
}

// hitStruct represents a relation hit for comparison.
type hitStruct struct {
	OKFID       string `json:"okf_id"`
	Edge        string `json:"edge"`
	State       string `json:"state"`
	IsChainHead bool   `json:"is_chain_head"`
}

// extractHits parses full hit structures from an envelope.
func extractHits(t *testing.T, env map[string]any) []hitStruct {
	t.Helper()
	result, ok := env["result"].(map[string]any)
	if !ok {
		return nil
	}
	hitsRaw, ok := result["hits"].([]any)
	if !ok {
		return nil
	}
	out := make([]hitStruct, 0, len(hitsRaw))
	for _, h := range hitsRaw {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		hs := hitStruct{}
		if v, ok := hm["okf_id"].(string); ok {
			hs.OKFID = v
		}
		if v, ok := hm["edge"].(string); ok {
			hs.Edge = v
		}
		if v, ok := hm["state"].(string); ok {
			hs.State = v
		}
		if v, ok := hm["is_chain_head"].(bool); ok {
			hs.IsChainHead = v
		}
		out = append(out, hs)
	}
	return out
}

// compareHits asserts two hit slices are identical in order and fields.
func compareHits(t *testing.T, label string, cli, mcp []hitStruct) {
	t.Helper()
	if len(cli) != len(mcp) {
		t.Fatalf("%s: hit count mismatch CLI=%d MCP=%d", label, len(cli), len(mcp))
	}
	for i := range cli {
		if cli[i] != mcp[i] {
			t.Fatalf("%s: hit[%d] mismatch CLI=%+v MCP=%+v", label, i, cli[i], mcp[i])
		}
	}
}

func callMCPRelationEnv(t *testing.T, bin, repo, anchor string) map[string]any {
	t.Helper()
	proc := exec.Command(bin, "mcp", "--repo", repo)
	stdin, _ := proc.StdinPipe()
	stdout, _ := proc.StdoutPipe()
	proc.Stderr = io.Discard
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	defer proc.Process.Kill()

	send := func(msg map[string]any) {
		body, _ := json.Marshal(msg)
		fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}

	send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}},
	})
	readMCPMsg(t, stdout)
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "okf_relation_recall", "arguments": map[string]any{"anchor": anchor}},
	})
	resp := readMCPMsg(t, stdout)

	// Extract envelope from MCP content.
	env := map[string]any{"ok": true}
	if res, ok := resp["result"].(map[string]any); ok {
		if content, ok := res["content"].([]any); ok {
			if first, ok := content[0].(map[string]any); ok {
				if text, ok := first["text"].(string); ok {
					json.Unmarshal([]byte(text), &env)
				}
			}
		}
	}
	if _, hasErr := resp["error"]; hasErr {
		env["ok"] = false
	}
	return env
}

func callMCPRelationRaw(t *testing.T, bin, repo, anchor string) map[string]any {
	env := callMCPRelationEnv(t, bin, repo, anchor)
	return map[string]any{"ok": env["ok"]}
}

func readMCPMsg(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	var contentLen int
	for {
		n, err := r.Read(tmp)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(buf, tmp[:n]...)
		if i := bytes.Index(buf, []byte("\r\n\r\n")); i >= 0 {
			header := string(buf[:i])
			fmt.Sscanf(header, "Content-Length: %d", &contentLen)
			buf = buf[i+4:]
			break
		}
	}
	for len(buf) < contentLen {
		n, _ := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
	}
	var msg map[string]any
	json.Unmarshal(buf[:contentLen], &msg)
	return msg
}
