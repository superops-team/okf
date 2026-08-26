---
type: index
title: OKF Project Knowledge Base
description: Knowledge base documenting the OKF (Open Knowledge Format) project architecture, modules, and usage.
tags: [okf, knowledge-format, documentation, index]
okf_version: "0.2"
---

# OKF Project Knowledge Base

This knowledge base documents the OKF (Open Knowledge Format) project.

## Modules

- [Core Types](core-types.md) - v0.2 type system with provenance, trust, lifecycle
- [Parser](parser.md) - Markdown + YAML frontmatter parser with v0.1/v0.2 support
- [Lint Rules](lint.md) - Specification compliance checking rules
- [MCP Server](mcp-server.md) - Model Context Protocol server for agent integration
- [CLI Commands](cli.md) - Command-line interface reference

## Specification

OKF v0.2 adds:
- Provenance and sources tracking
- Trust tiers (unverified / machine-confirmed / human-reviewed)
- Lifecycle status (draft / stable / deprecated)
- Attested Computation type
- index.md and log.md special files
