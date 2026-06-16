# Implementation Tasks

## Phase 1: Configuration Management

### Task 1.1: Add platform default path detection
- File: `pkg/okf/config.go`
- Description: Implement `GetPlatformDefault()` function that returns platform-appropriate default path
- Dependencies: None
- Priority: High

### Task 1.2: Implement config resolution logic
- File: `pkg/okf/config.go`
- Description: Implement `ResolveKnowledgeDir(flags)` with precedence handling (CLI > Env > Default)
- Dependencies: Task 1.1
- Priority: High

### Task 1.3: Add config file persistence
- File: `pkg/okf/config.go`
- Description: Implement `LoadConfig()` and `SaveConfig()` functions
- Dependencies: None
- Priority: High

## Phase 2: File Import Core

### Task 2.1: Implement file collector
- File: `pkg/okf/import.go`
- Description: Recursively collect files from directories
- Dependencies: None
- Priority: High

### Task 2.2: Implement archive detection and extraction
- File: `pkg/okf/import.go`
- Description: Detect archive types and extract contents safely
- Dependencies: None
- Priority: High

### Task 2.3: Implement file validation
- File: `pkg/okf/import.go`
- Description: Validate imported files against OKF specification
- Dependencies: Existing parser module
- Priority: High

### Task 2.4: Implement import logic
- File: `pkg/okf/import.go`
- Description: Copy validated files to knowledge base with path preservation
- Dependencies: Tasks 2.1, 2.2, 2.3
- Priority: High

## Phase 3: CLI Commands

### Task 3.1: Implement `okf add` command
- File: `cmd/okf/cmd_add.go`
- Description: Add CLI command for file/directory/archive import
- Dependencies: Phase 2
- Priority: High

### Task 3.2: Implement `okf config` command
- File: `cmd/okf/cmd_config.go`
- Description: Add CLI command for configuration management
- Dependencies: Phase 1
- Priority: High

## Phase 4: Testing

### Task 4.1: Unit tests for config
- File: `pkg/okf/config_test.go`
- Description: Test config resolution, platform defaults
- Dependencies: Phase 1
- Priority: Medium

### Task 4.2: Unit tests for import
- File: `pkg/okf/import_test.go`
- Description: Test file collection, archive extraction, validation
- Dependencies: Phase 2
- Priority: Medium

### Task 4.3: Integration tests
- File: `pkg/okf/integration_test.go`
- Description: Test end-to-end import scenarios
- Dependencies: Phase 2, Phase 3
- Priority: Medium

## Phase 5: Documentation

### Task 5.1: Update README with usage examples
- File: `README.md`, `README.zh-CN.md`
- Description: Add documentation for new commands
- Dependencies: Phase 3
- Priority: Low

## Acceptance Criteria

1. ✅ `okf add <file>` imports a single file
2. ✅ `okf add <directory>` imports all files recursively
3. ✅ `okf add <archive.zip>` extracts and imports contents
4. ✅ `okf add <archive.tar.gz>` handles compressed tar
5. ✅ `okf add --dry-run` shows what would be imported
6. ✅ `okf add --force` overwrites existing files
7. ✅ `okf config list` shows all settings
8. ✅ `okf config get knowledge_dir` shows path
9. ✅ `okf config set knowledge_dir <path>` persists setting
10. ✅ Platform defaults are correctly applied
11. ✅ Configuration precedence works correctly
12. ✅ Path traversal attacks are prevented
13. ✅ Invalid OKF files are rejected with clear errors
14. ✅ Import preserves directory structure