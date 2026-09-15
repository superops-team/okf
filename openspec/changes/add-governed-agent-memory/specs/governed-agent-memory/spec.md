# Specification: Governed Agent Memory Enhancement

## Requirement: Governance semantics via CustomFields

OKF SHALL support an optional `governance` frontmatter field (constraint|hold|context), stored in existing CustomFields and accessed via `pkg/memorymeta` typed accessor. The core Concept struct SHALL NOT be modified.

### Scenario S01: Governance field read from CustomFields
- **GIVEN** a concept with `governance: Constraint` in frontmatter
- **WHEN** `memorymeta.Governance(concept)` is called
- **THEN** it returns `constraint` (normalized lowercase)
- **AND** the core Concept struct has no governance field

### Scenario S02: Default governance is context for all concepts
- **GIVEN** a concept with no `governance` field, located anywhere (including convention/)
- **WHEN** effective governance is computed
- **THEN** it defaults to `context`
- **AND** no path-based inference is performed

### Scenario S03: Explicit governance takes effect
- **GIVEN** a concept with `governance: hold`
- **WHEN** effective governance is computed
- **THEN** it returns `hold`
- **AND** concepts with `governance: constraint` return `constraint`

### Scenario S04: Unknown governance value handling
- **GIVEN** a concept with `governance: weird_value`
- **WHEN** validated in non-strict mode
- **THEN** effective governance is `context` and a warning is emitted
- **WHEN** validated in strict mode
- **THEN** validation fails with a clear error

### Scenario S05: Default manifest order unchanged without new params
- **GIVEN** a knowledge base with mixed governance concepts
- **WHEN** `okf tool manifest` is called WITHOUT --for-path or --governance
- **THEN** the concept order is byte/semantic identical to the pre-change manifest order
- **AND** no governance sorting is applied

### Scenario S06: Governance sort activates only with new params
- **GIVEN** concepts with governance context, hold, constraint
- **WHEN** `okf tool manifest --governance constraint,hold` is called
- **THEN** order is hold → constraint → context, then original stable order within each level
- **AND** without --governance, original order is preserved

### Scenario S07: Hold is advisory warning only
- **GIVEN** a hold concept with code_refs matching a path
- **WHEN** `okf tool manifest --for-path pkg/x.go` is called
- **THEN** the response includes `governance_warning: true`
- **AND** no write or edit is blocked by the OKF server
- **AND** the Agent Skill instructs user confirmation, but the server does not enforce

### Scenario S08: Governance filtering
- **GIVEN** a knowledge base with mixed governance concepts
- **WHEN** `okf tool manifest --governance hold` is called
- **THEN** only concepts with effective governance hold are returned
- **AND** `--governance constraint,hold` returns both levels

### Scenario S09: SetGovernance writes to CustomFields
- **GIVEN** a concept with no governance field
- **WHEN** `memorymeta.SetGovernance(concept, GovernanceConstraint)` is called
- **THEN** `concept.CustomFields["governance"]` equals `"constraint"`
- **AND** no other Concept field is modified

## Requirement: Code-to-knowledge binding via CustomFields

OKF SHALL support an optional `code_refs` frontmatter field (list of repo-relative paths/globs), stored in CustomFields and accessed via `pkg/memorymeta`. A `for_path` parameter SHALL match input paths against code_refs using lexical matching only.

### Scenario S10: code_refs read from CustomFields
- **GIVEN** a concept with `code_refs: [pkg/mcp/*.go, cmd/okf/main.go]`
- **WHEN** `memorymeta.CodeRefs(concept)` is called
- **THEN** it returns exactly those two normalized patterns
- **AND** empty/missing code_refs returns empty slice

### Scenario S11: for_path exact match
- **GIVEN** a concept with `code_refs: [pkg/mcp/server.go]`
- **WHEN** `okf tool manifest --for-path pkg/mcp/server.go` is called
- **THEN** the concept is returned with `matched_code_ref: pkg/mcp/server.go`
- **AND** `--for-path pkg/mcp/other.go` does not return it

### Scenario S12: for_path single-segment glob
- **GIVEN** a concept with `code_refs: [pkg/mcp/*.go]`
- **WHEN** `--for-path pkg/mcp/server.go` is called
- **THEN** the concept is returned
- **AND** `--for-path pkg/mcp/subdir/server.go` is NOT returned (single-segment *)

### Scenario S13: for_path recursive glob depth limit
- **GIVEN** a concept with `code_refs: [pkg/**/*.go]`
- **WHEN** `--for-path pkg/a/b/c/d/e/f/g/h/file.go` (8 levels) is called
- **THEN** the concept is returned
- **AND** `--for-path pkg/a/b/c/d/e/f/g/h/i/file.go` (9 levels) is NOT returned

