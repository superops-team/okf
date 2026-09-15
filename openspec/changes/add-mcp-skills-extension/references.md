# References and Decision Record

## Pinned extension inputs

- Skills Over MCP incubation repository: `https://github.com/modelcontextprotocol/ext-skills`
- Reviewed extension commit: `d866efdba298b55b8156c7b7aa1bdebc1b625f4c`
- Synced Skills specification: `specification/stable/skills.mdx`
- Threat model: `docs/threat-model.md`
- Canonical SEP: `https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2640`
- Agent Skills format: `https://agentskills.io/specification`

## Official modern base-protocol inputs

- `https://modelcontextprotocol.io/specification/2026-07-28/changelog`
- `https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning`
- `https://modelcontextprotocol.io/specification/2026-07-28/server/discover`
- `https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/stdio`
- `https://modelcontextprotocol.io/specification/2026-07-28/server/tools`
- `https://modelcontextprotocol.io/specification/2026-07-28/server/resources`
- `https://modelcontextprotocol.io/specification/2026-07-28/server/prompts`

## Protocol facts adopted after review

- Modern `2026-07-28` is stateless: no initialize handshake; version and client capabilities are carried per request in `_meta`.
- Modern servers implement `server/discover`; clients may also call an ordinary method directly with modern metadata.
- `server/discover.supportedVersions` and `-32022.data.supported` list modern per-request versions only; OKF legacy `2024-11-05` support is reached through initialize fallback and is not advertised as valid modern `_meta`.
- Unsupported modern versions return `-32022` with requested/supported data.
- Required client capability failures use `-32021`.
- Every modern successful result carries `resultType`; server identity belongs in result `_meta`.
- `tools/list`, `prompts/list`, `resources/list`, `resources/read` and discovery are cacheable in the modern base protocol.
- Modern stdio framing is one newline-delimited JSON-RPC message per line. Content-Length is retained only as an OKF compatibility extension.
- Legacy and modern eras may coexist in one server entry point; OKF chooses one era per stdio process to isolate existing mutable legacy bundle state.
- The Skills extension identifier is `io.modelcontextprotocol/skills`; declaring it requires Resources plus `skills/list/get`.
- Skill files are read through `resources/read`; Resource read alone is not activation.
- Static Resources form a complete digest/size manifest; limits are 512 entries and 16 MiB.
- Digests prove consistency, not trust.

## OKF-specific decisions

| Decision | Selected | Rejected alternative | Reason |
|---|---|---|---|
| Protocol architecture | one-era-per-process dual-era server | modern fields on initialize | modern base protocol removed initialize/session semantics |
| Modern catalog | 11 Service-backed Agent tools | all 20 legacy tools | nine tools depend on implicit mutable bundle selection |
| Modern Prompts | omitted | expose existing Prompts | existing Prompts call legacy bundle-state tools |
| Modern Resources | static Skill only | expose mutable legacy bundle Resources | avoids connection-state-dependent catalog in stateless era |
| Legacy catalog | unchanged 20 tools + Prompts + prior Resources | migrate legacy behavior | backward compatibility |
| BundlePath loading | deferred until legacy initialize | constructor auto-load | modern discovery/Skill paths must not read knowledge storage |
| Initial Skill catalog | one static Skill | generic plugin catalog | no second Skill requires abstraction |
| Skill URI | `skill://okf/SKILL.md` | project/absolute path URI | stable and path-private |
| Workflow source | W01–W07 canonical clauses | separate MCP text | prevents drift |
| Metadata source | final bytes | duplicate constants | prevents frontmatter drift |
| Cache scope | private | public | project/server-associated context |
| Directory/dynamic/nested | unsupported | implement now | no first-release value; larger security surface |
| Legacy Skill discovery | append ordinary Resource | hide from legacy clients | current Hosts can use Resource compatibility |
| Existing `okf://` URI migration | defer | rewrite in this change | separate persistence/compatibility risk |
| Server version | existing shared MCP/library version source | handler literals per era | prevents legacy/modern drift |
| Invalid built-in Skill | startup error | panic/partial catalog | fail-closed and testable |

## Superseded PoC assumptions

The PoC demonstrated Registry and Resource feasibility, both historical OKF framing modes and real Codex Resource use. The following PoC assumptions are explicitly rejected after review:

- using `initialize` to negotiate `2026-07-28`;
- storing a modern protocol profile as session state;
- exposing all 20 tools under modern semantics;
- advertising existing Prompts in the modern catalog;
- treating Content-Length as normative modern stdio framing.

All release evidence must come from the reviewed implementation, not the superseded PoC.
