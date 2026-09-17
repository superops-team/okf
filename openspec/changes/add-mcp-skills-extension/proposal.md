# Proposal: MCP Skills Extension and Dual-Era MCP Compatibility

## Summary

OKF SHALL publish its canonical Agent workflow as one static, instructor-only Agent Skill over the experimental MCP Skills extension `io.modelcontextprotocol/skills`. Because the extension targets the modern MCP base revision `2026-07-28`, OKF SHALL implement the minimum complete dual-era base-protocol boundary instead of layering modern extension fields onto the legacy initialize/session model.

The release adds:

1. dual-era stdio dispatch: legacy `initialize` compatibility and modern per-request `_meta` semantics;
2. mandatory modern `server/discover`, supported-version advertisement and `UnsupportedProtocolVersionError`;
3. modern required `resultType`, cache metadata and result `_meta` on every affected response;
4. `skills/list` and `skills/get` for clients that declare the Skills extension;
5. `skill://okf/SKILL.md` through standard `resources/read`;
6. a portable Skill renderer that reuses the existing W01–W07 canonical workflow;
7. per-file SHA-256, byte length, complete static manifest and private cache metadata;
8. ordinary Resource discovery for current hosts that cannot call Skills methods.

This remains an additive transport binding and protocol-compatibility layer. It does not create another knowledge model, retrieval engine, workflow source, runtime or client installation mechanism.

## Motivation

OKF already exposes 20 MCP tools and project-level instructions for Cursor, Claude Code and Codex. Tools describe individual actions but do not deliver the canonical multi-step workflow for using them safely. Skills Over MCP provides that transport binding.

The original Spec review uncovered a protocol-era mismatch: `2026-07-28` removes the `initialize` handshake and protocol-level session, introduces `server/discover`, and requires version, client identity and capabilities in per-request `_meta`. Therefore a conformant Skills implementation cannot be achieved by adding an extension field to the existing legacy initialize response. The minimum correct design is a dual-era server that preserves current legacy behavior and implements the complete modern request envelope/result boundary needed by the existing OKF methods.

A local PoC established that the Skill registry and Resource compatibility paths are technically feasible, but its initialize-based modern negotiation is explicitly superseded by this reviewed Spec. PoC measurements are feasibility evidence only and are not completion evidence.

## Requirements

### MUST

- Preserve the existing legacy `2024-11-05` initialize-based path and its response shapes.
- Implement modern `2026-07-28` stateless request handling with required per-request `_meta`.
- Implement `server/discover` and advertise the supported modern protocol version, server capabilities and server identity; legacy support is exposed through the initialize fallback path, not as a modern `_meta` version.
- Return `UnsupportedProtocolVersionError` code `-32022` with requested and supported versions for an unsupported modern version.
- Require modern Skills requests to declare `io.modelcontextprotocol/skills` in client capabilities; otherwise return `MissingRequiredClientCapability` code `-32021`.
- Declare the base Resources capability whenever declaring the Skills extension.
- Implement `skills/list` and `skills/get`; do not declare `directoryRead`.
- Serve exactly one static Skill at `skill://okf/SKILL.md` whose frontmatter name is `okf`.
- Generate Skill content from existing W01–W07 clauses while keeping client ownership wrappers separate.
- Derive all advertised frontmatter from final served bytes.
- Return a complete manifest containing `SKILL.md` exactly once with lowercase SHA-256 and raw byte length.
- Enforce at most 512 Resources and 16 MiB total advertised bytes.
- Serve only immutable in-memory instructor content; no scripts, hooks, dynamic/nested Skills or filesystem materialization.
- Emit all required modern result fields, including `resultType: complete`; emit CacheableResult fields on modern cacheable operations.
- Include server identity in modern result `_meta` without using self-reported identity for authorization.
- Preserve all existing legacy Tools, Prompts, Resources and project Agent adapters; expose the reviewed 11-tool stateless subset in the modern era.
- Document that Resource reading is not Skill activation and digest consistency is not trust.
- Map every Scenario to an implementation symbol, automated test and real entry-point check.

### SHOULD

