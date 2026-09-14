# Proposal: MCP Skills Extension

## Summary

OKF SHALL publish its canonical Agent workflow as one static, instructor-only Agent Skill over the experimental MCP Skills extension `io.modelcontextprotocol/skills`, while preserving all existing MCP Tools, Resources, Prompts and project-scoped Cursor/Claude Code/Codex adapters.

The first release adds:

1. base-protocol negotiation for the legacy revision and the Skills-compatible revision;
2. `skills/list` and `skills/get`;
3. `skill://okf/SKILL.md` through the existing `resources/read` primitive;
4. a portable Skill renderer that reuses the existing W01–W07 canonical workflow;
5. per-file SHA-256, byte length, complete static manifest and private cache metadata;
6. compatibility discovery through ordinary `resources/list` for hosts that do not yet implement the extension.

The change is an additive transport binding. It does not create another knowledge model, retrieval engine, workflow source or client installation mechanism.

## Motivation

OKF already exposes 20 MCP tools and generates project-level instructions for Cursor, Claude Code and Codex. A connected host can discover what each tool does but may not discover the multi-step workflow for using those tools safely and efficiently. The extension allows the MCP server to ship the canonical workflow together with the tools and lets compatible hosts discover and verify it without a separate installation step.

A local proof of concept established technical feasibility before this proposal:

- `skills/list`, `skills/get` and `resources/read` returned one consistent static Skill;
- newline JSON and Content-Length stdio framing both passed;
- all existing 20 tools and the legacy MCP E2E suite remained available;
- a real Codex process discovered and read `skill://okf/SKILL.md` through the ordinary Resource compatibility path and correctly identified W01–W07 without invoking a mutating OKF tool;
- full build, vet, tests, race and shuffled changed-package tests passed.

The proof of concept is evidence for feasibility only. It is not the implementation contract and will not be merged without completing this specification.

## Requirements

### MUST

- Declare `io.modelcontextprotocol/skills` only after negotiating a base protocol revision that supports the extension.
- Declare the base `resources` capability whenever declaring the Skills extension.
- Implement `skills/list` and `skills/get`; do not declare `directoryRead` in this change.
- Serve exactly one static Skill at `skill://okf/SKILL.md` whose frontmatter name is `okf`.
- Generate the Skill body from the existing canonical W01–W07 workflow and preserve the existing client-specific ownership wrappers separately.
- Parse the advertised frontmatter from the final `SKILL.md` bytes; do not maintain a second metadata constant.
- Return a complete manifest containing `SKILL.md` exactly once, with lowercase `sha256:<64 hex>` and raw byte length.
- Enforce a maximum of 512 resources and 16 MiB total advertised bytes before serving the registry.
- Serve only immutable, in-memory, instructor-only content; no scripts, executable hooks, dynamic Skills, nested Skills or filesystem traversal.
- Return `resultType: complete`, `ttlMs: 300000` and `cacheScope: private` on Skills results and on the compatible list/read result shapes of the negotiated new base protocol.
- Preserve the exact legacy JSON shape for legacy clients except for the separately specified additive Skill Resource visibility decision.
- Preserve all existing Tools, Tool calls, Prompts and project-scoped Agent configuration behavior.
- Document that digests prove entry/content consistency, not trust, and that loading, approval, permission gating, origin binding and re-approval are Host responsibilities.
- Map every Scenario to an implementation symbol, automated test and real entry-point check before completion.

### SHOULD

- Keep the implementation inside `pkg/agentconfig` and `pkg/mcp`, with no new runtime dependency and no new top-level CLI command.
- Use a small immutable registry rather than a generic plugin framework.
- Keep `cacheScope` private because the served workflow is associated with the connected OKF project and server context.
- Publish the Skill URI in server instructions as a compatibility hint.
- Retain ordinary Resource discovery for currently deployed hosts that can list/read resources but cannot invoke `skills/list/get`.

### MAY

- A later change may add multiple Skills, supporting files, directory reads, HTTP transport, dynamic content, signatures/attestations, RBAC or per-tenant catalogs.
- A later Host-focused change may implement content-bound approval, server-origin namespaces and isolated caches. These are not Server responsibilities in this change.

## Non-Goals

- No automatic activation or local installation of remote Skill content.
- No execution of scripts, hooks or commands from a served Skill.
- No honoring of `allowed-tools` as a permission grant.
- No `resources/directory/read` capability or route.
- No dynamic manifest and no `resources: "dynamic"`.
- No nested Skill publication.
- No archive, bundle download or filesystem materialization.
- No HTTP/SSE transport implementation.
- No replacement of existing Agent adapters or existing MCP Tools/Prompts.
- No claim that current Codex has native SEP-2640 `skills/list/get` support; its acceptance path is ordinary Resource compatibility.
- No cryptographic authorship or trust claim from SHA-256.

## Dependency Order and Completion Boundary

`P0 executable protocol contract → P1 portable Skill renderer → P2 immutable registry → P3 MCP wiring and compatibility → P4 adversarial/real-host verification → P5 documentation, conformance and evidence`.

P0–P5 indicate dependency order only. The change is complete only after every phase, Scenario, public entry point, legacy regression, real host check, documentation update and final `conformance.md` is complete.

## Expected Impact

- New: `pkg/mcp/skills.go`, focused tests and a persisted ext-skills stdio E2E script.
- Modified: `pkg/mcp/protocol.go`, `pkg/mcp/server.go`, `pkg/agentconfig/workflow.go` and tests.
- Existing `okf mcp` command remains the sole server entry point.
- No new Go module dependency: YAML parsing reuses the repository's existing `gopkg.in/yaml.v3` dependency.
- Skill content is static and small; no knowledge files or vector index are read to enumerate or fetch it.

## Compatibility Decision

- Legacy initialization continues to return the legacy protocol revision and does not declare `extensions`.
- New-protocol initialization declares the Skills extension with an empty settings object, meaning `directoryRead=false`.
- `skills/list` and `skills/get` return Method Not Found before extension-capable initialization; they are available only for a negotiated Skills session.
- `skill://okf/SKILL.md` is readable through `resources/read` in both new and legacy sessions so resource-capable non-Skills hosts can inspect it.
- `resources/list` includes the Skill Resource additively for both session types. This changes the resource count but does not change or remove any previous resource. Exact legacy list order and prior resources remain stable after appending the Skill Resource.
- Existing path-bearing `okf://bundle/...` and `okf://concept/...` URIs are not rewritten in this change; stable logical URI migration requires an independent compatibility specification because deployed clients may persist those URIs.

## Principal Risks

- Protocol version strings compared lexically or by exact equality could negotiate incorrectly; use an explicit supported-revision table.
- Returning Skills methods before capability negotiation would violate client/server expectations; gate the routes by negotiated session state.
- Frontmatter or manifest duplication could drift; derive both from final bytes and validate at registry construction.
- Manifest entries could point outside the Skill root or omit content; enforce parsed URI subtree membership and exact manifest/content set equality.
- A Host may treat Resource read as activation; documentation and test evidence must explicitly state that Resource compatibility only proves readability.
- Digest/size may be treated as a trust boundary; docs must distinguish integrity consistency from authenticity.
- Adding Skill to legacy `resources/list` changes resource count; contract tests must assert additive behavior and no legacy field-shape changes.
- An unbounded future generic registry could increase attack surface; this release deliberately supports one immutable Skill.
