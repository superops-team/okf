# Specification: OKF Project Admission Gate

## Requirement: Core repository invariants

The project gate SHALL validate OKF-specific repository invariants before heavy execution gates run.

### Scenario S01: Go and downmark constraints are pinned
- **GIVEN** a repository with `go.mod`
- **WHEN** `tools/okf-project-gate.sh` runs
- **THEN** it succeeds only if `go 1.26.0`, `toolchain go1.26.7`, and `github.com/giraffesyo/downmark v0.10.0` are present exactly
- **AND** any drift fails with a non-zero exit

### Scenario S02: Gauntlet critical layers remain present
- **GIVEN** a repository with `tools/gauntlet.sh`
- **WHEN** `tools/okf-project-gate.sh` runs
- **THEN** it verifies that gauntlet still contains build, vet, gofmt, staticcheck, full tests, race tests, coverage, shuffle, mutation, and real execution layers
- **AND** missing critical layer text fails with a non-zero exit

### Scenario S03: Core gate is read-only
- **WHEN** the project gate runs without `--change-id`
- **THEN** it checks repository invariants
- **AND** it does not write repository files
- **AND** it does not access network resources

## Requirement: OpenSpec change admission

The project gate SHALL validate the current or explicit OpenSpec change shape without requiring all historical draft changes to be complete.

### Scenario S04: Explicit change has required artifacts
- **GIVEN** `openspec/changes/<change-id>/`
- **WHEN** `tools/okf-project-gate.sh --change-id <change-id>` runs
- **THEN** `proposal.md`, `design.md`, `tasks.md`, `conformance.md`, and at least one `specs/*/spec.md` must exist
- **AND** any missing artifact fails with a non-zero exit

### Scenario S05: Spec uses Requirement and Scenario structure
- **GIVEN** an explicit or inferred change
- **WHEN** the project gate reads its spec files
- **THEN** at least one spec must contain `## Requirement:`
- **AND** at least one spec must contain `### Scenario`
- **AND** missing either structure fails with a non-zero exit

### Scenario S06: Tasks express full dependency boundary
- **GIVEN** an explicit or inferred change
- **WHEN** the project gate reads `tasks.md`
- **THEN** it requires a completion contract
- **AND** it requires P0, P1, P2, P3, and P4 dependency stages

### Scenario S07: Conformance maps scenarios to verification
- **GIVEN** an explicit or inferred change
- **WHEN** the project gate reads `conformance.md`
- **THEN** the file must contain scenario and verification semantics
- **AND** it must use at least one allowed alignment status: `fully`, `aligned`, `partial`, or `gap`

### Scenario S08: Current change can be inferred from git status
- **GIVEN** uncommitted or untracked files under `openspec/changes/<change-id>/`
- **WHEN** `tools/okf-project-gate.sh` runs without `--change-id`
- **THEN** it validates the inferred change IDs
- **AND** if no change ID is inferred, it only performs core repository invariant checks

## Requirement: Independent self-verification

The project gate SHALL include a self-verification script with positive and negative controls.

### Scenario S09: Valid fixture passes
- **WHEN** `tools/verify-okf-project-gate.sh` creates a valid temporary OKF-like fixture
- **THEN** `tools/okf-project-gate.sh --repo <fixture> --change-id demo` exits 0

### Scenario S10: Version drift negative control fails
- **WHEN** the verification script changes the fixture's downmark requirement away from `v0.10.0`
- **THEN** the project gate rejects the fixture

### Scenario S11: Missing spec negative control fails
- **WHEN** the verification script removes the fixture's `specs/*/spec.md`
- **THEN** the project gate rejects the fixture

### Scenario S12: Missing Scenario negative control fails
- **WHEN** the verification script removes `### Scenario` from the fixture spec
- **THEN** the project gate rejects the fixture

## Requirement: Gauntlet integration

The heavy gauntlet SHALL invoke the project gate before expensive execution layers without weakening existing layers.

### Scenario S13: Gauntlet runs project gate as L0
- **WHEN** `tools/gauntlet.sh` runs
- **THEN** it invokes `tools/okf-project-gate.sh` before the build layer
- **AND** all pre-existing build, static, test, mutation, and real execution layers remain present
