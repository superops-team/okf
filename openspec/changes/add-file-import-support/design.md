## Overview

This design implements support for importing files, directories, and archive files into the OKF knowledge base. It also adds configurable knowledge base paths with platform-specific defaults.

## Architecture

### Configuration Management

```
┌─────────────────────────────────────────────────────────────────┐
│                      Configuration Resolution                    │
├─────────────────────────────────────────────────────────────────┤
│  CLI Flag (-dir)  ──┐                                          │
│                     │                                          │
│  Env Variable      ──┼──> ResolveKnowledgeDir() ──> Final Path │
│  (OKF_KNOWLEDGE_DIR) │                                          │
│                     │                                          │
│  Platform Default ──┘                                          │
└─────────────────────────────────────────────────────────────────┘
```

### File Import Flow

```
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│   Input Handler     │───>│   Archive Detector  │───>│   Extractor         │
│  (File/Directory/  │    │  (.zip/.tar/.tar.gz)│    │  (ZIP/TAR support)  │
│   Archive Path)     │    └─────────────────────┘    └─────────┬───────────┘
└─────────────────────┘                                          │
                                                                ▼
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│    Validator        │<───│    Importer         │<───│   File Collector    │
│ (OKF Spec Check)    │    │  (Copy/Persist)    │    │  (Recursive Walk)   │
└─────────┬───────────┘    └─────────────────────┘    └─────────────────────┘
          │
          ▼
┌─────────────────────┐
│     Reporter        │
│ (Output/Errors)     │
└─────────────────────┘
```

## Data Models

### Config Struct

```go
type Config struct {
    KnowledgeDir string `yaml:"knowledge_dir"`
}
```

### ImportOptions Struct

```go
type ImportOptions struct {
    DryRun    bool   // Preview mode, no changes
    Force     bool   // Overwrite existing files
    Silent    bool   // Suppress informational output
}
```

### ImportResult Struct

```go
type ImportResult struct {
    TotalFiles     int
    ImportedFiles  int
    SkippedFiles   int
    FailedFiles    int
    Errors         []ImportError
}

type ImportError struct {
    FilePath string
    Message  string
    Err      error
}
```

## Module Structure

```
pkg/
├── okf/
│   ├── config.go        # Configuration management (Load/Save/Resolve)
│   ├── config_test.go
│   ├── import.go        # File import logic (ImportFile/ImportDirectory/ImportArchive)
│   └── import_test.go
└── cmd/okf/
    ├── cmd_add.go       # okf add command
    └── cmd_config.go    # okf config command
```

## API Design

### Configuration API

| Function | Purpose |
|----------|---------|
| `LoadConfig(path)` | Load config from file (returns default if not exists) |
| `SaveConfig(cfg, path)` | Save config to file |
| `ResolveKnowledgeDir(cliDir)` | Resolve path with precedence: CLI > Env > Default |
| `GetPlatformDefault()` | Return platform-specific default path |

#### Path Resolution Rules

1. **CLI Flag** (`-dir`) - Highest priority
2. **Environment Variable** (`OKF_KNOWLEDGE_DIR`)
3. **Existing Local KB** - If `.okf/knowledge` exists in current directory
4. **Platform Default** - Lowest priority

### Import API

| Function | Purpose |
|----------|---------|
| `ImportFile(srcPath, dstBase, opts)` | Import single file, preserve relative path |
| `ImportDirectory(srcDir, dstBase, opts)` | Import directory recursively |
| `ImportArchive(srcPath, dstBase, opts)` | Extract and import archive contents |
| `Import(srcPath, dstBase, opts)` | Auto-detect type and import |

#### Path Handling Semantics

- **Single File** (`okf add /a/b/file.md`) → `<dst>/file.md`
- **Directory** (`okf add /a/b/`) → `<dst>/<relative_path>/file.md`
- **Archive** (`okf add archive.zip`) → `<dst>/<archive_content>/file.md`

## CLI Commands

### `okf add`

