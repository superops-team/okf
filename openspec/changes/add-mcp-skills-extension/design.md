# Design: MCP Skills Extension and Dual-Era MCP Compatibility

## 1. Design principles

1. **Protocol facts before convenience**: modern `2026-07-28` behavior follows the modern stateless base protocol; it is not modeled as a newer legacy session.
2. **One workflow source**: `CanonicalClauses()` remains the W01–W07 fact source.
3. **One server entry**: extend `okf mcp`; no daemon or sidecar.
4. **Static first**: one immutable instructor Skill and one file.
5. **Per-request modern validation**: modern version/capabilities/identity come from each request `_meta`.
6. **Fail closed**: invalid registry, version, capability or URI never yields partial Skill data.
7. **Backward compatible by explicit era**: legacy clients keep the current wire path; modern responses use modern types.
8. **Server/Host boundary is explicit**: OKF provides bytes and consistent metadata; the Host owns activation, trust and execution policy.

## 2. Architecture

```text
stdio JSON-RPC line
        |
        v
  classify opening/request era
   |                     |
legacy initialize     modern _meta
   |                     |
legacy handlers      validate version/capabilities
                         |
                   server/discover + modern handlers
                         |
         +---------------+----------------+
         |               |                |
   existing tools   resources/prompts   skills/list|get
                                          |
pkg/agentconfig CanonicalClauses -> RenderAgentSkill
                                          |
                           immutable SkillRegistry
```

Content-Length input/output remains an OKF backward-compatibility framing extension. New modern conformance fixtures use newline-delimited JSON, the normative modern stdio framing.

## 3. Dual-era dispatch

### 3.1 Legacy era

An `initialize` opening selects legacy semantics for the process. Only after this selection may the server auto-load `ServerConfig.BundlePath` into the legacy in-memory ToolRegistry. This preserves:

- protocol `2024-11-05`;
- existing capabilities and instructions;
- legacy response/result shapes;
- legacy `notifications/initialized`, `notifications/cancelled` and `ping` behavior.

Modern methods are not exposed under legacy semantics. An unsupported requested legacy version is handled according to the existing legacy compatibility contract and is covered by regression tests; this change does not claim support for every pre-2026 revision.

### 3.2 Modern era

Modern requests carry a `_meta` object inside `params`:

- `io.modelcontextprotocol/protocolVersion`: required exact string `2026-07-28`;
- `io.modelcontextprotocol/clientCapabilities`: required JSON object;
- `io.modelcontextprotocol/clientInfo`: SHOULD be present; omission is accepted but produces an empty display identity.

There is no modern initialize state. Each request is validated independently. The server SHALL NOT remember client capabilities from one modern request and apply them to another. Server construction and modern opening SHALL NOT auto-load `BundlePath`; modern Agent tools resolve configured repository knowledge within each Service operation.

Unsupported versions return error `-32022` with:

```json
{"supported":["2026-07-28"],"requested":"..."}
```

Missing required modern metadata returns `-32602`. Missing required extension capability for a Skills method returns `-32021` with the required extension identifier in error data.

### 3.3 Opening ambiguity and coexistence

- `server/discover` with valid modern `_meta` identifies a modern request.
- Any other request with valid modern `_meta` is processed directly as modern; discovery is optional.
- An actual `initialize` request selects legacy behavior.
- A request that is neither a valid modern envelope nor an initialize request is rejected; it is not guessed into an era.
- Once a stdio process has successfully entered legacy mode, modern requests are rejected to avoid sharing legacy mutable session assumptions in one process. Modern mode is stateless but the process-level era choice keeps current OKF ToolRegistry bundle state semantics deterministic. Mixed-era parallel service is deferred until tool state is made explicitly request-scoped.

This is a deliberate, documented subset of the dual-era permission to serve both eras concurrently: OKF serves either era per stdio process, not both interleaved in one process.

## 4. Modern `server/discover`

The result contains:

- `resultType: complete`;
- `supportedVersions`: `["2026-07-28"]`; legacy `2024-11-05` is intentionally absent because it is not valid per-request `_meta` and is reached through initialize fallback;
- modern capabilities: 11 service-backed Agent Tools, the static Skill Resource and `extensions.io.modelcontextprotocol/skills: {}`; Prompts are intentionally omitted;
- instructions including `skill://okf/SKILL.md`;
- `_meta.io.modelcontextprotocol/serverInfo` with existing server name/version;
- `ttlMs: 300000`, `cacheScope: private`.

`serverInfo` is display/debug metadata only and is not used for authorization or origin identity decisions.

## 5. Portable Agent Skill

`RenderAgentSkill()` produces minimum Agent Skills frontmatter, a short heading and exactly one rendering of W01–W07. It references only registered tools and excludes client ownership markers, installation text, secrets, hooks, scripts and `allowed-tools`.

Existing Claude/Cursor/Codex projections retain their wrappers and reuse the canonical body renderer.

## 6. Immutable Skill registry

Construction:

1. render bytes once;
2. parse all YAML frontmatter fields into JSON-compatible values;
3. create `skill://okf/SKILL.md`;
4. compute lowercase SHA-256 and raw byte length;
5. validate entry/content equality and limits;
6. store immutable bytes and return defensive copies.

URI validation SHALL use parsed and canonicalized components, not a string prefix:

