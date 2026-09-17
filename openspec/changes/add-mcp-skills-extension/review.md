# Spec Review: MCP Skills Extension

## Scope and method

Two complete review rounds were performed against:

- proposal, design, S01–S38, P0–P5 plan and metadata;
- current OKF MCP server/tool state model;
- ext-skills commit `d866efdba298b55b8156c7b7aa1bdebc1b625f4c` and SEP-2640;
- official MCP `2026-07-28` changelog, versioning, discovery, stdio, Tools, Resources and Prompts specifications;
- project `AGENTS.md` 19-dimension and SDD/TDD completion rules.

The first committed Spec (`8d72343`) was the review baseline. This file records issues found and corrections actually applied.

## Final verdict

**Pass after material redesign; ready for human approval and then RED tests.**

The original Spec's Skill registry direction was sound, but its base-protocol model was not: it treated modern `2026-07-28` as a newer initialize-based session. The reviewed Spec now implements the minimum complete dual-era boundary, preserves legacy behavior and restricts modern exposure to stateless operations. No unresolved design question blocks TDD.

## Round 1 — protocol facts and explicit correctness

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| R1-01 | Critical | `2026-07-28` removes initialize/session state; original S01–S05 negotiated it through initialize. | Replaced with modern per-request `_meta`, mandatory `server/discover`, direct modern calls and dual-era dispatch. |
| R1-02 | Critical | Unknown modern versions were silently downgraded to legacy, contrary to required `UnsupportedProtocolVersionError`. | Added `-32022` with requested/supported data; only actual legacy initialize selects legacy. |
| R1-03 | High | Client extension capability was not required for Skills calls. | Added per-request capability gate and `-32021 MissingRequiredClientCapability`. |
| R1-04 | High | Original design added `resultType` only to selected list/read results. Modern protocol requires it on every successful result. | Added a common modern result envelope, including tools/call and non-cacheable results. |
| R1-05 | High | Modern result serverInfo was absent. | Added `_meta.io.modelcontextprotocol/serverInfo` to every modern result. |
| R1-06 | High | `server/discover`—mandatory for modern servers—was absent. | Added discovery DTO, method, caching fields, capabilities, versions and instructions. |
| R1-07 | Medium | Modern stdio was described as also using Content-Length normatively. | Newline JSON is normative; Content-Length is an explicit OKF compatibility regression only. |
| R1-08 | Medium | Empty/malformed modern metadata was grouped with unknown version fallback. | Missing/wrong-type metadata is `-32602`; unsupported non-empty version is `-32022`. |
| R1-09 | Medium | Fixed p95 requirement was not reproducible with standard Go benchmarks. | Replaced with deterministic zero-I/O/O(1) gates plus recorded ns/op/B/op/allocs; timing threshold requires measured calibration. |
| R1-10 | Low | Upstream base-protocol sources were not pinned in references. | Added official changelog/versioning/discovery/transport/method pages and superseded assumptions. |

## Round 2 — hidden state, compatibility and maintainability

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| R2-01 | Critical | Existing `okf_load_bundle` mutates in-memory bundle state used by later tools. Exposing all 20 tools modernly violates stateless request semantics. | Modern catalog is exactly the 11 Service-backed Agent tools; legacy keeps all 20. Exact set and exclusion tests added. |
| R2-02 | High | Existing Prompts instruct use of legacy in-memory-bundle tools. Advertising them modernly would reference unavailable/sessionful operations. | Modern discovery omits Prompts and modern prompt methods are not supported; legacy remains unchanged. |
| R2-03 | High | Constructor auto-loads BundlePath before knowing the era, causing modern discovery/Skill requests to read knowledge storage. | BundlePath auto-load is deferred until successful legacy initialize; modern startup/operations have zero legacy bundle loads. |
| R2-04 | High | Existing bundle/Concept Resource catalog varies with mutable bundle state. | Modern Resources contain only the static Skill; legacy keeps prior Resources and appended Skill. |
| R2-05 | Medium | Interleaving modern and legacy requests in one stdio process could leak mutable legacy state into modern calls. | One era is selected per process; mixed-era request is rejected; restart resets selection. |
| R2-06 | Medium | URI tests did not explicitly cover userinfo, port, scheme case, empty segment and integer overflow. | S11/S13 and T2.2 now enumerate them. |
| R2-07 | Medium | The exact 512/16 MiB acceptance boundary was not clearly separated from rejection. | S14 now has explicit accepted and rejected GIVEN blocks. |
| R2-08 | Medium | Race safety of defensive registry copies was implicit. | S15 requires concurrent read-only calls under the race detector. |
| R2-09 | Medium | W01–W07 tool parity checked the global inventory but not the modern published subset. | S07/T3.3 require closure against the exact modern 11-tool inventory. |
| R2-10 | Medium | Discovery initially listed the legacy initialize revision as a valid modern per-request version. | Discovery and `-32022.supported` now list only `2026-07-28`; legacy is documented as initialize fallback. |
| R2-11 | Medium | MCP serverInfo version was a handler literal and could drift between legacy and modern responses. | T0.3 and S01 require a shared existing version source and cross-era parity test. |
| R2-12 | Low | Skill/additive Resource behavior could be mistaken for identical catalogs across eras. | S22 explicitly defines legacy prior Resources + Skill versus modern Skill-only Resources. |

