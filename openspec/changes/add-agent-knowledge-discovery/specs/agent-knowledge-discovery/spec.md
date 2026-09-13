# Agent Knowledge Discovery Specification

## Requirement: Optional stable concept identity

The system SHALL support a persisted optional extension field named `okf_id` without changing the required field set of OKF v0.2. A canonical ID SHALL match `^okf_[0-9a-f]{32}$`; its canonical URI SHALL be `okf://concept/<okf_id>`.

### Scenario S01: Legacy concept remains valid
- **GIVEN** a valid OKF v0.2 Concept without `okf_id`
- **WHEN** the bundle is parsed, linted and queried
- **THEN** all operations succeed without modifying the file
- **AND** its identity state is `legacy-unstable`

### Scenario S02: Valid stable ID round-trips
- **GIVEN** a Concept with `okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e`
- **WHEN** it is parsed, converted across OKF/query adapters and serialized
- **THEN** the exact ID is preserved
- **AND** its ref is `okf://concept/okf_17a2c56db85c4889b4f8fe02ca9ac67e`

### Scenario S03: Invalid ID fails identity-aware use
- **GIVEN** a Concept whose `okf_id` is uppercase, truncated, overlong or contains a non-hex character
- **WHEN** identity registry, Manifest, migration or stable-ref resolution is requested
- **THEN** the operation fails with `invalid_concept_id`
- **AND** generic parse compatibility is not changed

### Scenario S04: Duplicate ID fails closed
- **GIVEN** two Concept files with the same valid `okf_id`
- **WHEN** an identity-aware operation starts
- **THEN** the operation fails with `duplicate_concept_id`
- **AND** the error identifies both bundle-relative paths
- **AND** no file or index is modified

### Scenario S05: Generated IDs have canonical entropy
- **WHEN** 10,000 IDs are generated using the production generator
- **THEN** each matches the canonical grammar
- **AND** no duplicate appears in the sample
- **AND** a controllable failing entropy reader produces a non-nil error rather than a fallback ID

## Requirement: Explicit and recoverable identity migration

The system SHALL add IDs to legacy Concepts only through an explicit migration command. Dry-run SHALL be the default; apply SHALL validate the complete plan before writing, SHALL atomically replace each individual file, and SHALL attempt deterministic rollback after a cross-file failure without claiming cross-file atomicity.

### Scenario S06: Migration defaults to deterministic dry-run
- **GIVEN** a bundle with two legacy Concepts and one valid stable Concept
- **WHEN** `okf identity ensure` runs repeatedly without `--apply`
- **THEN** it reports two planned additions and one preserved ID
- **AND** it lists paths/actions without generating or printing proposed random IDs
- **AND** all three files remain byte-identical
- **AND** repeated dry-run JSON is byte-identical

### Scenario S07: Apply is idempotent
- **WHEN** `okf identity ensure --apply` runs twice on the same bundle
- **THEN** the first run adds IDs only to missing Concepts
- **AND** the second run reports zero changes
- **AND** existing valid IDs are unchanged

### Scenario S08: Validation prevents partial migration
- **GIVEN** a bundle containing a duplicate or invalid explicit ID
- **WHEN** `okf identity ensure --apply` runs
- **THEN** validation fails before the first write
- **AND** every file remains byte-identical

### Scenario S09: Write failure triggers rollback
- **GIVEN** a valid migration plan and an injected rename failure after one replacement
- **WHEN** apply runs
- **THEN** previously replaced files are restored from their original bytes
- **AND** the command exits non-zero
- **AND** any rollback failure is reported as `identity_migration_partial` with affected paths

### Scenario S10: Unsafe paths are rejected
- **GIVEN** a Concept path that escapes the knowledge root or traverses a symlink root
- **WHEN** migration attempts to plan or apply it
- **THEN** the operation fails before writing

## Requirement: Stable reference resolution and writer wiring

The system SHALL use one identity service across all OKF-owned Concept writers and SHALL resolve a stable URI against the current bundle path through `Service.Resolve`, CLI `okf identity resolve` and MCP `okf_resolve`.

