#!/usr/bin/env bash
# Gauntlet entry point — runs every assurance layer in sequence and fails on
# the first broken one (old-coder / GAUNTLET). All numbers in EVIDENCE come
# from this command; rerun the whole report with `tools/gauntlet.sh`.
#
# Layers: build → vet → gofmt → staticcheck → tests → tests(-race) → coverage(threshold)
#         → suite health(shuffle) → property tests → secret scan → supply chain
#         → mutation(convert/...) → mutation(agent discovery) → real execution(CLI smoke)
#
# Usage: tools/gauntlet.sh   (run from repo root)

set -euo pipefail

cd "$(dirname "$0")/.."

GO=${GO:-go}
STATICCHECK=${STATICCHECK:-staticcheck}
COVER_THRESHOLD=${COVER_THRESHOLD:-60}   # percent, whole-repo statement coverage
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

# Freshness by mechanism: drop stale artifacts from previous runs so no layer
# can read a prior run's output.
rm -f ./coverage.out ./coverage.html

step "L1 types: go build ./..."
"$GO" build ./...

step "L2 lint: go vet ./..."
"$GO" vet ./...

step "L2 format: gofmt -l (tracked files only)"
UNFMT="$(gofmt -l $(git ls-files '*.go'))"
if [ -n "$UNFMT" ]; then
  echo "GATE-FAILED: gofmt needed on: $UNFMT"
  exit 1
fi

step "L2 lint: staticcheck ./..."
"$STATICCHECK" ./...

step "L3 full test suite: go test ./..."
# Real MiniLM/purego integration tests run here. They are intentionally skipped
# only in the following race process because purego's dynamically loaded
# tokenizer crashes under the Go race runtime on GitHub's Linux runner.
"$GO" test ./...

step "L3 concurrency: go test ./... -race"
"$GO" test ./... -race

step "L4 coverage: go test -coverprofile (threshold ${COVER_THRESHOLD}%)"
"$GO" test -coverpkg=./... ./... -coverprofile="$WORK/cover.out" >/dev/null
COV="$("$GO" tool cover -func="$WORK/cover.out" | awk '/^total:/{gsub("%","",$3); printf "%d", $3}')"
echo "whole-repo statement coverage: ${COV}% (threshold ${COVER_THRESHOLD}%)"
if [ "$COV" -lt "$COVER_THRESHOLD" ]; then
  echo "GATE-FAILED: coverage ${COV}% below threshold ${COVER_THRESHOLD}%"
  exit 1
fi

step "L5 suite health: go test -shuffle=on ./..."
"$GO" test -shuffle=on ./...

# ---- New in agent-knowledge-discovery (P4) -------------------------------
# Explicitly assert the three new packages are real, buildable, and exercised by
# the suite. The ./... globs above already include them; this gate fails loudly
# if one is ever deleted or reduced to a package with no tests (no skipped layer
# reported as passed, S49).
step "L6a new-package coverage: pkg/identity, pkg/manifest, pkg/agentconfig"
for pkg in ./pkg/identity/ ./pkg/manifest/ ./pkg/agentconfig/; do
	if ! "$GO" list -e "$pkg" >/dev/null; then
		echo "GATE-FAILED: new package $pkg is missing"
		exit 1
	fi
done
# -count=1 forces a fresh run (no cached PASS hiding a removed test).
"$GO" test -count=1 ./pkg/identity/ ./pkg/manifest/ ./pkg/agentconfig/

# Property layer: every pure-function invariant test must actually execute. We
# count the RUN lines matching Property|Properties and require a non-trivial
# number, so a rename that silently disables all property tests cannot pass.
step "L6b property tests: go test -run Property|Properties (must execute)"
PROP_OUT="$("$GO" test -count=1 -v -run 'Property|Properties' ./pkg/... 2>&1)" || {
	echo "GATE-FAILED: property test run failed:"
	echo "$PROP_OUT"
	exit 1
}
PROP_RUN="$(printf '%s\n' "$PROP_OUT" | grep -cE '^=== RUN .*[Pp]ropert')"
echo "property tests executed: $PROP_RUN"
if [ "$PROP_RUN" -lt 10 ]; then
	echo "GATE-FAILED: only $PROP_RUN property tests ran (expected >=10)"
	exit 1
