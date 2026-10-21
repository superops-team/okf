# Specification: Temporal Memory Relations and Review

## Requirement: Temporal memory metadata via CustomFields

OKF SHALL support optional `memory_state`, `memory_confidence`, `memory_relation`, and latest `memory_review` metadata through existing `Concept.CustomFields` and typed accessors in `pkg/memorymeta`. Core `Concept`, `Status`, and generated code-relation fields SHALL NOT be modified.

### Scenario S01: Missing state defaults to approved
- **GIVEN** a legacy durable note/event/feedback with no `memory_state`
- **WHEN** `memorymeta.State` is called
- **THEN** effective state is `approved`
- **AND** default query/write behavior remains unchanged

### Scenario S02: Explicit memory states are normalized
- **GIVEN** durable concepts declaring `Approved`, `proposed`, or `DECLINED`
- **WHEN** state is parsed
- **THEN** values normalize to `approved`, `proposed`, and `declined`
- **AND** state remains in CustomFields, not core Status

### Scenario S03: Proposed memory_confidence contract
- **GIVEN** a proposed concept
- **WHEN** temporal validation runs
- **THEN** finite `memory_confidence` in `[0,1]` is required
- **AND** at least one evidence_ref is required
- **AND** approved/declined concepts carrying `memory_confidence` fail strict validation

### Scenario S04: Relation structure is namespaced and typed
- **GIVEN** `memory_relation: {kind: updates, targets: [okf_11111111111111111111111111111111]}`
- **WHEN** `memorymeta.Relation` is called
- **THEN** it returns canonical relation kind and bare Stable IDs
- **AND** existing code fields `relation_kind/source/target` are neither read nor changed

### Scenario S05: Relation bounds and Stable ID validation
- **GIVEN** updates with zero/two targets, extends with zero/nine targets, duplicate targets, path/title targets, invalid IDs, or a self-edge
- **WHEN** validation runs
- **THEN** each is rejected with `invalid_memory_relation`
- **AND** updates accepts exactly one target and extends accepts 1–8 unique Stable IDs

### Scenario S06: Malformed metadata is conservative
- **GIVEN** unknown/wrong-type `memory_state`, relation kind, nested field, timestamp, or confidence
- **WHEN** read in non-strict mode
- **THEN** the concept is treated as non-current/proposed and a warning is emitted
- **WHEN** `okf lint --strict` runs
- **THEN** validation fails with the concept's bundle-relative path

### Scenario S07: Temporal CustomFields round-trip losslessly
- **GIVEN** a concept containing temporal fields plus arbitrary user fields
- **WHEN** parsed, serialized, and parsed again
- **THEN** temporal and arbitrary fields are preserved
- **AND** the core Concept struct contains no temporal relation/state fields

## Requirement: Deterministic current and history projection

OKF SHALL compute currentness from approved `updates` edges over Stable IDs. It SHALL NOT persist a duplicated `is_latest` field or build a second database/index.

### Scenario S08: Legacy and unrelated memories remain current
- **GIVEN** approved durable concepts without updates edges
- **WHEN** the temporal view is built
- **THEN** all are current
- **AND** extends edges do not make any concept historical

### Scenario S09: Linear update chain selects one current head
- **GIVEN** approved A, B updates A, and C updates B
- **WHEN** current view is computed
- **THEN** C is current and A/B are historical
- **AND** history is ordered A, B, C

### Scenario S10: Proposed and declined relations are inactive
- **GIVEN** proposed or declined P that updates approved A
- **WHEN** current view is computed
- **THEN** A remains current
- **AND** P remains inspectable but does not affect currentness

### Scenario S11: Update must target current head
- **GIVEN** approved B already updates A
- **WHEN** a new approved C attempts to update A
- **THEN** write/approval fails with `target_not_current`
- **AND** remediation identifies current head B

### Scenario S12: Multiple update heads isolate the invalid component
- **GIVEN** malformed existing approved B and C both updating A
- **WHEN** history for A/B/C is requested
- **THEN** it fails with `ambiguous_update_head` and no arbitrary winner is chosen
- **WHEN** current query spans the repository
- **THEN** that invalid component is excluded with stable-ref warnings while unrelated projects/components remain queryable

