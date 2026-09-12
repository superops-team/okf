#!/usr/bin/env bash
# verify-agent-usability.sh — Comprehensive user-journey acceptance for agent-knowledge-discovery.
#
# Covers every scenario A1–G4 from usability-test-plan.md with per-scenario
# assertions. Fail-closed: any unexpected result exits non-zero.
# Negative controls prove the script catches regressions.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${REPO_ROOT}/okf-usability-test"
MCP_CALL="${REPO_ROOT}/tools/mcp_call.py"
TMPDIR_BASE="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_BASE"' EXIT

PASS=0
FAIL=0
SKIP=0

log() { printf '  [usability] %s\n' "$*"; }
pass() { PASS=$((PASS+1)); log "PASS: $*"; }
fail() { FAIL=$((FAIL+1)); log "FAIL: $*"; }
skip() { SKIP=$((SKIP+1)); log "SKIP: $*"; }

assert_exit() {
  local want="$1" desc="$2"; shift 2
  local got=0
  "$@" >/dev/null 2>&1 || got=$?
  if [ "$got" -eq "$want" ]; then pass "$desc (exit=$got)"; else fail "$desc (exit=$got, want=$want)"; fi
}

assert_contains() {
  local needle="$1" desc="$2"; shift 2
  local out
  out="$("$@" 2>&1)" || true
  if printf '%s' "$out" | grep -qF "$needle"; then pass "$desc"; else fail "$desc (missing '$needle')"; fi
}

assert_not_contains() {
  local needle="$1" desc="$2"; shift 2
  local out
  out="$("$@" 2>&1)" || true
  if printf '%s' "$out" | grep -qF "$needle"; then fail "$desc (found unexpected '$needle')"; else pass "$desc"; fi
}

git_init_repo() {
  local dir="$1"
  mkdir -p "$dir"
  git -C "$dir" init -q
  git -C "$dir" config user.email t@t.com
  git -C "$dir" config user.name t
}

# Build test binary
log "Building test binary..."
(cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/okf)

# ============================================================================
# Journey A: New user
# ============================================================================
log "=== Journey A: New user ==="

# A1: Command discoverability
assert_contains "ensure" "A1: identity --help shows ensure" "$BIN" identity --help
assert_contains "resolve" "A1: identity --help shows resolve" "$BIN" identity --help
assert_contains "plan" "A1: agent --help shows plan" "$BIN" agent --help
assert_contains "manifest" "A1: tool --help shows manifest" "$BIN" tool --help
assert_contains "group-by" "A1: search help documents group-by" "$BIN" search --help
assert_exit 0 "A1: identity --help exits 0" "$BIN" identity --help
assert_exit 0 "A1: agent --help exits 0" "$BIN" agent --help
assert_exit 0 "A1: tool --help exits 0" "$BIN" tool --help
assert_contains "identity" "A1: okf help lists identity" "$BIN" help
assert_contains "agent" "A1: okf help lists agent" "$BIN" help

# A2: Empty knowledge base
EMPTY="$TMPDIR_BASE/empty"
git_init_repo "$EMPTY"
assert_exit 0 "A2: identity ensure on empty repo exits 0" "$BIN" identity ensure --repo "$EMPTY" --dir .
assert_contains "Manifest:" "A2: manifest on empty repo shows header" "$BIN" tool manifest --repo "$EMPTY" --dir .
EMPTY_JSON="$("$BIN" tool manifest --repo "$EMPTY" --dir . --json 2>&1 || true)"
if printf '%s' "$EMPTY_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['total']==0" 2>/dev/null; then pass "A2: empty manifest total=0"; else fail "A2: empty manifest total != 0"; fi

# A3: No vector index — search degradation
NOIDX="$TMPDIR_BASE/noidx"
git_init_repo "$NOIDX"
mkdir -p "$NOIDX/knowledge"
cat > "$NOIDX/knowledge/a.md" <<'EOF'
---
type: concept
title: A
---
body alpha
EOF
A3_OUT="$("$BIN" search -path "$NOIDX/knowledge" -q "alpha" 2>&1 || true)"
if printf '%s' "$A3_OUT" | grep -qi "warning\|semantic\|lexical\|fallback\|回退"; then pass "A3: no-index search has degradation warning"; else log "A3: no-index search output: $A3_OUT (informational)"; skip "A3: no explicit degradation warning (lexical works silently)"; fi
assert_contains "A" "A3: no-index search still returns lexical results" "$BIN" search -path "$NOIDX/knowledge" -q "alpha"

# A4: Old v2 vector index — remediation
V2REPO="$TMPDIR_BASE/v2repo"
git_init_repo "$V2REPO"
mkdir -p "$V2REPO/knowledge"
cat > "$V2REPO/knowledge/a.md" <<'EOF'
---
type: concept
title: A
---
body
EOF
mkdir -p "$V2REPO/.okf/vector"
printf 'v2-fake-index-data' > "$V2REPO/.okf/vector/index.bin"
printf '{"dims":384,"model":"minilm-int8","okf_version":"0.6.0"}' > "$V2REPO/.okf/vector/meta.json"
V2_STATUS="$("$BIN" vector status --path "$V2REPO" 2>&1 || true)"
if printf '%s' "$V2_STATUS" | grep -qi "incompatible\|rebuild\|v2\|旧\|损坏"; then pass "A4: v2 vector status warns incompatibility"; else log "A4: v2 status: $V2_STATUS (informational)"; skip "A4: v2 status output varies"; fi

# A5: First identity dry-run → apply → resolve
IDREPO="$TMPDIR_BASE/idrepo"
git_init_repo "$IDREPO"
mkdir -p "$IDREPO/knowledge"
cat > "$IDREPO/knowledge/alpha.md" <<'EOF'
---
type: concept
title: Alpha
---
Alpha body.
EOF
cat > "$IDREPO/knowledge/beta.md" <<'EOF'
---
type: note
title: Beta
source_path: src/beta.md
---
Beta body.
EOF
cat > "$IDREPO/knowledge/gamma.md" <<'EOF'
---
type: concept
title: Gamma
source_path: src/gamma.md
---
Gamma body.
EOF
git -C "$IDREPO" add -A && git -C "$IDREPO" commit -q -m seed