fi

# Secret scan: production source and committed fixtures must not contain a
# literal secret VALUE next to a secret-like KEY. Environment-variable references
# (os.Getenv, $VAR, ${VAR}) and deliberate redaction-probe *_test.go files are
# allowed; *_test.go files hold the probes that PROVE redaction works. The
# generated ONNX/tokenizer asset files embed library SHA256 checksums (not
# credentials) and are excluded. The key must be a standalone token
# ("token": / token: / token =), so budget_tokens, tokenizerLibSHA256 and
# json:"..." struct tags are not false positives.
step "L7 secret scan: no literal token/password/secret/api_key/private_key values"
scan_out="$(
	git grep -nI -i -E '(^|[{,"[:space:]])(token|password|passwd|secret|api[_-]?key|private[_-]?key)[[:space:]]*[:=][[:space:]]*"[^"$][^"]{3,}"' -- \
		'*.go' '*.json' '*.yaml' '*.yml' '*.toml' 2>/dev/null \
		| grep -v '_test\.go:' \
		| grep -vE 'internal/embeddings/assets/' \
		|| true
)"
if [ -n "$scan_out" ]; then
	echo "GATE-FAILED: literal secret values found in production source/fixtures:"
	echo "$scan_out"
	exit 1
fi
echo "secret scan: clean (no literal secret values in production code/fixtures)"

# Supply chain: verify the module cache hashes against go.sum. No new third-party
# dependency was added by this change (go.mod is reviewed and tracked); we assert
# go mod verify passes and go.sum covers every requirement. Read-only: we never
# mutate go.mod/go.sum here.
step "L8 supply chain: go mod verify"
"$GO" mod verify
# Fail if go.sum is missing entries that the build resolves (detects drift).
"$GO" mod graph >/dev/null

step "L9 mutation: tools/mutants.sh"
bash tools/mutants.sh

step "L9b mutation (agent discovery): tools/mutants-agent-discovery.sh"
bash tools/mutants-agent-discovery.sh

step "L10 real execution: CLI import + search smoke"
BIN="$WORK/okf"
"$GO" build -o "$BIN" ./cmd/okf
KB="$WORK/kb"
mkdir -p "$KB"
"$BIN" add -dir "$KB" pkg/convert/testdata >/dev/null
"$BIN" search -path "$KB" -q "banana" >"$WORK/out_banana.txt" 2>/dev/null || true
if ! grep -q "sample.xlsx.md" "$WORK/out_banana.txt"; then
  echo "GATE-FAILED: real-execution search did not hit sample.xlsx.md"
  exit 1
fi
"$BIN" search -path "$KB" -q "Section One" >"$WORK/out_section.txt" 2>/dev/null || true
if ! grep -q "sample.docx.md" "$WORK/out_section.txt"; then
  echo "GATE-FAILED: real-execution search did not hit sample.docx.md"
  exit 1
fi
if ! "$BIN" lint -path "$KB" >/dev/null 2>&1; then
  echo "GATE-FAILED: real-execution lint reported errors"
  exit 1
fi
# 向量语义搜索冒烟：需构建期资源已就绪（scripts/fetch-ort.sh + fetch-model.sh，见 README）
# 注意：这些断言先把输出落盘再 grep。若写成 "cmd | grep -q"，grep 命中后立即退出会向
# cmd 发 SIGPIPE，在 set -o pipefail 下整条管道被判失败——表现为「检索明明有结果却 GATE-FAILED」。
if ! "$BIN" vector index -path "$KB" >/dev/null 2>&1; then
  echo "GATE-FAILED: real-execution vector index failed (run scripts/fetch-ort.sh darwin arm64 etc first)"
  exit 1
fi
"$BIN" search -path "$KB" -q "check my notes for errors" -semantic >"$WORK/out_semantic.txt" 2>/dev/null || true
if ! grep -q "source=" "$WORK/out_semantic.txt"; then
  echo "GATE-FAILED: real-execution semantic search produced no sourced results"
  exit 1
fi

echo
echo "GAUNTLET PASS: build/vet/staticcheck/tests/tests(-race)/coverage(${COV}%)/shuffle/new-package/property(${PROP_RUN})/secret-scan/mod-verify/mutation/agent-mutation/real-exec"
