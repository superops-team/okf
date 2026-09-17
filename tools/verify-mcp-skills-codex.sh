#!/usr/bin/env bash
# Real Codex Resource compatibility harness for add-mcp-skills-extension (S35).
#
# This script:
#   1. Builds the OKF binary from the current tree
#   2. Configures an isolated CODEX_HOME with OKF MCP server
#   3. Runs real Codex to list and read skill://okf/SKILL.md
#   4. Parses the JSONL event stream and outputs ONLY a safe summary
#   5. Fails closed if any assertion fails
#
# It does NOT commit or print the raw event stream (which may contain
# model-internal tokens). The summary is safe for repository evidence.
#
# Prerequisites: codex CLI installed, xeart channel configured
#   (see ~/.codex/get-xeart-key.sh and local proxy 127.0.0.1:18080).

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

BIN="$WORK_DIR/okf"
REPO="$WORK_DIR/repo"
CODEX_HOME="$WORK_DIR/codex-home"
EVENTS="$WORK_DIR/events.jsonl"

echo "=== Building OKF binary ==="
(cd "$ROOT" && go build -o "$BIN" ./cmd/okf)
echo "Build OK: $BIN"

echo "=== Setting up isolated Codex HOME ==="
mkdir -p "$REPO" "$CODEX_HOME"
# Start from global config (model provider, approval policy)
if [[ -f "$HOME/.codex/config.toml" ]]; then
  cp "$HOME/.codex/config.toml" "$CODEX_HOME/config.toml"
else
  printf 'model = "gpt-5.6-sol__dev"\nmodel_provider = "xeart"\napproval_policy = "never"\nsandbox_mode = "danger-full-access"\n' > "$CODEX_HOME/config.toml"
fi
# Append OKF MCP server config
cat >> "$CODEX_HOME/config.toml" <<TOML

[mcp_servers.okf]
command = "$BIN"
args = ["mcp", "--repo", "$REPO"]
TOML
echo "CODEX_HOME: $CODEX_HOME"

echo "=== Ensuring xeart channel ==="
export XEART_API_KEY="$("$HOME/.codex/get-xeart-key.sh" 2>/dev/null || echo "")"
if [[ -z "$XEART_API_KEY" ]]; then
  echo "BLOCKED_AUTH: no xeart API key available" >&2
  exit 3
fi
if ! curl -s http://127.0.0.1:18080/healthz >/dev/null 2>&1; then
  echo "BLOCKED_PROXY: xeart proxy not running on 127.0.0.1:18080" >&2
  exit 4
fi

echo "=== Running real Codex (list + read skill resource) ==="
PROMPT='List all MCP resources you can see. Then read the resource with URI skill://okf/SKILL.md. Report: (1) the resource name, (2) whether it contains W01 and W07 workflow clauses, (3) whether you called any mutating tool (okf_note, okf_init, okf_refresh, okf_feedback). Do not call any other tools.'

cd "$REPO"
CODEX_HOME="$CODEX_HOME" timeout 120 codex exec --json --skip-git-repo-check -s read-only "$PROMPT" > "$EVENTS" 2>/dev/null || {
  echo "Codex exec failed (exit $?)" >&2
  exit 5
}

echo "=== Parsing event stream (safe summary only) ==="
python3 - "$EVENTS" <<'PY'
import json, sys

events_path = sys.argv[1]
calls = []
final = ""
for line in open(events_path):
    try:
        e = json.loads(line)
    except json.JSONDecodeError:
        continue
    item = e.get("item", {})
    if e.get("type") == "item.completed" and item.get("type") == "mcp_tool_call":
        calls.append((item.get("server"), item.get("tool"), item.get("status")))
    if e.get("type") == "item.completed" and item.get("type") == "agent_message":
        final = item.get("text", "")

# Assertions (fail closed)
errors = []

# Must have listed resources
if not any(c[1] == "list_mcp_resources" for c in calls):
    errors.append("no list_mcp_resources call observed")

# Must have read the OKF skill resource
okf_reads = [c for c in calls if c[0] == "okf" and "read" in c[1].lower()]
if not okf_reads:
    errors.append("no okf resource read call observed")

# Resource name must be okf
if "okf" not in final.lower():
    errors.append("final answer does not mention resource name okf")

# Must contain W01 and W07
if "w01" not in final.lower():
    errors.append("final answer does not mention W01")
if "w07" not in final.lower():
    errors.append("final answer does not mention W07")

# No mutating tools
mutating = [c for c in calls if any(m in c[1].lower() for m in ["note", "init", "refresh", "feedback"])]
if mutating:
    errors.append(f"mutating tool call observed: {mutating}")

# Output safe summary
print("=== Codex Resource Compatibility Summary (S35) ===")
print(f"MCP tool calls: {len(calls)}")
for server, tool, status in calls:
    print(f"  - {server}.{tool} ({status})")
print(f"Final answer mentions: name=okf={'okf' in final.lower()}, W01={'w01' in final.lower()}, W07={'w07' in final.lower()}")
print(f"Mutating tools: {len(mutating)}")

if errors:
    print(f"\nFAIL: {'; '.join(errors)}", file=sys.stderr)
    sys.exit(1)
print("\nPASS: Codex Resource compatibility verified")
PY

echo ""
echo "=== Harness complete ==="
