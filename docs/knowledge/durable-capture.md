---
type: documentation
title: Durable Knowledge Capture over MCP
description: Secure, idempotent persistence and retrieval of explicit note, event, and feedback concepts through OKF MCP.
tags: [okf, mcp, knowledge, persistence, agents]
status: stable
---

# Durable knowledge capture over MCP

OKF exposes durable repository knowledge to any MCP client without depending on host-specific runtime types or event buses.

## Ownership boundary

- `okf_init` and `okf_refresh` index ordinary repository Markdown, including documentation and Wiki-style files.
- `okf_note`, `okf_log`, and `okf_feedback` persist only knowledge explicitly submitted by the caller.
- `okf_ask` queries durable note/event/feedback concepts through the shared service query path.
- Session memory, conversational state, and private host events remain owned by the calling application.

## Isolated setup

```bash
okf mcp --repo /path/to/repository --dir /path/to/isolated/okf-knowledge
```

Absolute `--dir` values remain absolute; relative values resolve under `--repo`.

## Stable identities and retries

Every write requires a stable `idempotency_key` derived from a durable source identity rather than execution time. The concept identity is derived from the canonical repository root, concept kind, and idempotency key.

- Same key and normalized payload returns the existing concept without duplication.
- Same key and a different payload returns `idempotency_conflict` without overwriting the original.
- Concurrent requests with the same key converge on one committed concept.

Example:

```json
{
  "content": "Prefer structured facts over raw command strings.",
  "project": "sample-project",
  "tags": ["runtime", "verification"],
  "metadata": {
    "source_system": "agent-review",
    "source_id": "finding-123",
    "source_content_sha256": "<sha256>"
  },
  "idempotency_key": "review-finding:finding-123"
}
```

Do not place secrets, tokens, passwords, private keys, or credentials in content or metadata. Credential-like metadata keys are rejected.

## Verification checklist

1. Call `okf_init` for the target repository and isolated knowledge directory.
2. Write representative note, event, and feedback records with stable keys.
3. Repeat an identical write and confirm identity reuse without duplicates.
4. Repeat the key with a changed payload and confirm `idempotency_conflict`.
5. Restart the MCP server and confirm `okf_ask` returns the committed content.
6. Verify project filters, deterministic ordering, result limits, and context token budgets.
7. Confirm malformed fields, oversize input, symlink roots, and credential-like metadata fail closed.
