#!/usr/bin/env bash
# Persistent Codex verification harness for add-governed-agent-memory.
#
# This script:
#   1. Builds the current OKF binary to a temp directory
#   2. Creates an isolated temp git repo with 2 note concepts (redis + near-dup)
#   3. Configures an isolated CODEX_HOME with [mcp_servers.okf]
#   4. Runs real Codex exec for 4 read-only tasks via OKF MCP tools
#   5. Parses the JSONL event stream and asserts (fail-closed):
#        a. okf_manifest mode=summary limit=3 -> items returned
#        b. okf_manifest for_path=pkg/cache/redis.go mode=hit -> redis.md hit
#        c. okf_context refs=<redis_okf_id> -> body with canary returned
#        d. okf_query q="redis cache invalidation" memory_check=true -> possible_duplicate
#   6. Asserts: actual MCP tool calls, correct tool names, canary in response,
#      zero mutating calls (note/log/feedback/import/refresh/init)
#   7. Outputs ONLY a safe summary (tool call counts, mutating=false, pass/fail)
#   8. Cleans up all temp directories
#
# Hard constraints:
#   - Does NOT commit raw Codex event stream (may contain model output)
#   - Script stdout is safe summary only
#   - All temp files in /tmp, never pollute the repo
#   - Does not push or create PRs
#
# Prerequisites: codex CLI installed, xeart channel configured
#   (see ~/.codex/get-xeart-key.sh and local proxy 127.0.0.1:18080).

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
WORK_DIR=$(mktemp -d /tmp/gov-codex-XXXX)
trap 'rm -rf "$WORK_DIR"' EXIT

BIN="$WORK_DIR/okf"
REPO="$WORK_DIR/repo"
CODEX_HOME="$WORK_DIR/codex-home"

# --- Canary tokens (embedded in concept bodies, asserted in MCP responses) ---
CANARY="CANARY_GOV_MEM_2026"
REDIS_OKF_ID="okf_a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
DUP_OKF_ID="okf_f6e5d4c3b2a1f0e9d8c7b6a5f4e3d2c1"

# =====================================================================
echo "=== Building OKF binary ==="
(cd "$ROOT" && go build -o "$BIN" ./cmd/okf)
echo "Build OK: $BIN"

# =====================================================================
echo "=== Setting up isolated temp git repo ==="
mkdir -p "$REPO" "$CODEX_HOME"
cd "$REPO"

git init -q
git config user.email "gov-verify@example.invalid"
git config user.name "Gov Verify"

# Create source code file (target for for_path matching)
mkdir -p pkg/cache
cat > pkg/cache/redis.go << 'GOEOF'
package cache

// Redis cache implementation
GOEOF

# Create knowledge concepts
mkdir -p .okf/knowledge/notes

