#!/usr/bin/env bash
# Authorized Codex review-flow harness for add-temporal-memory-relations (S43).
#
# This is a REAL Codex + OKF-MCP end-to-end governance check. It:
#   1. Builds the current OKF binary into /tmp.
#   2. Creates an isolated temp git repo with a knowledge dir.
#   3. Seeds an APPROVED note A via the durable write (okf_note through MCP),
#      capturing its okf_id/ref from the JSON envelope.
#   4. Installs the OKF MCP server into an isolated CODEX_HOME
#      ([mcp_servers.okf], sandbox=read-only, approval=never base config).
#   5. Phase A (READ-ONLY): Codex lists current memory + the review queue.
#      ASSERT: zero mutating calls; A is current; P is the pending proposal.
#   6. Seeds a PROPOSED note P (memory_state=proposed, confidence, evidence,
#      updates->A). Current view must still show A (not P).
#   7. Phase B (AUTHORIZED): only after an explicit authorization line in the
#      prompt, Codex approves P via okf_memory_review (CAS).
#      ASSERT: exactly one okf_memory_review, action=approve, ref=P,
#      expected_state=proposed, result proposed->approved.
#   8. Phase C (VERIFY): Codex re-queries current + history.
#      ASSERT: current view now shows P (A historical), history returns the
#      ordered A<-P chain.
#
# Hard requirements:
#   - Parses actual mcp_tool_call events from `codex --json` JSONL; asserts real
#     tool names (okf_query / okf_memory_review / okf_context) and key args.
#   - Counts mutating calls per phase. Phase A and Phase C MUST be 0 mutations.
#   - Redacts secrets: never prints raw JWT/keys; only names, key args, pass/fail.
#   - set -euo pipefail; trap EXIT cleanup; non-zero exit on any assertion
#     failure or when Codex never calls an expected tool.
#   - Does NOT modify Go source.
#
# Prereqs: codex CLI installed, xeart proxy on 127.0.0.1:18080 (healthy).

set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
WORK_DIR=$(mktemp -d /tmp/temporal-codex-XXXX)
trap 'rm -rf "$WORK_DIR"' EXIT

BIN="$WORK_DIR/okf"
REPO="$WORK_DIR/repo"
CODEX_HOME="$WORK_DIR/codex-home"
MCP_CALL="$ROOT/tools/mcp_call.py"

# Low-reasoning / low-load model for speed; overridable. Recorded in the summary.
MODEL="${TEMPORAL_MODEL:-gpt-5.6-sol__dev}"

# --- Fixture tokens (embedded in concept bodies; asserted in MCP responses) ---
SHARED="TemporalRoutingQuorum"
A_TITLE="Approved Temporal Quorum v1"
P_TITLE="Proposed Temporal Quorum v2"
A_CANARY="CANARY_TEMP_A_2026"
P_CANARY="CANARY_TEMP_P_2026"

# =====================================================================
echo "=== Building OKF binary (current source, /tmp) ==="
(cd "$ROOT" && go build -o "$BIN" ./cmd/okf)
echo "Build OK: $BIN"

# =====================================================================
echo "=== Setting up isolated temp git repo ==="
mkdir -p "$REPO" "$CODEX_HOME"
cd "$REPO"
git init -q
git config user.email "temporal-verify@example.invalid"
git config user.name "Temporal Verify"
mkdir -p .okf/knowledge/notes
# A placeholder source file so the initial commit has content (git does not
# track empty dirs); the knowledge bundle is seeded afterwards via MCP.
cat > placeholder.go <<'GOEOF'
package cache

