# External Contract References

These URLs are normative only for client file locations and syntax verified during design. Adapter contract fixtures in this repository remain the executable compatibility gate.

## Cursor

- MCP configuration and project path `.cursor/mcp.json`: https://cursor.com/docs/mcp
- Project rules in `.cursor/rules` and supported `.md`/`.mdc`: https://cursor.com/docs/context/rules

## Claude Code

- Project MCP configuration `.mcp.json`: https://docs.anthropic.com/en/docs/claude-code/mcp
- Project settings and instruction locations: https://docs.anthropic.com/en/docs/claude-code/settings?47cf6d6d_page=2&d5c91f40_page=2
- Project skills `.claude/skills/<skill-name>/SKILL.md`: https://docs.anthropic.com/en/docs/claude-code/skills

## Codex

- Project-scoped `.codex/config.toml` MCP tables for trusted projects: https://developers.openai.com/codex/mcp/
- `config.toml` reference: https://developers.openai.com/codex/config-reference/
- Repository `AGENTS.md` discovery and precedence: https://developers.openai.com/codex/guides/agents-md/

## Upstream inspiration

- OpenContext repository: https://github.com/0xranx/OpenContext

The implementation must not silently assume an undocumented client format. When a supported adapter contract fixture no longer matches a client version under test, `doctor/status` reports `unsupported` or `conflict`; the adapter is updated through a reviewed spec change.
