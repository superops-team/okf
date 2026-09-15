# Design: Governed Agent Memory Enhancement

## 1. Design principles

1. **Core Concept struct immutable**: governance/code_refs in existing `CustomFields`; typed access via `pkg/memorymeta`. No second field set.
2. **Default behavior unchanged**: no new params → byte/semantic identical to current.
3. **Advisory before blocking**: memory_check read-only; hold warning-only.
4. **Local only**: no external LLM; BM25 built per-call on bounded durable set.
5. **Lexical for_path**: no FS access for matching; stale-refs is only FS scanner and fails closed.
6. **Deterministic**: all filtering/sorting/tokenization deterministic.
7. **No ghost commands**: all entry points verified against current `okf tool` CLI and MCP tools.
8. **No pseudo-audit**: read-only check results not persisted as audit; caller uses per-call.

## 2. Architecture

```
Concept (existing, UNMODIFIED)
├── Type, Title, Description, Tags, Sources, Generated, Verified, Status, StaleAfter [existing]
├── okf_id [existing Stable ID]
└── CustomFields map[string]any [existing extension mechanism]
    ├── governance  → accessed via memorymeta.Governance(concept)
    └── code_refs   → accessed via memorymeta.CodeRefs(concept)

pkg/memorymeta (NEW, typed accessor package)
├── Governance(c) (level, warning, error) — normalize, validate, default=context
├── SetGovernance(c, level) — write to CustomFields
├── CodeRefs(c) ([]string, error) — normalize paths, validate globs/bounds
├── SetCodeRefs(c, refs) — write to CustomFields
├── Validate(c, strict) []string — governance/code_refs validation
└── CheckMemory(content, types, project, tag) MemoryCheckResult — BM25+Jaccard (read-only)

Tool surface (existing, EXTENDED with optional params)
├── okf tool manifest  --for-path <path> --governance <filter> --mode summary|hit|full --max-tokens N
├── okf tool query     -q <content> --type <single> --for-path <path> --governance <filter> --memory-check
├── okf tool context   -q <query> | --refs <ref1,ref2>  --budget-tokens N
├── okf tool status/init/refresh (unchanged)
├── okf lint --strict (validation, existing CLI)
└── MCP okf_note/okf_log/okf_feedback (unchanged; always write; no CLI)

MCP (both modern and legacy eras)
├── okf_manifest: gains for_path, governance, mode, max_tokens
├── okf_query: gains for_path, governance, memory_check
├── okf_context: gains refs
└── No new MCP tools in v1

Agent Skill (W01-W07 extension)
├── W01 (status): okf tool manifest --mode summary
├── W02 (mutation consent): check holds via --for-path; SHOULD ask user (advisory)
├── W03 (discovery): --for-path primary mechanism
└── W06 (persistence): first okf tool query --memory-check, then explicit MCP write
```

## 3. Governance semantics

### 3.1 Storage

`governance` in `Concept.CustomFields["governance"]`. Accessed via `memorymeta.Governance(concept)`.

### 3.2 Values and defaults

| Condition | Effective governance |
|---|---|
| Explicit `governance: constraint/hold/context` | normalized lowercase |
| No `governance` field | `context` (ALL concepts, no path inference) |
| Unknown value | non-strict: `context` + warning; strict: error |

**No path-based inference.** `convention/` does NOT auto-default. Minimal surprise/backward compat.

### 3.3 No inheritance

Per-concept only. No parent-directory inheritance.

### 3.4 Filtering and sorting

- `--governance constraint,hold`: filter by effective governance.
- **Default sort unchanged** without `--for-path` or `--governance`: existing manifest order.
- **Sort activates only** with `--for-path` or `--governance`:
  - `--for-path` without `--governance`: hold → constraint → context, then original order within each level.
  - `--governance <filter>`: sort only the filtered levels in hold→constraint→context order, then original order.
- `for_path` results include `governance_warning: true` when any result has `hold`.

