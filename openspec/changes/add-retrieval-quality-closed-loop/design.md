# Design: Retrieval Quality Closed Loop

## Architecture

```
pkg/convert/
├── convert.go          # unchanged public API (ConvertToMarkdown, WrapConcept, ...)
└── chunk.go            # NEW: Chunk(doc, opts) []Chunk + HeadingPath tracking + CJK-aware counting

pkg/query/
├── query.go            # SearchResult gains RerankScore/Heading/ChunkIndex/ChunkCount/Warning (additive)
├── semantic.go         # SemanticSearch: dedupe-by-source + optional Reranker stage between RRF fusion and TopK cut
└── rerank.go           # NEW: Reranker interface + RerankResults helper + RRF-fallback + warning channel

pkg/embeddings/
├── embeddings.go       # unchanged MiniLM
└── crossencoder.go     # NEW: CrossEncoder (pure-onnx AdvancedSession + tokenizer EncodePair)

internal/embeddings/assets/   # cross-encoder model added to the existing embed/extract mechanism
scripts/fetch-crossencoder.sh # build-time model fetch (mirrors fetch-model.sh)
cmd/okf/cmd_add.go            # chunk-aware import for documents over threshold
cmd/okf/cmd_vector.go         # -semantic path passes reranker; unified output
pkg/mcp/tools.go              # okf_search / okf_semantic_search include new fields
pkg/eval/benchmark.go         # injectable searcher (lexical / semantic RRF / semantic RRF+rerank)
```

## Key Design Decisions

### 1. Chunking lives in `pkg/convert`, operates on markdown, not raw formats

All 7 formats already funnel through `ConvertToMarkdown`. Chunking the resulting
markdown means one code path covers every format (khoj does the same: it chunks
the compiled entry). API:

```go
type Chunk struct {
    Text        string // chunk body (heading prefix NOT included in Text)
    HeadingPath string // "H1 > H2" inherited heading path (empty if no headings)
    Index       int    // 0-based chunk index within the document
}

type ChunkOptions struct {
    MaxWords   int // default 256; counted CJK-aware (see below)
    MinChunk   int // minimum words to force a split on (default 32)
}

func Chunk(markdown string, opts *ChunkOptions) []Chunk
```

Split precedence: `paragraph ("\n\n") > heading ("\n# ") > sentence ("\n", "! ", "? ", ". ") > word`.
Fenced code blocks and table rows are atomic: the chunker scans for ``` fences and
table delimiter lines and never splits inside them.

**Word counting is CJK-aware (review fix B4).** `MaxWords` is not `strings.Fields`
count — a Chinese paragraph would count as one "word" and never split. Instead:

- whitespace-separated tokens count as words (English and other scripts);
- **runs of CJK characters (Han/Hiragana/Katakana/Hangul ranges) count per rune**,
  so a 300-char Chinese paragraph ≈ 300 "words" and splits correctly;
- mixed text (English + Chinese) counts both ways.

A dedicated `countWords(text string) int` helper owns this logic and is unit-tested
for: English, Chinese-only, mixed, punctuation-only, empty.

**Heading budget is reserved before body splitting (review fix I5).** Each chunk's
indexed text = heading prefix (bounded to last 100 chars, khoj-style) + body. The
word budget for the body is `MaxWords − ceil(words(headingPrefix))`, so
`words(Text) + words(headingPrefix) ≤ MaxWords` always holds. Splitting happens on
the body with the reserved budget; the prefix is prepended after.

Heading inheritance: when a split happens at a heading, every later chunk carries
the accumulated heading path; the first chunk's `HeadingPath` is the heading chain
before its first body text.

### 2. Chunked import is opt-in by size, preserving all existing behavior

`cmd_add` converts a document, then:

- if the markdown is small (`≤ MaxChunkWords` budget ≈ 2000 chars), import as one
  concept — **identical to today** (the 7 fixtures, golden tests, eval golden set
  are untouched — verified: all 7 fixtures convert to <160 chars);
- if larger, split via `Chunk` and write **one file per chunk**:
  - `docs/<original>.md` — the whole-document concept (v0.2 spec compliance,
    backward-compatible `Resource`);
  - `docs/<original>__c1.md`, `__c2.md`, ... — chunk concepts.

**Chunk concept metadata (review fixes I1/I6):**

- `title` is written explicitly at import time: `<document title> — part N`, or
  the heading-derived title when the chunk's `HeadingPath` is non-empty. It is
  **never derived from the filename** (`titleFromImportPath` would turn
  `sample.pdf__c1.md` into "sample pdf c1"). The parser's `__cN` suffix is
  verified not to collide with reserved names (`index.md`/`log.md`) — P0 check.
- custom fields: `chunk_index`, `chunk_count`, `heading_path`, `source_path`
  (original file path), and **`derived: true`**.
- `derived: true` marks the concept as a chunk artifact so `okf sync` treats it
  as derived from `source_path`: when the source file is removed, its chunk files
  are removed with it (`DetectSourceMissing` extended to understand `__cN`
  artifacts); direct user edits to chunk files are overwritten on re-import (they
  are derived, not author-owned — documented in `docs/knowledge/search.md`).

`okf sync` file-level change detection already works: a changed source document
re-converts and re-writes all chunk files. Chunk-level incremental sync is
explicitly a non-goal this phase.

### 3. Cross-encoder reuses the vendored ONNX stack

`pkg/embeddings.CrossEncoder` wraps `pure-onnx`'s `NewAdvancedSession` (verified
present) and `pure-tokenizers`' `EncodePair` (verified present):

```go
type CrossEncoder struct {
    mu sync.Mutex // serialized, same policy as MiniLM
    session *ort.AdvancedSession
    tok *tokenizers.Tokenizer
}

