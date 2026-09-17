# MCP Skills Extension Specification

## Requirement: Dual-era MCP protocol boundary

OKF SHALL support the legacy `2024-11-05` initialize-based era and the modern `2026-07-28` stateless era from the same `okf mcp` entry point. Each stdio process SHALL select exactly one era from its opening exchange and SHALL not mix era semantics afterward.

### Scenario S01: Modern discovery advertises supported behavior
- **GIVEN** a `server/discover` request with valid modern `_meta`
- **WHEN** discovery completes
- **THEN** it returns `resultType: complete`
- **AND** `supportedVersions` contains exactly `2026-07-28`; legacy `2024-11-05` remains available only through initialize fallback
- **AND** capabilities contain modern Tools, Resources and `extensions.io.modelcontextprotocol/skills`
- **AND** capabilities omit Prompts and do not set `directoryRead: true`
- **AND** instructions name `skill://okf/SKILL.md`
- **AND** result `_meta` contains serverInfo whose version comes from the repository's single MCP/library version source rather than a handler literal

### Scenario S02: Legacy initialize remains compatible
- **GIVEN** the process opens with an initialize request for `2024-11-05`
- **WHEN** initialization completes
- **THEN** the response and subsequent behavior use the existing legacy era
- **AND** capabilities omit `extensions`
- **AND** all 20 legacy Tools, existing Resources, Prompts and legacy ping remain available

### Scenario S03: Modern version and metadata are validated per request
- **GIVEN** a modern request whose `_meta.io.modelcontextprotocol/protocolVersion` is unsupported
- **WHEN** the request is validated
- **THEN** it returns `UnsupportedProtocolVersionError` code `-32022`
- **AND** error data contains the requested version and supported modern version `2026-07-28`
- **WHEN** required protocolVersion or clientCapabilities metadata is absent or has the wrong JSON type
- **THEN** it returns `-32602`
- **AND** no method handler runs

### Scenario S04: Modern Skills methods require client extension capability
- **GIVEN** a valid modern version but client capabilities omit `io.modelcontextprotocol/skills`
- **WHEN** `skills/list` or `skills/get` is called
- **THEN** it returns `MissingRequiredClientCapability` code `-32021`
- **AND** error data identifies `io.modelcontextprotocol/skills`
- **AND** ordinary modern Tool and Resource methods remain usable

### Scenario S05: Process era cannot be mixed
- **GIVEN** a process whose opening request selected legacy or modern behavior
- **WHEN** a later request attempts to use the other era
- **THEN** it is rejected with a deterministic protocol-era error
- **AND** no handler or state mutation occurs
- **AND** restarting `okf mcp` permits either era to be selected afresh

## Requirement: Portable canonical OKF Agent Skill

The server SHALL render one portable Agent Skill from the existing W01–W07 workflow without copying client installation semantics.

### Scenario S06: Skill satisfies Agent Skills frontmatter minimum
- **WHEN** the portable Skill is rendered
- **THEN** it begins with closed YAML frontmatter
- **AND** frontmatter contains non-empty `name: okf` and `description`
- **AND** it has a Markdown body

### Scenario S07: Canonical workflow appears exactly once
- **WHEN** portable, Cursor, Claude Code and Codex workflow projections are rendered
- **THEN** W01 through W07 each appear exactly once in every applicable projection
- **AND** every referenced `okf_*` tool is present in both the canonical registered inventory and the modern Agent-tool inventory

### Scenario S08: Portable Skill excludes client ownership and privilege metadata
- **WHEN** the portable Skill is rendered
- **THEN** it contains no `OKF-MANAGED`, managed-file marker, client install/remove instruction, credential or absolute user path
- **AND** it contains no hook, script execution directive or `allowed-tools` field
- **AND** existing client-specific renderers retain their current ownership behavior

### Scenario S09: Advertised metadata is derived from served bytes
- **GIVEN** final portable `SKILL.md` bytes
- **WHEN** the Skill entry and Resource metadata are constructed
- **THEN** name, description and every frontmatter field equal the parsed frontmatter field-by-field
- **AND** no separate name or description constant can diverge from the bytes

## Requirement: Complete immutable Skill manifest

The server SHALL build an immutable static manifest for `skill://okf/SKILL.md` and fail startup if manifest/content consistency cannot be proven.

### Scenario S10: Static Skill entry is complete
- **WHEN** the registry is constructed
- **THEN** it contains exactly one Skill identified by `skill://okf/SKILL.md`
- **AND** its resources array contains exactly one entry for the same URI
- **AND** the digest is `sha256:` followed by 64 lowercase hexadecimal characters
- **AND** size equals raw byte length

