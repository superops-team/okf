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
│  Env Variable      ──┼──> ConfigResolver.Resolve() ──> Final Path│
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
    // ... other config options
}
```

### ImportOptions Struct

```go
type ImportOptions struct {
    DryRun    bool
    Force     bool
    Silent    bool
    Recursive bool // default: true
}
```

### ImportResult Struct

```go
type ImportResult struct {
    TotalFiles     int
    ImportedFiles  int
    SkippedFiles   int
    FailedFiles    int
    Errors         []error
}
```

## Module Structure

```
pkg/
├── okf/
│   ├── config.go        # Configuration management
│   ├── config_test.go
│   └── import.go        # File import logic
└── cmd/okf/
    ├── cmd_add.go       # okf add command
    └── cmd_config.go    # okf config command
```

## API Design

### Configuration API

| Function | Purpose |
|----------|---------|
| `LoadConfig()` | Load config from file |
| `SaveConfig(cfg)` | Save config to file |
| `ResolveKnowledgeDir(flags)` | Resolve path with precedence |
| `GetPlatformDefault()` | Return platform-specific default |

### Import API

| Function | Purpose |
|----------|---------|
| `ImportFile(path, opts)` | Import single file |
| `ImportDirectory(path, opts)` | Import directory recursively |
| `ImportArchive(path, opts)` | Extract and import archive |
| `Import(path, opts)` | Auto-detect and import |

## CLI Commands

### `okf add`

```
okf add <path> [options]

Options:
  -dir PATH        Knowledge base directory (default: resolved config)
  -force           Overwrite existing files
  -dry-run         Show what would be imported
  -silent          Suppress informational output
```

### `okf config`

```
okf config list                    # Show all config
okf config get <key>               # Get specific config
okf config set <key> <value>       # Set config value
```

## Platform Default Paths

| Platform | Path |
|----------|------|
| Linux | `$HOME/.okf/knowledge` |
| macOS | `$HOME/Library/Application Support/okf/knowledge` |
| Windows | `%APPDATA%\okf\knowledge` |

## Security Considerations

1. **Path Traversal Protection**: Sanitize all paths during archive extraction
2. **Permission Handling**: Strip dangerous permission bits (setuid/setgid)
3. **Size Limits**: Consider adding limits on archive size to prevent DoS
4. **Validation**: Validate all imported files against OKF spec

## Error Handling

- **Invalid OKF Format**: Return specific error with line number and field name
- **Missing Permissions**: Clear error message about write access
- **Archive Corruption**: Handle decompression errors gracefully
- **Network Issues**: If fetching remote files (future enhancement)

## Testing Strategy

- **Unit Tests**: Test config resolution, path sanitization, validation
- **Integration Tests**: Test actual file/directory/archive imports
- **Platform Tests**: Verify platform defaults on different OS
- **Edge Cases**: Empty directories, malformed archives, permission issues