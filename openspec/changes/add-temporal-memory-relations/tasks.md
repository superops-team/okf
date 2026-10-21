# Tasks: Temporal Memory Relations and Review

P0–P5 are dependency order only. All phases, reviews, evidence, conformance, and final verification are required before completion.

## P0 — Metadata contract and topology projection

### T0.1 Typed temporal accessors
- **Files**: `pkg/memorymeta/temporal.go`, `pkg/memorymeta/temporal_test.go`
- **RED first**: missing/explicit/malformed states; proposed confidence/evidence; relation shapes/bounds; unknown nested keys; canonical Stable ID normalization.
- **Implement**: `State`, `SetState`, `Confidence`, `Relation`, `SetRelation`, `Review`, `ValidateTemporal`.
- **Scenarios**: S01–S07.

### T0.2 Lossless parser/serializer compatibility
- **Files**: `pkg/memorymeta/temporal_roundtrip_test.go`, existing parser test fixtures only if needed.
- **RED first**: frontmatter → CustomFields → serialization → parse preserves temporal and unrelated fields; reflection confirms no core struct additions.
- **Implement**: expected to require no parser production changes; any necessary change must remain generic CustomFields behavior.
- **Scenarios**: S02, S04, S07.

### T0.3 Deterministic temporal projection
- **Files**: `pkg/memorymeta/temporal_view.go`, `pkg/memorymeta/temporal_view_test.go`
- **RED first**: current/head/history golden topology, inactive proposals/declines, target-not-current, fork, cycle, dangling, cross-project, depth bound.
- **Implement**: O(n+e) bundle projection using `identity.Registry`, linear update-chain constraints, extends validation, deterministic warnings/errors.
- **Scenarios**: S08–S16, S40.

## P1 — Durable write and response contract

### T1.1 Extend write request and payload hash
- **Files**: `pkg/tool/write.go`, `pkg/tool/write_test.go`
- **RED first**: legacy byte/semantic parity; new state/confidence/relation normalization; creation-time temporal fields change payload hash; proposed evidence requirement; original retry after review returns current state without reverting it.
- **Implement**: additive request fields and payload hashing; no changes to ordinary path when temporal fields omitted.
- **Scenarios**: S18, S20, S21.

### T1.2 Return stable review handle
- **Files**: `pkg/tool/write.go`, MCP write response tests.
- **RED first**: new/reused durable write result includes canonical `okf_id` and `ref`, while existing `concept_id`, `concept_path`, and `created` remain unchanged.
- **Implement**: extend `WriteKnowledgeResult` with `OKFID` and `Ref`; verify persisted stable identity before response.
- **Scenarios**: supports S20, S25, S37–S39; explicit compatibility test.

### T1.3 Relation preflight and repository mutation lock
- **Files**: `pkg/tool/write.go`, `pkg/tool/temporal_lock.go`, `pkg/tool/write_temporal_test.go`
- **RED first**: approved update, proposed quarantine, invalid target, cross-project, stale head, concurrent competing updates, lock-order deadline.
- **Implement**: repository-root temporal mutex, fresh bundle preflight, lock order temporal→path, existing atomic write/verify/rollback reuse.
- **Scenarios**: S19–S24.

## P2 — Review mutation

### T2.1 Service ReviewMemory operation
- **Files**: `pkg/tool/memory_review.go`, `pkg/tool/memory_review_test.go`
- **RED first**: approve, decline, undo, invalid transitions, stale expected state, competing proposal approvals, persistence failures, bounded record.
- **Implement**: CAS state machine under repository temporal lock, injected clock/write hook, atomic same-file rewrite, parse-back verification and rollback.
- **Scenarios**: S25–S31.

### T2.2 Undo dependency safety
- **Files**: same as T2.1.
- **RED first**: undo approved chain head succeeds; undo approved historical member with an approved updater fails; undo of approved extends does not invalidate unrelated nodes.
- **Implement**: reject undo when any approved update depends on the target (`memory_has_approved_updater`), with dependent ref in remediation.
- **Scenarios**: amendment to S27/S28 and topology integrity.