### Scenario S13: Update cycles isolate the invalid component
- **GIVEN** A updates B and B updates A, including longer cycles
- **WHEN** history for an involved ref is requested
- **THEN** it fails with `memory_relation_cycle`
- **WHEN** current query spans the repository
- **THEN** the cycle component is excluded with warnings and unrelated components remain available
- **AND** strict lint reports every involved stable ref

### Scenario S14: Dangling and cross-project targets fail validation
- **GIVEN** a target that is missing, non-durable, proposed/declined for an approved relation, or in a different effective project
- **WHEN** relation preflight or approval runs
- **THEN** it fails with the precise safe error code
- **AND** no file is modified

### Scenario S15: History resolves from any chain member
- **GIVEN** chain A ← B ← C
- **WHEN** history is requested for A, B, or C
- **THEN** each returns the same ordered chain A, B, C
- **AND** current_ref is C and requested_ref reflects the input

### Scenario S16: History traversal is bounded
- **GIVEN** a chain longer than 128 items
- **WHEN** history is requested
- **THEN** it fails with `temporal_history_too_deep`
- **AND** it never labels a partial chain as complete

### Scenario S17: Legacy-data default parity and safe temporal default
- **GIVEN** a repository containing no temporal fields
- **WHEN** normal Query/Context is called with no `memory_view`
- **THEN** its existing response bytes and selection behavior remain unchanged
- **GIVEN** temporal fields are present
- **WHEN** `memory_view` is omitted
- **THEN** it behaves as `current`, excluding historical/proposed/declined durable memories
- **AND** `memory_view=all` explicitly includes them for audit with temporal annotations

## Requirement: Temporal-aware durable writes

OKF SHALL extend existing durable note/event/feedback writes with optional state, `memory_confidence`, and explicit memory relation fields while preserving existing idempotency, atomic persistence, and rollback behavior.

### Scenario S18: Legacy durable write stays approved
- **GIVEN** the existing okf_note/log/feedback request with no temporal fields
- **WHEN** written
- **THEN** output and persisted fields remain backward compatible
- **AND** effective memory state is approved by default

### Scenario S19: Approved update write activates immediately
- **GIVEN** current approved A
- **WHEN** approved B is written with updates target A
- **THEN** B is persisted atomically and becomes current
- **AND** A remains on disk and appears in history

### Scenario S20: Proposed relation write is quarantined and returns a review handle
- **GIVEN** current approved A
- **WHEN** proposed P with `memory_confidence` and evidence_refs and updates target A is written through any durable MCP write tool
- **THEN** P is persisted as proposed
- **AND** A remains current until P is approved
- **AND** response preserves concept_id/concept_path/created and adds canonical okf_id/ref
- **AND** note, log, and feedback all accept evidence_refs

### Scenario S21: Temporal creation fields participate in idempotency hash
- **GIVEN** a successful durable write
- **WHEN** identical creation payload is retried with the same idempotency key
- **THEN** the existing concept is reused and its current review state/stable handle are returned without reverting review
- **WHEN** creation-time state, memory_confidence, relation kind, or targets change under that key
- **THEN** existing `idempotency_conflict` is returned
- **AND** review transitions never rewrite creation-time payload_hash/idempotency_key

### Scenario S22: Relation preflight and atomic rollback
- **GIVEN** invalid topology or a persistence/verification failure
- **WHEN** a relation-bearing write runs
- **THEN** preflight occurs before the first write where possible
- **AND** any visible unverified file is rolled back
- **AND** current/history projection remains unchanged

### Scenario S23: Concurrent approved updates serialize
- **GIVEN** two concurrent approved writes targeting the same current A
- **WHEN** both execute
- **THEN** repository-scoped mutation locking allows at most one success
- **AND** the loser receives `target_not_current` or `ambiguous_update_head`
- **AND** race tests show no data race

