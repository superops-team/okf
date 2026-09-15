# Specification: Governed Agent Memory Enhancement

## Requirement: Governance semantics

OKF SHALL support an optional `governance` frontmatter field with values `constraint`, `hold`, and `context`, enabling agents to distinguish mandatory guardrails, execution freezes, and informative knowledge.

### Scenario S01: Governance field is parsed and normalized
- **GIVEN** a concept with `governance: Constraint` (mixed case)
- **WHEN** the concept is parsed
- **THEN** effective governance is `constraint` (lowercase)
- **AND** unknown values like `weird` are treated as `context` with a warning

### Scenario S02: Default governance inference
- **GIVEN** a concept under `convention/` with no explicit `governance`
- **WHEN** effective governance is computed
- **THEN** it defaults to `constraint`
- **AND** a concept elsewhere with no explicit `governance` defaults to `context`

### Scenario S03: Explicit governance overrides default
- **GIVEN** a concept under `convention/` with `governance: context`
- **WHEN** effective governance is computed
- **THEN** explicit `context` wins (not the `convention/` default)

### Scenario S04: Governance filtering in manifest
- **GIVEN** a knowledge base with mixed governance concepts
- **WHEN** `okf_manifest --governance hold` is called
- **THEN** only concepts with effective governance `hold` are returned
- **AND** `--governance constraint,hold` returns both levels

### Scenario S05: Governance sorting is stable
- **GIVEN** concepts with governance `context`, `hold`, `constraint`
- **WHEN** manifest is queried without explicit sort
- **THEN** order is `hold` → `constraint` → `context`
- **AND** within each level, existing manifest order is preserved

### Scenario S06: Hold concepts surfaced in for_path results
- **GIVEN** a `hold` concept with `code_refs: pkg/auth/*.go`
- **WHEN** `okf_manifest --for_path pkg/auth/login.go` is called
- **THEN** the `hold` concept appears first in results
- **AND** the response includes a `hold_warning` field

### Scenario S07: Governance validation rejects malformed values
- **GIVEN** a concept with `governance: invalid value with spaces`
- **WHEN** the bundle is validated with strict mode
- **THEN** validation fails with a clear error
- **AND** in non-strict mode, it is treated as `context` with a warning

## Requirement: Code-to-knowledge binding

OKF SHALL support an optional `code_refs` frontmatter field and a `for_path` query parameter, enabling proactive discovery of concepts that govern specific source files.

### Scenario S08: code_refs field is parsed
- **GIVEN** a concept with `code_refs: [pkg/mcp/*.go, cmd/okf/main.go]`
- **WHEN** the concept is parsed
- **THEN** `code_refs` contains exactly those two patterns
- **AND** empty or missing `code_refs` results in an empty list

### Scenario S09: for_path exact match
- **GIVEN** a concept with `code_refs: [pkg/mcp/server.go]`
- **WHEN** `okf_manifest --for_path pkg/mcp/server.go` is called
- **THEN** the concept is returned
- **AND** `--for_path pkg/mcp/other.go` does not return it

### Scenario S10: for_path glob match
- **GIVEN** a concept with `code_refs: [pkg/mcp/*.go]`
- **WHEN** `--for_path pkg/mcp/server.go` is called
- **THEN** the concept is returned
- **AND** `--for_path pkg/mcp/subdir/server.go` is NOT returned (single-segment `*`)

### Scenario S11: for_path recursive glob with depth limit
- **GIVEN** a concept with `code_refs: [pkg/**/*.go]`
- **WHEN** `--for_path pkg/a/b/c/d/e/f/g/h/file.go` (8 levels) is called
- **THEN** the concept is returned
- **AND** `--for_path pkg/a/b/c/d/e/f/g/h/i/file.go` (9 levels) is NOT returned (depth limit)

### Scenario S12: code_refs paths are repo-relative and normalized
- **GIVEN** a concept with `code_refs: [./pkg/mcp/server.go, pkg\\mcp\\server.go]`
- **WHEN** parsed
- **THEN** both are normalized to `pkg/mcp/server.go`
- **AND** absolute paths like `/etc/passwd` are rejected

### Scenario S13: Path traversal and symlink escape rejected
- **GIVEN** a concept with `code_refs: [../secret/file]`
- **WHEN** parsed
- **THEN** validation rejects it
- **AND** symlinks resolving outside the repo root are rejected at match time

### Scenario S14: for_path returns matched pattern for transparency
- **GIVEN** a concept with multiple `code_refs`
- **WHEN** `--for_path` matches one pattern
- **THEN** the response includes `matched_code_ref: <pattern>`
- **AND** the agent can see which pattern triggered the match

### Scenario S15: Stale code_refs detection (advisory)
- **GIVEN** a concept with `code_refs: [deleted/file.go]`
- **WHEN** `okf_manifest --stale-refs` is called
- **THEN** the concept is listed with `stale_code_refs: [deleted/file.go]`
- **AND** this is advisory only; no auto-deletion or auto-fix

### Scenario S16: for_path with governance filter
- **GIVEN** concepts with mixed governance and code_refs
- **WHEN** `okf_manifest --for_path pkg/x.go --governance hold` is called
- **THEN** only `hold` concepts matching the path are returned
- **AND** `constraint` and `context` matches are excluded

## Requirement: Search-before-write (advisory)

OKF SHALL run a similarity check before durable writes (note/log/feedback), returning an advisory duplicate/conflict assessment without blocking the write.