### Scenario S11: Ref survives rename and move through every resolver entry
- **GIVEN** a Concept with a stable ID
- **WHEN** its Markdown file and title are changed without changing `okf_id`
- **THEN** Service Resolve, `okf identity resolve --ref <uri>` and `okf_resolve` return the renamed Concept at its current bundle-relative path
- **AND** a syntactically valid unknown ref fails with `concept_ref_not_found`

### Scenario S12: Every owned writer persists stable identity without changing existing handles
- **WHEN** generated code knowledge, document import, MCP document import, durable note, event and feedback each create a new final Concept
- **THEN** each persisted Concept has one valid `okf_id`
- **AND** temporary conversion/staging artifacts do not receive random IDs
- **WHEN** the same owned logical artifact is refreshed or re-imported to the same final target
- **THEN** its existing valid `okf_id` is preserved
- **AND** derived chunks persist the parent Concept ID as `parent_okf_id`
- **AND** durable capture keeps its existing deterministic `concept_id` and response behavior as a separate idempotency/path handle
- **AND** `concept_id` is never interpreted as `okf_id`

### Scenario S13: Query conversion preserves metadata
- **GIVEN** an `okf.Concept` with `okf_id`, `source_path` and another custom field
- **WHEN** it crosses the canonical query conversion used by CLI, MCP and Service
- **THEN** all custom fields are preserved in the query Concept
- **AND** the destination map cannot mutate the source map

## Requirement: Identity-aware index compatibility

The system SHALL version identity-aware concept/chunk keys and SHALL never silently mix them with an older vector index.

### Scenario S14: Stable and legacy index keys are deterministic
- **GIVEN** one Concept with ID and one legacy Concept
- **WHEN** index keys are built repeatedly from equivalent bundles
- **THEN** the ID Concept key is `v3:id:<okf_id>`
- **AND** the legacy key starts with `v3:legacy:`
- **AND** chunk keys append the ordinal without losing the parent key

### Scenario S15: Old index requires explicit rebuild
- **GIVEN** a vector index with format v2
- **WHEN** v3 search or status loads it
- **THEN** status is `incompatible`
- **AND** the stable error is `index_rebuild_required`
- **AND** remediation names `okf vector rebuild`
- **AND** no v2 vector hit is returned as a v3 hit

### Scenario S16: ID migration reports index impact
- **GIVEN** an existing vector index and at least one planned ID addition
- **WHEN** identity migration is planned or applied
- **THEN** the result reports `vector_rebuild_required=true`

## Requirement: Metadata-only Manifest discovery

The system SHALL provide `Service.Manifest`, CLI `okf tool manifest` and MCP `okf_manifest` using one request/result contract and no query string.

### Scenario S17: Default Manifest listing
- **GIVEN** a valid bundle
- **WHEN** Manifest is called with an empty request
- **THEN** it returns at most 100 items
- **AND** offset is 0
- **AND** items are ordered by normalized bundle-relative path then ID

### Scenario S18: Pagination bounds are explicit
- **WHEN** offset is negative, or an explicitly provided limit is less than 1 or greater than 500
- **THEN** the request fails with `invalid_request`
- **AND** an omitted limit uses 100
- **AND** the request representation preserves omitted-versus-explicit presence

### Scenario S19: Filters have deterministic semantics
- **GIVEN** Concepts with different types, tags, effective statuses, stale states and folders
- **WHEN** multiple Manifest filters are provided
- **THEN** values within one dimension use OR
- **AND** dimensions use AND
- **AND** missing status is exposed and matched as `stable`

### Scenario S20: Manifest exposes bounded metadata
- **WHEN** a Concept has ID, provenance, status, verification and sources
- **THEN** its item includes identity/ref, path, title, description, type, tags, effective status, trust tier, stale data, latest valid generated/verified timestamp, source count and at most three source resource strings
- **AND** it never returns Markdown body content

### Scenario S21: Token estimate is labeled
- **GIVEN** a file of N bytes
- **WHEN** it appears in Manifest
- **THEN** `estimated_tokens=ceil(N/4)`
- **AND** `estimate_kind` is `file_bytes_div4`

