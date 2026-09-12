package agentconfig

// claudeAdapter projects the OKF project configuration into Claude Code's
// project-scoped files:
//   - .mcp.json                        (mcpServers.okf, semantic JSON merge)
//   - .claude/skills/okf/SKILL.md      (whole-file OKF-owned skill, valid frontmatter)
type claudeAdapter struct{}

func (claudeAdapter) name() string    { return "claude-code" }
func (claudeAdapter) version() string { return adapterVersion }

func (claudeAdapter) specs() []fileSpec {
	return []fileSpec{
		jsonMCPSpec(".mcp.json"),
		wholeFileSpec(".claude/skills/okf/SKILL.md", []byte(RenderClaudeSkill())),
	}
}
