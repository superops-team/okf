# MCP Skills Extension Specification

## Requirement: Explicit base-protocol and capability negotiation

The server SHALL select behavior from an explicit supported-revision table. It SHALL advertise `io.modelcontextprotocol/skills` and Resources together only for a revision whose response shapes and methods are implemented.

### Scenario S01: Skills revision is negotiated
- **GIVEN** an initialize request for `2026-07-28`
- **WHEN** initialization completes
- **THEN** the response protocol is `2026-07-28`
- **AND** capabilities include Resources and `extensions.io.modelcontextprotocol/skills`
- **AND** the extension settings object does not contain `directoryRead: true`
- **AND** instructions name `skill://okf/SKILL.md`

### Scenario S02: Legacy revision remains legacy
- **GIVEN** an initialize request for `2024-11-05`
- **WHEN** initialization completes
- **THEN** the response protocol is `2024-11-05`
- **AND** capabilities omit `extensions`
- **AND** existing Tools, Resources and Prompts capabilities remain present

### Scenario S03: Unsupported revisions do not imply future support
- **GIVEN** an empty, malformed, older or unknown newer protocol revision
- **WHEN** initialization completes
- **THEN** the server selects the documented legacy compatibility profile
- **AND** it does not advertise the Skills extension or new cacheable result shapes
- **AND** the decision is made by explicit revision membership, not lexical date comparison

### Scenario S04: Skills methods are capability-gated
- **GIVEN** a session that has not negotiated the Skills profile
- **WHEN** it calls `skills/list` or `skills/get`
- **THEN** the server returns `-32601`
- **AND** no Skill metadata is returned through that method

### Scenario S05: Re-initialize replaces the profile deterministically
- **GIVEN** a serial stdio session initialized first as legacy and then as the Skills revision
- **WHEN** list methods are called after each initialization
- **THEN** each response uses the most recently negotiated profile
- **AND** no fields from the previous profile leak into the next response

## Requirement: Portable canonical OKF Agent Skill

The server SHALL render one portable Agent Skill from the existing W01–W07 canonical workflow without copying client installation semantics.

### Scenario S06: Skill satisfies Agent Skills frontmatter minimum
- **WHEN** the portable Skill is rendered
- **THEN** it begins with closed YAML frontmatter
- **AND** frontmatter contains non-empty `name: okf` and `description`
- **AND** it has a Markdown body

### Scenario S07: Canonical workflow appears exactly once
- **WHEN** portable, Cursor, Claude Code and Codex workflow projections are rendered
- **THEN** W01 through W07 each appear exactly once in each applicable projection
- **AND** every referenced `okf_*` tool is present in the registered MCP tool inventory

### Scenario S08: Portable Skill excludes client ownership and privilege metadata
- **WHEN** the portable Skill is rendered
- **THEN** it contains no `OKF-MANAGED`, managed-file marker, client install/remove instruction, credential or absolute user path
- **AND** it contains no hook, script execution directive or `allowed-tools` field
- **AND** existing client-specific renderers retain their current ownership behavior

### Scenario S09: Advertised metadata is derived from served bytes
- **GIVEN** final portable `SKILL.md` bytes
- **WHEN** the Skill entry and Resource metadata are constructed
- **THEN** name, description and all other frontmatter fields equal the parsed frontmatter field-by-field
- **AND** no separate name or description constant can diverge from the bytes

## Requirement: Complete immutable Skill manifest

The server SHALL build an immutable static manifest for `skill://okf/SKILL.md` and fail startup if it cannot prove manifest/content consistency.

### Scenario S10: Static Skill entry is complete
- **WHEN** the registry is constructed
- **THEN** it contains exactly one Skill identified by `skill://okf/SKILL.md`
- **AND** its resources array contains exactly one entry for the same URI
- **AND** the digest is `sha256:` followed by 64 lowercase hexadecimal characters
- **AND** size equals the raw byte length

### Scenario S11: URI and name constraints are enforced
- **GIVEN** an entry URI with a wrong scheme, query, fragment, backslash, dot segment, encoded traversal, missing `/SKILL.md`, or final skill-path segment different from frontmatter name
- **WHEN** registry validation runs
- **THEN** construction fails before the server advertises the Skill

### Scenario S12: Manifest completeness and uniqueness are enforced
- **GIVEN** a manifest with a duplicate URI, missing content, unlisted content, missing SKILL.md or a Resource outside the Skill root
- **WHEN** registry validation runs
- **THEN** construction fails closed

