// Package embeddings 定义文本向量化抽象（Embedder）及 MiniLM 默认实现。
//
// 默认实现基于 github.com/amikos-tech/pure-onnx（MIT，purego，无 CGO），
// 模型与 ONNX Runtime 由 internal/embeddings/assets 内嵌并解包，运行时零联网。
// Embedder 接口同时作为测试注入点（单测使用确定性 fake，不依赖真实模型）。
package embeddings

import (
	"sync"

	"github.com/amikos-tech/pure-onnx/embeddings/minilm"
	"github.com/amikos-tech/pure-onnx/ort"

	"github.com/superops-team/okf/internal/embeddings/assets"
)

// Embedder 是文本向量化抽象，实现需线程安全。
type Embedder interface {
	// EmbedQuery 将单条查询文本编码为归一化向量。
	EmbedQuery(text string) ([]float32, error)
	// EmbedDocuments 将多条文档文本批量编码。
	EmbedDocuments(texts []string) ([][]float32, error)
	// Dimension 返回向量维度。
	Dimension() int
	// Close 释放底层资源；Close 后再次调用应返回错误。
	Close() error
}

// DefaultDimension 是 MiniLM-L6-v2 的输出维度。
const DefaultDimension = 384

// ortenvOnce 保证 ONNX Runtime 全局环境只初始化一次（其 API 为进程级单例）。
var (
	ortenvOnce sync.Once
	ortenvErr  error
)

// initORT 以内嵌 ORT 库初始化全局环境（显式路径模式，不使用 bootstrap/联网）。
func initORT(libPath string) error {
	ortenvOnce.Do(func() {
		if ort.IsInitialized() {
			return
		}
		if err := ort.SetSharedLibraryPath(libPath); err != nil {
			ortenvErr = err
			return
		}
		ortenvErr = ort.InitializeEnvironment()
	})
	return ortenvErr
}

// MiniLM 是内嵌 MiniLM-L6-v2 int8 量化模型的 Embedder 实现。
type MiniLM struct {
	mu    sync.Mutex // pure-onnx session 串行化，避免并发未定义行为
	inner *minilm.Embedder
	dim   int
}

// NewMiniLM 构建 MiniLM 实例：确保内嵌资源解包、初始化 ORT、加载模型。
func NewMiniLM() (*MiniLM, error) {
	paths, err := assets.Ensure()
	if err != nil {
		return nil, err
	}
	if err := initORT(paths.ORTLib); err != nil {
		return nil, err
	}
	inner, err := minilm.NewEmbedder(paths.Model, paths.Tokenizer,
		minilm.WithMeanPooling(),
		minilm.WithL2Normalization(),
	)
	if err != nil {
		return nil, err
	}
	return &MiniLM{inner: inner, dim: int(minilm.OutputEmbeddingDimension)}, nil
}

// EmbedQuery 编码单条查询文本。
func (m *MiniLM) EmbedQuery(text string) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inner.EmbedQuery(text)
}

// EmbedDocuments 批量编码文档文本。
func (m *MiniLM) EmbedDocuments(texts []string) ([][]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inner.EmbedDocuments(texts)
}

// Dimension 返回 384。
func (m *MiniLM) Dimension() int { return m.dim }

// Close 释放模型资源。
func (m *MiniLM) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inner.Close()
}
