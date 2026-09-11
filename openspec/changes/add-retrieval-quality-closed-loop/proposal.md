# Proposal: Retrieval Quality Closed Loop

## Problem

The khoj comparison (2026-09-04) identified that okf's largest quality gap versus a mature "second brain" is **retrieval quality**, concentrated in three dimensions:

1. **No content chunking.** `pkg/convert` converts a whole document into a single markdown concept. A 50-page PDF becomes one concept: either the query matches the whole blob (imprecise) or not at all (misses content buried deep in the document). khoj splits entries by `max_tokens` with heading inheritance, so a query about content on page 40 retrieves just that section.
2. **No re-ranking.** okf has bi-encoder semantic search + lexical search fused via RRF (a score-fusion heuristic that never sees query–doc interaction). khoj adds a cross-encoder pass over the top-K candidates, which substantially improves ranking quality.
3. **No unified result format.** `SearchResult` fields differ between lexical search, semantic search, MCP responses, and CLI output, and carry no chunk context (heading, chunk index).

The IR evaluation benchmark (v0.5.0) measures but does not fix these: baseline MRR is 1.0 on tiny fixtures, which cannot distinguish ranked retrieval quality.

## Goal

Close the retrieval quality loop in three P0-level capabilities, each with tests and a measurable benchmark delta:

1. **Content chunking** — pure-Go chunker in `pkg/convert` that splits document markdown at paragraph > heading > sentence boundaries, inherits the heading path into every chunk (like khoj), and assigns stable chunk indices. Large documents are imported as multiple chunk concepts sharing the source file; small documents keep the single-concept behavior (minimal disruption, no behavior change for the existing 7 fixtures).
2. **Cross-encoder two-stage re-ranking** — a `Reranker` abstraction in `pkg/query`; a production implementation in `pkg/embeddings` using the existing `pure-onnx` + `pure-tokenizers` stack (both already vendored; tokenizer `EncodePair` is verified available). The semantic search pipeline becomes: bi-encoder coarse top-K → cross-encoder re-rank → top-N. Falls back to RRF-only when the model is unavailable (fail-open by design, documented).
3. **Unified search result format** — `SearchResult` gains `RerankScore float32`, `Heading string`, `ChunkIndex int`, `ChunkCount int`; CLI `okf search`/`okf search -semantic` and MCP `okf_search`/`okf_semantic_search` output one consistent structure.

Plus two correctness requirements surfaced by review (without which chunking would *hurt* search quality):

4. **Search deduplication by source file.** A chunked document must occupy at most one result slot (top-scoring chunk wins; duplicates hidden by default). Without this, a 10-chunk document fills the top-5 with the same source (khoj has `deduplicated_search_responses` for exactly this reason).
5. **Semantic-link evaluation.** The eval benchmark must measure the semantic path (`SemanticSearch` RRF vs RRF+rerank), not only the lexical path that chunking/rerank never touch.

## Non-Goals

- **No natural-language query filters** (date/file/word filters from khoj) — second phase.
- **No chunk-level incremental sync** (content-hash change detection) — second phase; file-level detection stays.
- **No MCP client** (calling external MCP servers) — third phase.
- **No ProcessLock** (global index write lock) — second phase.
- **No LLM-as-judge metrics** (faithfulness etc.) — okf has no generation layer.
- **No change to the OKF v0.2 spec** — chunk concepts reuse existing concept fields; chunk metadata lives in custom fields.
- **No new external dependencies** — the cross-encoder reuses `pure-onnx` + `pure-tokenizers` already in go.mod. A new embedded model resource is added (build-time fetch + embed, same mechanism as the MiniLM model).
- **No default re-ranking of lexical-only search** — re-ranking applies to semantic search where candidates exist.

## Success Criteria

- A >2000-char document imported via `okf add` produces multiple chunk concepts; a semantic query for content in the deep tail retrieves the correct chunk (test proves pre-chunk semantic search missed it due to the 256-token truncation).
- A chunked document occupies at most one result slot in semantic search (dedupe test).
- `SemanticSearch` with a `Reranker` returns the same result set re-ordered by cross-encoder score; with `nil` ranker the behavior is byte-identical to today (identity-ranker regression test).
- The IR benchmark semantic-link evaluation shows RRF+rerank MRR ≥ RRF-only MRR on the golden set (no regression, measurable delta).
- `SearchResult` is serialized identically across CLI and MCP (same JSON field names).
- Full gauntlet (`tools/gauntlet.sh`) passes, including the new coverage threshold and mutation tests for chunker + reranker.
- `conformance.md` maps every spec Scenario to a test with no gaps.

## Risks (pre-registered)

| Risk | Mitigation |
|---|---|
| Cross-encoder model availability as quantized ONNX + license. **Expected reality: ms-marco-MiniLM-L-6-v2 has no official ONNX export; we anticipate self-exporting/quantizing (Python toolchain, build-time only) or adopting a community ONNX build.** | P0 spike: locate/fetch/quantize, verify inference via `NewAdvancedSession` + `EncodePair` before building integration; license recorded in README; if unobtainable, use an alternative open cross-encoder (e.g. MiniLM-L-3) and record the substitution |
| **Binary size grows further (~+20MB for an int8 cross-encoder on top of the existing ~10MB ONNX stack).** The v0.3.0 release already documented a +10MB growth as a behavior change. | Proposal declares the expected size increase; docs record it; size is measured in the gauntlet real-execution step and reported in the PR |
| Chunking changes import behavior and may affect existing golden tests / eval expected_docs | Chunking only activates above a size threshold; existing 7 fixtures stay single-concept (their golden tests unchanged); eval golden set unchanged; new tests cover the chunked path |
| Cross-encoder latency (query × top-K pair inferences) | Re-rank only top-K=20 candidates (bounded), default on only in `-semantic` mode; model inference serialized behind the same mutex as MiniLM; a `Benchmark` test records per-query re-rank latency as a budget |
| Chunk boundary quality (splitting mid-table / code block) | Chunker respects fenced code blocks and table rows as atomic units (separators disabled inside them) |
| **CJK documents fail to chunk if word-counting is whitespace-based** (a Chinese paragraph is one "word") | Chunker counts CJK runs per character (rune), mixed text per whitespace token + CJK char; dedicated Chinese boundary tests are part of P0 |

## Delivery Milestones

Review (2026-09-06) recommended splitting delivery so the model spike cannot block the chunking value:

- **M1 — Chunking + dedupe + eval adaptation (independent PR).** `pkg/convert` chunker, chunk-aware import, search dedupe, semantic-link eval foundation. Fully shippable on its own; the searchability gain is proven against the unchunked control.
- **M2 — Cross-encoder re-ranking (dependent PR).** `Reranker` abstraction + `pkg/embeddings.CrossEncoder` + pipeline integration + rerank eval delta. Starts only after the P0 model spike passes (or a recorded substitution is chosen).

Both milestones share this openspec change; each is a separate PR with its own conformance.md progress (M1: chunking/import/dedupe/eval scenarios fully; M2: rerank scenarios fully).
