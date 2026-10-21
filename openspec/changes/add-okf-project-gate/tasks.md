# Implementation Tasks

## Completion contract

P0-P4 are dependency order only. This change is complete only when the prompt rewrite, OpenSpec artifacts, project gate script, self-verification script, gauntlet L0 integration, conformance matrix, and local verification commands are complete.

## P0 — Project learning and prompt deepening

- [x] Read repository instructions and mstack/deepen-task-prompt contracts.
- [x] Inspect `AGENTS.md`, `go.mod`, `README.md`, `tools/gauntlet.sh`, mutation scripts, verify scripts, and high-completeness OpenSpec examples.
- [x] Produce `prompt-rewrite.md` with depth diagnosis and executable rewritten prompt.

## P1 — OpenSpec design

- [x] Create `.openspec.yaml`.
- [x] Create `proposal.md`.
- [x] Create `design.md` with facts, requirements, risks, acceptance criteria, and trace matrix.
- [x] Create `specs/okf-project-gate/spec.md` with Requirement + Scenario structure.
- [x] Create `tasks.md` with P0-P4 dependency stages.

## P2 — Project gate implementation

- [x] Add `tools/okf-project-gate.sh`.
- [x] Validate Go/toolchain/downmark repository invariants.
- [x] Validate gauntlet critical layer presence.
- [x] Support `--repo` and `--change-id`.
- [x] Infer active OpenSpec changes from `git status` when no explicit change ID is provided.
- [x] Validate required OpenSpec artifacts, Requirement/Scenario structure, tasks P0-P4, and conformance mapping.

## P3 — Self-verification and negative controls

- [x] Add `tools/verify-okf-project-gate.sh`.
- [x] Build a valid temporary fixture and prove the gate accepts it.
- [x] Add negative control for downmark version drift.
- [x] Add negative control for missing spec file.
- [x] Add negative control for missing Scenario structure.

## P4 — Gauntlet integration and conformance

- [x] Add L0 project gate call to `tools/gauntlet.sh`.
- [x] Preserve all existing gauntlet layers.
- [x] Add `conformance.md` mapping S01-S13 to implementation and verification.
- [x] Run syntax and targeted verification commands.
