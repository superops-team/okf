# Specification: Retrieval Quality Closed Loop

## Requirement: Content Chunking

The system SHALL provide a pure-Go chunker that splits document markdown into
bounded chunks with inherited heading context, without breaking code blocks or
table rows, and with CJK-aware word counting.

### Scenario: chunking a multi-paragraph document
- **GIVEN** markdown with 3 paragraphs totaling 600+ words and default options
- **WHEN** `Chunk(markdown, nil)` is called
- **THEN** more than one chunk is returned and every chunk has ≤ 256 words
  (word count of `Text` plus heading prefix)

### Scenario: chunk at paragraph boundary
- **GIVEN** markdown `"para one\n\npara two\n\npara three"` with `MaxWords=2`
- **WHEN** `Chunk` is called
- **THEN** each paragraph becomes its own chunk (paragraph `\n\n` is the preferred
  split point)

### Scenario: CJK document is chunked correctly
- **GIVEN** a Chinese document of 500+ CJK characters with no whitespace
  (`"这是一个很长的中文段落……"` repeated), `MaxWords=100`
- **WHEN** `Chunk` is called
- **THEN** more than one chunk is returned and every chunk has ≤ 100 CJK
  characters (CJK runs count per rune, not as one whitespace token)

### Scenario: mixed CJK/English counting
- **GIVEN** text mixing English words and CJK runs, `MaxWords` small enough to
  force splits
- **WHEN** `countWords` is called
- **THEN** English tokens count as words and each CJK character counts as one
  word (the sum drives the same `MaxWords` budget)

### Scenario: heading inheritance
- **GIVEN** markdown `"# H1\n\nbody a\n\n## H2\n\nbody b"` with `MaxWords=3`
- **WHEN** `Chunk` is called
- **THEN** the chunk containing `body b` has `HeadingPath = "H1 > H2"` and its
  indexed text includes the bounded heading prefix

### Scenario: first chunk heading path
- **GIVEN** the same document
- **WHEN** the first chunk is inspected
- **THEN** its `HeadingPath` is `"H1"` (headings before the first body are still
  tracked)

### Scenario: heading prefix counts against the word budget
- **GIVEN** a document with a long heading chain (100+ chars of headings) and
  `MaxWords=32`
- **WHEN** `Chunk` is called
- **THEN** every chunk satisfies `words(Text) + words(headingPrefix) ≤ 32`
  (the body budget is `MaxWords − words(headingPrefix)`, reserved before splitting)

