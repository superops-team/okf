# Code Review: Reflective Retrieval and Memory Defense

## Round 1 — Obvious issues

| # | Severity | File:Line | Problem | Fix | Verified |
|---|---|---|---|---|---|
| R1-1 | Medium | pkg/reflect/reflect.go:130-145 | O(n²) round-assignment loop; `_ = i` dead code | Replaced linear scans with `round1Set`/`round2Set` map lookups | tests pass |
| R1-2 | High | pkg/trapeval/trap.go:144-149 | Summarize counted EVERY case as poison (poisonTotal++ for all), making poison_blocked ratio meaningless | Added `IsPoison bool` to CaseScore; only count IsPoison cases; default ratio=1.0 when no poison cases | TestPoisonBlockedRatio added |
| R1-3 | Low | pkg/trapeval/trap_test.go:87 | `var _ = json.Marshal` dead reference after removing import | Removed | build clean |

## Round 2 — Boundary, resource, compatibility, security, maintainability

| # | Severity | File:Line | Observation | Decision |
|---|---|---|---|---|
| R2-1 | Low | pkg/memorydefense/screen.go:64-94 | Sequential detectors: after detector N redacts, detector N+1 scans the redacted string. A secret spanning a redaction boundary is unlikely and would already be partially redacted. | Acceptable — no fix needed |
| R2-2 | Info | pkg/relationrecall/recall.go:54 | Outgoing extends scan is O(n) per call. For 10k concepts this is 10k map lookups — acceptable for local-first CLI/MCP usage. | Acceptable — benchmark confirms (see evidence.md) |
| R2-3 | Info | pkg/tool/reflect.go:66-76 | relationFn called per alive anchor in round 2; each call does O(n) scan. For 10 alive anchors × 10k entries = 100k iterations. | Acceptable for local-first |
| R2-4 | Security | pkg/memorydefense/screen.go:80 | ErrBlocked.Error() includes only detector ID, never the matched secret text. | Correct — no leak |
| R2-5 | Compatibility | pkg/memorydefense/policy.go:23 | Default Policy{Enabled:false, Action:"redact"}. Missing config file returns disabled policy, not error. | Correct — backward compatible |
| R2-6 | Resource | pkg/memorydefense/screen.go:83 | strings.Builder allocated per detector per write. No goroutines, no file handles, no network. | Clean — no leaks |
| R2-7 | Maintainability | pkg/reflect/reflect.go | Run() takes 3 function params (queryFn, relationFn, trapGate). This is dependency injection for testability; acceptable for a pure function. | Acceptable |
| R2-8 | Edge | pkg/relationrecall/recall.go:49 | Anchor itself is only added if approved. If anchor is proposed, no self-hit but neighbors still returned. | Correct — proposed anchor should not appear as self |
| R2-9 | Edge | pkg/reflect/reflect.go:148 | NeedClarify triggers when evidence < min_evidence AFTER poison gate. If poison gate drops half the results, abstention is more aggressive. | Correct — safety-first |

## Conclusion
- Round 1: 3 issues found and fixed.
- Round 2: 9 observations, 0 require code changes (all acceptable by design).

## Round 3 — Final hardening (document import defense, E2E, test expansion)

Scope: handleImportDocument Defense wiring, test_mcp.py expansion, Codex E2E harness, import_defense_test.go.

| # | Severity | File:Line | Observation | Decision |
|---|---|---|---|---|
| R3-1 | Security | pkg/mcp/tools.go handleImportDocument | Screen runs on res.Markdown before WrapConcept+atomicWrite. No path traversal: out is filepath.Join(bundlePath, basename(path)) — basename strips directories. | Safe |
| R3-2 | Security | pkg/mcp/tools.go | Block error returns fmt.Sprintf("memory_defense blocked: %v", screenErr) — screenErr is *ErrBlocked which only contains DetectorID, never the secret text. | No leak |
| R3-3 | Resource | pkg/mcp/tools.go | No temp files created; atomicWriteFiles writes directly to bundle path. Temp fixtures in tests use t.TempDir(). | Clean |
| R3-4 | Compatibility | pkg/mcp/tools.go | LoadPolicy reads .okf/config.yaml from filepath.Dir(bundlePath). Missing file → disabled policy → byte-for-byte identical behavior. | Backward compatible |
| R3-5 | Error handling | pkg/mcp/tools.go | Config parse error returns immediately; screen block error returns immediately. No partial writes. | Correct |
| R3-6 | Maintainability | pkg/mcp/tools.go | importDefenseNotes field added but not yet surfaced in response. Minor: should include in sb output. | Fixed below |
| R3-7 | Test quality | pkg/mcp/import_defense_test.go | 4 tests cover disabled/block/redact/clean. Block test verifies no file on disk + no secret in error. | Good |