func NewCrossEncoder(modelPath, tokenizerPath string) (*CrossEncoder, error)
func (c *CrossEncoder) Score(query string, docs []string) ([]float32, error)
```

`Score` tokenizes each (query, doc) pair via `EncodePair`, runs the session
(inputs: input_ids, attention_mask, token_type_ids; output: logits), and returns
the sigmoid/probability score. Batch up to 32 pairs per session run.

Model: ms-marco-MiniLM-L-6-v2 exported to ONNX + int8 quantized. **Expected: no
official ONNX export exists; the P0 spike self-exports/quantizes (Python, build
time only) or adopts a community build, and records the substitution if needed.**
Fetched by `scripts/fetch-crossencoder.sh`, embedded via `go:embed`, extracted to
user cache with checksum verification — the exact MiniLM mechanism. License
(Apache-2.0 for ms-marco) recorded in README. **Binary size grows ~+20MB; this is
a declared behavior change (see proposal risks) measured in the gauntlet
real-execution step.**

### 4. `Reranker` is a narrow interface; nil = today's behavior; errors = warning

```go
type Reranker interface {
    // Rerank re-orders candidates (already RRF-fused, top-K) by query-doc relevance.
    // Must return the same slice length; may mutate scores in place via RerankScore.
    Rerank(query string, results []SearchResult) ([]SearchResult, error)
}
```

`SemanticSearch` gains an optional `Reranker Reranker` field on `SearchOptions`.
Pipeline: RRF fusion (unchanged) → dedupe by source (see §6) → if
`Reranker != nil` and `len(results) > 1` → `rerank.RerankResults` → TopK cut.
With `nil`, `SemanticSearch` is byte-identical to today (proved by an
**identity-ranker** regression test — review fix S4: the test ranker returns
scores equal to the input order, no circular construction).

**Fail-open with a warning channel (review fix I3).** If the ranker returns an
error at runtime (model missing, ONNX failure), `RerankResults` returns the
original RRF order **and** attaches a `Warning` string to the first result
(`SearchResult.Warning`, additive field). `SemanticSearch` never propagates the
ranker error to the caller — the CLI therefore does **not** fall back to lexical
search (which would be worse than RRF+warning); the warning is surfaced in CLI and
MCP output. No new error path in the `SemanticSearch` signature.

### 5. Unified result format is additive, one JSON contract

```go
type SearchResult struct {
    Concept *Concept
    SymbolMatches []SymbolMatch
    Source string        // semantic / lexical / both (existing)
    SemanticScore float32 // RRF fusion score — ALWAYS the RRF score, never overwritten (review fix I4)
    RerankScore float32  // NEW: cross-encoder score; 0 when not re-ranked
    Heading string       // NEW: chunk heading path or doc title
    ChunkIndex int       // NEW: -1 for single-concept results (no chunk)
    ChunkCount int       // NEW: 1 for single-concept results
    Warning string       // NEW: re-rank degradation notice; empty when none
    DuplicateCount int   // NEW: number of hidden duplicate chunks from the same source (0 when none)
}
```

Field contract: `SemanticScore` is defined as the RRF fusion score in every path
(lexical-only sets it 0); `RerankScore` is the cross-encoder score and is only
non-zero after re-ranking; the two are never merged or overwritten. CLI
(`cmd_vector.go`) and MCP (`okf_search` / `okf_semantic_search`) serialize the
same fields to JSON. A test asserts field-name parity between CLI and MCP JSON.

Lexical-only results (`okf search` without `-semantic`): `rerank_score` 0/absent,
`chunk_index` -1/absent, `warning` empty — no misleading cross-encoder attribution.

### 6. Search deduplication by source file (review fix B1)

Chunk concepts share `source_path` but have distinct fingerprints
(`type:title:path`), so without dedupe a query returns several chunks of the same
document and fills Top-K with one source. `SemanticSearch` (and only it — lexical
search has no chunk concepts by default) dedupes after RRF fusion:

- `sourceKey(c *Concept) string` = `CustomFields["source_path"]` when present,
  else `FilePath` with a trailing `__cN` artifact suffix stripped;
- when multiple results share a `sourceKey`, keep the highest-scoring one
  (tie → lowest rank) and record `DuplicateCount` on the survivor;
- `SearchOptions.DedupeBySource` defaults to `true`; `false` restores raw
  per-chunk results for callers that want the full list (documented, tested).

This mirrors khoj's `deduplicated_search_responses` (dedupe by compiled content)
while being keyed on source identity.

### 7. Eval benchmark measures the semantic link (review fix B2)

The current `RunBenchmark` hardcodes lexical `query.Search`, which chunking and
re-ranking never touch — its "no regression" assertion would be vacuous. Fix:

- `RunBenchmark` gains an injectable `Searcher` option:
  `type Searcher func(bundle *query.KnowledgeBundle, q string, k int) []query.SearchResult`,
  defaulting to lexical `query.Search` (existing behavior preserved);
- a **semantic-link evaluation** runs the same 20-case golden set through
  `SemanticSearch` in two modes — RRF-only and RRF+identity-rerank (M1, no model
  dependency) then RRF+cross-encoder (M2, with model) — and compares MRR/NDCG;
- the test asserts `MRR(rerank) ≥ MRR(rrf)` on the semantic path (delta, not an
  absolute number), and prints both scores in the report for documentation;
- `TestChunkedBundleSearchable` proves the chunking gain: deep-tail term
  retrievable in the chunked bundle via **semantic** search, NOT retrievable in
  the unchunked control via semantic search (review fix B3 — the mechanism is
  MiniLM's 256-token per-concept truncation, which drops the tail; chunking
  keeps every chunk ≤ 256 tokens so the tail chunk is embedded and findable).
  Lexical search is out of scope for this control (substring matching finds the
  tail in both cases; it is the semantic path where chunking changes the outcome).

Scoring over chunked bundles: results are keyed by source file (dedupe is already
applied by `SemanticSearch`; lexical path has no chunks), so the golden set's
`expected_docs` (source filenames) map 1:1 — no double counting.

### 8. MCP chunked write is failure-atomic (review fix I2)

`handleImportDocument` currently writes one file with `os.WriteFile` (non-atomic
even today). With chunked import it writes N+1 files; a mid-write failure must not
leave a partial chunk set visible. Strategy (consistent with `pkg/tool/write.go`'s
durable write contract):

1. write every chunk file to a temp name in the bundle dir (`*.tmp-<rand>`);
2. after all temp files are written + fsynced, rename each into place;
3. on any failure, remove all temp files (and any already-renamed chunk files for
   this import) before returning the error.

A test injects a write failure on chunk N and asserts the bundle contains no
partial chunk set afterwards.

## File Change Summary

| File | Change | Reason |
|---|---|---|
| `pkg/convert/chunk.go` | new | pure-Go chunker, CJK-aware counting, heading inheritance |
| `pkg/convert/chunk_test.go` | new | boundaries, CJK, atomic blocks, heading inheritance, property test |
| `pkg/query/rerank.go` | new | Reranker interface + RerankResults (warning channel, stable sort) |
| `pkg/query/rerank_test.go` | new | identity-ranker parity, re-order, fail-open warning, TopK cutoff |
| `pkg/query/query.go` | modify | SearchResult additive fields (RerankScore/Heading/ChunkIndex/ChunkCount/Warning/DuplicateCount) |
| `pkg/query/semantic.go` | modify | dedupe-by-source + optional rerank stage + warning surfacing |
| `pkg/query/semantic_test.go` | modify | dedupe tests, sourceKey helper, nil-path parity |
| `pkg/embeddings/crossencoder.go` | new | ONNX cross-encoder (session + EncodePair) |
| `pkg/embeddings/crossencoder_test.go` | new | stub-seam unit + `-race` concurrency + missing-model error |
| `internal/embeddings/assets/*` | modify | cross-encoder model resource (build-time fetched) |
| `scripts/fetch-crossencoder.sh` | new | model fetch (mirrors fetch-model.sh) |
| `cmd/okf/cmd_add.go` | modify | chunk-aware import, explicit chunk titles, derived metadata |
| `cmd/okf/cmd_add_chunk_test.go` | new | chunked import, CJK doc, title/derived metadata, idempotent re-import |
| `cmd/okf/cmd_vector.go` | modify | pass reranker, unified output incl. warning/duplicate count |
| `pkg/mcp/tools.go` | modify | chunked write (failure-atomic), new result fields |
| `pkg/mcp/tools_import_test.go` | modify | atomic chunk write failure test, field parity |
| `pkg/eval/benchmark.go` | modify | injectable Searcher, semantic-link eval modes, report deltas |
| `pkg/eval/benchmark_test.go` | modify | chunked-bundle searchability (semantic control), rerank≥rrf delta |
| `pkg/eval/metrics_test.go` | unchanged | metrics already covered |
| `docs/knowledge/search.md` | new | chunking + re-ranking + dedupe user doc |
| `README.md` / `docs/knowledge/releases.md` | modify | v0.6.0 feature + scores + size note |
