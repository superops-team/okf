#!/usr/bin/env bash
# tools/eval.sh — one-command IR evaluation benchmark for okf search quality.
#
# Runs the golden query benchmark (20 cases over 7 document formats) and
# prints Recall@K / Precision@K / MRR / NDCG@K baseline scores.
#
# Usage:
#   tools/eval.sh
#
# Exit code: 0 if all benchmark assertions pass, non-zero otherwise.
set -euo pipefail

cd "$(dirname "$0")/.."
GO=${GO:-go}

echo "=== okf IR Evaluation Benchmark ==="
echo "K=5, 20 golden queries (18 positive, 2 negative), 7 document formats"
echo

"$GO" test ./pkg/eval/ -run TestEvalBenchmark -v 2>&1