```
okf add <path> [options]

Options:
  -dir PATH        Knowledge base directory (default: resolved config)
  -force           Overwrite existing files
  -dry-run         Show what would be imported (no changes made)
  -silent          Suppress informational output (only show errors)

Examples:
  okf add document.md                    # Import single file
  okf add ./docs                        # Import directory recursively
  okf add backup.zip                    # Extract and import archive
  okf add ./docs -dir ~/knowledge       # Custom destination
```

### `okf config`

```
okf config list                    # Show all configuration
okf config get <key>               # Get specific configuration value
okf config set <key> <value>       # Set configuration value

Examples:
  okf config get knowledge_dir     # Show current knowledge base path
  okf config set knowledge_dir ~/kb # Set custom path
```

## Platform Default Paths

| Platform | Default Path |
|----------|------|
| Linux | `$HOME/.okf/knowledge` |
| macOS | `$HOME/Library/Application Support/okf/knowledge` |
| Windows | `%APPDATA%\okf\knowledge` |

## Security Considerations

### Path Traversal Protection
- **Sanitize all paths** using `filepath.Clean()` before extraction
- **Prefix check** - ensure resolved path is within destination directory
- **Strip leading slashes** from archive entries
- **Log warnings** for suspicious path patterns

### Resource Limits
- **Maximum archive size**: 50MB (configurable)
- **Maximum file count per import**: 10000
- **Maximum directory depth**: 10 levels
- **Single file size limit**: 10MB

### Permission Handling
- **Files**: 0644 (rw-r--r--)
- **Directories**: 0755 (rwxr-xr-x)
- **Strip dangerous bits**: Remove setuid/setgid from imported files

## Error Handling

### Error Types

| Type | Description | Recovery |
|------|-------------|----------|
| `ErrInvalidFormat` | Missing required frontmatter fields | Fix YAML frontmatter |
| `ErrPathTraversal` | Suspicious path in archive | Review archive content |
| `ErrPermissionDenied` | Cannot write to destination | Check directory permissions |
| `ErrArchiveCorrupt` | Cannot decompress archive | Verify archive integrity |
| `ErrFileTooLarge` | File exceeds size limit | Split or increase limit |

### Error Handling Strategy
- **Continue on error**: Invalid files don't block entire import
- **Aggregate errors**: Collect all errors and report at end
- **Clear messages**: Include file path and specific issue
- **Exit code**: Non-zero if any files failed to import

## Testing Strategy

### Unit Tests
- **Configuration**: Platform detection, path resolution, config file I/O
- **Path Handling**: Sanitization, traversal protection, boundary cases
- **Validation**: OKF format validation, error reporting
- **Archive Handling**: Format detection, extraction, cleanup

### Integration Tests
- **Full Import Flow**: Directory import, archive extraction
- **Error Scenarios**: Invalid files, permission issues, corrupt archives
- **Concurrency**: Config file concurrent access (future)

### End-to-End Tests
- **CLI Commands**: `okf add`, `okf config` complete workflows
- **Platform Compatibility**: Path handling on Linux/macOS/Windows
- **Edge Cases**: Empty directories, nested archives, large imports

### Test Utilities
- **Mock Environment**: Override platform detection for testing
- **Test Archives**: Embedded small test archives (ZIP/TAR)
- **Temp Directories**: Isolated test environments

## Backward Compatibility

### Migration Strategy
1. **Detect existing local KB**: Check `.okf/knowledge` in current directory
2. **Priority order**: Local KB > Config > Env > Default
3. **Migration prompt**: Suggest migrating to centralized location (optional)

### Deprecation Plan
- **Soft deprecation**: No breaking changes, just new defaults
- **Configuration override**: Users can always specify `-dir` or set env var
- **Documentation**: Update examples to reflect new behavior

## Extensibility Considerations

### Future Enhancements
- **Remote sources**: HTTP/HTTPS URL import
- **Additional formats**: RAR, 7z support
- **Transformation hooks**: Pre/post processing pipelines
- **Indexing options**: Control over what gets indexed

### Extension Points
- **Archive handlers**: Implement `ArchiveExtractor` interface for new formats
- **Validation hooks**: Add custom validators via configuration
- **Storage backends**: Abstract file system access for cloud storage