# Run on BigQuery

Executor skill for running BigQuery SQL and returning a receipt.

## Input
- `sql`: The SQL query to execute
- `params`: Bind parameters

## Output (receipt)
- `job_id`: BigQuery job ID
- `executed_sql`: The actual SQL that was executed (after parameter binding)
- `result`: Query result rows
