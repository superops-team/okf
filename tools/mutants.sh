#!/usr/bin/env bash
# Manual mutation runner (Gauntlet layer: Mutation).
#
# Go has no mature mutation-testing default, so we persist a hand-rolled
# runner per the old-coder gauntlet procedure. Each mutant is applied to the
# REAL source by a unique string replacement (proved to have applied by an
# assertion that the original pattern was present), the named test must kill
# it, then the original is restored and re-verified with `git diff --exit-code`.
#
# Covered packages:
#   pkg/convert  — document import (M1-M4)
#   pkg/chunk    — heading-aware chunking (M5-M8)
#   pkg/lexical  — tokenization + BM25 (M9-M12)
#   pkg/query    — weighted RRF fusion (M13-M14)
#
# Usage: tools/mutants.sh   (run from repo root)

set -euo pipefail

cd "$(dirname "$0")/.."

# Files touched by mutants; each is backed up and restored.
FILES=(
  "pkg/convert/convert.go"
  "pkg/chunk/chunk.go"
  "pkg/lexical/lexical.go"
  "pkg/query/semantic.go"
)

BAKDIR="$(mktemp -d)"
restore_all() {
  local f
  for f in "${FILES[@]}"; do
    cp "$BAKDIR/$(echo "$f" | tr '/' '_')" "$f" 2>/dev/null || true
  done
}
trap 'restore_all; rm -rf "$BAKDIR"' EXIT

for f in "${FILES[@]}"; do
  cp "$f" "$BAKDIR/$(echo "$f" | tr '/' '_')"
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
s = open(path).read()
assert orig in s, "pattern missing"
open(path, "w").write(s.replace(orig, mut, 1))
PYEOF
  if go test "$@" >/dev/null 2>&1; then
    echo "MUTANT-SURVIVED: $label (go test $* did not kill it)"
    exit 1
  fi
  echo "killed: $label"
  KILLED=$((KILLED + 1))
}

# ---------- pkg/convert ----------

run "M1 drop ToLower in IsSupportedDocument" \
  "pkg/convert/convert.go" \
  "documentExts[strings.ToLower(filepath.Ext(path))]" \
  "documentExts[filepath.Ext(path)]" \
  ./pkg/convert/ -run "TestIsSupportedDocument_CaseInsensitive"

run "M2 remove input size guard" \
  "pkg/convert/convert.go" \
  "if opts.MaxInputBytes > 0 {" \
  "if false {" \
  ./pkg/convert/ -run "TestConvertInputTooLarge"

run "M3 invert no-text mapping" \
  "pkg/convert/convert.go" \
  "if isNoTextError(err) {" \
  "if !isNoTextError(err) {" \
  ./pkg/convert/ -run "TestConvertNoTextPDF"

run "M4 hardcode WrapConcept ctype" \
  "pkg/convert/convert.go" \
  'fmt.Sprintf("---\ntype: %s\ntitle: %q\ndescription: %q\n---\n%s\n", ctype, title, desc, body)' \
  'fmt.Sprintf("---\ntype: %s\ntitle: %q\ndescription: %q\n---\n%s\n", "source", title, desc, body)' \
  ./pkg/mcp/ -run TestMCPImportWithOverrides

# ---------- pkg/chunk ----------
# Chunking correctness is what keeps long-document content reachable by search,
# so its guards must be provably tested.

run "M5 ignore code-fence state when parsing headings" \
  "pkg/chunk/chunk.go" \
  "if !inFence {" \
  "if true {" \
  ./pkg/chunk/ -run "TestCodeFence"

run "M6 allow H1 to split" \
  "pkg/chunk/chunk.go" \
  "ok && lv >= minSplitLevel && lv <= maxSplitLevel" \
  "ok && lv >= 1 && lv <= maxSplitLevel" \
  ./pkg/chunk/

run "M7 drop breadcrumb from chunk text" \
  "pkg/chunk/chunk.go" \
  "return c.Breadcrumb + \"\\n\" + c.Body" \
  "return c.Body" \
  ./pkg/chunk/

run "M8 skip tiny-chunk merge" \
  "pkg/chunk/chunk.go" \
  "out = mergeTiny(out, opts)" \
  "_ = mergeTiny" \
  ./pkg/chunk/

# ---------- pkg/lexical ----------

run "M9 drop identifier subword expansion" \
  "pkg/lexical/lexical.go" \
  "if subs := splitIdentifier(string(latin)); len(subs) > 1 || (len(subs) == 1 && subs[0] != whole) {" \
  "if false {" \
  ./pkg/lexical/ -run "TestTokenizeIdentifierKeepsWholeAndSubwords"

run "M10 emit CJK unigrams instead of bigrams" \
  "pkg/lexical/lexical.go" \
  "out = append(out, string(cjk[i:i+2]))" \
  "out = append(out, string(cjk[i:i+1]))" \
  ./pkg/lexical/ -run "TestTokenizeChineseBigram"

run "M11 remove BM25 length normalization" \
  "pkg/lexical/lexical.go" \
  "norm := f + paramK1*(1-paramB+paramB*b.lens[i]/b.avgdl)" \
  "norm := f + paramK1" \
  ./pkg/lexical/ -run "TestBM25LengthNormalizationIsolated"

run "M12 drop BM25 key tie-break" \
  "pkg/lexical/lexical.go" \
  "return hits[i].Key < hits[j].Key" \
  "return false" \
  ./pkg/lexical/ -run "TestBM25TieBreakIsStable"

# ---------- pkg/query ----------

run "M13 ignore lexical weight (always run lexical channel)" \
  "pkg/query/semantic.go" \
  "if opts.LexicalWeight > 0 {" \
  "if true {" \
  ./pkg/query/ -run "TestZeroLexicalWeightSkipsLexicalChannel"

run "M14 drop multi-chunk score accumulation" \
  "pkg/query/semantic.go" \
  "score[c] += rrfScore(opts.VectorWeight, opts.RRFK, r) * float32(hits)" \
  "score[c] += rrfScore(opts.VectorWeight, opts.RRFK, r)" \
  ./pkg/query/ -run "TestMultiChunkHitsAccumulate"

# ---------- restore and prove nothing else changed ----------
# 与运行前的备份逐字节比对，而不是与 HEAD 比对：
# 变异脚本要验证的是"自己没有留下残留改动"，
# 而被测文件本身可能有合法的未提交改动（开发中）。
restore_all
for f in "${FILES[@]}"; do
  if ! cmp -s "$f" "$BAKDIR/$(echo "$f" | tr '/' '_')"; then
    echo "MUTANT-FAILED(restore): $f differs from its pre-run state"
    exit 1
  fi
done
if ! go test ./pkg/convert/ ./pkg/chunk/ ./pkg/lexical/ ./pkg/query/ >/dev/null 2>&1; then
  echo "MUTANT-FAILED(restore): suite not green after restore"
  exit 1
fi
echo "manual mutation: $KILLED/$TOTAL killed (restore verified, suite green)"