// Placeholder source for the temporal memory verification repo.
GOEOF
git add -A
git commit -qm "init temporal memory fixture"
echo "Repo ready: $REPO"

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
# Helper: call the OKF MCP server over stdio and extract one field from the
# tool envelope (result.<field>). Prints the field value to stdout.
#   usage: MCP_FIELD <tool_name> <arg-json> <field>
# Exits non-zero if the tool call failed (isError) or the field is absent.
MCP_FIELD() {
  local tool_name="$1" arg_json="$2" field="$3"
  python3 - "$MCP_CALL" "$BIN" "$REPO" "$tool_name" "$arg_json" "$field" <<'PY'
import json, subprocess, sys

mcp_call, binary, repo, tool, args, field = sys.argv[1:7]
proc = subprocess.Popen(
    [binary, "mcp", "--repo", repo, "--dir", ".okf/knowledge"],
    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
    text=True, bufsize=1,
)

def send(msg):
    data = json.dumps(msg)
    proc.stdin.write(f"Content-Length: {len(data)}\r\n\r\n{data}")
    proc.stdin.flush()

def recv():
    headers = {}
    while True:
        line = proc.stdout.readline()
        if not line:
            return None
        line = line.strip()
        if not line:
            break
        if ":" in line:
            k, v = line.split(":", 1)
            headers[k.strip().lower()] = v.strip()
    n = int(headers.get("content-length", "0"))
    body = proc.stdout.read(n) if n > 0 else ""
    return json.loads(body) if body else None

def rpc(method, params, mid):
    send({"jsonrpc": "2.0", "id": mid, "method": method, "params": params})
    return recv()

try:
    rpc("initialize", {"protocolVersion": "2024-11-05", "capabilities": {},
                       "clientInfo": {"name": "seed-verify", "version": "0"}}, 1)
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    if tool != "okf_load_bundle":
        rpc("tools/call", {"name": "okf_load_bundle", "arguments": {}}, 2)
    res = rpc("tools/call", {"name": tool, "arguments": json.loads(args)}, 3)
finally:
    proc.terminate()
    try:
        proc.wait(timeout=3)
    except subprocess.TimeoutExpired:
        proc.kill()

if res is None:
    print("PROTOCOL_ERROR: no response", file=sys.stderr); sys.exit(2)
if "error" in res:
    print("PROTOCOL_ERROR: " + json.dumps(res["error"]), file=sys.stderr); sys.exit(2)
outer = res.get("result", {})
if outer.get("isError"):
    print("TOOL_ERROR: " + json.dumps(outer)[:500], file=sys.stderr); sys.exit(1)
text = "".join(c.get("text", "") for c in outer.get("content", []) if isinstance(c, dict))
env = json.loads(text)
if not env.get("ok"):
    print("ENV_ERROR: " + json.dumps(env.get("error", {}))[:500], file=sys.stderr); sys.exit(1)
r = env.get("result", {}) or {}
print(r.get(field, ""))
PY
}

# =====================================================================
echo "=== Seeding APPROVED note A via durable write (okf_note) ==="
A_CONTENT=$(printf '%s\n\nThe %s policy is the approved baseline for durable memory updates. %s. It is the authoritative v1 rule.' \
  "$A_TITLE" "$SHARED" "$A_CANARY")
A_ARGS=$(python3 -c 'import json,sys; print(json.dumps({"content":sys.argv[1],"idempotency_key":"temporal-seed-a-v1","tags":["seed","approved"]}))' "$A_CONTENT")
A_OKF_ID="$(MCP_FIELD okf_note "$A_ARGS" okf_id)"
A_REF="$(MCP_FIELD okf_note "$A_ARGS" ref)"
if [[ -z "$A_OKF_ID" ]]; then echo "FAIL: did not capture A okf_id" >&2; exit 1; fi
echo "  A okf_id: $A_OKF_ID"
echo "  A ref:    $A_REF"

# =====================================================================
echo "=== Seeding PROPOSED note P (proposed, updates->A) ==="
P_CONTENT=$(printf '%s\n\nThe %s policy is refined. %s. This v2 proposal supersedes v1 after approval.' \
  "$P_TITLE" "$SHARED" "$P_CANARY")
P_ARGS=$(python3 -c 'import json,sys; print(json.dumps({
  "content": sys.argv[1],
  "idempotency_key": "temporal-seed-p-v1",
  "tags": ["seed","proposed"],
  "memory_state": "proposed",
  "memory_confidence": 0.6,
  "evidence_refs": ["evid/seed-a"],
  "memory_relation_kind": "updates",
  "memory_relation_targets": [sys.argv[2]],
}))' "$P_CONTENT" "$A_OKF_ID")
P_OKF_ID="$(MCP_FIELD okf_note "$P_ARGS" okf_id)"
P_REF="$(MCP_FIELD okf_note "$P_ARGS" ref)"
if [[ -z "$P_OKF_ID" || "$P_OKF_ID" == "$A_OKF_ID" ]]; then
  echo "FAIL: did not capture distinct P okf_id (got '$P_OKF_ID')" >&2; exit 1
