# Release Notes: Temporal Memory Relations and Review

## What's new

OKF now tracks how durable knowledge **evolves over time**, with an auditable
propose → review loop — all without a database, a second index, or any change to
the core `Concept` model. Everything is optional frontmatter (`memory_state`,
`memory_confidence`, `memory_relation`, latest `memory_review`), so existing
repositories keep working exactly as before.

### Temporal memory relations

A durable note, event, or feedback can now declare one relation to another
memory (by stable `okf_id` only):

- **`updates`** — this memory supersedes one earlier memory for default-current
  retrieval, while the earlier one stays on disk in history.
- **`extends`** — this memory enriches one or more memories; all remain current.

Currentness is computed deterministically from approved `updates` edges — there
is no persisted `is_latest` flag to get out of sync.

### Review workflow (proposed quarantine)

Agent- or author-generated candidate knowledge no longer lands as stable fact
immediately. You can now write it as **`proposed`** (with a confidence in `[0,1]`
and evidence refs), then review it explicitly:

- **Approve** a proposal → it becomes `approved` and any valid update relation
  activates.
- **Decline** a proposal → it stays on disk for audit but is excluded from
  current retrieval.
- **Undo** → returns an approved/declined memory to `proposed` and restores its
  original confidence (undo of a memory that something already updates is blocked).

Review uses compare-and-set: you pass the `expected_state` you saw, so stale
reviewers get a conflict instead of silently racing. Agents are instructed to
**propose but never self-approve** their own proposals.

### Read views

Query and context now have three temporal views:

- **`current`** (default once temporal metadata exists) — current approved
  memories only; historical/proposed/declined are hidden.
- **`all`** — explicit audit view that includes historical/proposed/declined
  items with additive temporal annotations.
- **`history`** — for one stable ref, the full update chain (oldest → newest).

Plus a dedicated **review queue**: body-free proposed memories sorted by
confidence (desc), then oldest-first, then `okf_id`.

## CLI additions

```bash
# Default current view, explicit audit view, or one-ref history
okf tool query -q "current architecture decision" --memory-view current
okf tool query -q "..." --memory-view all
okf tool query --memory-view history --refs okf_22222222222222222222222222222222

# Proposed-memory review queue (body-free)
okf tool query --memory-review-queue --limit 20

# Context honors the same current/all views
okf tool context -q "current architecture decision" --memory-view current

# CAS review of a single proposed durable memory
okf tool memory-review --ref okf_22222222222222222222222222222222 \
    --action approve --expected-state proposed
```

## MCP additions

- **New tool `okf_memory_review`** in both eras — the same
  approve/decline/undo compare-and-set mutation, exposed as a service-backed
  mutating tool (annotated destructive/non-idempotent).
- `okf_query` gains `memory_view`, `refs`, and `memory_review_queue`.
- `okf_context` gains `memory_view` (`current`/`all`).
- `okf_note`, `okf_log`, and `okf_feedback` accept `memory_state`,
  `memory_confidence`, `memory_relation_kind`, `memory_relation_targets`, and
  now expose `evidence_refs`.
- Successful durable writes additionally return a canonical `okf_id` and `ref`
  (existing `concept_id`, `concept_path`, and `created` are unchanged).
- Catalog counts move intentionally: **modern 11 → 12**, **legacy 20 → 21**.

## Backward compatibility

- **No breaking change.** All new fields are optional.
- Repositories with **no temporal fields** produce byte/semantic-identical query,
  context, and durable-write results (a compatibility fast path preserves the old
  output bytes).
- Repositories *with* temporal fields default to the safe `current` view when the
  view is omitted; `all` is the explicit audit escape hatch.
- Existing `relation_kind/source/target` code-graph fields and their expansion are
  untouched.
- Decline is a reversible soft state, not deletion; explicitly requested refs stay
  readable.
