# Release Notes: Reflective Retrieval and Memory Defense

## New features

### Memory Defense (write-time scanning)
- 16-detector regex catalog covering GitHub PAT/OAuth, AWS, OpenAI, Anthropic, JWT, PEM, Postgres/MySQL DSN, Slack, Stripe, credit card (Luhn-verified), SSN, Google API key, generic long token.
- Two actions: `redact` (replace with `[REDACTED:type]`) or `block` (zero bytes on disk).
- Config: `.okf/config.yaml` → `memory_defense.enabled/action/detectors`. Default disabled (backward compatible).
- Redaction runs before payload hashing for idempotency retry correctness.
- Error messages never leak the original secret text.

### Relation Recall
- Bidirectional extends neighbors (both outgoing and incoming directions).
- Updates chain walk to current head, with `is_chain_head` annotation.
- Proposed/declined/invalid concepts excluded from results.
- Depth=1 (no transitive extends recursion).

### Reflect Workflow
- Bounded 2-round retrieval: round 1 lexical/semantic, round 2 relation expansion.
- RRF(k=60) fusion with deterministic tiebreak (by okf_id).
- Poison gate drops non-approved concepts before evidence.
- `need_clarify` abstention when evidence < min_evidence (default 2).
- Max rounds hard-capped at 3.

### Trap Evaluation
- 3-layer scoring: answer (must_contain/must_not_contain), evidence (expected/forbidden refs), abstention.
- 5 case types: single-hop, multi-hop, temporal, knowledge-update, abstain.
- `okf eval trap -golden <file>` CLI with 4-means output; exits non-zero if poison_blocked < 1.0.

### CLI / MCP
- `okf tool reflect -q "..." [--min-evidence N] [--max-rounds N]`
- `okf tool relation --anchor okf_...`
- `okf eval trap -golden <file>`
- MCP: `okf_reflect`, `okf_relation_recall` registered.

## Backward compatibility
- No config = defense disabled; existing writes behave byte-for-byte identically.
- New MCP tools do not affect existing tool schemas.
- `WriteKnowledgeResult` adds `redactions` (omitempty).

## Known limitations
- Chinese PII (phone/ID card) not covered by design.
- Document import path (PDF/DOCX via pkg/convert) does not go through Memory Defense.
- Trap eval CLI currently scores deterministically; live reflect-in-the-loop scoring is future work.
- 10k concept latency not benchmarked.
