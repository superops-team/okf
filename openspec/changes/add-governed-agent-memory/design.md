# Design: Governed Agent Memory Enhancement

## 1. Design principles

1. **Core Concept struct immutable**: governance/code_refs stored in existing `CustomFields`; typed access via `pkg/memorymeta`. No second field set.
2. **Default behavior unchanged**: no new params → byte/semantic identical to current behavior.
3. **Advisory before blocking**: search-before-write is read-only; hold is warning-only.
4. **Local only**: no external LLM; reuse existing BM25 for candidate generation.
5. **Lexical for_path**: no FS access for matching; stale-refs is the only FS scanner and fails closed.
6. **Deterministic**: all filtering/sorting deterministic; no probabilistic behavior.
7. **No ghost commands**: all entry points verified against current `okf tool` CLI and MCP tools.

## 2. Architecture

```
Concept (existing, UNMODIFIED)
├── Type, Title, Description, Tags, Sources, Generated, Verified, Status, StaleAfter [existing]
├── okf_id [existing Stable ID]
└── CustomFields map[string]any [existing extension mechanism]
    ├── governance  → accessed via memorymeta.Governance(concept)
    └── code_refs   → accessed via memorymeta.CodeRefs(concept)

pkg/memorymeta (NEW, typed accessor package)
├── Governance(c) (GovernanceLevel, error) — normalize, validate, default=context
├── SetGovernance(c, level) — write to CustomFields
├── CodeRefs(c) ([]string, error) — normalize paths, validate globs
├── SetCodeRefs(c, refs) — write to CustomFields
└── Validate(c, strict) — governance/code_refs validation

Tool surface (existing, EXTENDED with optional params)
├── okf tool manifest  --for_path <path> --governance <filter> --mode summary|hit|full --max-tokens N
├── okf tool query     --for_path <path> --governance <filter> --memory-check true
├── okf tool context   (unchanged)
├── okf tool status/init/refresh (unchanged)
└── okf_note/okf_log/okf_feedback (unchanged; always write; advisory via query memory_check)

MCP (both modern and legacy eras)
├── okf_manifest: gains for_path, governance, mode, max_tokens params
├── okf_query: gains for_path, governance, memory_check params
└── No new MCP tools in v1

Agent Skill (W01-W07 extension)
├── W01 (status): okf tool manifest --mode summary (fast overview)
├── W02 (mutation consent): check holds via --for_path before editing; ask user
├── W03 (discovery): --for_path primary mechanism for code-related knowledge
└── W06 (persistence): first okf tool query --memory-check, then explicit write
```

## 3. Governance semantics

### 3.1 Storage

`governance` is a frontmatter field stored in `Concept.CustomFields["governance"]`. Accessed via `memorymeta.Governance(concept)`.

### 3.2 Values and defaults

| Condition | Effective governance |
|---|---|
| Explicit `governance: constraint/hold/context` | normalized lowercase value |
| No `governance` field | `context` (default for ALL concepts) |
| Unknown value (e.g. `weird`) | non-strict: `context` + warning; strict: error |

**No path-based inference**. `convention/` concepts do NOT auto-default to constraint. This avoids mislabeling and respects minimal surprise/backward compatibility. Templates/docs may suggest explicit `governance: constraint` for convention docs.

### 3.3 No inheritance

Governance is per-concept only. No parent-directory inheritance. No conflict resolution needed (each concept has exactly one effective level).

### 3.4 Filtering and sorting

- `--governance constraint,hold`: filter by effective governance.
- **Default sort unchanged** when no `--for_path` or `--governance` is used: existing manifest order preserved (byte/semantic compatible).
- **Governance sort activates only** with `--for_path` or `--governance`: order is `hold` → `constraint` → `context`, then original stable order within each level.
- `for_path` results include `governance_warning: true` when any result has `hold` governance.

### 3.5 Hold is advisory, not enforced

- OKF server/tools never block writes or edits based on governance.
- `hold` concepts are surfaced with `governance_warning` in responses.
- Agent Skill W02 instructs: before modifying code governed by a `hold` concept, ask user for confirmation.
- No "fail closed for holds" language anywhere.

### 3.6 Agent consumption

- MCP Skill W02: before modifying code, run `okf tool manifest --for_path <target>` and check for `hold` concepts.
- W03: `--for_path` is primary discovery mechanism.
- `hold` = warning, not block. Agent decides (with user input) whether to proceed.

## 4. Code-to-knowledge binding (code_refs + for_path)

### 4.1 Storage

