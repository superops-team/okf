# References and feasibility notes

## 1. External inspiration: Supermemory

Source reviewed: `https://github.com/supermemoryai/supermemory` at commit `57b430b5b6a19106a989651f4cde853c05147682` (2026-09-18).

Relevant product concepts:

- documents remain source material while atomic memories carry evolving facts;
- explicit `updates`, `extends`, and `derives` relationships;
- current truth selected without erasing history;
- proposed/inferred memories down-ranked until reviewed;
- approve/decline/undo review loop;
- static/dynamic profile separate from targeted search;
- soft forget and versioned updates;
- MemoryBench checkpointed end-to-end evaluation.

Boundary found in the repository:

- MCP server, SDK/integration packages, Memory Graph UI, docs, and tests are source-visible;
- documentation describes the learning model and temporal vector-graph engine as internally built;
- v1 OKF design therefore adopts auditable product semantics, not unverifiable extraction/model internals.

## 2. Current OKF facts verified from code

| Capability | Current implementation | Design consequence |
|---|---|---|
| Stable identity | `pkg/identity`, `okf_id`, registry duplicate checks | all memory edges use stable IDs |
| Extension fields | `Concept.CustomFields` round-trip | no core struct changes |
| Durable writes | `pkg/tool/write.go`, atomic write/verify/rollback | relation writes extend this path |
| Idempotency | payload hash + deterministic `concept_id` | temporal fields enter payload hash |
| Query | `QueryRequest`, normal query requires non-empty query | special history/queue modes need explicit validation branch |
| Context refs | `ContextRequest.Refs` and Stable ID resolution | explicit historical proposal inspection reuses context |
| Code relations | `relation_kind/source/target` query fields | memory relations need separate namespace |
| Governance | `pkg/memorymeta` CustomFields accessors | temporal accessors belong in same package |
| MCP catalogs | modern 11, legacy 20 | adding review intentionally changes exact counts to 12/21 |
| Core status | fixed OKF enum | proposed/declined cannot reuse core Status |

## 3. Completed feasibility experiments

### Experiment A: CustomFields nested shape — PASS

A temporary Go probe used the real `parser.SerializeConcept` and `parser.ParseConcept` paths with:

- `memory_state: proposed`;
- `memory_confidence: 0.72`;
- nested `memory_relation` with targets array;
- nested `memory_review` with `previous_confidence`;
- unrelated nested `arbitrary` field.

Observed: all values and nested shapes survived, and serialize → parse → serialize was byte-identical (`stable_bytes=true`). No parser production change is required.

### Experiment B: Stable-ID registry baseline — PASS

Existing `pkg/identity` duplicate Stable ID fail-closed test passed. Temporal projection will build one Registry and reuse its existing resolution semantics rather than create a second lookup map with different validation.

### Experiment C: Current code baseline — PASS

Fresh `go test ./pkg/parser ./pkg/memorymeta ./pkg/tool -count=1` passed before implementation. This is the baseline for later default-parity and regression comparison.

## 4. Required implementation experiments

### Experiment D: 10k topology projection

Generate 10,000 durable concepts:

- 7,000 independent approved;
- 1,000 linear update chains of length 3;
- 500 extends edges;
- 500 proposed/declined concepts.

Measure projection and one current query. Use no persistent cache. Record repeat variance before setting a performance ceiling.

### Experiment E: CAS review race

Create one proposed memory; run approve and decline concurrently with the same expected state. Repeat under `-race` at least 100 times.

Acceptance: exactly one success per iteration, one state-conflict, valid parse after every run, no deadlock/data race.

### Experiment F: default behavior parity

Run base-vs-head on fixtures for:

- ordinary `okf tool query`;
- ordinary `okf tool context`;
- ordinary MCP durable note/log/feedback;
- manifest full mode.

Acceptance: no change without new params except additive stable write handle fields explicitly approved by the Spec.

## 4. Rejected alternatives

### Persist `is_latest`

Rejected because currentness would be duplicated across the new and old files; crashes could leave both false or both true. Compute from edges instead.

### Reuse code `relation_*`

Rejected because those fields represent source-code dependencies and are indexed/filterable using different semantics.

### Extend core Status

Rejected by AGENTS.md core-model constraint and because memory review is orthogonal to document lifecycle status.

### Add graph database/index

Rejected as a second fact source and unnecessary for bounded O(n+e) local repositories.

### Automatic derives/contradiction LLM

Rejected in v1: quality cannot be guaranteed locally and it would write inferred truth without a review boundary.

### Append full review history in frontmatter

Rejected as unbounded. Store latest transition only; Git commits are the durable audit history.

### Make approve idempotent

Rejected because a retry would silently mutate review timestamps or hide a stale caller. Required expected state makes repeated actions observable.

## 5. Key safety decisions

- proposed writes require evidence and `memory_confidence`;
- agent cannot auto-approve its own proposal;
- approval reruns fresh topology validation under repository lock;
- bulk review absent in v1;
- decline is reversible soft state, not deletion;
- explicit historical refs remain readable;
- errors never include memory bodies or absolute paths;
- relation/history traversal bounded;
- omitted/current view safely hides historical/proposed/declined durable memories once temporal metadata exists;
- repositories without temporal metadata preserve legacy result bytes; explicit `all` is the audit view.
