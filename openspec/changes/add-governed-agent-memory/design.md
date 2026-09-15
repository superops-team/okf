# Design: Governed Agent Memory Enhancement

## 1. Design principles

1. **Additive, backward-compatible**: all new fields optional; existing concepts work unchanged.
2. **No second fact source**: governance and code_refs are concept frontmatter; progressive disclosure reuses Manifest.
3. **Advisory before blocking**: search-before-write returns recommendations, never blocks without explicit override.
4. **Local only**: no external LLM, database, or network; reuse existing BM25/semantic indexes.
5. **Fail closed for holds**: `hold` governance is surfaced prominently; unknown governance values degrade to `context`.
6. **Deterministic**: all filtering/sorting is deterministic and testable; no probabilistic behavior in governance/code_refs.

## 2. Architecture

```
Concept frontmatter (existing + new)
├── type, title, description, tags      [existing]
├── sources, generated, verified         [existing v0.2]
├── status, stale_after                  [existing v0.2]
├── okf_id                               [existing Stable ID]
├── governance                           [NEW: constraint|hold|context]
└── code_refs                            [NEW: []string (paths/globs)]

Tool surface (existing + extended)
├── okf_manifest  --for_path <path> --mode summary|hit|full  [EXTENDED]
├── okf_query     --for_path <path> --governance <filter>    [EXTENDED]
├── okf_context                                              [unchanged]
├── okf_note      --check-duplicates (advisory)              [EXTENDED]
├── okf_log       --check-duplicates (advisory)              [EXTENDED]
└── okf_feedback  --check-duplicates (advisory)              [EXTENDED]

Agent Skill (W01-W07 extension)
└── W02 (mutation consent) gains: check holds for target paths
    W03 (discovery) gains: for_path lookup before editing
```

## 3. Governance semantics

### 3.1 Field definition

```yaml
governance: constraint  # optional; one of: constraint | hold | context
```

### 3.2 Default inference

| Condition | Default governance |
|---|---|
| Explicit `governance: constraint/hold/context` | respected (normalized lowercase) |
| Concept ID under `convention/` | `constraint` |
| All other concepts | `context` |
| Unknown value (e.g. `weird`) | `context` (backward compatible, logged) |

### 3.3 Inheritance and conflict

- Governance is per-concept, not inherited from parent directories (except the `convention/` default).
- If a concept has both explicit `governance` and is under `convention/`, explicit wins.
- No conflict resolution needed: each concept has exactly one effective governance level.

### 3.4 Filtering and sorting

- `okf_manifest --governance constraint|hold|context`: filter by effective governance.
- `okf_query --governance ...`: same filter.
- Default sort: `hold` first, then `constraint`, then `context`; within each level, by existing manifest order (stable).
- `for_path` results: `hold` and `constraint` surfaced before `context`.

### 3.5 Agent consumption

- MCP Skill W02 (mutation consent): before modifying code, agent MUST run `okf_manifest --for_path <target>` and check for `hold` concepts.
- W03 (discovery): `for_path` is the primary discovery mechanism for code-related knowledge.
- `hold` concepts are surfaced as warnings in tool responses; agent should not proceed without human signoff.

## 4. Code-to-knowledge binding (code_refs + for_path)

### 4.1 Field definition

```yaml
code_refs:
  - pkg/mcp/*.go
  - cmd/okf/main.go
  - docs/guides/*.md
```

### 4.2 Path syntax

- **Repo-relative** paths only; no absolute paths, no `..` traversal.
- Forward slashes on all platforms (normalized at parse time).
- Glob syntax: `*` (single segment), `**` (recursive, depth-limited to 8 levels), `?` (single char), `[abc]` (char class).
- Symlinks: resolved at match time; path escape via symlink is rejected (resolved path must remain under repo root).
- ReDoS prevention: glob matching uses Go `filepath.Match` for simple patterns and a bounded recursive matcher for `**` (max 8 levels, max 1000 files per pattern).

### 4.3 for_path matching

```
okf_manifest --for_path pkg/mcp/server.go
```

- Returns concepts where any `code_refs` pattern matches the given path.
- Path is normalized to repo-relative, forward-slash form before matching.
- Matching is deterministic: a pattern either matches or not; no fuzzy matching.
- Results include the matched `code_refs` pattern for transparency.

### 4.4 Rename/delete handling

- `code_refs` are static strings; if a referenced file is renamed, the concept's `code_refs` should be updated (manual or via `okf_refresh`).
- `for_path` does not auto-fix stale `code_refs`; a `--stale-refs` flag on `okf_manifest` can list concepts whose `code_refs` match no existing files (advisory).

### 4.5 Relationship to existing fields

- `resource` field: references the actual resource this knowledge describes (e.g. a URL, a file). `code_refs` is specifically for code files this concept governs. They are complementary, not redundant.
- `file_path`/`source_path`: these are the concept's own file path, not code references. No conflict.

