## ADDED Requirements

### Requirement: Core packages have executable behavior tests
The system SHALL include deterministic Go tests for parser, lint, query, okf bundle operations, git file filtering, git analysis, and incremental update behavior.

#### Scenario: Parser behavior is protected
- **WHEN** `go test ./pkg/parser` is run
- **THEN** tests MUST cover markdown files with frontmatter, markdown files without frontmatter, malformed YAML, missing required `title`, missing required `type`, and CRLF frontmatter separators

#### Scenario: Lint behavior is protected
- **WHEN** `go test ./pkg/lint` is run
- **THEN** tests MUST cover required fields, description length, type format, timestamp validation, tag normalization, long content lines, duplicate tags, duplicate titles, and strict-mode failure semantics

#### Scenario: Query and bundle behavior is protected
- **WHEN** `go test ./pkg/query ./pkg/okf` is run
- **THEN** tests MUST cover text search, type filtering, tag filtering, resource filtering, related concept lookup, save/load round trip, duplicate filename handling, and empty bundle behavior

### Requirement: Git integration has isolated tests
The system SHALL test git integration logic with temporary repositories or isolated filesystem fixtures, without depending on developer machine state.

#### Scenario: Include and exclude rules are tested
- **WHEN** git filtering tests run
- **THEN** they MUST verify include patterns, excluded directories, max file size, deleted files, nested paths, and unsupported extensions

#### Scenario: Incremental update tests are isolated
- **WHEN** incremental update tests run
- **THEN** they MUST create a temporary Git repository, generate an initial knowledge base, add/modify/delete tracked files, run update logic, and assert the saved knowledge base matches the changed file set

### Requirement: CLI smoke tests cover user-visible commands
The system SHALL include smoke tests for core CLI commands that execute the compiled command in temporary repositories.

#### Scenario: CLI init, lint, search, and update work together
- **WHEN** the CLI smoke test creates a temp repository and runs `okf init`, `okf lint`, `okf search`, and `okf update`
- **THEN** each command MUST exit with the expected status and produce user-visible output consistent with the updated knowledge base

### Requirement: Performance benchmarks establish baseline budgets
The system SHALL include benchmarks for repository generation, file analysis, query filtering, and save/load round trips.

#### Scenario: Benchmarks report allocation and runtime data
- **WHEN** `go test -bench=. -benchmem ./...` is run
- **THEN** benchmarks MUST report allocations and runtime for representative small and medium repositories, and must be stable enough for future benchstat comparison

### Requirement: Full test suite passes under race detection
The system SHALL keep the unit test suite compatible with Go race detection.

#### Scenario: Race detector validates concurrent indexing
- **WHEN** `go test -race ./...` is run after concurrent indexing is implemented
- **THEN** the test suite MUST pass without data race reports
