#!/usr/bin/env bash
# Gauntlet entry point — runs every assurance layer in sequence and fails on
# the first broken one (old-coder / GAUNTLET). All numbers in EVIDENCE come
# from this command; rerun the whole report with `tools/gauntlet.sh`.
#
# Layers: build → vet → staticcheck → tests(-race) → coverage(threshold)
#         → suite health(shuffle) → mutation → real execution(CLI smoke)
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

step "L3 tests + concurrency: go test ./... -race"
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

step "L9 mutation: tools/mutants.sh"
bash tools/mutants.sh

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
echo "GAUNTLET PASS: build/vet/staticcheck/tests(-race)/coverage(${COV}%)/shuffle/mutation/real-exec"