## 5. Search-before-write (advisory)

### 5.1 Flow

```
okf_note --check-duplicates (default true for advisory)
  1. Build query from title + body (first 500 chars)
  2. Run BM25 search over existing concepts (title + description + body)
  3. Compute similarity score for top-5 candidates
  4. Classify:
     - score >= 0.7: possible_duplicate
     - score >= 0.5 AND type conflicts: possible_conflict
     - otherwise: no_duplicate
  5. Return advisory result alongside write result
  6. If allow_duplicate=true, write proceeds regardless
  7. Audit trace: log the check result to the concept's frontmatter (search_before_write: {checked_at, candidates, decision})
```

### 5.2 Thresholds (first version, calibrated from experiment)

- `possible_duplicate`: BM25 score >= 0.7 (normalized 0-1)
- `possible_conflict`: score >= 0.5 AND same type AND title overlap >= 0.4
- Thresholds are configurable via `--dup-threshold` but default to above.

### 5.3 Idempotency and concurrency

- Search-before-write is read-only; it does not modify existing concepts.
- Concurrent writes: each write runs its own check; TOCTOU is acceptable for advisory mode (the audit trace records the state at check time).
- If two concurrent writes create similar concepts, the audit trace on each records the other as a candidate (if visible at check time).

### 5.4 False positive handling

- Advisory mode means false positives are non-blocking; agent reviews candidates and decides.
- `allow_duplicate: true` explicitly overrides; recorded in audit trace.
- Threshold calibration: measured on existing 329-concept knowledge base; title+body Jaccard at 30% yields 0 false positives for novel notes (see references/experiments).

### 5.5 Audit trace

```yaml
search_before_write:
  checked_at: "2026-09-16T00:00:00Z"
  candidates:
    - okf_id: okf_abc123...
      score: 0.75
      reason: possible_duplicate
  decision: written  # written | overridden | skipped
```

## 6. Progressive disclosure

### 6.1 Manifest modes

| Mode | Fields per concept | Est. tokens | Use case |
|---|---|---|---|
| `summary` | id, title, type, governance, description (1 line) | ~50 | Initial scan, W01 status check |
| `hit` | summary + tags, code_refs, status, stale_after | ~150 | W03 discovery, for_path results |
| `full` | all metadata (existing default) | ~300+ | Deep inspection |

### 6.2 Token budget

- `okf_manifest --mode summary --max-tokens 2000`: returns at most N concepts fitting the budget.
- Stable sort: governance priority, then existing manifest order.
- No truncation of individual concepts; if a concept doesn't fit, it's omitted (with `truncated: true` in response).

### 6.3 Recall@K guarantee

- Summary mode includes `okf_id` and `title`, so agent can always follow up with `okf_context --id <okf_id>` for full body.
- Recall@5: measured on existing golden queries; summary mode must not regress vs full mode (because all concepts are still listed, just with less metadata).

### 6.4 Integration with W01-W07

- W01 (status check): `okf_manifest --mode summary` — fast overview.
- W03 (discovery): `okf_manifest --for_path <path> --mode hit` — governing concepts.
- W04 (retrieval): `okf_context --id <okf_id>` — full body for selected concepts.
- No new workflow steps; existing W01-W07 gain efficiency.

## 7. Data model changes

### 7.1 Concept struct additions

```go
type GovernanceLevel string
const (
    GovernanceConstraint GovernanceLevel = "constraint"
    GovernanceHold       GovernanceLevel = "hold"
    GovernanceContext    GovernanceLevel = "context"
)

type Concept struct {
    // ... existing fields ...
    Governance GovernanceLevel `yaml:"governance,omitempty" json:"governance,omitempty"`
    CodeRefs   []string        `yaml:"code_refs,omitempty" json:"code_refs,omitempty"`
}
```

- Formal fields (not CustomFields) for type safety and validation.
- CustomFields still preserved for unknown keys.

### 7.2 Validation

- `governance`: if present, must be one of `constraint|hold|context` (case-insensitive, normalized). Unknown values → warning + treated as `context`.
- `code_refs`: each entry must be non-empty, repo-relative, no `..`, no absolute paths. Glob syntax validated at parse time.

## 8. Security and safety

- `hold` concepts are advisory warnings, not enforced blocks (agent framework may enforce).
- `for_path` does not read file contents; only matches paths.
- Search-before-write does not modify existing concepts.
- No new filesystem writes beyond the concept being written.
- Audit trace is stored in the concept's own frontmatter (no separate log file).

## 9. Performance

- Governance filtering: O(n) over concept list, in-memory, no I/O.
- `for_path` matching: O(n * p) where n=concepts, p=patterns per concept; glob matching O(path length).
- Search-before-write: reuses existing BM25 index (already built); adds one search per write (~300µs per experiment).
- Progressive disclosure: reduces output size, no additional computation.
