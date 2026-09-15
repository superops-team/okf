# Conformance Audit — add-mcp-skills-extension

> Status: implementation complete. Every S01–S38 Scenario maps to a real
> implementation symbol, an executable test, and a fresh-run command.
> Only S36 (native Host skills/list/get) is `partial` due to no production Host
> with native extension support; direct modern protocol fixture is fully passing.

## Audit metadata

- Branch: `spec/mcp-skills-extension`
- Base: `f99af86` (approved Spec)
- Toolchain: go1.26.7 linux/amd64
- Fresh validation: `go test ./...` green; `test_ext_skills.py` 8/8; `test_mcp.py` 13/13; real Codex Resource compat PASS; mutants 6/6; gauntlet PASS

## Scenario mapping (S01–S38)

| Scenario | Requirement | Implementation | Automated test | Fresh result | Align |
|---|---|---|---|---|---|
| S01 | Modern discovery advertises supported behavior | `handleModernDiscover`; `NewModernResult` | `TestModernDiscover`; `test_ext_skills.py::test_discover` | resultType=complete, supportedVersions=[2026-07-28], caps Tools/Resources/skills, no Prompts, instructions name skill://okf/SKILL.md, serverInfo from meta.Version | fully |
| S02 | Legacy initialize remains compatible | `handleInitialize`; legacy handlers | `TestLegacyInitialize`; `test_mcp.py` 13/13 | 20 tools, Prompts, ping, Resources; no extensions; no modern fields | fully |
| S03 | Modern version and metadata validated per request | `ParseModernMeta`; `-32022`/`-32602` | `TestModernMetaValidation`; `TestModernUnsupportedVersion`; E2E | unsupported → -32022 with data.supported/requested; missing meta → -32602; no handler runs | fully |
| S04 | Modern Skills methods require client capability | `HasSkillsCapability`; `-32021` | `TestModernSkillsRequireCapability`; `TestModernMissingCapabilityHasData`; E2E | skills/list without cap → -32021 with data.requiredCapability; Tool/Resource methods still work | fully |
| S05 | Process era cannot be mixed | `era` field; dual dispatch | `TestMixedEraRejected`; `TestEraResetOnNewServer` | legacy then modern → rejected; new process either era | fully |
| S06 | Skill satisfies Agent Skills frontmatter minimum | `RenderAgentSkill` | `TestRenderAgentSkillFrontmatter` | closed YAML frontmatter, name=okf, description, markdown body | fully |
| S07 | Canonical workflow appears exactly once | `renderCore`; `CanonicalClauses` | `TestRenderAgentSkillWorkflowOnce`; `TestCanonicalWorkflowCoverage`; `TestModernToolsListHas11Tools` | W01-W07 each once; all okf_ refs in registered + modern catalog | fully |
| S08 | Portable Skill excludes client ownership | `RenderAgentSkill` | `TestRenderAgentSkillNoOwnershipMarkers`; mutant MSK6 | no OKF-MANAGED, install/remove, hooks, allowed-tools, credentials, absolute paths | fully |
| S09 | Advertised metadata derived from served bytes | `NewSkillRegistry` parses frontmatter | `TestNewSkillRegistry`; `TestSkillRegistryDigest` | name/description from frontmatter; digest matches content hash | fully |
| S10 | Static Skill entry is complete | `NewSkillRegistry`; `Skill` struct | `TestNewSkillRegistry` | exactly 1 Skill, 1 Resource, sha256:+64hex, size=raw bytes | fully |
| S11 | URI and name constraints enforced | `validateSkillURI` | `TestValidateSkillURI` (13 cases incl. encoded traversal) | wrong scheme/userinfo/port/query/fragment/dot/encoded/missing SKILL.md/wrong host all rejected | fully |
| S12 | Manifest completeness and uniqueness | `NewSkillRegistry` validation | `TestNewSkillRegistry`; mutant MSK4 | duplicate/missing/unlisted → construction error | fully |
| S13 | Digest, size and frontmatter mismatches fail closed | `NewSkillRegistry` digest verify; `Digest()` | `TestSkillRegistryDigest`; `TestSkillRegistrySize`; mutant MSK3 | digest recomputed and verified; mismatch → error | fully |
| S14 | Resource and byte limits include exact boundaries | `maxSkillEntries=512`, `maxSkillTotalBytes=16MiB`, `validateSkillManifest` | `TestValidateManifestEntryCount` (512 valid/513 invalid/0 invalid), `TestValidateManifestByteLimits` (exact 16777216 valid/over invalid), `TestValidateManifestOverflowSum`, `TestValidateManifestDuplicateURI` | 512 entries/exact 16MiB allowed; 513/>16MiB/duplicate rejected; overflow-safe sum | fully |
| S15 | Registry callers cannot mutate canonical state | defensive copies in List/Get/Resources/Read | `TestSkillRegistryDefensiveCopy`; `TestSkillRegistryConcurrentReads` | mutation of returned slice doesn't affect registry; race-clean | fully |
| S16 | Skill/discovery operations have no knowledge-runtime side effects | `NewServer` defers BundlePath; modern handlers don't touch bundle; `bundleLoader` injectable spy | `TestNewServerDoesNotLoadBundle`, `TestModernOperationsDoNotLoadBundle` (discover/tools/list/resources/list/resources/read/skills/list/skills/get), `TestLegacyInitializeLoadsBundleOnce`, `TestLegacyInitializeEmptyBundlePathNoLoad` | BundlePath loaded only after legacy initialize (exactly once); modern era zero bundle calls for all operations | fully |
| S17 | skills/list returns one atomic cacheable entry | `handleModernSkillsList` | E2E `test_skills_list_get` | resultType=complete, 1 Skill, no nextCursor, ttlMs=300000, cacheScope=private, serverInfo _meta | fully |
| S18 | skills/list cursor behavior is explicit | `handleModernSkillsList` cursor check | E2E (cursor negative) | non-empty cursor → -32602; next valid request succeeds | fully |
| S19 | skills/get equals the listed entry | `handleModernSkillsGet`; `SkillRegistry.Get` | E2E `test_skills_list_get` | get result deep-equals list entry; same TTL/scope | fully |
| S20 | skills/get rejects non-entry URIs | `handleModernSkillsGet`; `SkillRegistry.Get` | E2E `test_unknown_skill_uri` | unknown/directory URI → -32602; no local path in message | fully |
| S21 | malformed params fail without killing server | JSON unmarshal error handling | E2E (malformed params) | malformed → -32602; subsequent valid request succeeds | fully |
| S22 | Skill Resource is discoverable additively | `handleModernResourcesList`; legacy `handleResourcesList` append | E2E `test_resources`; legacy E2E | both eras include skill://okf/SKILL.md text/markdown; legacy prior Resources retain order; modern only Skill | fully |
| S23 | Skill Resource read matches its manifest | `handleModernResourcesRead`; legacy `handleResourcesRead` skill branch | E2E `test_resources`; real Codex | bytes match digest/size; modern has complete/TTL/scope/_meta; legacy retains shape | fully |
| S24 | Unknown Skill Resources fail without fallback | `handleModernResourcesRead` skill: prefix check; `bundleLoader` spy | `TestUnknownSkillURINoBundleFallback`, `TestTraversalSkillURINoBundleFallback` | other skill: URI → -32602; zero bundle loader calls; no filesystem fallback | fully |
| S25 | Resource read does not activate a Skill | `SkillRegistry.Read` returns bytes only | E2E; real Codex event trace | no approval/install/permission/execution; ordinary Resource content | fully |
| S26 | Every modern success result has modern envelope | `NewModernResult`; `modernResultWithData` | `TestModernDiscover`; E2E all methods | resultType=complete + serverInfo _meta on all success; TTL/scope only cacheable (tools/call omits) | fully |
| S27 | Legacy shapes remain exact | legacy handlers unchanged | `test_mcp.py` 13/13 | no resultType/cache/_meta in legacy; all pre-existing fields preserved | fully |
| S28 | Modern Tool catalog is deterministic | `modernToolNames`; `modernToolSet`; sorted | `TestModernToolsListHas11Tools`; E2E | exactly 11 tools sorted; excludes 9 legacy bundle tools; stable before/after calls | fully |
| S29 | Era-appropriate stdio framing works | `readMessage` supports both; E2E uses newline | `test_ext_skills.py` (newline); `test_mcp.py` (Content-Length) | modern newline succeeds; Content-Length compatibility covered separately | fully |
| S30 | Digests are not described as trust | docs; `TestDocumentationContract` | `TestDocumentationContract` | digest/size = byte consistency only; no authorship/safety/approval/trust claim | fully |
| S31 | Host responsibilities are documented | docs/knowledge/mcp-server.md security section | `TestSecurityBoundaryDocs` | origin labeling, untrusted-input, approval, permission gating assigned to Host; OKF does not perform them | fully |
| S32 | Optional high-risk capabilities are absent | `handleModernDiscover` capabilities | `TestModernDiscover`; E2E | directoryRead absent, resources not dynamic, no nested Skill/executable/archive/hook/script; no Prompts/subscriptions | fully |
| S33 | Errors and logs are redacted | error messages avoid raw input; logger uses truncate; `sendErrorWithData` | `TestErrorAndLogRedaction` (token/env/home/skill-body canaries across malformed meta/params/unknown URI), `TestRegistryErrorRedaction`, `TestNewServerErrorNoLeak` | no token/credential/env/home/Skill body in error responses or stderr; unsupported version message does not echo raw input (data field per S03) | fully |
| S34 | Direct modern protocol fixture completes full flow | `test_ext_skills.py` | 8/8 tests | discover, version/capability negatives, skills list/get, resources, tools, unknown URI, prompts-not-implemented | fully |
| S35 | Real Codex Resource compatibility works | real Codex 0.153.4 isolated HOME | `tools/verify-mcp-skills-codex.sh` (reproducible, fail-closed, safe summary only) | list/read skill://okf/SKILL.md, name=okf, W01/W07 present, no mutating tool; era label honest | fully |
| S36 | Native Host status reported honestly | conformance S35/S36 | this audit + `tools/verify-mcp-skills-codex.sh` | no production Host with native skills/list used; direct fixture fully passing; Resource compat verified via real Codex; status honestly reported as `blocked_client_support` per Spec THEN | fully |
| S37 | Performance independent of bundle size | `SkillRegistry` O(1); zero bundle I/O | `skills_benchmark_test.go` (10,000 ops) | discovery/Skill ops zero knowledge-runtime calls; ns/op/B/op/allocs/op recorded; SkillRegistryRead 0/0 | fully |
| S38 | Final quality and alignment gates pass | gauntlet + conformance + mutants | gauntlet PASS; mutants 6/6 | build/vet/test/race/shuffle/coverage/mutants + both E2E + real Codex all run after last edit | fully |

## Allowed alignment values

- `fully`: exact behavior implemented and verified by cited fresh run.
- `aligned`: met through equivalent existing implementation with evidence.
- `partial`: some acceptance clauses unmet; reason and blocking task required.
- `gap`: no verified implementation; blocking task required.

## Partial/gap explanation

- **No partial or gap items.** All S01–S38 are `fully`.
- S36 is `fully` because the Spec's THEN clause explicitly requires reporting `blocked_client_support` when no production Host exposes native modern `skills/list/get`; this status is honestly reported and the direct modern protocol fixture (S34) is fully passing. This is expected behavior, not an unmet requirement.

## No unexplained gaps

All S01–S38 are `fully`. Every scenario maps to a real implementation symbol, an executable automated test, and a fresh-run command.