### R3-6 fix: surface defense notes in import response
The importDefenseNotes field was collected but never printed. Let me surface it.

## Round 4 — okf add Defense wiring (obvious issues)

Scope: cmd_add.go screenImportTree, cmd_add_defense_test.go.

| # | Severity | File:Line | Observation | Decision |
|---|---|---|---|---|
| R4-1 | Medium | cmd_add.go screenImportTree | Block mode aborts on first high-severity hit; files screened before the bad one are already redacted in staging. Since staging is cleaned by defer, no kb contamination. | Correct — staging isolation |
| R4-2 | Low | cmd_add.go | LoadPolicy(kbDir) looks for kbDir/.okf/config.yaml. But kbDir is the knowledge dir, not repo root. The .okf dir is typically at repo root, not inside kb. | Fixed below |
| R4-3 | Low | cmd_add.go | Non-.md files in staging (e.g. images copied from archive) are not screened. This is acceptable: Defense targets text content, not binaries. | Acceptable |

### R4-2 fix: LoadPolicy path
LoadPolicy should read from the repo root (parent of kbDir if kbDir ends in knowledge, or kbDir itself). Let me fix.


## Review A (independent round) — Obvious issues

Reviewed files: screen.go, recall.go, reflect.go, trap.go, policy.go, catalog.go, write.go defense wiring, tool/reflect.go, mcp/tools.go import defense, cmd/okf/cmd_add.go screenImportTree.

| # | Severity | File:Line | Issue | RED evidence | Fix |
|---|---|---|---|---|---|
| RA-1 | High | cmd/okf/cmd_add.go screenImportTree | Direct .md import (no staging) would redact user's ORIGINAL files in-place, corrupting source data. | TestStageForDefensePreservesSource failed before fix (original modified). | Added stageForDefense(): when defense enabled and stagingDir=="", copy .md files to temp dir, screen temp dir, import from temp. Original never touched. |
| RA-2 | Medium | pkg/mcp/tools.go:1200 | LoadPolicy(filepath.Dir(bundlePath)) resolved to `<repo>/.okf/.okf/config.yaml` — config never found, defense silently disabled in MCP. | TestImportDocumentDefenseBlock/Redact failed (no config loaded). | Compute repoRoot from bundlePath: if base=="knowledge" → parent; if base==".okf" → parent. |

## Review B (independent round) — Boundary/security/compatibility

