## ADDED Requirements

### Requirement: Knowledge directory resolution uses a single source-of-truth resolver

The system SHALL resolve the effective write knowledge directory through one shared resolver used by CLI commands, import, watch, and agent tool operations.

#### Scenario: Configured knowledge directory participates in precedence

- **WHEN** resolving the write knowledge directory
- **THEN** the system MUST use this precedence order from highest to lowest:
  1. CLI `--dir` or `-dir`
  2. `OKF_KNOWLEDGE_DIR` environment variable
  3. config file `knowledge_dir`
  4. existing local `.okf/knowledge` under the discovered repository root
  5. existing local `.okf/knowledge` under the current working directory when no repository root is discovered
  6. platform default knowledge directory
- **AND** the resolver MUST return both the resolved path and the source that selected it

#### Scenario: Read-only resolution does not create files

- **WHEN** a read-only operation resolves knowledge paths
- **THEN** the resolver MUST NOT create the config file
- **AND** it MUST NOT create the platform default directory
- **AND** it MUST NOT create a missing knowledge directory, state file, metadata index, or cache file

#### Scenario: Mutating resolution may create write target

- **WHEN** a mutating operation resolves the write knowledge directory
- **THEN** the resolver MAY create the selected write knowledge directory
- **AND** it MUST NOT create any configured read-only overlay path

#### Scenario: Config commands report value source

- **WHEN** running `okf config get knowledge_dir`
- **THEN** the system MUST print the effective resolved value
- **AND** it MUST indicate whether the value came from CLI, environment, config file, repository local directory, current working directory local directory, or platform default when the output format supports metadata

#### Scenario: Missing config file does not create one during read-only operations

- **WHEN** no config file exists
- **AND** a read-only command resolves knowledge paths
- **THEN** the system MUST use environment, local, or platform defaults
- **AND** it MUST NOT create a config file

### Requirement: Multiple knowledge paths can be read as an overlay

The system SHALL support ordered read-only `knowledge_paths[]` overlays while preserving a single write target.

#### Scenario: Read paths include configured overlay paths

- **WHEN** config defines `knowledge_paths[]`
- **THEN** query and context operations MUST use this read path order:
  1. resolved write knowledge directory
  2. configured `knowledge_paths[]` in config order
- **AND** paths MUST be de-duplicated by canonical absolute path
- **AND** missing optional overlay paths MUST be reported as warnings
- **AND** permission denied, non-directory paths, and invalid path syntax MUST be actionable errors

#### Scenario: Legacy single directory behavior is preserved

- **WHEN** config has `knowledge_dir` but no `knowledge_paths[]`
- **THEN** read paths MUST contain exactly the resolved write knowledge directory
- **AND** query and context behavior MUST match the previous single knowledge directory behavior except for additive metadata fields

#### Scenario: Overlay source metadata is attached to hits

- **WHEN** a query result comes from an overlay path
- **THEN** the result MUST include the knowledge path or a stable path label
- **AND** it MUST include numeric source rank precedence metadata
- **AND** deterministic ranking MUST use source rank as a tie-break after score and exactness

#### Scenario: Mutating operations write to only one target

- **WHEN** running `okf add`, `okf init`, or `okf refresh`
- **THEN** the operation MUST write only to the resolved write knowledge directory
- **AND** it MUST NOT modify additional `knowledge_paths[]` overlay directories unless an explicit future mutating flag selects a different write target

#### Scenario: Duplicate generated concepts are merged deterministically

- **WHEN** the same generated concept identity appears in multiple knowledge paths
- **THEN** the system MUST choose the highest-precedence source as the primary result
- **AND** it MUST retain duplicate source information in metadata or trace when trace is requested
- **AND** user-authored concepts MUST NOT be discarded solely because a generated concept has the same title

#### Scenario: Generated duplicate identity is explicit

- **WHEN** deciding whether two generated concepts are duplicates
- **THEN** the system MUST require matching generated metadata, generator name, source path, concept type, and stable resource or file path identity
- **AND** title-only matches MUST NOT be treated as duplicate identity

#### Scenario: Resolved write target failure is not hidden by overlay

- **WHEN** the resolved write knowledge directory cannot be read during a normal query or context operation
- **THEN** the operation MUST return an actionable error
- **AND** it MUST NOT silently succeed only because another overlay path is readable
