package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/chunk"
	"github.com/superops-team/okf/pkg/embeddings"
	"github.com/superops-team/okf/pkg/lexical"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/query"
	"github.com/superops-team/okf/pkg/vectorindex"
)

// vectorIndexDir 返回知识库下的向量索引目录。
func vectorIndexDir(path string) string {
	return filepath.Join(path, ".okf", "vector")
}

// loadKnowledgeBundle 加载知识库（与 cmdSearch 相同的路径解析逻辑）。
func loadKnowledgeBundle(path string) (*query.KnowledgeBundle, *okf.KnowledgeBundle, error) {
	okfDir := filepath.Join(path, ".okf/knowledge")
	var bundle *okf.KnowledgeBundle
	var err error
	if okf.Exists(okfDir) {
		bundle, err = okf.LoadBundle(okfDir, okf.DefaultLoadOptions())
	} else {
		bundle, err = okf.LoadBundle(path, okf.DefaultLoadOptions())
	}
	if err != nil {
		return nil, nil, err
	}
	return toQueryBundle(bundle), bundle, nil
}

// conceptChunkSource 返回参与分块的正文。
// description 拼在正文前：它是概念的摘要，对首块的语义定位有帮助。
func conceptChunkSource(c *query.Concept) string {
	if c.Description == "" {
		return c.Content
	}
	if c.Content == "" {
		return c.Description
	}
	return c.Description + "\n\n" + c.Content
}

func cmdVector(args []string) int {
	if len(args) < 1 {
		fmt.Println("Error: vector requires a subcommand: index|status|rebuild")
		return 1
	}
	sub := args[0]
	fs := flag.NewFlagSet("vector "+sub, flag.ExitOnError)
	path := fs.String("path", "", "Knowledge base path")
	force := fs.Bool("force", false, "Force full rebuild (ignored unless subcommand supports it)")
	fs.Parse(args[1:])

	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	switch sub {
	case "index":
		return cmdVectorIndex(*path, *force)
	case "rebuild":
		return cmdVectorRebuild(*path)
	case "status":
		return cmdVectorStatus(*path)
	default:
		fmt.Printf("Error: unknown vector subcommand %q (index|status|rebuild)\n", sub)
		return 1
	}
}

func cmdVectorIndex(path string, force bool) int {
	qb, _, err := loadKnowledgeBundle(path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}
	dir := vectorIndexDir(path)

	fmt.Println("Loading embedding model (first run extracts embedded resources to cache)...")
	emb, err := embeddings.NewMiniLM()
	if err != nil {
		fmt.Printf("Error: 初始化向量模型失败: %v\n", err)
		return 1
	}
	defer emb.Close()

	idx := vectorindex.NewHNSW(emb.Dimension())
	loaded := false
	if !force {
		if _, err := idx.Load(dir); err == nil {
			loaded = true
		}
	}

	start := time.Now()
	added := 0
	chunks := 0
	for _, c := range qb.Concepts {
		// 分块编码：整概念直接编码会在 MiniLM 的 256 token 处被截断，
		// 长文档大部分内容无法进入索引（实测本仓库知识库丢失 70.6%）。
		cs := chunk.Split(c.Title, conceptChunkSource(c), chunk.Options{})
		if len(cs) == 0 {
			// 无正文的概念：仍以标题+描述建立一个块，保证可被检索到
			cs = []chunk.Chunk{{Breadcrumb: c.Title, Body: c.Description}}
		}
		for _, ch := range cs {
			vec, err := emb.EmbedQuery(ch.Text())
			if err != nil {
				fmt.Printf("Error: 编码概念 %s (块 %d) 失败: %v\n", c.Title, ch.Ordinal, err)
				return 1
			}
			before := idx.Len()
			idx.Add(query.ChunkKey(c, ch.Ordinal), vec)
			if idx.Len() > before {
				added++
			}
			chunks++
		}
	}

	meta := vectorindex.Meta{Model: "minilm-int8", OkfVersion: Version, Concepts: len(qb.Concepts)}
	if err := idx.Save(dir, meta); err != nil {
		fmt.Printf("Error: 保存索引失败: %v\n", err)
		return 1
	}
	mode := "build"
	if loaded {
		mode = "incremental"
	}
	if force {
		mode = "rebuild"
	}
	fmt.Printf("Indexed %d chunks from %d concepts (%d new, %s) in %s → %s\n",
		idx.Len(), len(qb.Concepts), added, mode, time.Since(start).Round(time.Millisecond), dir)
	return 0
}

func cmdVectorRebuild(path string) int {
	dir := vectorIndexDir(path)
	if err := os.RemoveAll(dir); err != nil {
		fmt.Printf("Error: 清理旧索引失败: %v\n", err)
		return 1
	}
	fmt.Println("Old index removed. Rebuilding...")
	return cmdVectorIndex(path, true)
}

