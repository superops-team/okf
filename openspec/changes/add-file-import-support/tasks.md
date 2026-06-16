# Implementation Tasks

## Phase 1: Configuration Management (1 week)

### Task 1.1: Platform default path detection
- File: `pkg/okf/config.go`
- Description: Implement `GetPlatformDefault()` function that returns platform-appropriate default path
- Acceptance Criteria:
  - Returns `~/.okf/knowledge` on Linux
  - Returns `~/Library/Application Support/okf/knowledge` on macOS
  - Returns `%APPDATA%\okf\knowledge` on Windows
- Tests: Unit tests covering all platforms (via mock)
- Priority: High

### Task 1.2: Config file I/O
- File: `pkg/okf/config.go`
- Description: Implement `LoadConfig(path)` and `SaveConfig(cfg, path)` functions
- Acceptance Criteria:
  - LoadConfig returns default config if file doesn't exist
  - SaveConfig creates parent directories if needed
  - Uses YAML format for config file
- Tests: Config file read/write round-trip, missing file handling
- Priority: High

### Task 1.3: Path resolution logic
- File: `pkg/okf/config.go`
- Description: Implement `ResolveKnowledgeDir(cliDir)` with precedence handling
- Acceptance Criteria:
  - Priority: CLI > Env > Local KB > Platform Default
  - Detects existing `.okf/knowledge` in current directory
  - Creates directory if it doesn't exist
- Tests: All priority combinations, directory creation
- Priority: High

### Task 1.4: Unit tests for config
- File: `pkg/okf/config_test.go`
- Description: Comprehensive tests for config module
- Coverage: Platform detection, path resolution, config file I/O
- Priority: High

---

## Phase 2: File Import Core (1.5 weeks)

### Task 2.1: File collector
- File: `pkg/okf/import.go`
- Description: Recursively collect markdown files from directories
- Acceptance Criteria:
  - Only collects `.md` files
  - Recursive by default (no depth limit)
  - Returns relative paths from source directory
- Tests: Empty directory, nested directories, non-md files filtering
- Priority: High

### Task 2.2: Archive detection and extraction
- File: `pkg/okf/import.go`
- Description: Detect archive types and extract contents safely
- Acceptance Criteria:
  - Detects `.zip`, `.tar`, `.tar.gz`, `.tar.bz2`
  - Uses filepath.Clean() on extracted paths
  - Prevents path traversal attacks
  - Cleans up temp files after extraction
- Tests: Normal extraction, path traversal attempts, corrupt archives
- Priority: High

### Task 2.3: OKF format validation
- File: `pkg/okf/import.go`
- Description: Validate imported files against OKF specification
- Acceptance Criteria:
  - Checks for required frontmatter fields (`type`, `title`)
  - Uses existing parser module for validation
  - Returns specific error messages
- Tests: Valid OKF files, missing fields, invalid YAML
- Priority: High

### Task 2.4: File import logic
- File: `pkg/okf/import.go`
- Description: Copy validated files to knowledge base with proper permissions
- Acceptance Criteria:
  - Preserves directory structure for directory imports
  - Uses filename only for single file imports
  - Sets permissions: 0644 for files, 0755 for directories
  - Supports force overwrite option
- Tests: Single file import, directory import, overwrite behavior
- Priority: High

### Task 2.5: Unit tests for import
- File: `pkg/okf/import_test.go`
- Description: Comprehensive tests for import module
- Coverage: File collection, archive extraction, validation, import logic
- Priority: High

---

## Phase 3: CLI Commands (0.5 week)

### Task 3.1: Implement `okf add` command
- File: `cmd/okf/cmd_add.go`
- Description: Add CLI command for file/directory/archive import
- Acceptance Criteria:
  - Supports `-dir`, `-force`, `-dry-run`, `-silent` flags
  - Auto-detects input type (file/directory/archive)
  - Shows progress and summary
- Tests: CLI argument parsing, integration with import module
- Priority: High

### Task 3.2: Implement `okf config` command
- File: `cmd/okf/cmd_config.go`
- Description: Add CLI command for configuration management
- Acceptance Criteria:
  - `okf config list` shows all settings
  - `okf config get <key>` shows specific setting
  - `okf config set <key> <value>` persists setting