fi
echo "  P okf_id: $P_OKF_ID"
echo "  P ref:    $P_REF"

# Sanity: current view must STILL show A, not P (quarantine) before any Codex run.
echo "=== Pre-flight: CLI confirms A current, P quarantined ==="
"$BIN" tool query --repo "$REPO" --dir .okf/knowledge -q "$SHARED" --memory-view current --json \
  | python3 -c 'import json,sys; e=json.load(sys.stdin); t=" ".join(r.get("title","") for r in e["result"]["results"]); print("  current-view titles:", t)'
"$BIN" tool query --repo "$REPO" --dir .okf/knowledge --memory-review-queue --json \
  | python3 -c 'import json,sys; e=json.load(sys.stdin); print("  review-queue items:", [i.get("okf_id") for i in e["result"]["items"]])'

# =====================================================================
echo "=== Configuring isolated CODEX_HOME ==="
if [[ -f "$HOME/.codex/config.toml" ]]; then
  cp "$HOME/.codex/config.toml" "$CODEX_HOME/config.toml"
else
  printf 'model = "%s"\nmodel_provider = "xeart"\napproval_policy = "never"\nsandbox_mode = "read-only"\n' "$MODEL" > "$CODEX_HOME/config.toml"
fi
# Force deterministic approval/sandbox posture regardless of the global copy.
cat >> "$CODEX_HOME/config.toml" <<TOML

approval_policy = "never"
sandbox_mode = "read-only"

[mcp_servers.okf]
command = "$BIN"
args = ["mcp", "--repo", "$REPO", "--dir", ".okf/knowledge"]
TOML
echo "CODEX_HOME: $CODEX_HOME (model=$MODEL)"

# =====================================================================
# Run three Codex phases. Each writes its own JSONL event file + .err.
# =====================================================================
RUN_PHASE() {
  local name="$1" sandbox="$2" prompt="$3"
  local events="$WORK_DIR/${name}.jsonl"
  local stderr_log="$WORK_DIR/${name}.err"
  echo ""
  echo "--- Phase: ${name} (sandbox=${sandbox}, model=${MODEL}) ---"
  cd "$REPO"
  set +e
  CODEX_HOME="$CODEX_HOME" timeout 300 codex exec --json --skip-git-repo-check -s "$sandbox" -m "$MODEL" \
    "$prompt" > "$events" 2>"$stderr_log"
  local rc=$?
  set -e
  local lines
  lines=$(wc -l < "$events" 2>/dev/null || echo 0)
  echo "  exit=$rc, event lines=$lines"
}

# ---- Phase A: READ-ONLY (must be zero mutations) ----
PROMPT_A="You are verifying temporal memory through the OKF MCP server.
Perform EXACTLY these two read-only okf_query calls, in order, and NO other tool calls.
Do NOT use shell. Do NOT read files. Do NOT call any write, note, log, feedback, review, approve, refresh, or init tool.

