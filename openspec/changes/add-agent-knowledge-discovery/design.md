# Design: Agent Knowledge Discovery and Integration

## 1. Context

### 1.1 Existing architecture to preserve

- Markdown Concept + YAML frontmatter is the only knowledge source of truth.
- `pkg/okf.Concept.CustomFields` preserves non-standard frontmatter extensions.
- `pkg/query.SemanticSearch` owns MiniLM/BM25/weighted-RRF retrieval and source dedupe.
- `pkg/tool.Service` owns agent-facing status/query/context semantics and stable envelopes.
- `pkg/mcp.ToolRegistry` exposes legacy bundle tools and service-backed agent tools.
- CLI classic search and MCP semantic search separately convert `okf.Concept` to `query.Concept`.
- Existing `Context` reads repository source files; it cannot satisfy metadata-only discovery.

### 1.2 Confirmed gaps

1. `query.Fingerprint` uses type, title and path; identity changes after rename/move.
2. `cmd/okf/main.go:toQueryBundle` and `pkg/mcp/tools.go:mcpToQueryBundle` omit `CustomFields`, losing `source_path`, future `okf_id`, and grouping metadata.
3. There is no frontmatter-only discovery contract.
4. Existing source dedupe is a fixed search behavior, not a general result projection.
5. MCP availability does not install or maintain client-specific project integration.

## 2. Design principles

- **One model**: no database identity catalog; `okf_id` lives in the Markdown frontmatter extension map.
- **One service**: Manifest and grouped query extend `pkg/tool.Service`; client adapters do not reimplement knowledge behavior.
- **One ranking**: grouping is a pure projection after retrieval; it never computes a new relevance formula.
- **Explicit mutation**: read operations never add IDs or modify client files.
- **Fail closed**: duplicate IDs, path escape, malformed managed state and config ownership conflicts block mutation.
- **Backward-compatible by omission**: old callers that do not pass new fields keep current behavior.
- **No new third-party dependency by default**: IDs use `crypto/rand`; JSON uses `encoding/json`; TOML edits use a narrow owned-block strategy rather than adding a general TOML library.

## 3. Stable Concept ID

### 3.1 Data contract

`okf_id` is an OKF implementation extension, not a Google OKF v0.2 field.

```yaml
---
type: source
okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e
---
```

Grammar:

```text
okf_id = "okf_" 32( DIGIT / %x61-66 )
```

- Lowercase canonical form only.
- 128 random bits generated with `crypto/rand.Reader`.
- URI: `okf://concept/<okf_id>`.
- Invalid strings are preserved by generic parsing for compatibility but rejected by identity-aware operations with `invalid_concept_id`.

### 3.2 Package boundary

New `pkg/identity` owns pure identity semantics:

```go
const Field = "okf_id"

type State string // stable | legacy-unstable | invalid

type Ref struct {
    ID       string
    URI      string
    State    State
    FilePath string
}

func Parse(value string) (string, error)
func New() (string, error)
func FromConcept(*okf.Concept) Ref
func Resolve([]*okf.Concept, string) (*okf.Concept, error)
func BuildRegistry([]*okf.Concept) (*Registry, error)
func Key(id, legacyFingerprint string) string
```

`BuildRegistry` validates all explicit IDs and rejects duplicates before returning a resolver. There is no mutable global registry. `pkg/identity` may depend on `pkg/okf` but SHALL NOT depend on `pkg/query`; `pkg/query` calls the string-level `identity.Key` helper so the new adapter cannot create an import cycle.

Public resolution is wired once through `pkg/tool.Service.Resolve(ResolveRequest{Ref})`, CLI `okf identity resolve --ref <okf://...> [--format json]`, and MCP `okf_resolve`. All three return the current bundle-relative path and stable ref through one request/result contract; missing refs fail with `concept_ref_not_found`.

### 3.3 Creation and migration

All OKF-owned concept writers call a shared final-destination identity helper immediately before persistence. Temporary conversion/staging files SHALL NOT receive random IDs. At the final target, a writer preserves the existing valid `okf_id` when rewriting the same owned logical artifact and generates a new ID only for a genuinely new target. Generated refresh matches the existing artifact by its current owned logical identity/path before serialization; document import checks the final destination rather than staging; derived chunks persist `parent_okf_id`. Existing durable-capture `concept_id` remains the deterministic idempotency/path handle and keeps its public response semantics; the new random `okf_id` is stored separately and exposed as an additive stable ref. The two fields are never substituted for one another.