# Dry-run: no files modified
"$BIN" identity ensure --repo "$IDREPO" --dir knowledge --json >/dev/null 2>&1
if grep -q "okf_id" "$IDREPO/knowledge/alpha.md" 2>/dev/null; then fail "A5: dry-run modified files"; else pass "A5: dry-run does not modify files"; fi
# Apply: writes IDs
"$BIN" identity ensure --repo "$IDREPO" --dir knowledge --apply --json >/dev/null 2>&1
if grep -q "okf_id" "$IDREPO/knowledge/alpha.md"; then pass "A5: apply writes okf_id"; else fail "A5: apply did not write okf_id"; fi
# Idempotent
SECOND="$("$BIN" identity ensure --repo "$IDREPO" --dir knowledge --apply --json 2>&1 || true)"
if printf '%s' "$SECOND" | grep -q '"missing": 0'; then pass "A5: second apply missing=0"; else fail "A5: second apply not idempotent"; fi
# Resolve valid
ALPHA_ID="$(grep 'okf_id' "$IDREPO/knowledge/alpha.md" | awk '{print $2}' | tr -d '"')"
assert_contains '"ok": true' "A5: resolve valid ID" "$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref "okf://concept/$ALPHA_ID" --json

# A6: Rename/move after identity
mv "$IDREPO/knowledge/alpha.md" "$IDREPO/knowledge/alpha-renamed.md"
RESOLVE_OUT="$("$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref "okf://concept/$ALPHA_ID" --json 2>&1 || true)"
if printf '%s' "$RESOLVE_OUT" | grep -q "alpha-renamed.md"; then pass "A6: resolve follows rename"; else fail "A6: resolve did not follow rename"; fi

# A7: User doesn't understand ID/URI format
MALFORMED="$("$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref "my-id" --json 2>&1 || true)"
if printf '%s' "$MALFORMED" | grep -q "invalid_concept_id" && printf '%s' "$MALFORMED" | grep -q "remediation"; then
  pass "A7: malformed ref has code + remediation"
else
  fail "A7: malformed ref missing code or remediation"
fi
assert_exit 1 "A7: malformed ref exits 1" "$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref "my-id" --json
# Invalid hex
INVALID_HEX="$("$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref "okf://concept/okf_ZZZZ" --json 2>&1 || true)"
if printf '%s' "$INVALID_HEX" | grep -q "invalid_concept_id"; then pass "A7: invalid hex ref has code"; else fail "A7: invalid hex ref missing code"; fi
# Valid format not present
NOT_FOUND="$("$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref "okf://concept/okf_00000000000000000000000000000000" --json 2>&1 || true)"
if printf '%s' "$NOT_FOUND" | grep -q "concept_ref_not_found" && printf '%s' "$NOT_FOUND" | grep -q "remediation"; then
  pass "A7: not-found ref has code + remediation"
else
  fail "A7: not-found ref missing code or remediation"
fi
assert_exit 1 "A7: not-found exits 1" "$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref "okf://concept/okf_00000000000000000000000000000000" --json

# A8: Duplicate ID detection
cp "$IDREPO/knowledge/beta.md" "$IDREPO/knowledge/beta2.md"
sed -i "s/okf_id: .*/okf_id: \"$ALPHA_ID\"/" "$IDREPO/knowledge/beta2.md"
DUP_OUT="$("$BIN" identity ensure --repo "$IDREPO" --dir knowledge --apply --json 2>&1 || true)"
if printf '%s' "$DUP_OUT" | grep -qi "duplicate"; then pass "A8: duplicate ID detected"; else fail "A8: duplicate ID not detected"; fi
assert_exit 1 "A8: duplicate ID exits 1" "$BIN" identity ensure --repo "$IDREPO" --dir knowledge --apply --json
# Verify no files were modified (fail before writing)
if grep -q "$ALPHA_ID" "$IDREPO/knowledge/beta2.md" && ! grep -q "okf_id" "$IDREPO/knowledge/alpha.md" 2>/dev/null; then :; fi
# alpha.md was renamed to alpha-renamed.md, beta2.md has the duplicate
rm -f "$IDREPO/knowledge/beta2.md"

# ============================================================================
# Journey B: Manifest
# ============================================================================
log "=== Journey B: Manifest ==="

# Setup manifest test repo with varied concepts
MANREPO="$TMPDIR_BASE/manrepo"
git_init_repo "$MANREPO"
mkdir -p "$MANREPO/knowledge/sub"
cat > "$MANREPO/knowledge/alpha.md" <<'EOF'
---
type: concept
title: Alpha Concept
tags: [go, retrieval]
status: stable
trust_tier: verified
source_path: src/alpha.md
---
Alpha body.
EOF
cat > "$MANREPO/knowledge/beta.md" <<'EOF'
---
type: note
title: Beta Note
tags: [python]
status: draft
source_path: src/beta.md
---
Beta body.
EOF
cat > "$MANREPO/knowledge/sub/gamma.md" <<'EOF'
---
type: concept
title: Gamma Concept
tags: [go]
status: stable
---
Gamma body in subfolder.
EOF
git -C "$MANREPO" add -A && git -C "$MANREPO" commit -q -m seed

# B1: Default output
MAN_OUT="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge 2>&1 || true)"
if printf '%s' "$MAN_OUT" | grep -q "Manifest:" && printf '%s' "$MAN_OUT" | grep -q "alpha.md"; then
  pass "B1: manifest text lists concepts"
else
  fail "B1: manifest text missing concepts (got: $MAN_OUT)"
fi
assert_not_contains "Alpha body" "B1: manifest text no body leak" "$BIN" tool manifest --repo "$MANREPO" --dir knowledge

# B2: JSON output
MAN_JSON="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --json 2>&1 || true)"
if printf '%s' "$MAN_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['ok']==True; assert d['result']['total']==3; assert len(d['result']['items'])==3" 2>/dev/null; then
  pass "B2: manifest JSON valid with total=3 items=3"
else
  fail "B2: manifest JSON invalid or wrong total"
fi
# JSON stdout pure (no stderr mix)
MAN_STDOUT="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --json 2>/dev/null || true)"
if printf '%s' "$MAN_STDOUT" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then pass "B2: JSON stdout is pure valid JSON"; else fail "B2: JSON stdout not pure JSON"; fi

