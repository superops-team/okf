---
type: documentation
title: OKF Lint Rules
description: Specification compliance checking rules for OKF v0.2, including error codes, severity levels, and strict mode.
tags: [okf, lint, validation, spec-compliance, v0.2]
status: stable
---

# OKF Lint Rules

The `pkg/lint` package checks knowledge bundles for specification compliance. Each rule is identified by an `OKF###` code with a fixed severity.

## Severity Levels

- `Error` - Must fix, violates a spec requirement
- `Warning` - Should fix, best practice violation
- `Info` - Informational, optional improvement

## Rule Codes

### OKF001 - Required Type
- Severity: Error
- Concept must have a non-empty `type` field.
- Suggestion: set `type` to one of `table`, `api`, `metric`, `concept`, `component`, `project`, `system`, `service`.

### OKF002 - Missing Title (recommended in v0.2)
- Severity: Warning (was Error in v0.1)
- `title` is optional in v0.2 and derived from the filename when missing, but a concise title is recommended for discoverability.

### OKF003 - Short Description
- Severity: Warning
- `description` should be at least `Config.MinDescriptionLength` characters (default 10).

### OKF004 - Type Naming Convention (informational)
- Severity: Info
- Lowercase is recommended for simple types; mixed-case is valid for spec-defined types such as `Attested Computation`.

### OKF005 - Invalid or Missing Generated At
- Severity: Warning
- `generated.at` is recommended but missing, or not a valid ISO 8601 timestamp (e.g. `2024-01-15T10:30:00Z`). Legacy `timestamp` is still accepted.

### OKF006 - Non-Lowercase Tags
- Severity: Warning
- Tags should be lowercase and must not contain spaces (use hyphens or underscores).

### OKF007 - Empty Content Body
- Severity: Warning
- The Markdown content body after the frontmatter should not be empty.

### OKF009 - Long Lines
- Severity: Warning
- Content lines must not exceed `Config.MaxLineLength` characters (default 240).

### OKF010 - Duplicate Tags
- Severity: Warning
- The same tag appears more than once in `tags`.

### OKF011 - Missing Required Tag
- Severity: Warning
- A tag listed in `Config.RequiredTags` is missing. No check is performed when `RequiredTags` is empty.

### OKF012 - Missing Sources (v0.2 §5.1)
- Severity: Warning
- `sources` is recommended to record provenance and credibility signals.

### OKF013 - Duplicate Title
- Severity: Warning
- Two or more concepts in the same bundle share the same `title`. Titles should be unique.

### OKF014 - Attested Computation Missing Runtime (v0.2 §10.2)
- Severity: Error
- Concepts with `type: Attested Computation` must provide a `runtime` field (e.g. `bigquery`, `dbt`, `python`).

### OKF015 - Invalid Stale After Format (v0.2 §6.2)
- Severity: Warning
- `stale_after` must be a valid `YYYY-MM-DD` date (e.g. `2026-12-31`).

### OKF016 - Legacy Timestamp (v0.2 §13.1)
- Severity: Info
- Legacy `timestamp` detected; consider migrating to `generated: {by: <actor>, at: <ISO8601>}`.

### OKF017 - Missing Verified (v0.2 §5.2)
- Severity: Info
- `verified` is recommended to elevate the trust tier beyond `unverified`; add events with `human:<id>` or `process:<id>` actors.

## Strict Mode

When `StrictMode` is enabled, warnings are treated as errors. Use the `-strict` flag in the CLI or set `Config.StrictMode = true`.

## Configuration

```go
cfg := lint.DefaultConfig()
cfg.MaxLineLength = 240         // default 240
cfg.MinDescriptionLength = 10   // default 10
cfg.RequiredTags = []string{"okf"}  // optional; empty by default
cfg.StrictMode = true           // warnings fail
```

## Usage

```go
cfg := lint.DefaultConfig()
result := lint.LintBundle(concepts, cfg)
if result.HasErrors() {
    // handle errors
}
```
