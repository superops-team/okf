## ADDED Requirements

### Requirement: Tool service is exposed through okf CLI

The system SHALL expose `pkg/tool.Service` through an `okf tool` command family without duplicating service behavior in CLI handlers.

#### Scenario: Tool status returns stable JSON envelope

- **WHEN** running `okf tool status --json`
- **THEN** the command MUST call the shared tool service status operation
- **AND** output MUST use `schema_version: "okf.tool.v1"`
- **AND** output MUST preserve existing `okf.tool.v1` top-level fields
- **AND** output MUST include operation, ok flag, mutating flag, repo root, knowledge directory, freshness, warnings, and the documented result fields for that operation
- **AND** newly added fields MUST be optional and additive unless schema version is bumped

#### Scenario: Tool status result fields are concrete

- **WHEN** running `okf tool status --json`
- **THEN** `result` MUST include ready state, concept count, unique type count, and unique tag count
- **AND** path metadata MUST include the resolved write knowledge directory and read knowledge paths when overlay resolution is available

#### Scenario: Tool init and refresh are explicit mutating operations

- **WHEN** running `okf tool init --json`
- **OR** running `okf tool refresh --mode incremental --json`
- **THEN** the command MAY write generated knowledge, state, and derived cache files
- **AND** the JSON envelope MUST include `mutating: true`

#### Scenario: Tool query and context are read-only

- **WHEN** running `okf tool query --q <query> --json`
- **OR** running `okf tool context --q <query> --budget-tokens <n> --json`
- **THEN** the command MUST NOT create, modify, or delete knowledge files, state files, metadata index files, or cache files
- **AND** the JSON envelope MUST include `mutating: false`
- **AND** stale knowledge MUST be reported through freshness and warnings rather than auto-refreshing by default

#### Scenario: Tool context has explicit V1 defaults

- **WHEN** running `okf tool context --q <query> --json` without budget or relation flags
- **THEN** the service MUST use the existing default budget of 4000 estimated tokens
- **AND** relation expansion MUST be disabled
- **AND** trace output MUST be disabled

### Requirement: Tool CLI supports structured filters

The system SHALL expose structured query filters through CLI flags and map them to the service request model.

#### Scenario: Query filters are passed to service

- **WHEN** running `okf tool query --q <query> --language go --symbol-kind function --qualified-name Foo --relation-kind contains --limit 5 --json`
- **THEN** the command MUST pass those filters to the tool service
- **AND** the response MUST preserve deterministic ranking and navigation metadata

#### Scenario: Trace is explicit opt-in

- **WHEN** running `okf tool query --q <query> --json`
- **OR** running `okf tool context --q <query> --json`
- **THEN** trace output MUST be omitted by default
- **AND** trace output MUST be included only when `--include-trace` is provided

#### Scenario: Invalid flags return tool errors

- **WHEN** a tool command receives an invalid mode, invalid limit, invalid budget, or empty query
- **THEN** it MUST return a stable tool error code in the JSON envelope when `--json` is set
- **AND** it MUST exit non-zero