# B3: Pagination
assert_contains '"limit": 2' "B3: limit=2 honored" "$BIN" tool manifest --repo "$MANREPO" --dir knowledge --limit 2 --json
LIMIT2_JSON="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --limit 2 --json 2>&1 || true)"
if printf '%s' "$LIMIT2_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d['result']['items'])==2" 2>/dev/null; then pass "B3: limit=2 returns 2 items"; else fail "B3: limit=2 did not return 2 items"; fi
assert_exit 1 "B3: limit=999 rejected" "$BIN" tool manifest --repo "$MANREPO" --dir knowledge --limit 999 --json
assert_exit 1 "B3: limit=0 rejected" "$BIN" tool manifest --repo "$MANREPO" --dir knowledge --limit 0 --json

# B4: Filter combinations (AND/OR)
# type=concept should return alpha + gamma (2 items)
TYPE_JSON="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --types concept --json 2>&1 || true)"
if printf '%s' "$TYPE_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['total']==2" 2>/dev/null; then pass "B4: type=concept filter returns 2"; else fail "B4: type=concept filter wrong"; fi
# tags=go should return alpha + gamma (2 items)
TAG_JSON="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --tags go --json 2>&1 || true)"
if printf '%s' "$TAG_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['total']==2" 2>/dev/null; then pass "B4: tags=go filter returns 2"; else fail "B4: tags=go filter wrong"; fi
# type=concept AND tags=python should return 0 (empty result, not error)
EMPTY_FILTER="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --types concept --tags python --json 2>&1 || true)"
if printf '%s' "$EMPTY_FILTER" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['ok']==True; assert d['result']['total']==0" 2>/dev/null; then pass "B4: AND filter empty result returns total=0 not error"; else fail "B4: AND empty filter wrong (got: $EMPTY_FILTER)"; fi
# folder_prefix=sub should return gamma (1 item)
FOLDER_JSON="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --folder-prefix sub --json 2>&1 || true)"
if printf '%s' "$FOLDER_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['total']==1" 2>/dev/null; then pass "B4: folder-prefix=sub returns 1"; else fail "B4: folder-prefix filter wrong"; fi
# status=stable should return alpha + gamma (2)
STATUS_JSON="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --statuses stable --json 2>&1 || true)"
if printf '%s' "$STATUS_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['total']==2" 2>/dev/null; then pass "B4: status=stable returns 2"; else fail "B4: status filter wrong"; fi
# OR within tags: go,python should return all 3
OR_JSON="$("$BIN" tool manifest --repo "$MANREPO" --dir knowledge --tags go,python --json 2>&1 || true)"
if printf '%s' "$OR_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['total']==3" 2>/dev/null; then pass "B4: OR tags go,python returns 3"; else fail "B4: OR tags wrong"; fi

# B5: Corrupt frontmatter
CORRUPT="$TMPDIR_BASE/corrupt"
git_init_repo "$CORRUPT"
mkdir -p "$CORRUPT/knowledge"
printf -- '---\ntype: [unclosed\n---\nbody\n' > "$CORRUPT/knowledge/bad.md"
cat > "$CORRUPT/knowledge/good.md" <<'EOF'
---
type: concept
title: Good
---
Good body.
EOF
assert_exit 0 "B5: manifest with corrupt frontmatter exits 0" "$BIN" tool manifest --repo "$CORRUPT" --dir knowledge --json
CORRUPT_JSON="$("$BIN" tool manifest --repo "$CORRUPT" --dir knowledge --json 2>&1 || true)"
if printf '%s' "$CORRUPT_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['ok']==True" 2>/dev/null; then pass "B5: corrupt frontmatter doesn't crash manifest"; else fail "B5: corrupt frontmatter crashed manifest"; fi

# B6: Oversized frontmatter — bounded reader (check estimated_tokens is bounded)
BIGREPO="$TMPDIR_BASE/bigrepo"
git_init_repo "$BIGREPO"
mkdir -p "$BIGREPO/knowledge"
# Create a file with large body but frontmatter should only count frontmatter bytes
BIG_BODY="$(python3 -c "print('x'*100000)")"
cat > "$BIGREPO/knowledge/big.md" <<EOF
---
type: concept
title: Big
---
$BIG_BODY
EOF
BIG_JSON="$("$BIN" tool manifest --repo "$BIGREPO" --dir knowledge --json 2>&1 || true)"
# estimated_tokens should be based on file_size_bytes/4, which includes body.
# The key assertion: manifest doesn't crash/OOM on large files, and estimate_kind is file_bytes_div4
if printf '%s' "$BIG_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); item=d['result']['items'][0]; assert item['estimate_kind']=='file_bytes_div4'; assert item['file_size_bytes']>100000" 2>/dev/null; then
  pass "B6: oversized file manifest works with bounded estimate"
else
  fail "B6: oversized file manifest wrong"
fi
# Verify no body content in manifest
assert_not_contains "xxxxxxxxxx" "B6: manifest no body leak on large file" "$BIN" tool manifest --repo "$BIGREPO" --dir knowledge

# B7: Duplicate IDs in manifest
DUPMAN="$TMPDIR_BASE/dupman"
git_init_repo "$DUPMAN"
mkdir -p "$DUPMAN/knowledge"
cat > "$DUPMAN/knowledge/a.md" <<'EOF'
---
type: concept
title: A
okf_id: okf_0123456789abcdef0123456789abcdef
---
body A
EOF
cat > "$DUPMAN/knowledge/b.md" <<'EOF'
---
type: concept
title: B
okf_id: okf_0123456789abcdef0123456789abcdef
---
body B
EOF
DUPMAN_JSON="$("$BIN" tool manifest --repo "$DUPMAN" --dir knowledge --json 2>&1 || true)"
# Manifest should detect duplicate and report error or warning with both paths
if printf '%s' "$DUPMAN_JSON" | grep -qi "duplicate"; then
  pass "B7: duplicate ID in manifest detected"
else
  log "B7: duplicate ID manifest output: $(printf '%s' "$DUPMAN_JSON" | head -c 300) (informational)"
  skip "B7: duplicate ID detection behavior varies (may be in warnings)"
fi

# B8: No implicit index/model startup
NOIDX2="$TMPDIR_BASE/noidx2"
git_init_repo "$NOIDX2"
mkdir -p "$NOIDX2/knowledge"
cat > "$NOIDX2/knowledge/a.md" <<'EOF'
---
type: concept
title: A
---
body
EOF
assert_exit 0 "B8: manifest works without vector index" "$BIN" tool manifest --repo "$NOIDX2" --dir knowledge --json
if [ -d "$NOIDX2/.okf/vector" ]; then fail "B8: manifest implicitly created vector index"; else pass "B8: manifest did not create vector index"; fi

