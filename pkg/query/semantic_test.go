package query

import (
	"errors"
	"testing"
)

// fakeBackend 是确定性测试后端：固定查询向量与召回结果。
type fakeBackend struct {
	hits []SemanticHit
	err  error
}

func (f *fakeBackend) EmbedQuery(text string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []float32{1, 0}, nil
}

func (f *fakeBackend) Search(vec []float32, k int) []SemanticHit {
	if k < len(f.hits) {
		return f.hits[:k]
	}
	return f.hits
}

func newTestBundle() *KnowledgeBundle {
	return &KnowledgeBundle{Concepts: []*Concept{
		{Type: "doc", Title: "Alpha", Content: "contains apple banana fruit", FilePath: "/kb/a.md"},
		{Type: "doc", Title: "Beta", Content: "completely different topic", FilePath: "/kb/b.md"},
		{Type: "doc", Title: "Gamma", Content: "apple appears again", FilePath: "/kb/c.md"},
	}}
}

func TestSemanticSearchRRFAndSource(t *testing.T) {
	b := newTestBundle()
	c1, c2 := b.Concepts[0], b.Concepts[1]
	// 语义召回：Beta(最相关) + Alpha；词法 "apple" 命中 Alpha、Gamma
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(c2), Score: 0.9},
		{Key: Fingerprint(c1), Score: 0.8},
	}}

	res, err := SemanticSearch(b, "apple", fb, SearchOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results, want 3", len(res))
	}
	// Alpha 双通道命中（both）应排第一
	if res[0].Concept != c1 || res[0].Source != "both" {
		t.Fatalf("top = %q source=%q, want Alpha/both", res[0].Concept.Title, res[0].Source)
	}
	// Beta 仅语义、Gamma 仅词法
	sourceByTitle := map[string]string{}
	for _, r := range res {
		sourceByTitle[r.Concept.Title] = r.Source
	}
	if sourceByTitle["Beta"] != "semantic" {
		t.Fatalf("Beta source = %q, want semantic", sourceByTitle["Beta"])
	}
	if sourceByTitle["Gamma"] != "lexical" {
		t.Fatalf("Gamma source = %q, want lexical", sourceByTitle["Gamma"])
	}
	// 融合分递减
	for i := 1; i < len(res); i++ {
		if res[i].SemanticScore > res[i-1].SemanticScore {
			t.Fatalf("scores not descending: %v", res)
		}
	}
}

func TestSemanticSearchNilBackendIsLexicalOnly(t *testing.T) {
	b := newTestBundle()
	res, err := SemanticSearch(b, "apple", nil, SearchOptions{TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	// 纯词法：Alpha 与 Gamma 命中（content 含 apple），Beta 不命中
	titles := map[string]bool{}
	for _, r := range res {
		titles[r.Concept.Title] = true
		if r.Source != "lexical" {
			t.Fatalf("source = %q, want lexical", r.Source)
		}
	}
	if !titles["Alpha"] || !titles["Gamma"] || titles["Beta"] {
		t.Fatalf("lexical titles = %v", titles)
	}
}

func TestSemanticSearchEmbedError(t *testing.T) {
	b := newTestBundle()
	fb := &fakeBackend{err: errors.New("embed failed")}
	_, err := SemanticSearch(b, "apple", fb, SearchOptions{})
	if err == nil {
		t.Fatal("embed 失败未返回错误")
	}
}

func TestSemanticSearchTopK(t *testing.T) {
	b := newTestBundle()
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(b.Concepts[0]), Score: 0.9},
		{Key: Fingerprint(b.Concepts[1]), Score: 0.8},
		{Key: Fingerprint(b.Concepts[2]), Score: 0.7},
	}}
	res, err := SemanticSearch(b, "apple", fb, SearchOptions{TopK: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d results, want 2", len(res))
	}
}

func TestFingerprintNormalization(t *testing.T) {
	a := &Concept{Type: "Doc", Title: "Alpha", FilePath: "kb/./a.md"}
	b := &Concept{Type: "doc", Title: "alpha", FilePath: "kb/a.md"}
	c := &Concept{Type: "doc", Title: "alpha", FilePath: "/abs/kb/a.md"}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatalf("指纹应归一化: %q != %q", Fingerprint(a), Fingerprint(b))
	}
	// 相对路径不应被 Abs 化（避免依赖 cwd），与绝对路径应不同
	if Fingerprint(a) == Fingerprint(c) {
		t.Fatal("相对/绝对路径指纹不应相同（避免 cwd 依赖）")
	}
}
