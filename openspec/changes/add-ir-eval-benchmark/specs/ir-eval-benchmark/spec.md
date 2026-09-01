# Specification: IR Evaluation Benchmark

## Requirement: IR Metric Functions

The system SHALL provide pure functions for the four canonical information-retrieval metrics, operating on ordered result lists and expected relevant document sets.

### Scenario: Precision@K with fewer results than K

- **GIVEN** a result list of 2 documents, both relevant, and K=5
- **WHEN** `PrecisionAtK(results, expected, 5)` is called
- **THEN** the returned value is 1.0 (denominator is actual result count 2, not K)

### Scenario: Precision@K with mixed relevance

- **GIVEN** results `[a, b, c, d]` where `a` and `c` are relevant, and K=3
- **WHEN** `PrecisionAtK(results, expected, 3)` is called
- **THEN** the returned value is 2/3 (a and c in top 3)

### Scenario: Recall@K with partial recall

- **GIVEN** expected set `{a, b, c, d}` and results `[a, x, b, y]`, K=4
- **WHEN** `RecallAtK(results, expected, 4)` is called
- **THEN** the returned value is 0.5 (2 of 4 expected docs recalled)

### Scenario: Recall@K with empty expected set

- **GIVEN** expected set is empty and results `[a, b]`
- **WHEN** `RecallAtK(results, expected, 5)` is called
- **THEN** the returned value is 1.0 (no relevant docs to miss)

### Scenario: MRR first relevant at rank 3

- **GIVEN** results `[x, y, a, b]` where `a` is the first relevant doc
- **WHEN** `MRR(results, expected)` is called
- **THEN** the returned value is 1/3

### Scenario: MRR with no relevant result

- **GIVEN** results `[x, y, z]` and none are in expected set
- **WHEN** `MRR(results, expected)` is called
- **THEN** the returned value is 0.0

### Scenario: NDCG@K perfect ranking

- **GIVEN** expected `[a, b, c]` (relevance order) and results `[a, b, c]`, K=3
- **WHEN** `NDCG(results, expected, 3)` is called
- **THEN** the returned value is 1.0

### Scenario: NDCG@K with irrelevant doc pushing relevant down

- **GIVEN** expected `[a, b, c]` and results `[x, a, b]`, K=2
- **WHEN** `NDCG(results, expected, 2)` is called
- **THEN** the returned value is strictly between 0 and 1 (irrelevant `x` at rank 1 reduces DCG vs the ideal ranking)

### Scenario: NDCG@K all-relevant reorder is 1.0 (binary relevance)

- **GIVEN** expected `[a, b, c]` and results `[c, b, a]`, K=3
- **WHEN** `NDCG(results, expected, 3)` is called
- **THEN** the returned value is 1.0 (binary relevance: every relevant doc has gain 1, so order among relevant docs does not change DCG)

### Scenario: NDCG@K with empty expected set

- **GIVEN** expected set is empty and results `[a, b]`
- **WHEN** `NDCG(results, expected, 5)` is called
- **THEN** the returned value is 1.0

## Requirement: Golden Query Benchmark Set

The system SHALL provide a committed golden query set covering the 7 supported document formats, with expected relevant documents in descending relevance order.

### Scenario: Golden set covers all 7 formats

- **GIVEN** the golden set file `pkg/eval/testdata/golden_queries.json`
- **WHEN** the set is loaded
- **THEN** it contains at least one positive query for each of: pdf, docx, xlsx, pptx, html, csv, txt

### Scenario: Golden set includes negative queries

- **GIVEN** the golden set
- **WHEN** inspected
- **THEN** it contains at least one case with `expected_docs: []` (zero results expected)

### Scenario: Golden set includes multi-hit queries

- **GIVEN** the golden set
- **WHEN** inspected
- **THEN** it contains at least one case with 2+ expected docs

### Scenario: Golden set includes table and list content queries

- **GIVEN** the golden set
- **WHEN** inspected
- **THEN** it contains queries targeting table cell content (xlsx/csv/docx tables) and list item content (html)

## Requirement: Benchmark Runner

The system SHALL run the golden benchmark against a real okf knowledge bundle built from the 7 document fixtures, score every case, and produce an aggregate report.

### Scenario: Benchmark produces aggregate scores

- **GIVEN** a knowledge bundle containing all 7 converted documents
- **WHEN** `RunBenchmark(bundle, cases, 5)` is called
- **THEN** the returned `EvalReport` contains mean Recall@5, Precision@5, MRR, and NDCG@5 over all cases

### Scenario: Positive queries recall at least one expected doc

- **GIVEN** the benchmark report
- **WHEN** positive cases (non-empty expected) are examined
- **THEN** every positive case has Recall@5 ≥ 0.5

### Scenario: Negative queries return zero results

- **GIVEN** the benchmark report
- **WHEN** negative cases (empty expected) are examined
- **THEN** every negative case has 0 search results (MRR=0, Recall=1.0 by convention)

### Scenario: Report includes per-case detail

- **GIVEN** the benchmark report
- **WHEN** rendered
- **THEN** each case shows query, Recall@5, Precision@5, MRR, NDCG@5, and the top-1 result document

## Requirement: Reproducible Eval Command

The system SHALL provide a one-command entrypoint that runs the full benchmark and prints the report, suitable for CI and local use.

### Scenario: eval.sh exits zero and prints report

- **GIVEN** a clean checkout
- **WHEN** `tools/eval.sh` is executed
- **THEN** it exits 0 and prints a report containing all four metric names and their aggregate scores

### Scenario: eval.sh is deterministic

- **GIVEN** the same checkout
- **WHEN** `tools/eval.sh` is executed twice
- **THEN** the printed scores are identical (no randomness, no network)
