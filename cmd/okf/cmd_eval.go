package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/superops-team/okf/pkg/embeddings"
	"github.com/superops-team/okf/pkg/eval"
	"github.com/superops-team/okf/pkg/query"
	"github.com/superops-team/okf/pkg/vectorindex"
)

// cmdEval 对知识库运行 IR 评测基准，可对比多种检索策略。
//
// 存在意义：改造检索质量时必须有可复现的量化依据，否则只能凭感觉调参。
// 该命令让 golden set 评测从"只能在测试里跑"变成用户可直接执行。
func cmdEval(args []string) int {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	path := fs.String("path", "", "Knowledge base path (default: current directory)")
	golden := fs.String("golden", "", "Path to golden query set JSON (required)")
	k := fs.Int("k", 0, "Cut-off K for Recall@K/Precision@K/NDCG@K (default: value from golden set, else 5)")
	compare := fs.Bool("compare", false, "Compare retrieval strategies (lexical / semantic / hybrid) instead of running one")
	verbose := fs.Bool("verbose", false, "Print per-case results")
	fs.Parse(args)

	if *golden == "" {
		fmt.Println("Error: -golden is required (path to golden query set JSON)")
		fmt.Println("Example: okf eval -golden pkg/eval/testdata/golden_semantic.json -path docs/knowledge -compare")
		return 1
	}
	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	cases, err := eval.LoadGoldenCases(*golden)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}
	cutoff := *k
	if cutoff <= 0 {
		if set, err := loadGoldenSetK(*golden); err == nil && set > 0 {
			cutoff = set
		} else {
			cutoff = 5
		}
	}

	qb, _, err := loadKnowledgeBundle(*path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}
	if len(qb.Concepts) == 0 {
		fmt.Printf("Error: no concepts found under %s\n", *path)
		return 1
	}

	// golden set 与知识库不匹配时所有指标恒为 0，容易被误读为"检索完全失效"。
	// 这里显式提示，避免用户对着假零分调参。
	if missing := missingExpectedDocs(qb, cases); len(missing) > 0 {
		fmt.Printf("Warning: golden set 中有 %d 个期望文档不在知识库 %s 中（指标会偏低或为 0）\n",
			len(missing), *path)
		show := missing
		if len(show) > 5 {
			show = show[:5]
		}
		fmt.Printf("         例如: %v\n", show)
		fmt.Printf("         请确认 -path 指向该 golden set 对应的知识库。\n\n")
	}

	if !*compare {
		report := eval.RunBenchmark(qb, cases, cutoff)
		printEvalReport(report, *verbose)
		return 0
	}
	return runEvalComparison(qb, cases, cutoff, *path, *verbose)
}

// runEvalComparison 对比 词法 / 纯语义 / 混合 三类策略。
// 语义类策略需要向量索引；索引不可用时降级为仅词法对比并明确说明。
func runEvalComparison(qb *query.KnowledgeBundle, cases []eval.EvalCase, cutoff int, path string, verbose bool) int {
	strategies := map[string]eval.SearchStrategy{
		"lexical-substring": eval.DefaultStrategy,
	}

	emb, err := embeddings.NewMiniLM()
	if err != nil {
		fmt.Printf("Warning: 向量模型不可用 (%v)，仅评测词法策略\n\n", err)
	} else {
		defer emb.Close()
		idx := vectorindex.NewHNSW(emb.Dimension())
		if _, lerr := idx.Load(vectorIndexDir(path)); lerr != nil {
			fmt.Printf("Warning: 向量索引不可用 (%v)，仅评测词法策略。请先执行 okf vector index\n\n", lerr)
		} else {
			sem := &semanticBackend{emb: emb, idx: idx}
			lex := buildLexicalBackend(qb)
			strategies["bm25-only"] = makeEvalStrategy(nil, lex, cutoff, 0, 1.0)
			strategies["semantic-only"] = makeEvalStrategy(sem, nil, cutoff, 1.0, 0)
			strategies["hybrid-default"] = makeEvalStrategy(sem, lex, cutoff,
				query.DefaultVectorWeight, query.DefaultLexicalWeight)
		}
	}

	reports := eval.CompareStrategies(qb, cases, cutoff, strategies)
	fmt.Printf("=== Strategy comparison (K=%d, %d cases: %d positive, %d negative) ===\n",
		cutoff, len(cases), reports["lexical-substring"].PositiveCount,
		reports["lexical-substring"].NegativeCount)
	fmt.Println("(means over positive cases only; negative cases are excluded so that")
	fmt.Println(" a strategy is not rewarded for returning nothing)")
	fmt.Println()
	fmt.Print(eval.FormatComparison(reports))

	if verbose {
		for _, name := range sortedKeys(reports) {
			fmt.Printf("\n--- %s ---\n", name)
			printEvalReport(reports[name], true)
		}
	}
	return 0
}

// makeEvalStrategy 构造一个注入指定通道与权重的评测策略。
func makeEvalStrategy(sem query.SemanticBackend, lex query.LexicalBackend, topK int, wv, wl float64) eval.SearchStrategy {
	return func(bundle *query.KnowledgeBundle, q string) []*query.Concept {
		opts := query.SearchOptions{TopK: topK, VectorWeight: wv, Lexical: lex}
		opts = opts.WithLexicalWeight(wl)
		res, err := query.SemanticSearch(bundle, q, sem, opts)
		if err != nil {
			return nil
		}
		out := make([]*query.Concept, 0, len(res))
		for _, r := range res {
			out = append(out, r.Concept)
		}
		return out
	}
}

// missingExpectedDocs 返回 golden set 期望但知识库中不存在的文档标识（去重、保持出现顺序）。
func missingExpectedDocs(qb *query.KnowledgeBundle, cases []eval.EvalCase) []string {
	have := make(map[string]struct{}, len(qb.Concepts))
	for _, c := range qb.Concepts {
		have[c.FilePath] = struct{}{}
	}
	seen := make(map[string]struct{})
	var missing []string
	for _, c := range cases {
		for _, d := range c.ExpectedDocs {
			if _, ok := have[d]; ok {
				continue
			}
			if _, dup := seen[d]; dup {
				continue
			}
			seen[d] = struct{}{}
			missing = append(missing, d)
		}
	}
	return missing
}

func printEvalReport(r *eval.EvalReport, verbose bool) {
	if verbose {
		fmt.Print(r.String())
		return
	}
	a := r.AggregateNonNegative
	fmt.Printf("=== IR Eval (K=%d, %d cases: %d positive, %d negative) ===\n",
		r.K, len(r.Cases), r.PositiveCount, r.NegativeCount)
	fmt.Printf("  Recall@%d:    %.4f\n", r.K, a.Recall)
	fmt.Printf("  Precision@%d: %.4f\n", r.K, a.Precision)
	fmt.Printf("  MRR:         %.4f\n", a.MRR)
	fmt.Printf("  NDCG@%d:      %.4f\n", r.K, a.NDCG)
	fmt.Println("  (means over positive cases)")
}

// loadGoldenSetK 读取 golden set 声明的 K 值。
func loadGoldenSetK(path string) (int, error) {
	set, err := eval.LoadGoldenSet(filepath.Clean(path))
	if err != nil {
		return 0, err
	}
	return set.K, nil
}

func sortedKeys(m map[string]*eval.EvalReport) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
