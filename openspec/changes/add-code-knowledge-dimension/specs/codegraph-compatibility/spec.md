## ADDED Requirements

### Requirement: CodeGraph nodes map to OKF code entities
The system SHALL define a deterministic mapping from CodeGraph node records to OKF code dimension entities.

#### Scenario: Symbol node maps to symbol entity
- **WHEN** a CodeGraph node has kind `function`, `method`, `class`, `struct`, `interface`, `trait`, `protocol`, `enum`, `type_alias`, `component`, or another supported symbol kind
- **THEN** the system MUST map it to an OKF `CodeEntity` with symbol kind, name, qualified name, language, repository-relative file path, line/column range, signature/docstring when available, visibility/exported metadata when available, and provenance `codegraph`

#### Scenario: File and module nodes map to structural entities
- **WHEN** a CodeGraph node has kind `file`, `module`, or `namespace`
- **THEN** the system MUST map it to an OKF structural code entity and use it as a target for containment and navigation views

#### Scenario: Unsupported node kind is preserved safely
- **WHEN** a CodeGraph node kind is not known by OKF
- **THEN** the system MUST preserve it as `symbol_kind` or `external_kind` metadata and MUST NOT fail the whole import solely because of that kind

### Requirement: CodeGraph edges map to OKF code relations
The system SHALL define a deterministic mapping from CodeGraph edge records to OKF code relations.

#### Scenario: Known edge kinds retain semantics
- **WHEN** a CodeGraph edge has kind `contains`, `calls`, `imports`, `exports`, `extends`, `implements`, `references`, `type_of`, `returns`, `instantiates`, `overrides`, or `decorates`
- **THEN** the system MUST create an OKF `CodeRelation` with the same normalized kind, mapped source and target identities, source location when available, metadata, and provenance `codegraph`

#### Scenario: Unresolved edge target is represented as reference
- **WHEN** a CodeGraph edge target cannot be mapped to an OKF code entity
- **THEN** the system MUST retain the relationship as an unresolved `target_ref` with enough metadata to display and debug it, instead of dropping it silently

### Requirement: CodeGraph file records map to file freshness metadata
The system SHALL use CodeGraph file metadata as optional freshness and diagnostic input.

#### Scenario: File content hash is imported
- **WHEN** a CodeGraph file record contains content hash, language, indexed timestamp, node count, or errors
- **THEN** the system MUST preserve those fields in OKF code metadata where applicable and MAY use content hash to skip unchanged generated artifacts

#### Scenario: Extractor errors are visible
- **WHEN** CodeGraph reports file-level errors
- **THEN** the generated OKF code concept MUST expose those errors in a warnings or diagnostics section without failing unrelated files

### Requirement: CodeGraph subgraphs can become OKF context views
The system SHALL support representing CodeGraph-style subgraphs and context results as OKF generated views.

#### Scenario: Subgraph import creates context view
- **WHEN** an imported graph slice contains a root node set plus related nodes and edges
- **THEN** the system SHOULD create or update a `code_context_view` concept that lists the root entities, included files, included symbols, relationships, truncation limits, and provenance

#### Scenario: Context view is reproducible
- **WHEN** the same graph slice input is imported twice
- **THEN** the resulting OKF context view MUST be deterministic except for explicitly volatile generated timestamps

### Requirement: CodeGraph compatibility does not require CodeGraph runtime
The system SHALL keep CodeGraph compatibility as a data contract, not a mandatory runtime dependency.

#### Scenario: OKF native analyzer works without CodeGraph
- **WHEN** CodeGraph is not installed and no CodeGraph fixture or database is provided
- **THEN** OKF code dimension generation MUST still work using native analyzers and must not require Node.js, SQLite CodeGraph DBs, or CodeGraph MCP tools

#### Scenario: Rich external data augments native output
- **WHEN** CodeGraph-compatible data is available
- **THEN** OKF MAY import richer symbols and relations, mark them with provenance `codegraph`, and merge them with native generated concepts using OKF stable identities