### Scenario S13: Digest, size and frontmatter mismatches fail closed
- **GIVEN** a mismatching digest, mismatching size or advertised frontmatter different from the served SKILL.md
- **WHEN** registry validation runs
- **THEN** construction fails closed with a stable diagnostic category

### Scenario S14: Resource and byte limits are enforced
- **GIVEN** 513 Resource entries or total advertised bytes greater than 16,777,216
- **WHEN** registry validation runs
- **THEN** construction fails before serving any entry
- **AND** exactly 512 entries and exactly 16,777,216 bytes are accepted when all other rules pass

### Scenario S15: Registry callers cannot mutate server state
- **WHEN** a caller mutates maps, slices or byte buffers returned by List, Get, Read or Resources
- **THEN** a later call returns the original canonical data

### Scenario S16: Skill operations have no knowledge-runtime side effects
- **GIVEN** spies around bundle loading, filesystem scanning, embedding and vector-index initialization
- **WHEN** registry construction, skills/list, skills/get and Skill resources/read run
- **THEN** every forbidden subsystem has zero calls

## Requirement: Skills list and get protocol methods

A negotiated Skills session SHALL implement `skills/list` and `skills/get` with spec-conformant result and error semantics.

### Scenario S17: skills/list returns an atomic entry
- **GIVEN** a negotiated Skills session and empty params
- **WHEN** `skills/list` is called
- **THEN** it returns `resultType: complete`
- **AND** one complete Skill entry
- **AND** no `nextCursor`
- **AND** `ttlMs: 300000` and `cacheScope: private`

### Scenario S18: skills/list cursor behavior is explicit
- **WHEN** `skills/list` is called with an empty cursor
- **THEN** it returns the first and only page
- **WHEN** it is called with a non-empty cursor
- **THEN** it returns `-32602` with the unknown-cursor category

### Scenario S19: skills/get equals the listed entry
- **GIVEN** a negotiated Skills session
- **WHEN** `skills/get` is called with `skill://okf/SKILL.md`
- **THEN** its Skill object is deeply equal to the `skills/list` entry
- **AND** it returns `resultType: complete`, the same TTL and private scope
- **AND** it has no pagination cursor

### Scenario S20: skills/get rejects unknown and non-entry URIs
- **WHEN** `skills/get` receives an unknown URI, Skill directory URI or supporting-file URI
- **THEN** it returns `-32602`
- **AND** the message identifies that the Skill is not served without exposing local paths

### Scenario S21: malformed parameters are rejected
- **WHEN** list/get params are malformed JSON or get omits/non-stringifies `uri`
- **THEN** the server returns `-32602`
- **AND** it remains available for the next valid request

## Requirement: Skill content is served through Resources

Skill files SHALL use the existing Resources primitive and SHALL remain ordinary Resource content for clients that do not activate the extension.

### Scenario S22: Skill Resource is discoverable additively
- **WHEN** `resources/list` is called in either profile
- **THEN** it includes one `skill://okf/SKILL.md` Resource with MIME `text/markdown`
- **AND** its name and description are derived from Skill frontmatter
- **AND** every previously available Resource is still present in its previous relative order
- **AND** the Skill Resource is appended after prior Resources

### Scenario S23: Skill Resource read matches the manifest
- **WHEN** `resources/read` reads `skill://okf/SKILL.md`
- **THEN** returned text hashes and sizes exactly to the corresponding manifest entry
- **AND** URI and MIME type match the listed Resource

### Scenario S24: Unknown Skill Resource fails without filesystem fallback
- **WHEN** `resources/read` receives another URI below `skill://okf/` or a traversal-shaped URI
- **THEN** it returns `-32602`
- **AND** it performs no bundle lookup or filesystem read

### Scenario S25: Resource read does not imply activation
- **GIVEN** a Resource-only client
- **WHEN** it lists and reads `skill://okf/SKILL.md`
- **THEN** OKF returns ordinary Resource content only
- **AND** OKF does not persist approval, install files, grant permissions or execute content

## Requirement: New and legacy response shapes remain compatible

The server SHALL emit CacheableResult fields only for the negotiated Skills-compatible base revision and SHALL not alter unrelated response contracts.

### Scenario S26: New list and read shapes contain cache fields
- **GIVEN** a Skills-profile session
- **WHEN** tools/list, resources/list, resources/read or prompts/list succeeds
- **THEN** the result includes `resultType: complete`, `ttlMs: 300000` and `cacheScope: private`

