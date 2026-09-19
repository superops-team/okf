# Proposal: Temporal Memory Relations and Review

## Problem

OKF now provides stable `okf_id`, durable note/event/feedback writes, provenance, governance, `code_refs`, memory-check, progressive disclosure, query/context, and dual-era MCP. It still treats durable memories as independent facts:

1. A later decision cannot explicitly say it **updates** an older decision while preserving history.
2. Supporting detail cannot say it **extends** another memory without being mistaken for a replacement.
3. Query and context cannot distinguish the **current** member of an update chain from historical members.
4. Agent-generated candidate knowledge is either written as stable knowledge immediately or kept outside OKF; there is no auditable `proposed → approved/declined` review state.
5. Existing `relation_kind/source/target` fields belong to the generated code graph and cannot safely represent memory evolution.

## Proposed solution

Add a small, Git-native temporal memory layer using optional fields in existing `Concept.CustomFields`, typed accessors in `pkg/memorymeta`, and existing Stable IDs.

### 1. Explicit temporal relations

A durable concept may declare one memory relation:

```yaml
memory_relation:
  kind: updates | extends
  targets:
    - okf_0123456789abcdef0123456789abcdef
```

- `updates`: the new memory supersedes one current memory for default-current retrieval while preserving the target in history.
- `extends`: the new memory enriches one or more memories; all remain current.
- Targets are stable `okf_id` references only. Paths and titles are forbidden as relation identity.
- No automatic `derives` relation in v1. LLM-generated inferences enter the review queue as `proposed` memories instead.
- These fields are unrelated to existing code-graph `relation_kind`, `relation_source`, and `relation_target`.

### 2. Computed current/history view

Currentness is a deterministic projection over concept files, not a persisted `is_latest` flag:

- a valid `updates` edge makes its target historical;
- the updating concept remains current unless another valid update targets it;
- `extends` never changes currentness;
- concepts without temporal metadata remain current;
- query/context default behavior remains unchanged for repositories without temporal fields; when temporal fields exist, omitted view safely means `current`.

The following optional view is added to existing query/context operations:

- omitted / `current` (default): return current approved memories plus all unaffected non-durable/legacy concepts; for a repository without temporal fields this is byte/semantic compatible with the existing behavior;
- `all`: include historical, proposed, and declined concepts for audit, with additive temporal annotations;
- `history`: for explicitly supplied stable `refs`, return deterministic update-chain history including edge metadata.

CLI uses `--memory-view`; MCP/JSON uses `memory_view`.

### 3. Proposed memory review

Durable write requests may add trusted server-validated fields:

```json
{
  "memory_state": "proposed",
  "memory_confidence": 0.72,
  "evidence_refs": ["okf_11111111111111111111111111111111"]
}
```

Review is a dedicated mutating operation with explicit stable refs:

```text
okf tool memory-review --ref okf_22222222222222222222222222222222 --action approve|decline|undo --expected-state proposed
```

MCP exposes `okf_memory_review` with the same contract.

State model:

```text
proposed ──approve──▶ approved
    │                    │
    └──decline──▶ declined
approved/declined ──undo──▶ proposed
```

- No implicit approval.
- Declined memories remain in Git for audit but are excluded from `current` view.
- Review rewrites only the target concept atomically and records bounded review metadata in that concept's CustomFields.
- `expected_state` is required for compare-and-set semantics; stale reviewers receive `memory_state_conflict` and no write occurs.
- The existing core `Status` enum is not extended.

### 4. Write-time relation validation

Existing durable write tools gain optional fields:

- `memory_state`: `approved` (default) or `proposed`;
- `memory_confidence`: required for `proposed`, range `[0,1]`; omitted for approved writes;
- `memory_relation_kind`: `updates|extends`;
- `memory_relation_targets`: stable refs;
- existing `evidence_refs` becomes available on all three MCP durable write tools (it already exists on `WriteKnowledgeRequest`; today only feedback exposes it);
- durable write success adds canonical `okf_id` and `ref` while preserving existing `concept_id`, `concept_path`, and `created` fields.