### T2.3 Review metadata confidence restoration
- **Files**: `pkg/memorymeta/temporal.go`, `pkg/tool/memory_review.go`.
- **RED first**: approve/decline stores previous confidence in latest bounded review record; undo restores exact finite value; second review cycle remains correct.
- **Implement**: optional `previous_confidence`; latest record only.
- **Scenarios**: S25–S27, S31.

## P3 — Query and context views

### T3.1 Query modes and validation
- **Files**: `pkg/tool/service.go`, `pkg/tool/query_temporal_test.go`
- **RED first**: safe-current default with legacy-data byte parity, explicit all audit view, current pre-rank filter, history one-ref, body-free queue ordering/limits, incompatible mode matrix.
- **Implement**: additive `MemoryView`, `Refs`, `MemoryReviewQueue`; dedicated result types; validation before normal query-required guard.
- **Scenarios**: S17, S32–S35.

### T3.2 Context current view and explicit refs
- **Files**: `pkg/tool/service.go`, `pkg/tool/context_temporal_test.go`
- **RED first**: safe-current default with legacy-data byte parity; explicit all annotations; explicit historical/proposed/declined refs preserved; current filtering before scoring/packing.
- **Implement**: additive `MemoryView`; reuse temporal projection; explicit refs override visibility filter.
- **Scenarios**: S17, S36.

### T3.3 Strict lint whole-bundle topology
- **Files**: existing lint integration, `pkg/memorymeta` validator, lint tests.
- **RED first**: field syntax, non-durable temporal fields, dangling/cross-project/fork/cycle/depth errors include safe paths; default lint warns, strict fails.
- **Implement**: one whole-bundle temporal validation pass; no per-file false success for graph errors.
- **Scenarios**: S05–S06, S12–S16.

## P4 — CLI, MCP, and Agent integration

### T4.1 CLI flags and memory-review command
- **Files**: `cmd/okf/cmd_tool.go`, `cmd/okf/cmd_tool_temporal_test.go`
- **RED first**: real command parsing, mode matrix, query-optional special modes, review envelope, help contracts, hyphen naming.
- **Implement**: extend query/context and add `tool memory-review` dispatcher.
- **Scenarios**: S33–S37.

### T4.2 Modern/legacy MCP parity
- **Files**: `pkg/mcp/tools.go`, `pkg/mcp/tools_temporal_test.go`, `test_mcp.py`, `test_ext_skills.py`
- **RED first**: identical temporal schemas, request mapping, stable write handles, review annotations, exact catalogs 21/12, unknown fields rejected.
- **Implement**: shared Service handlers; no duplicate protocol-specific behavior.
- **Scenarios**: S18–S21, S25–S38.

### T4.3 Agent Skill workflow
- **Files**: `pkg/agentconfig/workflow.go`, documentation contract tests.
- **RED first**: inference→proposed with evidence/confidence; review requires explicit user instruction; current view default for memory-sensitive task; no auto-approve language.
- **Implement**: update relevant W clauses once without duplicating canonical rules.
- **Scenarios**: S39.

## P5 — Quality closure

### T5.1 Golden and property tests
- **Files**: topology golden fixture; property tests with standard `testing/quick`.
- **Cover**: stable ordering independent of input permutation; acyclic linear chain returns one head; extends preserves current set; parser round-trip.
- **Scenarios**: S07–S16, S40.

### T5.2 Benchmarks and allocation evidence
- **Files**: `pkg/memorymeta/temporal_benchmark_test.go`, `pkg/tool/memory_review_benchmark_test.go`.
- **Run**: 10,000 concepts for current projection/query, history, queue, approved update, competing approval.
- **Record**: ns/op, B/op, allocs/op and repeat variance; set regression gates only after fresh baseline.
- **Scenarios**: S41.

