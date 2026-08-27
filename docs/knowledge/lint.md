---
type: documentation
title: OKF Lint Rules
description: Specification compliance checking rules for OKF v0.2, including error codes, severity levels, and strict mode.
tags: [okf, lint, validation, spec-compliance, v0.2]
status: stable
---

# OKF Lint Rules

The lint package checks knowledge bundles for specification compliance.

## Severity Levels

- `Error` - Must fix, violates spec requirement
- `Warning` - Should fix, best practice violation
- `Info` - Informational, optional improvement

## Rule Codes

### OKF001 - Missing Required Type
- Severity: Error
- Concept must have a non-empty `type` field

### OKF002 - Missing Title (downgraded in v0.2)
- Severity: Warning (was Error in v0.1)
- v0.2 makes title optional but recommended

### OKF003 - Invalid YAML Frontmatter
- Severity: Error
- Frontmatter cannot be parsed as valid YAML

### OKF004 - Missing Description (downgraded in v0.2)
- Severity: Warning (was Error in v0.1)
- Description is recommended but not required

### OKF005 - Empty Tags (downgraded in v0.2)
- Severity: Info (was Warning in v0.1)
- Tags are optional in v0.2

### OKF012 - Invalid Status (v0.2)
- Severity: Error
- `status` must be one of: draft, stable, deprecated

### OKF014 - Invalid Stale After Format (v0.2)
- Severity: Error
- `stale_after` must be YYYY-MM-DD format

### OKF015 - Stale Concept (v0.2)
- Severity: Warning
- Concept has `stale_after` date in the past

### OKF016 - Missing Runtime for Attested Computation (v0.2)
- Severity: Error
- Concepts with type "Attested Computation" must have a `runtime` field

### OKF017 - Invalid Trust Tier (v0.2)
- Severity: Error
- Verified actor format must be valid for trust tier determination

## Strict Mode

When `StrictMode` is enabled, warnings are treated as errors. Use the `-strict` flag in CLI or set `Config.StrictMode = true`.

## Usage

```go
cfg := lint.DefaultConfig()
result := lint.LintBundle(concepts, cfg)
if result.HasErrors() {
    // handle errors
}
```
