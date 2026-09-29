# Spec Review: Temporal Memory Relations and Review

## Review method

Two independent passes were applied after grounding against current `main`, AGENTS.md, `pkg/memorymeta`, `pkg/tool`, CLI, MCP schemas, Stable ID registry, and the existing code relation model.

Severity policy:

- Critical/high/medium findings must be resolved before approval.
- Low findings affecting testability or long-term evolution are fixed in the same revision.
- Historical findings remain documented rather than deleted.

## Round 1 — contract and architecture review

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| R1.1 | critical | Initial recommendation implied default `all`, so proposed/declined memories would still contaminate ordinary retrieval | Omitted view now means safe `current` when temporal fields exist. A no-temporal-data fast path preserves legacy bytes. `all` is explicit audit mode. |
| R1.2 | high | Existing `relation_kind/source/target` already represents generated code relations | Temporal metadata uses nested `memory_relation`; Spec forbids reading or changing code relation fields. |
| R1.3 | high | Core Concept/Status changes would violate AGENTS.md | All fields use CustomFields typed accessors; core structs remain unchanged. |
| R1.4 | high | Proposed write response lacked `okf_id/ref`, so caller could not review the created proposal | Add stable handle fields to successful write response while retaining existing fields. |
| R1.5 | high | Proposed write requires evidence, but current note/log MCP schemas do not expose existing Service `EvidenceRefs` | Extend all three MCP durable write schemas with `evidence_refs`; schema parity tests required. |
| R1.6 | high | Undoing an approved historical node that has an approved updater would break chain validity | Undo is rejected with `memory_has_approved_updater` and dependent Stable ID. |
| R1.7 | medium | Approve removes confidence, so undo could not restore the original proposal exactly | Latest bounded review record stores `previous_confidence`; undo restores it. |
| R1.8 | medium | Concurrent proposals can both target one head and later form a fork | Approval reruns topology preflight under repository mutation lock; only first compatible approval succeeds. |
| R1.9 | medium | Persisted `is_latest` would be a second mutable truth | Currentness is always computed from approved update edges. |
| R1.10 | medium | Automatic derives would require an unverified external LLM path | V1 has no derives generation; inferred knowledge is explicit proposed content only. |
| R1.11 | low | Full review history in frontmatter would grow without bound | Store latest transition only; Git history is the audit log. |

## Round 2 — adversarial implementation review

| ID | Severity | Finding | Resolution |
|---|---|---|---|
| R2.1 | high | Proposal used `confidence` while design used `memory_confidence` | Canonical frontmatter/request field is `memory_confidence`; prose and examples aligned. |
| R2.2 | high | Reusing new-file rollback for review could delete an existing concept after post-rename failure | Review saves original bytes and uses dedicated atomic restore + parse verification; deletion rollback is forbidden. |
| R2.3 | high | Treating all `Generated` concepts as non-reviewable would reject every MCP durable write (`Generated.By=okf-mcp`) | Review eligibility is based on durable type, not Generated metadata; auto code/source concepts are rejected by type. |
| R2.4 | high | Recomputing creation payload hash after review would break original idempotent retries | Review never rewrites `payload_hash`/`idempotency_key`; original retry returns current reviewed state without reverting it. |
| R2.5 | high | One malformed/cyclic component could make all projects unavailable | Projection isolates invalid components by effective project/component; current excludes them with warnings, history for involved ref fails precisely, unrelated data remains available. |
| R2.6 | medium | `all/current` annotations were described but response fields undefined | Added optional `memory_state`, pointer `memory_current`, relation kind, and targets to QueryHit/ContextItem design. |
| R2.7 | medium | Examples used empty/ignored `-q`, which could mask CLI validation requirements | Queue and history examples omit `-q`; validation matrix defines query/refs exactly. |
| R2.8 | medium | State transition table contained unresolved prose about idempotency | Table finalized: approve/decline only from proposed; undo from reviewed states with dependency check; stale retry returns conflict. |
| R2.9 | medium | Project comparison semantics were unspecified | Effective project is normalized `CustomFields.project`; empty↔empty same, empty↔non-empty cross-project. |
| R2.10 | low | Review tool action-dependent destructiveness cannot be expressed in static MCP annotations | Whole tool marked mutating/destructive/non-idempotent; docs explain approve/undo/decline semantics. |
| R2.11 | low | MCP exact tool counts would silently change | Explicitly update modern 11→12 and legacy 20→21 with exact-count E2E gates. |

## Feasibility evidence

- Nested temporal CustomFields round-trip through real parser/serializer: PASS, byte-identical after normalization.
- Existing Stable ID duplicate fail-closed test: PASS.
- Fresh parser/memorymeta/tool baseline: PASS.
- Remaining implementation experiments are explicit Tasks: 10k projection benchmark, 100-iteration CAS race, base-vs-head legacy parity.

## Acceptance review

| Dimension | Result |
|---|---|
| Existing architecture reused | Yes: CustomFields, memorymeta, identity Registry, Service, atomic persistence |
| Second fact source/index | None |
| Core model changes | None |
| Default safety | Current-only once temporal metadata exists |
| Legacy-data compatibility | Explicit fast path and byte-parity tests |
| State machine closed | Yes, including undo dependency and confidence restoration |
| Concurrency | Repository lock + required expected state |
| Error/failure semantics | Defined, bounded, fail-closed per component |
| CLI/MCP names verified | Yes, existing routes inspected; one explicit new subcommand/tool |
| Testability | S01–S44 each mapped to automated test/entry point |
| External dependency | None |
| Automatic inference | Explicitly excluded |

## Decision

Spec is ready for implementation approval. All 22 findings are resolved in the current text. No open questions, ghost commands, automatic inference, graph database, unbounded audit log, or unexplained compatibility gaps remain.
