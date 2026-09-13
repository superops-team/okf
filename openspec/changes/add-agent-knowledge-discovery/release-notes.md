# Release Notes — Agent Knowledge Discovery

Change: `add-agent-knowledge-discovery` (S01–S56).
Target CLI version: **0.7.0** (`cmd/okf/main.go`).
Library meta version remains `0.4.1` (`pkg/okf/meta/version.go`), overridable at
release time via `-ldflags "-X ...meta.Version=v0.7.0"`; the two version strings
serve different purposes (CLI user-facing vs. library build stamp) and are
intentionally not forced to match.
This is an additive release: no OKF v0.2 required field changes, no new third-party
dependency, and the ungrouped retrieval path is byte-for-byte unchanged.

Real Agent client E2E (S51–S56, four-layer): Codex fully passes adapter fixture,
official config discovery, real model MCP calls, and final answer/effect. Claude
Code and Cursor pass adapter fixture + official config discovery but are
BLOCKED_AUTH at the model layer in this environment (fail-closed, never
aggregated into PASS). See `tools/verify-real-agent-e2e.sh` and `evidence.md`.

## New capabilities

- **Optional stable concept identity (`okf_id`)** — concepts may carry
  `okf_id: okf_<32 hex>` (URI `okf://concept/<id>`). Legacy concepts without it
  remain valid and unmodified. Assignment is explicit only, via
  `okf identity ensure` (dry-run by default; `--apply` writes, validates the whole
  plan first, replaces each file atomically, and is idempotent). Resolution via
  `Service.Resolve`, `okf identity resolve --ref`, and MCP `okf_resolve`.
- **Identity-aware vector index v3** — keys are `v3:id:<okf_id>` for stable
  concepts and `v3:legacy:<fingerprint>` otherwise; chunk keys append the ordinal.
- **Metadata-only Manifest** — `okf tool manifest` (CLI) and `okf_manifest` (MCP)
  list bounded frontmatter + file metadata: no body, no embedding, no index
  build/load side effects. Pagination `limit` 1..500 (omitted ⇒ 100), filters
  OR-within-dimension / AND-across, `estimated_tokens = ceil(bytes/4)`.
- **Hierarchical grouped retrieval** — `okf search -group-by chunk|concept|source|folder`
  and `okf eval -group-by` project the existing fused/scored candidate window
  before final dedupe/TopK. Omitted `group-by` keeps legacy output exactly.
- **Project-scoped agent integration** — `okf agent plan|apply|status|remove
  --client cursor|claude-code|codex|all`. Ownership marked with the non-secret
  `OKF_MANAGED=agentconfig-v1`; no credentials stored; no `--force`.

## Behavior changes (read before upgrading)

1. **Vector index v2 must be rebuilt.** Loading a pre-v2 or v2 index against the
   v3 lookup returns status `incompatible` and the stable error
   `index_rebuild_required`, naming `okf vector rebuild`. Search falls back to
   lexical during this window; no v2 vector hit is ever returned as a v3 hit.
   Running `okf identity ensure --apply` reports `vector_rebuild_required=true`.
2. **Ungrouped retrieval compatibility is preserved.** When `group_by` is omitted,
   result order, score/source meaning, and non-additive text golden are unchanged.
   Grouping only activates on an explicit request.
3. **Client config ownership is strict.** `okf agent apply` never overwrites an
   existing `mcpServers.okf` / Codex `[mcp_servers.okf]` / owned rule file that is
   malformed, unowned, or has unbalanced markers — it reports `conflict` and
   leaves the file byte-identical. There is no force flag. Remove is likewise
   ownership-gated.
4. **Manifest never reads bodies.** A multi-megabyte Markdown body costs only its
   frontmatter bytes plus one 4 KiB read prefetch; broken frontmatter omits just
   that file with a stable warning code and scanning continues.
5. **`identity ensure --apply` rewrites frontmatter.** Injecting the stable
   `okf_id` re-serializes the concept frontmatter, so inline comments and the
   original key order may be normalized (the Markdown body is preserved). The
   rewrite is atomic per file and idempotent on re-run.

## Compatibility

- Fully backward compatible with existing knowledge bases: no `okf_id` required,
  no migration forced, no on-disk format migration for concept files.
- MCP `okf_manifest` and `okf_resolve` reuse the existing `okf.tool.v1` envelope
  with additive fields.
- Go `1.26.0` (toolchain `go1.26.7`); no new module dependencies.

## One-command verification

```bash
tools/gauntlet.sh                 # build/vet/gofmt/staticcheck/race/coverage/shuffle/mutation/real-exec + new layers
tools/verify-agent-discovery.sh    # identity/manifest/grouped-query/agent/eval/1000-file evidence → evidence.md
tools/mutants-agent-discovery.sh   # targeted mutation set for the new code
```
