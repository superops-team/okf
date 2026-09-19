# Design: Temporal Memory Relations and Review

## 1. Principles

1. **Git files remain the fact source.** Relations and review state live in the related concept's frontmatter CustomFields.
2. **Stable IDs are the only edge identity.** No paths, titles, or content hashes as relation targets.
3. **Currentness is computed.** Do not persist `is_latest`; avoid duplicated state and stale flags.
4. **Explicit relations only.** V1 never asks an LLM to infer updates, contradictions, or derives.
5. **Proposals are quarantined.** Proposed/declined memories do not affect current retrieval.
6. **CAS mutations.** Review requires the caller's observed state and fails closed when stale.
7. **Legacy-data parity.** Repositories without temporal fields produce existing results; once temporal fields exist, omitted view defaults to safe current-only behavior.
8. **Bounded graph work.** Only update-chain traversal; no arbitrary recursive graph query.

## 2. Existing constraints and naming

- `Concept` and core `Status` cannot change (`AGENTS.md`).
- Existing `relation_kind/source/target` describe generated code relationships. Temporal memory uses the `memory_` namespace.
- `pkg/memorymeta` already owns governance, code refs, and duplicate checking; temporal accessors belong there.
- Stable ref resolution uses `pkg/identity.Registry` and fails on duplicate IDs.
- Durable writes use `Service.WriteKnowledge`, atomic rename, per-path locking, persisted parse verification, and rollback.
- Normal `okf tool query` requires `-q`; explicit review/history modes define their own validation.

## 3. CustomFields model

### 3.1 Canonical frontmatter

```yaml
memory_state: approved                  # approved | proposed | declined
memory_confidence: 0.72                 # required only for proposed
memory_relation:
  kind: updates                         # updates | extends
  targets:
    - okf_0123456789abcdef0123456789abcdef
memory_review:
  reviewed_at: "2026-09-19T12:00:00Z"
  previous_state: proposed
  current_state: approved
  action: approve
```

`memory_review` contains only the latest transition; Git history is the complete audit log. No unbounded transition array is stored.

### 3.2 Typed API

```go
package memorymeta

type MemoryState string
const (
    MemoryApproved MemoryState = "approved"
    MemoryProposed MemoryState = "proposed"
    MemoryDeclined MemoryState = "declined"
)

type RelationKind string
const (
    RelationUpdates RelationKind = "updates"
    RelationExtends RelationKind = "extends"
)

type MemoryRelation struct {
    Kind    RelationKind
    Targets []string
}

type ReviewRecord struct {
    ReviewedAt        string
    PreviousState     MemoryState
    CurrentState      MemoryState
    Action            string
    PreviousConfidence *float64
}

func State(c *okf.Concept) (MemoryState, string)
func SetState(c *okf.Concept, state MemoryState)
func Confidence(c *okf.Concept) (float64, bool, string)
func Relation(c *okf.Concept) (MemoryRelation, string)
func SetRelation(c *okf.Concept, relation MemoryRelation)
func Review(c *okf.Concept) (ReviewRecord, string)
func ValidateTemporal(c *okf.Concept, strict bool) []string
```

Missing `memory_state` defaults to `approved`. Unknown/wrong-type values produce warning + conservative `proposed` in non-strict reads and fail in strict validation. Conservative fallback prevents malformed memories from silently entering current view.

### 3.3 Bounds

- targets per relation: updates exactly 1, extends 1–8;
- target string max 64 bytes and must parse as canonical `okf_id` or canonical URI normalized to bare ID;
- relation object accepts only `kind` and `targets`; unknown nested fields warn/non-strict and fail/strict;
- review timestamps RFC3339Nano UTC; action enum approve/decline/undo;
- confidence finite `[0,1]`, required only for proposed, forbidden for approved/declined after review;
- no nested history list.

## 4. Temporal graph projection

### 4.1 Build

`BuildTemporalView(concepts)`:

1. Build one `identity.Registry`; duplicate or invalid explicit IDs fail closed.
2. Parse state/relation for durable concepts and group them by effective project.
3. Treat malformed temporal metadata as non-current within its project and return warnings in non-strict query mode; strict lint rejects it.
4. Ignore relations originating from proposed/declined concepts for currentness, but retain them for inspection.
5. For each approved `updates` relation, validate target exists, is durable and approved, is not self, and shares the same effective project.
6. Build `updatedBy[target] = source` and mark a connected component invalid if multiple approved sources update the same target (`ambiguous_update_head`).
7. Detect directed update cycles using color DFS and mark only involved connected components invalid.
8. Current approved concepts are those not present in `updatedBy`; approved extends concepts and their targets remain current.
9. A current query excludes invalid temporal components and emits stable-ref warnings; history for a ref in an invalid component fails with the precise topology error. Unrelated projects/components remain available. `all` remains an explicit audit view and includes invalid items with warnings.

Effective project is the normalized `CustomFields["project"]` string; both empty counts as the same default project. One empty and one non-empty is cross-project and rejected.

### 4.2 Update-chain rules

An update chain is linear in v1:

```text
A <-updates- B <-updates- C
```

- Each approved memory can update at most one target.
- Each target can be updated by at most one approved memory.
- A new approved update must target the current head. Targeting A after B already updates A fails with `target_not_current`, and remediation returns B's stable ID.
- Proposed updates may target the current head but do not reserve it. At approval time validation is repeated under mutation lock; only the first compatible approval wins.
- `extends` can point to current or historical approved memories, but not proposed/declined memories.

### 4.3 History response

For one stable ref, history finds the containing update chain and returns oldest → newest:

```json
{
  "requested_ref": "okf_B",
  "current_ref": "okf_C",
  "items": [
    {"okf_id":"okf_A","memory_state":"approved","current":false},
    {"okf_id":"okf_B","memory_state":"approved","current":false,
     "edge":{"kind":"updates","target":"okf_A"}},
    {"okf_id":"okf_C","memory_state":"approved","current":true,
     "edge":{"kind":"updates","target":"okf_B"}}
  ]
}
```

Hard bounds: max 128 chain items. Exceeding, cycles, or ambiguity return errors, never partial success described as complete.

## 5. Write path

### 5.1 Request extension

```go
type WriteKnowledgeRequest struct {
    // Kind, Content, Project, Tags, Metadata, IdempotencyKey, and EvidenceRefs remain unchanged.
    MemoryState           string   `json:"memory_state,omitempty"`
    MemoryConfidence      *float64 `json:"memory_confidence,omitempty"`
    MemoryRelationKind    string   `json:"memory_relation_kind,omitempty"`
    MemoryRelationTargets []string `json:"memory_relation_targets,omitempty"`
}
```

Pointer confidence preserves omitted versus explicit zero.

### 5.2 Validation and locking

Relation-bearing writes need bundle-wide preflight plus path persistence. To prevent two concurrent updates of the same head:

- add a repository-scoped temporal mutation mutex keyed by canonical knowledge root;
- acquire it before bundle load/preflight and hold through atomic write + persisted verification;
- existing per-path lock remains for idempotent same-path serialization;
- lock order is repository temporal lock, then per-path write lock everywhere to avoid deadlock.

Normal writes without temporal fields keep the existing per-path path and do not acquire the repository temporal lock.

### 5.3 Defaults

- omitted state = approved;
- approved relation write validates and immediately affects current view;
- proposed requires confidence and at least one `evidence_ref`;
- proposed relation metadata is stored but inactive;
- declined cannot be created directly through durable write; it only results from review;
- relation fields omitted means no relation.

### 5.4 Idempotency

New fields are included in `writeKnowledgePayload` and `payload_hash`. Retrying the same idempotency key and identical temporal payload reuses the concept; changing state, confidence, or relation under the same key produces existing `idempotency_conflict`.

## 6. Review mutation

### 6.1 Service API

