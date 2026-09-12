package eval

import (
	"path/filepath"
	"testing"

	"github.com/superops-team/okf/pkg/embeddings"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/query"
	"github.com/superops-team/okf/pkg/vectorindex"
)

// TestHybridBaselineGate is the machine-enforced non-regression gate for S46.
//
// It loads the real docs/knowledge bundle and the semantic golden set
// (golden_semantic.json), builds a hybrid (semantic + BM25) strategy with the
// same weights as the CLI default, and asserts that Recall@5 and MRR stay at or
// above the reproducible committed baseline.
//
// Historical note: releases.md once recorded hybrid Recall@5=0.9615,
// MRR=0.7256, but those figures are not reproducible from the current tree
// (measured at an earlier point with different knowledge-base content). The
// reproducible baseline on the current tree is Recall@5≈0.92, MRR≈0.66. This
// test enforces conservative thresholds (Recall@5≥0.90, MRR≥0.60) so that a
// real retrieval regression is caught while normal content drift is not.
//
// See openspec/changes/add-agent-knowledge-discovery/spec-amendment.md.
func TestHybridBaselineGate(t *testing.T) {
	skipPuregoUnderRace(t)

	// Locate docs/knowledge relative to the repo root (test runs in pkg/eval/).
	repoRoot := filepath.Join("..", "..")
	kbPath := filepath.Join(repoRoot, "docs", "knowledge")
	goldenPath := filepath.Join("testdata", "golden_semantic.json")

	cases, err := LoadGoldenCases(goldenPath)
	if err != nil {
		t.Fatalf("load golden_semantic.json: %v", err)
	}

	// Load the knowledge bundle using the same loader as the CLI.
	okfBundle, err := okf.LoadBundle(kbPath, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatalf("load docs/knowledge bundle: %v", err)
	}
	qb := query.BundleFromOKF(okfBundle)
	if len(qb.Concepts) == 0 {
		t.Fatal("docs/knowledge bundle is empty")
	}

	emb, err := embeddings.NewMiniLM()
	if err != nil {
		t.Skipf("embedding model unavailable, skipping hybrid baseline gate: %v", err)
	}
	defer emb.Close()

	// Build in-memory HNSW index with v3 ConceptKey (matches production format).
	idx := vectorindex.NewHNSW(emb.Dimension())
	for _, c := range qb.Concepts {
		vec, verr := emb.EmbedQuery(c.Content)
		if verr != nil {
			t.Fatalf("embed %s: %v", c.FilePath, verr)
		}
		idx.Add(query.ConceptKey(c), vec)
	}
	sem := &evalBackend{emb: emb, idx: idx}
	lex := query.BuildLexicalBackend(qb)

	hybrid := func(bundle *query.KnowledgeBundle, q string) []*query.Concept {
		opts := query.SearchOptions{TopK: 5, Lexical: lex}.
			WithVectorWeight(query.DefaultVectorWeight).
			WithLexicalWeight(query.DefaultLexicalWeight)
		res, serr := query.SemanticSearch(bundle, q, sem, opts)
		if serr != nil {
			return nil
		}
		out := make([]*query.Concept, 0, len(res))
		for _, r := range res {
			out = append(out, r.Concept)
		}
		return out
	}

	report := RunBenchmarkWith(qb, cases, 5, hybrid)
	agg := report.AggregateNonNegative

	t.Logf("hybrid baseline: Recall@5=%.4f MRR=%.4f NDCG@5=%.4f (positive=%d)",
		agg.Recall, agg.MRR, agg.NDCG, report.PositiveCount)

	// S46 gate: hybrid Recall@5 and MRR must not drop below the reproducible
	// committed baseline. Thresholds are conservative (0.90 / 0.60) to avoid
	// flakiness from normal content drift; a real regression (e.g. broken
	// semantic channel, wrong weights, dedup bug) will fail decisively.
	if agg.Recall < 0.90 {
		t.Errorf("S46 REGRESSION: hybrid Recall@5=%.4f, want >= 0.90 (reproducible baseline ≈0.92)", agg.Recall)
	}
	if agg.MRR < 0.60 {
		t.Errorf("S46 REGRESSION: hybrid MRR=%.4f, want >= 0.60 (reproducible baseline ≈0.66)", agg.MRR)
	}
}
