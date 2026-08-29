#!/usr/bin/env bash
# Manual mutation runner for pkg/convert/convert.go (Gauntlet layer: Mutation).
#
# Go has no mature mutation-testing default, so we persist a hand-rolled
# runner per the old-coder gauntlet procedure. Each mutant is applied to the
# REAL source by a unique string replacement (proved to have applied by an
# assertion that the original pattern was present), the convert suite must kill
# it, then the original is restored and re-verified with `git diff --exit-code`.
#
# Usage: tools/mutants.sh   (run from repo root)

set -euo pipefail

cd "$(dirname "$0")/.."
F="pkg/convert/convert.go"
BAK="$(mktemp)"
trap 'cp "$BAK" "$F"; rm -f "$BAK"' EXIT
cp "$F" "$BAK"

run() { # run <label> <original> <mutant> <go-test-args...>
  local label="$1" orig="$2" mut="$3"
  shift 3
  cp "$BAK" "$F"
  if ! grep -qF -- "$orig" "$F"; then
    echo "MUTANT-FAILED(apply): $label: original pattern not found"
    exit 1
  fi
  python3 - "$F" "$orig" "$mut" <<'PYEOF'
import sys
path, orig, mut = sys.argv[1], sys.argv[2], sys.argv[3]
s = open(path).read()
assert orig in s, "pattern missing"
open(path, "w").write(s.replace(orig, mut, 1))
PYEOF
  if go test "$@" >/dev/null 2>&1; then
    echo "MUTANT-SURVIVED: $label (go test $* did not kill it)"
    exit 1
  fi
  echo "killed: $label"
}

run "M1 drop ToLower in IsSupportedDocument" \
  "documentExts[strings.ToLower(filepath.Ext(path))]" \
  "documentExts[filepath.Ext(path)]" \
  "TestIsSupportedDocument_CaseInsensitive"

run "M2 remove input size guard" \
  "if opts.MaxInputBytes > 0 {" \
  "if false {" \
  "TestConvertInputTooLarge"

run "M3 invert no-text mapping" \
  "if isNoTextError(err) {" \
  "if !isNoTextError(err) {" \
  "TestConvertNoTextPDF"

run "M4 hardcode WrapConcept ctype" \
  'fmt.Sprintf("---\ntype: %s\ntitle: %q\ndescription: %q\n---\n%s\n", ctype, title, desc, body)' \
  'fmt.Sprintf("---\ntype: %s\ntitle: %q\ndescription: %q\n---\n%s\n", "source", title, desc, body)' \
  ./pkg/mcp/ -run TestMCPImportWithOverrides

# restore and prove nothing else changed
cp "$BAK" "$F"
if ! git diff --exit-code -- "$F" >/dev/null 2>&1; then
  echo "MUTANT-FAILED(restore): convert.go differs from HEAD after run"
  exit 1
fi
if ! go test ./pkg/convert/ >/dev/null 2>&1; then
  echo "MUTANT-FAILED(restore): suite not green after restore"
  exit 1
fi
echo "manual mutation: 4/4 killed (restore verified, suite green)"
