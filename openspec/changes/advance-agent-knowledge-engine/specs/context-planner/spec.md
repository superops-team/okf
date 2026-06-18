## ADDED Requirements

### Requirement: Context operation builds a planned context pack

The system SHALL build context responses from a deterministic plan while preserving existing context item fields as additive-compatible output.

#### Scenario: Symbol line ranges are preferred

- **WHEN** a query result contains a symbol with start and end line metadata
- **THEN** context extraction MUST prefer that line range over a raw keyword search
- **AND** it MUST include deterministic surrounding context lines when budget allows

#### Scenario: Same-file ranges are merged

- **WHEN** multiple selected hits refer to overlapping or adjacent ranges in the same source file
- **THEN** the context planner MUST merge those ranges into a single item when this reduces duplication
- **AND** the item MUST preserve references to the contributing hits or reasons

#### Scenario: Relation neighborhood can be included

- **WHEN** selected code hits have relation metadata
- **AND** relation expansion is explicitly enabled through `include_relations=true` or `--include-relations`
- **THEN** the planner MUST include deterministic one-hop neighbor candidates before lower-ranked unrelated candidates when budget allows
- **AND** relation-expanded items MUST include a reason explaining the relation used
- **AND** V1 relation expansion MUST be limited to at most 3 neighbor candidates per primary hit unless a future explicit setting changes that limit

### Requirement: Context packing respects budget with explicit omissions

The system SHALL pack context items under a token budget and explain omitted content.

#### Scenario: Budget truncation is deterministic

- **WHEN** candidate context items exceed the budget
- **THEN** the planner MUST include higher-ranked primary items first
- **AND** omit lower-ranked or duplicate items deterministically
- **AND** return omission records with reason and count

#### Scenario: Missing source files do not fail the whole context operation

- **WHEN** a knowledge hit references a source file that no longer exists in the working tree
- **THEN** the context response MUST include an omission or warning for that item
- **AND** it MUST continue packing other available items

#### Scenario: Token estimation is stable

- **WHEN** estimating snippet token usage in V1
- **THEN** the system MUST use `ceil(rune_count / 4)` as the deterministic local estimator
- **AND** the estimator MUST not call external services or models

#### Scenario: Source reads are bounded and safe

- **WHEN** context planner reads source files for snippets
- **THEN** it MUST read only files under the resolved repository root or explicitly allowed source roots
- **AND** it MUST omit paths outside those roots with a warning
- **AND** it MUST apply deterministic limits before snippet extraction
- **AND** V1 defaults MUST be at most 50 candidate items, 256KiB read per source file, and the requested budget tokens or 4000 estimated tokens when omitted
