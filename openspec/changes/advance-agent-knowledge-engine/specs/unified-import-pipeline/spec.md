## ADDED Requirements

### Requirement: Add command uses a unified import dispatcher

The system SHALL route `okf add` through a shared import dispatcher that handles files, directories, archives, validation, and smart import strategy consistently.

#### Scenario: Single file import is validated before write

- **WHEN** running `okf add /path/to/file.md`
- **THEN** if the file is OKF Markdown, it MUST be parsed and validated as an OKF concept before being written
- **AND** missing required frontmatter fields MUST reject that file with a clear error
- **AND** if a future conversion mode accepts non-OKF Markdown, conversion MUST produce valid OKF Markdown before write
- **AND** successful imports MUST update the metadata index consistently with smart import behavior

#### Scenario: Directory import validates each candidate independently

- **WHEN** running `okf add /path/to/directory`
- **THEN** the dispatcher MUST recursively collect Markdown candidates using the configured include/exclude and depth rules
- **AND** each candidate MUST be validated before applying a write strategy
- **AND** invalid files MUST be reported with file-specific errors
- **AND** valid files MUST continue importing after invalid siblings
- **AND** the command MUST exit non-zero if any candidate failed

#### Scenario: Archive import uses the same validation and strategy path

- **WHEN** running `okf add /path/to/archive.zip`
- **OR** running `okf add /path/to/archive.tar.gz`
- **OR** running `okf add /path/to/archive.tar.bz2`
- **THEN** the dispatcher MUST extract the archive to a temporary directory
- **AND** reject entries with absolute paths, parent traversal, symlinks, hardlinks, or special files
- **AND** reject entries exceeding configured per-file or total uncompressed size limits
- **AND** if no explicit limits are configured, V1 MUST use the existing file-import defaults of 10MB per file and 50MB total archive content
- **AND** import extracted Markdown candidates through the same validation and strategy path as directory import
- **AND** clean up temporary files after success or failure

### Requirement: Smart import remains the strategy adapter

The system SHALL preserve smart import strategies while preventing them from bypassing core OKF validation.

#### Scenario: Strategy is applied after validation

- **WHEN** a candidate file is valid OKF Markdown
- **THEN** smart import MUST choose skip, overwrite, merge, or patch according to options and detected changes
- **AND** strategy output MUST be reflected in the import summary

#### Scenario: Detect-only does not write files or metadata

- **WHEN** running `okf add <source> --detect-only`
- **THEN** the dispatcher MUST report what would change
- **AND** it MUST NOT write knowledge files
- **AND** it MUST NOT persist metadata index changes

#### Scenario: Watch daemon uses shared import semantics

- **WHEN** watch daemon imports or updates a Markdown file
- **THEN** it MUST enforce the same concept validation and smart strategy semantics as `okf add`
- **AND** it MUST share the validation helper with the dispatcher when full dispatcher reuse would add unnecessary event-handling complexity