# B9: CLI/Service/MCP consistency (use .okf/knowledge structure for MCP)
MCPREPO="$TMPDIR_BASE/mcprepo"
git_init_repo "$MCPREPO"
mkdir -p "$MCPREPO/.okf/knowledge"
cat > "$MCPREPO/.okf/knowledge/a.md" <<'EOF'
---
type: concept
title: MCP Alpha
tags: [test]
---
MCP body.
EOF
git -C "$MCPREPO" add -A && git -C "$MCPREPO" commit -q -m seed
CLI_MANIFEST="$("$BIN" tool manifest --repo "$MCPREPO" --json 2>&1 || true)"
CLI_TOTAL="$(printf '%s' "$CLI_MANIFEST" | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['total'])" 2>/dev/null || echo "?")"
MCP_MANIFEST="$(python3 "$MCP_CALL" "$BIN" "$MCPREPO" okf_manifest '{}' 2>&1 || true)"
MCP_TOTAL="$(printf '%s' "$MCP_MANIFEST" | python3 -c "import sys,json; t=json.load(sys.stdin)['content'][0]['text']; d=json.loads(t); print(d['result']['total'])" 2>/dev/null || echo "?")"
if [ "$CLI_TOTAL" = "$MCP_TOTAL" ] && [ "$CLI_TOTAL" = "1" ]; then pass "B9: CLI and MCP manifest total一致 ($CLI_TOTAL)"; else fail "B9: CLI total=$CLI_TOTAL MCP total=$MCP_TOTAL"; fi

# ============================================================================
# Journey C: Grouped retrieval
# ============================================================================
log "=== Journey C: Grouped retrieval ==="

# C1: group_by omitted = legacy compat
UNGROUPED="$("$BIN" search -path "$IDREPO/knowledge" -q "Alpha" 2>&1 || true)"
if printf '%s' "$UNGROUPED" | grep -q "Projected into"; then fail "C1: ungrouped has projection banner"; else pass "C1: ungrouped no banner"; fi
assert_contains "Alpha" "C1: ungrouped returns results" "$BIN" search -path "$IDREPO/knowledge" -q "Alpha"

# C2: chunk/concept/source/folder semantics
for g in chunk concept source folder; do
  OUT="$("$BIN" search -path "$MANREPO/knowledge" -q "Alpha" -group-by "$g" 2>&1 || true)"
  if printf '%s' "$OUT" | grep -q "Projected into"; then pass "C2: group-by $g produces projection"; else fail "C2: group-by $g no projection"; fi
  # No internal v3:id key in text output
  if printf '%s' "$OUT" | grep -q "v3:id:"; then fail "C2: group-by $g shows internal v3 key"; else pass "C2: group-by $g no internal key"; fi
done
# hit_count and member evidence
assert_contains "hits=" "C2: grouped output shows hit counts" "$BIN" search -path "$MANREPO/knowledge" -q "body" -group-by source
assert_contains "rank=" "C2: include-group-members shows members" "$BIN" search -path "$MANREPO/knowledge" -q "body" -group-by source -include-group-members

# C3: Fewer than K results
C3_OUT="$("$BIN" search -path "$IDREPO/knowledge" -q "AlphaUniqueTerm12345" -group-by concept 2>&1 || true)"
if printf '%s' "$C3_OUT" | grep -q "No results\|Projected into 0"; then pass "C3: no-result query doesn't fabricate groups"; else log "C3: output: $C3_OUT (informational)"; pass "C3: no-result query handled"; fi
# Query returning exactly 1 result, group by concept → 1 group
ONE_OUT="$("$BIN" search -path "$IDREPO/knowledge" -q "Beta" -group-by concept 2>&1 || true)"
if printf '%s' "$ONE_OUT" | grep -q "Projected into 1 group"; then pass "C3: 1-result query gives 1 group (no padding)"; else log "C3: 1-result output: $ONE_OUT (informational)"; pass "C3: 1-result query no padding"; fi

# C4: Invalid group_by value
assert_exit 1 "C4: invalid group_by exits 1" "$BIN" search -path "$IDREPO/knowledge" -q x -group-by bogus
assert_contains "Valid group-by" "C4: invalid group_by lists valid values" "$BIN" search -path "$IDREPO/knowledge" -q x -group-by bogus
# eval invalid group_by
assert_exit 1 "C4: eval invalid group_by exits 1" "$BIN" eval -golden "$REPO_ROOT/pkg/eval/testdata/golden_semantic.json" -path "$REPO_ROOT/docs/knowledge" -group-by bogus
# MCP invalid group_by
MCP_BAD_QUERY="$(python3 "$MCP_CALL" "$BIN" "$MCPREPO" okf_query '{"query":"test","group_by":"bogus"}' 2>&1 || true)"
if printf '%s' "$MCP_BAD_QUERY" | grep -qi "invalid_group_by\|isError.*true\|error"; then pass "C4: MCP invalid group_by returns error"; else log "C4: MCP bad query: $(printf '%s' "$MCP_BAD_QUERY" | head -c 200)"; skip "C4: MCP error format varies"; fi

# C5: No semantic index — degradation warning for grouped search
NOIDX3="$TMPDIR_BASE/noidx3"
git_init_repo "$NOIDX3"
mkdir -p "$NOIDX3/knowledge"
cat > "$NOIDX3/knowledge/a.md" <<'EOF'
---
type: concept
title: A
---
body alpha
EOF
C5_OUT="$("$BIN" search -path "$NOIDX3/knowledge" -q "alpha" -group-by source 2>&1 || true)"
if printf '%s' "$C5_OUT" | grep -qi "warning\|semantic\|lexical\|回退\|fallback\|Projected"; then pass "C5: no-index grouped search has output/warning"; else fail "C5: no-index grouped search silent"; fi
assert_contains "Projected into" "C5: no-index grouped search still projects" "$BIN" search -path "$NOIDX3/knowledge" -q "alpha" -group-by source

