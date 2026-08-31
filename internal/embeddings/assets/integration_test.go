//go:build integration

package assets

import (
	"math"
	"testing"

	"github.com/amikos-tech/pure-onnx/embeddings/minilm"
	"github.com/amikos-tech/pure-onnx/ort"
)

// TestIntegrationInt8Embedding 端到端验证：Ensure 解包 → ORT 初始化 → int8 模型推理。
// 仅以 -tags integration 运行（CI 冒烟/gauntlet L10），默认单测不触碰真实模型。
func TestIntegrationInt8Embedding(t *testing.T) {
	t.Setenv("OKF_ORT_DIR", t.TempDir())
	p, err := Ensure()
	if err != nil {
		t.Fatal(err)
	}

	if err := ort.SetSharedLibraryPath(p.ORTLib); err != nil {
		t.Fatalf("SetSharedLibraryPath: %v", err)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = ort.DestroyEnvironment() })

	emb, err := minilm.NewEmbedder(p.Model, p.Tokenizer, minilm.WithMeanPooling(), minilm.WithL2Normalization())
	if err != nil {
		t.Fatalf("NewEmbedder: %v", err)
	}
	defer emb.Close()

	v, err := emb.EmbedQuery("hello world")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(v) != int(minilm.OutputEmbeddingDimension) {
		t.Fatalf("dim = %d, want %d", len(v), minilm.OutputEmbeddingDimension)
	}
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if math.Abs(norm-1.0) > 1e-3 {
		t.Fatalf("not unit norm: %f", norm)
	}

	a, _ := emb.EmbedQuery("the dog runs in the park")
	b, _ := emb.EmbedQuery("a dog is running outside")
	c, _ := emb.EmbedQuery("i love eating pizza")
	if dot(a, b) <= dot(a, c) {
		t.Fatalf("语义 sanity 失败: dog/dog=%f <= dog/pizza=%f", dot(a, b), dot(a, c))
	}
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}
