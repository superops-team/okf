# Release Notes — MCP Skills Extension

Change: `add-mcp-skills-extension` (S01–S38).
Target CLI version: **0.7.0** (`cmd/okf/main.go`).
Library meta version remains `0.4.1` (`pkg/okf/meta/version.go`).

## Summary

Adds dual-era MCP protocol support (legacy 2024-11-05 + modern 2026-07-28), a portable canonical Agent Skill, and an immutable Skill registry. The modern era exposes 11 service-backed Agent tools, the static Skill resource, and skills/list/get methods; legacy behavior is unchanged.

## New features

### Dual-era MCP protocol
- **Legacy 2024-11-05**: initialize-based, 20 tools, Prompts, Resources, ping — unchanged.
- **Modern 2026-07-28**: stateless, per-request `_meta` validation, `server/discover`, `resultType: complete` + serverInfo `_meta` on all success responses.
- One era per stdio process; mixed-era requests rejected.
- Modern newline-delimited JSON framing; Content-Length remains for legacy compatibility.

### Portable Agent Skill
- `skill://okf/SKILL.md` — canonical W01–W07 workflow, YAML frontmatter (name=okf), no client ownership markers.
- Immutable registry with SHA-256 digest, size validation, URI confinement, defensive copies.
- Available as Resource in both eras; as Skill via `skills/list`/`skills/get` in modern era with client capability gate.

### Modern tool catalog
- Exactly 11 service-backed tools: `okf_ask`, `okf_context`, `okf_feedback`, `okf_init`, `okf_log`, `okf_manifest`, `okf_note`, `okf_query`, `okf_refresh`, `okf_resolve`, `okf_status`.
- Legacy bundle-state tools (e.g., `okf_load_bundle`, `okf_search`) excluded from modern era.

### Error codes
- `-32022`: unsupported modern protocol version (data: supported/requested).
- `-32021`: missing required client capability (data: requiredCapability).
- `-32602`: malformed metadata/params, unknown URI, non-empty cursor.

## Compatibility

- Legacy clients (`test_mcp.py`) continue to work unchanged.
- `--bundle` auto-load occurs only after legacy initialize; modern era has zero bundle I/O for discovery/Skill operations.
- No breaking changes to existing CLI commands or knowledge storage.

## Validation

- `go test ./...` all green; `-race` clean; `-shuffle=on` clean.
- `test_ext_skills.py`: 8/8 modern E2E.
- `test_mcp.py`: 13/13 legacy regression.
- Real Codex 0.153.4 Resource compatibility: list/read `skill://okf/SKILL.md`, name=okf, W01/W07 present, no mutating tool.
- Targeted mutants: 6/6 killed (version, capability, digest, completeness, stateless filter, wrapper leakage).
- Benchmarks: 10,000 ops each; Skill registry read 0 B/op, 0 allocs/op.
- `tools/gauntlet.sh`: PASS (coverage 69%).

## Known limitations

- No production Host with native modern `skills/list/get` was available; direct modern protocol fixture is fully passing (S36 `blocked_client_support`).
- Native Host Skills interop requires client-side extension support; Resource compatibility works today.
