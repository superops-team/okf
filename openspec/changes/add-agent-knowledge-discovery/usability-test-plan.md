# Usability Test Plan — add-agent-knowledge-discovery

Date: 2026-09-12
Branch: spec/agent-knowledge-discovery (HEAD 02d1312)
Version under test: okf CLI 0.7.0

## Personas

1. **New user (Alex)**: first-time OKF user, has a folder of Markdown notes, wants to discover what's in the knowledge base and set up agent access.
2. **Existing user (Sam)**: has a v0.6.0 knowledge base with v2 vector index, upgrading to 0.7.0, needs stable IDs and agent config.
3. **Agent/automation (Bot)**: drives OKF via MCP stdio, needs machine-parseable JSON, stable error codes, no interactive prompts.
4. **Ops user (Riley)**: needs to recover from failures (corrupt index, partial writes, config conflicts), needs clear remediation.

## Preconditions

- `okf` binary built from current source (`go build -o okf ./cmd/okf`)
- Temporary isolated git repo per test (no pollution of real config/home)
- No `OKF_*` environment variables set unless explicitly testing
- MiniLM embedding model available (for semantic/hybrid tests)
- Python 3 for MCP Content-Length E2E

## Journey A: New user — discovery, identity, resolution

### A1. Command discoverability
- **Goal**: New user finds all new commands via `okf help`
- **Steps**: `okf help`, `okf identity --help`, `okf agent --help`, `okf tool --help`
- **Expected**: `identity`, `agent` subcommands visible; `tool manifest` visible; `search -group-by` flag documented; `eval -group-by` documented
- **Failure**: any new capability not discoverable from help

### A2. Empty knowledge base
- **Goal**: User runs identity/manifest on empty dir
- **Steps**: mkdir empty; `okf identity ensure --repo . --dir .`; `okf tool manifest --repo . --dir .`
- **Expected**: identity reports missing=0, no crash; manifest reports total=0; no implicit index build
- **Failure**: crash, panic, implicit model load, unhelpful error

### A3. No vector index — search degradation
- **Goal**: User searches without vector index
- **Steps**: `okf search -path . -q "test"` (no .okf/vector)
- **Expected**: falls back to lexical/BM25; clear warning that semantic channel unavailable; no crash
- **Failure**: silent degradation, confusing error

### A4. Old v2 vector index — remediation
- **Goal**: User with v2 index runs search/eval
- **Steps**: create v2 index (or use fixture); `okf search -path . -q test`; `okf eval -golden x -path . -compare`
- **Expected**: `index_rebuild_required` error with `okf vector rebuild` remediation; eval warns and falls back to lexical
- **Failure**: cryptic error, no remediation, v2 results silently returned as v3

### A5. First identity dry-run → apply → resolve
- **Goal**: User assigns stable IDs to concepts
- **Steps**: create 3 concept .md files; `okf identity ensure --repo . --dir knowledge` (dry-run); verify no files changed; `okf identity ensure --repo . --dir knowledge --apply`; verify IDs written; `okf identity resolve --repo . --dir knowledge --ref okf://concept/<id>`
- **Expected**: dry-run JSON has missing=3, no file modifications; apply writes okf_id to frontmatter; resolve returns correct path; second apply missing=0 (idempotent)
- **Failure**: dry-run modifies files; apply not idempotent; resolve fails for valid ID

### A6. Rename/move after identity
- **Goal**: Stable ID survives file rename
- **Steps**: rename a concept file; `okf identity resolve --ref <id>`
- **Expected**: resolve follows to new path; no re-migration needed
- **Failure**: resolve returns old path or not found

### A7. User doesn't understand ID/URI format
- **Goal**: User passes malformed ref
- **Steps**: `okf identity resolve --ref "my-id"`; `okf identity resolve --ref "okf://concept/okf_ZZZZ"` (invalid hex); `okf identity resolve --ref "okf://concept/okf_00000000000000000000000000000000"` (valid format, not present)
- **Expected**: clear error with code `invalid_ref` or `concept_ref_not_found`; remediation showing valid URI format; exit code non-zero
- **Failure**: vague error, panic, zero exit on failure

### A8. Duplicate ID detection
- **Goal**: User manually copies okf_id to two files
- **Steps**: set same okf_id in two files; `okf identity ensure --apply`
- **Expected**: error with both file paths, code `duplicate_okf_id`; no files modified (fail before writing)
- **Failure**: silently overwrites one, error missing path info

## Journey B: Manifest

