# Run dbt

Executor skill for running dbt models and returning a receipt.

## Input
- `model`: dbt model name
- `vars`: dbt variables

## Output (receipt)
- `run_id`: dbt run ID
- `compiled_sql`: The compiled SQL after template rendering
- `result`: Query result rows
