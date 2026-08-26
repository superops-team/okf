---
type: Attested Computation
title: Revenue for fiscal year
description: Recognized revenue for a fiscal year, per Finance's definition.
tags: [finance, revenue]
status: stable
runtime: bigquery
parameters:
 - { name: year, type: integer, required: true }
executor:
 resource: references/skills/run-on-bq.md
 receipt: [job_id, executed_sql, result]
attester:
 resource: references/attesters/sql-equality.py
generated: { by: reference_agent/gemini-2.5-pro, at: 2026-06-28T14:00:00Z }
verified: { by: human:ahormati, at: 2026-06-25T09:00:00Z }
stale_after: 2026-12-31
sources:
 - id: rev-policy
   resource: https://wiki.acme.com/finance/revenue-recognition
   title: Revenue recognition policy
   author: team:finance-fpa
   last_modified: 2026-04-02
 - id: exec-rev-dash
   resource: dashboards/exec-revenue
   title: Executive revenue dashboard
   author: team:finance-fpa
   usage_count: 5000
   last_modified: 2026-06-18
usage_window: { from: 2026-06-01, to: 2026-06-30 }
---

# Computation

```sql
 SELECT SUM(amount) AS revenue
 FROM finance.recognized_revenue
 WHERE fiscal_year = @year
```

Recognized revenue per the recognition policy,[^rev-policy] corroborated by
the executive revenue dashboard.[^exec-rev-dash]

[^rev-policy]: Revenue recognition policy
[^exec-rev-dash]: Executive revenue dashboard
