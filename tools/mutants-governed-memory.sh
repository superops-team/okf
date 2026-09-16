#!/usr/bin/env bash
# Manual mutation runner (Gauntlet layer L9c: governed-agent-memory).
#
# Each mutant is applied to real source by an exact replacement, killed by a
# targeted test, then all touched files are restored byte-for-byte.
#
# Covered packages:
#   pkg/memorymeta — governance normalization, code_refs glob matching,
#                    duplicate-check threshold & type filtering (M-GM1..GM6)
#   pkg/manifest   — governance sort rank, summary projection (M-GM2, M-GM7)
#   pkg/tool       — okf_context refs body resolution (M-GM8)

set -euo pipefail

cd "$(dirname "$0")/.."
GO=${GO:-go}
FILES=(
  "pkg/memorymeta/governance.go"
  "pkg/memorymeta/coderefs.go"
  "pkg/memorymeta/duplicate.go"
  "pkg/manifest/manifest.go"
  "pkg/tool/service.go"
)

BAKDIR="$(mktemp -d)"
restore_all() {
  local f
  for f in "${FILES[@]}"; do
    cp "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')" "$f" 2>/dev/null || true
  done
}
trap 'restore_all; rm -rf "$BAKDIR"' EXIT

for f in "${FILES[@]}"; do
  cp "$f" "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')"
done

KILLED=0
TOTAL=0

run() { # run <label> <file> <original> <mutant> <go-test-args...>
  local label="$1" file="$2" orig="$3" mut="$4"
  shift 4
  TOTAL=$((TOTAL + 1))
  restore_all
  if ! grep -qF -- "$orig" "$file"; then
    echo "MUTANT-FAILED(apply): $label: original pattern not found in $file"
    exit 1
  fi
  python3 - "$file" "$orig" "$mut" <<'PYEOF'
import sys
path, orig, mut = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, encoding="utf-8") as handle:
    source = handle.read()
assert orig in source, "pattern missing"
with open(path, "w", encoding="utf-8") as handle:
    handle.write(source.replace(orig, mut, 1))
PYEOF
  if "$GO" test "$@" >/dev/null 2>&1; then
    echo "MUTANT-SURVIVED: $label ($GO test $* did not kill it)"
    exit 1
  fi
  echo "killed: $label"
  KILLED=$((KILLED + 1))
}

# ---------- M-GM1: governance unknown value does not degrade to context --------
run "M-GM1 governance unknown returns raw string" \
  "pkg/memorymeta/governance.go" \
  'return GovernanceContext, fmt.Sprintf("unknown governance value %q; treating as context", s)' \
  'return GovernanceLevel(s), fmt.Sprintf("unknown governance value %q; treating as context", s)' \
  ./pkg/memorymeta/ -run TestGovernanceUnknownNonStrict

# ---------- M-GM2: govRank hold no longer sorts first --------
run "M-GM2 govRank hold returns 2 instead of 0" \
  "pkg/manifest/manifest.go" \
  $'case memorymeta.GovernanceHold:\n\t\treturn 0' \
  $'case memorymeta.GovernanceHold:\n\t\treturn 2' \
  ./pkg/manifest/ -run TestS06GovernanceSortActivation

# ---------- M-GM3: single-segment * crosses / boundary --------
run "M-GM3 wildcard segment treated as recursive **" \
  "pkg/memorymeta/coderefs.go" \
  'if seg == "**" {' \
  'if seg == "**" || strings.Contains(seg, "*") {' \
  ./pkg/memorymeta/ -run TestMatchCodeRefsSingleGlob

# ---------- M-GM4: ** depth limit removed --------
run "M-GM4 maxDoubleStarSpan raised to 100" \
  "pkg/memorymeta/coderefs.go" \
  'maxDoubleStarSpan      = 8 // each ** matches at most 8 path segments' \
  'maxDoubleStarSpan      = 100 // each ** matches at most 8 path segments' \
  ./pkg/memorymeta/ -run TestMatchCodeRefsRecursiveDepth

# ---------- M-GM5: Jaccard threshold comparison disabled --------
run "M-GM5 dupThreshold comparison hardcoded to 0" \
  "pkg/memorymeta/duplicate.go" \
  'if j < threshold {' \
  'if j < 0 {' \
  ./pkg/memorymeta/ -run TestCheckMemoryThresholdConfigurable

# ---------- M-GM6: durable type filtering bypassed --------
run "M-GM6 typeFilter no longer narrows candidateTypes" \
  "pkg/memorymeta/duplicate.go" \
  'if tf := strings.TrimSpace(typeFilter); tf != "" {' \
  'if tf := strings.TrimSpace(typeFilter); tf != "" && false {' \
  ./pkg/memorymeta/ -run TestCheckMemoryTypeFilter

# ---------- M-GM7: summary projection falls back to full shape --------
run "M-GM7 projectItem summary keeps full projectionMode" \
  "pkg/manifest/manifest.go" \
  $'case ModeSummary:\n\t\tit.Description = oneLineDescription(it.Description)\n\t\tit.projectionMode = ModeSummary' \
  $'case ModeSummary:\n\t\tit.Description = oneLineDescription(it.Description)\n\t\tit.projectionMode = ""' \
  ./pkg/manifest/ -run TestS28SummaryModeShape

# ---------- M-GM8: okf_context refs branch skips body resolution --------
run "M-GM8 Service.Context refs loop emptied" \
  "pkg/tool/service.go" \
  'for _, ref := range req.Refs {' \
  'for _, ref := range []string{} {' \
  ./pkg/tool/ -run TestServiceContextRefsReadsConceptBody

restore_all
for f in "${FILES[@]}"; do
  if ! cmp -s "$f" "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')"; then
    echo "MUTANT-FAILED(restore): $f differs from its pre-run state"
    exit 1
  fi
done
if ! "$GO" test ./pkg/memorymeta/ ./pkg/manifest/ ./pkg/tool/ >/dev/null 2>&1; then
  echo "MUTANT-FAILED(restore): suite not green after restore"
  exit 1
fi
echo "governed-memory mutation: $KILLED/$TOTAL killed (restore verified, suite green)"
