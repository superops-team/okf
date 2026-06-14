## ADDED Requirements

### Requirement: Update command persists changed concepts
The system SHALL make `okf update` persist incremental changes to the configured knowledge directory.

#### Scenario: Modified tracked file updates existing concept
- **WHEN** a tracked source file is modified after `okf init` and the user runs `okf update`
- **THEN** the corresponding concept file in `.okf/knowledge` MUST be rewritten with current metadata, timestamp, content summary, symbols, and commit metadata

#### Scenario: Added tracked file creates new concept
- **WHEN** a new tracked source file is committed and the user runs `okf update`
- **THEN** a new concept file MUST be created under `.okf/knowledge` using the same naming and serialization rules as full generation

#### Scenario: Deleted tracked file removes stale concept
- **WHEN** a tracked source file is deleted and committed and the user runs `okf update`
- **THEN** the previous concept for that file MUST be removed from `.okf/knowledge`, unless a tombstone mode is explicitly enabled by configuration

### Requirement: Incremental update is idempotent
The system SHALL make repeated update runs safe and stable.

#### Scenario: Re-running update after same commit is a no-op
- **WHEN** `okf update` is run twice for the same repository state
- **THEN** the second run MUST not duplicate concepts, change unrelated concept files, or rewrite files whose source metadata has not changed

#### Scenario: Full regeneration and incremental state converge
- **WHEN** a repository is indexed with `okf init -force` and separately updated through equivalent incremental commits
- **THEN** both knowledge directories MUST contain equivalent concepts for tracked included files, excluding generated-at timestamps and other explicitly volatile fields

### Requirement: Update tracks last indexed commit
The system SHALL record the last successfully indexed commit so updates do not silently miss multiple commits.

#### Scenario: Multiple commits since last update are all processed
- **WHEN** two or more commits are created after the last indexed commit and the user runs `okf update`
- **THEN** the update MUST process the complete diff range from last indexed commit to HEAD rather than only the latest commit

#### Scenario: Missing index state falls back safely
- **WHEN** no last-indexed state exists and the user runs `okf update`
- **THEN** the system MUST either perform a full regeneration or compute a safe baseline, and MUST explain the selected path in CLI output

### Requirement: Update uses one serialization path
The system SHALL serialize knowledge concepts through one canonical serializer.

#### Scenario: SaveKnowledgeBase and SaveBundle produce compatible markdown
- **WHEN** generated or updated concepts are saved
- **THEN** frontmatter and body formatting MUST be produced by the shared parser serialization path, not by duplicate ad hoc YAML serialization logic

### Requirement: Update reports accurate user-visible results
The system SHALL report only changes that were actually written or removed.

#### Scenario: CLI output matches persisted changes
- **WHEN** `okf update -verbose` completes
- **THEN** the listed changed files, created concepts, updated concepts, deleted concepts, skipped files, and elapsed time MUST match the filesystem state under `.okf/knowledge`
