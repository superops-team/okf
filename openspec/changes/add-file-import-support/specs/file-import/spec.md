## ADDED Requirements

### Requirement: Knowledge base path is configurable with platform defaults

The system SHALL support configurable knowledge base paths with platform-appropriate defaults.

#### Scenario: Platform default paths are used when not configured

| Platform | Default Path | Environment Variable |
|----------|-------------|---------------------|
| Linux | `$HOME/.okf/knowledge` | `OKF_KNOWLEDGE_DIR` |
| macOS | `$HOME/Library/Application Support/okf/knowledge` | `OKF_KNOWLEDGE_DIR` |
| Windows | `%APPDATA%\okf\knowledge` | `OKF_KNOWLEDGE_DIR` |

- **WHEN** no explicit path is provided via CLI or environment
- **THEN** the system MUST use the platform-appropriate default path
- **AND** the system MUST create the directory if it doesn't exist

#### Scenario: Configuration is resolved in order of precedence

- **WHEN** resolving the knowledge base path
- **THEN** the system MUST use this precedence order:
  1. CLI `-dir` / `--dir` flag
  2. `OKF_KNOWLEDGE_DIR` environment variable
  3. Platform default path

#### Scenario: Config command shows current path

- **WHEN** running `okf config get knowledge_dir`
- **THEN** the system MUST output the resolved knowledge base path

---

### Requirement: Files and directories can be added to knowledge base

The system SHALL support importing files and directories into the knowledge base.

#### Scenario: Single file is imported

- **WHEN** running `okf add /path/to/file.md`
- **THEN** the system MUST copy the file to the knowledge base
- **AND** preserve original filename and directory structure relative to source
- **AND** parse and validate the file as an OKF concept

#### Scenario: Directory is imported recursively

- **WHEN** running `okf add /path/to/directory`
- **THEN** the system MUST recursively traverse the directory
- **AND** import all `.md` files matching OKF format
- **AND** preserve relative directory structure

#### Scenario: Import preserves frontmatter

- **WHEN** importing a file with YAML frontmatter
- **THEN** the system MUST preserve all frontmatter fields
- **AND** validate required fields (`type`, `title`) exist

---

### Requirement: Archive files are automatically extracted

The system SHALL detect and extract common archive formats.

#### Scenario: ZIP archive is extracted and imported

- **WHEN** running `okf add /path/to/archive.zip`
- **THEN** the system MUST detect it as a ZIP archive
- **AND** extract contents to a temporary directory
- **AND** recursively import all valid OKF files from the extraction
- **AND** clean up temporary files after import

#### Scenario: TAR archive with compression is handled

- **WHEN** running `okf add /path/to/archive.tar.gz`
- **OR** running `okf add /path/to/archive.tar.bz2`
- **THEN** the system MUST detect compression format
- **AND** decompress and extract contents
- **AND** import valid OKF files

#### Scenario: Archive extraction preserves structure

- **WHEN** extracting an archive containing subdirectories
- **THEN** the system MUST preserve directory structure relative to archive root
- **AND** only import files with `.md` extension

---

### Requirement: Import supports filtering and options

The system SHALL provide options to control import behavior.

#### Scenario: Dry run shows what would be imported

- **WHEN** running `okf add /path/to/files --dry-run`
- **THEN** the system MUST show a list of files that would be imported
- **AND** NOT make any changes to the knowledge base

#### Scenario: Force overwrite is supported

- **WHEN** running `okf add /path/to/file.md --force`
- **AND** a file with the same name already exists in the knowledge base
- **THEN** the system MUST overwrite the existing file

#### Scenario: Silent mode suppresses output

- **WHEN** running `okf add /path/to/files --silent`
- **THEN** the system MUST only output errors
- **AND** suppress informational messages about imported files

---

### Requirement: Config command manages knowledge base settings

The system SHALL provide commands to inspect and modify configuration.

#### Scenario: Show all configuration

- **WHEN** running `okf config list`
- **THEN** the system MUST display all configurable settings
- **AND** their current values

#### Scenario: Get specific configuration

- **WHEN** running `okf config get <key>`
- **THEN** the system MUST display only the specified configuration value

#### Scenario: Set configuration

- **WHEN** running `okf config set knowledge_dir /custom/path`
- **THEN** the system MUST persist the setting to the config file
- **AND** use this value as default for subsequent commands

---

### Requirement: Imported files are validated

The system SHALL validate imported files against OKF specification.

#### Scenario: Invalid OKF files are rejected with error

- **WHEN** attempting to import a file missing required frontmatter fields
- **THEN** the system MUST reject the file
- **AND** display a clear error message indicating which field is missing

#### Scenario: Validation errors don't block other imports

- **WHEN** importing a directory containing both valid and invalid files
- **THEN** the system MUST import valid files
- **AND** report errors for invalid files
- **AND** continue processing remaining files

---

### NON-FUNCTIONAL Requirements

#### Performance

- **SCENARIO**: Import 1000 files completes in reasonable time
  - **WHEN** importing a directory with 1000 valid OKF files
  - **THEN** the operation MUST complete within 60 seconds
  - **AND** memory usage MUST not exceed 512MB

#### Security

- **SCENARIO**: Archive extraction prevents path traversal
  - **WHEN** extracting an archive containing files with `..` in paths
  - **THEN** the system MUST sanitize paths to prevent directory traversal attacks
  - **AND** log a warning about suspicious archive content

- **SCENARIO**: Import respects file permissions
  - **WHEN** importing files
  - **THEN** the system MUST set reasonable default permissions (644 for files, 755 for directories)
  - **AND** NOT preserve potentially dangerous setuid/setgid bits