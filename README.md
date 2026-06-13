<div align="right">

[English](README.md) | [中文](README.zh-CN.md)

</div>

# okf — Open Knowledge Format

> Project-level knowledge base system for AI Agents, with automatic Git repository scanning, specification linting, and automated updates.

## Features

- **📁 Open Knowledge Format** — Open knowledge format based on Markdown + YAML Frontmatter
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

## Quick Start

```bash
# Build the CLI
go build -o okf ./cmd/okf/

# Initialize knowledge base
cd /your/repo
./okf init

# Show knowledge base information
./okf show

# Search concepts
./okf search -q "database"

# Lint check
./okf lint

# Install Git Hook (automatic updates)
./okf hook -type post-commit
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

## License

Apache 2.0

---

<div align="center">

[⬆ Back to Top](#okf--open-knowledge-format) &nbsp;•&nbsp; [🇨🇳 切换到中文](README.zh-CN.md)

</div>