### B1. Default output
- **Goal**: `okf tool manifest` shows useful info
- **Steps**: create 3 concepts; `okf tool manifest --repo . --dir knowledge`
- **Expected**: total=3, default limit=100, each item has title/type/path/estimated_tokens; no body content; human-readable text
- **Failure**: missing fields, body leak, unreadable output

### B2. JSON output
- **Goal**: Machine-parseable manifest
- **Steps**: `okf tool manifest --repo . --dir knowledge --json`
- **Expected**: valid JSON, same fields as text, total/limit/items structure
- **Failure**: invalid JSON, mixed stdout/stderr, missing fields

### B3. Pagination
- **Goal**: limit works
- **Steps**: `okf tool manifest --limit 2`; `okf tool manifest --limit 999`; `okf tool manifest --limit 0`
- **Expected**: limit=2 returns 2 items; limit=999 rejected with `invalid_request`; limit=0 rejected
- **Failure**: limit ignored, out-of-range silently accepted

### B4. Filter combinations
- **Goal**: type/source/folder filters work with AND/OR semantics
- **Steps**: create concepts with different types/sources/folders; filter by type=concept; filter by source_path; filter by folder; combine filters
- **Expected**: correct subset; OR-within-dimension AND-across; empty result returns total=0 not error
- **Failure**: wrong semantics, crash on empty

### B5. Corrupt frontmatter
- **Goal**: One bad file doesn't break whole manifest
- **Steps**: create concept with malformed YAML frontmatter; run manifest
- **Expected**: bad file skipped with stable warning code; other files still listed; no crash
- **Failure**: whole manifest fails, panic, silent skip without warning

### B6. Oversized frontmatter
- **Goal**: Bounded reader doesn't read body
- **Steps**: create file with 1MB frontmatter (or body); run manifest; instrument bytes read
- **Expected**: only frontmatter + one 4KiB prefetch read; body never streamed; `estimated_tokens=ceil(bytes/4)`
- **Failure**: body read, OOM, wrong token estimate

### B7. Duplicate IDs in manifest
- **Goal**: Manifest detects duplicate okf_id
- **Steps**: two files same okf_id; run manifest
- **Expected**: warning/error with both paths; doesn't silently dedupe
- **Failure**: silent dedupe, no warning

### B8. No implicit index/model startup
- **Goal**: Manifest is metadata-only
- **Steps**: run manifest in env without embedding model; check no model load, no index build
- **Expected**: works without model; no .okf/vector created; no embedder initialization
- **Failure**: implicit model load, index creation

### B9. CLI/Service/MCP consistency
- **Goal**: Same data across all three entry points
- **Steps**: run manifest via CLI, via Service (Go test), via MCP okf_manifest
- **Expected**: same total, same items, same field semantics
- **Failure**: discrepancies between entry points

## Journey C: Grouped retrieval

### C1. group_by omitted = legacy compatibility
- **Goal**: No grouping = old behavior
- **Steps**: `okf search -path . -q test` (no -group-by)
- **Expected**: output format identical to v0.6.0; no group headers; scores/sources unchanged
- **Failure**: output format changed, group headers appear without flag

### C2. chunk/concept/source/folder semantics
- **Goal**: Each grouping dimension produces understandable output
- **Steps**: run search with each -group-by value; check group keys, hit counts, representative
- **Expected**: chunk=per-chunk; concept=per-concept; source=per source_path; folder=per directory; group_key human-readable; hit_count/member evidence present
- **Failure**: opaque group keys, no counts, wrong grouping

### C3. Fewer than K results
- **Goal**: Grouping doesn't fabricate results
- **Steps**: query that returns 2 results; group by concept
- **Expected**: 2 groups (or fewer if same concept); no padding to K
- **Failure**: fabricated groups, empty groups

### C4. Invalid group_by value
- **Goal**: Clear error for invalid grouping
- **Steps**: `okf search -group-by bogus`; `okf eval -group-by bogus`; MCP okf_query group_by="bogus"
- **Expected**: `invalid_group_by` error with valid values listed; non-zero exit; MCP returns tool error
- **Failure**: silent fallback, panic, vague error

### C5. No semantic index — degradation warning
- **Goal**: Grouped search without vector index
- **Steps**: remove .okf/vector; `okf search -group-by source -q test`
- **Expected**: warning that semantic channel unavailable; grouping still works on lexical results; clear which channel active
- **Failure**: silent degradation, confusing output

### C6. Same query, different entry points
- **Goal**: CLI search, MCP okf_query, Service.Query consistent
- **Steps**: same query with same group_by across all three
- **Expected**: same groups, same representative, same ordering
- **Failure**: discrepancies