func cmdVectorStatus(path string) int {
	dir := vectorIndexDir(path)
	metaPath := filepath.Join(dir, vectorindex.MetaFileName)
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		fmt.Println("尚未构建向量索引。请先执行: okf vector index")
		return 0
	}
	var meta vectorindex.Meta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		fmt.Printf("Error: 解析索引元信息失败: %v（请执行 okf vector rebuild）\n", err)
		return 1
	}
	binPath := filepath.Join(dir, vectorindex.IndexFileName)
	info, err := os.Stat(binPath)
	if err != nil {
		fmt.Printf("Error: 索引文件缺失: %v（请执行 okf vector rebuild）\n", err)
		return 1
	}
	fmt.Printf("向量索引状态:\n")
	fmt.Printf("  索引文件:  %s (%d bytes)\n", binPath, info.Size())
	fmt.Printf("  分块数:    %d\n", meta.Count)
	if meta.Concepts > 0 {
		fmt.Printf("  概念数:    %d\n", meta.Concepts)
	}
	fmt.Printf("  向量维度:  %d\n", meta.Dims)
	fmt.Printf("  模型:      %s\n", meta.Model)
	fmt.Printf("  索引格式:  v%d\n", meta.IndexFormatVersion)
	fmt.Printf("  okf 版本:  %s\n", meta.OkfVersion)
	if meta.IndexFormatVersion != vectorindex.CurrentIndexFormatVersion {
		fmt.Printf("\n注意: 索引格式为 v%d，当前版本需 v%d，请执行 okf vector rebuild\n",
			meta.IndexFormatVersion, vectorindex.CurrentIndexFormatVersion)
	}
	return 0
}

// runSemanticSearch 执行语义检索：加载索引 + MiniLM 编码 + BM25 词法通道 + 加权 RRF 混合。
// 返回 error 时调用方应回退到词法检索。
// lexicalWeight < 0 表示使用默认权重；0 表示关闭词法通道（纯语义）。
func runSemanticSearch(qb *query.KnowledgeBundle, text, path string, topK int, lexicalWeight float64) ([]query.SearchResult, error) {
	if text == "" {
		return nil, fmt.Errorf("语义检索需要 -q 查询文本")
	}
	emb, err := embeddings.NewMiniLM()
	if err != nil {
		return nil, fmt.Errorf("初始化向量模型失败: %v", err)
	}
	defer emb.Close()
	idx := vectorindex.NewHNSW(emb.Dimension())
	if _, err := idx.Load(vectorIndexDir(path)); err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			return nil, fmt.Errorf("尚未构建向量索引，请先执行 okf vector index")
		}
		return nil, fmt.Errorf("向量索引不可用: %v", err)
	}
	backend := &semanticBackend{emb: emb, idx: idx}

	opts := query.SearchOptions{TopK: topK}
	if lexicalWeight != 0 {
		// BM25 索引在内存中即时构建：分块是纯函数、BM25 只统计词频，
		// 本仓库知识库实测 <10ms，无需落盘（避免与向量索引的一致性维护成本）。
		opts.Lexical = buildLexicalBackend(qb)
	}
	if lexicalWeight >= 0 {
		opts = opts.WithLexicalWeight(lexicalWeight)
	}
	return query.SemanticSearch(qb, text, backend, opts)
}

// buildLexicalBackend 以 chunk 为单位构建 BM25 索引，key 与向量索引一致，
// 使两个通道的命中都能回溯到同一父概念。
func buildLexicalBackend(qb *query.KnowledgeBundle) *bm25Backend {
	bm := lexical.NewBM25()
	for _, c := range qb.Concepts {
		cs := chunk.Split(c.Title, conceptChunkSource(c), chunk.Options{})
		if len(cs) == 0 {
			cs = []chunk.Chunk{{Breadcrumb: c.Title, Body: c.Description}}
		}
		for _, ch := range cs {
			bm.Add(query.ChunkKey(c, ch.Ordinal), ch.Text())
		}
	}
	bm.Finalize()
	return &bm25Backend{bm: bm}
}

// bm25Backend 适配 query.LexicalBackend。
type bm25Backend struct{ bm *lexical.BM25 }

func (b *bm25Backend) Search(q string, k int) []query.SemanticHit {
	hits := b.bm.Search(q, k)
	out := make([]query.SemanticHit, len(hits))
	for i, h := range hits {
		out[i] = query.SemanticHit{Key: h.Key, Score: float32(h.Score)}
	}
	return out
}

// semanticBackend 适配 query.SemanticBackend：MiniLM 编码 + HNSW 近邻检索。
type semanticBackend struct {
	emb *embeddings.MiniLM
	idx *vectorindex.HNSW
}

func (b *semanticBackend) EmbedQuery(text string) ([]float32, error) {
	return b.emb.EmbedQuery(text)
}

func (b *semanticBackend) Search(vec []float32, k int) []query.SemanticHit {
	matches := b.idx.Search(vec, k)
	out := make([]query.SemanticHit, len(matches))
	for i, m := range matches {
		out[i] = query.SemanticHit{Key: m.Key, Score: m.Score}
	}
	return out
}
