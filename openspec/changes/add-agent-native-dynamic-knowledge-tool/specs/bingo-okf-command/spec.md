## ADDED Requirements

### Requirement: Bingo includes an OKF knowledge-management command
Bingo SHALL include an `okf` subcommand that manages the current repository's OKF knowledge base from inside the Bingo binary.

#### Scenario: Command registration
- **WHEN** a user runs `bingo okf --help`
- **THEN** Bingo MUST show the OKF knowledge-management command without requiring a separately installed `okf` executable

#### Scenario: Core subcommands are discoverable
- **WHEN** a user runs `bingo okf --help`
- **THEN** the help output MUST mention `status`, `init`, `refresh`, `query`, `context` and `lint` unless a deferred subcommand is explicitly documented as out of scope for the current release

### Requirement: Bingo calls OKF packages directly
Bingo SHALL integrate OKF through compile-time Go package dependencies rather than shelling out to external commands.

#### Scenario: External okf is not installed
- **WHEN** `okf` is absent from `$PATH`
- **THEN** `bingo okf status`, `bingo okf init`, `bingo okf refresh`, `bingo okf query` and `bingo okf context` MUST still work when their repository preconditions are satisfied

#### Scenario: External okf is broken or shadowed
- **WHEN** an unrelated or broken `okf` executable exists earlier in `$PATH`
- **THEN** `bingo okf` behavior MUST be unaffected because it uses OKF package APIs directly

### Requirement: Bingo okf supports human and JSON output
Bingo SHALL support both concise human output and versioned JSON output for OKF operations.

#### Scenario: Human status output
- **WHEN** a user runs `bingo okf status`
- **THEN** Bingo MUST print repository path, knowledge directory, freshness and summary counts in a human-readable format

#### Scenario: JSON status output
- **WHEN** a user runs `bingo okf status --json`
- **THEN** Bingo MUST emit the versioned status response schema from the OKF service with `schema_version="okf.tool.v1"`

### Requirement: Bingo okf follows repository defaults
Bingo SHALL use project-local defaults that match OKF package conventions.

#### Scenario: Default repository path
- **WHEN** the user omits `--repo`
- **THEN** Bingo MUST use the current working directory and resolve the Git repository root through OKF package logic

#### Scenario: Default knowledge directory
- **WHEN** the user omits `--dir`
- **THEN** Bingo MUST use `.okf/knowledge`

### Requirement: Bingo okf rejects worktree orchestration in V1
Bingo SHALL avoid hidden worktree creation or cross-repository mutation for OKF commands in V1.

#### Scenario: Worktree flag provided
- **WHEN** a user runs `bingo okf` with a Bingo worktree flag
- **THEN** Bingo MUST reject the command with a `worktree_not_supported` error rather than preparing or using a separate worktree

### Requirement: Bingo okf read operations are side-effect free
Bingo SHALL keep status, query and context read paths free from repository mutations by default.

#### Scenario: Query through Bingo
- **WHEN** a user runs `bingo okf query --q <text>`
- **THEN** Bingo MUST NOT modify `.okf/knowledge`, `.okf/state.json` or cache files unless a future explicit mutating refresh option is provided

#### Scenario: Context through Bingo
- **WHEN** a user runs `bingo okf context --q <text>`
- **THEN** Bingo MUST NOT modify `.okf/knowledge`, `.okf/state.json` or cache files unless a future explicit mutating refresh option is provided
