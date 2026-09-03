# OKF Release Notes

## v0.5.0 (unreleased) — Retrieval Quality: Chunking, BM25 and Weighted RRF

> Minor version bump: this release changes retrieval behaviour and the on-disk
> vector index format. **`okf vector rebuild` is required after upgrading.**

### Breaking: vector index format v2

The vector index now stores one vector per **chunk** instead of per concept, so
keys changed from `<concept fingerprint>` to `<concept fingerprint>#<ordinal>`.

- `index.meta.json` carries `index_format_version` (now `2`).
- Loading an index written by an earlier version **fails with an explicit error**
  telling you to run `okf vector rebuild`; semantic search falls back to lexical
  search meanwhile. It deliberately does not silently reuse the old index,
  because concept-level keys cannot be resolved back to chunks and would
  return empty or wrong results.
- `okf vector status` now reports chunk count, concept count and format version.

### New: heading-aware chunking

- New package `pkg/chunk` splits concepts on `##`–`####` headings into chunks of
  at most 1024 characters, each prefixed with a `Title > Section` breadcrumb so
  an isolated chunk keeps its context. Code fences and tables are never split;
  oversized segments fall back to paragraph, line and finally rune-boundary
  splitting; adjacent tiny chunks under the same heading are merged.
- **Why:** MiniLM truncates input at 256 tokens. Indexing whole concepts meant
  only **29.4%** of this repository's knowledge-base content ever reached the
  index — **70.6% was silently unsearchable**. After chunking, coverage is
  complete (7 concepts → 97 chunks, 0 oversized chunks).

### New: BM25 lexical channel

- New package `pkg/lexical` provides tokenization and BM25 (`k1=1.2`, `b=0.75`),
  standard library only, no dictionary or third-party dependency.
- Tokenization keeps whole identifiers *and* their subwords
  (`okf_semantic_search` → `okf` / `semantic` / `search`, `HTTPServer` →
  `http` / `server`) and splits CJK runs into overlapping bigrams.
- **Why:** the previous lexical channel was whole-string substring matching with
  no scoring. Multi-word and Chinese queries scored **Recall 0.0769** on the new
  golden set — as an RRF input it was mostly noise.

### New: configurable weighted RRF

- `query.SearchOptions` gains `RRFK`, `VectorWeight`, `LexicalWeight`,
  `CandidateFactor` and a pluggable `Lexical` backend.
- Defaults: `k=60` (matching Elasticsearch / Milvus convention) and **equal
  weights** for the two channels. Equal weighting was chosen from a measured
  sweep of vector:lexical ratios from 0.8 to 4.0 on two independent query sets;
  differences inside the 0.8–1.5 band were within one case of noise, so no
  single "peak" ratio was fitted.
- `okf search -lexical-weight W` tunes the blend; `0` disables the lexical
  channel for pure semantic search.

### New: `okf eval` command

- `pkg/eval` gains injectable strategies (`RunBenchmarkWith`,
  `CompareStrategies`, `FormatComparison`) so one golden set can score several
  retrieval strategies.
- `okf eval -golden <set> [-compare] [-verbose]` exposes this on the CLI, and
  warns when a golden set's expected documents are missing from the knowledge
  base (which otherwise yields all-zero metrics that look like total failure).
- New golden set `pkg/eval/testdata/golden_semantic.json`: 28 natural-language,
  multi-word, identifier and Chinese queries over `docs/knowledge`. The existing
  `golden_queries.json` cases are exact single-word lookups that score full
  marks on substring matching alone and therefore cannot discriminate
  strategies.

### Fixed: reproducibility

- `pkg/vectorindex` pins the HNSW RNG seed, and searches indexes below 2048
  chunks by scanning stored vectors exactly instead of traversing the graph.
  Identical queries could otherwise return different orderings across index
  rebuilds. Two independent causes had to be addressed: the underlying library
  picks its layer entry point by Go map iteration order (fixing the seed alone
  does not help), and its `Search` is approximate — asking for every node does
  **not** return every node (97 requested, 96 returned; 500 requested, 427
  returned), with the omissions shifting as the graph changes. On this
  knowledge base that surfaced as one query's top hit alternating between two
  documents, moving the reported MRR between 0.7577 and 0.7769 run to run.
  Results are now identical across repeated rebuilds.
- `Meta` records the sorted key list, both so a loaded index can restore the
  exact-search path and so the metadata bytes no longer depend on map order.
- `pkg/query` fusion ranking gained deterministic tie-breaks (score, semantic
  rank, semantic presence, source, fingerprint).
- `pkg/eval` now identifies documents by `FilePath` rather than the optional
  `Resource` frontmatter field, which is empty for converted concepts and made
  every metric collapse to zero on real bundles.
- The MCP `okf_semantic_search` tool now uses the same fused pipeline as the
  CLI. It passed no lexical backend, so it silently fell back to the substring
  channel (Recall@5 0.0769) — the agent-facing entry point was getting
  materially worse results than the command line. Chunking and BM25 index
  construction moved into `pkg/query` (`ConceptChunks`, `BuildLexicalBackend`)
  so both callers share one implementation and their chunk keys cannot drift.
- `SearchOptions.VectorWeight` can now be set to 0 via `WithVectorWeight` to
  disable the semantic channel, mirroring `WithLexicalWeight`. Assigning the
  field directly was indistinguishable from leaving it unset, so the value
  silently reverted to the default and a lexical-only configuration could not
  be expressed.

### Measured results

`okf eval -golden pkg/eval/testdata/golden_semantic.json -path docs/knowledge -compare`
(28 cases, 26 positive, K=5, means over positive cases):