Call 1: okf_query with arguments {\"query\":\"$SHARED\",\"memory_view\":\"current\",\"limit\":20}
Call 2: okf_query with arguments {\"memory_review_queue\":true}

After both return, report: (a) the note titles present in the Call 1 current view, and (b) the okf_id and title of the item listed in Call 2."
RUN_PHASE phase_a read-only "$PROMPT_A"

# ---- Phase B: AUTHORIZED (single CAS approve). The MCP server is a child of
# codex; only danger-full-access lets it persist the in-place state transition
# (workspace-write does not propagate write perms to the MCP child, so the call
# returns a null result and nothing is mutated). Read-only phases stay locked. ----
PROMPT_B="You are performing a governed memory review through the OKF MCP server.

>>> EXPLICIT AUTHORIZATION GRANTED <<<
This is a controlled, approved governance authorization. You are permitted to make EXACTLY ONE memory review mutation.

Call the OKF MCP tool okf_memory_review EXACTLY ONCE with these exact arguments:
{\"ref\":\"$P_OKF_ID\",\"action\":\"approve\",\"expected_state\":\"proposed\"}

Do NOT call any other tool. Do NOT use shell. Do NOT read files.
After it returns, report the previous_state and current_state it reported."
RUN_PHASE phase_b danger-full-access "$PROMPT_B"

# ---- Phase C: READ-ONLY (verify the transition) ----
PROMPT_C="You are verifying temporal memory AFTER a review through the OKF MCP server.
Perform EXACTLY these two read-only okf_query calls, in order, and NO other tool calls.
Do NOT use shell. Do NOT read files. Do NOT call any write, note, review, approve, refresh, or init tool.

Call 1: okf_query with arguments {\"query\":\"$SHARED\",\"memory_view\":\"current\",\"limit\":20}
Call 2: okf_query with arguments {\"memory_view\":\"history\",\"refs\":[\"$P_OKF_ID\"]}

After both return, report: (a) the note titles present in the Call 1 current view, and (b) for Call 2 list each chain item's okf_id, memory_state, and whether it is current, in order."
RUN_PHASE phase_c read-only "$PROMPT_C"

# =====================================================================
echo ""
echo "=== Parsing event streams (safe summary only) ==="
python3 - "$WORK_DIR" "$A_OKF_ID" "$P_OKF_ID" "$A_REF" "$P_REF" \
  "$A_TITLE" "$P_TITLE" "$A_CANARY" "$P_CANARY" "$MODEL" <<'PY'
import json, sys, os

(work_dir, A_ID, P_ID, A_REF, P_REF,
 A_TITLE, P_TITLE, A_CANARY, P_CANARY, MODEL) = sys.argv[1:11]

# Mutating MCP tools. Phase A and Phase C MUST observe zero of these.
MUTATING = {
    "okf_memory_review", "okf_note", "okf_log", "okf_feedback",
    "okf_refresh", "okf_init", "okf_import_document",
}

def parse_events(path):
    """Return list of dicts: tool, args(dict), envelope(dict|None), server."""
    calls = []
    if not os.path.exists(path):
        return calls, [f"event file missing: {path}"]
    errs = []
    for lineno, line in enumerate(open(path), 1):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            continue
        if e.get("type") != "item.completed":
            continue
        item = e.get("item") or {}
        if item.get("type") != "mcp_tool_call":
            continue
        server = item.get("server", "")
        tool = item.get("tool", "")
        args = item.get("arguments") or {}
        if isinstance(args, str):
            try:
                args = json.loads(args)
            except json.JSONDecodeError:
                args = {}
        # Envelope is the first text content of the result.
        env = None
        text = ""
        for c in (item.get("result") or {}).get("content", []):
            if isinstance(c, dict) and c.get("type") == "text":
                text += c.get("text", "")
        text = text.strip()
        if text.startswith("{"):
            try:
                env = json.loads(text)
            except json.JSONDecodeError:
                env = None
        calls.append({"server": server, "tool": tool, "args": args or {},
                      "env": env, "raw": text})
    return calls, errs

def env_result(call):
    if call.get("env") and call["env"].get("result") is not None:
        return call["env"].get("result")
    return None

errors = []
summary = {}

# ---- Phase A ----
a_calls, _ = parse_events(os.path.join(work_dir, "phase_a.jsonl"))
a_mut = [c for c in a_calls if c["tool"] in MUTATING]
a_tools = [c["tool"] for c in a_calls]
# Locate the two expected calls
a_current = [c for c in a_calls if c["tool"] == "okf_query" and c["args"].get("memory_view") == "current"]
a_queue = [c for c in a_calls if c["tool"] == "okf_query" and c["args"].get("memory_review_queue") is True]

if not a_calls:
    errors.append("PHASE A: Codex made NO mcp_tool_call events")
if a_mut:
    errors.append(f"PHASE A: {len(a_mut)} mutating call(s) before authorization: {[c['tool'] for c in a_mut]}")
if not a_current:
    errors.append("PHASE A: no okf_query memory_view=current call")
else:
    res = env_result(a_current[-1]) or {}
    titles = [r.get("title", "") for r in (res.get("results") or [])]
    has_a = any(A_TITLE in t for t in titles)
    has_p = any(P_TITLE in t for t in titles)
    if not has_a:
        errors.append(f"PHASE A: current view does NOT show approved note A (titles={titles})")
    if has_p:
        errors.append(f"PHASE A: current view incorrectly shows proposed note P (titles={titles})")
    summary["A_current_titles"] = titles
if not a_queue:
    errors.append("PHASE A: no okf_query memory_review_queue call")
else:
    res = env_result(a_queue[-1]) or {}
    qids = [i.get("okf_id") for i in (res.get("items") or [])]
    qtitles = [i.get("title") for i in (res.get("items") or [])]
    if P_ID not in qids:
        errors.append(f"PHASE A: review queue does not list proposal P (ids={qids})")
    summary["A_queue_ids"] = qids
    summary["A_queue_titles"] = qtitles

# ---- Phase B ----
b_calls, _ = parse_events(os.path.join(work_dir, "phase_b.jsonl"))
b_review = [c for c in b_calls if c["tool"] == "okf_memory_review"]
b_mut = [c for c in b_calls if c["tool"] in MUTATING]
if not b_calls:
    errors.append("PHASE B: Codex made NO mcp_tool_call events (review never happened)")
if len(b_review) != 1:
    errors.append(f"PHASE B: expected exactly 1 okf_memory_review call, got {len(b_review)} ({[c['tool'] for c in b_calls]})")
else:
    c = b_review[0]
    a = c["args"]
    ref_ok = a.get("ref") in (P_ID, P_REF)
    if a.get("action") != "approve":
        errors.append(f"PHASE B: review action={a.get('action')!r}, want 'approve'")
    if a.get("expected_state") != "proposed":
        errors.append(f"PHASE B: review expected_state={a.get('expected_state')!r}, want 'proposed'")
    if not ref_ok:
        errors.append(f"PHASE B: review ref={a.get('ref')!r}, want {P_ID}")
    res = env_result(c) or {}
    if res.get("previous_state") != "proposed":
        errors.append(f"PHASE B: review result previous_state={res.get('previous_state')!r}, want 'proposed'")
    if res.get("current_state") != "approved":
        errors.append(f"PHASE B: review result current_state={res.get('current_state')!r}, want 'approved'")
    summary["B_review_args"] = {k: a.get(k) for k in ("ref", "action", "expected_state")}
    summary["B_review_result"] = {k: res.get(k) for k in ("previous_state", "current_state")}
if len(b_mut) != 1:
    errors.append(f"PHASE B: expected exactly 1 mutating call (the review), got {len(b_mut)}: {[c['tool'] for c in b_mut]}")

# ---- Phase C ----
c_calls, _ = parse_events(os.path.join(work_dir, "phase_c.jsonl"))
c_mut = [c for c in c_calls if c["tool"] in MUTATING]
c_current = [c for c in c_calls if c["tool"] == "okf_query" and c["args"].get("memory_view") == "current"]
c_history = [c for c in c_calls if c["tool"] == "okf_query" and c["args"].get("memory_view") == "history"]
if not c_calls:
    errors.append("PHASE C: Codex made NO mcp_tool_call events")
if c_mut:
    errors.append(f"PHASE C: {len(c_mut)} mutating call(s) after verification: {[c['tool'] for c in c_calls]}")
if not c_current:
    errors.append("PHASE C: no okf_query memory_view=current call")
else:
    res = env_result(c_current[-1]) or {}
    titles = [r.get("title", "") for r in (res.get("results") or [])]
    has_p = any(P_TITLE in t for t in titles)
    has_a = any(A_TITLE in t for t in titles)
    if not has_p:
        errors.append(f"PHASE C: current view does NOT show newly-approved P (titles={titles})")
    if has_a:
        errors.append(f"PHASE C: current view still shows historical A (titles={titles})")
    summary["C_current_titles"] = titles
if not c_history:
    errors.append("PHASE C: no okf_query memory_view=history call")
else:
    res = env_result(c_history[-1]) or {}
    items = res.get("items") or []
    by_id = {i.get("okf_id"): i for i in items}
    if res.get("current_ref") not in (P_ID, P_REF):
        errors.append(f"PHASE C: history current_ref={res.get('current_ref')!r}, want {P_ID}")
    p_item = by_id.get(P_ID)
    a_item = by_id.get(A_ID)
    if p_item is None:
        errors.append(f"PHASE C: history chain missing P (ids={[i.get('okf_id') for i in items]})")
    else:
        if p_item.get("current") is not True:
            errors.append(f"PHASE C: P not marked current in history: {p_item}")
        if p_item.get("memory_state") != "approved":
            errors.append(f"PHASE C: P history memory_state={p_item.get('memory_state')!r}, want approved")
        if p_item.get("relation_kind") != "updates":
            errors.append(f"PHASE C: P relation_kind={p_item.get('relation_kind')!r}, want updates")
        if p_item.get("relation_target") not in (A_ID, A_REF):
            errors.append(f"PHASE C: P relation_target={p_item.get('relation_target')!r}, want {A_ID}")
    if a_item is None:
        errors.append(f"PHASE C: history chain missing historical A (ids={[i.get('okf_id') for i in items]})")
    elif a_item.get("current") is not False:
        errors.append(f"PHASE C: historical A still marked current: {a_item}")
    summary["C_history_items"] = [
        {"okf_id": i.get("okf_id"), "memory_state": i.get("memory_state"),
         "current": i.get("current"), "relation_kind": i.get("relation_kind"),
         "relation_target": i.get("relation_target")}
        for i in items
    ]

# ---- Emit safe summary ----
print("=== Temporal Memory Authorized Review-Flow Verification ===")
print(f"Model: {MODEL}")
print(f"Seeded A okf_id: {A_ID}  (current before approval)")
print(f"Seeded P okf_id: {P_ID}  (proposed, updates->A)")
print()
print("Per-phase tool call sequence (tool, key args):")
for label, calls in [("A", a_calls), ("B", b_calls), ("C", c_calls)]:
    print(f"  Phase {label}:")
    if not calls:
        print("    (no mcp_tool_call events observed)")
    for c in calls:
        a = c["args"]
        # redact: only print key arg names/values that are non-secret
        key = {k: v for k, v in a.items() if k in (
            "query", "memory_view", "refs", "memory_review_queue", "limit",
            "ref", "action", "expected_state")}
        print(f"    - {c['server']}.{c['tool']}  args={json.dumps(key, ensure_ascii=False)}")

print()
print("Mutation counts per phase:")
print(f"  Phase A mutating calls: {len(a_mut)} (expected 0)")
print(f"  Phase B mutating calls: {len(b_mut)} (expected 1 = the single approve)")
print(f"  Phase C mutating calls: {len(c_mut)} (expected 0)")

print()
print("Current-view / chain transition evidence:")
for k in ("A_current_titles", "A_queue_ids", "A_queue_titles",
          "B_review_args", "B_review_result",
          "C_current_titles", "C_history_items"):
    if k in summary:
        print(f"  {k}: {json.dumps(summary[k], ensure_ascii=False)}")

# Surface codex stderr (redacted, small) only when a phase produced no calls.
for name, calls in [("phase_a", a_calls), ("phase_b", b_calls), ("phase_c", c_calls)]:
    if not calls:
        errf = os.path.join(work_dir, name + ".err")
        if os.path.exists(errf):
            tail = open(errf, errors="replace").read()[-400:]
            print(f"  (codex stderr tail for {name}): {tail!r}")

print()
if errors:
    print(f"FAIL: {len(errors)} assertion(s) failed:")
    for err in errors:
        print(f"  - {err}")
    sys.exit(1)
else:
    print("PASS: temporal memory authorized review flow verified.")
    print("  Phase A: 0 mutations; A current, P pending in review queue.")
    print("  Phase B: exactly one authorized okf_memory_review approve (proposed->approved).")
    print("  Phase C: 0 mutations; P now current, A historical; ordered chain confirmed.")
PY

echo ""
echo "=== Harness complete ==="