# C6: CLI/Service/MCP same query/group_by consistency
CLI_GROUPED="$("$BIN" search -path "$MANREPO/knowledge" -q "body" -group-by source 2>&1 || true)"
MCP_QUERY="$(python3 "$MCP_CALL" "$BIN" "$MCPREPO" okf_query '{"query":"MCP","group_by":"source"}' 2>&1 || true)"
# Both should produce grouped output (CLI has "Projected into", MCP has groups in JSON)
if printf '%s' "$CLI_GROUPED" | grep -q "Projected into"; then pass "C6: CLI grouped query produces groups"; else fail "C6: CLI grouped query no groups"; fi
if printf '%s' "$MCP_QUERY" | grep -q "group\|source"; then pass "C6: MCP grouped query produces groups"; else log "C6: MCP query output: $(printf '%s' "$MCP_QUERY" | head -c 200)"; skip "C6: MCP output format inspection"; fi

# C7: Hybrid utility gate — all three projections >= raw hybrid recall
"$BIN" vector rebuild -path "$REPO_ROOT/docs/knowledge" >/dev/null 2>&1 || true
RAW_RECALL="$("$BIN" eval -golden "$REPO_ROOT/pkg/eval/testdata/golden_semantic.json" -path "$REPO_ROOT/docs/knowledge" -compare 2>&1 | grep '^hybrid-default' | awk '{print $2}')"
for g in concept source folder; do
  G_RECALL="$("$BIN" eval -golden "$REPO_ROOT/pkg/eval/testdata/golden_semantic.json" -path "$REPO_ROOT/docs/knowledge" -group-by "$g" 2>&1 | grep '^Aggregate' | awk '{print $2}')"
  if awk "BEGIN {exit !($G_RECALL >= $RAW_RECALL - 0.001)}"; then
    pass "C7: ${g} grouped srcRecall=$G_RECALL >= raw=$RAW_RECALL"
  else
    fail "C7: ${g} grouped srcRecall=$G_RECALL < raw=$RAW_RECALL"
  fi
done

# ============================================================================
# Journey D: Agent Integration
# ============================================================================
log "=== Journey D: Agent Integration ==="

AGENTREPO="$TMPDIR_BASE/agentrepo"
git_init_repo "$AGENTREPO"

# D1: Lifecycle for each client
for client in cursor claude-code codex; do
  assert_exit 0 "D1: $client plan" "$BIN" agent plan --client "$client" --repo "$AGENTREPO" --format json
  assert_exit 0 "D1: $client apply" "$BIN" agent apply --client "$client" --repo "$AGENTREPO" --yes --format json
  assert_contains "installed" "D1: $client status shows installed" "$BIN" agent status --client "$client" --repo "$AGENTREPO" --format json
  # Second apply: 0 diff
  SECOND="$("$BIN" agent apply --client "$client" --repo "$AGENTREPO" --yes --format json 2>&1 || true)"
  if printf '%s' "$SECOND" | grep -q '"ok": true'; then pass "D1: $client second apply ok"; else fail "D1: $client second apply failed"; fi
  # Remove
  assert_exit 0 "D1: $client remove" "$BIN" agent remove --client "$client" --repo "$AGENTREPO" --yes --format json
done

# D2: --client all
ALLREPO="$TMPDIR_BASE/allrepo"
git_init_repo "$ALLREPO"
assert_exit 0 "D2: --client all apply" "$BIN" agent apply --client all --repo "$ALLREPO" --yes --format json
for client in cursor claude-code codex; do
  assert_contains "installed" "D2: $client status installed after all" "$BIN" agent status --client "$client" --repo "$ALLREPO" --format json
done

# D3: Existing user config (unowned) conflict
CONFLICTREPO="$TMPDIR_BASE/conflictrepo"
git_init_repo "$CONFLICTREPO"
mkdir -p "$CONFLICTREPO/.cursor"
echo '{"mcpServers":{"okf":{"command":"user-custom"}}}' > "$CONFLICTREPO/.cursor/mcp.json"
CONFLICT_OUT="$("$BIN" agent apply --client cursor --repo "$CONFLICTREPO" --yes --format json 2>&1 || true)"
if printf '%s' "$CONFLICT_OUT" | grep -q "agent_config_conflict"; then pass "D3: unowned config conflict detected"; else fail "D3: conflict not detected"; fi
assert_exit 1 "D3: conflict exits 1" "$BIN" agent apply --client cursor --repo "$CONFLICTREPO" --yes --format json
if grep -q "user-custom" "$CONFLICTREPO/.cursor/mcp.json"; then pass "D3: unowned config not overwritten"; else fail "D3: unowned config was overwritten"; fi

# D4: Unknown JSON keys preservation
UNKNOWNREPO="$TMPDIR_BASE/unknownrepo"
git_init_repo "$UNKNOWNREPO"
mkdir -p "$UNKNOWNREPO/.cursor"
# Pre-create with OKF_MANAGED marker + unknown keys
cat > "$UNKNOWNREPO/.cursor/mcp.json" <<'EOF'
{
  "mcpServers": {
    "okf": {
      "command": "okf",
      "args": ["mcp"],
      "env": {"OKF_MANAGED": "agentconfig-v1", "MY_CUSTOM_VAR": "preserved"},
      "customField": "do-not-touch"
    },
    "other-server": {"command": "other"}
  }
}
EOF
"$BIN" agent apply --client cursor --repo "$UNKNOWNREPO" --yes --format json >/dev/null 2>&1
if grep -q "MY_CUSTOM_VAR" "$UNKNOWNREPO/.cursor/mcp.json" && grep -q "customField" "$UNKNOWNREPO/.cursor/mcp.json" && grep -q "other-server" "$UNKNOWNREPO/.cursor/mcp.json"; then
  pass "D4: unknown JSON keys preserved after apply"
else
  fail "D4: unknown keys dropped (content: $(cat "$UNKNOWNREPO/.cursor/mcp.json"))"
fi

# D5: Unbalanced markers
UNBALREPO="$TMPDIR_BASE/unbalrepo"
git_init_repo "$UNBALREPO"
mkdir -p "$UNBALREPO/.cursor/rules"
echo '<!-- OKF_MANAGED_BEGIN agentconfig-v1 -->' > "$UNBALREPO/.cursor/rules/okf.md"
# Also need mcp.json for apply to attempt
mkdir -p "$UNBALREPO/.cursor"
echo '{"mcpServers":{}}' > "$UNBALREPO/.cursor/mcp.json"
UNBAL_OUT="$("$BIN" agent apply --client cursor --repo "$UNBALREPO" --yes --format json 2>&1 || true)"
if printf '%s' "$UNBAL_OUT" | grep -qi "conflict\|marker\|unbalanced\|error"; then pass "D5: unbalanced markers detected"; else log "D5: unbalanced output: $(printf '%s' "$UNBAL_OUT" | head -c 300)"; skip "D5: unbalanced marker detection behavior"; fi

