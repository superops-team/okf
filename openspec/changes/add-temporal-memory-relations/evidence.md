# Evidence: Temporal Memory Relations and Review

## 0. Provenance

- Change: `add-temporal-memory-relations` (branch `spec/temporal-memory-relations`)
- Implementation SHA: **af64c35** (implementation commit). This evidence
  document and conformance.md land in a follow-up documentation commit; that
  commit's SHA is recorded by `git log` after commit. The implementation SHA
  is the verified source of record.
- Baseline for comparison (references.md Experiment C):
  `go test ./pkg/parser ./pkg/memorymeta ./pkg/tool -count=1` on pre-implementation
  `main`.
- Build note: after any code change the modern E2E binary must be rebuilt
  (`./okf`) before CLI/MCP end-to-end runs; the shipped harness scripts build
  from source at run time.

## 1. Implementation approach (what changed and what did NOT)

**Approach: CustomFields-only, computed currentness, no second fact source.**

- All temporal state lives in existing `Concept.CustomFields` as optional nested
  keys (`memory_state`, `memory_confidence`, `memory_relation`, `memory_review`).
- Typed accessors live in `pkg/memorymeta` (`State`, `SetState`, `Confidence`,
  `Relation`, `SetRelation`, `Review`, `ValidateTemporal`).
- **No `Concept` struct change, no core `Status` change, no OKF v0.2 struct
  change.** Verified by `TestS07NoNamedTemporalStructFields` (reflection).
- **No graph database, no adjacency database, no second index, no daemon, no
  cache, no external LLM.** Currentness is a deterministic projection built per
  request by `BuildTemporalView` (`pkg/memorymeta/temporal_view.go`) over Stable
  IDs via the existing `identity.Registry`.
- **No reuse of generated code-graph `relation_kind/source/target`**; temporal
  relations use the separate `memory_` namespace.
- Latest transition only in frontmatter (`memory_review`); Git history is the
  unbounded audit log. No transition array.

Write/review persistence reuses the existing atomic rename + parse-verify path
under a new repository-scoped temporal mutex (`repositoryTemporalLock`), acquired
**before** the per-path lock (fixed order, deadlock-free).

## 2. Error codes (wire)

Defined in `pkg/tool/temporal_errors.go`; surfaced verbatim on the wire:

| Code | Meaning |
|---|---|
| `invalid_memory_state` | state/confidence combination not allowed (e.g. proposed without confidence) |
| `invalid_memory_relation` | relation shape wrong (kind, target count, duplicate, self, non-ID target) |
| `memory_ref_not_found` | relation target / review ref resolves to no known durable concept |
| `memory_relation_cross_project` | edge crosses effective projects |
| `memory_relation_cycle` | update edge is inside a directed cycle |
| `ambiguous_update_head` | two+ approved sources update the same target |
| `target_not_current` | approved update does not target the current chain head |
| `memory_state_conflict` | review `expected_state` did not match observed state (CAS miss) |
| `invalid_review_transition` | action/state pair not in the allowed transition table |
| `memory_has_approved_updater` | undo rejected; an approved update depends on the target |
| `temporal_history_too_deep` | walked chain exceeds the 128-item bound |

Errors carry stable refs and bundle-relative paths only; never bodies,
credentials, or absolute paths.

## 3. CLI flags (hyphen) vs MCP JSON (underscore)

Naming follows the existing governed-memory convention:

| Concept | CLI flag (hyphen) | MCP/JSON field (underscore) |
|---|---|---|
| Temporal view | `--memory-view current\|all\|history` | `memory_view` |
| Stable refs (history / explicit context) | `--refs okf_xxx` | `refs` |
| Review queue mode | `--memory-review-queue` | `memory_review_queue` |
| Memory state (writes) | (on `okf_note/log/feedback` payload) | `memory_state` |
| Confidence (writes) | (payload) | `memory_confidence` |
| Relation kind | (payload) | `memory_relation_kind` |
| Relation targets | (payload) | `memory_relation_targets` |
| Evidence refs (writes) | (payload) | `evidence_refs` |
| Review mutation | `okf tool memory-review --ref --action --expected-state` | `okf_memory_review` (`ref`, `action`, `expected_state`) |

## 4. Dual-era catalog counts

- Modern `2026-07-28`: **11 → 12** service-backed tools (added `okf_memory_review`).
- Legacy `2024-11-05`: **20 → 21** tools (same tool added to both eras).
- Exact-count parity is locked by `TestMCPMemoryReviewToolPresentBothEras` and
  `TestMCPTemporalSchemaSharedAcrossEras`. `okf_memory_review` is annotated
  mutating/destructive/non-idempotent/closed-world (MCP annotations are per
  tool, not per action; decline is destructive, approve/undo are not, so the
  conservative whole-tool annotation is used).

## 5. Benchmark table (S41)

Corpus: **10,000 durable concepts** (6,000 independent approved notes; 1,000
length-3 linear update chains; 500 extends edges; 400 proposed + 100 declined).
Run: `go test -bench=. -benchmem -benchtime=20x`. Environment recorded in
`pkg/memorymeta/temporal_benchmark_test.go` (AMD EPYC 9Y24, go1.26). **No hard
regression gate is set yet** — gates follow only after a repeatable baseline.

