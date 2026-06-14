## ADDED Requirements

### Requirement: Code repositories are modeled as a first-class knowledge dimension
The system SHALL represent indexed source repositories as code knowledge entities and relationships within the existing OKF Concept/Bundle model.

#### Scenario: Repository generation creates code dimension artifacts
- **WHEN** a user generates an OKF knowledge base from a Git repository with code dimension enabled
- **THEN** the system MUST create generated code concepts for repository-level overview, indexed source files, and relationship indexes using OKF markdown concepts rather than a separate required database

#### Scenario: Existing OKF concept contract remains valid
- **WHEN** code dimension concepts are written
- **THEN** they MUST preserve the existing required OKF frontmatter fields and place code-specific machine metadata in custom fields or generated index concepts

### Requirement: Code entity identity is stable
The system SHALL assign deterministic identities to code entities independent of extractor-specific IDs.

#### Scenario: Unchanged symbols keep identity across regeneration
- **WHEN** a source symbol's repository-relative file path, kind, qualified name, and source line range do not change
- **THEN** full regeneration and incremental update MUST produce the same OKF code entity identity for that symbol

#### Scenario: External graph IDs are secondary metadata
- **WHEN** an external extractor such as CodeGraph provides its own node ID
- **THEN** the system MAY store that ID as an external reference but MUST NOT use it as the primary OKF code entity identity

### Requirement: Source files expose navigable code summaries
The system SHALL render source-file concepts with enough structure for humans and agents to navigate the repository.

#### Scenario: File concept includes source metadata and symbols
- **WHEN** a source file is indexed
- **THEN** the generated concept MUST include repository-relative path, language, content hash when available, line count or line range, imports, symbols, parse warnings, and each symbol's `file_path:start_line-end_line` location when available

#### Scenario: Source text remains on demand
- **WHEN** a generated code concept is written
- **THEN** it MUST NOT include the full source file body by default, unless the user explicitly enables a source embedding option

### Requirement: Relationships are represented as generated knowledge
The system SHALL represent code relationships as deterministic generated knowledge records.

#### Scenario: Structural relationships are emitted
- **WHEN** repository generation completes
- **THEN** the system MUST emit deterministic relationship records for repository/package/file containment, file-to-symbol ownership, package-to-symbol ownership when package data exists, and import relationships discovered by supported analyzers

#### Scenario: Rich relationships are preserved when available
- **WHEN** an extractor provides richer relations such as calls, extends, implements, references, type_of, or returns
- **THEN** the system MUST preserve those relations with provenance metadata even if OKF's native analyzer cannot produce them

### Requirement: Repository-level navigation views are generated
The system SHALL provide generated views that summarize the codebase at repository level.

#### Scenario: Repository overview summarizes code dimension
- **WHEN** code dimension generation succeeds
- **THEN** a repository overview concept MUST summarize indexed file count, language distribution, package/module list when available, symbol counts by kind, relation counts by kind, and generated artifact manifest

#### Scenario: Relation index is deterministic
- **WHEN** the same repository state is generated twice
- **THEN** relation index concepts MUST have deterministic ordering and content except for explicitly volatile timestamps

### Requirement: Incremental update maintains code dimension consistency
The system SHALL update generated code entities, relationships, and derived views consistently during incremental updates.

#### Scenario: Modified file updates dependent generated artifacts
- **WHEN** a tracked source file changes and the user runs `okf update`
- **THEN** the system MUST update that file's generated code concept, replace its stale symbol and relation records, and regenerate repository-level views that reference those records

#### Scenario: Deleted file removes stale generated records safely
- **WHEN** a tracked source file is deleted and the user runs `okf update`
- **THEN** the system MUST remove generated code concepts and relation records for that file, but MUST NOT remove human-authored concepts that do not carry generated code metadata

### Requirement: Code dimension search returns source navigation
The system SHALL make code dimension entities discoverable through OKF query/search APIs.

#### Scenario: Symbol search includes location
- **WHEN** a user searches for a symbol by simple or qualified name
- **THEN** results MUST include symbol kind, language, and `file_path:start_line-end_line` location when available

#### Scenario: Code filters use machine metadata
- **WHEN** a user filters by language, file path, symbol kind, qualified name, relation kind, or generated code concept type
- **THEN** the query engine MUST use code metadata rather than relying only on free-text markdown body matching

