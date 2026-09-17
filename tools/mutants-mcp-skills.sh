#!/usr/bin/env bash
# Targeted mutation runner for add-mcp-skills-extension.
# Each mutant is applied by exact string replacement, killed by a targeted test,
# then all touched files are restored byte-for-byte.
#
# Mutants (S38):
#   MSK1  skip modern protocol version validation
#   MSK2  skip skills client capability gate
#   MSK3  bypass digest verification in registry
#   MSK4  allow missing resource in skill entry
#   MSK5  expose legacy okf_load_bundle in modern tool catalog
#   MSK6  leak OKF-MANAGED marker into portable skill

set -euo pipefail

cd "$(dirname "$0")/.."
GO=${GO:-go}

FILES=(
  "pkg/mcp/modern_protocol.go"
  "pkg/mcp/modern_handlers.go"
  "pkg/mcp/skills.go"
  "pkg/mcp/server.go"
  "pkg/agentconfig/workflow.go"
)

BAKDIR="$(mktemp -d)"
restore_all() {
  local f
  for f in "${FILES[@]}"; do
    cp "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')" "$f" 2>/dev/null || true
  done
}
trap restore_all EXIT

for f in "${FILES[@]}"; do
  cp "$f" "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')"
done

killed=0
total=0

run_mutant() {
  local name=$1 file=$2 old=$3 new=$4 testpkg=$5 testrun=$6
  total=$((total + 1))
  printf 'MSK %s: ' "$name"
  # Apply mutant
  if ! sed -i "s|$old|$new|" "$file"; then
    echo "APPLY_FAILED"
    return 1
  fi
  # Run targeted test
  if $GO test "$testpkg" -run "$testrun" -count=1 >/dev/null 2>&1; then
    echo "SURVIVED (test passed with mutant — bad)"
    restore_all
    return 1
  else
    echo "KILLED"
    killed=$((killed + 1))
  fi
  restore_all
}

echo "=== MCP Skills Extension Targeted Mutants ==="

# MSK1: skip version validation — change unsupported version check to always pass
run_mutant "version-validation" \
  "pkg/mcp/modern_protocol.go" \
  'if meta.ProtocolVersion != ModernProtocolVersion' \
  'if false \&\& meta.ProtocolVersion != ModernProtocolVersion' \
  "./pkg/mcp/" "TestModernMetaValidation"

# MSK2: skip skills capability gate — always return true
run_mutant "capability-gate" \
  "pkg/mcp/modern_handlers.go" \
  'if !meta.HasSkillsCapability()' \
  'if false \&\& !meta.HasSkillsCapability()' \
  "./pkg/mcp/" "TestModernSkillsRequireCapability"

# MSK3: tamper digest algorithm — use sha1 instead of sha256
run_mutant "digest-verify" \
  "pkg/mcp/skills.go" \
  'digest := sha256.Sum256(skillBytes)' \
  'digest := [32]byte{}; copy(digest[:], skillBytes[:32])' \
  "./pkg/mcp/" "TestSkillRegistryDigest"

# MSK4: tamper skill URI — change canonical URI
run_mutant "completeness" \
  "pkg/mcp/skills.go" \
  'uri := "skill://okf/SKILL.md"' \
  'uri := "skill://okf/WRONG.md"' \
  "./pkg/mcp/" "TestNewSkillRegistry"

# MSK5: expose legacy tool in modern — add okf_load_bundle to modern set
run_mutant "stateless-tool-filter" \
  "pkg/mcp/modern_handlers.go" \
  '"okf_status",' \
  '"okf_status","okf_load_bundle",' \
  "./pkg/mcp/" "TestModernToolsListHas11Tools"

# MSK6: leak OKF-MANAGED into portable skill
run_mutant "wrapper-leakage" \
  "pkg/agentconfig/workflow.go" \
  'b.WriteString("name: okf\\n")' \
  'b.WriteString("name: okf\\nOKF-MANAGED: agentconfig-v1\\n")' \
  "./pkg/agentconfig/" "TestRenderAgentSkillNoOwnershipMarkers"

echo ""
echo "=== Result: $killed/$total mutants killed ==="
if [ "$killed" -ne "$total" ]; then
  echo "MUTATION FAILURE: $((total - killed)) mutant(s) survived"
  exit 1
fi
echo "ALL MUTANTS KILLED"