### 3.5 Hold is advisory (SHOULD, not MUST)

- OKF server/tools never block writes or edits.
- `hold` concepts surfaced with `governance_warning`.
- Agent Skill W02: **SHOULD** request user confirmation before modifying hold-governed code. Not MUST (server cannot enforce; agent may be non-interactive).

## 4. Code-to-knowledge binding (code_refs + for_path)

### 4.1 Storage

`code_refs` in `Concept.CustomFields["code_refs"]`. Accessed via `memorymeta.CodeRefs(concept)`.

### 4.2 Path syntax and bounds

- Repo-relative only. No absolute. No `..` escaping root. No NUL. **Backslash rejected** (ambiguity; forward slash canonical on all platforms).
- Glob: `*` (single segment, no `/`), `**` (recursive), `?` (single char), `[abc]` (char class).
- **Bounds**:
  - max 16 patterns per concept
  - max 256 bytes per pattern
  - max 32 path segments per pattern
  - max 2 `**` operators per pattern
  - each `**` matches max 8 segments
- for_path input bounds: max 1024 bytes, max 32 segments.

### 4.3 for_path matching (lexical, no FS)

```
okf tool manifest --for-path pkg/mcp/server.go
```

- Input canonicalized: repo-relative, forward slashes, `.`/`..` resolved, trailing slash removed.
- Reject: absolute, `..` escaping root, NUL, backslash.
- **No EvalSymlinks, no file-existence check**. Supports not-yet-created/renamed targets.
- Matching: for each concept, for each pattern, test against canonicalized input.
- `**` recursion bounded to 8 segments; patterns with >32 segments or >2 `**` rejected at validation.
- Complexity: O(concepts × patterns × path_length). No FS scan.
- Response includes `matched_code_ref: <pattern>`.

### 4.4 stale-refs (FS scanner, fail-closed)

`okf tool manifest --stale-refs` scans FS for concepts whose code_refs match no existing files.

- **Only operation** accessing FS for code_refs.
- **Fail closed**: symlink escapes repo root OR directory unreadable → `incomplete: true` + `warnings: [...]`. Never silently returns empty.
- Scan bound: max 50,000 **entries** (not patterns). Exceeded → `incomplete: true` + warning.
- Advisory only: no auto-deletion or auto-fix.

### 4.5 Relationship to existing fields

| Field | Meaning | code_refs relationship |
|---|---|---|
| `resource` | What resource this knowledge describes | Different: code_refs = what code this knowledge governs |
| `source_path`/`file_path` | Concept's own file path | Different: code_refs = target code, not concept's own file |
| `code_file` concepts | Auto-generated concepts for source files | code_refs for domain concepts governing code; does not duplicate code_file paths |

## 5. Search-before-write (read-only memory_check via okf_query)

### 5.1 Design decision

- **No `allow_duplicate`** (zombie when advisory never blocks).
- **No `possible_conflict`** (cannot infer semantic conflict without LLM; false capability).
- **No audit trace persistence** (read-only result; client-supplied metadata untrusted, no audit value).
- **Approach**: extend `okf tool query -q <content> --memory-check`. Read-only. Returns dedicated `MemoryCheckResult` envelope (skips normal Query ranking output). Agent Skill W06: check-then-explicit-write via MCP okf_note etc.

### 5.2 QueryRequest usage

Uses existing `QueryRequest` fields: `Query` (required when memory_check=true), `Types` (default note,event,feedback), `Project`, `Tag`. No new `content`/`title` fields.

### 5.3 Candidate source

- **Default: durable types only** (note, event, feedback). Avoids code_file concept pollution.
- If user explicitly passes `--type X` (CLI) or `type: "X"` (MCP), candidate set is limited to that single existing type.
- **No new `--types` flag for query CLI** (current CLI query only has `--type` singular). MCP query reuses existing `type` field (singular); no new `types` schema.
- Candidate set bounded: max 1000 durable concepts; if exceeded, warning + truncated candidate set.

### 5.4 BM25 per-call build