### Scenario: fenced code block is atomic
- **GIVEN** markdown with a ``` fenced block containing 300+ words inside a
  paragraph context, `MaxWords=32`
- **WHEN** `Chunk` is called
- **THEN** no chunk boundary falls inside the fenced block (the whole block is
  one chunk or aligned to its edges)

### Scenario: table rows are atomic
- **GIVEN** markdown with a multi-row markdown table, `MaxWords` small enough to
  otherwise split rows
- **WHEN** `Chunk` is called
- **THEN** no chunk boundary splits a table row (rows stay whole; the chunker may
  align at the table's edges)

### Scenario: empty input
- **GIVEN** empty markdown
- **WHEN** `Chunk` is called
- **THEN** an empty slice is returned (no panic, no 1-chunk artifact)

### Scenario: chunk indices are stable and sequential
- **GIVEN** any document
- **WHEN** `Chunk` is called twice
- **THEN** both calls return identical `Text`, `HeadingPath`, and `Index` values
  (0, 1, 2, ...) — deterministic, no randomness

### Scenario: word-count invariant (property)
- **GIVEN** any markdown (English, Chinese, mixed) and `MaxWords=N`
- **WHEN** `Chunk` is called
- **THEN** for every chunk, `words(Text) + words(headingPrefix) ≤ N` (property
  test over 200 random documents including CJK fixtures)

## Requirement: Cross-Encoder Re-Ranking

The system SHALL provide a re-ranking stage that re-orders RRF-fused semantic
search results by query–document relevance, with a nil-safe fallback that
preserves today's behavior exactly, and a fail-open degradation that never
propagates a ranker error as a search failure.

### Scenario: Reranker re-orders results
- **GIVEN** a fake ranker that assigns a known score to each result
- **WHEN** `RerankResults(query, results, ranker)` is called
- **THEN** results are sorted by `RerankScore` descending, stable for ties
  (original RRF order preserved among equal scores)

### Scenario: identity ranker is byte-identical to today
- **GIVEN** a bundle and a query
- **WHEN** `SemanticSearch` runs once with `Reranker=nil` and once with an
  identity ranker (scores equal to the input order, no reordering)
- **THEN** both runs return the same result order and the same
  `Source`/`SemanticScore` values (regression proof: nil path unchanged)

### Scenario: single result is not re-ranked
- **GIVEN** `len(results) == 1`
- **WHEN** re-ranking is applied
- **THEN** the single result keeps its position (no unnecessary re-rank)

### Scenario: ranker failure degrades to RRF order with a warning
- **GIVEN** a ranker that returns an error
- **WHEN** re-ranking is attempted
- **THEN** results are returned in original RRF order, the first result carries
  a non-empty `Warning`, and `SemanticSearch` returns no error (fail-open, no
  fallback to lexical search)

### Scenario: re-rank respects TopK cutoff
- **GIVEN** 20 RRF-fused results, a ranker, and `TopK=5`
- **WHEN** the pipeline runs
- **THEN** exactly 5 results are returned, selected from the re-ranked order
  (the cross-encoder sees 20 candidates, the user sees 5)

### Scenario: ranker receives bounded candidates
- **GIVEN** a ranker that records the inputs it was called with
- **WHEN** the pipeline runs with 50 RRF-fused candidates
- **THEN** the ranker is called with at most 20 candidates (bounded cost, no
  unbounded pair inference)

## Requirement: Cross-Encoder Production Implementation

The system SHALL provide a cross-encoder that scores (query, document) pairs
using the vendored ONNX stack, with a clearly separated test seam.

### Scenario: CrossEncoder.Score returns one score per document
- **GIVEN** a query and 3 documents
- **WHEN** `Score(query, docs)` is called on a stub/recorded implementation
- **THEN** it returns 3 scores and each is a finite float32

### Scenario: pair encoding uses sequence-pair input
- **GIVEN** the tokenizer seam
- **WHEN** `Score` tokenizes
- **THEN** each (query, doc) pair is encoded via the two-sequence API
  (`EncodePair`), not as concatenated single sequences

### Scenario: inference is serialized
- **GIVEN** concurrent `Score` calls
- **WHEN** 10 goroutines call `Score` simultaneously
- **THEN** no data race is detected under `-race` (the implementation serializes
  session access, same policy as MiniLM)

### Scenario: model unavailable is a clear error
- **GIVEN** a missing model path
- **WHEN** `NewCrossEncoder` is called
- **THEN** a non-nil error is returned identifying the missing resource

### Scenario: re-rank latency has a budget
- **GIVEN** a `Benchmark` test over 20 candidate pairs
- **WHEN** `Score` runs
- **THEN** the recorded per-query re-rank time is reported in the benchmark
  output (budget tracked, not asserted — CI observes drift)

## Requirement: Chunk-Aware Document Import

The system SHALL import large documents as multiple chunk concepts sharing the
source file, while preserving single-concept behavior for small documents, with
explicit chunk titles, derived-file metadata, and failure-atomic writes.

### Scenario: small document stays single-concept
- **GIVEN** a document whose markdown fits the threshold (the existing 7
  fixtures qualify — all convert to <160 chars)
- **WHEN** `okf add` imports it
- **THEN** exactly one concept is created (no `__cN` files) — behavior identical
  to today

### Scenario: large document is chunked
- **GIVEN** a synthetic document with 5000+ chars of markdown
- **WHEN** `okf add` imports it
- **THEN** the bundle contains the whole-document concept plus 2+ chunk concepts
  (`<original>__c1.md`, `__c2.md`, ...)

### Scenario: chunk concept title is explicit, not filename-derived
- **GIVEN** a chunked bundle
- **WHEN** a chunk concept is inspected
- **THEN** its `title` is `<document title> — part N` (or heading-derived when a
  heading path exists), and does NOT contain the `__cN` artifact (the title is
  written at import time, never derived from the filename)

### Scenario: chunk concept metadata
- **GIVEN** the chunked bundle
- **WHEN** a chunk concept is inspected
- **THEN** its custom fields contain `chunk_index`, `chunk_count`,
  `heading_path` (when the source has headings), `source_path` (original file
  path), and `derived: true`

### Scenario: chunked deep-tail content is searchable (semantic)
- **GIVEN** a long document whose tail contains a unique term, imported chunked
- **WHEN** semantic search (`-semantic` / `okf_semantic_search`) for the unique
  term runs
- **THEN** the result includes the chunk concept containing the term

### Scenario: unchunked semantic control cannot retrieve deep-tail content
- **GIVEN** the same long document imported unchunked (small-doc path disabled)
- **WHEN** the same **semantic** search runs
- **THEN** the term is not retrievable in the top results, because the embedding
  model truncates each concept to 256 tokens and the tail is dropped (the
  control proves the chunking gain on the semantic path; lexical search is out
  of scope for this control — substring matching finds the tail in both cases)

### Scenario: re-import is idempotent
- **GIVEN** a chunked document already imported
- **WHEN** `okf add` runs again on the unchanged source
- **THEN** no new or changed concepts are produced (deterministic chunking +
  existing file-level change detection)

### Scenario: chunk files are derived artifacts
- **GIVEN** a chunked bundle and its source document removed
- **WHEN** `okf sync` runs
- **THEN** the chunk files (`__cN`) are removed together with the source
  (`derived: true` + `source_path` drive lifecycle; direct edits to chunk files
  are overwritten on re-import, not treated as author-owned)

### Scenario: MCP chunked import is failure-atomic
- **GIVEN** `okf_import_document` with a large document and a forced write
  failure on chunk N
- **WHEN** the import attempts to write
- **THEN** no partial chunk set remains in the bundle (temp-file + rename, or
  cleanup of already-written chunks on failure)

## Requirement: Search Deduplication by Source

The system SHALL return at most one result per source document in semantic
search, so chunked documents do not monopolize the result list.

### Scenario: chunked document occupies one result slot
- **GIVEN** a bundle with one 10-chunk document and `TopK=5`
- **WHEN** semantic search returns
- **THEN** the document appears at most once in the 5 results (top-scoring chunk
  wins) and the survivor reports `DuplicateCount` = number of hidden chunks

### Scenario: dedupe keyed by source, not fingerprint
- **GIVEN** a bundle where two different documents share a chunk's fingerprint
  pattern (defensive check)
- **WHEN** dedupe runs
- **THEN** the key is `source_path` (or `FilePath` with `__cN` stripped), so two
  distinct sources never collapse into one result

### Scenario: dedupe can be disabled
- **GIVEN** `SearchOptions.DisableDedupe = true`
- **WHEN** semantic search returns
- **THEN** all chunk results are returned (raw per-chunk list, for callers that
  want full detail)

### Scenario: lexical search is unaffected
- **GIVEN** `okf search` without `-semantic`
- **WHEN** results are returned
- **THEN** dedupe does not apply (no chunk concepts by default; behavior
  unchanged)

## Requirement: Unified Search Result Format

The system SHALL expose chunk, re-rank, warning, and duplicate metadata
uniformly across CLI and MCP search responses, with an unambiguous field
contract.

### Scenario: SearchResult carries new fields
- **GIVEN** a search result from `SemanticSearch` with a ranker
- **WHEN** the result is inspected
- **THEN** `RerankScore > 0`, `Heading` matches the chunk heading (or title),
  `ChunkIndex ≥ 0` for chunk concepts and `-1` for single concepts,
  `ChunkCount ≥ 1`, and `DuplicateCount` is present (0 when not deduped)

### Scenario: SemanticScore is always the RRF score
- **GIVEN** a re-ranked result
- **WHEN** its fields are inspected
- **THEN** `SemanticScore` still equals the RRF fusion score (never overwritten
  by the cross-encoder score; `RerankScore` is the separate re-rank score)

### Scenario: CLI and MCP JSON field parity
- **GIVEN** the same query on the same bundle via `okf search -semantic -json`
  and via MCP `okf_semantic_search`
- **WHEN** both JSON responses are parsed
- **THEN** result objects expose the same field names (`rerank_score`, `heading`,
  `chunk_index`, `chunk_count`, `warning`, `duplicate_count`, `source`,
  `semantic_score`)

### Scenario: lexical-only results have zero re-rank fields
- **GIVEN** `okf search` (no `-semantic`)
- **WHEN** a result is serialized
- **THEN** `rerank_score` is absent or 0, `chunk_index` is absent or `-1`,
  and `warning` is empty (no misleading cross-encoder attribution)

### Scenario: ranker warning surfaces in output
- **GIVEN** a semantic search where the ranker degraded to RRF order
- **WHEN** the CLI or MCP response is inspected
- **THEN** the response includes the warning text (visible degradation notice,
  not silent)

## Requirement: Eval Benchmark Adaptation

The system SHALL measure the semantic retrieval path in addition to the lexical
path, keep the lexical baseline stable, and prove the chunking + re-ranking
gains.

### Scenario: lexical baseline does not regress
- **GIVEN** the unchanged 20-case golden set on the unchunked 7-fixture bundle
- **WHEN** `tools/eval.sh` runs (lexical mode, unchanged)
- **THEN** Recall@5 ≥ 0.9 and NDCG@5 ≥ 0.9 (unchanged lexical path)

### Scenario: semantic-link evaluation exists
- **GIVEN** the golden set and a bundle with a vector index
- **WHEN** the semantic eval runs `SemanticSearch` (RRF-only) over the golden set
- **THEN** MRR and NDCG are reported alongside the lexical scores (the eval
  report distinguishes lexical vs semantic rows)

### Scenario: re-ranking does not hurt semantic ranking
- **GIVEN** the semantic eval with RRF-only and with a ranker
- **WHEN** both modes run over the same golden set
- **THEN** `MRR(rerank) ≥ MRR(rrf)` and `NDCG(rerank) ≥ NDCG(rrf)` (delta
  assertion, not an absolute threshold)

### Scenario: chunked bundle scoring dedupes by source file
- **GIVEN** a chunked bundle where one source file has 3 chunk concepts
- **WHEN** `RunBenchmark` scores a query whose expected doc is that source file
- **THEN** the source file counts as retrieved if any of its 3 chunks appears in
  top-K (no double counting, no false negative)

### Scenario: chunked-bundle searchability test exists
- **GIVEN** the test suite
- **WHEN** tests run
- **THEN** `TestChunkedBundleSearchable` passes: deep-tail term retrievable in
  the chunked bundle via semantic search, not retrievable in the unchunked
  semantic control

## Requirement: CLI and MCP Integration

The system SHALL expose chunking, dedupe, and re-ranking through the existing CLI
and MCP surfaces without breaking their contracts.

### Scenario: okf add accepts large documents without error
- **GIVEN** a large PDF/DOCX/XLSX/PPTX/HTML/CSV/TXT document
- **WHEN** `okf add` imports it
- **THEN** the command exits 0 and reports the created chunk concepts

### Scenario: MCP okf_search response includes new fields
- **GIVEN** an MCP session with a chunked bundle
- **WHEN** `okf_search` is called
- **THEN** the response schema validates (all prior fields intact) and result
  objects carry the new chunk/rerank/warning keys without breaking the existing
  envelope version

### Scenario: MCP import document writes chunk files
- **GIVEN** `okf_import_document` with a large document
- **WHEN** the import completes
- **THEN** chunk files exist under the bundle root and the bundle reloads
  successfully

## Requirement: Regression and Quality Gates

### Scenario: full gauntlet passes
- **GIVEN** a clean checkout
- **WHEN** `tools/gauntlet.sh` runs
- **THEN** all layers pass: build, vet, gofmt, staticcheck, tests -race,
  coverage ≥ 60%, shuffle, mutation (chunker + reranker mutants killed),
  real execution

### Scenario: conformance is complete
- **GIVEN** the spec
- **WHEN** conformance.md is written
- **THEN** every scenario above maps to a named test with status
  fully/aligned/partial/gap, and no scenario is unmapped
