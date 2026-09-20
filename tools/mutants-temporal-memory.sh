#!/usr/bin/env bash
# Manual mutation runner (Gauntlet layer L9d: temporal-memory-relations).
#
# Each mutant is applied to real source by an exact replacement, killed by a
# targeted existing temporal test, then all touched files are restored
# byte-for-byte.
#
# Covered packages:
#   pkg/memorymeta — temporal view projection: extends/updates kind acceptance,
#                    proposed-source currentness, cycle detection, declined
#                    currentness leak, history ordering (T-M1,M2,M3,M5,M6)
#   pkg/tool       — ReviewMemory expected_state CAS (T-M4),
#                    approved-relation target_not_current preflight (T-M7)

set -euo pipefail

cd "$(dirname "$0")/.."
GO=${GO:-go}
FILES=(
  "pkg/memorymeta/temporal_view.go"
  "pkg/tool/memory_review.go"
  "pkg/tool/temporal_preflight.go"
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

# ---------- T-M1: extends accepted as updates (kind check dropped) ----------
# An extends edge with a single target builds an active approved-updates edge,
# making its target historical. S08 requires extends to never make the target
# historical.
run "T-M1 extends accepted as updates" \
  "pkg/memorymeta/temporal_view.go" \
  'if e.State != MemoryApproved || e.Relation.Kind != RelationUpdates || len(e.Relation.Targets) != 1 {' \
  'if e.State != MemoryApproved || len(e.Relation.Targets) != 1 {' \
  ./pkg/memorymeta/ -run TestS08LegacyAndExtendsCurrent

# ---------- T-M2: proposed/declined-source edges affect currentness ----------
# The approved-source guard is dropped, so a proposed-updates edge targets and
# suppresses the currentness of its approved target. S10 requires the target to
# stay current despite inactive proposed/declined updaters.
run "T-M2 proposed-source edge affects currentness" \
  "pkg/memorymeta/temporal_view.go" \
  'if e.State != MemoryApproved || e.Relation.Kind != RelationUpdates || len(e.Relation.Targets) != 1 {' \
  'if e.Relation.Kind != RelationUpdates || len(e.Relation.Targets) != 1 {' \
  ./pkg/memorymeta/ -run TestS10ProposedDeclinedInactive

# ---------- T-M3: directed cycle no longer marked ----------
# The cycle marking loop is neutralized, so a directed update component keeps
# InvalidReason empty. S13 requires the component to be isolated with
# memory_relation_cycle.
run "T-M3 directed cycle accepted (cycle DFS neutralized)" \
  "pkg/memorymeta/temporal_view.go" \
  $'for start := range cyclic {\n\t\tmarkReason(start, "memory_relation_cycle")\n\t}' \
  $'for start := range cyclic {\n\t\t_ = start\n\t}' \
  ./pkg/memorymeta/ -run TestS13Cycle

# ---------- T-M4: expected_state CAS comparison skipped in ReviewMemory ----------
# The compare-and-swap guard is neutralized, so a second concurrent review no
# longer sees ErrMemoryStateConflict. S29 requires exactly one winner and one
# conflict.
run "T-M4 ReviewMemory expected_state CAS skipped" \
  "pkg/tool/memory_review.go" \
  'if string(currentState) != expected {' \
  'if false && string(currentState) != expected {' \
  ./pkg/tool/ -run TestTemporalS29CASRace

# ---------- T-M5: declined leaks into the current view ----------
# The currentness predicate also accepts declined concepts, so a declined
# concept with no incoming edge becomes current. S10 requires declined concepts
# to never be current.
run "T-M5 declined leaks into current" \
  "pkg/memorymeta/temporal_view.go" \
  'e.Current = e.State == MemoryApproved && !targeted && e.InvalidReason == ""' \
  'e.Current = (e.State == MemoryApproved || e.State == MemoryDeclined) && !targeted && e.InvalidReason == ""' \
  ./pkg/memorymeta/ -run TestS10ProposedDeclinedInactive

# ---------- T-M6: history ordering destabilized ----------
# The history output loop emits newest->oldest instead of oldest->newest. S15
# pins the ordered chain from every member.
run "T-M6 history returns newest-first" \
  "pkg/memorymeta/temporal_view.go" \
  'for i := len(chain) - 1; i >= 0; i-- {' \
  'for i := 0; i < len(chain); i++ {' \
  ./pkg/memorymeta/ -run TestS15HistoryFromAnyMember

# ---------- T-M7: target_not_current preflight disabled ----------
# The chain-head requirement for an approved updates edge is neutralized, so a
# stale proposal targeting a superseded head approves. S30 requires the stale
# approve to fail with target_not_current.
run "T-M7 approved updates target_not_current check disabled" \
  "pkg/tool/temporal_preflight.go" \
  'if rel.Kind == memorymeta.RelationUpdates && !view.IsCurrent(targetID) {' \
  'if rel.Kind == memorymeta.RelationUpdates && false && !view.IsCurrent(targetID) {' \
  ./pkg/tool/ -run TestTemporalS30ApprovedThenTargetNotCurrent

restore_all
for f in "${FILES[@]}"; do
  if ! cmp -s "$f" "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')"; then
    echo "MUTANT-FAILED(restore): $f differs from its pre-run state"
    exit 1
  fi
done
if ! "$GO" test ./pkg/memorymeta/ ./pkg/tool/ >/dev/null 2>&1; then
  echo "MUTANT-FAILED(restore): suite not green after restore"
  exit 1
fi
echo "temporal-memory mutation: $KILLED/$TOTAL killed (restore verified, suite green)"