- Tests: CLI argument parsing, integration with config module
- Priority: High

---

## Phase 4: Integration & E2E Tests (0.5 week)

### Task 4.1: Integration tests
- File: `pkg/okf/integration_test.go`
- Description: Test end-to-end import scenarios
- Coverage: Directory import, archive extraction, error handling
- Priority: Medium

### Task 4.2: CLI end-to-end tests
- File: `cmd/okf/cmd_test.go`
- Description: Test CLI commands in real scenarios
- Coverage: `okf add`, `okf config` workflows
- Priority: Medium

### Task 4.3: Edge case tests
- Description: Test boundary conditions
- Coverage: Empty directories, malformed archives, permission issues, large imports
- Priority: Medium

---

## Phase 5: Documentation (0.5 week)

### Task 5.1: Update README
- Files: `README.md`, `README.zh-CN.md`
- Description: Add documentation for new commands
- Coverage: Installation, usage examples, configuration
- Priority: Low

---

## Acceptance Criteria Summary

### Functional
| # | Criteria | Status |
|---|----------|--------|
| 1 | `okf add <file>` imports a single file | ✅ |
| 2 | `okf add <directory>` imports all files recursively | ✅ |
| 3 | `okf add <archive.zip>` extracts and imports contents | ✅ |
| 4 | `okf add <archive.tar.gz>` handles compressed tar | ✅ |
| 5 | `okf add --dry-run` shows what would be imported | ✅ |
| 6 | `okf add --force` overwrites existing files | ✅ |
| 7 | `okf config list` shows all settings | ✅ |
| 8 | `okf config get knowledge_dir` shows path | ✅ |
| 9 | `okf config set knowledge_dir <path>` persists setting | ✅ |
| 10 | Platform defaults are correctly applied | ✅ |
| 11 | Configuration precedence works correctly | ✅ |
| 12 | Path traversal attacks are prevented | ✅ |
| 13 | Invalid OKF files are rejected with clear errors | ✅ |
| 14 | Import preserves directory structure | ✅ |

### Non-Functional
| # | Criteria | Status |
|---|----------|--------|
| 15 | Import 1000 files completes within 60 seconds | ✅ |
| 16 | Memory usage does not exceed 512MB | ✅ |
| 17 | Archive size limit of 50MB is enforced | ✅ |
| 18 | Files get permission 0644, directories 0755 | ✅ |

### Security
| # | Criteria | Status |
|---|----------|--------|
| 19 | Path traversal attacks are prevented | ✅ |
| 20 | Dangerous permission bits are stripped | ✅ |
| 21 | Suspicious paths trigger warnings | ✅ |

---

## Development Schedule

| Phase | Duration | Start | End |
|-------|----------|-------|-----|
| Phase 1: Configuration | 1 week | Day 1 | Day 5 |
| Phase 2: File Import Core | 1.5 weeks | Day 6 | Day 13 |
| Phase 3: CLI Commands | 0.5 week | Day 14 | Day 17 |
| Phase 4: Integration Tests | 0.5 week | Day 18 | Day 21 |
| Phase 5: Documentation | 0.5 week | Day 22 | Day 25 |
| **Total** | **~4 weeks** | **Day 1** | **Day 25** |

---

## Resource Requirements

| Role | Count | Responsibilities |
|------|-------|------------------|
| Senior Developer | 1 | Architecture, security review |
| Developer | 1-2 | Implementation, unit tests |
| QA | 1 | Integration testing, edge cases |

---

## Risk Register

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Windows path handling bugs | Medium | High | Cross-platform testing, CI on Windows |
| Archive decompression failures | Low | Medium | Graceful error handling, cleanup |
| Memory exhaustion from large archives | Low | High | Size limits, streaming extraction |
| Concurrent config file access | Low | Medium | Atomic writes, single-process assumption |

---

## Test Coverage Targets

| Module | Target Coverage |
|--------|-----------------|
| `pkg/okf/config.go` | 95% |
| `pkg/okf/import.go` | 90% |
| `cmd/okf/cmd_add.go` | 80% |
| `cmd/okf/cmd_config.go` | 80% |
| **Overall** | **85%** |