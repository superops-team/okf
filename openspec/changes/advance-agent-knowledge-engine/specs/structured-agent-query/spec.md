## ADDED Requirements

### Requirement: Agent query uses structured indexing

The system SHALL make agent-facing query behavior support a concrete minimum set of structured fields instead of relying only on raw full-text matching.

#### Scenario: Query supports code metadata filters

- **WHEN** a tool query includes language, file path, symbol kind, qualified name, or relation kind filters
- **THEN** the system MUST filter candidates using structured metadata fields when present
- **AND** when structured metadata fields are absent, it MAY use parsed generated knowledge sections such as language, path, symbols, and relation tables
- **AND** it MUST NOT rely only on raw full-text contains for those filters

#### Scenario: Relation source and target filters are additive

- **WHEN** relation source or relation target metadata is available in the query model
- **THEN** the system MAY expose relation source and relation target filters as additive fields
- **AND** those filters MUST be covered by golden tests before being exposed through `okf tool query`

#### Scenario: Symbol hits return navigation metadata

- **WHEN** a query matches a code symbol with known source range
- **THEN** the result MUST include symbol kind, qualified name, source file path, start line, end line, and formatted location
- **AND** formatted location MUST be suitable for humans and agents to navigate to the source

#### Scenario: Deterministic ranking is stable across runs

- **WHEN** the same knowledge inputs and query are provided multiple times
- **THEN** result ordering MUST be identical across runs
- **AND** tie-breaks MUST include score, exactness, source preference, source rank, file path, line number, title, and concept path or stable identity

#### Scenario: Legacy single-path ordering is preserved

- **WHEN** the query reads from a single knowledge directory
- **AND** no new structured exactness signal applies
- **THEN** result ordering MUST remain compatible with the previous deterministic ordering
- **AND** any ordering change MUST be covered by a test explaining the new exactness signal

### Requirement: Query results expose provenance and freshness

The system SHALL return enough metadata for agents to judge whether a result is trustworthy and current.

#### Scenario: Result includes provenance

- **WHEN** a query returns a result
- **THEN** the result MUST include whether it is generated or user-authored when known
- **AND** it MUST include generator metadata when available
- **AND** it MUST include knowledge path source metadata when loaded from an overlay

#### Scenario: Stale index is visible but non-mutating

- **WHEN** the repository HEAD differs from the last indexed commit
- **AND** the user runs a read-only query
- **THEN** the query MUST still return best-effort results from existing knowledge
- **AND** the response MUST include freshness metadata and stale warnings
- **AND** it MUST NOT refresh automatically by default