### Scenario S14: for_path path canonicalization and rejection
- **GIVEN** input paths `./pkg/mcp/server.go`, `pkg\\mcp\\server.go`, `/abs/path`, `pkg/../secret/file`
- **WHEN** `--for-path` is called
- **THEN** `./pkg/mcp/server.go` is canonicalized to `pkg/mcp/server.go` and matched
- **AND** backslash paths are rejected with error
- **AND** absolute paths are rejected
- **AND** `..` traversal escaping repo root is rejected
- **AND** no EvalSymlinks or file-existence check is performed

### Scenario S15: for_path supports not-yet-created paths
- **GIVEN** a concept with `code_refs: [pkg/newfeature/*.go]` where no such files exist
- **WHEN** `--for-path pkg/newfeature/widget.go` is called
- **THEN** the concept is returned (lexical match, no FS check)
- **AND** no error about missing files is raised

### Scenario S16: code_refs pattern bounds
- **GIVEN** a concept with 17 code_refs patterns, or a pattern >256 chars, or >8 segments
- **WHEN** `memorymeta.CodeRefs(concept)` validates
- **THEN** validation fails with a clear error in strict mode
- **AND** in non-strict mode, invalid patterns are dropped with a warning

### Scenario S17: stale-refs scans FS and fails closed
- **GIVEN** a concept with `code_refs: [deleted/file.go]`
- **WHEN** `okf tool manifest --stale-refs` is called
- **THEN** the concept is listed with `stale_code_refs: [deleted/file.go]`
- **AND** if a symlink escapes repo root or a directory is unreadable, response has `incomplete: true` + warnings
- **AND** it never silently returns empty when errors occur

### Scenario S18: stale-refs scan bounds
- **GIVEN** a repo with >50,000 files
- **WHEN** `--stale-refs` is called
- **THEN** scan stops at 50,000 files and returns `incomplete: true` + warning
- **AND** no auto-deletion or auto-fix is performed

### Scenario S19: code_refs does not duplicate code_file concepts
- **GIVEN** a domain Decision concept with code_refs and auto-generated code_file concepts
- **WHEN** for_path matches the Decision's code_refs
- **THEN** only the Decision is returned (code_file concepts are separate and not auto-bound)
- **AND** code_refs is for domain concepts governing code, not for code_file concept paths

## Requirement: Search-before-write (read-only memory_check via okf_query)

OKF SHALL extend `okf tool query` with a read-only `memory_check` parameter that returns similar existing concepts using BM25 candidate generation + Jaccard re-ranking. No write blocking, no allow_duplicate, no conflict classification.

### Scenario S20: memory_check returns no_similar for novel content
- **GIVEN** a knowledge base with no similar concepts
- **WHEN** `okf tool query --memory-check true --content "quantum coffee machines"` is called
- **THEN** response has `memory_check: {status: no_similar, candidates: []}`
- **AND** no write is performed (read-only)

### Scenario S21: memory_check returns possible_duplicate
- **GIVEN** an existing note with similar title and body
- **WHEN** `--memory-check true` is called with similar content
- **THEN** response has `memory_check: {status: possible_duplicate, candidates: [{okf_id, ref, jaccard_score}]}`
- **AND** candidates are limited to top-3 with score >= threshold

### Scenario S22: BM25 candidate generation then Jaccard threshold
- **GIVEN** a query and a knowledge base
- **WHEN** memory_check runs
- **THEN** BM25 returns top-10 candidates (raw scores, not [0,1])
- **AND** Jaccard similarity [0,1] is computed on title+description+first-500-chars body
- **AND** classification uses Jaccard threshold, not BM25 raw score

### Scenario S23: No conflict classification
- **GIVEN** two concepts of the same type with similar titles
- **WHEN** memory_check runs
- **THEN** classification is either `no_similar` or `possible_duplicate`
- **AND** no `possible_conflict` status exists (cannot infer semantic conflict without LLM)

### Scenario S24: No allow_duplicate or write blocking
- **GIVEN** a possible_duplicate result
- **WHEN** the agent subsequently calls okf_note
- **THEN** the write proceeds normally (memory_check is advisory, read-only)
- **AND** no `allow_duplicate` parameter exists on write tools

### Scenario S25: memory_check is deterministic and read-only
- **GIVEN** the same query content
- **WHEN** memory_check is called twice
- **THEN** both return identical results
- **AND** no concepts are modified

### Scenario S26: Concurrent memory_check calls
- **GIVEN** two concurrent memory_check calls
- **WHEN** both complete
- **THEN** neither interferes with the other (read-only)
- **AND** two concurrent writes' advisories may not see each other (documented TOCTOU)

