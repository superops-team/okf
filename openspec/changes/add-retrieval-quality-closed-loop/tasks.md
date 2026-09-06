# Tasks: Retrieval Quality Closed Loop

P0-P4 indicate dependency order only — **all must be completed** (AGENTS.md §18).

Delivery is split into two milestones per review (proposal.md "Delivery
Milestones"): **M1 = chunking + dedupe + eval foundation (independent PR)**,
**M2 = cross-encoder re-ranking (after model spike)**. M1 tasks are marked
`[M1]`, M2 tasks `[M2]`; M2 depends on P0 spike passing.

## P0: Chunker (pure function + tests) — [M1]
- [x] P0-1: Create `pkg/convert/chunk.go` with `Chunk`, `ChunkOptions`, `Chunk` struct (Text/HeadingPath/Index); split precedence paragraph > heading > sentence > word; fenced-code and table-row atomicity
- [x] P0-2: Implement `countWords` (CJK-aware): whitespace tokens + CJK per-rune counting; unit tests for English / Chinese-only / mixed / punctuation / empty
- [x] P0-3: Implement heading-budget reservation: body budget = `MaxWords − words(headingPrefix)` (prefix bounded to last 100 chars); splitting happens on the reserved budget
- [x] P0-4: Create `pkg/convert/chunk_test.go` — RED first: paragraph split, heading inheritance (H1/H1>H2), first-chunk path, code-block atomic, table-row atomic, CJK chunking, mixed counting, heading-budget, empty input, determinism
- [x] P0-5: Property test: `words(Text)+words(headingPrefix) ≤ MaxWords` over 200 random docs incl. CJK fixtures (testing/quick)
- [x] P0-6: Confirm existing `convert` golden tests unchanged (7 fixtures stay single-concept — all convert to <160 chars, threshold-safe)

## P0: Cross-encoder feasibility spike (blocking risk) — [M2]
- [ ] P0-7: Locate ms-marco-MiniLM-L-6-v2 ONNX export (expected: none official — self-export/quantize with Python build-time toolchain, or community build); verify inference via `pure-onnx` `NewAdvancedSession` + `pure-tokenizers` `EncodePair` in a throwaway spike; record license + source + measured binary-size delta in spike notes
- [ ] P0-8: Add `scripts/fetch-crossencoder.sh` (mirrors fetch-model.sh) and embed resource in `internal/embeddings/assets` (checksum-verified, same mechanism as MiniLM)
- [ ] P0-9: Decision gate: if model unobtainable, substitute (e.g. MiniLM-L3 cross-encoder) and update proposal.md risks table with the substitution

## P1: Search dedupe by source — [M1]
- [x] P1-1: Add `sourceKey(c *Concept)` helper (`source_path` custom field, else FilePath with `__cN` stripped); unit tests for both paths
- [x] P1-2: Modify `SemanticSearch`: after RRF fusion, dedupe by `sourceKey` when `SearchOptions.DedupeBySource` (default true); survivor keeps highest score + records `DuplicateCount`; `false` restores raw chunk list
- [x] P1-3: `pkg/query/semantic_test.go`: one-slot-per-source (10-chunk doc, TopK=5), two-distinct-sources never collapse, dedupe-off returns all chunks, lexical path untouched

## P1: Reranker abstraction + pipeline — [M2]
- [ ] P1-4: Create `pkg/query/rerank.go`: `Reranker` interface + `RerankResults(query, results, ranker)` (stable sort by RerankScore desc, tie → original order)
- [ ] P1-5: Extend `SearchResult` (additive): `RerankScore float32`, `Heading string`, `ChunkIndex int`, `ChunkCount int`, `Warning string`, `DuplicateCount int`
- [ ] P1-6: Modify `SemanticSearch`: optional `Reranker` on `SearchOptions`; bounded candidates (≤20) → re-rank → TopK cut; nil = byte-identical today
- [ ] P1-7: Fail-open: ranker error → RRF order + `Warning` on first result, no error from `SemanticSearch` (no lexical fallback); `SemanticScore` stays the RRF score, never overwritten
- [ ] P1-8: `pkg/query/rerank_test.go`: fake ranker re-order, **identity-ranker byte-parity** (not circular), single-result skip, ranker-error → warning + RRF order, TopK cutoff, bounded candidates (≤20)
- [ ] P1-9: Create `pkg/embeddings/crossencoder.go`: `CrossEncoder` (session + tokenizer, mutex-serialized) + `Score(query, docs) ([]float32, error)`; unit tests with stub seam + `-race` concurrency + missing-model error
- [ ] P1-10: `Benchmark` test recording per-query re-rank latency (20 pairs) — budget observed in CI output

## P2: Chunk-aware import — [M1]
- [x] P2-1: Modify `cmd/okf/cmd_add.go`: chunk threshold check after conversion; large docs → write `<original>__cN.md` chunk concepts with **explicit titles** (`<doc title> — part N` or heading-derived — never filename-derived) + custom fields (chunk_index/chunk_count/heading_path/source_path/**derived: true**); small docs unchanged
- [x] P2-2: Verify `__cN` suffix does not collide with parser reserved names (index.md/log.md) — P0 check in cmd_add
- [x] P2-3: `cmd/okf/cmd_add_chunk_test.go`: small-doc single-concept, large-doc chunked, explicit title, derived metadata, **CJK large-doc chunked**, semantic deep-tail searchable, unchunked semantic control not retrievable, re-import idempotent
- [x] P2-4: `okf sync` derived-file lifecycle: source removed → `__cN` chunk files removed with it (DetectSourceMissing understands derived artifacts); direct chunk edits overwritten on re-import
- [x] P2-5: MCP `okf_import_document` chunked write — **failure-atomic**: temp files → fsync → rename; on failure remove temps + already-renamed chunk files; test injects write failure on chunk N and asserts no partial chunk set

## P2: Unified output + eval adaptation — [M1 eval foundation / M2 rerank fields]
- [x] P2-6: `cmd/okf/cmd_vector.go`: unified JSON output with rerank_score/heading/chunk_index/chunk_count/warning/duplicate_count; `-semantic` passes reranker (M2) or nil (M1)
- [ ] P2-7: `pkg/mcp/tools.go`: `okf_search`/`okf_semantic_search` include new fields; envelope version compatibility test
- [ ] P2-8: CLI/MCP JSON field parity test (same field names both sides)
- [x] P2-9: `pkg/eval/benchmark.go`: injectable `Searcher` (default lexical, unchanged); semantic-link mode runs `SemanticSearch` RRF-only and RRF+ranker over golden set; report distinguishes lexical vs semantic rows + delta
- [x] P2-10: `pkg/eval/benchmark_test.go`: lexical baseline no-regression (≥0.9); `MRR(rerank) ≥ MRR(rrf)` delta assertion; `TestChunkedBundleSearchable` (chunked semantic retrievable / unchunked semantic control not)

## P3: Docs + conformance + full regression
- [ ] P3-1: `docs/knowledge/search.md` (chunking + dedupe + re-ranking + derived-file behavior user doc)
- [x] P3-2: `README.md` feature line + `docs/knowledge/releases.md` v0.6.0 with benchmark delta + **binary size note (+~20MB cross-encoder)**
- [x] P3-3: `conformance.md` — map every spec scenario to a test, status fully/aligned/partial/gap; M1 PR covers chunking/import/dedupe/eval scenarios, M2 PR covers rerank scenarios
- [x] P3-4: `go build ./...` + `go vet ./...` + `gofmt -l` + `staticcheck ./...`
- [x] P3-5: `go test ./... -race` + `go test -shuffle=on ./...` + `python3 test_mcp.py`
- [ ] P3-6: Extend `tools/mutants.sh` to cover chunker + reranker; `tools/gauntlet.sh` full pass (coverage ≥60%, mutation killed, real execution incl. binary-size measurement)

## P4: Delivery
- [ ] P4-1: M1 commit on branch `feat/retrieval-quality-closed-loop` → push → PR → CI green
- [ ] P4-2: M2 commit (after spike passes) on same branch → push → PR → CI green
- [ ] P4-3: `tools/eval.sh` baseline recorded in PR body (lexical unchanged; semantic RRF vs rerank delta)
