## ADDED Requirements

### Requirement: Query and context can return deterministic retrieval traces

The system SHALL provide trace steps explaining how query and context results were selected when trace output is explicitly enabled.

#### Scenario: Trace is disabled by default

- **WHEN** query or context is called without `include_trace=true`
- **THEN** the response MUST omit trace output
- **AND** normal result fields MUST remain unchanged except for other additive optional fields

#### Scenario: Query trace explains candidate selection

- **WHEN** a query response includes trace
- **THEN** trace steps MUST include path resolution, bundle loading, filter application, candidate scoring, tie-break ordering, overlay merge decisions, and freshness warnings when applicable
- **AND** steps MUST be ordered deterministically

#### Scenario: Context trace explains packing decisions

- **WHEN** a context response includes trace
- **THEN** trace steps MUST include selected primary hits, same-file range merge decisions, relation expansion decisions, snippet extraction source, budget packing, and omission reasons
- **AND** trace MUST identify source paths and knowledge paths without duplicating full source content

### Requirement: Trace is safe and stable for agents

The system SHALL make retrieval traces useful for automated agents without leaking unnecessary content.

#### Scenario: Trace avoids full source duplication

- **WHEN** a trace step references a source file or concept
- **THEN** it MUST reference repo-relative file path, concept path, stable knowledge path label, score, reason, or omission metadata for every field that exists on the selected result or omission
- **AND** it MUST NOT include full source file contents outside normal context result snippets

#### Scenario: Trace schema is golden-testable

- **WHEN** trace output is enabled for a fixed fixture repository
- **THEN** the trace JSON field names and step ordering MUST be deterministic and asserted by golden tests
- **AND** changes to trace schema MUST be treated as agent-facing contract changes

#### Scenario: Trace schema remains compact in V1

- **WHEN** returning V1 trace steps
- **THEN** each step MUST use a compact schema with type, message, refs, counts, score delta, omission reason, and warnings
- **AND** arbitrary nested raw inputs MUST NOT be part of the stable trace schema
