## ADDED Requirements

### Requirement: Query returns ranked navigable results
The system SHALL return ranked search results with enough navigation metadata for an agent to open source files directly.

#### Scenario: Symbol-oriented query
- **WHEN** a query matches a code symbol
- **THEN** the result MUST include symbol kind, qualified name when known, source file path, line range when known, score, reason and provenance

#### Scenario: Relation-oriented query
- **WHEN** a query or filter matches code relationships
- **THEN** the result MUST include relation kind, source, target, source location when known and provenance

#### Scenario: Deterministic result ordering
- **WHEN** the same query runs against the same knowledge state
- **THEN** results MUST be ordered deterministically by score, exactness, source preference, file path, line number, title and stable concept path or ID

### Requirement: Context bundles are token-budget-aware
The system SHALL build context bundles that fit within a requested budget and explain omissions.

#### Scenario: Context fits budget
- **WHEN** selected search hits and snippets fit within the requested budget
- **THEN** the context response MUST include all selected items with estimated token usage

#### Scenario: Context exceeds budget
- **WHEN** candidate context exceeds the requested budget
- **THEN** the system MUST truncate lower-ranked items or snippets deterministically and report omitted counts and reasons

#### Scenario: Token estimate is deterministic
- **WHEN** context estimates token usage in V1
- **THEN** it MUST use a deterministic approximation such as `ceil(rune_count / 4)` and report the estimated total

### Requirement: Context bundles include freshness metadata
The system SHALL disclose whether context was built from fresh or stale knowledge.

#### Scenario: Fresh context
- **WHEN** context is built after successful refresh or from a fresh index
- **THEN** the response MUST mark `freshness.stale=false`

#### Scenario: Stale context allowed
- **WHEN** context is requested with stale results allowed and refresh was not explicitly requested
- **THEN** the response MUST mark `freshness.stale=true` and include a warning that results may not reflect HEAD

#### Scenario: Context does not auto-refresh by default
- **WHEN** context runs without an explicit refresh option
- **THEN** it MUST NOT mutate `.okf/knowledge`, `.okf/state.json` or cache files before returning the bundle

### Requirement: Snippets are read on demand
The system SHALL read source snippets from the working tree on demand rather than storing full source code in generated knowledge by default.

#### Scenario: Snippet requested for symbol hit
- **WHEN** a context item has a file path and line range
- **THEN** the system SHOULD include a bounded snippet around that line range unless the budget or include policy excludes snippets

#### Scenario: Source file missing
- **WHEN** a context item references a source file that no longer exists in the working tree
- **THEN** the system MUST omit the snippet, keep the navigable metadata, and include a warning
