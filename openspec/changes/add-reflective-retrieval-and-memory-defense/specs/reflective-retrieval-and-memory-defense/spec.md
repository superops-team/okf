# Specification: Reflective Retrieval and Memory Defense

## Requirement: Memory Defense write-time scanning

OKF SHALL scan durable write content (`WriteKnowledgeRequest.Content`, and feedback `principle` mapped to content) with a fixed-order regex catalog before persisting bytes. Scanning SHALL run after `normalizeWriteKnowledgeRequest` and before `hashKnowledgePayload`. The existing `hasCredentialField` metadata-field gate SHALL remain in place and SHALL NOT be replaced.

### Scenario MD-01: Default disabled preserves backward compatibility
- **GIVEN** a repo with no `.okf/config.yaml` `memory_defense` section
- **WHEN** `WriteKnowledge` is called with content containing a known secret
- **THEN** the write succeeds unchanged (defense disabled by default)
- **AND** payload serialization is byte-for-byte identical to pre-change behavior

### Scenario MD-02: Enabled redact replaces known secrets in-place
- **GIVEN** `memory_defense.enabled=true, action=redact`
- **WHEN** content contains `ghp_` followed by 36 alphanumerics (GitHub PAT shape)
- **THEN** the persisted content has the token replaced with `[REDACTED:github_pat]`
- **AND** the original token string does NOT appear anywhere in the persisted file
- **AND** the success envelope carries a `redactions` list naming `github_pat`

### Scenario MD-03: Block action prevents any byte from hitting disk
- **GIVEN** `memory_defense.action=block`
- **WHEN** content matches a high-severity detector
- **THEN** the write fails with `ErrMemoryDefenseBlocked`
- **AND** no temporary file or concept file is created on disk
- **AND** the error message names only the detector ID, never the matched secret text

### Scenario MD-04: Redact-then-empty rejects the write
- **GIVEN** `action=redact` and content that is entirely a secret (e.g. just `sk-...`)
- **WHEN** redaction produces an empty or whitespace-only string
- **THEN** the write fails with `ErrRedactionEmpty`
- **AND** no file is persisted

### Scenario MD-05: Catalog has at least 16 detectors
- **GIVEN** the built-in detector catalog
- **WHEN** counted
- **THEN** the count is ≥ 16
- **AND** every detector has a non-empty ID, label, severity, and compiles its regex

### Scenario MD-06: Every detector has a positive hit and a normal-text non-hit
- **GIVEN** a table-driven test over all catalog detectors
- **WHEN** each detector's positive sample is scanned
- **THEN** it matches (redact replaces / block blocks)
- **AND** when a corpus of normal English/Chinese prose is scanned
- **THEN** zero detectors fire (no false positives)

### Scenario MD-07: Credit-card detector requires Luhn check
- **GIVEN** content containing a 16-digit number that fails Luhn (e.g. a byte count like `1234567890123456`)
- **WHEN** scanned
- **THEN** the credit-card detector does NOT fire
- **AND** when content contains a Luhn-valid test number (e.g. `4111-1111-1111-1111`)
- **THEN** the credit-card detector fires

### Scenario MD-08: Redaction is deterministic across retries
- **GIVEN** `action=redact` and a write with idempotency_key `K1` and content containing a secret
- **WHEN** the write succeeds, then the same request with the same `K1` is retried
- **THEN** the retry returns `created=false` and reuses the existing concept
- **AND** the payload_hash matches (redacted content hashed identically both times)

### Scenario MD-09: Block does not leak secrets in errors or logs
- **GIVEN** `action=block` and content containing a real AWS key
- **WHEN** the write is blocked
- **THEN** the returned envelope error contains no substring of the original key
- **AND** remediation names only the detector ID (e.g. `aws_access_key`)

### Scenario MD-10: Config missing or malformed
- **GIVEN** no `.okf/config.yaml` at repo root
- **WHEN** LoadPolicy runs
- **THEN** it returns `Enabled=false` with no error
- **AND** when `action: bogus` is present
- **THEN** LoadPolicy returns a stable error naming the invalid action

### Scenario MD-11: Detector whitelist
- **GIVEN** `detectors: [github_pat]`
- **WHEN** content contains an AWS key but no GitHub PAT
- **THEN** no detector fires (whitelist active)
- **AND** when content contains a GitHub PAT
- **THEN** it fires

## Requirement: Relation recall from an anchor

OKF SHALL provide a read-only relation recall that, given an anchor stable okf_id, returns the anchor (if approved and healthy), all approved `extends` neighbors in both directions, and the approved members of the anchor's `updates` chain up to the current head.

### Scenario RR-01: Bidirectional extends neighbors
- **GIVEN** approved A, B extends A (B targets A), and C extends A (C targets A)
- **WHEN** RelationRecall(anchor=A) runs
- **THEN** B and C are both returned with edge `extends`
- **AND** when RelationRecall(anchor=B) runs
- **THEN** A is returned (incoming extends direction)

### Scenario RR-02: Update chain walks to current head
- **GIVEN** approved A, B updates A, C updates B
- **WHEN** RelationRecall(anchor=A) runs
- **THEN** A, B, C are all returned
- **AND** C is marked `is_chain_head=true`
- **AND** A and B are not marked head

