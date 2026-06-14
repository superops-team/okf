## ADDED Requirements

### Requirement: Repository generation avoids per-file Git process amplification
The system SHALL avoid launching multiple Git subprocesses for every indexed file.

#### Scenario: Git metadata is collected in batches
- **WHEN** a repository with many tracked files is generated
- **THEN** author, last commit, commit count, and changed-file metadata MUST be collected using batched Git operations rather than one or more `git log` calls per file

### Requirement: File analysis can run concurrently
The system SHALL analyze independent files with bounded concurrency.

#### Scenario: Concurrent generation is deterministic
- **WHEN** repository generation runs with concurrency greater than one
- **THEN** the resulting concepts and relationship records MUST have deterministic ordering and content independent of goroutine scheduling

#### Scenario: Concurrent generation respects resource limits
- **WHEN** a repository contains more files than the worker count
- **THEN** the system MUST bound concurrent workers, avoid unbounded goroutine creation, and close all opened files

### Requirement: Regex fallback avoids repeated compilation
The system SHALL precompile static regular expressions used by fallback import and function extraction.

#### Scenario: Repeated file analysis reuses compiled patterns
- **WHEN** many files of the same language are analyzed
- **THEN** extraction MUST reuse compiled patterns instead of calling `regexp.Compile` or `regexp.MatchString` inside the per-line hot loop

### Requirement: Query operations use indexes for common filters
The system SHALL build in-memory indexes for common query dimensions after loading or generating a bundle.

#### Scenario: Type and tag filters avoid full scans when index is available
- **WHEN** a bundle has an index and the user filters by type, tag, resource, title, or symbol name
- **THEN** the query engine MUST use the relevant index instead of scanning every concept where doing so preserves result semantics

#### Scenario: Full-text search remains correct
- **WHEN** the user performs free-text search
- **THEN** results MUST remain semantically equivalent to the current search behavior, even if the first implementation still performs a full scan

### Requirement: Performance budgets are measured and enforced by benchmarks
The system SHALL define performance targets that can be checked with Go benchmarks.

#### Scenario: Medium repository generation has a target budget
- **WHEN** benchmark fixtures simulate approximately 1,000 source files
- **THEN** full generation SHOULD complete within a documented target budget on a developer laptop, and benchmark output MUST be suitable for benchstat regression comparison

#### Scenario: Query benchmark covers indexed and non-indexed paths
- **WHEN** query benchmarks run against a bundle with at least 1,000 concepts
- **THEN** benchmark results MUST separately report indexed filter queries and free-text queries
