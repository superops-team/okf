# Tasks: IR Evaluation Benchmark

P0-P4 indicate dependency order only — all must be completed.

## P0: IR Metric Functions (pure functions + unit tests)

- [ ] P0-1: Create `pkg/eval/metrics.go` with `PrecisionAtK`, `RecallAtK`, `MRR`, `NDCG` (all `[]string` based, pure functions)
- [ ] P0-2: Create `pkg/eval/metrics_test.go` covering every spec Scenario under "IR Metric Functions": empty results, all relevant, none relevant, K>len, mixed relevance, MRR rank cases, NDCG perfect/reversed/empty
- [ ] P0-3: RED→GREEN: write tests first, confirm they fail, then implement minimal functions
- [ ] P0-4: Add property test (testing/quick): for any result list and expected set, 0 ≤ Precision ≤ 1, 0 ≤ Recall ≤ 1, 0 ≤ MRR ≤ 1, 0 ≤ NDCG ≤ 1

## P0: Golden Query Set

- [ ] P0-5: Create `pkg/eval/testdata/golden_queries.json` with 20 cases: 7 single-hit (one per format), 5 multi-hit, 2 negative, 3 table-content, 2 list-content, 1 title-match
- [ ] P0-6: Create `pkg/eval/benchmark.go` with `EvalCase` struct, `LoadGoldenCases(path)`, `EvalReport`, `RunBenchmark(bundle, cases, k)`
- [ ] P0-7: `RunBenchmark` maps `query.Search` results to `Concept.Resource`, scores each case, aggregates means

## P1: End-to-End Benchmark Test

- [ ] P1-1: Create `pkg/eval/benchmark_test.go`: build temp KB from 7 fixtures (reuse `pkg/convert/testdata/`), import via `okf add`, load bundle, run `RunBenchmark`
- [ ] P1-2: Assert: all positive cases Recall@5 ≥ 0.5; all negative cases 0 results; aggregate MRR > 0.7; report well-formed
- [ ] P1-3: Print full report (per-case + aggregate) via `t.Log` for baseline documentation

## P1: Reproducible Eval Command

- [ ] P1-4: Create `tools/eval.sh` (executable, 100755): runs `go test ./pkg/eval -run TestEvalBenchmark -v`, exits with test result
- [ ] P1-5: Verify `tools/eval.sh` runs clean and prints report; run twice to confirm determinism

## P2: Documentation

- [ ] P2-1: Update `README.md`: add "Evaluation" section with `tools/eval.sh` usage and metric definitions
- [ ] P2-2: Update `docs/knowledge/releases.md`: v1.3.0 entry with baseline scores (actual numbers from P1-3 run)
- [ ] P2-3: Write `conformance.md`: map every spec Scenario to test function, status fully/aligned/partial/gap

## P3: Full Regression + Gauntlet

- [ ] P3-1: `go build ./...` + `go vet ./...` + `gofmt -l` + `staticcheck ./...`
- [ ] P3-2: `go test ./... -race` + `go test -shuffle=on ./...`
- [ ] P3-3: `tools/gauntlet.sh` full pass (coverage ≥60%, mutation 4/4, real execution)
- [ ] P3-4: `python3 test_mcp.py` (MCP E2E unaffected)

## P4: Delivery

- [ ] P4-1: Commit all changes on branch `feat/ir-eval-benchmark`
- [ ] P4-2: Push and create PR
- [ ] P4-3: Confirm CI green