### Scenario S24: Lock order is deterministic
- **GIVEN** relation-bearing and ordinary writes run concurrently
- **WHEN** locks are acquired
- **THEN** relation operations acquire repository temporal lock before per-path lock
- **AND** ordinary writes keep the existing per-path path
- **AND** deadlock stress tests terminate within the test deadline

## Requirement: Proposed-memory review

OKF SHALL provide an explicit, compare-and-set review mutation for durable proposed memories. Agents SHALL NOT approve their own proposals without explicit user instruction.

### Scenario S25: Approve proposed memory
- **GIVEN** a durable proposed memory and `expected_state=proposed`
- **WHEN** review action approve runs
- **THEN** state becomes approved, memory_confidence is removed after being saved in the bounded review record, and that record is written
- **AND** any valid updates relation is activated after fresh topology validation

### Scenario S26: Decline proposed memory
- **GIVEN** a durable proposed memory
- **WHEN** decline runs with expected proposed state
- **THEN** state becomes declined and memory_confidence is removed after being saved in the bounded review record
- **AND** it is excluded from current view but remains on disk and addressable by explicit ref

### Scenario S27: Undo reviewed memory and preserve chain integrity
- **GIVEN** an approved or declined memory with a valid review record
- **WHEN** undo runs with matching expected state
- **THEN** state returns to proposed and exact previous memory_confidence is restored
- **AND** approved update effects disappear from current view
- **BUT** undo of an approved memory that has an approved updater is rejected with `memory_has_approved_updater` and the dependent ref

### Scenario S28: Invalid transition is rejected
- **GIVEN** approve on approved, decline on declined, undo on unreviewed approved, or review of a non-durable code/source concept
- **WHEN** review runs
- **THEN** it fails with `invalid_review_transition`
- **AND** MCP-authored durable concepts remain reviewable even though `Generated.By=okf-mcp`
- **AND** no file or review timestamp changes

### Scenario S29: Stale reviewer loses CAS race
- **GIVEN** two reviewers observed proposed state
- **WHEN** approve and decline race with `expected_state=proposed`
- **THEN** exactly one transition succeeds
- **AND** the other receives `memory_state_conflict`
- **AND** no duplicate review record is appended

### Scenario S30: Approval revalidates target freshness
- **GIVEN** two proposed updates P1 and P2 both targeting current A
- **WHEN** P1 is approved, then P2 approval is attempted
- **THEN** P1 succeeds and becomes current
- **AND** P2 remains proposed and fails with `target_not_current`, pointing to P1

### Scenario S31: Review persistence is atomic and bounded
- **GIVEN** review persistence fails before or after rename
- **WHEN** review executes
- **THEN** the implementation restores the saved original bytes atomically and verifies the restored concept, or leaves the original unchanged
- **AND** it never uses new-file rollback semantics that delete the reviewed concept
- **AND** `memory_review` contains only the latest transition, not an unbounded array
- **AND** Git history remains the full audit source

## Requirement: Query, context, CLI, and MCP integration

OKF SHALL expose temporal views through existing query/context operations and one dedicated review mutation, with matching modern and legacy MCP contracts.

### Scenario S32: Default/current query filters before ranking
- **GIVEN** historical, current, proposed, and declined concepts with similar content
- **WHEN** `okf tool query -q "current architecture decision"` or explicit `--memory-view current` runs
- **THEN** only current approved concepts enter structured filtering/scoring/TopK
- **AND** excluded items cannot consume the limit
- **AND** `--memory-view all` includes every state with additive `memory_state`, `memory_current`, `memory_relation_kind`, and `memory_relation_targets` annotations but never adds bodies

### Scenario S33: History query uses exactly one ref
- **GIVEN** `--memory-view history`
- **WHEN** exactly one valid `--refs` value and empty query are supplied
- **THEN** a dedicated MemoryHistoryResult is returned
- **AND** zero/multiple refs or a non-empty query are rejected as ambiguous

