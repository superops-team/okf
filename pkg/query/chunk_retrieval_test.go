package query

import (
	"testing"
)

// 本文件覆盖 spec「分块级向量索引与父概念回溯」中尚未被其他测试固定的场景：
// 长文档尾部可命中、候选放大、TopK 上限。

// chunkAwareBackend 模拟真实的块级向量索引：
// 只有被显式登记为「已入索引」的 chunk key 才可能被召回，
// 借此复现改动前「概念级索引 + 256 token 截断」导致尾部内容不可命中的行为。
type chunkAwareBackend struct {
	// hits 是按相关性排好序的 chunk key
	hits []string
	// lastK 记录调用方请求的候选数，用于验证候选放大
	lastK int
}

func (b *chunkAwareBackend) EmbedQuery(string) ([]float32, error) { return []float32{1}, nil }

func (b *chunkAwareBackend) Search(_ []float32, k int) []SemanticHit {
	b.lastK = k
	out := make([]SemanticHit, 0, k)
	for i, key := range b.hits {
		if i >= k {
			break
		}
		out = append(out, SemanticHit{Key: key, Score: float32(1.0 / float64(i+1))})
	}
	return out
}

// Scenario: 长文档的尾部内容可被语义检索命中。
//
// 对比两种索引方式对同一 query 的效果：
//   - 概念级（改动前）：整篇只有 1 个向量，尾部小节因 256 token 截断丢失 → 召回不到
//   - 分块级（改动后）：尾部小节自成一块 → 可召回
func TestLongDocumentTailIsRetrievable(t *testing.T) {
	long := &Concept{
		Type: "documentation", Title: "Long Doc", FilePath: "long.md",
		Content: "## Head\n\nintro\n\n## Tail\n\nzzz-tail-marker-phrase\n",
	}
	other := &Concept{
		Type: "documentation", Title: "Other", FilePath: "other.md", Content: "unrelated",
	}
	bundle := &KnowledgeBundle{Concepts: []*Concept{long, other}}

	// 改动前：概念级索引，尾部内容不在索引中 → 只有 other 能被召回
	before := &chunkAwareBackend{hits: []string{Fingerprint(other)}}
	got, err := SemanticSearch(bundle, "tail marker phrase", before,
		SearchOptions{TopK: 5}.WithLexicalWeight(0))
	if err != nil {
		t.Fatal(err)
	}
	if containsConcept(got, long) {
		t.Fatal("前置条件不成立：概念级索引下尾部内容本应召回不到")
	}

	// 改动后：尾部小节自成一块（#1）并进入索引 → 必须能召回到父概念
	after := &chunkAwareBackend{hits: []string{ChunkKey(long, 1), Fingerprint(other)}}
	got, err = SemanticSearch(bundle, "tail marker phrase", after,
		SearchOptions{TopK: 5}.WithLexicalWeight(0))
	if err != nil {
		t.Fatal(err)
	}
	if !containsConcept(got, long) {
		t.Errorf("分块级索引下尾部命中未回溯到父概念，结果=%v", titlesOf(got))
	}
	if got[0].Concept != long {
		t.Errorf("top1 = %q，期望 Long Doc", got[0].Concept.Title)
	}
}

// Scenario: 召回候选放大以避免单概念占满 TopK。
func TestCandidateAmplification(t *testing.T) {
	c := &Concept{Type: "documentation", Title: "a", FilePath: "a.md", Content: "x"}
	bundle := &KnowledgeBundle{Concepts: []*Concept{c}}

	b := &chunkAwareBackend{hits: []string{ChunkKey(c, 0)}}
	_, err := SemanticSearch(bundle, "q", b,
		SearchOptions{TopK: 5, CandidateFactor: 4}.WithLexicalWeight(0))
	if err != nil {
		t.Fatal(err)
	}
	if want := 5 * 4; b.lastK < want {
		t.Errorf("底层候选数 = %d，至少应为 TopK*CandidateFactor = %d", b.lastK, want)
	}
}

// CandidateFactor 未设置时应使用默认值 4。
func TestCandidateFactorDefaultsToFour(t *testing.T) {
	c := &Concept{Type: "documentation", Title: "a", FilePath: "a.md", Content: "x"}
	bundle := &KnowledgeBundle{Concepts: []*Concept{c}}

	b := &chunkAwareBackend{hits: []string{ChunkKey(c, 0)}}
	if _, err := SemanticSearch(bundle, "q", b,
		SearchOptions{TopK: 3}.WithLexicalWeight(0)); err != nil {
		t.Fatal(err)
	}
	if want := 3 * 4; b.lastK != want {
		t.Errorf("默认候选数 = %d，期望 %d（CandidateFactor 默认 4）", b.lastK, want)
	}
}

// Scenario: 同一概念的多个 chunk 命中后合并为一条结果（去重 + 不超过 TopK）。
func TestManyChunksOfSameConceptCollapseToOneResult(t *testing.T) {
	c1 := &Concept{Type: "documentation", Title: "a", FilePath: "a.md", Content: "x"}
	c2 := &Concept{Type: "documentation", Title: "b", FilePath: "b.md", Content: "y"}
	bundle := &KnowledgeBundle{Concepts: []*Concept{c1, c2}}

	// c1 占据 6 个候选位，c2 只占 1 个
	b := &chunkAwareBackend{hits: []string{
		ChunkKey(c1, 0), ChunkKey(c1, 1), ChunkKey(c1, 2),
		ChunkKey(c1, 3), ChunkKey(c1, 4), ChunkKey(c1, 5),
		ChunkKey(c2, 0),
	}}
	got, err := SemanticSearch(bundle, "q", b,
		SearchOptions{TopK: 2}.WithLexicalWeight(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 2 {
		t.Errorf("返回 %d 条，MUST NOT 超过 TopK=2", len(got))
	}
	seen := map[*Concept]int{}
	for _, r := range got {
		seen[r.Concept]++
	}
	for c, n := range seen {
		if n != 1 {
			t.Errorf("概念 %q 出现 %d 次，应去重为 1 次", c.Title, n)
		}
	}
	// c2 必须仍能进入结果（否则单概念占满了 TopK，候选放大失去意义）
	if _, ok := seen[c2]; !ok {
		t.Errorf("c2 未进入结果，单概念占满了 TopK；结果=%v", titlesOf(got))
	}
}

func containsConcept(rs []SearchResult, c *Concept) bool {
	for _, r := range rs {
		if r.Concept == c {
			return true
		}
	}
	return false
}

func titlesOf(rs []SearchResult) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Concept.Title)
	}
	return out
}
