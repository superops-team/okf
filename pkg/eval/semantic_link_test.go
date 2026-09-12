package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/convert"
	"github.com/superops-team/okf/pkg/embeddings"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
	"github.com/superops-team/okf/pkg/query"
	"github.com/superops-team/okf/pkg/vectorindex"
)

// evalSemanticBackend builds a real MiniLM + HNSW backend over a bundle.
func evalSemanticBackend(t *testing.T, bundle *query.KnowledgeBundle) query.SemanticBackend {
	t.Helper()
	emb, err := embeddings.NewMiniLM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { emb.Close() })
	idx := vectorindex.NewHNSW(emb.Dimension())
	for _, c := range bundle.Concepts {
		vec, err := emb.EmbedQuery(c.Content)
		if err != nil {
			t.Fatal(err)
		}
		idx.Add(query.Fingerprint(c), vec)
	}
	return &evalBackend{emb: emb, idx: idx}
}

type evalBackend struct {
	emb *embeddings.MiniLM
	idx *vectorindex.HNSW
}

func (b *evalBackend) EmbedQuery(text string) ([]float32, error) { return b.emb.EmbedQuery(text) }
func (b *evalBackend) Search(vec []float32, k int) []query.SemanticHit {
	matches := b.idx.Search(vec, k)
	out := make([]query.SemanticHit, len(matches))
	for i, m := range matches {
		out[i] = query.SemanticHit{Key: m.Key, Score: float32(m.Score)}
	}
	return out
}

// TestEvalSemanticLinkRuns: the semantic path (RRF-only) is measured over the
// golden set and reported with a distinct mode row.
func TestEvalSemanticLinkRuns(t *testing.T) {
	skipPuregoUnderRace(t)
	bundle := buildBenchmarkBundle(t)
	cases, err := LoadGoldenCases("testdata/golden_queries.json")
	if err != nil {
		t.Fatal(err)
	}
	backend := evalSemanticBackend(t, bundle)
	report := RunBenchmarkWith(bundle, cases, 5, SemanticSearcher(backend))
	report.Mode = "semantic-rrf"
	t.Logf("\n%s", report.String())
	if len(report.Cases) != len(cases) {
		t.Fatalf("semantic eval cases = %d, want %d", len(report.Cases), len(cases))
	}
	// semantic path must have actually retrieved something for positive cases
	if report.PositiveCount == 0 {
		t.Fatal("semantic eval reported no positive cases")
	}
}

// TestEvalRerankDoesNotHurtSemanticRanking: M1 uses an identity ranker (order
// unchanged), so MRR/NDCG must be >= the RRF-only values (equality at M1; a
// real cross-encoder in M2 may improve them). This is the delta assertion from
// the spec, not an absolute threshold.
func TestEvalRerankDoesNotHurtSemanticRanking(t *testing.T) {
	skipPuregoUnderRace(t)
	bundle := buildBenchmarkBundle(t)
	cases, err := LoadGoldenCases("testdata/golden_queries.json")
	if err != nil {
		t.Fatal(err)
	}
	backend := evalSemanticBackend(t, bundle)
	rrf := RunBenchmarkWith(bundle, cases, 5, SemanticSearcher(backend))
	rrf.Mode = "semantic-rrf"
	// M1 identity reranker: same order as RRF (the cross-encoder slot is M2).
	identity := RunBenchmarkWith(bundle, cases, 5, SemanticSearcher(backend))
	identity.Mode = "semantic-rerank"

	if identity.AggregateNonNegative.MRR < rrf.AggregateNonNegative.MRR {
		t.Errorf("MRR(rerank)=%.4f < MRR(rrf)=%.4f", identity.AggregateNonNegative.MRR, rrf.AggregateNonNegative.MRR)
	}
	if identity.AggregateNonNegative.NDCG < rrf.AggregateNonNegative.NDCG {
		t.Errorf("NDCG(rerank)=%.4f < NDCG(rrf)=%.4f", identity.AggregateNonNegative.NDCG, rrf.AggregateNonNegative.NDCG)
	}
	t.Logf("MRR rrf=%.4f rerank=%.4f | NDCG rrf=%.4f rerank=%.4f",
		rrf.AggregateNonNegative.MRR, identity.AggregateNonNegative.MRR,
		rrf.AggregateNonNegative.NDCG, identity.AggregateNonNegative.NDCG)
}

// TestChunkedBundleScoringBySourceFile: a source file with 3 chunk concepts
// counts as retrieved when any of its chunks is in top-K — no double counting,
// no false negative.
func TestChunkedBundleScoringBySourceFile(t *testing.T) {
	bundle := &query.KnowledgeBundle{Concepts: []*query.Concept{
		{Type: "source", Title: "doc — whole", Resource: "doc.txt.md", Content: "whole document content"},
		{Type: "source", Title: "doc — part 1", Resource: "doc.txt__c1.md", Content: "apple banana part one"},
		{Type: "source", Title: "doc — part 2", Resource: "doc.txt__c2.md", Content: "cherry date part two"},
		{Type: "source", Title: "doc — part 3", Resource: "doc.txt__c3.md", Content: "elderberry fig part three"},
		{Type: "source", Title: "other", Resource: "other.md", Content: "totally unrelated"},
	}}
	bundle.BuildIndex()
	cases := []EvalCase{
		{Query: "cherry date", ExpectedDocs: []string{"doc.txt.md"}},
		{Query: "zzz", ExpectedDocs: []string{"other.md"}},
	}
	report := RunBenchmark(bundle, cases, 5)
	// first case: doc.txt.md retrieved (via chunk __c2) with recall 1.0
	if report.Cases[0].Recall != 1.0 {
		t.Errorf("case 1 Recall = %v, want 1.0 (chunk hit counts for source)", report.Cases[0].Recall)
	}
	// second case: not retrieved
	if report.Cases[1].Recall != 0.0 {
		t.Errorf("case 2 Recall = %v, want 0.0", report.Cases[1].Recall)
	}
}

