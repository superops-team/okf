---
type: documentation
title: OKF CLI Commands
description: Command-line interface reference for OKF, including init, update, lint, show, search, add, sync, watch, mcp, and tool commands.
tags: [okf, cli, command-line, usage, reference]
status: stable
---

# OKF CLI Commands

The `okf` CLI provides commands for managing knowledge bases.

## Usage

```bash
okf <command> [options]
```

## Commands

### init / generate
Initialize a knowledge base from a git repository.
- `-repo PATH` - Repository path (default: current directory)
- `-dir PATH` - Knowledge directory (default: .okf/knowledge)
- `-force` - Overwrite existing

### update
Update the knowledge base from the latest commit.
- `-repo PATH` - Repository path
- `-full` - Full regeneration
- `-verbose` - Show changed files

### lint
Check the knowledge base for specification compliance.
- `-path PATH` - Knowledge base path
- `-strict` - Strict mode (warnings fail)
- `-verbose` - Show all issues including info

### show / info
Show knowledge base information and statistics.
- `-path PATH` - Knowledge base path
- `-detail` - Show all concepts with details

### search
Search the knowledge base.
- `-path PATH` - Knowledge base path
- `-q QUERY` - Search query text
- `-type TYPE` - Filter by concept type
- `-tag TAG` - Filter by tag
- `-semantic` - Enable hybrid semantic search (requires `okf vector index` first)
- `-k N` - Number of results for semantic search (default 10)
- `-lexical-weight W` - Weight of the BM25 lexical channel in hybrid search (default 0.5; `0` disables it for pure semantic search)
- Code filters: `-code-language`, `-code-path`, `-code-symbol-kind`, `-code-qualified-name`, `-code-relation-kind`

### add
Import files, directories, or archives into the knowledge base with smart detection.
Documents (PDF, DOCX, XLSX, PPTX, HTML, CSV, TXT, DOC) are automatically
converted to Markdown (`<original>.md`) before import via the built-in pure-Go
converter (downmark). Conversion is deterministic and pinned to downmark
v0.10.0; a downmark upgrade may trigger a one-time re-import of previously
imported documents (documented re-import semantics — see Release Notes).
- `-strategy STRATEGY` - Merge strategy: skip|overwrite|merge|patch
- `-patch-fields LIST` - Comma-separated frontmatter fields for patch strategy
- `-detect-only` - Only detect changes without importing
- `-dry-run` - Preview changes without applying
- `-force` - Overwrite existing (shorthand for -strategy=overwrite)

### sync
Synchronize all indexed files, detecting changes and applying strategies.

### watch
Watch source directories and auto-sync on file changes (requires .watch.yaml).

### metadata
Manage the metadata index.
- Subcommands: `inspect`, `rebuild`, `clean`

### vector
Manage the semantic vector index. The index stores one vector per **chunk**
(concepts are split on `##`–`####` headings), keyed as `<concept fingerprint>#<ordinal>`.
- Subcommands: `index`, `status`, `rebuild`
- `index` - Build or incrementally update the index
- `index -force` - Force re-encode all chunks
- `status` - Show chunk count, concept count, dimensions, model and index format version
- `rebuild` - Remove the old index and rebuild from scratch (required when the
  index format version changes; loading an incompatible index fails with an
  explicit prompt rather than returning wrong results)
- `-path PATH` - Knowledge base path

### eval
Run the IR evaluation benchmark against a golden query set. Used to quantify
retrieval quality changes instead of relying on impressions.
- `-golden PATH` - Golden query set JSON (required)
- `-path PATH` - Knowledge base path
- `-k N` - Cut-off K for Recall@K / Precision@K / NDCG@K (default: the value declared in the golden set, else 5)
- `-compare` - Compare strategies side by side: `lexical-substring`, `bm25-only`, `semantic-only`, `hybrid-default`
- `-verbose` - Print per-case results

Warns when the golden set's expected documents are absent from the knowledge
base, since that yields all-zero metrics that are easy to misread as a
retrieval failure.

### config
Manage configuration.

### tool
Agent-facing JSON tool operations.
- Subcommands: `status`, `init`, `refresh`, `query`, `context`

### mcp
Start the MCP (Model Context Protocol) server for AI agent integration.
- `-repo PATH` - Repository root used by the agent-facing repository knowledge service
- `-dir PATH` - Knowledge directory; relative paths resolve under `repo`, absolute paths remain absolute
- `-bundle PATH` - Optional legacy bundle path to auto-load on startup
- Communicates over stdio using JSON-RPC 2.0
- Exposes service-backed repository tools plus durable note/event/feedback tools; see `mcp-server.md`

### hook
Install git hooks for automatic updates.
- `-type TYPE` - Hook type: pre-commit, post-commit, pre-push
- `-uninstall` - Remove the hook
- `-force` - Overwrite existing hook

### version / --version / -v
Show version information.

### help / --help / -h
Show the help message.

## Global Options

- `-repo PATH` - Repository path (default: current directory)
- `-dir PATH` - Knowledge directory (default: .okf/knowledge)
- `-verbose` - Show detailed output
- `-strict` - Strict lint mode (warnings fail)