### Scenario S27: Audit trace bounded in CustomFields metadata
- **GIVEN** a write with memory_check_result in Metadata
- **WHEN** the concept is persisted
- **THEN** memory_check metadata has max 3 candidates, each only okf_id + ref + score
- **AND** total metadata size ≤ 256 bytes
- **AND** same idempotency_key replay does not re-run check or change checked_at

### Scenario S28: Threshold configurable
- **GIVEN** content with Jaccard 0.15 to an existing concept
- **WHEN** `--memory-check true --dup-threshold 0.10` is called
- **THEN** it is classified as possible_duplicate
- **AND** with `--dup-threshold 0.30` it is classified as no_similar

## Requirement: Progressive disclosure (manifest modes)

OKF SHALL extend `okf tool manifest` with summary/hit/full modes and max_tokens budget, using JSON response bytes/4 for token estimation.

### Scenario S29: Summary mode returns minimal fields
- **GIVEN** a knowledge base
- **WHEN** `okf tool manifest --mode summary` is called
- **THEN** each concept includes only okf_id, title, type, governance, 1-line description (≤80 chars)
- **AND** measured ~44 tokens/concept (JSON bytes/4) on 329-concept corpus

### Scenario S30: Hit mode includes code_refs and tags
- **GIVEN** a concept with code_refs and tags
- **WHEN** `--mode hit` is called
- **THEN** concept includes summary fields plus tags, code_refs, status, stale_after
- **AND** measured ~50 tokens/concept on 329-concept corpus

### Scenario S31: Full mode is default and backward compatible
- **GIVEN** an existing client calling `okf tool manifest` without --mode
- **WHEN** response is parsed
- **THEN** it matches the pre-existing full manifest shape exactly
- **AND** measured ~111 tokens/concept on 329-concept corpus

### Scenario S32: ID parity across modes
- **GIVEN** a knowledge base with concepts
- **WHEN** summary, hit, and full modes are compared (without max_tokens)
- **THEN** all three return identical concept ID sets and order
- **AND** for_path filtering produces identical ID sets across all three modes

### Scenario S33: max_tokens pipeline order
- **GIVEN** concepts with mixed governance
- **WHEN** `--for-path x --governance hold --offset 5 --limit 50 --max-tokens 1000` is called
- **THEN** pipeline order is: filter → sort → offset → limit → token budget
- **AND** response includes next_offset, omitted_count, truncated

### Scenario S34: budget_too_small error prevents infinite loop
- **GIVEN** `--max-tokens 10` where first concept requires 50 tokens
- **WHEN** manifest is called
- **THEN** response is a `budget_too_small` error with `min_required_tokens: 50`
- **AND** no items are returned and next_offset does not advance

### Scenario S35: Token estimate uses JSON bytes/4
- **GIVEN** a manifest response
- **WHEN** token budget is computed
- **THEN** estimate = JSON response bytes / 4
- **AND** not file_bytes/4 (which is the existing ManifestItem.EstimatedTokens for source files)

### Scenario S36: Follow-up via existing query/resolve
- **GIVEN** a summary mode result with okf_id
- **WHEN** agent needs full body
- **THEN** it uses `okf tool query --ref <okf_id>` or `okf tool resolve --ref <okf_id>`
- **AND** no `okf_context --id` interface is referenced (it does not exist)

## Requirement: Backward compatibility and validation

### Scenario S37: Existing concepts without new fields work unchanged
- **GIVEN** a concept with no governance or code_refs
- **WHEN** parsed, validated, and queried
- **THEN** it behaves exactly as before (governance=context, code_refs=empty)
- **AND** no validation errors in default mode

### Scenario S38: CustomFields preserved alongside new fields
- **GIVEN** a concept with both `governance: constraint` and a custom field `my_custom: value`
- **WHEN** parsed and re-serialized
- **THEN** both governance and my_custom are preserved in CustomFields
- **AND** governance is accessed via memorymeta, my_custom remains raw CustomFields

### Scenario S39: Strict validation catches malformed new fields
- **GIVEN** concepts with unknown governance, code_refs with traversal, or invalid glob
- **WHEN** `okf tool lint --strict` is run
- **THEN** all malformed fields are reported with file path
- **AND** exit code is non-zero

### Scenario S40: Real CLI entry points verified
- **GIVEN** the current OKF CLI
- **WHEN** `okf tool manifest --for-path x --mode summary` and `okf tool query --memory-check true` are called
- **THEN** these are valid `okf tool` subcommands (not ghost `okf manifest` commands)
- **AND** MCP tools okf_manifest and okf_query gain the same optional params in both modern and legacy eras