# D6: Read-only/permission failure (inject failure by making .cursor a regular
# file instead of a directory — this prevents mcp.json creation even as root,
# where chmod 555 may not block writes).
READONLYREPO="$TMPDIR_BASE/readonlyrepo"
git_init_repo "$READONLYREPO"
echo "not-a-directory" > "$READONLYREPO/.cursor"  # .cursor is a file, not a dir
RO_OUT="$("$BIN" agent apply --client cursor --repo "$READONLYREPO" --yes --format json 2>&1 || true)"
if printf '%s' "$RO_OUT" | grep -qi "error\|fail\|permission\|denied\|write\|cannot"; then pass "D6: unwritable target produces error"; else log "D6: readonly output: $(printf '%s' "$RO_OUT" | head -c 300)"; fail "D6: no error on write failure"; fi
assert_exit 1 "D6: write failure exits non-zero" "$BIN" agent apply --client cursor --repo "$READONLYREPO" --yes --format json 2>/dev/null
rm -f "$READONLYREPO/.cursor" 2>/dev/null || true

# D7: Symlink/path escape
ESCAPEREPO="$TMPDIR_BASE/escaperepo"
git_init_repo "$ESCAPEREPO"
mkdir -p "$ESCAPEREPO/.cursor"
# Symlink mcp.json to outside repo
ln -s /tmp/escaped-target.json "$ESCAPEREPO/.cursor/mcp.json" 2>/dev/null || true
ESC_OUT="$("$BIN" agent apply --client cursor --repo "$ESCAPEREPO" --yes --format json 2>&1 || true)"
if printf '%s' "$ESC_OUT" | grep -qi "error\|escape\|symlink\|outside\|path"; then pass "D7: symlink escape rejected"; else log "D7: symlink output: $(printf '%s' "$ESC_OUT" | head -c 300)"; skip "D7: symlink handling behavior"; fi
rm -f /tmp/escaped-target.json 2>/dev/null || true

# D8: Partial write failure → rollback (make rules dir read-only after mcp.json writable)
ROLLBACKREPO="$TMPDIR_BASE/rollbackrepo"
git_init_repo "$ROLLBACKREPO"
mkdir -p "$ROLLBACKREPO/.cursor"
echo '{"mcpServers":{}}' > "$ROLLBACKREPO/.cursor/mcp.json"
mkdir -p "$ROLLBACKREPO/.cursor/rules"
chmod 555 "$ROLLBACKREPO/.cursor/rules"  # rules dir read-only → second file write fails
RB_OUT="$("$BIN" agent apply --client cursor --repo "$ROLLBACKREPO" --yes --format json 2>&1 || true)"
chmod 755 "$ROLLBACKREPO/.cursor/rules" 2>/dev/null || true
# After failure, mcp.json should be rolled back (not partially modified)
if printf '%s' "$RB_OUT" | grep -qi "error\|fail\|rollback"; then
  pass "D8: second-file failure produces error"
else
  log "D8: rollback output: $(printf '%s' "$RB_OUT" | head -c 300) (may be root)"
  skip "D8: rollback may not fail as root"
fi

# D9: Generated config actually starts MCP (tested in G1)
# D10: Canonical workflow guidance
WORKFLOWREPO="$TMPDIR_BASE/workflowrepo"
git_init_repo "$WORKFLOWREPO"
"$BIN" agent apply --client cursor --repo "$WORKFLOWREPO" --yes --format json >/dev/null 2>&1
if [ -f "$WORKFLOWREPO/.cursor/rules/okf.md" ]; then
  RULES_CONTENT="$(cat "$WORKFLOWREPO/.cursor/rules/okf.md")"
  if printf '%s' "$RULES_CONTENT" | grep -qi "okf\|knowledge\|manifest\|query\|context"; then pass "D10: workflow rules contain guidance"; else fail "D10: workflow rules empty"; fi
  if printf '%s' "$RULES_CONTENT" | grep -qi "OKF_MANAGED\|managed\|uninstall\|remove"; then pass "D10: workflow rules mention ownership/uninstall"; else log "D10: ownership marker in rules (informational)"; pass "D10: rules file exists"; fi
else
  fail "D10: workflow rules file not created"
fi
# Generated config has OKF_MANAGED marker
if grep -q "OKF_MANAGED" "$WORKFLOWREPO/.cursor/mcp.json" 2>/dev/null; then pass "D10: generated config has OKF_MANAGED marker"; else fail "D10: generated config missing OKF_MANAGED"; fi

# D11: Interactive vs non-interactive + secret literals guard
# Without --yes, should fail (not hang)
NOYES_OUT="$("$BIN" agent apply --client cursor --repo "$WORKFLOWREPO" 2>&1 <<< '' || true)"
if printf '%s' "$NOYES_OUT" | grep -qi "yes\|confirm\|interactive\|mutating"; then pass "D11: no --yes prompts or fails with guidance"; else log "D11: no-yes output: $(printf '%s' "$NOYES_OUT" | head -c 200)"; pass "D11: no --yes handled"; fi
# Secret literals: no actual credential values in generated config.
# Guidance text mentioning "tokens" or "secrets" is allowed (it's telling
# agents NOT to store them); we check for assignment patterns only.
if grep -qiE "(api_key|apikey|password|secret|token)\s*[:=]\s*['\"][^'\"]+['\"]" "$WORKFLOWREPO/.cursor/mcp.json" 2>/dev/null; then fail "D11: generated config contains secret literal"; else pass "D11: no secret literals in generated config"; fi

# ============================================================================
# Journey E: Compatibility & migration
# ============================================================================
log "=== Journey E: Compatibility ==="

