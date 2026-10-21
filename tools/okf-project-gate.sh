#!/usr/bin/env bash
# Lightweight OKF-specific project admission gate.
#
# This is intentionally read-only. It validates cheap repository and current
# OpenSpec-change invariants before the heavyweight gauntlet spends time on
# build/test/mutation/real-execution layers.

set -euo pipefail

REPO="."
CHANGE_ID=""

usage() {
  cat <<'EOF'
Usage: tools/okf-project-gate.sh [--repo <path>] [--change-id <id>]

Checks:
  - core OKF repository invariants (Go/toolchain/downmark and gauntlet layers)
  - explicit OpenSpec change shape when --change-id is provided
  - inferred current OpenSpec changes from git status when --change-id is omitted
EOF
}

fail() {
  printf 'OKF-GATE-FAILED: %s\n' "$*" >&2
  exit 1
}

step() {
  printf '\n== %s ==\n' "$1"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo)
      [ "$#" -ge 2 ] || fail "--repo requires a value"
      REPO="$2"
      shift 2
      ;;
    --change-id)
      [ "$#" -ge 2 ] || fail "--change-id requires a value"
      CHANGE_ID="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[ -d "$REPO" ] || fail "repository path does not exist: $REPO"
cd "$REPO"

require_file() {
  [ -f "$1" ] || fail "missing required file: $1"
}

require_dir() {
  [ -d "$1" ] || fail "missing required directory: $1"
}

require_line() {
  local pattern="$1" file="$2" desc="$3"
  grep -Eq -- "$pattern" "$file" || fail "$desc not found in $file"
}

require_text() {
  local needle="$1" file="$2" desc="$3"
  grep -Fq -- "$needle" "$file" || fail "$desc not found in $file"
}

check_core_invariants() {
  step "core repository invariants"
  require_file "go.mod"
  require_file "tools/gauntlet.sh"

  require_line '^go 1\.26\.0$' "go.mod" "go directive go 1.26.0"
  require_line '^toolchain go1\.26\.7$' "go.mod" "toolchain go1.26.7"
  require_line 'github\.com/giraffesyo/downmark v0\.10\.0([[:space:]]|$)' "go.mod" "pinned downmark v0.10.0"

  require_text '"$GO" build ./...' "tools/gauntlet.sh" "gauntlet build layer"
  require_text '"$GO" vet ./...' "tools/gauntlet.sh" "gauntlet vet layer"
  require_text "gofmt -l \$(git ls-files '*.go')" "tools/gauntlet.sh" "gauntlet gofmt tracked-file layer"
  require_text '"$STATICCHECK" ./...' "tools/gauntlet.sh" "gauntlet staticcheck layer"
  require_text '"$GO" test ./...' "tools/gauntlet.sh" "gauntlet full test layer"
  require_text '"$GO" test ./... -race' "tools/gauntlet.sh" "gauntlet race layer"
  require_text '-coverpkg=./...' "tools/gauntlet.sh" "gauntlet coverage layer"
  require_text '"$GO" test -shuffle=on ./...' "tools/gauntlet.sh" "gauntlet shuffle layer"
  require_text 'bash tools/mutants.sh' "tools/gauntlet.sh" "gauntlet mutation layer"
  require_text 'real execution: CLI import + search smoke' "tools/gauntlet.sh" "gauntlet real execution layer"
  require_text '"$BIN" vector index -path "$KB"' "tools/gauntlet.sh" "gauntlet semantic real execution layer"
}

infer_changed_ids() {
  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    return 0
  fi
  git status --porcelain --untracked-files=all |
    awk '
      {
        path=$0
        sub(/^.../, "", path)
        if (path ~ /^openspec\/changes\/[^\/]+\//) {
          split(path, parts, "/")
          print parts[3]
        }
      }
    ' | sort -u
}

check_no_placeholders() {
  local file="$1"
  local hit
  hit="$(grep -nE '\b(TBD|TODO)\b|待补' "$file" | head -1 || true)"
  if [ -n "$hit" ]; then
    fail "placeholder text remains in $file: $hit"
  fi
}

check_change() {
  local id="$1"
  local dir="openspec/changes/$id"
  step "OpenSpec change: $id"
  require_dir "$dir"
  require_file "$dir/proposal.md"
  require_file "$dir/design.md"
  require_file "$dir/tasks.md"
  require_file "$dir/conformance.md"

  shopt -s nullglob
  local specs=("$dir"/specs/*/spec.md)
  shopt -u nullglob
  [ "${#specs[@]}" -gt 0 ] || fail "missing spec file under $dir/specs/*/spec.md"

  local spec_has_requirement=false
  local spec_has_scenario=false
  local spec
  for spec in "${specs[@]}"; do
    if grep -Eq '^## Requirement:' "$spec"; then
      spec_has_requirement=true
    fi
    if grep -Eq '^### Scenario' "$spec"; then
      spec_has_scenario=true
    fi
    check_no_placeholders "$spec"
  done
  [ "$spec_has_requirement" = true ] || fail "no spec contains '## Requirement:' for change $id"
  [ "$spec_has_scenario" = true ] || fail "no spec contains '### Scenario' for change $id"

  require_line 'Completion contract|完成边界|完成契约' "$dir/tasks.md" "tasks completion contract"
  local phase
  for phase in P0 P1 P2 P3 P4; do
    require_line "$phase" "$dir/tasks.md" "tasks dependency phase $phase"
  done

  require_line 'Scenario|场景' "$dir/conformance.md" "conformance scenario mapping"
  require_line 'Verification|验证' "$dir/conformance.md" "conformance verification mapping"
  require_line 'fully|aligned|partial|gap' "$dir/conformance.md" "conformance alignment status"

  check_no_placeholders "$dir/proposal.md"
  check_no_placeholders "$dir/design.md"
  check_no_placeholders "$dir/tasks.md"
  check_no_placeholders "$dir/conformance.md"
}

check_core_invariants

targets=()
if [ -n "$CHANGE_ID" ]; then
  targets=("$CHANGE_ID")
else
  while IFS= read -r id; do
    [ -n "$id" ] && targets+=("$id")
  done < <(infer_changed_ids)
fi

if [ "${#targets[@]}" -eq 0 ]; then
  step "OpenSpec change targets"
  echo "No explicit or modified OpenSpec change detected; core checks only."
else
  for id in "${targets[@]}"; do
    check_change "$id"
  done
fi

echo
echo "OKF PROJECT GATE PASS"
