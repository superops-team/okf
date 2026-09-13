# Real Agent Evidence — add-agent-knowledge-discovery (fresh run)

- Source commit: `22d52dad795250812fd3c9015f6b03314fe02541`
- Source dirty: `true`
- Source tree SHA-256: `126470a7d5698f017df9cb284768a9e3703ec44d708e49504750c3439cdfc94c` (tracked + non-ignored untracked files; generated evidence excluded)
- Verification timestamp (UTC): 2026-09-13T05:36:06Z
- Entry point: `tools/verify-real-agent-e2e.sh`
- Direct MCP helpers are excluded from Agent-client evidence.

| Client | Layer | Status | Evidence |
|---|---|---|---|
| adapter | code_knowledge_init | PASS | okf init generated code_file concepts from the Go fixture |
| adapter | generated_config_parse | PASS | cursor/claude/codex configs parse with standard parsers |
| codex | official_config_discovery | PASS | official Codex CLI discovered project MCP server okf |
| claude-code | official_config_discovery | PASS | official Claude Code CLI discovered okf (approval state recorded) |
| cursor | official_config_discovery | PASS | official Cursor Agent CLI discovered okf (approval state recorded) |
| validator | negative_controls | PASS | fabricated answer/recovery/write/code-symbol evidence is rejected |
| codex | model_read_answer | PASS | real Agent called status→manifest→query→context; no shell; answer correct |
| codex | model_error_recovery | PASS | real Agent consumed structured error/remediation and retried successfully |
| codex | model_controlled_write | PASS | real Agent called note→query; persisted file and answer verified |
| codex | model_code_symbol | PASS | real Agent queried generated code_file concept; args, ownership, and line numbers verified |
| codex | model_propagate_parent_ids | PASS | real Agent retrieved propagateParentIDs snippet; behavior and line numbers verified |
| claude-code | model_e2e | BLOCKED_AUTH | official client is not logged in |
| cursor | model_e2e | BLOCKED_AUTH | official client login status could not be positively confirmed (fail-closed) |

## Counts

```json
{"BLOCKED_AUTH": 2, "PASS": 11}
```

Strict release acceptance requires `REQUIRE_ALL_AGENT_MODELS=1` and exit code 0.
