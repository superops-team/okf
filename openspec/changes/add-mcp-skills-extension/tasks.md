# Implementation Tasks

## Completion contract

P0–P5 are dependency order only. This change is complete only when all 38 Scenarios, all public and compatibility entry points, final evidence and `conformance.md` are complete. A passing PoC or a partial phase is not a releasable subset.

## P0 — Executable protocol contract (0.75 person-day)

### T0.1 Lock negotiated protocol profiles
- **Files**: `pkg/mcp/protocol.go`, focused tests.
- **RED first**: exact Skills revision, exact legacy revision, empty/malformed/older/unknown-newer versions, profile replacement after reinitialize.
- **Implement**: closed supported-revision table and session profile; no lexical date comparison.
- **Tests**: `TestProtocolProfileNegotiation`, `TestProtocolProfileReinitialize`, JSON shape goldens.
- **Scenarios**: S01–S05.

### T0.2 Define Skills wire types
- **Files**: `pkg/mcp/protocol.go`.
- **Implement**: SkillResource, Skill with static/dynamic-capable representation if needed by the schema, list/get params/results and shared cache result metadata.
- **Tests**: marshal/unmarshal golden fixtures derived from pinned ext-skills commit.
- **Scenarios**: S10, S17–S21, S26–S27.

## P1 — Portable canonical Skill (0.5 person-day)

### T1.1 Extract portable renderer
- **Files**: `pkg/agentconfig/workflow.go`, tests.
- **RED first**: W01–W07 once, registered tool references, no ownership marker, no privilege metadata.
- **Implement**: `RenderAgentSkill()` over the existing canonical clause renderer; retain current client wrappers.
- **Tests**: extend `TestCanonicalWorkflowCoverage`, `TestGeneratedToolsExist`, add `TestPortableSkillHasNoClientOwnership`.
- **Scenarios**: S06–S09.

## P2 — Immutable registry (1.0 person-day)

### T2.1 Build and validate one static Skill
- **Files**: `pkg/mcp/skills.go`, `skills_test.go`.
- **RED first**: valid manifest plus each independent failure branch.
- **Implement**: render once, parse frontmatter, compute digest/size, validate and clone outputs.
- **Tests**: `TestSkillRegistryEntry`, `TestSkillRegistryDefensiveCopies`, `TestSkillFrontmatterIdentity`.
- **Scenarios**: S09–S10, S15–S16.

### T2.2 Fail-closed URI, completeness and limits
- **Files**: same.
- **Fixtures**: wrong schemes, query/fragment, backslashes, dot/encoded traversal, duplicate/missing/unlisted/out-of-root Resources, digest/size/frontmatter mismatch, 512/513 files, exact/over 16 MiB.
- **Tests**: table-driven validation and `testing/quick` path-property tests.
- **Scenarios**: S11–S14.

### T2.3 Startup error propagation
- **Files**: `pkg/mcp/server.go`, constructors and tests.
- **Implement**: registry construction returns errors; server/CLI startup reports invalid built-in Skill without panic or partial service.
- **Tests**: injected invalid registry factory and CLI startup exit check.
- **Scenarios**: S12–S14, S33.

## P3 — MCP wiring and compatibility (1.25 person-days)

### T3.1 Capability-gated methods
- **Files**: `pkg/mcp/server.go`, protocol tests.
- **Implement**: initialize negotiation; gated `skills/list/get`; strict params/cursor/error codes.
- **Tests**: request/response matrix for pre-init, legacy, Skills, reinitialize and malformed requests.
- **Scenarios**: S01–S05, S17–S21.

### T3.2 Shared Resource registry path
- **Files**: `pkg/mcp/server.go` plus the minimal Resource registry helper.
- **Implement**: append Skill Resource; read exact bytes; resolve Skill namespace before bundle namespace; retain previous resource order.
- **Tests**: list/read parity, unknown/traversal no-fallback spies, legacy resource additive golden.
- **Scenarios**: S22–S25.

### T3.3 Cacheable new shapes and exact legacy shapes
- **Files**: protocol result DTOs and list/read handlers.
- **Implement**: new profile emits resultType/TTL/private scope; legacy omits them.
- **Tests**: canonicalized JSON goldens for tools/resources/prompts and unchanged calls/get operations.
- **Scenarios**: S26–S29.

## P4 — Security, compatibility and real execution (1.25 person-days)

### T4.1 Persist ext-skills stdio E2E
- **Files**: `test_ext_skills.py` or `tools/test-ext-skills.py`.
- **Implement**: build/launch real `okf mcp`, run full flow under newline and Content-Length, verify entries and all 20 tools.
- **Tests**: executable script exits nonzero on any mismatch and has a negative-control fixture.
- **Scenarios**: S23, S29, S34.

### T4.2 Existing MCP regression
- **Files**: `test_mcp.py` only if expectations need additive Resource count.
- **Implement**: keep all prior assertions; explicitly assert legacy response shapes and Skill Resource addition.
- **Scenarios**: S22, S27–S28.

