# Evidence — add-mcp-skills-extension (fresh run)

- Branch: `spec/mcp-skills-extension`
- Toolchain: go1.26.7 linux/amd64
- CLI version: okf 0.7.0 (library 0.4.1)
- Verification timestamp: fresh run after implementation

## Build / static checks
- `go build ./...`: PASS
- `go vet ./...`: PASS
- `gofmt -l` (tracked): clean
- `staticcheck ./...`: PASS (gauntlet L2)

## Unit tests
- `go test ./...`: all green
- `go test -race ./pkg/mcp/...`: clean
- `go test -shuffle=on ./...`: no order dependency

## Modern protocol E2E (test_ext_skills.py)
- 8/8 tests passed
- server/discover: resultType=complete, supportedVersions=[2026-07-28], capabilities Tools/Resources/skills, no Prompts
- unsupported version: -32022 with supported/requested data
- skills/list without capability: -32021
- skills/list + skills/get: 1 Skill (skill://okf/SKILL.md), deep-equal, ttlMs=300000, cacheScope=private
- resources/list + resources/read: 1 Resource, text/markdown, content contains W01-W07
- tools/list: exactly 11 modern tools, sorted, no legacy bundle-state tools
- unknown skill URI: -32602
- modern prompts/list: -32601 (not implemented in modern era)

## Legacy protocol E2E (test_mcp.py)
- 13/13 tests passed
- 20 legacy tools, Prompts, ping, Resources all available
- No modern resultType/cache/_meta fields in legacy responses
- Additive Skill Resource in legacy resources/list

## Dual-era behavior
- Modern opening (server/discover with _meta): selects modern era
- Legacy opening (initialize): selects legacy era, loads BundlePath
- Mixed-era: modern method after legacy initialize → rejected
- New process: either era selectable afresh

## Skill registry
- 1 Skill: skill://okf/SKILL.md, name=okf
- Digest: sha256:<64 hex>, matches served bytes
- Size: raw byte length
- URI validation: rejects wrong scheme/userinfo/port/query/fragment/dot/missing SKILL.md/wrong host
- Defensive copies: List/Get/Resources/Read return copies; mutation doesn't affect registry
- Concurrent reads: race-clean

## Key implementation files
- `pkg/mcp/modern_protocol.go`: modern metadata types, errors (-32022/-32021), result envelope
- `pkg/mcp/modern_handlers.go`: server/discover, tools, resources, skills handlers
- `pkg/mcp/skills.go`: immutable SkillRegistry, URI validation, digest/size
- `pkg/mcp/server.go`: dual-era dispatcher, deferred BundlePath load, shared serverInfo
- `pkg/agentconfig/workflow.go`: RenderAgentSkill() portable renderer
- `cmd/okf/main.go`: NewServer error propagation
