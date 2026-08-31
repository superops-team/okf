# OKF Release Notes

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