### Scenario S34: Review queue is body-free and deterministic
- **GIVEN** proposed memories with differing memory_confidence/timestamps/IDs
- **WHEN** `okf tool query --memory-review-queue --limit N` runs
- **THEN** order is memory_confidence desc, created_at asc, okf_id asc
- **AND** output contains metadata/evidence/relation but no body
- **AND** default limit is 20 and max is 100

### Scenario S35: Query modes are mutually exclusive
- **GIVEN** memory_review_queue combined with query/history/memory_check/grouping, or incompatible refs/view values
- **WHEN** validation runs
- **THEN** request fails with `invalid_request`
- **AND** remediation names the valid combinations

### Scenario S36: Context safe default, all view, and explicit refs
- **GIVEN** temporal fields exist and a normal context query omits memory_view or sets `current`
- **WHEN** context is built
- **THEN** temporal filtering occurs before scoring and packing
- **WHEN** context uses `memory_view=all`
- **THEN** all states are eligible and returned items carry temporal annotations
- **GIVEN** explicit refs to historical/proposed/declined concepts
- **WHEN** context is called
- **THEN** explicitly requested items are returned under budget with memory_state/current metadata rather than hidden

### Scenario S37: CLI entry points and naming are real
- **WHEN** the following commands run:
  - `okf tool query -q x --memory-view current`
  - `okf tool query --memory-view history --refs okf_x`
  - `okf tool query --memory-review-queue`
  - `okf tool context -q x --memory-view current`
  - `okf tool memory-review --ref okf_x --action approve --expected-state proposed`
- **THEN** CLI uses hyphen flags and returns standard ToolEnvelope JSON
- **AND** help text documents every mode and incompatibility

### Scenario S38: MCP schemas match across eras
- **GIVEN** modern and legacy MCP tool discovery
- **WHEN** schemas are compared
- **THEN** okf_query/context/write temporal fields and okf_memory_review are identical
- **AND** modern exact catalog is 12 and legacy exact catalog is 21
- **AND** review is marked mutating/destructive/non-idempotent

### Scenario S39: Agent workflow requires explicit review consent
- **GIVEN** an Agent generates an inferred reusable memory
- **WHEN** it persists that inference
- **THEN** workflow instructs it to write proposed state with memory_confidence and evidence_refs
- **AND** it may list or explain proposals without approval
- **AND** it SHALL call approve/decline only after explicit user instruction

## Requirement: Verification, performance, and safety

### Scenario S40: Temporal golden topology
- **GIVEN** a fixture covering linear updates, extends, legacy, proposed, declined, dangling, cycle, fork, stale head, and cross-project edges
- **WHEN** golden tests run
- **THEN** current/head/history/error outputs match expected results exactly

### Scenario S41: Performance is measured without hidden cache
- **GIVEN** 10,000 durable concepts and bounded relations
- **WHEN** current query, history, review queue, approved update, and competing approval benchmarks run
- **THEN** ns/op, B/op, and allocs/op are recorded
- **AND** no persistent/global graph cache or second index is introduced
- **AND** hard regression gates are set only from a repeatable implementation baseline

### Scenario S42: Targeted mutants are killed
- **GIVEN** mutants swapping updates/extends, activating proposed edges, accepting cycles, skipping expected-state, leaking declined into current, or destabilizing history order
- **WHEN** the mutation runner executes
- **THEN** all mutants are killed and modified files are byte-for-byte restored

### Scenario S43: Real Codex flow verifies product usability
- **GIVEN** a temporary OKF repository installed into a real Codex client
- **WHEN** Codex queries current memory, inspects a proposal, receives explicit authorization, approves it, and queries history
- **THEN** actual MCP call names and arguments match the contract
- **AND** no review mutation occurs before the authorization step
- **AND** default/history outputs reflect the approved transition

### Scenario S44: Full regression and conformance
- **WHEN** implementation is complete
- **THEN** GAUNTLET, full race/shuffle, modern and legacy MCP E2E, parser round-trip, default parity, and secret scan pass
- **AND** conformance maps S01–S44 one-to-one to implementation and automated tests
- **AND** no unexplained partial or gap remains