### T4.3 Real Codex compatibility harness
- **Files**: persisted shell/Python harness under `tools/`, documentation.
- **Implement**: isolated Codex home; generated MCP config; resource discovery/read; assert no mutating OKF tool call; redact event output.
- **Fallback**: if authentication/client unavailable, report `blocked_auth`/`blocked_client` and keep direct protocol fixture mandatory.
- **Scenarios**: S25, S35–S36.

### T4.4 Adversarial and mutation set
- **Files**: `tools/mutants-ext-skills.sh`, tests.
- **Mutants**: skip digest check; allow missing content; expose Skills method under legacy profile; reuse client-owned Skill wrapper; accept URI escape.
- **Negative controls**: prove each custom gate exits nonzero when a known bad fixture is introduced.
- **Scenarios**: S11–S16, S24, S32–S33, S38.

### T4.5 Performance and side-effect checks
- **Files**: Go benchmark plus forbidden-subsystem spies.
- **Run**: empty and 1,000-file bundle; 10,000 list/get/read operations.
- **Record**: ns/op, B/op, allocs/op, knowledge-storage read calls.
- **Scenarios**: S16, S37.

## P5 — Documentation, release and conformance (0.75 person-day)

### T5.1 User/developer documentation
- **Files**: README, README.zh-CN, MCP docs, security notes and Release Notes.
- **Cover**: three coexistence paths, experimental status, URI, negotiation, cache scope, inspection versus activation, digest limits, Host obligations and unsupported features.
- **Tests**: documentation contract assertions for prohibited claims and required terms.
- **Scenarios**: S25, S30–S32, S35–S36.

### T5.2 Final evidence and conformance
- **Files**: `evidence.md`, `conformance.md`.
- **Fresh run**: `tools/gauntlet.sh`; full build/vet/test/race/shuffle; existing MCP E2E; ext-skills E2E; real Codex compatibility; benchmarks; mutants.
- **Gate**: map S01–S38 to implementation symbol, automated test, real entry point and fresh result. `blocked_client_support` is allowed only for a production native Host and must not mask direct protocol fixture failure.
- **Scenarios**: S34–S38.

## Scenario → test → real entry-point matrix

| Scenario | Primary automated test | Real entry point/evidence |
|---|---|---|
| S01 | `TestProtocolProfileNegotiation/skills` | Skills initialize |
| S02 | `TestProtocolProfileNegotiation/legacy` | existing MCP initialize |
| S03 | unsupported revision table tests | stdio initialize matrix |
| S04 | `TestSkillsMethodsRequireCapability` | legacy stdio request |
| S05 | `TestProtocolProfileReinitialize` | serial stdio session |
| S06 | `TestPortableSkillFrontmatter` | Skill Resource read |
| S07 | canonical workflow coverage/parity | Codex reads W01–W07 |
| S08 | `TestPortableSkillHasNoClientOwnership` | inspect Resource bytes |
| S09 | `TestSkillFrontmatterIdentity` | list/get/read parity |
| S10 | `TestSkillRegistryEntry` | skills/list/get |
| S11 | URI validation table + property test | invalid registry fixture |
| S12 | completeness table | invalid registry fixture |
| S13 | digest/size/frontmatter mismatch tests | negative-control server |
| S14 | exact/over limits tests | registry startup |
| S15 | `TestSkillRegistryDefensiveCopies` | repeated list/get/read |
| S16 | forbidden-subsystem spy | empty/1000-file run |
| S17 | `TestSkillsListResult` | ext-skills E2E |
| S18 | list cursor cases | ext-skills E2E negative call |
| S19 | `TestSkillsListGetEquality` | ext-skills E2E |
| S20 | get URI errors | ext-skills E2E negative call |
| S21 | malformed params recovery | serial stdio fixture |
| S22 | additive Resource golden | legacy + Skills resources/list |
| S23 | digest/size read test | both-framing E2E |
| S24 | unknown/traversal no-fallback spy | negative stdio call |
| S25 | no-side-effect test | real Codex event trace |
| S26 | new response-shape golden | Skills stdio matrix |
| S27 | legacy response-shape golden | existing MCP E2E |
| S28 | existing regression suite | `python3 test_mcp.py` |
| S29 | framing parity | ext-skills E2E ×2 |
| S30 | documentation contract test | release docs inspection |
| S31 | security documentation test | release docs inspection |
| S32 | capability/manifest tests | initialize/list output |
| S33 | error/log redaction tests | captured stderr fixture |
| S34 | persisted protocol E2E | real server process |
| S35 | Codex harness assertions | real Codex process |
| S36 | client support status contract | conformance report |
| S37 | benchmarks + storage spies | empty/1000-file benchmark |
| S38 | conformance gate + mutants | final fresh gauntlet |

## Estimated schedule

| Phase | Estimate | Exit criterion |
|---|---:|---|
| P0 | 0.75 d | revision matrix and wire schema RED/GREEN |
| P1 | 0.5 d | portable renderer parity |
| P2 | 1.0 d | immutable fail-closed registry |
| P3 | 1.25 d | native methods + exact compatibility |
| P4 | 1.25 d | adversarial, framing, legacy and Codex verification |
| P5 | 0.75 d | docs, evidence and conformance |
| **Total** | **5.5 person-days** | all phases, not only P0 |
