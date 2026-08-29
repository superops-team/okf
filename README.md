<div align="right">
[English](README.md) | [中文](README.zh-CN.md)
</div>

# okf — Open Knowledge Format

> Project-level knowledge base system for AI Agents, with automatic Git repository scanning, specification linting, and automated updates.

[![Latest Release](https://img.shields.io/github/v/release/superops-team/okf?label=release&logo=github&style=flat-square)](https://github.com/superops-team/okf/releases)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-blue?style=flat-square)](#installation)
[![Go Version](https://img.shields.io/github/go-mod/go-version/superops-team/okf?logo=go&style=flat-square)](go.mod)

## Installation — Quick Start (30 seconds)

Pick one of these three install methods:

### 1. One-click installer (recommended)

**Linux / macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.ps1 | iex
```

> If the one-liner fails with `Unexpected token` / `&#34;` parse errors (caused by proxies HTML-encoding the response), use the download-then-run method:
> ```powershell
> iwr -useb "https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.ps1" -OutFile install.ps1; .\install.ps1
> ```

The installer:
- Automatically detects your OS (Linux / macOS) and CPU architecture (amd64 / arm64)
- Downloads the latest pre-built binary from GitHub Releases
- Verifies SHA256 checksums
- Installs to `/usr/local/bin/` (or `~/.local/bin/` without sudo)

### 2. Install via Go

```bash
go install github.com/superops-team/okf/cmd/okf@latest
```

### 3. Download from releases

Download pre-built binaries for your platform from the
[Releases](https://github.com/superops-team/okf/releases) page.

| OS | Architecture | Archive |
|----|-------------|---------|
| Linux | amd64 (x86_64) | `okf_<version>_linux_amd64.tar.gz` |
| Linux | arm64 (aarch64) | `okf_<version>_linux_arm64.tar.gz` |
| macOS | amd64 (Intel) | `okf_<version>_darwin_amd64.tar.gz` |
| macOS | arm64 (Apple Silicon) | `okf_<version>_darwin_arm64.tar.gz` |
| Windows | amd64 | `okf_<version>_windows_amd64.zip` |
| Windows | arm64 | `okf_<version>_windows_arm64.zip` |

---

## Features

- **📁 Open Knowledge Format** — Open knowledge format based on Markdown + YAML Frontmatter
- **📄 Document Import** — Import PDF, DOCX, XLSX, PPTX, HTML, CSV, TXT directly (pure-Go conversion, no Python/CGO); `okf add report.pdf` just works
- **🔍 Auto-Generation** — Automatically generates knowledge base by scanning Git repository source code
- **⚡ Incremental Updates** — Incremental updates based on Git commits
- **🛠 Git Hook** — One-click installation, automatic knowledge base updates on every commit
- **📋 Lint Checking** — Built-in specification compliance checker (13 rules)
- **🔎 Advanced Query** — Filter by type, tags, or full-text search
- **🏗 Modular Architecture** — Clean, layered design following Go best practices

## Project Structure

```
.
├── cmd/okf/          # CLI entry point
│   └── main.go      # Main application
├── pkg/
│   ├── okf/         # Core types and public API
│   │   ├── types.go # Concept, KnowledgeBundle definitions
│   │   ├── api.go   # LoadBundle, SaveBundle
│   │   ├── errors.go # Error types
│   │   ├── helpers.go # Helper functions
│   │   └── meta/    # Version information
│   ├── parser/      # Markdown + YAML parser
│   │   └── parser.go
│   ├── query/       # Query engine
│   │   └── query.go
│   ├── lint/        # Specification checker
│   │   └── lint.go
│   └── git/         # Git integration
│       ├── git.go       # Git operations
│       └── generator.go # Knowledge base generation
├── go.mod
├── README.md            # English version (default)
└── README.zh-CN.md      # Chinese version
```

## Usage

```bash
# Initialize knowledge base from your repo
cd /your/repo
okf init

# Show knowledge base information
okf show

# Search concepts
okf search -q "database"

# Lint check
okf lint

# Install Git Hook (automatic updates on every commit)
okf hook -type post-commit
```

## Module Reference

| Module | Path | Purpose |
|--------|------|---------|
| **okf** | pkg/okf/ | Core type definitions (Concept, KnowledgeBundle) and public API |
| **parser** | pkg/parser/ | Markdown + YAML frontmatter parsing and serialization |
| **query** | pkg/query/ | Advanced query builder and matching engine |
| **lint** | pkg/lint/ | OKF specification compliance checking (13 rules) |
| **git** | pkg/git/ | Git repository scanning, code analysis, knowledge base generation |

## OKF Concept Format

```markdown
---
type: table
title: users
description: User accounts table
resource: bigquery.project.dataset.users
tags:
  - production
  - pii
timestamp: "2024-01-15T10:30:00Z"
---

## Users Table
Stores all user account information.
```

## API Usage

```go
import (
    okf "github.com/superops-team/okf/pkg/okf"
    "github.com/superops-team/okf/pkg/git"
    "github.com/superops-team/okf/pkg/lint"
)

// Load knowledge base
bundle, err := okf.LoadBundle(".okf/knowledge", nil)

// Search concepts
results := bundle.Search("database")

// Lint check
result := lint.LintBundle(concepts, lint.DefaultConfig())

// Generate from Git
bundle, err := git.GenerateBundle(cfg, false)
```

## Lint Rules

| Code | Severity | Description |
|------|----------|-------------|
| OKF001 | ERROR | type field is required and must not be empty |
| OKF002 | ERROR | title field is required and must not be empty |
| OKF003 | WARNING | description is too short |
| OKF004 | WARNING | type should use lowercase alphanumeric |
| OKF005 | WARNING | timestamp format is invalid |
| OKF006 | WARNING | tags contain uppercase or spaces |
| OKF007 | WARNING | content body is empty |
| OKF009 | WARNING | content lines are too long |
| OKF010 | WARNING | duplicate tags found |

## Build & Test

```bash
# Build
go build ./...

# Build CLI
go build -o okf ./cmd/okf/

# Run all tests
go test ./...

# Run benchmarks
go test -bench=. -benchmem ./...
```

## OKF v0.2 Specification Support

This project implements the [OKF v0.2 specification](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md) with full backward compatibility for v0.1.

### What's New in v0.2

- **Provenance**: `sources` field with material references, usage counts, and credibility signals
- **Trust**: `generated` (by/at) and `verified` (list of verification events) fields with trust tier derivation (unverified → machine-confirmed → human-reviewed)
- **Lifecycle**: `status` (stable/draft/deprecated) and `stale_after` (YYYY-MM-DD) fields
- **Attested Computation**: New concept type with `runtime`, `parameters`, `computation`, `executor`, and `attester` fields
- **Reserved filenames**: `index.md` (directory listing) and `log.md` (update history)
- **Only `type` is required**: `title` is now optional and derived from filename if missing

### v0.2 Frontmatter Example

```yaml
---
type: Attested Computation
title: Revenue for fiscal year
description: Total revenue computed from BigQuery
runtime: bigquery
computation: |
  SELECT SUM(amount) AS revenue FROM transactions
parameters:
  - name: fiscal_year
    type: string
    required: true
sources:
  - id: transactions-table
    resource: bigquery://project/dataset.transactions
    title: Transactions Table
    author: data-team
    usage_count: 150
generated:
  by: process:etl-pipeline
  at: 2026-01-15T10:30:00Z
verified:
  - by: human:finance-reviewer
    at: 2026-02-01T00:00:00Z
status: stable
stale_after: 2026-12-31
executor:
  resource: references/skills/run-on-bq.md
attester:
  resource: references/attesters/sql-equality.py
---
# Revenue for fiscal year

This concept represents the total revenue for the fiscal year...
```

### Backward Compatibility

- v0.1 `timestamp` field is automatically mapped to `generated.at`
- v0.1 body `# Citations` section is automatically extracted to `sources`
- Legacy `generated: true` (boolean) is preserved for backward compatibility
- All v0.1 concepts parse without errors in v0.2 mode

### Official Example

See [`examples/v0.2/income-statement/`](examples/v0.2/income-statement/) for the complete Appendix A income statement example from the spec.

## License

Apache 2.0

---

<div align="center">
[⬆ Back to Top](#okf--open-knowledge-format) &nbsp;•&nbsp; [🇨🇳 切换到中文](README.zh-CN.md)
</div>