Legacy user-authored concepts remain legal without ID. `okf identity ensure` scans and reports:

```json
{
  "operation": "identity.ensure",
  "apply": false,
  "missing": 8,
  "existing": 12,
  "invalid": [],
  "duplicates": [],
  "changes": [{"path":"a.md","action":"add"}]
}
```

- Default is dry-run. Dry-run validates and lists target paths/actions but does not consume randomness or print proposed IDs, so repeated previews are deterministic.
- `--apply` performs full validation first, then creates temp files in the same directories, fsyncs, and atomically replaces each individual file.
- Cross-file atomicity is not claimed. A mid-commit failure rolls back already-replaced files from in-memory original bytes; rollback failure is reported as `identity_migration_partial` with affected paths. The test harness injects write/rename failures.
- Symlinks, paths outside the knowledge root and reserved files are rejected.
- No alias/path-history table is created.

### 3.4 Index compatibility

Introduce index key version `v3`:

```text
concept key with ID: v3:id:<okf_id>
legacy concept key:  v3:legacy:<legacy-fingerprint>
chunk key:           <concept-key>#<ordinal>
```

The index metadata format version is incremented. A v2 index is never queried by v3 code. `vector status` reports `incompatible` and remediation `okf vector rebuild`; semantic search may use its existing lexical fallback but must warn. Adding IDs changes keys, so `identity ensure --apply` reports `vector_rebuild_required=true` when an index exists.

### 3.5 Conversion boundaries

Create one canonical conversion helper in `pkg/query` or an internal adapter package and use it from CLI, MCP and Service. It copies `CustomFields` defensively. This removes the current two divergent conversion functions and is locked by table tests.

## 4. Manifest metadata discovery

### 4.1 Why a separate operation

`Context` requires a query and reads source bodies. Making query optional would create ambiguous behavior and violate the zero-body guarantee. Manifest is therefore a narrow operation on the existing Service, not another retrieval package.

### 4.2 Request and response

```go
type ManifestRequest struct {
    Offset       int      `json:"offset,omitempty"`
    Limit        *int     `json:"limit,omitempty"`
    Types        []string `json:"types,omitempty"`
    Tags         []string `json:"tags,omitempty"`
    Statuses     []string `json:"statuses,omitempty"`
    Stale        *bool    `json:"stale,omitempty"`
    FolderPrefix string   `json:"folder_prefix,omitempty"`
    IncludeTrace bool     `json:"include_trace,omitempty"`
}
```

Defaults and limits:

- `offset=0`; negative is `invalid_request`.
- `limit=nil` means 100; an explicitly provided limit must be `1..500`; out-of-range values are `invalid_request`, not silently clamped.
- Filters use AND between dimensions and OR within each list.
- Empty filter lists match all.
- Status absence is exposed as effective `stable`, matching OKF v0.2 semantics.
- `folder_prefix` is a normalized bundle-relative POSIX prefix; absolute paths and `..` are rejected.

```go
type ManifestResult struct {
    Total       int            `json:"total"`
    Offset      int            `json:"offset"`
    Limit       int            `json:"limit"`
    Items       []ManifestItem `json:"items"`
    IndexStatus IndexStatus    `json:"index_status"`
    Trace       []TraceStep    `json:"trace,omitempty"`
}
```

`ManifestItem` fields:

- `okf_id`, `ref`, `identity_state`
- `path`, `title`, `description`, `type`, `tags`
- `status`, `trust_tier`, `stale`, `stale_after`
- `generated_at`, `verified_at` (latest valid timestamp)
- `source_count`, `source_resources` (at most 3, no source body)
- `file_size_bytes`, `estimated_tokens`, `estimate_kind=file_bytes_div4`

`IndexStatus` is observed from index metadata only: `missing|ready|stale|incompatible|unreadable`. Manifest does not load embeddings or mutate the index.

### 4.3 Frontmatter-only reader

New `pkg/manifest` walks `.md` files and reads at most 256 KiB until the second frontmatter delimiter. It does not call `parser.ParseConcept(path)` because that reads the entire file. It decodes only the frontmatter into metadata types and uses `os.File.Stat` for file size.