### Scenario S17: Search-before-write returns no_duplicate for novel content
- **GIVEN** a new note with title "completely novel topic xyzzy"
- **WHEN** `okf_note --check-duplicates` is called
- **THEN** the response includes `duplicate_check: {status: no_duplicate, candidates: []}`
- **AND** the write proceeds

### Scenario S18: Search-before-write flags possible_duplicate
- **GIVEN** an existing note with title "MCP server dual era protocol"
- **WHEN** a new note with similar title and body is written
- **THEN** the response includes `duplicate_check: {status: possible_duplicate, candidates: [{okf_id, score: 0.75}]}`
- **AND** the write still proceeds (advisory)

### Scenario S19: Search-before-write flags possible_conflict
- **GIVEN** an existing concept of type `Decision` about auth flow
- **WHEN** a new `Decision` concept with conflicting content and similar title is written
- **THEN** the response includes `duplicate_check: {status: possible_conflict, candidates: [...]}`
- **AND** the conflict reason is provided

### Scenario S20: allow_duplicate override is recorded
- **GIVEN** a possible_duplicate result
- **WHEN** `okf_note --check-duplicates --allow-duplicate` is called
- **THEN** the write proceeds
- **AND** the audit trace records `decision: overridden`

### Scenario S21: Audit trace is persisted on written concept
- **GIVEN** a note written with `--check-duplicates`
- **WHEN** the persisted concept is inspected
- **THEN** its frontmatter contains `search_before_write: {checked_at, candidates, decision}`
- **AND** `checked_at` is an ISO 8601 timestamp

### Scenario S22: Search-before-write is read-only and concurrent-safe
- **GIVEN** two concurrent `okf_note` calls with similar content
- **WHEN** both run search-before-write simultaneously
- **THEN** neither modifies existing concepts
- **AND** both complete without error
- **AND** each audit trace records the state at its check time

### Scenario S23: Thresholds are configurable
- **GIVEN** a note with moderate similarity (score 0.6)
- **WHEN** `okf_note --check-duplicates --dup-threshold 0.5` is called
- **THEN** it is flagged as possible_duplicate
- **AND** with `--dup-threshold 0.8` it is flagged as no_duplicate

### Scenario S24: Search-before-write does not regress write performance
- **GIVEN** a knowledge base with 1000 concepts
- **WHEN** `okf_note --check-duplicates` is called
- **THEN** the duplicate check adds < 50ms to write latency
- **AND** the BM25 index is reused (not rebuilt)

## Requirement: Progressive disclosure

OKF SHALL support summary and hit modes in `okf_manifest`, reducing token consumption while preserving recall through stable `okf_id` references.

### Scenario S25: Summary mode returns minimal fields
- **GIVEN** a knowledge base with concepts
- **WHEN** `okf_manifest --mode summary` is called
- **THEN** each concept includes only `okf_id`, `title`, `type`, `governance`, and one-line `description`
- **AND** estimated tokens per concept ≤ 60

### Scenario S26: Hit mode includes code_refs and tags
- **GIVEN** a concept with `code_refs` and `tags`
- **WHEN** `okf_manifest --mode hit` is called
- **THEN** the concept includes summary fields plus `tags`, `code_refs`, `status`, `stale_after`
- **AND** estimated tokens per concept ≤ 160

### Scenario S27: Full mode remains backward compatible
- **GIVEN** an existing client calling `okf_manifest` without `--mode`
- **WHEN** the response is parsed
- **THEN** it matches the pre-existing full manifest shape
- **AND** no fields are removed or renamed

### Scenario S28: Token budget limits concept count
- **GIVEN** a knowledge base with 500 concepts
- **WHEN** `okf_manifest --mode summary --max-tokens 1000` is called
- **THEN** at most ~16 concepts are returned (1000 / 60)
- **AND** response includes `truncated: true` and `total_concepts: 500`
- **AND** returned concepts follow governance priority sort

### Scenario S29: Recall@K not regressed by summary mode
- **GIVEN** a golden query set with known relevant concepts
- **WHEN** summary mode and full mode are compared
- **THEN** Recall@5 is identical (all concepts are listed, just with less metadata)
- **AND** the agent can follow up with `okf_context --id <okf_id>` for full body

### Scenario S30: for_path works with all manifest modes
- **GIVEN** a concept with `code_refs`
- **WHEN** `okf_manifest --for_path pkg/x.go --mode summary` is called
- **THEN** the concept is returned in summary mode
- **AND** `--mode hit` includes the matched `code_refs` pattern

## Requirement: Backward compatibility and validation

### Scenario S31: Existing concepts without new fields work unchanged
- **GIVEN** a concept with no `governance` or `code_refs`
- **WHEN** parsed, validated, and queried
- **THEN** it behaves exactly as before (governance defaults to `context`, code_refs empty)
- **AND** no validation errors

### Scenario S32: CustomFields preserved alongside new fields
- **GIVEN** a concept with both `governance: constraint` and a custom field `my_custom: value`
- **WHEN** parsed and re-serialized
- **THEN** both `governance` and `my_custom` are preserved
- **AND** `governance` is a formal field, `my_custom` remains in CustomFields

### Scenario S33: OKF v0.2 spec compatibility
- **GIVEN** the OKF v0.2 specification
- **WHEN** `governance` and `code_refs` are added
- **THEN** they are treated as extension fields (spec §7 open family)
- **AND** no core v0.2 field is modified or removed

### Scenario S34: Validation strict mode catches all malformed new fields
- **GIVEN** concepts with malformed `governance`, `code_refs` with traversal, or invalid glob
- **WHEN** `okf lint --strict` is run
- **THEN** all malformed fields are reported with file path and line number
- **AND** exit code is non-zero