### Scenario RR-03: Proposed/declined never appear in default results
- **GIVEN** proposed P that extends approved A, and declined D that updates A
- **WHEN** RelationRecall(anchor=A) runs
- **THEN** P and D are excluded from hits
- **AND** no proposed/declined okf_id appears in the result set

### Scenario RR-04: Dangling/cyclic/fork anchors degrade gracefully
- **GIVEN** anchor in a cyclic or ambiguous component
- **WHEN** RelationRecall runs
- **THEN** extends neighbors that are healthy are still returned
- **AND** a warning names the topology error (cycle/ambiguous/dangling)
- **AND** the call does not panic

### Scenario RR-05: Depth bounded to one hop
- **GIVEN** A extends B, B extends C
- **WHEN** RelationRecall(anchor=A, depth=1) runs
- **THEN** B is returned but C is not (no transitive extends recursion)

### Scenario RR-06: Unknown anchor returns not_found
- **GIVEN** an okf_id not present in the bundle
- **WHEN** RelationRecall runs
- **THEN** it returns `ErrMemoryRefNotFound`

## Requirement: Reflective multi-round retrieval

OKF SHALL provide a read-only `reflect` operation that runs bounded multi-round retrieval: round 1 lexical/semantic, poison gate, round 2 relation expansion, RRF fusion, and abstention when evidence is thin. The operation SHALL be in-process, synchronous, deterministic, and SHALL NOT call an LLM.

### Scenario RF-01: Two rounds gather more evidence than one
- **GIVEN** a question whose lexical match finds A, and A extends B (B does not lexically match)
- **WHEN** reflect runs with max_rounds=2
- **THEN** the trace shows round 1 hits A and round 2 adds B
- **AND** final evidence count > round-1-only count

### Scenario RF-02: RRF ranks overlap candidates first
- **GIVEN** a document appears in both round-1 and round-2 ranked lists
- **WHEN** RRF(k=60) fuses the two rankings
- **THEN** the overlap document ranks above documents appearing in only one list

### Scenario RF-03: Thin evidence triggers need_clarify
- **GIVEN** a question with fewer than min_evidence matching concepts in the bundle
- **WHEN** reflect runs
- **THEN** the response has `need_clarify=true`
- **AND** no fabricated evidence is returned
- **AND** a clarifying-question suggestion is included

### Scenario RF-04: Poison candidates are dropped
- **GIVEN** a proposed poison concept P that lexically matches the question
- **WHEN** reflect runs
- **THEN** P does not appear in the final evidence list
- **AND** `trap_blocked` is true or the count of dropped poison candidates > 0

### Scenario RF-05: max_rounds is hard-capped
- **GIVEN** caller passes max_rounds=100
- **WHEN** reflect runs
- **THEN** it uses at most 3 rounds
- **AND** it does not loop indefinitely

### Scenario RF-06: Reflect is read-only
- **GIVEN** any reflect call
- **WHEN** it completes
- **THEN** no concept file is created, modified, or deleted
- **AND** the envelope `Mutating=false`

## Requirement: Trap evaluation

OKF SHALL provide a local deterministic trap evaluation that scores golden cases at answer, evidence, and abstention layers, and exits non-zero if poison is not fully blocked.

### Scenario TE-01: Answer-level graded hit
- **GIVEN** a golden case with must_contain=["prefer", "evening"] and an answer containing both
- **WHEN** scored
- **THEN** answer_score is the proportion of must_contain facts found

### Scenario TE-02: Forbidden evidence scores zero
- **GIVEN** a golden case whose answer cites forbidden_evidence (a poison okf_id)
- **WHEN** scored
- **THEN** evidence_score is 0.0
- **AND** the case is recorded as trap_leak

### Scenario TE-03: Correct abstention is rewarded
- **GIVEN** an abstain_ok=true case with no supporting evidence in the bundle
- **WHEN** the system returns need_clarify
- **THEN** abstention_score is 1.0
- **AND** when the system force-answers instead
- **THEN** abstention_score is 0.0

### Scenario TE-04: Case types are grouped
- **GIVEN** golden cases spanning single-hop, multi-hop, temporal, knowledge-update, abstain
- **WHEN** the report is generated
- **THEN** metrics are aggregated per case_type (not only an overall mean)

### Scenario TE-05: Poison block gate exits non-zero
- **GIVEN** a trap bundle with an embedded proposed poison memory
- **WHEN** `okf eval trap` runs
- **THEN** poison_blocked rate is 1.0 for the bundled fixture
- **AND** if poison_blocked < 1.0, the process exits non-zero

## Requirement: Wiring and parity

OKF SHALL wire all four modules through Service, CLI, and modern + legacy MCP with parity tests.

### Scenario WI-01: CLI reflect and relation exist
- **WHEN** `okf tool reflect -q "..."` and `okf tool relation --anchor okf_...` run
- **THEN** they produce valid envelopes without error
- **AND** `okf eval trap -golden ...` runs and prints the four means

### Scenario WI-02: MCP parity
- **WHEN** the MCP server lists tools
- **THEN** `okf_reflect` and `okf_relation_recall` are registered in both modern and legacy eras
- **AND** the dual-era parity test confirms both eras return equivalent results

### Scenario WI-03: Write tools route through defense
- **WHEN** `okf_note` / `okf_log` / `okf_feedback` MCP tools write content
- **THEN** the same Memory Defense screen applies as `WriteKnowledge`
- **AND** a blocked write returns the same error code over MCP as over CLI