```go
type ReviewMemoryRequest struct {
    Ref           string `json:"ref"`
    Action        string `json:"action"`
    ExpectedState string `json:"expected_state"`
}

type ReviewMemoryResult struct {
    Ref           string `json:"ref"`
    PreviousState string `json:"previous_state"`
    CurrentState  string `json:"current_state"`
    ConceptPath   string `json:"concept_path"`
}
```

Operation: `memory_review`; mutating=true.

### 6.2 State transitions

| Current | approve | decline | undo |
|---|---|---|---|
| proposed | approved | declined | invalid |
| approved | invalid | invalid | proposed only when no approved memory updates this concept |
| declined | invalid | invalid | proposed |

Repeated mutation is not silently idempotent because the caller must present exact `expected_state`; after a successful transition a retry with the old expected state receives `memory_state_conflict`. This makes outcomes observable and prevents accidental duplicate review timestamps.

### 6.3 Apply algorithm

1. Resolve knowledge root and acquire repository temporal mutation lock.
2. Load complete bundle and registry.
3. Resolve `ref`; accept only durable types (`note`, `event`, `feedback`). `Generated.By=okf-mcp` is expected for MCP durable writes and is not a rejection criterion; auto-indexed code/source concepts are rejected by type.
4. Parse current state; compare exactly with `expected_state`.
5. Validate transition.
6. On approve, revalidate active relation against the fresh bundle; proposed competing updates may now fail `target_not_current` or `ambiguous_update_head`.
7. Save the original file bytes, set state, remove confidence after approve/decline, and write the latest bounded review record using an injected clock.
8. Serialize to the same file through a dedicated atomic-replace helper; parse back and verify state/ref/review record.
9. On any post-rename failure, atomically restore the saved original bytes and verify the restored parse. Never call the new-file rollback helper that deletes the target.

### 6.4 Idempotency after review

Review changes `memory_state`, confidence, and `memory_review` but does not rewrite the creation-time `payload_hash` or `idempotency_key`. A later retry of the original write request with the same key still resolves the existing concept and returns its current review state plus stable handle; it does not recreate or revert it. A different creation payload under the same key still returns `idempotency_conflict`. Tests lock this behavior for approved, declined, and undone memories.

## 7. Query and context

### 7.1 QueryRequest extension

```go
type QueryRequest struct {
    // Query, filters, limits, tracing, grouping, and memory-check fields remain unchanged.
    MemoryView        string   `json:"memory_view,omitempty"`
    Refs              []string `json:"refs,omitempty"`
    MemoryReviewQueue bool     `json:"memory_review_queue,omitempty"`
}
```

Validation matrix:

| Mode | query | refs | output |
|---|---|---|---|
| omitted/current | required | forbidden | existing QueryResult shape; current approved durable memories plus unaffected concepts |
| all | required | forbidden | existing QueryResult shape with additive temporal annotations for durable hits |
| history | empty | exactly 1 | MemoryHistoryResult dedicated envelope |
| review queue | empty | forbidden | MemoryReviewQueueResult dedicated envelope |

Mutually exclusive modes are rejected.

### 7.2 Current filtering stage

For omitted/current view, temporal projection runs after bundle load but before structured filters/scoring. This prevents excluded history/proposals/declines from consuming TopK. `all` bypasses visibility filtering and adds temporal annotations. Repositories with no temporal fields take a compatibility fast path and preserve the existing result bytes. Warnings are returned for malformed inactive metadata; cycles/ambiguity cause `temporal_graph_invalid` instead of silently selecting a head.

### 7.3 Review queue

Only durable proposed concepts, ordered:

1. confidence descending;
2. `Generated.At` ascending (oldest first; missing timestamp sorts last);
3. okf_id ascending.

Fields: okf_id, title, type, confidence, evidence_refs, relation, created_at. Content body is excluded; caller follows with existing `okf_context refs` when needed. Limit defaults 20, max 100.

### 7.4 ContextRequest extension