## Requirement: Manifest has no body, embedding or index side effects

Manifest SHALL only read bounded frontmatter and file metadata, SHALL observe index metadata, and SHALL not trigger retrieval runtime initialization.

### Scenario S22: Body is not parsed or returned
- **GIVEN** a Concept with a small frontmatter and a multi-megabyte body
- **WHEN** Manifest reads the Concept through an instrumented 4 KiB buffered reader
- **THEN** YAML decoding stops at the closing frontmatter delimiter
- **AND** no body field is parsed or returned
- **AND** bytes read beyond the delimiter are at most one 4 KiB buffer prefetch

### Scenario S23: Header errors are bounded and stable
- **GIVEN** separate files whose frontmatter closure is absent, whose header exceeds 256 KiB, and whose bounded YAML is invalid
- **WHEN** Manifest scans them
- **THEN** they are omitted with warning codes `manifest_frontmatter_missing`, `manifest_frontmatter_too_large` and `manifest_frontmatter_invalid` respectively
- **AND** scanning continues for other files

### Scenario S24: Index is observed but never built
- **GIVEN** missing, ready, stale, incompatible and unreadable index metadata fixtures
- **WHEN** Manifest is called
- **THEN** it returns the corresponding `index_status`
- **AND** embedder, HNSW load/build and index write spies have zero calls

### Scenario S25: Duplicate stable refs invalidate Manifest
- **GIVEN** duplicate valid IDs
- **WHEN** Manifest runs
- **THEN** the entire request fails with `duplicate_concept_id`
- **AND** it does not return ambiguous partial items

### Scenario S26: CLI and MCP contracts match Service
- **GIVEN** the same repository and Manifest request
- **WHEN** Service, CLI JSON mode and MCP are invoked
- **THEN** their semantic result fields, ordering, errors and warnings are equivalent
- **AND** MCP uses the existing `okf.tool.v1` envelope with additive fields

## Requirement: Hierarchical result projection

The system SHALL project the existing fused/scored deterministic candidate order into `chunk`, `concept`, `source` or `folder` groups before final dedupe/TopK output shaping when grouping is explicitly requested. It SHALL NOT change channel candidate generation, scores or relevance formulas; the ungrouped path SHALL retain its existing output shaping.

### Scenario S27: Omitted grouping preserves behavior
- **GIVEN** an existing classic CLI, legacy MCP or Service query request without `group_by`
- **WHEN** it runs after this change
- **THEN** its existing result order, score/source meaning and non-additive text golden remain unchanged

### Scenario S28: Chunk projection keeps exposed hits distinct
- **GIVEN** multiple hit ranges from one Concept, or an entry point that exposes only Concept-level hits
- **WHEN** `group_by=chunk`
- **THEN** each distinct exposed hit/range remains a distinct group
- **AND** each representative preserves location, score and provenance
- **AND** the projection does not claim to reconstruct lower-level vector chunks already collapsed by the entry point

### Scenario S29: Concept projection uses stable parent identity
- **GIVEN** hits for a Concept and its derived chunks with valid `parent_okf_id`
- **WHEN** `group_by=concept`
- **THEN** they form one group keyed by the parent ID
- **AND** a legacy Concept falls back to a deterministic legacy key

### Scenario S30: Source projection removes source monopolization
- **GIVEN** multiple Concept/chunk hits with one normalized `source_path` and other sources
- **WHEN** `group_by=source`
- **THEN** that source occupies one group
- **AND** `hit_count`, `concept_count` and `source_count` are correct

### Scenario S31: Folder projection is portable and safe
- **GIVEN** Windows separators, POSIX separators, root files, missing source paths, absolute paths and `..` escapes
- **WHEN** `group_by=folder`
- **THEN** valid folders use bundle-relative `/` paths and root `.`
- **AND** invalid paths cannot become folder keys
- **AND** invalid paths fall back to concept grouping with warnings

