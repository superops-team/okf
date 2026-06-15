## ADDED Requirements

### Requirement: Knowledge index freshness is explicit
The system SHALL track whether generated knowledge and state are fresh relative to the current Git repository state.

#### Scenario: Fresh index status
- **WHEN** the repository HEAD matches the last successfully indexed commit
- **THEN** status output MUST report the index as fresh and include concept, entity and relation counts when they can be loaded

#### Scenario: Stale index status
- **WHEN** the repository HEAD differs from the last successfully indexed commit
- **THEN** status output MUST report the index as stale and include both commits plus the number of changed files when it can be computed

### Requirement: Query cache is optional, derived and rebuildable
The system SHALL treat any local query cache as a derived performance artifact rather than the source of truth, and V1 SHALL NOT require a persistent disk cache to answer queries.

#### Scenario: Cache missing during query
- **WHEN** a query runs and the cache file is missing
- **THEN** the system MUST rebuild an in-memory index from `.okf/knowledge` or fall back to direct bundle scanning with a warning

#### Scenario: Cache schema mismatch
- **WHEN** a cache schema version is incompatible with the running OKF package
- **THEN** the system MUST discard or ignore the cache and rebuild from `.okf/knowledge` before serving results

#### Scenario: Read-only query path
- **WHEN** `query` or `context` runs
- **THEN** the operation MUST NOT create, modify or delete `.okf/knowledge`, `.okf/state.json` or cache files by default

### Requirement: Refresh modes are explicit and deterministic
The system SHALL support explicit refresh modes for agent-safe operation.

#### Scenario: Incremental refresh
- **WHEN** refresh runs in `incremental` mode and valid state exists
- **THEN** the system MUST update only changed generated knowledge artifacts plus affected derived views and refresh affected in-memory/cache data

#### Scenario: Full refresh
- **WHEN** refresh runs in `full` mode
- **THEN** the system MUST regenerate generated knowledge artifacts from the current Git repository and replace stale generated artifacts safely

#### Scenario: Cache-only refresh
- **WHEN** refresh runs in `cache-only` mode
- **THEN** the system MUST rebuild local query cache or in-memory index data without rewriting generated Markdown concepts

#### Scenario: Auto refresh is not default
- **WHEN** query or context runs without an explicit auto-refresh option
- **THEN** the system MUST NOT mutate the knowledge base before serving results

### Requirement: Generated artifacts are deleted safely
The system SHALL distinguish generated concepts from human-authored concepts before deleting or replacing files.

#### Scenario: Deleted source file with generated metadata
- **WHEN** a source file is deleted and refresh runs
- **THEN** the system MAY delete concepts whose metadata includes `generated=true`, `generator=okf.git` and matching `source_path`

#### Scenario: Human-authored concept preservation
- **WHEN** a concept lacks trusted generated metadata
- **THEN** refresh MUST preserve it even if its title, resource or path resembles a deleted source file

#### Scenario: Ambiguous generated metadata
- **WHEN** generated metadata is missing, incomplete or inconsistent
- **THEN** refresh MUST preserve the concept and include a warning instead of deleting it
