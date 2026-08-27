---
type: documentation
title: OKF MCP Server
description: Model Context Protocol (MCP) server for OKF, enabling AI agents to load, query, search, and lint knowledge bundles through standard MCP tools.
tags: [okf, mcp, model-context-protocol, agent-integration, tools]
status: stable
---

# OKF MCP Server

The OKF MCP Server exposes knowledge bundle operations through the Model Context Protocol (MCP), allowing any MCP-compatible AI agent to interact with OKF knowledge bases.

## Protocol

- **Transport**: stdio (JSON-RPC 2.0 over standard input/output)
- **Protocol Version**: 2024-11-05
- **Server Name**: okf-mcp-server
- **Server Version**: 0.1.0

## Tools

### okf_load_bundle
Load an OKF knowledge bundle from a directory path.
- **Parameters**: `path` (string, required)
- **Returns**: Bundle summary with concept count and statistics

### okf_bundle_stats
Get statistics about the loaded knowledge bundle.
- **Parameters**: none
- **Returns**: Total concepts, type counts, status counts, trust tiers, stale count, attested computations, total sources

### okf_list_concepts
List concepts in the bundle with optional filtering.
- **Parameters**: `type` (string, optional), `limit` (integer, default 50)
- **Returns**: Numbered list of concepts with type, path, title, description

### okf_get_concept
Get full details of a concept by its file path.
- **Parameters**: `path` (string, required)
- **Returns**: Full concept serialized as Markdown with YAML frontmatter

### okf_search
Search concepts by text query, type, or tag.
- **Parameters**: `query` (string), `type` (string), `tag` (string), `limit` (integer, default 20)
- **Returns**: Matching concepts with relevance ordering

### okf_lint_bundle
Run lint checks on the entire knowledge bundle.
- **Parameters**: `strict` (boolean, optional)
- **Returns**: Lint results with errors, warnings, infos, and issue details

### okf_lint_concept
Run lint checks on a single concept.
- **Parameters**: `path` (string, required)
- **Returns**: Lint results for the specified concept

## Resources

- `okf://bundle/{path}` - Knowledge bundle metadata
- `okf://concept/{bundlePath}/{conceptPath}` - Individual concept content

## Prompts

- `okf_explain_concept` - Explain a concept from the knowledge bundle
- `okf_summarize_bundle` - Summarize the entire knowledge bundle

## Usage

### CLI
```bash
okf mcp --bundle /path/to/knowledge
```

### MCP Client Configuration (Claude Desktop)
```json
{
  "mcpServers": {
    "okf": {
      "command": "okf",
      "args": ["mcp", "--bundle", "/path/to/knowledge"]
    }
  }
}
```

## Architecture

- `pkg/mcp/protocol.go` - JSON-RPC 2.0 message types and MCP protocol types
- `pkg/mcp/tools.go` - Tool registry and 7 core tool implementations
- `pkg/mcp/server.go` - stdio server main loop and message dispatching
- `pkg/mcp/convert.go` - Type conversion between okf and parser packages
