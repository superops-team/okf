# Implementation Tasks

## Completion contract

P0–P5 are dependency order only. Completion requires all 38 Scenarios, both protocol eras, every exposed method, security/compatibility tests, final evidence and `conformance.md`. A passing PoC or one phase is not releasable.

## P0 — Dual-era executable base contract (1.5 person-days)

### T0.1 Request/result metadata and modern errors
- **Files**: `pkg/mcp/protocol.go`, focused tests.
- **RED first**: valid/missing/wrong-type `_meta`; supported/unsupported versions; error data; required resultType/serverInfo.
- **Implement**: modern request metadata types, result metadata, `-32022` unsupported version and `-32021` missing capability.
- **Tests**: schema/golden tests sourced from pinned official base protocol.
- **Scenarios**: S01, S03–S04, S26.

### T0.2 Dual-era process dispatcher
- **Files**: `pkg/mcp/server.go`, tests.
- **RED first**: modern discovery opening, direct modern request, legacy initialize, ambiguous opening, mixed-era rejection, restart reset, and BundlePath auto-load timing.
- **Implement**: one-era-per-stdio-process dispatcher; modern behavior determined per request; legacy state and deferred BundlePath auto-load isolated behind successful legacy initialize.
- **Tests**: `TestDualEraOpening`, `TestModernDirectRequest`, `TestMixedEraRejected`, `TestModernOpeningDoesNotLoadBundle`.
- **Scenarios**: S01–S05, S16.

### T0.3 Modern server/discover and shared server version
- **Files**: protocol DTOs, `server.go`, existing version metadata package.
- **Implement**: supported modern version, modern capabilities, instructions, serverInfo result `_meta`, private cache fields; remove MCP handler version literals so legacy initialize and modern results share one version source.
- **Tests**: raw-wire discovery golden, legacy/modern serverInfo parity and unsupported-version data.
- **Scenarios**: S01, S03, S26, S32.

### T0.4 Modern complete-result adaptation
- **Files**: response wrapper/handlers.
- **Implement**: add `resultType` and serverInfo `_meta` to every modern success; add TTL/scope only to cacheable methods.
- **Tests**: method matrix for discovery, tools list/call, resources list/read and future Skills methods.
- **Scenarios**: S26–S27.

## P1 — Portable canonical Skill (0.5 person-day)

### T1.1 Portable renderer
- **Files**: `pkg/agentconfig/workflow.go`, tests.
- **RED first**: W01–W07 once, registered modern tool references, no ownership/privilege markers.
- **Implement**: `RenderAgentSkill()` over canonical clauses; preserve existing client wrappers.
- **Scenarios**: S06–S09.

## P2 — Immutable registry (1.0 person-day)

### T2.1 Static Skill and defensive API
- **Files**: `pkg/mcp/skills.go`, `skills_test.go`.
- **Implement**: render once, parse all frontmatter, compute digest/size, validate, clone outputs.
- **Tests**: valid entry, frontmatter identity, defensive copies, concurrent reads, forbidden subsystem spies.
- **Scenarios**: S09–S10, S15–S16.

### T2.2 URI, completeness and limits
- **Fixtures**: wrong/case-varied schemes, userinfo/port/query/fragment, backslash, empty/dot/encoded traversal, duplicate/missing/unlisted/out-of-root entries, bad digest, negative/wrong/overflow size, exact/over limits.
- **Tests**: table-driven plus `testing/quick` URI properties.
- **Scenarios**: S11–S14.

### T2.3 Startup error propagation
- **Implement**: error-returning registry/server construction and CLI propagation; no panic/partial catalog.
- **Tests**: injected invalid registry factory, CLI exit/stderr redaction.
- **Scenarios**: S12–S14, S33.

## P3 — Skills, Resources and stateless modern tools (1.75 person-days)

### T3.1 Skills capability and methods
- **Implement**: client-capability validation, list/get, cursor and URI errors, cache/result metadata.
- **Tests**: modern request matrix including direct call without discovery.
- **Scenarios**: S04, S17–S21.

### T3.2 Resource compatibility
- **Implement**: append Skill to legacy Resources; expose only static Skill Resource in modern era; exact read; Skill namespace no-fallback.
- **Tests**: additive legacy golden, modern Resource golden, read parity, traversal spies.
- **Scenarios**: S22–S25.

### T3.3 Split deterministic tool catalogs by era
- **Files**: `pkg/mcp/tools.go`, `server.go`, tests.
- **Implement**: keep all 20 legacy tools; expose only 11 service-backed Agent tools in modern tools/list/call; sorted deterministic lists; reject legacy-only modern call.
- **Tests**: exact name sets, before/after call stability, W01–W07 closure, no shared bundle state.
- **Scenarios**: S07, S26–S28.

### T3.4 Modern Prompts and unsupported methods
- **Implement**: omit Prompts capability in modern discovery; modern prompts/list/get and ping return method-not-found; legacy paths unchanged.
- **Tests**: explicit per-era method matrix.
- **Scenarios**: S02, S27, S32.

## P4 — Security, compatibility and real execution (1.5 person-days)