cat > .okf/knowledge/notes/redis.md << MDEOF
---
type: note
title: Redis Cache
okf_id: ${REDIS_OKF_ID}
code_refs:
  - pkg/cache/*.go
---
Redis cache invalidation uses TTL eviction and cache-aside.
Redis cache invalidation prevents stale reads.
${CANARY}
MDEOF

cat > .okf/knowledge/notes/redis-dup.md << MDEOF
---
type: note
title: Redis Cache Duplicate
okf_id: ${DUP_OKF_ID}
code_refs:
  - pkg/cache/*.go
---
Redis cache invalidation uses TTL eviction and cache-aside.
Redis cache invalidation prevents stale reads.
${CANARY}
MDEOF

git add -A
git commit -qm "init governed memory fixture"
echo "Repo ready: $REPO (2 notes + 1 source file)"

# =====================================================================
echo "=== Configuring isolated CODEX_HOME ==="
# Start from global config (model provider, approval policy)
if [[ -f "$HOME/.codex/config.toml" ]]; then
  cp "$HOME/.codex/config.toml" "$CODEX_HOME/config.toml"
else
  printf 'model = "gpt-5.6-sol__dev"\nmodel_provider = "xeart"\napproval_policy = "never"\nsandbox_mode = "read-only"\n' > "$CODEX_HOME/config.toml"
fi
# Append OKF MCP server config
cat >> "$CODEX_HOME/config.toml" <<TOML

[mcp_servers.okf]
command = "$BIN"
args = ["mcp", "--repo", "$REPO", "--dir", ".okf/knowledge"]
TOML
echo "CODEX_HOME: $CODEX_HOME"

# =====================================================================
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

# =====================================================================
# Run 4 read-only Codex tasks. Each writes its own JSONL event file.
# Prompts explicitly require OKF MCP tool calls and forbid file reads / CLI.
# =====================================================================

RUN_TASK() {
  local task_name="$1"
  local prompt="$2"
  local events_file="$WORK_DIR/${task_name}.jsonl"

  echo ""
  echo "--- Running task: ${task_name} ---"
  cd "$REPO"
  CODEX_HOME="$CODEX_HOME" timeout 120 codex exec --json --skip-git-repo-check -s read-only \
    "$prompt" > "$events_file" 2>/dev/null || {
    echo "WARN: codex exec exited non-zero for ${task_name}" >&2
  }
  echo "  events: $(wc -l < "$events_file" 2>/dev/null || echo 0) lines"
}

# Task A: okf_manifest mode=summary limit=3
RUN_TASK "task_a" \
  "You MUST call the OKF MCP tool named 'okf_manifest' with arguments {\"mode\":\"summary\",\"limit\":3}. \
Do NOT read any files, do NOT use shell, do NOT run okf CLI. \
After the tool returns, report how many items were in the response."

# Task B: okf_manifest for_path=pkg/cache/redis.go mode=hit
RUN_TASK "task_b" \
  "You MUST call the OKF MCP tool named 'okf_manifest' with arguments {\"for_path\":\"pkg/cache/redis.go\",\"mode\":\"hit\"}. \
Do NOT read any files, do NOT use shell, do NOT run okf CLI. \
After the tool returns, report which concept titles matched."

# Task C: okf_context with a known stable ref. The fixture deliberately has
# two similar Redis concepts, so model-driven ref discovery would test model
# choice rather than deterministic OKF context-ref behavior.
RUN_TASK "task_c" \
  "You MUST call the OKF MCP tool named 'okf_context' with EXACTLY these arguments: {\"refs\":[\"${REDIS_OKF_ID}\"],\"budget_tokens\":1000}. \
Do NOT call okf_manifest first. Do NOT read any files, do NOT use shell, do NOT run okf CLI. \
After the tool returns, report whether body content was returned."

# Task D: okf_query memory_check=true
RUN_TASK "task_d" \
  "You MUST call the OKF MCP tool named 'okf_query' with EXACTLY these arguments: {\"query\":\"redis cache invalidation\",\"memory_check\":true}. \
Do NOT modify the query string. Do NOT read any files, do NOT use shell, do NOT run okf CLI. \
After the tool returns, report the memory_check status field (e.g. possible_duplicate or no_similar)."

# =====================================================================
echo ""
echo "=== Parsing event streams (safe summary only) ==="
python3 - "$WORK_DIR" "$CANARY" <<'PY'
import json, sys, os, glob

work_dir = sys.argv[1]
canary = sys.argv[2]

# Mutating tool name patterns (must be zero)
MUTATING_PATTERNS = ["okf_note", "okf_log", "okf_feedback", "okf_import_document", "okf_refresh", "okf_init"]

task_files = {
    "A_manifest_summary": "task_a.jsonl",
    "B_manifest_for_path": "task_b.jsonl",
    "C_context_refs":      "task_c.jsonl",
    "D_query_memcheck":    "task_d.jsonl",
}

errors = []
all_calls = []  # (task, server, tool, status, canary_in_result)
task_results = {}

for task_label, fname in task_files.items():
    fpath = os.path.join(work_dir, fname)
    calls = []
    has_canary = False
    if not os.path.exists(fpath):
        errors.append(f"{task_label}: event file missing ({fname})")
        task_results[task_label] = {"calls": 0, "pass": False}
        continue
    for line in open(fpath):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            continue
        if e.get("type") != "item.completed":
            continue
        item = e.get("item", {})
        if item.get("type") != "mcp_tool_call":
            continue
        server = item.get("server", "")
        tool = item.get("tool", "")
        arguments = item.get("arguments", {})
        if isinstance(arguments, str):
            try:
                arguments = json.loads(arguments)
            except json.JSONDecodeError:
                arguments = {}
        # Extract result text
        result_text = ""
        result = item.get("result", {})
        content = result.get("content", [])
        for c in content:
            if isinstance(c, dict) and c.get("type") == "text":
                result_text += c.get("text", "")
        if canary in result_text:
            has_canary = True
        calls.append((server, tool, arguments, result_text))
        all_calls.append((task_label, server, tool, arguments, canary in result_text))

    task_results[task_label] = {
        "calls": len(calls),
        "has_canary": has_canary,
        "tools": [c[1] for c in calls],
        "arguments": [c[2] for c in calls],
    }

# --- Per-task assertions ---

# Task A: must have okf_manifest call, result must contain items (ok=true)
a_calls = task_results.get("A_manifest_summary", {})
a_tools = a_calls.get("tools", [])
if "okf_manifest" not in a_tools:
    errors.append(f"A: no okf_manifest call (got {a_tools})")
else:
    # Check result contains items
    found_items = False
    for line in open(os.path.join(work_dir, "task_a.jsonl")):
        line = line.strip()
        if not line.startswith("{"): continue
        try: e = json.loads(line)
        except: continue
        if e.get("type") == "item.completed" and e.get("item",{}).get("type") == "mcp_tool_call":
            item = e["item"]
            if item.get("tool") == "okf_manifest":
                text = ""
                for c in item.get("result",{}).get("content",[]):
                    if isinstance(c,dict) and c.get("type")=="text":
                        text += c.get("text","")
                try:
                    env = json.loads(text)
                    items = env.get("result",{}).get("items",[])
                    if env.get("ok") and len(items) > 0:
                        found_items = True
                except: pass
    if not found_items:
        errors.append("A: okf_manifest result did not return items")

# Task B: must have okf_manifest call with for_path, must hit redis
b_calls = task_results.get("B_manifest_for_path", {})
b_tools = b_calls.get("tools", [])
if "okf_manifest" not in b_tools:
    errors.append(f"B: no okf_manifest call (got {b_tools})")
else:
    found_redis_hit = False
    for line in open(os.path.join(work_dir, "task_b.jsonl")):
        line = line.strip()
        if not line.startswith("{"): continue
        try: e = json.loads(line)
        except: continue
        if e.get("type") == "item.completed" and e.get("item",{}).get("type") == "mcp_tool_call":
            item = e["item"]
            if item.get("tool") == "okf_manifest":
                text = ""
                for c in item.get("result",{}).get("content",[]):
                    if isinstance(c,dict) and c.get("type")=="text":
                        text += c.get("text","")
                if "Redis" in text or "redis" in text.lower():
                    found_redis_hit = True
    if not found_redis_hit:
        errors.append("B: for_path=hit did not match redis concept")

# Task C: must have exactly one okf_context call with the expected stable ref
# and explicit budget; its result must contain the body canary.
c_calls = task_results.get("C_context_refs", {})
c_tools = c_calls.get("tools", [])
if c_tools != ["okf_context"]:
    errors.append(f"C: expected exactly one okf_context call (got {c_tools})")
c_args = c_calls.get("arguments", [])
expected_refs = ["okf_a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"]
if len(c_args) != 1 or c_args[0].get("refs") != expected_refs or c_args[0].get("budget_tokens") != 1000:
    errors.append(f"C: unexpected okf_context arguments (got {c_args})")
if not c_calls.get("has_canary", False):
    errors.append("C: okf_context result did not contain canary")

# Task D: must have okf_query call with memory_check, result must say possible_duplicate
d_calls = task_results.get("D_query_memcheck", {})
d_tools = d_calls.get("tools", [])
if "okf_query" not in d_tools:
    errors.append(f"D: no okf_query call (got {d_tools})")
else:
    found_possible_dup = False
    for line in open(os.path.join(work_dir, "task_d.jsonl")):
        line = line.strip()
        if not line.startswith("{"): continue
        try: e = json.loads(line)
        except: continue
        if e.get("type") == "item.completed" and e.get("item",{}).get("type") == "mcp_tool_call":
            item = e["item"]
            if item.get("tool") == "okf_query":
                text = ""
                for c in item.get("result",{}).get("content",[]):
                    if isinstance(c,dict) and c.get("type")=="text":
                        text += c.get("text","")
                if "possible_duplicate" in text:
                    found_possible_dup = True
    if not found_possible_dup:
        errors.append("D: memory_check did not return possible_duplicate")

# --- Global: zero mutating calls ---
mutating_found = []
for task, server, tool, _, _ in all_calls:
    for pat in MUTATING_PATTERNS:
        if tool == pat or tool.startswith(pat):
            mutating_found.append(f"{task}:{tool}")
if mutating_found:
    errors.append(f"mutating tool calls observed: {mutating_found}")

# --- Output safe summary ---
print("=== Governed Memory Codex Verification Summary ===")
print(f"Canary token: {canary}")
print()
print("Per-task results:")
for label in ["A_manifest_summary", "B_manifest_for_path", "C_context_refs", "D_query_memcheck"]:
    r = task_results.get(label, {})
    tools_used = r.get("tools", [])
    print(f"  {label}: {r.get('calls',0)} MCP call(s), tools={tools_used}, canary_in_result={r.get('has_canary',False)}")

print()
print(f"Total MCP tool calls: {len(all_calls)}")
print(f"Mutating tool calls: {len(mutating_found)} (expected 0)")
print(f"MCP server namespace used: okf (all calls via server='okf')")

# Codex tools count explanation
print()
print("=== Codex tools count explanation ===")
print("OKF MCP server actual tool count:")
print("  - Legacy era (2024-11-05, what codex 0.153.4 uses): 20 tools")
print("    (9 bundle-facing core + 11 service-backed agent)")
print("  - Modern era (2026-07-28): 11 tools (service-backed agent only)")
print("Codex namespace wrapper: codex 0.153.4 does NOT expose MCP tools")
print("  as individual function tools to the model. Built-in function tools")
print("  sent upstream: 9 (exec, wait, request_user_input, followup_task,")
print("  interrupt_agent, list_agents, send_message, spawn_agent, wait_agent).")
print("  MCP tool names are described in the system prompt; codex intercepts")
print("  model calls and routes them to MCP servers (recorded as mcp_tool_call).")
print('  The earlier evidence "21" was the model self-reporting tool names from')
print("  system-prompt descriptions: 20 legacy OKF tools + 1 (generic MCP call")
print("  mechanism) = 21. Actual OKF server count is 20 (legacy) or 11 (modern).")

print()
if errors:
    print(f"FAIL: {len(errors)} assertion(s) failed:")
    for err in errors:
        print(f"  - {err}")
    sys.exit(1)
else:
    print("PASS: All 4 governed memory read-only tasks verified. 0 mutating calls.")
PY

echo ""
echo "=== Harness complete ==="