`code_refs` is a frontmatter list stored in `Concept.CustomFields["code_refs"]`. Accessed via `memorymeta.CodeRefs(concept)`.

### 4.2 Path syntax

- **Repo-relative** paths only. No absolute paths. No `..` traversal. No NUL bytes. No backslash ambiguity (reject `\` in patterns).
- Forward slashes on all platforms (normalized at parse).
- Glob: `*` (single segment, no `/`), `**` (recursive, max 8 levels), `?` (single char), `[abc]` (char class).
- **No EvalSymlinks, no file-existence check** for for_path matching. This supports not-yet-created or renamed target paths.
- Pattern bounds: max 16 patterns/concept, max 256 chars/pattern, max 8 segments/pattern.

### 4.3 for_path matching (lexical, no FS)

```
okf tool manifest --for-path pkg/mcp/server.go
```

- Input path canonicalized: repo-relative, forward slashes, `.` and `..` resolved, trailing slashes removed.
- Reject: absolute paths, paths escaping repo root via `..`, NUL, backslash.
- Matching: for each concept, for each code_refs pattern, test pattern against canonicalized input path.
- `**` recursion bounded to 8 levels; patterns with >8 segments rejected at validation.
- Complexity: O(concepts × patterns × path_length). No FS scan.
- Response includes `matched_code_ref: <pattern>` for transparency.

### 4.4 stale-refs (FS scanner, fail-closed)

`okf tool manifest --stale-refs` scans the filesystem to find concepts whose code_refs match no existing files.

- This is the ONLY operation that accesses the filesystem for code_refs.
- **Fail closed**: if symlink escapes repo root, or directory unreadable, returns `incomplete: true` + `warnings: [...]`. Never silently returns empty (which would mislead as "no stale refs").
- Scan bounds: max 50,000 files scanned; if exceeded, `incomplete: true` + warning.
- Advisory only: no auto-deletion or auto-fix of stale code_refs.

### 4.5 Relationship to existing fields

| Field | Meaning | code_refs relationship |
|---|---|---|
| `resource` | What resource this knowledge describes (URL/file) | Different: code_refs = what code this knowledge governs |
| `source_path`/`file_path` | The concept's own file path | Different: code_refs = target code files, not the concept's own path |
| `code_file` concepts | Auto-generated concepts for source files | code_refs should NOT duplicate code_file concept paths; code_refs is for domain concepts governing code |

## 5. Search-before-write (read-only via okf_query memory_check)

### 5.1 Design decision

**No `allow_duplicate`/override** (zombie when advisory never blocks). **No `possible_conflict`** (cannot infer semantic conflict without LLM; false capability).

**Approach**: extend existing `okf tool query` with `memory_check: true`. Read-only. Returns similar concepts. Agent Skill W06 instructs check-then-write. Write tools always write.

### 5.2 Flow

```
okf tool query --memory-check true --content "<note content>" --kind note
  1. Build query from content (first 1000 chars) + kind + tags
  2. BM25 search → top-10 candidates (candidate generation, raw score not [0,1])
  3. For each candidate: compute normalized token Jaccard [0,1] on (title+description+first 500 chars body)
  4. Classify:
     - Jaccard >= threshold: possible_duplicate (with candidates: okf_id, ref, jaccard_score)
     - Jaccard < threshold: no_similar
  5. Return advisory result (read-only, no write)
```

### 5.3 Threshold calibration

- BM25 raw scores are not [0,1]; used only for top-N candidate generation.
- Jaccard [0,1] used for threshold decision.
- Pilot (n=10, Jaccard only, 329 code_file-centric concepts): TP=0 FP=0 TN=5 FN=5, Recall=0 FPR=0. This shows Jaccard-only has poor recall on code_file-centric KB; BM25 candidate generation is essential.
- Golden set (implementation phase): ≥40 cases with actual note/decision positives, Chinese/English/code identifiers, common-word negatives. Threshold calibrated from confusion matrix.
- Default threshold: 0.20 (initial, to be calibrated). Configurable via `--dup-threshold`.

### 5.4 Idempotency and concurrency

- `memory_check` is read-only; no idempotency key needed.
- Same query → same result (deterministic BM25 + Jaccard).
- Concurrent checks: read-only, no interference.
- Two concurrent writes: each advisory reflects state at check time; neither guarantees seeing the other. Documented explicitly.

### 5.5 Audit trace (bounded CustomFields metadata)

When a write (okf_note etc.) is performed, the caller MAY include `memory_check_result` in `Metadata`:
```json
{
  "memory_check": {
    "checked_at": "2026-09-16T00:00:00Z",
    "candidates": ["okf_abc...", "okf_def..."],
    "decision": "written"
  }
}
```
- Max 3 candidates, each only okf_id + ref + score (no body text).
- Same idempotency_key replay: does NOT re-run check or change checked_at (idempotent write reuses existing concept).
- Bounded: max 256 bytes total for memory_check metadata.

## 6. Progressive disclosure

### 6.1 Manifest modes (real measurements on 329 concepts)

| Mode | Fields | Bytes (329 concepts) | Tokens (JSON bytes/4) | Tokens/concept |
|---|---|---:|---:|---:|
| `full` (default) | all existing ManifestItem fields | 145,916 | 36,479 | 110.9 |
| `hit` | summary + tags, code_refs, status, stale_after | 65,247 | 16,311 | 49.6 |
| `summary` | okf_id, title, type, governance, 1-line desc (≤80 chars) | 57,351 | 14,337 | 43.6 |

- Token estimate = **JSON response bytes / 4**, NOT file_bytes/4. Existing `ManifestItem.EstimatedTokens` uses file_bytes/4 (source file estimate); response budget uses JSON projection bytes/4.
- Reduction: hit −55.3%, summary −60.7% vs full.

### 6.2 ID parity guarantee (not Recall@5)

Manifest listing has no query ranking. Therefore:
- Without `--max-tokens`: summary/hit/full return **identical concept ID sets and order** (100% parity).
- With `--for_path` filter: all three modes return identical filtered ID sets.
- Retrieval Recall is not affected by projection mode (can verify via existing eval regression, but this is not "summary Recall@5").

### 6.3 max_tokens interaction

Pipeline order: **filter → stable sort → offset → limit → token budget**

1. Filter by `--for_path`/`--governance` if specified.
2. Sort: default order (no new params) OR governance sort (with new params).
3. Apply `--offset`.
4. Apply `--limit` (existing).
5. Apply `--max-tokens`: accumulate concepts until budget exceeded; do NOT include partial concept.
6. Response: `items`, `next_offset`, `omitted_count`, `truncated: bool`.

**Edge case: budget < first item**: return `budget_too_small` error with `min_required_tokens` (tokens needed for first item). No infinite loop. Caller must increase budget.

### 6.4 Follow-up for full body

Use existing `okf tool query --ref <okf_id>` or `okf tool resolve --ref <okf_id>`. NOT a non-existent `okf_context --id`.

### 6.5 Integration with W01-W07

- W01 (status): `okf tool manifest --mode summary` — fast overview.
- W03 (discovery): `okf tool manifest --for-path <path> --mode hit` — governing concepts.
- W04 (retrieval): `okf tool query --ref <okf_id>` — full body for selected concepts.
- No new workflow steps; existing W01-W07 gain efficiency.

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

// Governance reads and normalizes the governance field from CustomFields.
// Default: context. Unknown: context + warning (non-strict) or error (strict).
func Governance(c *okf.Concept) (GovernanceLevel, warning string, err error)

// SetGovernance writes governance to CustomFields.
func SetGovernance(c *okf.Concept, level GovernanceLevel) error

// CodeRefs reads, normalizes, and validates code_refs from CustomFields.
func CodeRefs(c *okf.Concept) ([]string, error)

// SetCodeRefs writes code_refs to CustomFields after validation.
func SetCodeRefs(c *okf.Concept, refs []string) error

// Validate checks governance and code_refs fields.
func Validate(c *okf.Concept, strict bool) []string
```

- No second set of fields. Parser/query adapters call these accessors.
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

```yaml
---
type: Note
title: Auth subsystem in migration
governance: hold
code_refs:
  - pkg/auth/*.go
---
```

## 8. Security and safety

- `hold` = advisory warning, not enforced block.
- `for_path` does not read file contents; only lexical path matching.
- `memory_check` is read-only; does not modify existing concepts.
- `stale-refs` fails closed on symlink escape/unreadable (never silently empty).
- Audit trace bounded to 3 candidates × (okf_id + ref + score), no body text.
- No new filesystem writes beyond the concept being written.

## 9. Performance

- Governance filtering: O(n) in-memory, no I/O.
- `for_path` matching: O(n × p × path_length), no FS access. p ≤ 16 patterns/concept.
- `memory_check`: BM25 search (existing index) + Jaccard on top-10. BM25 index build: verify if cached per-Service or rebuilt per call (implementation task).
- Progressive disclosure: reduces output size; no additional computation.
- `stale-refs`: FS scan, bounded to 50,000 files, fail-closed on escape.
