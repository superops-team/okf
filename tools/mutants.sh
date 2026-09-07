#!/usr/bin/env bash
# Manual mutation runner (Gauntlet layer: Mutation).
#
# Each mutant is applied to real source by an exact replacement, killed by a
# targeted test, then all touched files are restored byte-for-byte.
#
# Covered packages:
#   pkg/convert  — document conversion and persisted import chunking (M1-M8)
#   pkg/chunk    — index-time heading-aware chunking (M9-M12)
#   pkg/lexical  — tokenization + BM25 (M13-M16)
#   pkg/query    — weighted RRF fusion (M17-M18)

set -euo pipefail

cd "$(dirname "$0")/.."
GO=${GO:-go}
FILES=(
  "pkg/convert/convert.go"
  "pkg/convert/chunk.go"
  "pkg/chunk/chunk.go"
  "pkg/lexical/lexical.go"
  "pkg/query/semantic.go"
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

# ---------- pkg/convert: conversion ----------
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
  'fmt.Sprintf("---\ntype: %s\ntitle: %q\ndescription: %q\ngenerated: true\ngenerator: %q\nsource_path: %q\n---\n%s\n",\n\t\tctype, title, desc, generatorName, filename, body)' \
  'fmt.Sprintf("---\ntype: %s\ntitle: %q\ndescription: %q\ngenerated: true\ngenerator: %q\nsource_path: %q\n---\n%s\n",\n\t\t"source", title, desc, generatorName, filename, body)' \
  ./pkg/mcp/ -run TestMCPImportWithOverrides

# ---------- pkg/convert: persisted import chunks ----------
run "M5 CJK counting disabled" \
  "pkg/convert/chunk.go" \
  "if isCJK(r) {" \
  "if false && isCJK(r) {" \
  ./pkg/convert/ -run TestCountWords_ChineseOnly

run "M6 heading budget not reserved" \
  "pkg/convert/chunk.go" \
  "b := maxWords - headingPrefixWords(headingStack)" \
  "b := maxWords" \
  ./pkg/convert/ -run TestChunk_HeadingPrefixCountsAgainstBudget

run "M7 fenced code not atomic" \
  "pkg/convert/chunk.go" \
  "case segFence, segTable:" \
  "case segTable:" \
  ./pkg/convert/ -run TestChunk_FencedCodeAtomic

run "M8 empty input returns one chunk" \
  "pkg/convert/chunk.go" \
  $'if markdown == "" {\n\t\treturn nil\n\t}' \
  $'if markdown == "" {\n\t\treturn []Chunk{{}}\n\t}' \
  ./pkg/convert/ -run TestChunk_EmptyInput

# ---------- pkg/chunk: index chunks ----------
run "M9 ignore code-fence state when parsing headings" \
  "pkg/chunk/chunk.go" \
  "if !inFence {" \
  "if true {" \
  ./pkg/chunk/ -run "TestCodeFence"

run "M10 allow H1 to split" \
  "pkg/chunk/chunk.go" \
  "ok && lv >= minSplitLevel && lv <= maxSplitLevel" \
  "ok && lv >= 1 && lv <= maxSplitLevel" \
  ./pkg/chunk/

run "M11 drop breadcrumb from chunk text" \
  "pkg/chunk/chunk.go" \
  "return c.Breadcrumb + \"\\n\" + c.Body" \
  "return c.Body" \
  ./pkg/chunk/

run "M12 skip tiny-chunk merge" \
  "pkg/chunk/chunk.go" \
  "out = mergeTiny(out, opts)" \
  "_ = mergeTiny" \
  ./pkg/chunk/

# ---------- pkg/lexical ----------
run "M13 drop identifier subword expansion" \
  "pkg/lexical/lexical.go" \
  "if subs := splitIdentifier(string(latin)); len(subs) > 1 || (len(subs) == 1 && subs[0] != whole) {" \
  "if false {" \
  ./pkg/lexical/ -run "TestTokenizeIdentifierKeepsWholeAndSubwords"

run "M14 emit CJK unigrams instead of bigrams" \
  "pkg/lexical/lexical.go" \
  "out = append(out, string(cjk[i:i+2]))" \
  "out = append(out, string(cjk[i:i+1]))" \
  ./pkg/lexical/ -run "TestTokenizeChineseBigram"

run "M15 remove BM25 length normalization" \
  "pkg/lexical/lexical.go" \
  "norm := f + paramK1*(1-paramB+paramB*b.lens[i]/b.avgdl)" \
  "norm := f + paramK1" \
  ./pkg/lexical/ -run "TestBM25LengthNormalizationIsolated"

run "M16 drop BM25 key tie-break" \
  "pkg/lexical/lexical.go" \
  "return hits[i].Key < hits[j].Key" \
  "return false" \
  ./pkg/lexical/ -run "TestBM25TieBreakIsStable"

# ---------- pkg/query ----------
run "M17 ignore lexical weight" \
  "pkg/query/semantic.go" \
  "if opts.LexicalWeight > 0 {" \
  "if true {" \
  ./pkg/query/ -run "TestZeroLexicalWeightSkipsLexicalChannel"

run "M18 drop multi-chunk score accumulation" \
  "pkg/query/semantic.go" \
  "score[c] += rrfScore(opts.VectorWeight, opts.RRFK, r) * float32(hits)" \
  "score[c] += rrfScore(opts.VectorWeight, opts.RRFK, r)" \
  ./pkg/query/ -run "TestMultiChunkHitsAccumulate"

restore_all
for f in "${FILES[@]}"; do
  if ! cmp -s "$f" "$BAKDIR/$(printf '%s' "$f" | tr '/' '_')"; then
    echo "MUTANT-FAILED(restore): $f differs from its pre-run state"
    exit 1
  fi
done
if ! "$GO" test ./pkg/convert/ ./pkg/chunk/ ./pkg/lexical/ ./pkg/query/ >/dev/null 2>&1; then
  echo "MUTANT-FAILED(restore): suite not green after restore"
  exit 1
fi
echo "manual mutation: $KILLED/$TOTAL killed (restore verified, suite green)"
