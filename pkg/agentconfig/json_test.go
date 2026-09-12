package agentconfig

import (
	"encoding/json"
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