- Service struct has **no BM25 cache field** (verified). BM25 index is **built per call** on the bounded durable candidate set.
- Budget: 1000 durable concepts × average content; benchmark in implementation phase.
- Flow: `NewBM25()` → Add(candidate title+body) → `Finalize()` → `Search(query, 10)` → top-10 candidates.

### 5.5 Jaccard re-ranking (complete definition)

**Tokenization**:
- Unicode lowercase all runes.
- Word characters: letters, digits, `_`, `-` (code identifiers).
- Chinese (Han) characters: each character is its own token.
- Other punctuation/whitespace: token boundary.
- Tokens length ≥ 1 (single Chinese chars count; ASCII single chars excluded? No—include all ≥1 for code identifiers like `x`).

**Jaccard**:
- Set A = tokens(query), Set B = tokens(candidate title + first 500 chars of body).
- Jaccard = |A∩B| / |A∪B|.
- Empty set → Jaccard = 0.
- Range [0, 1].

**Threshold**:
- Candidate default: 0.20 (pilot-validated).
- Golden set ≥40 calibrates final default in implementation.
- Configurable via `--dup-threshold`.

**Tie-break**: deterministic by Jaccard desc, then okf_id asc.

### 5.6 Classification

- Jaccard >= threshold → `possible_duplicate` (candidates: top-3 with okf_id, ref, jaccard_score).
- Jaccard < threshold → `no_similar`.
- No `possible_conflict`.

### 5.7 MemoryCheckResult envelope

```json
{
  "schema_version": "1.0",
  "operation": "memory_check",
  "ok": true,
  "mutating": false,
  "result": {
    "status": "possible_duplicate",
    "threshold": 0.20,
    "candidates": [
      {"okf_id": "okf_abc...", "ref": "notes/...", "jaccard_score": 0.35}
    ],
    "candidate_types": ["note", "event", "feedback"],
    "candidate_count": 5
  }
}
```

- Dedicated envelope; does NOT mix with normal Query ranking results.
- `memory_check=true` requires non-empty `query`; otherwise error.

### 5.8 Idempotency and concurrency

- memory_check is read-only; no idempotency key needed.
- Same query → same result (deterministic BM25 + Jaccard).
- Concurrent checks: read-only, no interference.
- Two concurrent writes: each advisory reflects state at check time; neither guarantees seeing the other. Documented.

## 6. Progressive disclosure

### 6.1 Manifest modes (real measurement on 329 concepts)

| Mode | Fields | Tokens (329 concepts, JSON bytes/4) | Tokens/concept |
|---|---|---:|---:|
| `full` (default) | all existing ManifestItem fields | 36,479 | 110.9 |
| `hit` | summary + tags, code_refs, status, stale_after | 16,311 | 49.6 |
| `summary` | okf_id, title, type, governance, 1-line desc (≤80 chars) | 14,337 | 43.6 |

- Numbers are corpus measurements (references.md), not per-call behavior requirements.
- Spec requires: field shape correctness + reproducible measurement.
- Success criterion: 329-concept corpus total token reduction summary≥50%, hit≥40% (regression fixture).

### 6.2 Token estimate definition

- Per-item estimate = `ceil(go_encoding_json_bytes(item) / 4)`.
- Go `encoding/json` default serialization: stable struct field order (struct definition order), **HTML escaping enabled** (Go default: `<`, `>`, `&` escaped as `\u003c`, `\u003e`, `\u0026`). No custom `SetEscapeHTML(false)`.
- **Budget accumulates item estimates only**; response wrapper/envelope overhead excluded.
- Response includes `estimated_item_tokens` sum for returned items.
- Distinct from existing `ManifestItem.EstimatedTokens` which is `file_bytes/4` (source file estimate).
- Implementation fixture re-measures with identical Go `encoding/json` default encoding; pilot temporary measurement used manual JSON and is marked for re-measurement.

### 6.3 ID parity guarantee (not Recall@5)