### Scenario S32: Group order, limit and scores are deterministic
- **GIVEN** the same finite pre-dedupe candidate pool already produced by an existing query path, with tied scores and repeated group keys
- **WHEN** any grouping with requested limit K is applied repeatedly
- **THEN** no candidate limit is increased and no second retrieval pass occurs
- **AND** projection runs before final group truncation and returns at most K groups, possibly fewer when the pool has fewer unique keys
- **AND** each representative is the first raw member
- **AND** its score is unchanged
- **AND** group order is representative rank then group key
- **AND** output is byte-deterministic

### Scenario S33: Invalid grouping is rejected
- **WHEN** a public request uses any value outside `chunk|concept|source|folder`
- **THEN** it fails with `invalid_group_by`
- **AND** it does not silently fall back

### Scenario S34: Grouping is wired through all supported entries
- **GIVEN** equivalent hits in classic CLI semantic search, service query, `okf_query`, legacy `okf_semantic_search` and legacy `okf_search` where applicable
- **WHEN** the same explicit grouping is requested
- **THEN** the shared projection produces equivalent keys, representatives and counts

## Requirement: Project-scoped Agent Integration

The system SHALL support deterministic project adapters for Cursor, Claude Code and Codex through `okf agent plan|apply|status|remove` without modifying user-global configuration.

### Scenario S35: Plan is read-only and complete
- **GIVEN** a supported client and repository
- **WHEN** `okf agent plan --client <client> --format json` runs
- **THEN** it reports every proposed path, action and redacted hash
- **AND** it writes no file
- **AND** it does not start MCP, embedding or index processes
- **WHEN** apply or remove runs without interactive input and without `--yes`
- **THEN** it fails before writing

### Scenario S36: Apply is idempotent and preserves unowned content
- **GIVEN** valid client files containing unrelated servers, keys, comments or instructions
- **WHEN** apply runs twice
- **THEN** the first run adds/updates only OKF-owned content
- **AND** the second run produces zero diff
- **AND** JSON adapters preserve the semantic value of all unknown keys
- **AND** marker-based TOML/Markdown adapters preserve all bytes outside the managed region

### Scenario S37: Canonical workflow does not drift
- **WHEN** Cursor rule, Claude skill and Codex AGENTS block are rendered
- **THEN** every canonical workflow clause ID appears exactly once in each applicable output
- **AND** each references only currently registered OKF MCP tools and CLI commands

### Scenario S38: Client paths and syntax match contract fixtures
- **WHEN** adapters render current fixtures
- **THEN** Cursor uses `.cursor/mcp.json` plus `.cursor/rules/okf.md`
- **AND** Claude Code uses `.mcp.json` plus `.claude/skills/okf/SKILL.md`
- **AND** Codex uses `.codex/config.toml` plus the root `AGENTS.md` managed block
- **AND** generated JSON/TOML/Markdown passes the repository validators

### Scenario S39: Status detects missing, installed, drifted and conflict
- **GIVEN** fixtures for each state
- **WHEN** `okf agent status` runs
- **THEN** it returns the exact state per owned item
- **AND** unknown or unbalanced ownership markers are `conflict`, not `installed`

### Scenario S40: Remove is ownership-safe
- **GIVEN** an installed client configuration
- **WHEN** remove runs for a JSON entry with recognized `OKF_MANAGED` marker, a balanced managed block or an ownership-header file
- **THEN** it removes only that OKF-owned content even when the deterministic render has drifted across adapter versions
- **AND** all unrelated content remains
- **WHEN** ownership is ambiguous, a JSON same-name key lacks the ownership marker, markers are unbalanced or a same-name file lacks the OKF header
- **THEN** remove fails with `agent_config_conflict`
- **AND** it provides no force mode that can delete unowned content

## Requirement: Agent configuration safety and recoverability

Agent configuration SHALL reject unsafe paths, secrets and ambiguous ownership, SHALL atomically replace each individual file, and SHALL provide deterministic rollback evidence after a cross-file failure without claiming cross-file atomicity.

### Scenario S41: Existing malformed configuration is not overwritten
- **GIVEN** malformed JSON, an unowned Codex `[mcp_servers.okf]` table, an unowned OKF rule path or duplicate managed markers
- **WHEN** plan/apply runs
- **THEN** status is `conflict`
- **AND** every input file remains byte-identical

