package query

import (
	"github.com/superops-team/okf/pkg/chunk"
	"github.com/superops-team/okf/pkg/lexical"
)

// ConceptChunkSource 返回参与分块的概念文本（description 置于正文前）。
//
// description 通常是概念的一句话摘要，语义密度高，放在最前可让首个 chunk
// 同时携带摘要与正文开头，提升短查询的命中率。
func ConceptChunkSource(c *Concept) string {
	if c == nil {
		return ""
	}
	if c.Description == "" {
		return c.Content
	}
	if c.Content == "" {
		return c.Description
	}
	return c.Description + "\n\n" + c.Content
}

// ConceptChunks 返回概念的分块结果，是向量通道与词法通道**共同**的分块入口。
//
// 两个通道必须使用完全相同的分块口径与 fallback，否则同一 ordinal 在两侧
// 指向不同文本，ChunkKey 对不齐，RRF 会把不相干的命中当作同一块融合。
// 因此这里集中实现，不允许调用方各自 chunk.Split。
func ConceptChunks(c *Concept) []chunk.Chunk {
	if c == nil {
		return nil
	}
	cs := chunk.Split(c.Title, ConceptChunkSource(c), chunk.Options{})
	if len(cs) == 0 {
		// 无正文的概念：仍以标题+描述建立一个块，保证可被检索到
		cs = []chunk.Chunk{{Breadcrumb: c.Title, Body: c.Description}}
	}
	return cs
}

// BuildLexicalBackend 以 chunk 为单位构建内存 BM25 索引，key 与向量索引一致，
// 使两个通道的命中都能经 ChunkKeyConcept 回溯到同一父概念。
//
// 索引在内存中即时构建而不落盘：分块是纯函数、BM25 仅统计词频，
// 本仓库知识库实测 <10ms，避免与向量索引的一致性维护成本。
//
// 本函数由 CLI 与 MCP 共用：两个入口若各自构建，极易出现 key 口径不一致
// 或其中一方漏接词法通道（退化为召回率极低的子串匹配）。
func BuildLexicalBackend(bundle *KnowledgeBundle) LexicalBackend {
	if bundle == nil {
		return nil
	}
	bm := lexical.NewBM25()
	for _, c := range bundle.Concepts {
		for _, ch := range ConceptChunks(c) {
			bm.Add(ChunkKey(c, ch.Ordinal), ch.Text())
		}
	}
	bm.Finalize()
	return &bm25Backend{bm: bm}
}

// bm25Backend 适配 LexicalBackend 接口。
type bm25Backend struct{ bm *lexical.BM25 }

func (b *bm25Backend) Search(q string, k int) []SemanticHit {
	hits := b.bm.Search(q, k)
	out := make([]SemanticHit, len(hits))
	for i, h := range hits {
		out[i] = SemanticHit{Key: h.Key, Score: float32(h.Score)}
	}
	return out
}
