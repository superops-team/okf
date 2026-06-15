## ADDED Requirements

### Requirement: Agent tool interface does not require MCP or external OKF binary
The system SHALL expose repository knowledge operations through an agent-native interface that does not require an MCP server, CodeGraph runtime, or separately installed `okf` executable.

#### Scenario: Embedded agent calls Go API
- **WHEN** an agent is built in Go or can link OKF as a Go module
- **THEN** it MUST be able to call status, init, refresh, query and context operations through a Go service API without starting an MCP server

#### Scenario: Bingo exposes built-in CLI JSON
- **WHEN** the user or a non-Go integration needs CLI access
- **THEN** it MUST be able to call equivalent operations through `bingo okf ... --json` and receive versioned machine-readable JSON responses from the built Bingo binary

#### Scenario: Bingo does not shell out to okf
- **WHEN** `bingo okf` handles status, init, refresh, query, context or lint operations
- **THEN** it MUST call OKF package APIs directly and MUST NOT execute an external `okf` command from `$PATH`

### Requirement: Tool responses are versioned and stable
The system SHALL version all agent-facing JSON responses and preserve backwards compatibility within a schema version.

#### Scenario: Successful response
- **WHEN** a tool operation succeeds
- **THEN** the response MUST include `schema_version="okf.tool.v1"`, `operation`, `ok=true`, `repo_root`, `knowledge_dir`, `freshness`, `warnings`, and an operation-specific `result`

#### Scenario: Failed response
- **WHEN** a tool operation fails
- **THEN** the response MUST include `schema_version="okf.tool.v1"`, `operation`, `ok=false`, a stable error `code`, a human-readable `message`, and a machine-actionable `remediation` when available

### Requirement: Read-only operations do not mutate repository knowledge
The system SHALL keep agent read paths side-effect free by default.

#### Scenario: Query is read-only
- **WHEN** `query` runs
- **THEN** it MUST NOT create, update or delete `.okf/knowledge`, `.okf/state.json` or cache files by default

#### Scenario: Context is read-only
- **WHEN** `context` runs
- **THEN** it MUST NOT create, update or delete `.okf/knowledge`, `.okf/state.json` or cache files by default

#### Scenario: Status is read-only
- **WHEN** `status` runs
- **THEN** it MUST report readiness and freshness without mutating repository knowledge files

### Requirement: Status operation reports readiness
The system SHALL provide a status operation suitable for agent preflight checks.

#### Scenario: Knowledge base is ready
- **WHEN** `.okf/knowledge` exists and can be loaded
- **THEN** status MUST report repository root, knowledge directory, last indexed commit, current HEAD, freshness and index counts

#### Scenario: Knowledge base is not initialized
- **WHEN** `.okf/knowledge` is missing
- **THEN** status MUST return `knowledge_not_initialized` or a non-ready status with instructions to run initialization or full refresh

### Requirement: Init operation creates generated knowledge explicitly
The system SHALL provide an explicit initialization operation for repositories without existing OKF knowledge.

#### Scenario: Initialize repository knowledge
- **WHEN** init runs inside or against a Git repository
- **THEN** it MUST create `.okf/knowledge` using OKF package generation APIs and report generated concept and relation counts

#### Scenario: Init outside Git repository
- **WHEN** init runs outside a Git repository
- **THEN** it MUST fail with `not_git_repository` and remediation text

### Requirement: Refresh operation is safe for generated and human knowledge
The system SHALL refresh generated knowledge without deleting human-authored concepts.

#### Scenario: Generated artifact deletion
- **WHEN** a source file is deleted and refresh runs
- **THEN** generated concepts and relation records for that file MUST be removed only when trusted generated metadata matches the deleted `source_path`

#### Scenario: Human-authored concept preservation
- **WHEN** a concept does not carry trusted generated metadata
- **THEN** refresh MUST NOT delete it automatically even if its title or resource resembles a deleted source file

### Requirement: Bingo okf command manages project knowledge
The system SHALL provide a Bingo subcommand for common OKF knowledge-base management operations.

#### Scenario: Bingo okf command is registered
- **WHEN** a user runs `bingo okf --help`
- **THEN** the command MUST be available and list status, init, refresh, query, context and lint subcommands or explicitly mark deferred subcommands

#### Scenario: Bingo okf initializes knowledge base
- **WHEN** a user runs `bingo okf init --repo <path>` inside or against a Git repository
- **THEN** Bingo MUST initialize `.okf/knowledge` by calling OKF package generation APIs and report the generated concept count

#### Scenario: Bingo okf queries knowledge base
- **WHEN** a user runs `bingo okf query --q <text> --json`
- **THEN** Bingo MUST return the same versioned query response schema as the embedded OKF service
