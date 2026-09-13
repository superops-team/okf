package agentconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestInspectJSONMCPPreservesUnknownKeys proves that user-added keys inside
// the OKF-managed mcpServers.okf entry are preserved across apply (D4).
// Custom env vars and unknown top-level keys must survive; managed keys
// (command, args, env.OKF_MANAGED) are always set to the desired values.
func TestInspectJSONMCPPreservesUnknownKeys(t *testing.T) {
	current := []byte(`{
  "mcpServers": {
    "okf": {
      "command": "old-okf",
      "args": ["mcp"],
      "env": {"OKF_MANAGED": "agentconfig-v1", "MY_CUSTOM_VAR": "preserved"},
      "customField": "do-not-touch"
    },
    "other-server": {"command": "other"}
  }
}`)
	cmd := []string{"okf", "mcp", "--repo", "."}
	proposed, st, act, err := inspectJSONMCP(current, cmd)
	if err != nil {
		t.Fatalf("inspectJSONMCP error: %v", err)
	}
	if st != StatusDrifted {
		t.Fatalf("status = %v, want drifted", st)
	}
	if act != ActionMerge {
		t.Fatalf("action = %v, want merge", act)
	}

	var root map[string]any
	if err := json.Unmarshal(proposed, &root); err != nil {
		t.Fatalf("proposed not valid JSON: %v\n%s", err, proposed)
	}
	servers := root["mcpServers"].(map[string]any)
	okf := servers["okf"].(map[string]any)

	// Managed keys updated
	if okf["command"] != "okf" {
		t.Errorf("command = %v, want okf", okf["command"])
	}
	// Custom top-level key preserved
	if okf["customField"] != "do-not-touch" {
		t.Errorf("customField = %v, want 'do-not-touch' (user key dropped)", okf["customField"])
	}
	// Custom env var preserved
	env := okf["env"].(map[string]any)
	if env["MY_CUSTOM_VAR"] != "preserved" {
		t.Errorf("MY_CUSTOM_VAR = %v, want 'preserved' (user env var dropped)", env["MY_CUSTOM_VAR"])
	}
	// OKF_MANAGED always set correctly
	if env[OwnedMarkerKey] != OwnedMarkerValue {
		t.Errorf("OKF_MANAGED = %v, want %v", env[OwnedMarkerKey], OwnedMarkerValue)
	}
	// Other server preserved
	if _, ok := servers["other-server"]; !ok {
		t.Error("other-server was dropped")
	}
}

// TestInspectJSONMCPUnownedConflict proves that an okf entry without the
// ownership marker is a conflict (D3), not silently overwritten.
func TestInspectJSONMCPUnownedConflict(t *testing.T) {
	current := []byte(`{"mcpServers":{"okf":{"command":"user-custom"}}}`)
	_, _, _, err := inspectJSONMCP(current, []string{"okf", "mcp"})
	if err == nil {
		t.Fatal("expected conflict for unowned okf entry, got nil")
	}
}

// TestApplyFailsOnUnownedRulesFile proves that Apply fails closed when a
// whole-file managed artifact (Cursor rule, Claude skill) exists without the
// OKF ownership header. The user's file must be preserved byte-for-byte (D5).
func TestApplyFailsOnUnownedRulesFile(t *testing.T) {
	root := t.TempDir()
	// Create mcp.json (OKF-managed, will be fine) and an unowned rules file.
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "mcp.json"),
		[]byte(`{"mcpServers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	userRule := []byte("# My custom rule\n# Do not overwrite\n")
	if err := os.WriteFile(filepath.Join(root, ".cursor", "rules", "okf.md"), userRule, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root, []string{"okf", "mcp", "--repo", "."})
	err := svc.Apply("cursor", true)
	if err == nil {
		t.Fatal("Apply should fail on unowned rules file")
	}
	var ace *AgentConfigError
	if !errors.As(err, &ace) {
		t.Fatalf("expected AgentConfigError, got %T: %v", err, err)
	}
	if ace.Code != ErrAgentConfigConflict {
		t.Errorf("code = %q, want %q", ace.Code, ErrAgentConfigConflict)
	}
	// User's file must be preserved byte-for-byte.
	got, rerr := os.ReadFile(filepath.Join(root, ".cursor", "rules", "okf.md"))
	if rerr != nil {
		t.Fatalf("rules file missing after failed apply: %v", rerr)
	}
	if !bytes.Equal(got, userRule) {
		t.Errorf("rules file was modified: got %q, want %q", got, userRule)
	}
}
