# Design: MCP Skills Extension

## 1. Design principles

1. **One workflow source**: `CanonicalClauses()` remains the semantic fact source. Renderers are projections, not independent workflow documents.
2. **One MCP server entry**: extend `okf mcp`; add no daemon or sidecar.
3. **Static first**: one immutable instructor Skill, one file, one digest. Avoid generic plugin discovery until a real second Skill requires it.
4. **Negotiated behavior**: response shapes and methods depend on explicit session negotiation, not on request method alone.
5. **Fail closed**: an invalid built-in Skill prevents server construction/startup; it is never partially advertised.
6. **Backward compatible by construction**: legacy structs/JSON remain unchanged unless a field is explicitly emitted only for the new revision.
7. **Server/Host boundary is explicit**: OKF serves bytes and consistent metadata; the Host owns trust, activation and execution policy.

## 2. Architecture

```text
pkg/agentconfig
  CanonicalClauses() ──> RenderAgentSkill() ──┐
  client wrappers ──> Cursor / Claude / Codex │
                                               v
pkg/mcp/skills.go
  parse frontmatter -> validate URI/manifest -> immutable SkillRegistry
       |                         |
       |                         +-> sha256 + size + limits
       v
pkg/mcp/server.go
  initialize -> negotiated SessionProfile
       |              |
       |              +-> capabilities.extensions
       +-> skills/list / skills/get
       +-> resources/list / resources/read compatibility
       +-> existing tools/prompts unchanged
```

No knowledge bundle, embedding model, vector index or filesystem scan is involved in Skill construction.

## 3. Session protocol profile

Introduce a closed `SessionProfile` chosen by an explicit table:

| Client requested revision | Server response revision | Skills extension | Cacheable list/read shape |
|---|---|---:|---:|
| empty, malformed or unsupported older/newer revision | legacy `2024-11-05` compatibility profile | no | no |
| exact supported Skills revision `2026-07-28` | `2026-07-28` | yes | yes |

Rationale: the server currently supports two known schemas. Date string comparison is forbidden because it implies unimplemented compatibility with future base revisions. A future revision must be added to the table with tests.

Before successful initialize, the session uses the legacy profile. Re-initialization replaces the profile atomically for subsequent messages. This stdio server processes messages serially, so no additional lock is required.

`skills/list` and `skills/get` are recognized only when the profile enables the extension; otherwise they return `-32601 Method not found`. This makes capability negotiation effective rather than decorative.

## 4. Portable Agent Skill rendering

`RenderAgentSkill()` outputs:

- Agent Skills YAML frontmatter with `name` and `description`;
- a short heading and explanation;
- exactly one rendering of each W01–W07 canonical clause;
- only registered OKF tool names.

It excludes:

- client ownership markers;
- project file installation/removal instructions;
- secrets, tokens or absolute paths;
- executable hooks, scripts and `allowed-tools`.

`RenderClaudeSkill()` continues to retain its existing ownership wrapper and calls the same canonical body renderer. Cursor and Codex projections continue unchanged.

## 5. Immutable Skill registry

The first implementation uses a concrete immutable `SkillRegistry`, not a plugin interface.

Construction algorithm:

1. render `SKILL.md` bytes once;
2. parse YAML frontmatter verbatim into JSON-compatible data;
3. create the single Resource at `skill://okf/SKILL.md`;
4. compute SHA-256 over raw bytes and byte length;
5. validate the complete entry/content set;
6. freeze internal byte copies for the server lifetime.

Validation rules:

- URI parses successfully and scheme is `skill`;
- URI explicitly ends in `/SKILL.md`;
- final skill-path segment equals frontmatter `name`;
- frontmatter has non-empty `name` and `description`;
- manifest contains 1–512 unique Resources;
- `SKILL.md` appears exactly once;
- every Resource is inside the same `skill://okf/` subtree;
- no `.`/`..`, encoded traversal, backslash path or query/fragment is accepted;
- every manifest URI has exactly one readable byte sequence and no unlisted content exists;
- digest format and value match raw bytes;
- size matches raw byte length;
- total advertised bytes do not exceed 16 MiB;
- parsed frontmatter from the served `SKILL.md` equals the advertised object field-by-field.

Construction returns an error. `NewServer` propagates invalid built-in registry construction rather than panicking, so startup fails with an actionable error and test injection remains possible.

All registry reads return defensive copies. `List` and `Get` also clone nested maps/slices so callers cannot mutate server state.

## 6. MCP methods and result shapes

### `skills/list`

- requires negotiated Skills profile;
- accepts an optional empty cursor;
- current single-page registry rejects any non-empty cursor with `-32602`;
- returns one atomic Skill entry, `resultType=complete`, no `nextCursor`, TTL 300000 ms, scope private.