### C7. Hybrid utility gate (all three projections)
- **Goal**: Grouped relevant-source recall >= raw hybrid recall
- **Steps**: eval -group-by concept/source/folder on golden_semantic.json
- **Expected**: all three srcRecall >= raw hybrid Recall@5 (0.9231)
- **Failure**: any projection below raw (regression)

## Journey D: Agent Integration

### D1. Plan → apply → status → second apply (0 diff) → remove
- **Goal**: Full lifecycle for each client
- **Steps**: for cursor/claude-code/codex: plan (JSON), apply (--yes), status, apply again (should report 0 diff/already installed), remove (--yes)
- **Expected**: plan shows what will change; apply writes config; status shows installed; second apply no-op; remove cleans up; no credentials in output
- **Failure**: second apply re-writes, remove leaves residue, credentials leaked

### D2. all client
- **Goal**: `--client all` works
- **Steps**: `okf agent apply --client all --repo . --yes`; status for each
- **Expected**: all three configured; each status reports installed
- **Failure**: partial install, error on one client aborts others without info

### D3. Existing user config (unowned)
- **Goal**: Don't overwrite user's existing config
- **Steps**: pre-create mcpServers.okf with user's settings; `okf agent apply --client cursor`
- **Expected**: conflict error, file unchanged, code `conflict`; remediation explains ownership
- **Failure**: silently overwrites user config

### D4. Unknown keys in config
- **Goal**: Unknown keys preserved
- **Steps**: config with extra custom keys; apply; verify extra keys preserved
- **Expected**: OKF-managed section updated, user keys untouched
- **Failure**: user keys dropped

### D5. Unbalanced markers
- **Goal**: Corrupt OKF markers detected
- **Steps**: config with only BEGIN marker (no END); apply
- **Expected**: conflict error, no write, remediation
- **Failure**: writes into broken config, panic

### D6. Read-only file
- **Goal**: Clear error on permission failure
- **Steps**: chmod 444 config file; apply
- **Expected**: error with path, code `config_write_failed`, remediation; no partial write
- **Failure**: vague error, partial write

### D7. Symlink/path escape
- **Goal**: Repo boundary enforced
- **Steps**: config path symlink outside repo; apply
- **Expected**: rejected, code `path_escape`; no write outside repo
- **Failure**: writes outside repo

### D8. Partial write failure → rollback
- **Goal**: Multi-file apply is atomic
- **Steps**: inject failure on second file write (via read-only or fixture); apply
- **Expected**: first file rolled back; no partial state; error `agent_config_rollback_failed` or similar
- **Failure**: partial config left behind

### D9. Generated config actually starts MCP
- **Goal**: Generated config is valid
- **Steps**: apply for cursor; extract generated command; run `okf mcp --repo .` manually; verify stdio protocol responds
- **Expected**: `okf mcp` starts, responds to initialize, tools/list includes okf_* tools
- **Failure**: generated command wrong, MCP doesn't start

### D10. Canonical workflow guidance
- **Goal**: Agent can understand what to do from config comments
- **Steps**: read generated config; check for repo path, command, env vars, ownership marker
- **Expected**: comments explain OKF_MANAGED marker, how to uninstall, what the server does
- **Failure**: cryptic config, no guidance

### D11. Interactive vs non-interactive
- **Goal**: --yes flag works; without it, prompts or fails appropriately
- **Steps**: apply without --yes (pipe empty stdin); apply with --yes
- **Expected**: without --yes: either prompts or fails with "use --yes"; with --yes: no prompt
- **Failure**: hangs waiting for input in script, ignores --yes

## Journey E: Compatibility & migration

### E1. Legacy bundle (no IDs) works normally
- **Goal**: Concepts without okf_id fully functional
- **Steps**: search, manifest, eval on legacy concepts
- **Expected**: all work; manifest shows no okf_id; search returns results; no errors
- **Failure**: legacy concepts rejected, errors

### E2. Writer preserves ID
- **Goal**: Owned writers (document import, code generation) preserve existing okf_id
- **Steps**: concept with okf_id; re-import/regenerate; verify ID unchanged
- **Expected**: okf_id preserved, not regenerated; new concepts get new ID
- **Failure**: ID overwritten, lost

### E3. Derived chunks parent ID
- **Goal**: Chunk-derived concepts carry parent_okf_id
- **Steps**: import large document (creates __cN chunks); check parent_okf_id in derived chunks
- **Expected**: parent_okf_id set; concept grouping uses parent identity
- **Failure**: missing parent ID, grouping breaks

