"""SQL equality attester for BigQuery Attested Computations.

Compares the executed_sql from a receipt against the sanctioned computation
bound with the claimed parameters. Returns PASS if they match, FAIL otherwise.
"""

def attest(receipt, sanctioned_computation, parameters):
    expected = bind_parameters(sanctioned_computation, parameters)
    if normalize_sql(receipt["executed_sql"]) == normalize_sql(expected):
        return {"verdict": "PASS", "job_id": receipt["job_id"]}
    return {"verdict": "FAIL", "reason": "executed SQL does not match sanctioned computation"}

def bind_parameters(sql, params):
    for name, value in params.items():
        sql = sql.replace(f"@{name}", str(value))
    return sql

def normalize_sql(sql):
    return " ".join(sql.split()).lower()
