//go:build integration

package embeddings

import (
	"math"
	"testing"
)

// TestIntegrationMiniLMNewAndBatch 验证 MiniLM 实现的完整生命周期：解包→初始化→查询/批量编码→Close。
// 仅以 -tags integration 运行（真实模型）。
func TestIntegrationMiniLMNewAndBatch(t *testing.T) {
	t.Setenv("OKF_ORT_DIR", t.TempDir())
	emb, err := NewMiniLM()
	if err != nil {
		t.Fatalf("NewMiniLM: %v", err)
	}
	defer emb.Close()

	if emb.Dimension() != DefaultDimension {
		t.Fatalf("Dimension = %d, want %d", emb.Dimension(), DefaultDimension)
	}

	q, err := emb.EmbedQuery("how do I lint my knowledge base")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(q) != DefaultDimension {
		t.Fatalf("query dim = %d", len(q))
	}

	docs, err := emb.EmbedDocuments([]string{
		"specification compliance checking rules",
		"markdown parser with frontmatter support",
	})
	if err != nil {
		t.Fatalf("EmbedDocuments: %v", err)
	}
	if len(docs) != 2 || len(docs[0]) != DefaultDimension || len(docs[1]) != DefaultDimension {
		t.Fatalf("docs dims wrong: %d docs, %d/%d", len(docs), len(docs[0]), len(docs[1]))
	}

	// 语义 sanity：文档1（lint）应比文档2（parser）更接近查询（lint）
	s1 := cosSim(q, docs[0])
	s2 := cosSim(q, docs[1])
	if s1 <= s2 {
		t.Fatalf("语义 sanity 失败: lint=%.4f parser=%.4f", s1, s2)
	}

	// 并发安全：-race 下多 goroutine 调用应无数据竞争
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = emb.EmbedQuery("concurrent embedding call")
		}()
	}
	for i := 0; i < 4; i++ {
		<-done
	}

	if err := emb.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func cosSim(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