Testability boundary:

```go
type FileReader interface {
    Open(path string) (io.ReadCloser, error)
    Stat(path string) (fs.FileInfo, error)
}
```

The reader uses a fixed 4 KiB buffer. YAML decoding stops at the closing delimiter and no body content is parsed or returned; a single buffer may prefetch at most 4 KiB past the delimiter. Instrumented-reader tests assert this bound on a multi-megabyte body, avoiding an inefficient byte-at-a-time implementation. A missing closing delimiter emits `manifest_frontmatter_missing`; a frontmatter section over 256 KiB emits `manifest_frontmatter_too_large`; invalid YAML within a bounded header emits `manifest_frontmatter_invalid`. Each warning omits only that file and scanning continues. Duplicate explicit IDs fail the entire request because refs would be ambiguous.

Ordering is `path ASC, okf_id ASC`; all paths are slash-normalized and relative to the resolved knowledge root.

## 5. Hierarchical retrieval projection

### 5.1 Scope

Projection accepts the existing fused/scored deterministic candidate order after current score sorting and before final result shaping (`source` dedupe and TopK) when grouping is explicitly requested. Ungrouped execution retains the current dedupe-and-TopK path byte-for-byte. The grouped path does not call an embedder itself, query an index itself, change channel candidates, modify scores or introduce a ranking formula; it only replaces final output shaping.

```go
type GroupBy string
const (
    GroupChunk GroupBy = "chunk"
    GroupConcept GroupBy = "concept"
    GroupSource GroupBy = "source"
    GroupFolder GroupBy = "folder"
)

type GroupedHit struct {
    GroupBy      GroupBy
    GroupKey     string
    Ref          string
    Representative QueryHit
    HitCount     int
    ConceptCount int
    SourceCount  int
    Members      []HitRef `json:"members,omitempty"`
}

func Project(hits []ResultHit, groupBy GroupBy, includeMembers bool) ([]GroupedHit, []string, error)
```

The shared `ResultHit` adapter carries ID/ref, concept path, source path, location, rank, score and provenance. Classic semantic results and Service `QueryHit` are converted to it through tested adapters.

### 5.2 Group keys

- `chunk`: one key per finest-grained hit already exposed by that entry point: `<concept-key>@<startLine>:<endLine>` when a range exists, otherwise `<concept-key>@rank:<original-rank>`. It does not promise to reconstruct lower-level vector chunks that an existing entry point has already collapsed.
- `concept`: stable ID key `id:<okf_id>`; otherwise `legacy:<legacy-fingerprint>`.
- `source`: normalized `source_path`; if missing/invalid, concept key.
- `folder`: dirname of valid normalized source path, else concept path; root is `.`. If neither path is valid, concept key and warning.

Path normalization rejects absolute paths and any cleaned path whose first segment is `..`. Output always uses `/`.

Derived chunk concepts use the parent identity when a valid `parent_okf_id` extension is present. Without it they fall back to the normalized source key; the migration task updates derived writers to persist `parent_okf_id`.

### 5.3 Ranking and representative

- Projection first consumes the same finite candidate pool the existing query path already produced before final source dedupe/TopK. It does not increase semantic `CandidateK`, lexical candidate limits or Service scoring scope. It then emits at most the caller's requested number of groups. If that pool contains fewer than K unique keys, fewer than K groups are returned and no hidden second retrieval pass occurs.
- Input order is authoritative.
- The first input member is the representative.
- Group score equals the representative score.
- Groups are ordered by representative input rank, then `group_key` for defensive determinism.
- Counts use unique normalized keys.
- `include_members=false` returns no member list but still computes counts.

This deliberately avoids sum/average/max re-ranking beyond the existing ordered list.

### 5.4 Public wiring

- Classic CLI search: add `-group-by` and `-include-group-members`; omitted flags preserve current text output.
- Service `QueryRequest`: add optional `group_by` and `include_group_members`; `QueryResult` adds optional `groups`, while `results` remains the existing raw result list unless grouping is requested. When grouping is requested, `results` contains representatives for backward-friendly clients and `groups` contains group metadata.
- MCP `okf_query` schema exposes the same enum and returns the existing `okf.tool.v1` envelope with additive fields.
- Legacy `okf_search`/`okf_semantic_search`: add optional grouping only after routing through the shared projection; their omitted behavior remains unchanged.