- exact `skill` scheme and lowercase canonical scheme;
- authority/path combine to the skill path whose final directory segment is `okf`;
- required terminal path `SKILL.md`;
- no userinfo, port, query, fragment, empty/`.`/`..` segment, backslash, percent-encoded slash/backslash/dot traversal or non-canonical equivalent;
- every supporting URI, if support is later added, remains below the canonical Skill root.

Manifest constraints:

- 1–512 unique entries;
- SKILL.md exactly once;
- manifest URI set equals readable-content URI set;
- digest string and value match;
- size matches;
- sum size ≤16,777,216 bytes without integer overflow;
- parsed served frontmatter deep-equals advertised frontmatter.

Construction returns an error; server startup propagates it without panic or partial service.

## 7. Modern core surface and result envelope

The modern Tools catalog contains exactly these 11 service-backed operations, sorted by name: `okf_ask`, `okf_context`, `okf_feedback`, `okf_init`, `okf_log`, `okf_manifest`, `okf_note`, `okf_query`, `okf_refresh`, `okf_resolve`, `okf_status`. They resolve the repository from immutable server startup configuration and explicit arguments. The nine legacy tools that depend on mutable `SetBundle`/`GetBundle` state—including `okf_load_bundle`—are omitted from modern discovery and rejected if called.

Modern Resources contain the static OKF Skill only. Modern Prompts are not advertised because the current prompts direct callers to legacy bundle-state tools. Modern `ping` is not implemented.

All successful modern results include `resultType: complete`, not only list operations. Existing method-specific fields remain.

Every modern result includes:

```json
"_meta": {
  "io.modelcontextprotocol/serverInfo": {
    "name": "okf-mcp-server",
    "version": "<single version source>"
  }
}
```

CacheableResult additionally applies to:

- `server/discover`;
- `tools/list`;
- `prompts/list`;
- `resources/list`;
- `resources/read`;
- `skills/list`;
- `skills/get`.

These receive `ttlMs: 300000` and `cacheScope: private`. Non-cacheable results such as `tools/call` and `prompts/get` receive `resultType` and result `_meta`, but no TTL/scope.

Errors do not contain success result fields.

## 8. Skills methods

`skills/list` and `skills/get` require:

1. modern version metadata;
2. client capabilities declaring `extensions.io.modelcontextprotocol/skills`.

`skills/list` accepts an omitted or empty cursor and rejects non-empty cursors with `-32602`. `skills/get` accepts only the exact entry URI and returns `-32602` for unknown/directory/supporting-file URIs. List and Get Skill objects deep-equal.

The server declares `directoryRead=false` by using an empty extension settings object and does not route `resources/directory/read`.

## 9. Resources compatibility

`skill://okf/SKILL.md` is appended to `resources/list` in both eras and can be read in both eras. Name/description come from parsed frontmatter.

Skill namespace dispatch happens before bundle URI resolution. Any unknown `skill:` URI returns `-32602` without bundle or filesystem fallback.

In the modern era, Resource results carry modern result fields. In the legacy era, prior result shape is retained. Existing path-bearing `okf://` URIs are unchanged.

## 10. Security boundary

OKF guarantees immutable process-lifetime Skill bytes, exact manifest consistency, URI confinement, count/size bounds, no executable/dynamic/nested content and no materialization.

The Host remains responsible for origin labeling using a host-assigned server identity, treating content as untrusted, approval before activation/execution, permission gating, content-bound approval/reapproval, verification on read, cross-server read controls and isolated caching.

## 11. Error contract

| Condition | Code | Required data/category |
|---|---:|---|
| unsupported modern version | -32022 | `supported`, `requested` |
| Skills capability missing | -32021 | required extension identifier |
| malformed/missing modern metadata | -32602 | invalid request metadata |
| non-empty list cursor | -32602 | unknown skills cursor |
| unknown Skill/Resource URI | -32602 | not served, no local path |
| method not implemented in selected era | -32601 | method not found |
| invalid built-in registry | startup error | invalid built-in skill |
| unexpected runtime failure | -32603 | internal error |

## 12. Testing strategy

- dual-era wire goldens for discovery, modern `_meta`, modern result envelopes and unchanged legacy shapes;
- direct modern calls without discovery;
- unsupported version and missing capability errors;
- actual initialize legacy path and modern rejection of mixed-era interleaving;
- registry validation, defensive copy and no-side-effect tests;
- newline-delimited modern E2E; separate Content-Length compatibility E2E;
- existing legacy MCP E2E;
- real Codex Resource compatibility;
- race/shuffle and persisted mutants.

## 13. Performance and maintainability

Deterministic gates:

- zero knowledge filesystem, embedding and vector calls for Skill/discovery operations;
- registry built once per server process;
- benchmark reports ns/op, B/op and allocs/op;
- list/get/read latency is recorded, with a conservative CI ceiling only after measuring the target CI baseline.

The prior fixed “p95 <1 ms” requirement is removed because Go benchmark output does not provide p95 without a custom harness and a universal wall-clock threshold would be environment-dependent. The actionable budget is O(1) in bundle size with bounded copies and zero knowledge-runtime I/O.

## 14. Rollout and rollback

Release as an experimental additive minor-version feature. Modern support is advertised only through discovery/modern metadata. Legacy clients remain usable. Rollback removes the modern/Skills paths without changing stored knowledge or project Agent configuration.