### E4. Vector v2 remediation
- **Goal**: Clear path from v2 to v3
- **Steps**: v2 index present; search; eval; `okf vector status`; `okf vector rebuild`
- **Expected**: status shows incompatible; search error with rebuild command; eval warns; rebuild creates v3; after rebuild all works
- **Failure**: no remediation, rebuild doesn't fix, v2 silently used

### E5. concept_id vs okf_id not confused
- **Goal**: No field name collision
- **Steps**: check frontmatter fields; check API responses; check MCP output
- **Expected**: okf_id is the stable ID; concept_id (if any) is distinct; no ambiguity in docs/help
- **Failure**: fields used interchangeably, docs confusing

## Journey F: Recoverability & usability

### F1. Error rubric: problem + no damage + next command
- **Goal**: Every user-facing error has: what went wrong, what was not damaged, what to do next
- **Steps**: trigger each error class (invalid ref, duplicate ID, path escape, config conflict, index incompatible); inspect error text
- **Expected**: error has code, message, path (if relevant), remediation command; JSON mode has structured fields
- **Failure**: vague "error occurred", no remediation, missing code

### F2. Exit codes script-friendly
- **Goal**: Non-zero on failure, zero on success
- **Steps**: run failing commands; check $?
- **Expected**: all failures non-zero; dry-run success zero; idempotent apply zero
- **Failure**: zero exit on failure, non-zero on success

### F3. JSON machine-parseable, clean streams
- **Goal**: --json outputs valid JSON to stdout; errors to stderr
- **Steps**: run commands with --json; pipe to jq; check stderr
- **Expected**: stdout is pure JSON; stderr has warnings/errors; no mixed content
- **Failure**: JSON mixed with text, errors on stdout, invalid JSON

### F4. Help and README examples copy-pasteable
- **Goal**: Examples in help/README actually work
- **Steps**: copy example from `okf identity --help`; run it; copy from README
- **Expected**: examples work with minimal adaptation; flag names match
- **Failure**: examples use wrong flags, don't work

### F5. Output verbosity reasonable
- **Goal**: Not too much, not too little
- **Steps**: run identity ensure (dry-run), manifest, agent plan; check output size
- **Expected**: key info visible; no debug spam; --verbose adds detail
- **Failure**: wall of text, missing key info

## Journey G: Real Agent smoke (MCP stdio)

### G1. Three-client config round-trip + MCP startup
- **Goal**: Generated configs produce working MCP server
- **Steps**: in temp repo, apply for all three clients; extract command from each; start `okf mcp --repo .`; send initialize; tools/list
- **Expected**: server responds to initialize; tools/list includes okf_resolve, okf_manifest, okf_query, okf_search, etc.; cwd correct
- **Failure**: server doesn't start, tools missing, wrong cwd

### G2. Actual tool calls through MCP
- **Goal**: Tools work end-to-end
- **Steps**: initialize; call okf_manifest (expect total=N); call okf_query with group_by=source; call okf_resolve with valid ref
- **Expected**: valid responses; manifest no body leak; query grouping works; resolve follows IDs
- **Failure**: tool errors, wrong responses, body leak

### G3. Error handling through MCP
- **Goal**: MCP tool errors are structured
- **Steps**: call okf_resolve with bad ref; call okf_query with bad group_by; call okf_manifest with bad limit
- **Expected**: tool returns isError=true with structured content; code/message/remediation present
- **Failure**: crash, empty error, vague message

### G4. Controlled note/feedback (if applicable)
- **Goal**: Durable capture works through MCP
- **Steps**: call okf_note (if exists) with content; verify file written with okf_id; re-query finds it
- **Expected**: note persisted, ID assigned, queryable; no credential leak
- **Failure**: note not saved, ID missing, not queryable

## Automation levels

| Level | Tool | Coverage |
|---|---|---|
| Go unit/property/fuzz | pkg/*_test.go | Logic, edge cases, determinism |
| CLI black-box | cmd/okf/*_test.go + verify script | User-visible output, exit codes, help text |
| MCP Content-Length E2E | test_mcp.py + verify script | Protocol, tool contract, cwd |
| Three-client fixture round-trip | verify-agent-usability.sh | Config parse/generate/parse |
| Golden/help/error text | help_golden_test.go + fixtures | Text stability, remediation |
| Failure injection | read-only files, symlinks, corrupt fixtures | Rollback, error paths |

## Evidence

All results recorded in `usability-evidence.md`. Each journey lists actual commands, outputs, issues found, fixes applied, and remaining limitations.