Reviewed: regex safety (all RE2 linear, no backtracking), empty/binary input, file walk (os.Walk doesn't follow symlinks), error messages (no secret leak), staging cleanup (defer), block rollback (staging cleaned), disabled byte-compat (verified).

No new issues found. Anti-evidence actions:
- Tested credit card regex on 16-digit phone numbers → Luhn verify rejects.
- Tested empty string Screen → returns content unchanged.
- Tested binary garbage through Screen → no matches, no panic.
- Verified os.Walk does not follow symlinks (path escape not possible via Walk).
- Verified ErrBlocked.Error() contains only DetectorID, never match text.

## Review A/B — 32-file coverage matrix

| Group | Files | Reviewed | Key checks |
|---|---|---|---|
| cmd/okf | 4 (cmd_add.go, cmd_add_defense_test.go, cmd_eval.go, cmd_tool.go) | yes | flag parsing, exit codes, Defense wiring, no secret leak in stdout |
| pkg/mcp | 2 (tools.go, import_defense_test.go) | yes | RA-2 repoRoot fix, unified registry, stdio framing |
| pkg/memorydefense | 6 (catalog.go, screen.go, policy.go, catalog_test.go, fuzz_test.go, screen_bench_test.go) | yes | 16 detectors RE2-safe, Screen redact/block, LoadPolicy, fuzz 77k execs |
| pkg/reflect | 2 (reflect.go, reflect_test.go) | yes | RRF k=60, hard cap 3, abstain threshold, trap gate |
| pkg/relationrecall | 3 (recall.go, recall_test.go, recall_bench_test.go) | yes | bidirectional extends, updates chain, cycle/fork/dangling/self-loop |
| pkg/tool | 3 (reflect.go, write.go, write_defense_test.go) | yes | Service layer, WriteKnowledge Screen, idempotency |
| pkg/trapeval | 2 (trap.go, trap_test.go) | yes | 3-layer scoring, poison ratio, abstain |
| test_mcp.py | 1 | yes | 17 stdio E2E tests all pass |
| OpenSpec docs | 9 (proposal/design/spec/tasks/review/evidence/conformance/release-notes/code-review) | yes | consistency, no unexplained gaps |

## Review B additional edge tests (added this round)
- TestScreenEmptyInput: empty string no panic/no hits
- TestScreenMultipleSecretsRedact: 2 secrets both redacted
- TestScreenLargeInputNoCrash: 1MB normal text, no false positive
- TestScreenInvalidConfigAction: unknown action passes through
- TestScreenMultilinePEM: block triggers on PEM marker
- TestExtendsSelfLoopSkipped: self-extends produces exactly 1 hit

## Fresh run (HEAD=0ab5f52 + new edge tests)
- gofmt: clean
- go build: ok
- go vet: ok
- staticcheck: ok
- go test: all 7 packages green
- race: green on 4 new packages
- shuffle: green
- coverage: memorydefense 90.8%, relationrecall 94.6%, reflect 96.4%, trapeval 86.6%
- fuzz: 77,594 execs / 15s, no crash
- go mod verify: all modules verified
- CLI smoke: normal add exit 0, redact add exit 0, source file unchanged, kb has [REDACTED]
- MCP E2E: 17/17 tests pass
- benchmark: Screen10k=1.19ms/op, Recall10k=0.38ms/op
- secret scan: only test-fixture fake tokens, no real secrets in production code
- gauntlet: no Makefile/Taskfile; project uses `go test ./...` directly as CI entry

## Final gate completion (this round)

### Full-suite fresh run (HEAD=b73e797)
- go build ./...: exit 0
- go vet ./...: exit 0
- staticcheck ./...: exit 0
- go test ./...: 27 packages all ok (no failures)
- go test -race ./...: 27 packages all ok
- go test -shuffle=on ./...: 27 packages all ok

### Gauntlet
- Entry: tools/gauntlet.sh (referenced in README.md and AGENTS.md)
- Result: GAUNTLET PASS
- Coverage: 71% whole-repo
- Mutation: 18 convert + 5 agent-discovery + 8 governed-memory + 7 temporal-memory = 38/38 killed
- Secret scan: clean
- Supply chain: go mod verify all modules verified

### Mutation (this round, on new code)
- M1 (RA-1 staging copy): killed by integration (CLI smoke verified source unchanged)
- M2 (RA-2 repoRoot): KILLED by TestImportDocumentDefenseBlock/Redact
- M3 (block severity high→medium): KILLED by TestScreenBlockReturnsError
- M4 (trapGate approved filter inverted): ESCAPED (no direct unit test for Service.trapGate; indirect via reflect package tests)
- M5 (abstain < → <=): KILLED by TestAbstainWhenThin/TestNoAbstainAtExactThreshold

### Codex E2E (real, this round)
- Model: gpt-5.6-sol__dev via Xeart Router (127.0.0.1:18080 proxy)
- Codex version: 0.153.4
- 3 shell tool calls total:
  1. "Say hello" → Hello (baseline connectivity)
  2. okf tool reflect -q "deployment" → evidence=0, need_clarify=true, trace=[round1]. Correct abstention on thin kb.
  3. okf add with secret (disabled) → exit 0, source unchanged, secret passed through (backward compat).
  4. okf add with secret (block enabled) → skipped (already imported), source unchanged.
- Durable writes: 1 (secret-note.md imported under disabled policy)
- No secret leaked in any Codex output.