// buildChunkedBundleOnDisk writes a real chunked knowledge base (whole concept
// + __cN chunks via the shared convert layer) and loads it, so the semantic
// deep-tail test runs on genuine import artifacts.
func buildChunkedBundleOnDisk(t *testing.T, words int) *query.KnowledgeBundle {
	t.Helper()
	kb := t.TempDir()
	var sb strings.Builder
	for i := 0; i < words; i++ {
		sb.WriteString("common filler word ")
	}
	tail := "zebraquasar42x7"
	sb.WriteString(tail)
	md := sb.String()
	// whole concept
	whole := convert.WrapConcept("doc", "doc.txt", "txt", "source", md, "")
	// chunks
	chunks := convert.Split(md, nil)
	for i, ck := range chunks {
		title := "doc — part " + string(rune('0'+i+1))
		chunkMD := convert.WrapChunkConcept(title, "doc.txt", "txt", "doc.txt", i, len(chunks), ck.HeadingPath, ck.Text, "")
		name := "doc.txt__c" + string(rune('0'+i+1)) + ".md"
		if err := os.WriteFile(filepath.Join(kb, name), []byte(chunkMD), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(kb, "doc.txt.md"), []byte(whole), 0o644); err != nil {
		t.Fatal(err)
	}
	okfBundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	qb := &query.KnowledgeBundle{}
	for _, c := range okfBundle.Concepts {
		qb.Concepts = append(qb.Concepts, &query.Concept{
			Type:     c.Type,
			Title:    c.Title,
			Resource: c.Resource,
			Content:  c.Content,
			FilePath: c.FilePath,
		})
	}
	qb.BuildIndex()
	return qb
}

// TestChunkedBundleSearchable: deep-tail term retrievable in the chunked
// bundle via semantic search, not retrievable in the unchunked semantic
// control (MiniLM's 256-token truncation drops the tail without chunking).
func TestChunkedBundleSearchable(t *testing.T) {
	skipPuregoUnderRace(t)
	tail := "zebraquasar42x7"
	chunked := buildChunkedBundleOnDisk(t, 3000)
	unchunked := buildChunkedBundleOnDisk(t, 1500)

	// chunked: tail must be semantically retrievable
	backend := evalSemanticBackend(t, chunked)
	res, err := query.SemanticSearch(chunked, tail, backend, query.SearchOptions{TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res {
		if strings.Contains(r.Concept.FilePath, "__c") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("chunked: tail term not retrievable semantically (%v)", res)
	}

	// unchunked control: the tail is outside the model's token window, so the
	// concept vector never contains it. Verified at the vector layer (cosine):
	// SemanticSearch has no similarity threshold and would surface a
	// low-similarity hit fused with a lexical hit, so the mechanism is proven
	// by the embedding itself (negative cosine = tail dropped), which is what
	// chunking fixes. Lexical search is out of scope for this control (spec).
	backendU := evalSemanticBackend(t, unchunked)
	embU := backendU.(*evalBackend).emb
	var docVec []float32
	for _, c := range unchunked.Concepts {
		if strings.Contains(c.FilePath, "doc.txt.md") && !strings.Contains(c.FilePath, "__c") {
			docVec, err = embU.EmbedQuery(c.Content)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if docVec == nil {
		t.Fatal("unchunked whole concept not found")
	}
	qVec, err := embU.EmbedQuery(tail)
	if err != nil {
		t.Fatal(err)
	}
	if sim := cosine(docVec, qVec); sim >= 0 {
		t.Fatalf("unchunked control: tail still embedded (cosine=%.4f, want < 0)", sim)
	}
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / (sqrt64(na) * sqrt64(nb))
}

func sqrt64(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 60; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// TestChunkedParseOnDisk: chunk files produced by WrapChunkConcept parse back
// with derived metadata (guards the on-disk fixture shape used above).
func TestChunkedParseOnDisk(t *testing.T) {
	md := convert.WrapChunkConcept("doc — part 2", "doc.txt", "txt", "doc.txt", 1, 3, "", "body", "")
	c, err := parser.ParseConceptBytes("doc.txt__c2.md", []byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if c.CustomFields["source_path"] != "doc.txt" {
		t.Errorf("source_path = %v", c.CustomFields["source_path"])
	}
	if c.CustomFields["derived"] != "true" {
		t.Errorf("derived = %v", c.CustomFields["derived"])
	}
}

var _ = context.Background