## 6. Agent Integration

### 6.1 Package and command

New `pkg/agentconfig` owns project-file planning. It receives a repository root, selected client, resolved OKF binary command and canonical workflow model. It never calls query/MCP logic.

```text
okf agent plan   --client cursor|claude-code|codex|all [--format json]
okf agent apply  --client ... [--yes]
okf agent status --client ... [--format json]
okf agent remove --client ... [--yes]
```

- `plan`, `status` are read-only.
- `apply`, `remove` are mutating and require interactive confirmation; non-interactive use requires explicit `--yes`.
- All paths resolve under the repository root and reject symlink escape.
- The generated MCP command is `okf mcp --repo .`; every adapter contract fixture assumes the project-scoped client launches the server with the repository root as working directory. An adapter version that cannot guarantee project-root cwd is `unsupported` rather than persisting an absolute machine path.

### 6.2 Confirmed project adapters

| Client | MCP config | Workflow guidance | Ownership strategy |
|---|---|---|---|
| Cursor | `.cursor/mcp.json`, key `mcpServers.okf` with `env.OKF_MANAGED=agentconfig-v1` | `.cursor/rules/okf.md` | semantic JSON marker + whole OKF-created rule file |
| Claude Code | `.mcp.json`, key `mcpServers.okf` with `env.OKF_MANAGED=agentconfig-v1` | `.claude/skills/okf/SKILL.md` | semantic JSON marker + whole OKF-created skill directory |
| Codex | `.codex/config.toml`, table `[mcp_servers.okf]` | `AGENTS.md` managed block | delimited TOML table + delimited Markdown block |

Official references are recorded in `references.md`. Adapter versions are explicit constants and included in status.

### 6.3 Canonical workflow

A single typed model renders all guidance:

1. Call `okf_status`.
2. If knowledge is unavailable, ask before a mutating init/refresh.
3. Use `okf_manifest` for inventory/cold start; use `okf_query` for relevance; use `okf_context` only for selected evidence under a token budget.
4. Preserve stable `okf://` refs in evidence.
5. Perform the requested task.
6. Use `okf_note`/`okf_feedback` only when the user requested persistence or the workflow explicitly requires it; supply idempotency keys.
7. Never store credentials or unrelated private content.

Client-specific wrappers may change frontmatter syntax but not these behavioral clauses. Golden tests render every adapter and assert canonical clause IDs are present exactly once.

### 6.4 Plan, ownership and drift

Plan operations emit:

```json
{
  "client":"cursor",
  "status":"change|no_change|conflict|unsupported",
  "files":[{"path":".cursor/mcp.json","action":"merge","before_hash":"...","after_hash":"..."}],
  "warnings":[]
}
```

Rules:

- Parse valid JSON before changing it; malformed JSON is `conflict`. JSON is re-encoded deterministically, so unknown key/value semantics are preserved but original whitespace/key ordering is not promised.
- Preserve all unknown JSON keys and non-OKF MCP servers semantically. A JSON `mcpServers.okf` entry is owned only when its environment contains the exact non-secret marker `OKF_MANAGED=agentconfig-v1` (or a recognized prior adapter marker); an unmarked same-name entry is `conflict`. Marker-based TOML and Markdown edits preserve every byte outside the managed block.
- TOML adapter owns only a delimited block:
  `# BEGIN OKF MANAGED MCP v1` / `# END OKF MANAGED MCP v1`.
  If an unowned `[mcp_servers.okf]` table already exists, return conflict; do not parse/rewrite the whole TOML.