### Scenario S11: URI and name constraints are enforced
- **GIVEN** a URI with a wrong scheme/case, userinfo, port, query, fragment, backslash, empty/dot segment, encoded slash/backslash/dot traversal, missing `/SKILL.md`, or final skill-path segment different from frontmatter name
- **WHEN** registry validation runs
- **THEN** construction fails before the Skill is advertised

### Scenario S12: Manifest completeness and uniqueness are enforced
- **GIVEN** duplicate URI, missing content, unlisted content, missing SKILL.md or Resource outside the canonical Skill root
- **WHEN** registry validation runs
- **THEN** construction fails closed

### Scenario S13: Digest, size and frontmatter mismatches fail closed
- **GIVEN** invalid digest syntax/value, negative/wrong size, integer-overflowing size sum or advertised frontmatter different from served SKILL.md
- **WHEN** registry validation runs
- **THEN** construction fails with a stable diagnostic category

### Scenario S14: Resource and byte limits include exact boundaries
- **GIVEN** 512 valid entries and exactly 16,777,216 total bytes
- **WHEN** registry validation runs
- **THEN** validation succeeds
- **GIVEN** 513 entries or more than 16,777,216 total bytes
- **WHEN** validation runs
- **THEN** it fails before serving any entry

### Scenario S15: Registry callers cannot mutate canonical state
- **WHEN** a caller mutates maps, slices or byte buffers returned by List, Get, Read or Resources
- **THEN** a later call returns the original canonical data
- **AND** concurrent read-only calls pass the race detector

### Scenario S16: Skill and discovery operations have no knowledge-runtime side effects
- **GIVEN** spies around legacy BundlePath auto-load, bundle loading, filesystem scanning, embedding and vector-index initialization
- **WHEN** server construction, modern opening, discovery, skills/list, skills/get and Skill resources/read run
- **THEN** every forbidden subsystem has zero calls
- **AND** legacy BundlePath auto-load occurs only after a legacy initialize selects the legacy era

## Requirement: Modern Skills methods

A modern request declaring the Skills client capability SHALL receive spec-conformant `skills/list` and `skills/get` behavior.

### Scenario S17: skills/list returns one atomic cacheable entry
- **WHEN** `skills/list` is called with valid modern metadata and omitted/empty cursor
- **THEN** it returns `resultType: complete`, one complete Skill and no `nextCursor`
- **AND** it returns `ttlMs: 300000`, `cacheScope: private` and serverInfo result `_meta`

### Scenario S18: skills/list cursor behavior is explicit
- **WHEN** `skills/list` receives a non-empty cursor
- **THEN** it returns `-32602` with the unknown-cursor category
- **AND** the next valid request still succeeds

### Scenario S19: skills/get equals the listed entry
- **WHEN** `skills/get` receives `skill://okf/SKILL.md` with valid modern metadata/capability
- **THEN** its Skill object deep-equals the `skills/list` entry
- **AND** it has complete result, same TTL/private scope and no pagination cursor

### Scenario S20: skills/get rejects non-entry URIs
- **WHEN** `skills/get` receives an unknown, directory or supporting-file URI
- **THEN** it returns `-32602`
- **AND** its message exposes no local path

### Scenario S21: malformed params fail without killing the server
- **WHEN** list/get params are malformed or get omits/non-stringifies `uri`
- **THEN** the server returns `-32602`
- **AND** the subsequent valid request succeeds

## Requirement: Resource compatibility in both eras

The Skill SHALL remain ordinary Resource content for Hosts that do not activate the extension.

### Scenario S22: Skill Resource is discoverable additively
- **WHEN** legacy or modern `resources/list` is called
- **THEN** it includes `skill://okf/SKILL.md` with MIME `text/markdown`
- **AND** name/description are derived from frontmatter
- **AND** legacy prior Resources retain their prior relative order before the appended Skill
- **AND** modern Resources contain only the static Skill Resource

### Scenario S23: Skill Resource read matches its manifest
- **WHEN** either era reads `skill://okf/SKILL.md`
- **THEN** returned bytes match advertised digest and size
- **AND** URI and MIME match the listed Resource
- **AND** modern response has complete result, private cache metadata and serverInfo result `_meta`
- **AND** legacy response retains its prior field shape

### Scenario S24: Unknown Skill Resources fail without fallback
- **WHEN** resources/read receives another `skill:` URI or traversal-shaped URI
- **THEN** it returns `-32602`
- **AND** it performs no bundle or filesystem lookup

### Scenario S25: Resource read does not activate a Skill
- **WHEN** a Resource-only Host lists and reads the Skill
- **THEN** OKF returns ordinary Resource content only
- **AND** it does not persist approval, install files, grant permissions or execute content

## Requirement: Modern core surface is complete and stateless

The modern era SHALL expose only existing OKF operations that do not depend on implicit cross-request bundle selection. Legacy behavior SHALL remain unchanged.

