## ADDED Requirements

### Requirement: File analysis populates imports and symbols
The system SHALL populate imports and function/type metadata during file analysis instead of returning only file-level metadata.

#### Scenario: Existing regex fallback populates non-Go summaries
- **WHEN** a supported non-Go source file is analyzed
- **THEN** the summary MUST include best-effort imports and functions using language-specific fallback extraction, with duplicate names removed and deterministic ordering

#### Scenario: Concept content exposes extracted structure
- **WHEN** a concept is generated from a source file with imports or symbols
- **THEN** the concept body MUST include structured sections for imports and symbols, including enough metadata for humans and agents to navigate back to the source

### Requirement: Go files use AST parsing for symbol extraction
The system SHALL use Go standard library AST parsing for `.go` files.

#### Scenario: Go package metadata is extracted
- **WHEN** a valid Go file is analyzed
- **THEN** the summary MUST include package name, import paths, declared functions, methods, structs, interfaces, type aliases, exported status, and source line ranges

#### Scenario: Go methods preserve receiver information
- **WHEN** a Go file declares methods with value or pointer receivers
- **THEN** each method symbol MUST include receiver type, method name, exported status, and start/end line numbers

#### Scenario: Go parser errors degrade gracefully
- **WHEN** a Go file contains syntax errors
- **THEN** analysis MUST return a partial summary when possible, record a parse warning, and continue indexing other files without failing the whole repository generation

### Requirement: Symbol concepts are addressable
The system SHALL create or expose addressable symbol records for indexed code.

#### Scenario: Symbol identity is stable
- **WHEN** a function, method, type, or interface is indexed
- **THEN** its identity MUST include repository-relative file path, package name when available, symbol kind, receiver when available, symbol name, and source line range

#### Scenario: Symbol search returns source location
- **WHEN** a user searches for a symbol name
- **THEN** the result MUST include the symbol kind and `file_path:start_line-end_line` location so the user can open the relevant source quickly

### Requirement: Lightweight relationship graph is generated
The system SHALL generate relationships that are computable without full LSP or SSA.

#### Scenario: Import graph is available
- **WHEN** a repository is indexed
- **THEN** the system MUST record file-to-import and package-to-import relationships for supported languages, with Go package relationships derived from AST imports

#### Scenario: File-to-symbol ownership is available
- **WHEN** symbols are extracted from a file
- **THEN** the system MUST record each symbol as belonging to that file and package, enabling future impact analysis to start from changed files

### Requirement: Source content remains on-demand
The system SHALL not write full source file bodies into knowledge concepts by default.

#### Scenario: Concepts store navigation metadata instead of full source
- **WHEN** a concept is generated for a source file
- **THEN** it MUST store path, line ranges, summaries, imports, and symbols, but MUST not embed the complete source code unless an explicit configuration option enables it
