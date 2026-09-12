package agentconfig

// cursorAdapter projects the OKF project configuration into Cursor's
// project-scoped files:
//   - .cursor/mcp.json        (mcpServers.okf, semantic JSON merge)
//   - .cursor/rules/okf.md    (whole-file OKF-owned rule)
type cursorAdapter struct{}

func (cursorAdapter) name() string    { return "cursor" }
func (cursorAdapter) version() string { return adapterVersion }

func (cursorAdapter) specs() []fileSpec {
	return []fileSpec{
		jsonMCPSpec(".cursor/mcp.json"),
		wholeFileSpec(".cursor/rules/okf.md", []byte(RenderCursorRule())),
	}
}
