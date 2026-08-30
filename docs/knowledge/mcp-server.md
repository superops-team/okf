---
type: documentation
title: OKF MCP Server
description: Model Context Protocol server for repository knowledge, durable note/event/feedback capture, legacy bundle access, resources, and prompts.
tags: [okf, mcp, model-context-protocol, agent-integration, tools]
status: stable
---

# OKF MCP Server

The OKF MCP Server exposes repository knowledge and durable knowledge capture through the Model Context Protocol.
Agent-facing repository operations delegate to one shared `pkg/tool.Service`; handlers only decode parameters, invoke the service, and encode the versioned envelope.

## Protocol and startup

- **Transport**: stdio, JSON-RPC 2.0 with byte-accurate `Content-Length` framing
- **Protocol version**: `2024-11-05`
- **Server name**: `okf-mcp-server`
- **Server version**: `0.1.0`

```bash
okf mcp --repo /path/to/repository --dir .okf/knowledge
```

`--repo` selects the repository root. A relative `--dir` resolves under the canonical repository root; an absolute `--dir` remains absolute.
`--bundle` remains available to preload a legacy bundle for the original bundle/list/get/search/lint/resource operations.

## Agent-facing repository tools

| Tool | Mutating | Purpose |
|---|---:|---|
| `okf_status` | No | Report readiness and freshness without creating files |
| `okf_init` | Yes | Generate repository knowledge at the configured knowledge directory |
| `okf_refresh` | Yes | Refresh using `incremental`, `full`, or `cache-only` mode |
| `okf_query` | No | Query repository knowledge through `pkg/tool.Service.Query` |
| `okf_context` | No | Build token-bounded context through `pkg/tool.Service.Context` |
| `okf_note` | Yes | Persist an explicit durable `note` concept |
| `okf_log` | Yes | Persist an explicit durable `event` concept |
| `okf_feedback` | Yes | Persist an explicit reusable feedback `principle`, `category`, and `evidence_refs` |
| `okf_ask` | No | Query only `note`, `event`, and `feedback` through the shared query service |

Read-only tools advertise read-only, idempotent, closed-world MCP annotations.
Mutating tools advertise mutating, non-destructive, idempotent, closed-world annotations.
These facts are explicit protocol metadata and are not inferred from tool names.

### Common response envelope

Agent-facing tools return JSON text with a stable envelope containing `schema_version`, `operation`, `ok`, `mutating`, canonical `repo_root`, and canonical `knowledge_dir`.
The same envelope also carries `freshness`, `warnings`, `result`, and structured `error` when applicable.
An uninitialized repository returns `knowledge_not_initialized`; `okf_status` does not create the knowledge directory.

### Query and context

`okf_query` and `okf_context` use the same service as the `okf tool` command. They do not run an independent contains-search implementation. `okf_context` guarantees `used_tokens <= budget_tokens` and reports omitted items structurally.

`okf_ask` fixes the type filter to `note`, `event`, and `feedback`. Optional `project` filtering is applied inside `Service.Query` before ranking; `limit` is honored and ordering is deterministic for identical inputs.

### Durable writes and idempotency

`okf_note` and `okf_log` accept:

- `content` (required string)
- `idempotency_key` (required string)
- `project` (optional string)
- `tags` (optional string array)
- `metadata` (optional JSON object)

`okf_feedback` replaces `content` with required `principle` and `category`, and additionally accepts optional `evidence_refs` as a string array.

A stable identity is derived from canonical repository root, concept kind, and `idempotency_key`.
Repeating the same key with the same normalized payload returns the original identity with `created=false`.
Reusing the key with different content returns `idempotency_conflict` and does not overwrite the existing concept.

Files are committed with a same-directory temporary file, file sync, rename, directory sync, and parse/hash verification.
In-process concurrent writes for the same target are serialized. Persisted concepts are queryable after a server restart.

## Validation and security boundaries

Write handlers fail closed before persistence:

- unknown fields or wrong field types return `invalid_request`;
- `content` is limited to 256 KiB;
- `metadata` is limited to 16 KiB and credential-like field names are rejected recursively;
- at most 64 tags are accepted, each at most 128 bytes;
- `idempotency_key` is limited to 256 bytes;
- empty content, missing idempotency keys, and unknown kinds are rejected;
- a symlink in the configured knowledge-root ancestry returns `path_outside_root` and no external path is written.

OKF persists only feedback explicitly provided by an MCP caller. It does not inspect or subscribe to a host application's private event bus.
Wiki files have no special state-machine behavior: a repository's existing `wiki/**/*.md` files are ordinary Markdown sources.
OKF does not automatically maintain `wiki/index.md` or `wiki/log.md`.

## Legacy MCP tools

The following existing tools remain available and preserve their contracts:

- `okf_load_bundle`
- `okf_bundle_stats`
- `okf_list_concepts`
- `okf_get_concept`
- `okf_search`
- `okf_lint_bundle`
- `okf_lint_concept`
- `okf_import_document`

## Resources and prompts

- `okf://bundle/{path}` — bundle metadata
- `okf://concept/{bundlePath}/{conceptPath}` — concept content
- `okf_explain_concept` — explain a concept
- `okf_summarize_bundle` — summarize a loaded bundle

## Client configuration

```json
{
  "mcpServers": {
    "okf": {
      "command": "okf",
      "args": ["mcp", "--repo", "/path/to/repository", "--dir", ".okf/knowledge"]
    }
  }
}
```

For durable note/event/feedback capture, isolated knowledge directories, idempotent retries, and restart verification, see `docs/knowledge/durable-capture.md`.

## Architecture

- `pkg/tool/service.go` — shared repository status/init/refresh/query/context semantics
- `pkg/tool/write.go` — validated, idempotent, atomic note/event/feedback persistence
- `pkg/mcp/protocol.go` — JSON-RPC and MCP types, including tool annotations
- `pkg/mcp/tools.go` — tool registry and thin MCP projections
- `pkg/mcp/server.go` — stdio loop and shared service construction
- `pkg/mcp/convert.go` — legacy bundle/parser type conversion
