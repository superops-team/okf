package agentconfig

// codexAdapter projects the OKF project configuration into Codex's
// project-scoped files using delimited blocks only (no TOML parsing library):
//   - .codex/config.toml  (# BEGIN/END OKF MANAGED MCP v1 block)
//   - AGENTS.md           (<!-- BEGIN/END OKF MANAGED AGENT v1 --> block)
type codexAdapter struct{}

func (codexAdapter) name() string    { return "codex" }
func (codexAdapter) version() string { return adapterVersion }

func (codexAdapter) specs() []fileSpec {
	return []fileSpec{
		codexTOMLSpec(),
		blockSpec("AGENTS.md", agBegin, agEnd, []byte(RenderCodexBlock()), nil),
	}
}

// codexTOMLSpec renders the delimited TOML MCP block. It conflicts when an
// unowned [mcp_servers.okf...] table exists outside the managed region.
func codexTOMLSpec() fileSpec {
	return fileSpec{
		rel: ".codex/config.toml",
		inspectInstall: func(current []byte, cmd []string) ([]byte, bool, FileStatus, FileAction, error) {
			body := []byte(RenderCodexMCPTOML(cmd))
			prop, st, act, err := inspectBlock(current, tomlBegin, tomlEnd, string(body), tomlHasUnownedTable)
			return prop, false, st, act, err
		},
		inspectRemove: func(current []byte) ([]byte, bool, error) {
			if len(current) == 0 {
				return nil, false, nil
			}
			prop, ok := removeBlock(current, tomlBegin, tomlEnd)
			if !ok {
				return nil, false, errConflict("managed TOML markers are unbalanced; refusing to remove ambiguous region")
			}
			return prop, false, nil
		},
	}
}