## 19-dimension review after fixes

| # | Dimension | Result | Reviewed outcome |
|---:|---|---|---|
| 1 | Context logic | Pass | Dual-era base contract precedes Skills extension; no incompatible session assumptions remain. |
| 2 | No empty claims | Pass | Every behavior has wire fields, error codes or observable side effects. |
| 3 | No ambiguity | Pass | Era selection, modern metadata, catalogs, method support, URI rules and boundaries are fixed. |
| 4 | Semantic precision | Pass | Modern/legacy, inspection/activation and consistency/trust are distinct terms. |
| 5 | SDD/TDD fit | Pass | P0 starts with wire RED tests; superseded PoC cannot count as GREEN. |
| 6 | Minimal implementation | Pass | Minimum modern base boundary plus Skills; no HTTP/subscriptions/MRTR/plugin framework. |
| 7 | Backward compatibility | Pass with declared additive Resource | Legacy 20 tools, Prompts, ping and shapes preserved. |
| 8 | Existing behavior impact | Pass | Exact per-era catalogs and regression suites are specified. |
| 9 | Runtime risks | Pass | Invalid metadata, mixed era, state leakage, URI escape, startup failure and post-error recovery are covered. |
| 10 | Feasibility | Pass | Registry/Resource feasibility proven; modern redesign uses official protocol facts and existing Service tools. |
| 11 | Layered plan/test/schedule | Pass | P0–P5 updated to 7.0 person-days with exit criteria. |
| 12 | Extensibility | Pass | Modern envelope/era boundary reusable; registry remains concrete until a second Skill. |
| 13 | Avoid over-design | Pass | No Streamable HTTP, dynamic/nested Skills, directory reads, signatures or Host cache implementation. |
| 14 | Small/high-value | Pass | 11 existing stateless tools reused; nine stateful tools are not rewritten. |
| 15 | Continuous improvement | Pass | Removes metadata duplication and makes hidden legacy state explicit. |
| 16 | Architectural consistency | Pass | Service-backed tools define modern path; legacy ToolRegistry state remains isolated. |
| 17 | Every requirement wired/tested | Pass in plan | 38/38 matrix includes automated and real entry points. |
| 18 | Priorities are dependencies only | Pass | Completion requires P0–P5 and both eras. |
| 19 | Spec/implementation consistency | Pass in plan | Fresh conformance and evidence required after last edit. |

## TDD execution constraints

1. Modern result-envelope tests must be written before handler adaptation; otherwise partial modern support can appear green.
2. Tool catalog splitting must use one registration source with explicit era filtering, not two copied tool-definition lists.
3. New worktrees may lack gitignored ONNX assets; setup must be disclosed and rebuilt/copied before the baseline run.
4. Modern E2E uses normative newline framing. Content-Length is a separate compatibility suite and cannot substitute for it.
5. Codex Resource compatibility remains supplementary. Direct modern protocol E2E cannot be marked blocked.
6. Any semantic deviation from the reviewed S01–S38 requires a visible Spec amendment before changing test assertions.

## Residual risks accepted

- Native production Host Skills interoperability may be `blocked_client_support`; server conformance is still mandatory through direct modern protocol fixtures.
- Existing absolute-path legacy `okf://` URIs remain technical debt.
- Modern clients receive only the 11 stateless Agent tools and no Prompts; this is intentional protocol correctness, not a missing phase.
- One-era-per-process is more restrictive than possible dual-era concurrency but avoids a broad state refactor outside this change.
- SHA-256 remains unsigned and non-authoritative.

## Approval gate

Implementation may start only after user approval of this revised dual-era Spec. Approval of the earlier `8d72343` content, if any, would not cover this materially revised contract.