- Keep implementation inside `pkg/agentconfig` and `pkg/mcp`; no new runtime dependency or top-level command.
- Use one immutable concrete registry rather than a generic plugin framework.
- Use `cacheScope: private` for OKF responses because content depends on the configured project/server context.
- Publish the Skill URI in both modern discovery instructions and legacy initialize instructions.
- Accept existing Content-Length framing only as an explicitly tested OKF backward-compatibility extension; modern MCP stdio conformance uses newline-delimited JSON.

### MAY

- A later change may add supporting files, multiple Skills, directory reads, HTTP transport, dynamic content, attestations, RBAC or per-tenant catalogs.
- A later Host-focused change may implement content-bound approval and isolated caches; those remain Host responsibilities.

## Non-Goals

- No automatic activation or installation of remote Skill content.
- No execution of scripts, hooks or Skill-authored commands.
- No honoring of `allowed-tools` as a permission grant.
- No `resources/directory/read`, dynamic manifest or nested Skill.
- No archive or filesystem materialization.
- No Streamable HTTP implementation.
- No implementation of modern subscriptions, MRTR input-required flows, HTTP headers or deprecated features; OKF returns only ordinary complete results over stdio.
- No replacement of existing Agent adapters, Tools or Prompts.
- No claim that current Codex natively supports `skills/list/get`; its acceptance path remains ordinary Resource compatibility.
- No trust/authorship claim from SHA-256 or server self-reported identity.

## Dependency Order and Completion Boundary

`P0 dual-era executable base contract → P1 portable Skill renderer → P2 immutable registry → P3 Skills/Resource wiring → P4 adversarial and real-host verification → P5 documentation/conformance/evidence`.

P0–P5 are dependency order only. The change is complete only after every phase, Scenario, modern and legacy entry point, regression, real-host check, documentation update and final conformance audit is complete.

## Expected Impact

- New: `pkg/mcp/skills.go`, focused tests and persisted modern/Skills stdio E2E fixtures.
- Modified: `pkg/mcp/protocol.go`, `pkg/mcp/server.go`, `pkg/agentconfig/workflow.go` and tests.
- Existing `okf mcp` remains the sole entry point.
- No new Go module dependency; reuse existing YAML support.
- Skill enumeration/read performs no knowledge-file, embedding or vector-index access.
- Modern support necessarily adds `server/discover`, request `_meta` validation, result `_meta`, modern errors and `resultType` to all existing modern method responses.

## Compatibility Decision

- A legacy client opens with `initialize`; OKF answers using existing legacy semantics for that stdio process.
- A modern client opens with `server/discover` or any request carrying valid modern `_meta`; each modern request is validated independently and no initialize state is required.
- `server/discover` advertises modern and legacy versions, but only modern capabilities in its discovery result.
- `skills/list/get` are valid only for modern requests whose client capabilities declare the Skills extension.
- `skill://okf/SKILL.md` remains readable in both eras through `resources/read`, allowing non-Skills Resource clients to inspect it.
- `resources/list` appends the Skill Resource in both eras; previous Resources and order remain unchanged.
- Existing path-bearing `okf://bundle/...` and `okf://concept/...` URIs remain unchanged; stable URI migration is a separate compatibility change.
- Modern `ping` is not supported because it was removed from the modern revision; legacy `ping` remains unchanged.

## Principal Risks

- Mixing legacy process state into modern requests would violate stateless semantics; modern dispatch derives behavior only from each request envelope.
- Treating missing/invalid modern `_meta` as legacy fallback could process ambiguous requests incorrectly; only an actual legacy initialize opening selects legacy behavior.
- Implementing only list/read modern shapes would leave tool-call and prompt-get results nonconformant; all modern success results require `resultType`.
- Capability advertising without checking client extension support could bypass negotiation; Skills methods require both modern version and declared client extension.
- Adding Skill to legacy `resources/list` changes resource count; tests lock additive compatibility.
- Frontmatter/manifest duplication and unsafe URI membership remain fail-closed registry risks.
- Host misuse of Resource reads or unsigned digest claims cannot be prevented server-side; documentation must state the boundary precisely.