### Scenario S27: Legacy shapes remain exact
- **GIVEN** a legacy-profile session
- **WHEN** the same operations run
- **THEN** their JSON omits `resultType`, `ttlMs` and `cacheScope`
- **AND** every pre-existing field and tool definition is byte-for-byte equivalent after deterministic JSON normalization

### Scenario S28: Existing MCP behavior is preserved
- **WHEN** the existing MCP E2E suite runs
- **THEN** all 20 tools remain listed and callable as before
- **AND** Prompts remain present
- **AND** bundle and Concept Resources remain readable
- **AND** durable data survives restart as before
- **AND** only the documented additive Skill Resource changes the legacy Resource count

### Scenario S29: Both stdio framing modes work
- **WHEN** a real server process receives the extension flow through newline JSON and Content-Length framing independently
- **THEN** initialize, skills/list, skills/get, Skill resources/read and tools/list all succeed
- **AND** the two modes return semantically identical results

## Requirement: Security and trust boundaries are explicit

The implementation and documentation SHALL distinguish server-side integrity consistency from Host-side trust, approval and execution policy.

### Scenario S30: Digests are not described as trust
- **WHEN** user and developer documentation is inspected
- **THEN** it states that digest and size prove advertised/served byte consistency only
- **AND** it does not claim authorship, safety, approval or trust

### Scenario S31: Host responsibilities are documented
- **WHEN** security documentation is inspected
- **THEN** it assigns origin labeling, untrusted-input treatment, content-bound approval, re-approval, permission gating, origin-scoped reads and isolated caching to the Host
- **AND** it states that OKF Server does not perform those Host actions

### Scenario S32: No optional high-risk capability is advertised
- **WHEN** initialize and skills/list outputs are inspected
- **THEN** `directoryRead` is absent/false
- **AND** the manifest is not dynamic
- **AND** no nested Skill, executable file, archive or script is present

### Scenario S33: Error and log output do not expose secrets
- **GIVEN** malformed params, unknown URIs and registry validation failures
- **WHEN** responses and server logs are captured
- **THEN** no token, credential, environment value, user home path or Skill body is emitted

## Requirement: Real-client and conformance validation

The delivered change SHALL prove native protocol behavior and current-client compatibility without overstating client capabilities.

### Scenario S34: Native protocol fixture completes the full flow
- **WHEN** the persisted ext-skills E2E client launches `okf mcp`
- **THEN** it completes initialize, skills/list, skills/get, resources/read and tools/list
- **AND** verifies extension capability, entry equality, digest, size, W01–W07 and 20 tools
- **AND** passes under both stdio framing modes

### Scenario S35: Real Codex Resource compatibility works
- **GIVEN** OKF is configured as a project MCP server in an isolated Codex home
- **WHEN** a real Codex process is asked to discover and read the OKF Skill without mutating knowledge
- **THEN** it calls its MCP Resource listing and read path
- **AND** reports readable=yes, name=okf, first=W01, last=W07 and no mutating OKF tool call
- **AND** evidence labels this as Resource compatibility, not native `skills/list/get` support

### Scenario S36: Native Host status is reported honestly
- **WHEN** no available production Host exposes native `skills/list/get`
- **THEN** conformance marks native production-host interoperability `blocked_client_support`
- **AND** the persisted direct-protocol fixture remains fully passing
- **AND** the change does not rename Resource compatibility as native support

### Scenario S37: Performance is independent of bundle size
- **GIVEN** an empty bundle and a 1,000-file bundle
- **WHEN** Skill registry/list/get/read benchmarks run
- **THEN** operation counts and bytes read from knowledge storage remain zero
- **AND** in-process p95 for each Skill operation is below 1 ms in the recorded benchmark environment
- **AND** CI uses a conservative 20 ms ceiling to avoid flaky timing failures

### Scenario S38: Final quality and Spec alignment gates pass
- **WHEN** implementation is ready for merge
- **THEN** `tools/gauntlet.sh`, full build/vet/test/race/shuffle, existing MCP E2E and ext-skills E2E all pass in one fresh run
- **AND** targeted mutants for digest, completeness, route gating and wrapper leakage are all killed
- **AND** `conformance.md` maps S01–S38 to implementation, tests and fresh evidence with no unexplained partial or gap