| Benchmark function | What it measures | ns/op | B/op | allocs/op |
|---|---|---:|---:|---:|
| `BenchmarkTemporalBuildView10k` | `BuildTemporalView` over the full 10k corpus (cold projection) | ~19.5–20.1M | ~6.15M (6,154,850) | 31,132 |
| `BenchmarkTemporalHistoryChain3` | `view.History(head)` on a healthy length-3 chain (view prebuilt) | ~172–235K (171,674) | 368 | 6 |
| `BenchmarkMemoryReviewQueueOrdering` | review-queue ordering over ~500 proposed concepts | ~592,238 | 61,568 | 708 |

No persistent/global graph cache or second index is introduced; each request
builds one projection.

## 6. Targeted mutant table (S42)

Runner: `tools/mutants-temporal-memory.sh`, wired as gauntlet layer **L9d**
(`tools/gauntlet.sh`). Modified files are byte-restored after every mutant.
**Result: 7/7 killed.**

| Mutant | Mutation injected | Invariant guarded | Killed |
|---|---|---|---|
| T-M1 | extends accepted as updates (kind check dropped) | S04/S08 edge-kind semantics | yes |
| T-M2 | proposed/declined-source edge affects currentness | S10 proposal/decline quarantine | yes |
| T-M3 | directed cycle no longer marked (cycle DFS neutralized) | S13 cycle fail-closed | yes |
| T-M4 | `expected_state` CAS comparison skipped in `ReviewMemory` | S29 compare-and-set | yes |
| T-M5 | declined leaks into the current view | S26 declined exclusion | yes |
| T-M6 | history returns newest-first | S09/S15 oldest→newest ordering | yes |
| T-M7 | approved-update `target_not_current` preflight disabled | S11/S30 head freshness | yes |

## 7. Real Codex harness (S43)

Script: `tools/verify-temporal-memory-codex.sh`. **Result: PASS**, model
`gpt-5.6-sol__dev`.

- **Phase A (read-only, 0 mutations):** `okf_query` current view sees A; the
  review queue lists proposal P. Asserts no review mutation occurs before
  authorization.
- **Phase B (exactly one mutation):** a single `okf_memory_review` call approves
  `proposed → approved`.
- **Phase C (read-only):** current view now equals P; history is ordered
  A → P.

Environment note: the `workspace-write` sandbox does not propagate a write to
the spawned MCP child process, so Phase B runs under `danger-full-access`;
Phases A and C stay strictly read-only.

## 8. Idempotency / review invariants (locked by tests)

- Temporal creation fields (`memory_state`, `memory_confidence`,
  `memory_relation_kind`, `memory_relation_targets`) are inside
  `writeKnowledgePayload` / `payload_hash`. Identical retry under the same
  idempotency key reuses the concept and returns its **current** review state +
  stable handle without reverting it; changing any temporal field under the same
  key returns `idempotency_conflict`. (`TestTemporalS21...`)
- Review never rewrites creation-time `payload_hash` / `idempotency_key`, so the
  original write retry keeps resolving the reviewed concept.
- Review is CAS: `expected_state` must match exactly; a stale reviewer gets
  `memory_state_conflict` and **no file / no review timestamp changes**.
  (`TestTemporalS29CASRace`, `-race`, exactly one winner per iteration.)
- Review rewrites the target file atomically; on post-rename failure it restores
  the saved original bytes and parse-verifies — it never uses the new-file
  rollback that would delete the concept. (`TestTemporalS31...`)
- Approve/decline stash `previous_confidence` in the latest bounded
  `memory_review` record; `undo` restores the exact finite value.
- Undo of an approved concept that has an approved updater is rejected with
  `memory_has_approved_updater` and the dependent ref.

## 9. Known design notes (recorded, not bugs)

1. **`reviewClock` is a package-level var in `pkg/tool/memory_review.go`**
   (`var reviewClock = func() time.Time { return time.Now().UTC() }`). It is the
   injectable wall clock used to stamp `memory_review.reviewed_at`. Tests swap
   it for determinism; it is process-global by design (one review mutation per
   repository at a time under the temporal lock), not per-instance state.
2. **T3.3 strict-lint pipeline — wired.** `pkg/lint/temporal_lint.go` runs a
   self-contained whole-bundle temporal pass inside `LintBundle`, reading
   additive `Temporal` fields on `lint.Concept` (populated at both conversion
   sites via `memorymeta.State/Relation/Confidence`). It flags malformed
   state/confidence/relation, dangling targets, cross-project edges, ambiguous
   forks, cycles, and chains >128 nodes. Non-strict → warnings; `--strict` →
   errors. Legacy/no-temporal bundles are unaffected. Covered by
   `pkg/lint/temporal_lint_test.go` (malformed/dangling/cross-project/ambiguous/
   cycle/depth/strict-vs-warning). S06 is fully conformant.