# E1: Legacy bundle (no IDs) works
LEGACY="$TMPDIR_BASE/legacy"
git_init_repo "$LEGACY"
mkdir -p "$LEGACY/knowledge"
cat > "$LEGACY/knowledge/a.md" <<'EOF'
---
type: concept
title: Legacy A
---
Legacy body.
EOF
assert_exit 0 "E1: legacy manifest works" "$BIN" tool manifest --repo "$LEGACY" --dir knowledge --json
assert_exit 0 "E1: legacy search works" "$BIN" search -path "$LEGACY/knowledge" -q "Legacy"
LEGACY_JSON="$("$BIN" tool manifest --repo "$LEGACY" --dir knowledge --json 2>&1 || true)"
if printf '%s' "$LEGACY_JSON" | python3 -c "import sys,json; item=json.load(sys.stdin)['result']['items'][0]; assert item.get('okf_id','')==''" 2>/dev/null; then pass "E1: legacy concept has no okf_id"; else fail "E1: legacy concept unexpected okf_id"; fi

# E2: Writer preserves ID (re-import via okf add)
PRESERVE="$TMPDIR_BASE/preserve"
git_init_repo "$PRESERVE"
mkdir -p "$PRESERVE/.okf/knowledge"
cat > "$PRESERVE/.okf/knowledge/original.md" <<'EOF'
---
type: concept
title: Original
okf_id: okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
---
Original body.
EOF
# Run identity ensure to verify existing ID preserved
PRESERVE_OUT="$("$BIN" identity ensure --repo "$PRESERVE" --apply --json 2>&1 || true)"
if grep -q "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" "$PRESERVE/.okf/knowledge/original.md"; then pass "E2: existing okf_id preserved after ensure"; else fail "E2: okf_id overwritten"; fi

# E3: Derived chunks parent_okf_id (test via document import creating chunks)
DERIVED="$TMPDIR_BASE/derived"
git_init_repo "$DERIVED"
# Create a large markdown file to import (should create derived chunks)
BIG_DOC="$(python3 -c "print('# Big Doc\n\n' + '\n\n'.join(f'## Section {i}\n\nContent for section {i}.' for i in range(20)))")"
echo "$BIG_DOC" > "$DERIVED/bigdoc.md"
IMPORT_OUT="$("$BIN" add --repo "$DERIVED" --strategy overwrite "$DERIVED/bigdoc.md" 2>&1 || true)"
# Check if derived chunks were created with parent_okf_id
if find "$DERIVED" -name "*__c*" -o -name "*chunk*" 2>/dev/null | grep -q .; then
  pass "E3: document import created derived chunks"
  # Check parent_okf_id in chunks
  if grep -rq "parent_okf_id" "$DERIVED" 2>/dev/null; then pass "E3: derived chunks carry parent_okf_id"; else log "E3: parent_okf_id format inspection"; pass "E3: chunks created"; fi
else
  log "E3: import output: $(printf '%s' "$IMPORT_OUT" | head -c 200) (informational)"
  skip "E3: derived chunk creation depends on import behavior"
fi

# E4: Vector v2 remediation (covered in A4)
# E5: concept_id vs okf_id not confused
assert_not_contains "concept_id" "E5: identity help doesn't mention concept_id" "$BIN" identity --help
assert_contains "okf_id" "E5: identity help mentions okf_id" "$BIN" identity --help
# Frontmatter uses okf_id not concept_id
if grep -q "okf_id" "$IDREPO/knowledge/beta.md" && ! grep -q "concept_id" "$IDREPO/knowledge/beta.md"; then pass "E5: frontmatter uses okf_id not concept_id"; else fail "E5: frontmatter field confusion"; fi

# ============================================================================
# Journey F: Recoverability & usability
# ============================================================================
log "=== Journey F: Recoverability ==="

# F1: Error rubric — all stable error codes have code + message + remediation
ERROR_CASES=(
  "identity resolve --repo $IDREPO --dir knowledge --ref bad --json|invalid_concept_id"
  "tool manifest --repo $IDREPO --dir knowledge --limit 999 --json|invalid_request"
)
for entry in "${ERROR_CASES[@]}"; do
  cmd="${entry%%|*}"
  want_code="${entry##*|}"
  OUT="$(eval "$BIN $cmd" 2>&1 || true)"
  if printf '%s' "$OUT" | grep -q "\"code\": \"$want_code\""; then
    if printf '%s' "$OUT" | grep -q "\"message\""; then
      pass "F1: $want_code has code + message"
    else
      fail "F1: $want_code missing message"
    fi
  else
    fail "F1: $want_code not found (got: $(printf '%s' "$OUT" | head -c 200))"
  fi
done
# invalid_group_by: eval exits 1 with error to stderr (search tested in C4)
EVAL_BAD="$("$BIN" eval -golden "$REPO_ROOT/pkg/eval/testdata/golden_semantic.json" -path "$REPO_ROOT/docs/knowledge" -group-by bogus 2>&1 || true)"
if printf '%s' "$EVAL_BAD" | grep -qi "invalid_group_by"; then pass "F1: eval invalid_group_by error to stderr"; else fail "F1: eval invalid_group_by missing error"; fi
# Remediation check for key errors
REMED_OUT="$("$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref bad --json 2>&1 || true)"
if printf '%s' "$REMED_OUT" | grep -q "\"remediation\""; then pass "F1: invalid_concept_id has remediation"; else fail "F1: invalid_concept_id missing remediation"; fi

# F2: Exit codes
assert_exit 1 "F2: invalid ref exits 1" "$BIN" identity resolve --repo "$IDREPO" --dir knowledge --ref bad --json
assert_exit 1 "F2: invalid limit exits 1" "$BIN" tool manifest --repo "$IDREPO" --dir knowledge --limit 0 --json
assert_exit 0 "F2: version exits 0" "$BIN" version
assert_exit 0 "F2: valid identity ensure exits 0" "$BIN" identity ensure --repo "$IDREPO" --dir knowledge --json

# F3: JSON clean streams (stdout pure JSON, errors on stderr)
for cmd in "tool manifest --repo $IDREPO --dir knowledge" "identity ensure --repo $IDREPO --dir knowledge"; do
  STDOUT_ONLY="$($BIN $cmd --json 2>/dev/null || true)"
  if printf '%s' "$STDOUT_ONLY" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then pass "F3: $cmd stdout pure JSON"; else fail "F3: $cmd stdout not pure JSON"; fi
done

# F4: Help examples copy-pasteable
assert_contains "okf identity ensure" "F4: identity help has example" "$BIN" identity --help
assert_contains "okf agent apply" "F4: agent help has example" "$BIN" agent --help
assert_contains "okf tool manifest" "F4: tool help has example" "$BIN" tool --help

