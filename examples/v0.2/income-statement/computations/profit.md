---
type: Attested Computation
title: Gross profit for fiscal year
description: Gross profit by segment for a fiscal year, per the cost-allocation standard.
tags: [finance, profit]
status: stable
runtime: dbt
parameters:
 - { name: year, type: integer, required: true }
 - { name: segment, type: string, required: true }
executor:
 resource: references/skills/run-dbt.md
 receipt: [run_id, compiled_sql, result]
attester:
 resource: references/attesters/dbt-binding.py
generated: { by: reference_agent/gemini-2.5-pro, at: 2026-06-14T14:00:00Z }
verified: { by: process:finance-nightly, at: 2026-06-12T08:00:00Z }
stale_after: 2026-06-15
sources:
 - id: cost-alloc
   resource: https://wiki.acme.com/finance/cost-allocation
   title: Cost allocation standard
---

# Computation

```sql
 SELECT gross_profit
 FROM {{ ref('fct_income_statement') }}
 WHERE fiscal_year = {{ var('year') }}
 AND segment = {{ var('segment') }}
```

Gross profit by segment per the cost-allocation standard.[^cost-alloc]

[^cost-alloc]: Cost allocation standard
