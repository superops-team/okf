package agentconfig

import (
	"encoding/json"
	"fmt"
)

// serverName is the mcpServers entry key owned by OKF.
const serverName = "okf"

// inspectJSONMCP computes the proposed mcp.json bytes for a client adapter.
// It preserves the semantic value of every unknown key and every non-OKF MCP
// server. A same-name entry lacking the ownership marker, malformed JSON, or a
// wrong-typed host structure is a conflict and returns nil bytes.
//
// Returns: proposed bytes (nil for no change), item status, item action and a
// conflict/parse error.
func inspectJSONMCP(current []byte, cmd []string) (proposed []byte, st FileStatus, act FileAction, err error) {
	root := map[string]any{}
	if len(current) > 0 {
		if uerr := json.Unmarshal(current, &root); uerr != nil {
			return nil, StatusConflict, ActionNoChange,
				errConflict(fmt.Sprintf("host JSON is malformed: %v", uerr))
		}
	}

	mcpServers, has := root["mcpServers"]
	if !has {
		mcpServers = map[string]any{}
		root["mcpServers"] = mcpServers
	}
	servers, ok := mcpServers.(map[string]any)
	if !ok {
		return nil, StatusConflict, ActionNoChange,
			errConflict("\"mcpServers\" is not a JSON object")
	}

	desired := RenderJSONMCPEntry(cmd)
	owned := false
	if existing, present := servers[serverName]; present {
		entry, okm := existing.(map[string]any)
		if !okm {
			return nil, StatusConflict, ActionNoChange,
				errConflict(fmt.Sprintf("mcpServers.%s is not a JSON object", serverName))
		}
		env, _ := entry["env"].(map[string]any)
		marker, _ := env[OwnedMarkerKey].(string)
		if marker != OwnedMarkerValue {
			return nil, StatusConflict, ActionNoChange,
				errConflict(fmt.Sprintf("mcpServers.%s exists without the %s ownership marker", serverName, OwnedMarkerKey))
		}
		owned = true
		// Merge: preserve user-added keys inside the OKF-managed entry.
		// Known managed keys (command, args, env) are overwritten with the
		// desired values; any other keys the user added are preserved.
		// Custom env vars are merged (OKF_MANAGED is always set correctly).
		for k, v := range entry {
			if k == "command" || k == "args" || k == "env" {
				continue // managed by OKF
			}
			if _, exists := desired[k]; !exists {
				desired[k] = v
			}
		}
		// Merge env: preserve custom env vars
		if desiredEnv, ok := desired["env"].(map[string]any); ok {
			for k, v := range env {
				if k == OwnedMarkerKey {
					continue // always managed
				}
				if _, exists := desiredEnv[k]; !exists {
					desiredEnv[k] = v
				}
			}
		}
	}
	servers[serverName] = desired

	out, merr := json.MarshalIndent(root, "", "  ")
	if merr != nil {
		return nil, StatusConflict, ActionNoChange, errConflict("cannot encode mcp config: " + merr.Error())
	}
	out = append(out, '\n')

	switch {
	case len(current) == 0:
		return out, StatusMissing, ActionCreate, nil
	case byteEqual(current, out):
		return current, StatusInstalled, ActionNoChange, nil
	case owned:
		return out, StatusDrifted, ActionMerge, nil
	default:
		return out, StatusMissing, ActionMerge, nil
	}
}

// removeJSONMCP computes the bytes that remove the OKF-owned mcpServers entry.
// It is a conflict when the entry is absent or lacks the ownership marker.
func removeJSONMCP(current []byte) (proposed []byte, err error) {
	root := map[string]any{}
	if len(current) > 0 {
		if uerr := json.Unmarshal(current, &root); uerr != nil {
			return nil, errConflict("host JSON is malformed: " + uerr.Error())
		}
	}
	servers, _ := root["mcpServers"].(map[string]any)
	existing, present := servers[serverName]
	if !present {
		return nil, nil // nothing to remove: treat as already absent
	}
	entry, ok := existing.(map[string]any)
	if !ok {
		return nil, errConflict(fmt.Sprintf("mcpServers.%s is not a JSON object", serverName))
	}
	env, _ := entry["env"].(map[string]any)
	if marker, _ := env[OwnedMarkerKey].(string); marker != OwnedMarkerValue {
		return nil, errConflict(fmt.Sprintf("mcpServers.%s lacks the ownership marker; refusing to remove unowned entry", serverName))
	}
	delete(servers, serverName)
	if len(servers) == 0 {
		delete(root, "mcpServers")
	}
	out, merr := json.MarshalIndent(root, "", "  ")
	if merr != nil {
		return nil, errConflict("cannot encode mcp config: " + merr.Error())
	}
	return append(out, '\n'), nil
}

func byteEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