### T5.3 Targeted mutation runner
- **Files**: `tools/mutants-temporal-memory.sh`, `tools/gauntlet.sh`.
- **Mutants**: updates↔extends, proposed edge active, cycle accepted, expected-state skipped, declined current, history order reversed; byte restore after every mutant.
- **Scenarios**: S42.

### T5.4 Real Codex harness
- **Files**: `tools/verify-temporal-memory-codex.sh`.
- **Flow**: build current source, temp repo with known refs, install MCP, current query, inspect proposal, assert zero mutations before explicit authorization, approve exact ref/state, current query, history query; parse actual event arguments.
- **Scenarios**: S43.

### T5.5 Docs, release notes, evidence, conformance
- **Files**: README EN/ZH, MCP docs, `release-notes.md`, `evidence.md`, `conformance.md`, `code-review.md`.
- **Requirements**: distinguish code relations vs memory relations; document safe current default, explicit all audit view, review consent, current/history, stable write handles, no graph DB/automatic inference.
- **Scenarios**: S37–S44.

### T5.6 Two strict code reviews and final fresh verification
- Review 1: correctness, topology, mutation durability, CAS, compatibility.
- Fix every critical/high/medium and test-lock relevant low finding.
- Review 2: adversarial state transitions, concurrency, evidence freshness, schema parity, security/redaction.
- Run final `tools/gauntlet.sh`, modern+legacy MCP E2E, real Codex harness after the last code change.
- **Scenarios**: S44.

## Scenario matrix

| Scenario | Primary automated test | Entry point |
|---|---|---|
| S01 | missing-state accessor | `memorymeta.State` |
| S02 | state normalization | `memorymeta.State` |
| S03 | proposed confidence/evidence | `ValidateTemporal` / write |
| S04 | relation namespace parse | `memorymeta.Relation` |
| S05 | relation bounds/identity | `ValidateTemporal` |
| S06 | malformed conservative + strict | accessor / `okf lint --strict` |
| S07 | CustomFields round-trip/reflection | parser + memorymeta |
| S08 | legacy/extends current set | `BuildTemporalView` |
| S09 | linear chain golden | `BuildTemporalView` |
| S10 | inactive proposal/decline | `BuildTemporalView` |
| S11 | stale target head | temporal write preflight |
| S12 | ambiguous fork | projection/lint |
| S13 | cycle | projection/lint |
| S14 | dangling/cross-project/type/state | projection/write/lint |
| S15 | history any member | query history |
| S16 | history depth bound | query history |
| S17 | default query/context parity | Service Query/Context |
| S18 | legacy write parity | WriteKnowledge/MCP |
| S19 | approved update activation | WriteKnowledge |
| S20 | proposed quarantine | WriteKnowledge |
| S21 | temporal idempotency hash | WriteKnowledge |
| S22 | preflight/rollback | WriteKnowledge |
| S23 | competing concurrent updates | WriteKnowledge race |
| S24 | lock order/deadlock | write stress test |
| S25 | approve | ReviewMemory |
| S26 | decline | ReviewMemory |
| S27 | undo/confidence restore | ReviewMemory |
| S28 | invalid transitions/dependency | ReviewMemory |
| S29 | CAS race | ReviewMemory race |
| S30 | competing proposal approvals | ReviewMemory |
| S31 | atomic bounded review | ReviewMemory fault injection |
| S32 | current pre-rank filter | Query |
| S33 | history one ref | Query CLI/Service |
| S34 | deterministic body-free queue | Query CLI/Service |
| S35 | mutual exclusion matrix | Query validation |
| S36 | context current/explicit refs | Context |
| S37 | CLI contracts | cmd_tool temporal tests |
| S38 | MCP era parity/counts/annotations | MCP E2E |
| S39 | workflow consent contract | agentconfig tests |
| S40 | topology golden | memorymeta golden |
| S41 | 10k benchmarks/no cache | benchmarks + structural test |
| S42 | six mutants | mutation runner |
| S43 | real Codex authorized flow | Codex harness |
| S44 | final regression/conformance | GAUNTLET + evidence |