Validation rules:

- relation target must resolve in the same loaded knowledge bundle;
- self-edge, duplicate target, dangling target, invalid Stable ID, and cross-project target are rejected;
- `updates` has exactly one target in v1;
- `extends` has 1–8 targets;
- target must be a durable type (`note`, `event`, `feedback`);
- updating an already historical target is rejected unless the caller explicitly targets the current chain head;
- `proposed` writes do not affect currentness until approved;
- an approved update cannot target a proposed or declined memory.

### 5. Review queue and history inspection

Extend existing query operation with two opt-in modes:

```text
okf tool query --memory-review-queue
okf tool query -q "architecture decision" --memory-view current
okf tool query --memory-view history --refs okf_22222222222222222222222222222222
```

Because the current CLI requires non-empty `-q`, review queue and ref-history are implemented as explicit QueryRequest modes whose validation supersedes normal query-required validation:

- `memory_review_queue=true`: query optional; returns proposed concepts sorted by confidence descending, then created timestamp ascending, then `okf_id` ascending; limit max 100.
- `memory_view=history`: exactly one ref required in v1; query ignored and rejected if non-empty to avoid ambiguous semantics.
- normal/current query: non-empty query required.

CLI flags:

- `--memory-view all|current|history`
- `--refs <stable-ref>` (history only, exactly one)
- `--memory-review-queue`

MCP fields use underscores.

## Non-goals

- No graph database, adjacency database, second index, daemon, cloud service, or external LLM.
- No automatic fact extraction, automatic contradiction detection, automatic `derives`, or background dreaming.
- No user/profile system in this change.
- No path/title relation identity; Stable ID only.
- No hard deletion. Decline is reversible and Git-auditable.
- No modification of core `Concept`, `Status`, or OKF v0.2 structs.
- No reuse of generated code-relation fields.
- No recursive arbitrary graph traversal. V1 supports bounded update-chain history only.

## Compatibility

- All fields are optional CustomFields; legacy concepts remain current and approved.
- Normal query/context requests over repositories with no temporal fields remain byte/semantic compatible; omitted `memory_view` is equivalent to `current` once temporal fields exist.
- `memory_view=all` is the explicit audit view that includes historical/proposed/declined items with temporal annotations.
- Existing durable write requests without new fields remain approved and unchanged.
- Existing `relation_*` code metadata and relation expansion remain untouched.
- Parser round-trip preserves all new fields through existing inline CustomFields.

## Real entry points

- Service: extend `WriteKnowledgeRequest`, `QueryRequest`, `ContextRequest`; add `Service.ReviewMemory`.
- CLI: extend `okf tool query`; add `okf tool memory-review` because review is a distinct mutation, not a query or durable capture.
- MCP: extend `okf_query`, `okf_context`, `okf_note`, `okf_log`, `okf_feedback`; add modern+legacy `okf_memory_review`.
- Metadata: extend existing `pkg/memorymeta`; do not create a parallel relation package.
- Validation: `okf lint --strict` validates temporal metadata and review state.

## Success criteria

- Current-head selection: 100% correct on a golden fixture containing linear chains, branching attempts, extends edges, proposed updates, declined updates, and legacy concepts.
- History chain: deterministic and complete up to the hard limit; cycles and ambiguity fail closed.
- Review CAS: zero lost updates under concurrent approve/decline attempts; exactly one succeeds.
- Default query/context behavior over legacy repositories and default durable write behavior are unchanged; repositories using temporal metadata default to the safe current view.
- 10,000-concept current-view benchmark recorded; no persistent cache and no second index.
- Targeted mutants kill edge-kind swap, proposed-current leakage, cycle acceptance, stale expected-state acceptance, declined-current leakage, and unstable chain ordering.
- Full GAUNTLET, modern/legacy MCP E2E, and real Codex read/review flow pass before merge.