- `AGENTS.md` owns only `<!-- BEGIN OKF MANAGED AGENT v1 -->` through matching end marker. Missing/duplicate/unbalanced markers are conflict.
- Whole-file rule/skill ownership applies only if the file is absent or has the OKF ownership header. A pre-existing unowned file is conflict.
- Ownership is self-describing: JSON ownership is the exact non-secret `OKF_MANAGED` marker inside `mcpServers.okf`, marker ownership is the exact begin/end block, and whole-file ownership is the OKF header. No install-state file or original-file snapshot is required.
- Status is `missing|installed|drifted|conflict|unsupported`. `drifted` means a recognized OKF-owned block/file is syntactically valid but differs from the current deterministic render; `conflict` means ownership is ambiguous or the host file cannot be safely parsed.
- Remove deletes only a JSON entry carrying a recognized OKF ownership marker, a delimited block or a header-owned file. It removes any recognized OKF-owned version, so `--force` is not part of the public contract; same-name unmarked, ambiguous or unowned content is always a conflict and is never removed.
- Writes use temp-file + fsync + rename. Multi-file apply creates all proposed bytes first; on failure it restores changed files from process memory and reports any rollback failure.

### 6.5 Security

- Secret scan rejects literal values for key names matching token/password/secret/api_key/private_key in generated or proposed additions. Environment-variable references are allowed.
- Config output redacts values of suspicious existing fields from plan JSON.
- No command contains shell interpolation; config uses argument arrays.
- Symlink roots and path escapes fail closed.

## 7. Error contract

Additive `pkg/tool` codes:

- `invalid_concept_id`
- `duplicate_concept_id`
- `concept_ref_not_found`
- `manifest_frontmatter_too_large`
- `manifest_frontmatter_missing`
- `manifest_frontmatter_invalid`
- `invalid_group_by`
- `index_rebuild_required`
- `agent_config_conflict`
- `agent_config_drifted`
- `unsupported_agent_client`
- `unsafe_agent_config_path`

MCP returns these inside the existing ToolEnvelope/Error format. CLI exits non-zero and prints the stable code plus remediation.

## 8. Performance and stability

- Manifest: O(files log files) due to deterministic sort, reads at most 256 KiB per file before delimiter; does not allocate body-sized buffers.
- Projection: O(n) grouping plus deterministic stable order; no unbounded recursion.
- Registry: O(concepts) memory and time.
- Agent plan: bounded by the three known clients and owned config files; no network access.
- All public operations check `context.Context` between files/candidates.

## 9. Compatibility matrix

| Existing behavior | Compatibility decision |
|---|---|
| OKF v0.2 requires only type | unchanged; `okf_id` optional extension |
| Old Concept without ID | readable/searchable; ref marked `legacy-unstable` |
| Existing query without `group_by` | identical behavior |
| Existing `okf.tool.v1` caller | additive optional fields only |
| Existing vector index v2 | explicit incompatible status; rebuild required |
| Existing user Agent config | preserved unless exact OKF-owned key/block is changed |
| Existing AGENTS.md | only delimited OKF block is added/replaced |

## 10. Test strategy

- Unit: ID grammar/generator/registry, header reader, filters, group keys/ranking, adapters/merge.
- Property: ID round-trip and uniqueness sample; path normalization/idempotence; projection determinism/idempotence; JSON unknown-key preservation.
- Integration: rename/move resolution, identity migration rollback, Manifest instrumented-reader bound, index v2 rejection, CLI/MCP contract equality.
- Retrieval eval: current hybrid baseline, relevant-source Recall@5, MRR, NDCG, diversity and deterministic rebuild.
- Agent fixtures: existing unknown JSON/TOML/Markdown content, malformed files, drift, symlink escape, secret redaction, apply twice, remove.
- Real execution: one repository flow covering identity dry-run/apply, Manifest, grouped query, context ref, all client plans/applies/status/removes and MCP stdio.
- Gauntlet: build, vet, gofmt, staticcheck, race, coverage threshold, shuffle, targeted mutation, property tests, secret/dependency audit.

## 11. Rollout

One change set and one release, implemented in dependency phases. The CLI/MCP additions remain opt-in. Release Notes include ID semantics, vector rebuild requirement, client file ownership and rollback instructions.

## 12. Alternatives rejected

- SQLite stable-ID catalog: rejected as a second fact source.
- Path-hash presented as stable ID: rejected because rename changes it.
- Auto-ID during load: rejected because reads would mutate user files.
- Manifest through `Context(query="")`: rejected because Context reads bodies and has query semantics.
- New aggregation score: rejected until OKF golden-set evidence exists.
- Whole-file TOML rewrite: rejected without a TOML dependency because it risks losing comments and unknown syntax.
- Global client config: rejected due to blast radius and authorization concerns.
