# Evidence — add-mcp-skills-extension (fresh run)

- Branch: `spec/mcp-skills-extension`
- Base Spec commit: `f99af86` (approved)
- Implementation commits: `6380226` (initial), `e4d51d5` (hard gap fixes), see `git log` for subsequent review fixes
- Evidence commit: this file is committed alongside the implementation; the tree state at verification time is recorded by `git rev-parse HEAD` and `git status --porcelain` (clean)
- Toolchain: go1.26.7 linux/amd64
- CLI version: okf 0.7.0 (library meta 0.4.1)
- Verification timestamp: 2026-09-16 (fresh after last code edit)

## Build / static checks
- `go build ./...`: PASS
- `go vet ./...`: PASS
- `gofmt -l` (tracked): clean
- `staticcheck ./...`: PASS (zero warnings)

## Unit tests
- `go test ./...`: all green
- `go test -race ./pkg/mcp/...`: clean (zero data races)
- `go test -shuffle=on ./pkg/mcp/...`: no order dependency
- `go test -coverpkg=./... ./pkg/mcp/...`: coverage 30.2% (mcp subset; full repo gauntlet coverage 69%)

## Modern protocol E2E (test_ext_skills.py)
- **8/8 tests passed**
- server/discover: resultType=complete, supportedVersions=[2026-07-28], capabilities Tools/Resources/skills, no Prompts
- unsupported version: -32022 with data.supported=[2026-07-28], data.requested=2024-11-05
- skills/list without capability: -32021 with data.requiredCapability=io.modelcontextprotocol/skills
- skills/list + skills/get: 1 Skill (skill://okf/SKILL.md), deep-equal, ttlMs=300000, cacheScope=private, serverInfo _meta
- resources/list + resources/read: 1 Resource, text/markdown, content contains W01-W07
- tools/list: exactly 11 modern tools, sorted, no legacy bundle-state tools
- unknown skill URI: -32602
- modern prompts/list: -32601 (not implemented in modern era)

## Legacy protocol E2E (test_mcp.py)
- **13/13 tests passed**
- 20 legacy tools, Prompts (2), ping, Resources all available
- No modern resultType/cache/_meta fields in legacy responses
- Additive Skill Resource in legacy resources/list (skill://okf/SKILL.md)
- Legacy resources/read for skill: URI, MIME, content all correct

## Real Codex Resource compatibility (S35)
- Reproducible harness: `tools/verify-mcp-skills-codex.sh` (builds OKF, configures isolated CODEX_HOME, runs real Codex, parses JSONL, outputs safe summary only, fail-closed)
- Codex version: 0.153.4
- Model channel: xeart (gpt-5.6-sol__dev via local proxy 127.0.0.1:18080)
- **Result: PASS**
  - MCP tool calls: `codex.list_mcp_resources` (completed), `okf.read_mcp_resource` (completed)
  - Resource name: okf
  - Contains W01: Yes
  - Contains W07: Yes
  - Mutating tools called (okf_note/init/refresh/feedback): 0
- Era label: modern 2026-07-28 Resource compatibility (NOT native skills/list/get)
- Raw event stream is NOT committed (may contain model-internal tokens); harness outputs only safe summary

## Dual-era behavior
- Modern opening (server/discover with _meta): selects modern era
- Legacy opening (initialize): selects legacy era, loads BundlePath
- Mixed-era: modern method after legacy initialize → rejected with deterministic error
- New process: either era selectable afresh
- BundlePath auto-load: only after legacy initialize; modern era zero bundle I/O for discovery/Skill

## Skill registry
- 1 Skill: skill://okf/SKILL.md, name=okf
- Digest: sha256:<64 hex>, matches served bytes (verified by test)
- Size: raw byte length
- URI validation: rejects wrong scheme/userinfo/port/query/fragment/dot/encoded traversal/missing SKILL.md/wrong host
- Defensive copies: List/Get/Resources/Read return copies; mutation doesn't affect registry
- Concurrent reads: race-clean (100 goroutines)
- Construction: fail-closed; NewServer returns error on invalid registry

## Benchmarks (S37, 10,000 ops each)
- Reproducible command: `go test ./pkg/mcp/ -bench="BenchmarkModern|BenchmarkSkill" -benchtime=10000x -run=^$ -benchmem`
- Test file: `pkg/mcp/skills_benchmark_test.go` (8 benchmarks)

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| ModernDiscover | ~11,000 | ~6,900 | ~64 |
| ModernToolsList | ~135,000 | ~67,700 | ~499 |
| ModernSkillsList | ~10,000 | ~4,700 | ~37 |
| ModernSkillsGet | ~10,000 | ~4,900 | ~43 |
| ModernResourcesRead | ~21,000 | ~12,700 | ~41 |
| SkillRegistryList | ~32 | 80 | 1 |
| SkillRegistryRead | ~19 | 0 | 0 |
| SkillRegistryNoBundleIO | ~1,400 | ~1,680 | ~5 |

- Zero knowledge-runtime I/O for discovery/Skill operations (registry built from static rendered bytes)
- SkillRegistryRead: 0 B/op, 0 allocs/op
- Numbers are representative; exact values depend on hardware

## Targeted mutants (S38, 6/6 killed)
| Mutant | Target | Killed by |
|---|---|---|
| MSK1 version-validation | skip protocol version check | TestModernMetaValidation |
| MSK2 capability-gate | skip skills capability check | TestModernSkillsRequireCapability |
| MSK3 digest-verify | tamper digest algorithm | TestSkillRegistryDigest |
| MSK4 completeness | tamper skill URI | TestNewSkillRegistry |
| MSK5 stateless-tool-filter | expose okf_load_bundle modernly | TestModernToolsListHas11Tools |
| MSK6 wrapper-leakage | leak OKF-MANAGED into skill | TestRenderAgentSkillNoOwnershipMarkers |

- Script: `tools/mutants-mcp-skills.sh`
- All mutants killed; files restored byte-for-byte after each

## Code review findings (post-implementation)
### Round 1 (explicit)
- HIGH: error responses missing `data` field for -32022/-32021 → FIXED (added sendErrorWithData, tests TestModernUnsupportedVersion/TestModernMissingCapabilityHasData)
- MEDIUM: type assertion instead of errors.As → acceptable (ParseModernMeta always returns *RPCError)

### Round 2 (implicit)
- MEDIUM: percent-encoded traversal defense-in-depth → already caught by Go url.Path decoding; added tests (encoded_dot/encoded_slash/encoded_backslash)
- Verified: BundlePath only loaded after legacy initialize; modern era zero bundle I/O
- Verified: registry concurrent safety; URI confinement; no path traversal

## Key implementation files
- `pkg/mcp/modern_protocol.go`: modern metadata types, errors (-32022/-32021), result envelope
- `pkg/mcp/modern_handlers.go`: server/discover, tools, resources, skills handlers
- `pkg/mcp/skills.go`: immutable SkillRegistry, URI validation, digest/size, defensive copies, `validateSkillManifest` (injectable for S14 boundary tests)
- `pkg/mcp/server.go`: dual-era dispatcher, deferred BundlePath load, shared serverInfo, sendErrorWithData, injectable `bundleLoader`
- `pkg/mcp/dual_era_test.go`: dual-era dispatch tests, error data tests (S01-S05, S26)
- `pkg/mcp/skills_test.go`: registry tests, URI validation (13 cases incl. encoded traversal), defensive copies, concurrent reads, S14 manifest boundary tests (512/513, 16777216/over, overflow, duplicate)
- `pkg/mcp/skills_benchmark_test.go`: S37 benchmarks (8 benchmarks, 10000 ops)
- `pkg/mcp/bundle_load_test.go`: S16/S24 loader spy tests (NewServer/modern no-load, legacy load-once, unknown/traversal URI no-fallback)
- `pkg/mcp/redaction_test.go`: S33 error/log redaction tests (token/env/home/skill-body canaries)
- `pkg/mcp/docs_contract_test.go`: S30-S32 documentation contract tests
- `pkg/agentconfig/workflow.go`: RenderAgentSkill() portable renderer
- `pkg/agentconfig/skill_test.go`: renderer tests
- `cmd/okf/main.go`: NewServer error propagation
- `docs/knowledge/mcp-server.md`: dual-era, Skill, security boundary documentation
- `README.md`: dual-era and Skill section
- `openspec/changes/add-mcp-skills-extension/release-notes.md`: release notes
- `test_ext_skills.py`: modern protocol E2E (8/8)
- `tools/mutants-mcp-skills.sh`: targeted mutation runner (6/6 killed)
- `tools/verify-mcp-skills-codex.sh`: real Codex Resource compatibility harness (reproducible, fail-closed, safe summary)
