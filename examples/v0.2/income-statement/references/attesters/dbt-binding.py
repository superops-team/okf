"""dbt binding attester for dbt Attested Computations.

Compares the compiled_sql from a receipt against the sanctioned dbt model
rendered with the claimed variables. Returns PASS if they match, FAIL otherwise.
"""

def attest(receipt, sanctioned_model, variables):
    expected = render_template(sanctioned_model, variables)
    if normalize_sql(receipt["compiled_sql"]) == normalize_sql(expected):
        return {"verdict": "PASS", "run_id": receipt["run_id"]}
    return {"verdict": "FAIL", "reason": "compiled SQL does not match sanctioned model"}

def render_template(template, variables):
    import re
    for name, value in variables.items():
        template = re.sub(r"\{\{\s*var\('" + name + r"'\)\s*\}\}", str(value), template)
    return template

def normalize_sql(sql):
    return " ".join(sql.split()).lower()
