# OKF Release Notes

## v1.3.0 (unreleased) — Document Format Import

### New: document format import

- `okf add` now automatically imports PDF, DOCX, XLSX, PPTX, HTML, CSV, TXT
  and DOC files by converting them to Markdown (`<original>.md`) through the
  built-in pure-Go converter (`pkg/convert`, backed by downmark).
- No Python, CGO or external binaries required; conversion runs locally.
- MCP server gains the `okf_import_document` tool.
- Go toolchain requirement is now **1.26.0** (`toolchain go1.26.7`).

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