### Scenario S26: Every modern success result has the modern envelope
- **WHEN** server/discover, modern tools/list, tools/call, resources/list, resources/read, skills/list or skills/get succeeds
- **THEN** the result includes `resultType: complete` and serverInfo result `_meta`
- **AND** cacheable operations additionally include TTL/private scope
- **AND** non-cacheable tools/call omits TTL/cacheScope

### Scenario S27: Legacy shapes remain exact
- **WHEN** legacy initialize, list/read/call/get and ping operations run
- **THEN** their result JSON omits modern-only resultType/cache/result `_meta` fields
- **AND** every pre-existing field and definition is equivalent after deterministic JSON normalization
- **AND** only the documented appended Skill Resource changes resource count

### Scenario S28: Modern Tool catalog is deterministic and session-independent
- **WHEN** modern tools/list is called repeatedly before and after any modern tool call
- **THEN** it returns exactly the 11 service-backed Agent tools sorted by name
- **AND** it excludes the 9 legacy in-memory-bundle tools including `okf_load_bundle`
- **AND** W01–W07 reference only tools in this modern catalog
- **AND** every modern tool call resolves repository/knowledge configuration from server startup configuration and request arguments, not prior calls

### Scenario S29: Era-appropriate stdio framing works
- **WHEN** modern flows use newline-delimited JSON and legacy regression uses its existing supported framing modes
- **THEN** modern discovery, skills/list, skills/get, Skill resources/read and tools/list succeed through normative newline framing
- **AND** OKF's Content-Length compatibility path remains covered by a separate non-normative regression test

## Requirement: Security and trust boundaries are explicit

Implementation and documentation SHALL distinguish server integrity consistency from Host trust, approval and execution policy.

### Scenario S30: Digests are not described as trust
- **WHEN** documentation is inspected
- **THEN** it states that digest and size prove advertised/served byte consistency only
- **AND** it does not claim authorship, safety, approval or trust

### Scenario S31: Host responsibilities are documented
- **WHEN** security documentation is inspected
- **THEN** it assigns origin labeling, untrusted-input treatment, content-bound approval/reapproval, permission gating, origin-scoped reads and isolated caching to the Host
- **AND** it states that OKF Server does not perform those Host actions

### Scenario S32: Optional high-risk capabilities are absent
- **WHEN** discovery, Skills entries and Resource content are inspected
- **THEN** directoryRead is absent/false, resources is not dynamic and no nested Skill, executable, archive, hook or script exists
- **AND** modern capabilities omit unsupported Prompts, subscriptions and deprecated features

### Scenario S33: Errors and logs are redacted
- **GIVEN** malformed metadata/params, unknown URIs and registry failures
- **WHEN** responses and stderr are captured
- **THEN** no token, credential, environment value, user home path or Skill body is emitted

## Requirement: Real-client and conformance validation

The change SHALL prove modern protocol behavior and current-client compatibility without overstating Host capabilities.

### Scenario S34: Direct modern protocol fixture completes the full flow
- **WHEN** the persisted client launches `okf mcp`
- **THEN** it completes server/discover, direct per-request metadata validation, skills/list, skills/get, resources/read, tools/list and one read-only tools/call
- **AND** it verifies capabilities, entry equality, digest, size, W01–W07 and 11 modern tools
- **AND** it separately proves unsupported-version and missing-capability errors

### Scenario S35: Real Codex Resource compatibility works
- **GIVEN** OKF is configured in an isolated Codex home
- **WHEN** real Codex discovers and reads the Skill without mutating knowledge
- **THEN** it reports readable=yes, name=okf, first=W01, last=W07 and no mutating OKF call
- **AND** evidence labels the connection era and Resource compatibility honestly

### Scenario S36: Native Host status is reported honestly
- **WHEN** no production Host used by the test exposes native modern `skills/list/get`
- **THEN** conformance marks that interoperability `blocked_client_support`
- **AND** the direct modern protocol fixture remains mandatory and fully passing
- **AND** Resource compatibility is not renamed as native support

### Scenario S37: Performance is independent of bundle size
- **GIVEN** empty and 1,000-file repository fixtures
- **WHEN** discovery/Skill list/get/read benchmarks run
- **THEN** knowledge-storage, embedding and vector calls remain zero
- **AND** ns/op, B/op and allocs/op are recorded over at least 10,000 operations
- **AND** any CI timing threshold is calibrated from recorded baseline rather than assumed in this Spec

### Scenario S38: Final quality and alignment gates pass
- **WHEN** implementation is ready for merge
- **THEN** gauntlet, full build/vet/test/race/shuffle, legacy MCP E2E, modern/Skills E2E and real Codex compatibility all run after the last code edit
- **AND** targeted mutants for version validation, capability gating, digest, completeness, stateless tool filtering and wrapper leakage are killed
- **AND** conformance maps S01–S38 to implementation, tests and fresh evidence with no unexplained partial/gap