### Scenario S42: Credentials are never persisted or echoed
- **GIVEN** existing fields or proposed values containing token/password/secret/api_key/private_key names
- **WHEN** plan, ownership inspection or error reporting runs
- **THEN** literal secret values are absent from output and state
- **AND** generated OKF config uses no credential field
- **AND** environment-variable references remain allowed

### Scenario S43: Repository boundary is enforced
- **GIVEN** a selected output path through `..`, an absolute path or a symlink escape
- **WHEN** adapter planning or writing runs
- **THEN** it fails with `unsafe_agent_config_path`
- **AND** no external file is touched

### Scenario S44: Multi-file failure rolls back
- **GIVEN** a plan that changes multiple project files and an injected write/rename failure
- **WHEN** apply runs
- **THEN** changed files are restored from in-process original bytes
- **AND** the command exits non-zero
- **AND** rollback failures list affected paths

### Scenario S45: Unknown clients fail explicitly
- **WHEN** the client value is unknown or a known adapter version is unsupported
- **THEN** plan/status returns `unsupported_agent_client`
- **AND** apply/remove writes nothing

## Requirement: Quality, performance and release evidence

The implementation SHALL preserve current retrieval quality, provide deterministic resource bounds, and complete the project gauntlet and spec conformance audit.

### Scenario S46: Existing retrieval baseline does not regress
- **WHEN** the committed golden retrieval suite runs before and after the change with grouping omitted
- **THEN** hybrid Recall@5 and MRR are not lower than the committed baseline
- **AND** the raw ordered result fixture has no unexplained diff

### Scenario S47: Grouped retrieval has measured utility
- **WHEN** the expanded golden set evaluates concept/source/folder projection
- **THEN** relevant-source Recall@5 does not fall below the raw candidate set
- **AND** source grouping has at most one slot per source
- **AND** NDCG, diversity and same-source occupancy are reported, not guessed

### Scenario S48: Manifest and projection resource bounds are verified
- **WHEN** benchmarks run on fixtures containing 1,000 Concepts and large bodies
- **THEN** Manifest bytes-read is bounded by frontmatter bytes plus fixed per-file overhead
- **AND** projection uses O(n) additional memory with bounded candidate n
- **AND** measured results are recorded in EVIDENCE without a flaky wall-clock gate

### Scenario S49: Full executable gauntlet passes
- **WHEN** the final source state runs `tools/gauntlet.sh`, MCP protocol E2E, identity migration E2E, client fixture E2E, `tools/verify-real-agent-e2e.sh` and retrieval eval
- **THEN** build, vet, gofmt, staticcheck, race, coverage threshold, shuffle, mutation, property, secret scan and real execution all pass
- **AND** no skipped or `BLOCKED_*` layer is reported as passed
- **AND** release mode `REQUIRE_ALL_AGENT_MODELS=1` exits non-zero while any official-client model E2E remains blocked

### Scenario S50: Spec implementation conformance is complete
- **WHEN** implementation is ready for merge
- **THEN** `conformance.md` maps every S01–S56 Scenario to implementation, test and fresh result
- **AND** every `partial`, `gap` or `BLOCKED_*` state is explicit and has a blocking action
- **AND** the strict three-client model-E2E release gate cannot pass while such a state remains
- **AND** the source commit and reproducible commands are recorded

## Requirement: Official Agent client acceptance

Agent Integration SHALL be accepted in the official Cursor Agent, Claude Code and Codex clients. Configuration fixtures, direct MCP protocol calls, and hand-written MCP clients are prerequisites but SHALL NOT be reported as official Agent end-to-end evidence.

### Scenario S51: Official clients consume generated project configuration
- **GIVEN** `okf agent apply --client all --yes` generated the project files
- **WHEN** each installed official client inspects project MCP configuration
- **THEN** Cursor Agent, Claude Code and Codex each discover a server named `okf`
- **AND** standard JSON/TOML parsing confirms the generated command is `okf mcp --repo .`
- **AND** a client that is absent is `BLOCKED_CLIENT_MISSING`, not `PASS`