| Strategy | Recall@5 | Precision@5 | MRR | NDCG@5 |
|---|---|---|---|---|
| `lexical-substring` (previous behaviour) | 0.0769 | 0.0769 | 0.0769 | 0.0769 |
| `bm25-only` | 0.8077 | 0.2744 | 0.6538 | 0.6941 |
| `semantic-only` | 0.9615 | 0.2641 | 0.7096 | 0.7685 |
| **`hybrid-default`** | **0.9615** | 0.2288 | **0.7256** | **0.7828** |

Hybrid retrieval edges out the semantic channel alone on MRR and NDCG. The gap is roughly one
query on a 26-query set, so treat it as "not worse, and better on lexical-heavy queries"
rather than a large win; the decisive gain here is over the pre-0.5.0 substring channel
(MRR 0.0769). Numbers are reproducible: rebuilding the index does not change them.

### Costs

- Index size grows ~20x and build time ~4x for the same content
  (7 concepts: 14.5 KB → 287 KB, 128 ms → 800 ms). Both scale with content
  volume rather than concept count.
- The BM25 index is built in memory per search invocation (<10 ms on this
  repository's knowledge base) and is not persisted, avoiding a second on-disk
  artifact to keep consistent with the vector index.

### Unchanged

- Core OKF v0.2 model (`Concept`, `KnowledgeBundle`) is untouched.
- `SmartImportSource`, `CollectFiles`, `ImportResult` and watch configuration
  are untouched.
- Retained IR metrics from the previous release: `Recall@K`, `Precision@K`,
  `MRR`, `NDCG@K` in `pkg/eval`, plus `tools/eval.sh`.

## v0.4.1 — Durable MCP write rollback

This patch release fixes durable MCP writes that fail after the target file has
become visible. Such failures now roll back the target file before returning an
error, preserving the atomic-write contract.

The CLI and MCP handshake report version 0.4.1.

## v0.4.0 — Vector Semantic Search

> Minor version bump: this release adds a new capability (local semantic search),
> not just fixes. Version becomes **0.4.0**.

### New: local semantic search

- `okf search -q "..." -semantic` performs natural-language search over concepts
  using a locally embedded MiniLM embedding model (384-dim) and an HNSW index.
- Fully offline and **no CGO**: the ONNX Runtime CPU library and a quantized
  MiniLM model are embedded into the binary (per-platform, via `go:embed`) and
  extracted to the user cache on first use (checksum-verified).
- Results blend semantic and lexical retrieval via RRF and are annotated with
  their source (`semantic` / `lexical` / `both`).
- New CLI group: `okf vector index | status | rebuild`.
- The MCP server exposes the same capability through `okf_semantic_search`.

### Behavior notes

- The ONNX Runtime shared library is loaded at runtime from the extracted cache
  (`os.UserCacheDir()/okf/`, override with `OKF_ORT_DIR`); the binary is
  self-contained but not statically linked.
- MiniLM truncates each concept to 256 tokens; embeddings are English-centric,
  so Chinese semantic quality is limited (lexical search still applies).
- Model/library sources and licenses are declared in the README; resources are
  fetched at build time by `scripts/fetch-ort.sh` / `scripts/fetch-model.sh`.
- New pinned dependencies: pure-onnx v0.0.1 (MIT), coder/hnsw v0.6.1 (CC0-1.0);
  `renameio` is replaced by a local cross-platform fork (upstream v1.0.1 does not
  compile on Windows).

## v0.3.0 — Document Format Import & Durable MCP Knowledge

> Versioning was unified starting at **v0.3.0**; subsequent releases bump the
> patch version (v0.3.x). Earlier v1.x tags are retired.

### New: document format import

- `okf add` now automatically imports PDF, DOCX, XLSX, PPTX, HTML, CSV, TXT
  and DOC files by converting them to Markdown (`<original>.md`) through the
  built-in pure-Go converter (`pkg/convert`, backed by downmark).
- No Python, CGO or external binaries required; conversion runs locally.
- MCP server gains the `okf_import_document` tool.
- Go toolchain requirement is now **1.26.0** (`toolchain go1.26.7`).

### New: durable MCP knowledge capture

- The MCP server exposes shared repository operations (`okf_status`, `okf_init`,
  `okf_refresh`, `okf_query`, `okf_context`) through a single `pkg/tool.Service`.
- Durable, explicit knowledge capture is available through `okf_note`, `okf_log`,
  and `okf_feedback`; `okf_ask` queries only those note/event/feedback concepts.
- Writes are idempotent (stable `idempotency_key`), atomic (temp-file + fsync +
  rename), and fail closed on unknown fields, oversize input, symlink roots, and
  credential-like metadata. The response envelope uses schema version `okf.tool.v1`.
- The server persists only feedback explicitly submitted by the caller and does
  not inspect a host application's private event bus.

### Behavior changes

- `okf add <non-md>` previously reported "No markdown files found" and skipped
  silently; it now converts and imports the document. This is the intended
  change for document support.
- Binary size grows ~10MB due to the bundled downmark conversion engine.

### Re-import semantics (important)

Document conversion is **deterministic and pinned to downmark v0.10.0**
(precise version in go.mod). Change detection identifies documents by their
original path, so an unchanged document is a no-change on re-import.

**A downmark dependency upgrade may change conversion output and trigger a
one-time re-import of previously imported documents.** This is an accepted,
documented behavior change. To avoid a mass re-import on upgrade, clear
`.metadata.json` first and re-import once, or pin the previous downmark
version until you are ready.

### Upgrade notes

- Users building from source must upgrade to Go 1.26+ (`go install` will pick
  the toolchain automatically via `GOTOOLCHAIN=auto`).
- The okf version is reported by `okf version` and by the MCP server handshake;
  both now report **v0.3.0**.