Manifest listing has no query ranking. Therefore:
- Without `--max-tokens`: summary/hit/full return **identical concept ID sets and order** (100% parity).
- With `--for-path` filter: all three modes return identical filtered ID sets.
- Retrieval Recall not affected by projection mode.

### 6.4 max_tokens pipeline

**Order**: filter → stable sort → offset → limit → token budget

1. Filter by `--for-path`/`--governance` if specified.
2. Sort: default order (no new params) OR governance sort (with new params).
3. Apply `--offset`.
4. Apply `--limit` (existing).
5. Apply `--max-tokens`: accumulate items until budget exceeded; do NOT include partial item.

**Response fields**:
- `items`: returned concepts.
- `next_offset`: index in the filtered+sorted list immediately after the last returned item.
- `omitted_count`: object with `budget_omitted` (items skipped due to token budget) and `total_remaining` (items after offset+limit not returned).
- `truncated: bool`.
- `estimated_item_tokens`: sum of returned item estimates.

**Edge case: budget < first item**:
- Return `budget_too_small` error with `min_required_tokens` = token estimate of first eligible item (dynamic, not hardcoded).
- No items returned; `next_offset` does not advance (no infinite loop).

### 6.5 Follow-up via okf_context refs

Add optional `refs` parameter to existing `okf tool context` (and MCP `okf_context`, both eras):
- `query` OR `refs` at least one required.
- `refs`: comma-separated stable refs (okf_id or okf://concept/<id>).
- Resolves each ref to concept path, then reads concept body.
- Subject to `--budget-tokens`.
- Uses existing `okf identity resolve` logic internally for ref→path mapping.
- NOT a new tool; extends existing ContextRequest.

### 6.6 Integration with W01-W07

- W01: `okf tool manifest --mode summary`.
- W03: `okf tool manifest --for-path <path> --mode hit`.
- W04: `okf tool context --refs <okf_id>` for full body of selected concepts.- W06: `okf tool query -q <content> --memory-check`, then MCP okf_note.

## 7. Data model (CustomFields, no Concept changes)

### 7.1 pkg/memorymeta (NEW package)

```go
package memorymeta

type GovernanceLevel string
const (
    GovernanceConstraint GovernanceLevel = "constraint"
    GovernanceHold       GovernanceLevel = "hold"
    GovernanceContext    GovernanceLevel = "context"
)

type MemoryCheckResult struct {
    Status         string             `json:"status"` // no_similar | possible_duplicate
    Threshold      float64            `json:"threshold"`
    Candidates     []MemoryCandidate  `json:"candidates"`
    CandidateTypes []string           `json:"candidate_types"`
    CandidateCount int                `json:"candidate_count"`
}

type MemoryCandidate struct {
    OKFID        string  `json:"okf_id"`
    Ref          string  `json:"ref"`
    JaccardScore float64 `json:"jaccard_score"`
}
```

- No second set of fields. Parser/query adapters call accessors.
- CustomFields preserved for unknown keys.

### 7.2 Frontmatter examples

```yaml
---
type: Decision
title: Zero external dependencies
governance: constraint
code_refs:
  - go.mod
  - pkg/**/*.go
---
```

## 8. Security and safety

- `hold` = advisory warning, not enforced block.
- `for_path` does not read file contents; lexical only.
- `memory_check` is read-only; does not modify concepts.
- `stale-refs` fails closed on symlink escape/unreadable.
- No audit trace persistence; no client-supplied metadata stored as audit.
- No new filesystem writes beyond the concept being written (via existing MCP tools).

## 9. Performance

- Governance filtering: O(n) in-memory.
- `for_path`: O(n × p × path_length), no FS. p ≤ 16 patterns/concept.
- `memory_check`: BM25 build on ≤1000 durable concepts + top-10 Jaccard. Per-call build (no cache).
- Progressive disclosure: reduces output size; no additional computation.
- `stale-refs`: FS scan, bounded 50,000 entries, fail-closed.