### T4.1 Persist modern/Skills stdio E2E
- **Files**: `test_ext_skills.py` or `tools/test-ext-skills.py`.
- **Run**: normative newline modern flow: discover, direct request, list/get/read/tools/read-only call, version/capability negative cases.
- **Negative control**: script must fail against a server missing modern resultType or accepting a missing capability.
- **Scenarios**: S17–S24, S26, S28, S34.

### T4.2 Legacy and framing regression
- **Implement**: retain `test_mcp.py`; explicitly assert 20 tools, Prompts, ping, shapes and additive Skill Resource. Test Content-Length as OKF compatibility only.
- **Scenarios**: S02, S22–S23, S27, S29.

### T4.3 Real Codex compatibility harness
- **Implement**: isolated home, generated config, Resource discovery/read, mutation absence, event redaction and era label.
- **Fallback**: external auth/client absence may produce `blocked_auth`/`blocked_client`; direct modern fixture cannot be blocked.
- **Scenarios**: S25, S35–S36.

### T4.4 Adversarial/mutation set
- **Mutants**: skip version check; skip client capability; advertise Prompts modernly; expose `okf_load_bundle` modernly; bypass digest/completeness; leak client wrapper; accept URI escape.
- **Negative controls**: every gate proven able to fail.
- **Scenarios**: S03–S04, S08, S11–S16, S24, S28, S32–S33, S38.

### T4.5 Benchmarks and side effects
- **Run**: empty and 1,000-file fixtures; at least 10,000 discovery/list/get/read operations.
- **Record**: ns/op, B/op, allocs/op and forbidden storage/runtime calls. Calibrate any CI wall-time gate from evidence.
- **Scenarios**: S16, S37.

## P5 — Documentation, release and conformance (0.75 person-day)

### T5.1 Documentation
- **Cover**: dual eras; modern request `_meta`; supported methods/tool counts; three Skill access paths; experimental status; inspection vs activation; integrity vs trust; Host duties; unsupported modern Prompts/ping/directory/dynamic/nested features; Content-Length compatibility status.
- **Tests**: required/prohibited claim contract checks.
- **Scenarios**: S25, S30–S32, S35–S36.

### T5.2 Final evidence and alignment
- **Fresh run after last edit**: gauntlet, build/vet/test/race/shuffle, legacy E2E, modern E2E, Content-Length regression, Codex compatibility, benchmarks and mutants.
- **Gate**: `conformance.md` maps S01–S38. Only native production-Host support may be `blocked_client_support`; no direct protocol gap is allowed.
- **Scenarios**: S34–S38.

## Scenario → test → real entry-point matrix

| Scenario | Primary automated test | Real entry point/evidence |
|---|---|---|
| S01 | modern discovery golden | `server/discover` |
| S02 | legacy initialize regression | legacy MCP E2E |
| S03 | metadata/version table | unsupported direct request |
| S04 | capability-gating test | Skills call without extension |
| S05 | mixed-era/restart test | two real process runs |
| S06 | portable frontmatter test | Resource read |
| S07 | workflow/tool-catalog parity | modern tools/list + Codex |
| S08 | ownership/privilege negative test | served bytes inspection |
| S09 | frontmatter identity test | list/get/read parity |
| S10 | static entry test | skills/list/get |
| S11 | URI table/property test | invalid registry fixture |
| S12 | completeness table | invalid registry fixture |
| S13 | digest/size/frontmatter/overflow tests | startup negative control |
| S14 | exact/over boundary tests | registry construction |
| S15 | defensive copy + race tests | repeated concurrent reads |
| S16 | forbidden-subsystem spies | empty/1000-file run |
| S17 | skills/list modern result test | modern E2E |
| S18 | cursor/recovery test | negative then valid request |
| S19 | list/get deep equality | modern E2E |
| S20 | get URI error table | modern E2E negatives |
| S21 | malformed params recovery | serial modern fixture |
| S22 | per-era Resource catalogs | legacy/modern list |
| S23 | per-era read shape/digest | legacy/modern read |
| S24 | no-fallback spies | traversal read negative |
| S25 | no-side-effect assertions | Codex event trace |
| S26 | all-modern-result matrix | modern full method E2E |
| S27 | legacy raw/canonical goldens | `test_mcp.py` |
| S28 | exact modern 11/legacy 20 catalogs | per-era tools/list/call |
| S29 | newline normative + Content-Length compatibility | separate process tests |
| S30 | documentation contract | release docs |
| S31 | Host-boundary contract | security docs |
| S32 | per-era capabilities/method tests | discover + legacy init |
| S33 | response/stderr redaction | captured failure run |
| S34 | persisted modern protocol E2E | real server process |
| S35 | Codex harness | real Codex process |
| S36 | support-status contract | conformance report |
| S37 | benchmark + storage spies | empty/1000-file benchmark |
| S38 | conformance + mutants | final fresh gauntlet |

## Estimated schedule

| Phase | Estimate | Exit criterion |
|---|---:|---|
| P0 | 1.5 d | complete dual-era base contract |
| P1 | 0.5 d | portable renderer parity |
| P2 | 1.0 d | immutable fail-closed registry |
| P3 | 1.75 d | Skills/Resources and stateless modern tool boundary |
| P4 | 1.5 d | adversarial, both-era and real Codex verification |
| P5 | 0.75 d | docs, evidence and conformance |
| **Total** | **7.0 person-days** | all phases, not only P0 |
