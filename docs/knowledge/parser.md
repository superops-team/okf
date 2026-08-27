---
type: documentation
title: OKF Parser
description: Markdown + YAML frontmatter parser supporting OKF v0.1 and v0.2 with flexible generated/verified parsing and round-trip serialization.
tags: [okf, parser, yaml, markdown, v0.2, serialization]
status: stable
---

# OKF Parser

The parser reads Markdown files with YAML frontmatter and converts them to `parser.Concept` structs, and serializes concepts back to Markdown.

## Key Functions

### ParseConcept(path string) (*Concept, error)
Reads a concept from a file path.

### ParseConceptBytes(path string, data []byte) (*Concept, error)
Parses a concept from raw bytes (useful for testing).

### SerializeConcept(c *Concept, prettyPrint bool) ([]byte, error)
Serializes a concept back to Markdown with YAML frontmatter.

## v0.2 Parsing Features

### Flexible Generated/Verified
- `generated` can be a string (shorthand for `by`) or a mapping with `by` and `at`
- `verified` can be a bare mapping (converted to one-element list) or a list of verification events

### v0.1 Backward Compatibility
- Legacy `timestamp` field is preserved and mapped to `Generated.At` when `generated` is absent
- Concepts without v0.2 fields parse as v0.1 with default status `stable`

## Serialization Constraints

- Frontmatter uses `,inline` CustomFields map
- Any key matching a struct field name MUST be removed from the inline map, otherwise yaml.v3 panics
- Empty optional fields are omitted with `omitempty`

## Round-Trip Stability

The parser guarantees that `SerializeConcept(ParseConcept(data))` produces semantically equivalent output. A 100-iteration random round-trip stability test verifies this.
