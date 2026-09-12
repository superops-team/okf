#!/usr/bin/env bash
# Targeted mutation runner for the agent-knowledge-discovery change (Gauntlet
# layer L9b). Complements tools/mutants.sh (which covers pkg/convert, pkg/chunk,
# pkg/lexical, pkg/query weighted-RRF).
#
# Each mutant is an exact string replacement applied to real source, killed by a
# targeted test, then every touched file is restored byte-for-byte. These are
# manual mutants (Go has no mature default mutation framework), mirroring the
# existing mutants.sh convention.
#
# Mutants (each must be killed):
#   M-AD1  identity: accept a duplicate okf_id            -> TestDuplicateStableID
#   M-AD2  manifest: read past the frontmatter delimiter  -> TestManifestDoesNotParseBody
#   M-AD3  project : allow an absolute path escape       -> TestProjectFolder
#   M-AD4  project : re-sum group score (not rep score)   -> TestProjectChunk (+family)
#   M-AD5  agentconfig: overwrite an unowned client entry -> TestAdapterConflicts / RemoveOwnership
#
# Negative controls: each run() applies a real defect and requires the targeted
# `go test` to FAIL (exit non-zero). A mutant that survives (test still passes)
# prints MUTANT-SURVIVED and exits 1. Restore is verified byte-for-byte and the
# affected packages are re-tested green before exit 0.

set -euo pipefail

cd "$(dirname "$0")/.."
GO=${GO:-go}
FILES=(
  "pkg/identity/identity.go"
  "pkg/manifest/reader.go"
  "pkg/query/project.go"
  "pkg/agentconfig/json.go"
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

# M-AD1: BuildRegistry must fail closed on duplicate okf_id. Removing the dup
# check lets the second concept silently overwrite the first in byID.
run "M-AD1 accept duplicate okf_id" \
  "pkg/identity/identity.go" \
  $'if existing, dup := byID[id]; dup {\n\t\t\treturn nil, ErrDuplicateConceptID.withPaths(existing.FilePath, c.FilePath).' \
  $'if false {\n\t\t\treturn nil, ErrDuplicateConceptID.withPaths(existing.FilePath, c.FilePath).' \
  ./pkg/identity/ -run TestDuplicateStableID

# M-AD2: the bounded frontmatter reader must stop at the closing delimiter.
# Disabling closingDelimIndex makes it read the multi-MiB body until the
# 256 KiB cap trips, which the instrumented bytes-read test catches.
run "M-AD2 read past frontmatter delimiter" \
  "pkg/manifest/reader.go" \
  "if region[i] == '-' && region[i+1] == '-' && region[i+2] == '-' {" \
  "if false && region[i] == '-' && region[i+1] == '-' && region[i+2] == '-' {" \
  ./pkg/manifest/ -run TestManifestDoesNotParseBody

# M-AD3: folder projection must reject absolute paths. Allowing an absolute path
# turns "/etc/passwd.md" into a "folder:/etc" key and suppresses the expected
# fallback warning.
run "M-AD3 allow absolute path escape" \
  "pkg/query/project.go" \
  "	if filepath.IsAbs(p) {
		return \"\", false
	}" \
  "	if false {
		return \"\", false
	}" \
  ./pkg/query/ -run TestProjectFolder

# M-AD4: the representative score must stay the first member's own score, never
# re-aggregated into a running sum. Accumulating onto the representative changes
# the exposed Representative.Score.
run "M-AD4 recompute group score as sum" \
  "pkg/query/project.go" \
  $'		acc.hits = append(acc.hits, h)\n\t\tacc.concepts[conceptIdentityKey(h)] = struct{}{}' \
  $'		acc.hits = append(acc.hits, h)\n\t\tacc.rep.Score += h.Score\n\t\tacc.concepts[conceptIdentityKey(h)] = struct{}{}' \
  ./pkg/query/ -run "TestProjectChunk|TestProjectSource|TestProjectDeterministic"

# M-AD5: inspectJSONMCP must conflict (not overwrite) a same-name mcpServers.okf
# entry that lacks the OKF ownership marker. Removing the check overwrites an
# unknown user config.
run "M-AD5 overwrite unowned client config" \
  "pkg/agentconfig/json.go" \
  "		if marker != OwnedMarkerValue {
			return nil, StatusConflict, ActionNoChange,
				errConflict(fmt.Sprintf(\"mcpServers.%s exists without the %s ownership marker\", serverName, OwnedMarkerKey))
		}" \
  "		if false {
			return nil, StatusConflict, ActionNoChange,
				errConflict(fmt.Sprintf(\"mcpServers.%s exists without the %s ownership marker\", serverName, OwnedMarkerKey))
		}" \
  ./pkg/agentconfig/ -run "TestAdapterConflicts|TestAdapterRemoveOwnership"

restore_all
for f in "${FILES[@]}"; do
  if ! cmp -s "$f" "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')"; then
    echo "MUTANT-FAILED(restore): $f differs from its pre-run state"
    exit 1
  fi
done
if ! "$GO" test ./pkg/identity/ ./pkg/manifest/ ./pkg/query/ ./pkg/agentconfig/ >/dev/null 2>&1; then
  echo "MUTANT-FAILED(restore): suite not green after restore"
  exit 1
fi
echo "agent-discovery mutation: $KILLED/$TOTAL killed (restore verified, suite green)"
