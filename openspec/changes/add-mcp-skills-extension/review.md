# Review: MCP Skills Extension

## Review scope

- Proposal, design, S01–S38 and P0–P5 implementation plan.
- Pinned protocol source: ext-skills commit `d866efdba298b55b8156c7b7aa1bdebc1b625f4c` and SEP-2640.
- Existing OKF MCP, Agent Integration and project `AGENTS.md` constraints.
- Local PoC evidence is used only to prove feasibility; it is not treated as completion evidence.

## Overall verdict

**Approved for TDD implementation after human approval of this Spec.** The design is intentionally limited to one static instructor Skill and extends the current MCP server without a second workflow source or another runtime. All 38 Scenarios have planned automated tests and real entry points. No unresolved design question is required to start RED tests.

## Issues found and corrections applied

| ID | Severity | Finding | Correction in current Spec |
|---|---|---|---|
| R01 | High | PoC enabled `skills/list/get` by routing alone, even for a legacy session. Capability negotiation would be decorative. | S04 and T3.1 require `-32601` unless the Skills profile was negotiated. |
| R02 | High | Exact-string-only logic did not define empty, malformed, older or future revisions. Lexical date comparison would imply unsupported future compatibility. | S03 and Design §3 define a closed supported-revision table and legacy fallback. |
| R03 | High | PoC constructor panicked on an invalid built-in registry, which is difficult to test and produces weak startup diagnostics. | Design §5 and T2.3 require error-returning construction and startup propagation without partial service. |
| R04 | High | Existing MCP Resource URIs expose local absolute paths. Rewriting them inside this feature could break persisted clients. | Proposal compatibility decision explicitly preserves them and requires a separate stable-URI migration Spec. |
| R05 | Medium | Legacy `resources/list` gains one Resource; calling the entire old output byte-identical would be false. | S22/S27 define exact additive compatibility: previous entries/order unchanged, Skill appended, legacy field shape unchanged. |
| R06 | Medium | Skill frontmatter metadata could become a second fact source. | S09 derives all advertised metadata from final served bytes and compares field-by-field. |
| R07 | Medium | Prefix-only URI checks could accept query/fragment, encoded traversal, dot segments or backslashes. | S11 enumerates parsed-URI rejection cases and requires property tests. |
| R08 | Medium | Returning internal slices/maps would let tests or handlers corrupt registry state. | S15 requires defensive copies for List/Get/Read/Resources. |
| R09 | Medium | Digest verification could be misrepresented as trust verification. | S30–S31 and Design §8 separate Server consistency from Host trust/approval duties. |
| R10 | Medium | Real Codex Resource reading could be overstated as native SEP-2640 support. | S35–S36 require explicit `Resource compatibility` labeling and `blocked_client_support` for native production Host support. |
| R11 | Medium | Performance wording initially depended on p95 without a defined measurement tool. | T4.5 defines a persisted benchmark/harness, records ns/op/B/op/allocs and uses a conservative CI bound; bundle-side-effect count is the deterministic gate. |
| R12 | Low | A generic plugin registry would over-design the first release. | Design §5 fixes one concrete immutable static registry; abstraction waits for a second real Skill. |
| R13 | Low | Optional directory read, dynamic Skills and nested Skills increase risk without first-release value. | Explicit non-goals and S32 prevent capability advertisement or accidental publication. |
| R14 | Low | Private cache choice lacked a formal reason. | Proposal and Design tie private scope to project/server context and prohibit claims that cache scope establishes integrity. |

## 19-dimension compliance review

| # | Dimension | Result | Evidence / decision |
|---:|---|---|---|
| 1 | Context logic | Pass | Existing canonical workflow → portable renderer → immutable registry → existing MCP server; no parallel architecture. |
| 2 | No empty claims | Pass | Every requirement has concrete inputs, outputs, errors and tests. |
| 3 | No ambiguity | Pass | Versions, URI, TTL, cache scope, method gating, limits, error codes and additive legacy behavior are fixed. |
| 4 | Model-readable semantics | Pass | Server, Host, Resource inspection and Skill activation are explicitly distinguished. |
| 5 | SDD/TDD fit | Pass | P0 begins with wire/negotiation RED tests; each task lists RED fixtures before implementation. |
| 6 | Minimal implementation | Pass | Two existing packages plus one focused registry; no command, daemon or plugin framework. |
| 7 | Backward compatibility | Pass with declared additive change | Legacy shapes preserved; one Resource is appended; existing URI migration deferred. |
| 8 | Existing behavior impact | Pass | All 20 Tools, Prompts, bundle reads, durable restart behavior and client adapters have regression gates. |
| 9 | Runtime failure risk | Pass | Startup fail-closed, params/cursor errors, post-error recovery, route gating and stderr redaction are covered. |
| 10 | Feasibility | Pass | PoC passed full build/vet/test, both stdio framings, legacy E2E and real Codex Resource use. |
| 11 | Layered plan/tests/schedule | Pass | P0–P5, 5.5 person-days, exit criteria and S01–S38 matrix are explicit. |
| 12 | Extensibility | Pass | Static registry can later be generalized, but current code does not pay that abstraction cost. |
| 13 | Avoid over-design | Pass | No directory read, dynamic/nested Skill, HTTP, RBAC, signature or archive. |
| 14 | Small/high-value change | Pass | Reuses Resources, YAML dependency, MCP entry point and canonical workflow. |
| 15 | Continuous improvement | Pass | Eliminates metadata duplication, adds typed session profile and defensive immutable API. |
| 16 | Architectural consistency | Pass | Existing `pkg/agentconfig` owns workflow rendering; `pkg/mcp` owns transport and registry. |
| 17 | Every requirement wired/tested | Pass in plan | Matrix contains an automated test and real entry point for every Scenario. Completion gate prevents unchecked boxes. |
| 18 | Priorities are dependencies only | Pass | Completion contract explicitly requires P0–P5. |
| 19 | Spec/implementation consistency | Pass in plan | T5.2 mandates fresh `conformance.md`, no unexplained partial/gap, client-support blockers named honestly. |

## SDD/TDD execution obstacles and mitigation

1. **New worktree lacks gitignored ONNX assets**: MCP package build can fail before a test reaches new code. The implementation worktree must either reuse/copy the same local assets or rebuild them by the repository-supported mechanism; evidence must disclose this environment step.
2. **Protocol goldens can become brittle**: compare canonical JSON structures, not encoder key order. Keep one raw-wire fixture to guard omission/presence semantics.
3. **Timing tests can flake**: deterministic gates are zero bundle/embedding/index calls and bounded allocations. Performance numbers are recorded from a benchmark; the CI ceiling is deliberately loose.
4. **Real Codex requires a live model channel**: direct protocol E2E remains mandatory. Codex is an additional compatibility proof and may be marked blocked only for external authentication/client availability, never as a replacement for a failing server test.
5. **Experimental upstream protocol may change**: pin the reviewed commit in metadata/release notes; upgrading the protocol requires a new Spec amendment and refreshed goldens.

## Residual risks accepted for this release

- Native production Host interoperability may remain `blocked_client_support`; the direct protocol fixture proves Server conformance and real Codex proves ordinary Resource compatibility.
- Existing absolute-path `okf://` Resources remain. This is known technical debt isolated from the new `skill://` namespace.
- SHA-256 is unsigned and not an authenticity mechanism.
- Hosts may misuse Resource reads as activation; OKF cannot enforce Host behavior and therefore documents the boundary precisely.

## Approval gate

Implementation must not begin until the user approves this exact Spec set. After approval, any semantic change to S01–S38 requires an explicit Spec amendment before changing assertions or implementation.