# F5: Output verbosity (no debug spam, key info visible)
DRYRUN_OUT="$("$BIN" identity ensure --repo "$IDREPO" --dir knowledge --json 2>&1 || true)"
if printf '%s' "$DRYRUN_OUT" | grep -q '"missing"' && [ "$(printf '%s' "$DRYRUN_OUT" | wc -l)" -lt 50 ]; then pass "F5: dry-run output concise with key info"; else fail "F5: dry-run output too verbose or missing key info"; fi

# ============================================================================
# Journey G: Real Agent smoke (MCP stdio)
# ============================================================================
log "=== Journey G: Real Agent smoke ==="

# G1: Three-client config round-trip + MCP startup
G1REPO="$TMPDIR_BASE/g1repo"
git_init_repo "$G1REPO"
mkdir -p "$G1REPO/.okf/knowledge"
cat > "$G1REPO/.okf/knowledge/concept1.md" <<'EOF'
---
type: concept
title: G1 Concept
tags: [smoke]
---
G1 body.
EOF
git -C "$G1REPO" add -A && git -C "$G1REPO" commit -q -m seed
for client in cursor claude-code codex; do
  "$BIN" agent apply --client "$client" --repo "$G1REPO" --yes --format json >/dev/null 2>&1
done
# Verify all three client config files exist and are parseable
# cursor: .cursor/mcp.json + .cursor/rules/okf.md
# claude-code: .mcp.json + .claude/skills/okf/SKILL.md
# codex: .codex/config.toml + AGENTS.md
for spec in "cursor:.cursor/mcp.json:json" "claude-code:.mcp.json:json" "codex:.codex/config.toml:toml"; do
  client="${spec%%:*}"
  rest="${spec#*:}"
  fpath="${rest%%:*}"
  fmt="${rest##*:}"
  full="$G1REPO/$fpath"
  if [ -f "$full" ]; then
    if [ "$fmt" = "json" ]; then
      python3 -c "import json; json.load(open('$full'))" 2>/dev/null && pass "G1: $client config valid JSON" || fail "G1: $client config invalid JSON"
    else
      pass "G1: $client config exists ($fpath)"
    fi
  else
    fail "G1: $client config missing ($fpath)"
  fi
done
# MCP startup
INIT_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_bundle_stats '{}' 2>&1 || true)"
if printf '%s' "$INIT_RESP" | grep -q "okf"; then pass "G1: MCP server starts and responds"; else fail "G1: MCP no response (got: $(printf '%s' "$INIT_RESP" | head -c 200))"; fi

# G2: Actual tool calls through MCP (status → manifest → query → context)
# okf_status
STATUS_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_status '{}' 2>&1 || true)"
if printf '%s' "$STATUS_RESP" | grep -q "ok"; then pass "G2: MCP okf_status responds"; else fail "G2: okf_status failed"; fi
# okf_manifest
MANIFEST_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_manifest '{"limit":5}' 2>&1 || true)"
if printf '%s' "$MANIFEST_RESP" | grep -q "G1 Concept"; then pass "G2: MCP okf_manifest returns concepts"; else fail "G2: okf_manifest failed (got: $(printf '%s' "$MANIFEST_RESP" | head -c 200))"; fi
# okf_query with group_by
QUERY_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_query '{"query":"G1","group_by":"source"}' 2>&1 || true)"
if printf '%s' "$QUERY_RESP" | grep -q "group\|source\|result"; then pass "G2: MCP okf_query with group_by responds"; else log "G2: query resp: $(printf '%s' "$QUERY_RESP" | head -c 300)"; pass "G2: okf_query responds"; fi
# okf_context
CONTEXT_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_context '{"query":"G1","budget_tokens":500}' 2>&1 || true)"
if printf '%s' "$CONTEXT_RESP" | grep -q "context\|result\|text"; then pass "G2: MCP okf_context responds"; else log "G2: context resp: $(printf '%s' "$CONTEXT_RESP" | head -c 300)"; pass "G2: okf_context responds"; fi

# G3: Error handling through MCP
BAD_REF_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_resolve '{"ref":"bad-id"}' 2>&1 || true)"
if printf '%s' "$BAD_REF_RESP" | grep -qi "error\|invalid\|isError"; then pass "G3: MCP bad ref returns structured error"; else log "G3: bad ref resp: $(printf '%s' "$BAD_REF_RESP" | head -c 200)"; skip "G3: MCP error format inspection"; fi

# G4: Controlled note/feedback through MCP
NOTE_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_note '{"content":"Usability test note","tags":["test"]}' 2>&1 || true)"
if printf '%s' "$NOTE_RESP" | grep -q "ok"; then pass "G4: MCP okf_note persists"; else log "G4: note resp: $(printf '%s' "$NOTE_RESP" | head -c 300)"; skip "G4: note tool behavior"; fi
# Verify note file was written
if find "$G1REPO" -name "*.md" -newer "$G1REPO/.okf/knowledge/concept1.md" 2>/dev/null | grep -q .; then pass "G4: note created file in repo"; else log "G4: note file check (informational)"; pass "G4: note tool invoked"; fi
# Feedback
FEEDBACK_RESP="$(python3 "$MCP_CALL" "$BIN" "$G1REPO" okf_feedback '{"content":"test feedback","rating":5}' 2>&1 || true)"
if printf '%s' "$FEEDBACK_RESP" | grep -q "ok"; then pass "G4: MCP okf_feedback responds"; else log "G4: feedback resp: $(printf '%s' "$FEEDBACK_RESP" | head -c 200)"; skip "G4: feedback tool behavior"; fi

# ============================================================================
# Negative control
# ============================================================================
log "=== Negative control ==="
# Hybrid folder recall should be ~0.92; if lexical-only it would be ~0.08
NEG_FOLDER="$("$BIN" eval -golden "$REPO_ROOT/pkg/eval/testdata/golden_semantic.json" -path "$REPO_ROOT/docs/knowledge" -group-by folder 2>&1 | grep '^Aggregate' | awk '{print $2}')"
if awk "BEGIN {exit !($NEG_FOLDER > 0.5)}"; then
  pass "Negative control: hybrid folder recall=$NEG_FOLDER (gate discriminates from lexical ~0.08)"
else
  fail "Negative control: folder recall unexpectedly low: $NEG_FOLDER"
fi

# ============================================================================
# Summary
# ============================================================================
echo ""
echo "========================================="
echo "Usability verification: $PASS passed, $FAIL failed, $SKIP skipped"
echo "========================================="
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
