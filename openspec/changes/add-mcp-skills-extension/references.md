# References and Decision Record

## Pinned inputs

- Skills Over MCP incubation repository: `https://github.com/modelcontextprotocol/ext-skills`
- Reviewed commit: `d866efdba298b55b8156c7b7aa1bdebc1b625f4c`
- Stable synced specification: `specification/stable/skills.mdx`
- Threat model: `docs/threat-model.md`
- Canonical SEP: `https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2640`
- Agent Skills format: `https://agentskills.io/specification`

## Normative protocol facts adopted

- Extension identifier is `io.modelcontextprotocol/skills`.
- It targets base protocol revision `2026-07-28` or later; OKF only claims the exact revision it implements.
- Declaring the extension requires Resources plus `skills/list` and `skills/get`.
- `resources/directory/read` is optional and is not declared by OKF.
- Skill files are read through standard `resources/read`.
- A static resources array is complete and carries URI, lowercase SHA-256 and raw byte length for every file.
- Limits are 512 resources and 16 MiB total per Skill.
- `skills/list` and `skills/get` carry required CacheableResult fields.
- Unknown Skill and Resource URIs use `-32602`.
- Resource read by itself is not Skill activation.
- Digests prove consistency, not trust.

## OKF-specific decisions

| Decision | Selected | Rejected alternative | Reason |
|---|---|---|---|
| Initial catalog | one static Skill | generic plugin catalog | no second Skill requires abstraction |
| Skill URI | `skill://okf/SKILL.md` | absolute/project path URI | stable, project-independent, no host path leak |
| Workflow source | W01–W07 canonical clauses | separate MCP Skill text | prevents drift |
| Metadata source | parse final Skill bytes | duplicate Go constants | prevents frontmatter drift |
| Cache scope | private | public | safer for project/server-associated context |
| Directory read | false/not declared | implement now | no supporting directories in v1 |
| Dynamic Skills | unsupported | `resources: dynamic` | loses content-bound integrity and is unnecessary |
| Legacy discovery | append ordinary Skill Resource | hide from legacy clients | current Codex can use Resource compatibility |
| Existing `okf://` URIs | preserve in this change | rewrite now | independent compatibility and persistence risk |
| Version negotiation | explicit revision table | lexical date compare | avoids claiming unimplemented future schemas |
| Invalid built-in Skill | startup error | panic or partial catalog | testable fail-closed behavior |

## PoC evidence retained outside completion evidence

The isolated PoC verified:

- one Skill, 1327 bytes at the time of the run;
- `skills/list/get` and Resource bytes/digest parity;
- newline and Content-Length stdio flows;
- full repository build/vet/test;
- changed-package race and shuffled tests;
- existing MCP E2E 13/13 with 20 tools;
- real Codex Resource list/read and W01/W07 recognition without a mutating tool.

These measurements are not release evidence because the formal implementation may differ and all final numbers must come from a fresh run after the last edit.