### Scenario S52: A real Agent produces a source-grounded answer through OKF MCP
- **GIVEN** a concept with stable ref and a repository-local `source_path` containing a deterministic canary fact
- **WHEN** an authenticated official Agent is instructed to use only OKF MCP
- **THEN** the recorded client event stream contains `okf_status`, `okf_manifest`, `okf_query`, then `okf_context`, once each and in order
- **AND** all four tool calls complete successfully
- **AND** the final answer contains the exact canary fact, prescribed action and stable ref derived from MCP evidence

### Scenario S53: Agent acceptance cannot bypass OKF MCP
- **WHEN** S52, S54 or S55 runs
- **THEN** the event stream contains no shell, terminal, direct file-read or direct OKF CLI execution
- **AND** direct `tools/mcp_call.py` or `test_mcp.py` results are not used as Agent-client evidence

### Scenario S54: A real Agent recovers from a structured OKF error
- **WHEN** the Agent first resolves an invalid ref
- **THEN** it observes `invalid_concept_id` and a non-empty remediation
- **AND** it calls `okf_manifest` to obtain a canonical stable ref
- **AND** retrying `okf_resolve` succeeds and the final answer reports the resolved path

### Scenario S55: A real Agent performs controlled durable capture
- **WHEN** the Agent is explicitly asked to persist a note with an idempotency key and read it back
- **THEN** the event stream contains `okf_note` followed by `okf_query`
- **AND** the note is persisted under the repository knowledge directory
- **AND** the final answer contains the queried durable content

### Scenario S56: Client capability status is fail-closed
- **WHEN** a client lacks executable, authentication, MCP approval, model access or a machine validator
- **THEN** its model E2E status is a specific `BLOCKED_*` state
- **AND** it is never aggregated into `PASS`
- **AND** evidence reports configuration discovery separately from model/tool/answer closure

## Requirement: Consistent code metadata filtering and symbol-body context

The system SHALL filter code metadata (file path, language, symbol kind, qualified name, relation endpoints) through the content-aware, case-insensitive, substring semantics of the query package, and SHALL keep post-filtering limited to the dimensions that the query builder cannot express (Type, Types, Project, Tag). Generated `code_file` concepts whose symbol metadata lives in the body SHALL be matched and SHALL return a token-bounded source snippet for the best-matching symbol, and `Status` SHALL report the count of code concepts.

### Scenario S57: Code metadata combination filter returns results
- **GIVEN** a generated `code_file` concept with a `source_path` CustomField and symbol lines in its body
- **WHEN** `okf_query` is called with `file_path`, `symbol_kind`, and `qualified_name`
- **THEN** the concept is returned
- **AND** no concept is rejected solely because symbol metadata lives in the body rather than CustomFields

### Scenario S58: Context returns token-bounded source snippet for matched symbol
- **GIVEN** a `code_file` concept whose body lists a symbol at a known line range
- **WHEN** `okf_context` queries for that symbol name
- **THEN** the returned ContextItem includes the source file path, correct `start_line` and `end_line`
- **AND** the snippet contains the full symbol body (multiple lines), not just the matching line
- **AND** provenance is `repo.source`

### Scenario S59: Status reports code concept count
- **GIVEN** a bundle with `code_file` concepts
- **WHEN** `okf_status` is called
- **THEN** `StatusResult` includes `code_concept_count` > 0
- **AND** a bundle without `code_file` concepts omits the field

### Scenario S60: Code metadata filtering is consistent across Service, CLI and MCP
- **GIVEN** the same `code_file` concept and the same `file_path`/`symbol_kind`/`qualified_name` filters
- **WHEN** `Service.Query`, CLI JSON mode, and MCP `okf_query` are invoked
- **THEN** they return equivalent result sets
- **AND** all three use content-aware substring matching, not exact CustomField equality

### Scenario S61: Non-code concept filtering is unchanged
- **GIVEN** a legacy document concept without symbol lines
- **WHEN** `type`, `tag`, or `project` filters are applied
- **THEN** behavior is identical to before the change
