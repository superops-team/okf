#!/usr/bin/env bash
# Positive and negative controls for tools/okf-project-gate.sh.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATE="$ROOT/tools/okf-project-gate.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
  printf 'VERIFY-FAILED: %s\n' "$*" >&2
  exit 1
}

say() {
  printf '\n== %s ==\n' "$1"
}

expect_fail() {
  local label="$1"
  shift
  if "$@" >/tmp/okf-project-gate-verify.$$ 2>&1; then
    cat /tmp/okf-project-gate-verify.$$ >&2
    rm -f /tmp/okf-project-gate-verify.$$
    fail "$label unexpectedly passed"
  fi
  rm -f /tmp/okf-project-gate-verify.$$
  echo "negative control passed: $label"
}

say "current change gate"
"$GATE" --repo "$ROOT" --change-id add-okf-project-gate

say "build temporary valid fixture"
FIX="$WORK/fixture"
mkdir -p "$FIX/tools" "$FIX/openspec/changes/demo/specs/demo"
cp "$ROOT/tools/gauntlet.sh" "$FIX/tools/gauntlet.sh"
cat >"$FIX/go.mod" <<'EOF'
module example.invalid/okf-fixture

go 1.26.0

toolchain go1.26.7

require github.com/giraffesyo/downmark v0.10.0
EOF
cat >"$FIX/openspec/changes/demo/proposal.md" <<'EOF'
# Proposal

Demo proposal.
EOF
cat >"$FIX/openspec/changes/demo/design.md" <<'EOF'
# Design

Demo design.
EOF
cat >"$FIX/openspec/changes/demo/tasks.md" <<'EOF'
# Implementation Tasks

## Completion contract

Complete only after P0-P4 are done.

## P0
## P1
## P2
## P3
## P4
EOF
cat >"$FIX/openspec/changes/demo/conformance.md" <<'EOF'
# Conformance

| Scenario | Verification | Align |
|---|---|---|
| S01 | demo command | fully |
EOF
cat >"$FIX/openspec/changes/demo/specs/demo/spec.md" <<'EOF'
# Specification

## Requirement: Demo gate

### Scenario S01: Demo passes
- **WHEN** the demo gate runs
- **THEN** it passes
EOF

"$GATE" --repo "$FIX" --change-id demo

say "negative controls"
cp "$FIX/go.mod" "$FIX/go.mod.ok"
sed -i.bak 's/downmark v0.10.0/downmark v0.10.1/' "$FIX/go.mod"
expect_fail "downmark version drift" "$GATE" --repo "$FIX" --change-id demo
mv "$FIX/go.mod.ok" "$FIX/go.mod"

mv "$FIX/openspec/changes/demo/specs/demo/spec.md" "$FIX/openspec/changes/demo/specs/demo/spec.md.missing"
expect_fail "missing spec file" "$GATE" --repo "$FIX" --change-id demo
mv "$FIX/openspec/changes/demo/specs/demo/spec.md.missing" "$FIX/openspec/changes/demo/specs/demo/spec.md"

cp "$FIX/openspec/changes/demo/specs/demo/spec.md" "$FIX/openspec/changes/demo/specs/demo/spec.md.ok"
grep -v '^### Scenario' "$FIX/openspec/changes/demo/specs/demo/spec.md.ok" >"$FIX/openspec/changes/demo/specs/demo/spec.md"
expect_fail "missing Scenario structure" "$GATE" --repo "$FIX" --change-id demo
mv "$FIX/openspec/changes/demo/specs/demo/spec.md.ok" "$FIX/openspec/changes/demo/specs/demo/spec.md"

echo
echo "OKF PROJECT GATE VERIFY PASS"