Add `MemoryView string`. Allowed values `current|all`; omitted means current. `history` is rejected because history has its own Query mode and shape. A no-temporal-fields fast path preserves old output bytes. For query-generated hits, current filters before scoring; all includes every state with annotations. Explicit refs remain explicit: if a caller asks for a historical/proposed/declined ref, context returns it and marks `memory_state/current` in item metadata rather than hiding it.

### 7.5 Temporal annotations

When temporal projection is active, `QueryHit` and `ContextItem` gain additive optional fields:

```go
MemoryState        string `json:"memory_state,omitempty"`
MemoryCurrent      *bool  `json:"memory_current,omitempty"`
MemoryRelationKind string `json:"memory_relation_kind,omitempty"`
MemoryTargets      []string `json:"memory_relation_targets,omitempty"`
```

The pointer boolean preserves `false` for historical/non-current results while omitting the field entirely on the legacy-data compatibility fast path. `all` and explicit-ref context populate these fields; current results may include `memory_state=approved` and `memory_current=true`. Bodies are never added to Query results.

## 8. CLI and MCP

### 8.1 CLI

```text
okf tool query -q "current architecture decision" --memory-view current
okf tool query --memory-view history --refs okf_xxx
okf tool query --memory-review-queue --limit 20
okf tool context -q "current architecture decision" --memory-view current
okf tool memory-review --ref okf_xxx --action approve --expected-state proposed
```

CLI flags use hyphens. JSON/MCP fields use underscores.

### 8.2 MCP

- Extend modern and legacy schemas identically.
- Add `okf_memory_review` to both eras as a service-backed mutating tool.
- Extend all three MCP durable write schemas (`okf_note`, `okf_log`, `okf_feedback`) with `evidence_refs`; Service already supports the field but note/log currently do not expose it.
- Extend write success with `okf_id` and canonical `ref`, preserving all existing response fields.
- Modern catalog increases from 11 to 12; legacy from 20 to 21. Exact-count E2E tests and docs must be updated intentionally.
- Tool annotations: destructiveHint=true for decline, but MCP annotations are per tool rather than per action; therefore `okf_memory_review` is marked mutating/destructive, non-idempotent, closed-world.
- Agent Skill: inferred knowledge uses proposed write; reviews require explicit user instruction. Agents never auto-approve their own proposals.

## 9. Validation and errors

New error codes:

- `invalid_memory_state`
- `invalid_memory_relation`
- `memory_ref_not_found`
- `memory_relation_cross_project`
- `memory_relation_cycle`
- `ambiguous_update_head`
- `target_not_current`
- `memory_state_conflict`
- `invalid_review_transition`
- `memory_has_approved_updater`
- `temporal_history_too_deep`

Errors include stable refs and safe bundle-relative paths only; no body, credentials, or absolute paths.

Strict lint validates syntax and whole-bundle topology. Default lint reports malformed/cyclic/ambiguous temporal metadata as warnings to preserve compatibility; `--strict` fails.

## 10. Performance

- Build projection: O(n + e), with e ≤ 8n by field bounds.
- Query current view: one projection build per request, no persistent cache.
- History: O(n + e), traversal ≤128.
- Review queue: O(n log n), bounded output 100.
- Write/review relation preflight: O(n + e) under repository mutation lock.
- Benchmark 10,000 concepts for current query, history, queue, approved update, and competing approval. Record ns/op, B/op, allocs/op; set hard gates only after repeatable baseline.

## 11. Test strategy

- pure accessor/property tests for CustomFields round-trip;
- golden topology tests: linear, extends, inactive proposed, decline, cycle, fork, dangling, cross-project;
- byte parity for default query/context/write;
- CAS concurrency and race tests;
- parser/serializer round-trip;
- CLI schema/envelope tests;
- modern/legacy MCP schema parity and exact catalog counts;
- real Codex flow: query current → inspect proposal → user-authorized approve → query current/history;
- targeted mutants for six high-risk invariants;
- full GAUNTLET and conformance matrix.