### `skills/get`

- requires negotiated Skills profile;
- requires URI equal to the served `SKILL.md`;
- unknown, directory or supporting-file URI returns `-32602`;
- returns an entry identical in shape and meaning to `skills/list`.

### `resources/read`

- remains the only content-read path;
- returns exact `SKILL.md` text for `skill://okf/SKILL.md`;
- unknown Skill Resource returns `-32602` and is not delegated to bundle resolution;
- new profile adds CacheableResult fields; legacy profile preserves the legacy result shape.

### `resources/list`

A Resource for the Skill is appended after existing resources and populated from parsed frontmatter. For deterministic discovery, Resource order is:

1. pre-existing bundle and Concept Resources in their current order;
2. published Skill Resources sorted by URI.

This is additive compatibility. Existing URI strings are preserved in this change.

### Other list methods

For the Skills-compatible base revision, existing `tools/list`, `resources/list` and `prompts/list` include required CacheableResult fields. Legacy responses omit them. Tool call and prompt-get payloads remain unchanged.

## 7. Error contract

| Condition | JSON-RPC code | Stable message category |
|---|---:|---|
| Skills method without negotiated capability | -32601 | method not found |
| malformed params | -32602 | invalid params |
| non-empty/unknown list cursor | -32602 | unknown skills cursor |
| unknown Skill URI | -32602 | skill not served |
| unknown Skill Resource | -32602 | resource not served |
| invalid built-in registry at startup | process/startup error | invalid built-in skill |
| unexpected registry/runtime failure | -32603 | internal error |

Messages must not expose credentials, host home paths or Skill body content.

## 8. Security model

OKF Server guarantees:

- immutable static content for a process lifetime;
- complete manifest, exact bytes, SHA-256 and byte size consistency;
- Skill-root URI confinement;
- bounded resource count and bytes;
- no filesystem materialization, script execution or permission grant;
- no dynamic/nested Skill in this release.

The MCP Host remains responsible for:

- tagging the originating server with a host-assigned identity;
- treating Skill text as untrusted model input;
- explicit user approval before activation or execution;
- ignoring/approval-gating `allowed-tools` and hooks;
- binding cached approvals to `(server identity, skill URI, complete resources digest set)`;
- re-approval when the resource set changes;
- digest/size/frontmatter verification on read;
- origin-scoped reads and cross-server approval;
- isolated cache outside filesystem Skill discovery.

The release documentation must not say that OKF “installs” or “activates” a remote Skill, or that digest matching establishes trust.

## 9. Compatibility strategy

Three paths coexist:

1. **Native extension path**: extension-capable hosts use capability negotiation plus `skills/list/get` and `resources/read`.
2. **Ordinary Resource path**: resource-capable hosts discover/read `skill://okf/SKILL.md`; this is inspection/context, not protocol-defined activation.
3. **Project adapter path**: existing `okf agent` writes managed project configuration for Cursor/Claude Code/Codex.

No path silently replaces another. This allows incremental client adoption without breaking existing integrations.

## 10. Testing strategy

- pure registry tests for grammar, manifest, cloning, limits and mismatch errors;
- protocol tests for negotiation, route gating, exact legacy/new JSON shapes, cursors and errors;
- parity tests that W01–W07 and registered tools match all client/portable renderers;
- stdio E2E over newline and Content-Length framing;
- existing `test_mcp.py` regression;
- real Codex test for ordinary Resource discovery/read and no mutation;
- native extension client fixture that directly sends `skills/list/get` and verifies bytes/digest;
- race and shuffled test suite;
- persisted manual mutants for digest bypass, manifest incompleteness, route-gating bypass and client-wrapper leakage.

## 11. Performance budgets

Because one 1–2 KiB static Skill is built once:

- registry construction: <5 ms in benchmark environment;
- `skills/list`, `skills/get`, Resource read: p95 <1 ms in-process over 10,000 operations;
- allocations: bounded by defensive output copies and independent of bundle size;
- no filesystem, bundle, embedding or vector access during Skill operations.

Budgets are enforced by Go benchmarks with `ReportAllocs` and by injected spies for forbidden subsystems. Wall-clock CI gating uses a conservative 20 ms operation threshold to avoid noisy failures; benchmark results are recorded, not overfitted.

## 12. Rollout and rollback

- release as an additive minor version feature marked experimental;
- extension is always available when a supported new revision is negotiated; no redundant feature flag;
- rollback is code rollback: legacy clients remain usable throughout;
- Hosts can ignore the extension and continue using ordinary Tools/Resources or existing project adapters.
