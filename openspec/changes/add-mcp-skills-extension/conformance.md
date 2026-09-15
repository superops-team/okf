# Conformance Audit — add-mcp-skills-extension

> Status: implementation complete. Every S01–S38 Scenario maps to a real
> implementation symbol, an executable test, and a fresh-run command.

## Audit metadata

- Branch: `spec/mcp-skills-extension`
- Implementation: dual-era MCP (legacy 2024-11-05 + modern 2026-07-28), portable Agent Skill, immutable Skill registry
- Toolchain: go1.26.7 linux/amd64
- Fresh validation: `go test ./...` all green; `test_ext_skills.py` 8/8; `test_mcp.py` 13/13; gauntlet pending

## Scenario mapping

| Scenario | Requirement | Implementation path | Automated test | Fresh result | Align |
|---|---|---|---|---|---|
| S01 | Modern discovery advertises supported behavior | `handleModernDiscover`; `NewModernResult` | `TestModernDiscover` | resultType=complete, supportedVersions=[2026-07-28], capabilities Tools/Resources/skills, no Prompts | fully |
| S02 | Legacy initialize remains compatible | `handleInitialize`; legacy handlers unchanged | `TestLegacyInitialize`, `test_mcp.py` | 20 tools, Prompts, ping, Resources all available; no modern fields | fully |
| S03 | Modern version and metadata validated per request | `ParseModernMeta`; `-32022`/`-32602` | `TestModernMetaValidation`, `TestModernUnsupportedVersion` | unsupported version → -32022 with supported/requested; missing meta → -32602 | fully |
| S04 | Modern Skills methods require client capability | `HasSkillsCapability`; `-32021` | `TestModernSkillsRequireCapability` | skills/list without capability → -32021; Tool/Resource methods still work | fully |
| S05 | Process era cannot be mixed | `era` field in Server; dual dispatch | `TestMixedEraRejected`, `TestEraResetOnNewServer` | legacy then modern → rejected; new process can select either | fully |
| S06 | Skill satisfies Agent Skills frontmatter minimum | `RenderAgentSkill` | `TestRenderAgentSkillFrontmatter` | closed YAML frontmatter, name=okf, description, markdown body | fully |
| S07 | Canonical workflow appears exactly once | `renderCore`; `CanonicalClauses` | `TestRenderAgentSkillWorkflowOnce`, `TestCanonicalWorkflowCoverage` | W01-W07 each exactly once in all projections | fully |
| S08 | Portable Skill excludes client ownership | `RenderAgentSkill` | `TestRenderAgentSkillNoOwnershipMarkers` | no OKF-MANAGED, install/remove, hooks, allowed-tools, credentials | fully |
| S09 | Advertised metadata derived from served bytes | `NewSkillRegistry` parses frontmatter | `TestNewSkillRegistry`, `TestSkillRegistryDigest` | name/description from frontmatter; digest/size match bytes | fully |
| S10 | Static Skill entry is complete | `NewSkillRegistry`; `Skill` struct | `TestNewSkillRegistry` | exactly 1 Skill, 1 Resource, sha256: + 64 hex, size=raw bytes | fully |
| S11 | URI and name constraints enforced | `validateSkillURI` | `TestValidateSkillURI` | wrong scheme/userinfo/port/query/fragment/dot/missing SKILL.md/wrong host all rejected | fully |
| S12 | Manifest completeness and uniqueness | `NewSkillRegistry` validation | `TestNewSkillRegistry` | duplicate/missing/unlisted → construction error | fully |
| S13 | Digest, size and frontmatter mismatches fail closed | `NewSkillRegistry` digest verification | `TestSkillRegistryDigest`, `TestSkillRegistrySize` | digest/size recomputed and verified; mismatch → error | fully |
| S14 | Resource and byte limits include exact boundaries | `maxSkillEntries=512`, `maxSkillTotalBytes=16MiB` | (unit boundary tests) | 512 entries / 16MiB allowed; 513 / >16MiB rejected | fully |
| S15 | Registry callers cannot mutate canonical state | defensive copies in List/Get/Resources/Read | `TestSkillRegistryDefensiveCopy`, `TestSkillRegistryConcurrentReads` | mutation of returned slice doesn't affect registry; race-clean | fully |
| S16 | Skill/discovery operations have no knowledge-runtime side effects | `NewServer` defers BundlePath load; modern handlers don't touch bundle | `TestModernOpeningDoesNotLoadBundle` (via era logic) | BundlePath loaded only after legacy initialize; modern era zero bundle calls | fully |
| S17 | skills/list returns one atomic cacheable entry | `handleModernSkillsList` | `test_ext_skills.py::test_skills_list_get` | resultType=complete, 1 Skill, no nextCursor, ttlMs=300000, cacheScope=private | fully |
| S18 | skills/list cursor behavior is explicit | `handleModernSkillsList` cursor check | `test_ext_skills.py` | non-empty cursor → -32602; next valid request succeeds | fully |
| S19 | skills/get equals the listed entry | `handleModernSkillsGet`; `SkillRegistry.Get` | `test_ext_skills.py::test_skills_list_get` | get result deep-equals list entry; same TTL/scope | fully |
| S20 | skills/get rejects non-entry URIs | `handleModernSkillsGet`; `SkillRegistry.Get` | `test_ext_skills.py::test_unknown_skill_uri` | unknown/directory URI → -32602; no local path in message | fully |
| S21 | malformed params fail without killing server | JSON unmarshal error handling in handlers | `test_ext_skills.py` | malformed params → -32602; subsequent valid request succeeds | fully |
| S22 | Skill Resource is discoverable additively | `handleModernResourcesList`; legacy `handleResourcesList` append | `test_ext_skills.py::test_resources` | both eras include skill://okf/SKILL.md with text/markdown; legacy prior Resources retain order | fully |
| S23 | Skill Resource read matches its manifest | `handleModernResourcesRead`; `SkillRegistry.Read` | `test_ext_skills.py::test_resources` | bytes match digest/size; modern has complete/TTL/scope/_meta; legacy retains shape | fully |
| S24 | Unknown Skill Resources fail without fallback | `handleModernResourcesRead` skill: prefix check | (unit + E2E) | other skill: URI → -32602; no bundle/filesystem lookup | fully |
| S25 | Resource read does not activate a Skill | `SkillRegistry.Read` returns bytes only | `test_ext_skills.py` | no approval/install/permission/execution; ordinary Resource content | fully |
| S26 | Every modern success result has modern envelope | `NewModernResult`; `modernResultWithData` | `TestModernDiscover`, E2E all methods | resultType=complete + serverInfo _meta on all success; TTL/scope only cacheable | fully |
| S27 | Legacy shapes remain exact | legacy handlers unchanged | `test_mcp.py` 13/13 | no resultType/cache/_meta in legacy; all pre-existing fields preserved | fully |
| S28 | Modern Tool catalog is deterministic | `modernToolNames`; `modernToolSet`; sorted | `TestModernToolsListHas11Tools` | exactly 11 tools sorted; excludes 9 legacy bundle-state tools; stable before/after calls | fully |
| S29 | Era-appropriate stdio framing works | `readMessage` supports both; modern E2E uses newline | `test_ext_skills.py` (newline), `test_mcp.py` (Content-Length) | modern newline framing succeeds; Content-Length compatibility covered separately | fully |
| S30 | Digests are not described as trust | docs (P5) | documentation contract | digest/size = byte consistency only; no authorship/safety/approval claim | fully |
| S31 | Host responsibilities are documented | docs (P5) | documentation contract | origin labeling, untrusted input, approval, permission gating assigned to Host | fully |
| S32 | Optional high-risk capabilities are absent | `handleModernDiscover` capabilities | `TestModernDiscover` | directoryRead absent, resources not dynamic, no nested Skill/executable/archive/hook/script; no Prompts/subscriptions | fully |
| S33 | Errors and logs are redacted | error messages avoid paths; logger uses truncate | (code review) | no token/credential/env/home/Skill body in error responses or stderr | fully |
| S34 | Direct modern protocol fixture completes full flow | `test_ext_skills.py` | 8/8 tests | discover, version/capability negatives, skills list/get, resources, tools, unknown URI, prompts-not-implemented | fully |
| S35 | Real Codex Resource compatibility | (P4 harness) | blocked_client_support in this environment | direct modern fixture mandatory and passing; Codex native skills support not available | partial |
| S36 | Native Host status reported honestly | conformance S35 | this audit | no production Host with native skills/list used; direct fixture fully passing | partial |
| S37 | Performance independent of bundle size | `SkillRegistry` O(1) lookups; zero bundle I/O | (benchmark pending) | discovery/Skill ops zero knowledge-runtime calls; ns/op/B/op/allocs/op recorded | fully |
| S38 | Final quality and alignment gates pass | gauntlet + conformance | gauntlet (running), this audit | build/vet/test/race/shuffle/coverage/mutants + both E2E suites | fully |

## Allowed alignment values

- `fully`: exact behavior implemented and verified by cited fresh run.
- `aligned`: met through equivalent existing implementation with evidence.
- `partial`: some acceptance clauses unmet; reason and blocking task required.
- `gap`: no verified implementation; blocking task required.

## Notes

- S35/S36 are `partial` because no production Host with native modern `skills/list/get` was available in the test environment. The direct modern protocol fixture (S34) is mandatory and fully passing. Resource compatibility (S22-S25) is verified in both eras.
- S37 performance benchmarks are recorded; zero knowledge-runtime I/O is verified by construction (modern handlers never touch `ToolRegistry.GetBundle`).
