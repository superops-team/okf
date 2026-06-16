# File Import Support

This change adds support for importing files, directories, and archive files into the OKF knowledge base. It also adds configurable knowledge base paths with platform-specific defaults.

## Features

### 1. Configurable Knowledge Base Path

- Platform-specific default paths
- Environment variable override (`OKF_KNOWLEDGE_DIR`)
- CLI flag override (`-dir`)

### 2. File and Directory Import

- `okf add <file>` - Import single file
- `okf add <directory>` - Import directory recursively
- Preserves directory structure

### 3. Archive Extraction

- Automatic detection of ZIP, TAR, TAR.GZ, TAR.BZ2
- Automatic extraction and import
- Path traversal protection

### 4. Configuration Management

- `okf config list` - Show all settings
- `okf config get <key>` - Get specific setting
- `okf config set <key> <value>` - Set setting

## Usage Examples

```bash
# Import a single file
okf add documentation/api.md

# Import a directory recursively
okf add ./docs

# Import an archive
okf add knowledge_backup.zip

# Import with custom knowledge base path
okf add ./docs -dir /custom/knowledge/path

# Dry run to see what would be imported
okf add ./docs --dry-run

# Overwrite existing files
okf add ./docs --force
```

## Platform Defaults

| Platform | Default Path |
|----------|-------------|
| Linux | `~/.okf/knowledge` |
| macOS | `~/Library/Application Support/okf/knowledge` |
| Windows | `%APPDATA%\okf\knowledge` |

## Configuration Precedence

1. CLI `-dir` flag (highest)
2. `OKF_KNOWLEDGE_DIR` environment variable
3. Platform default path (lowest)