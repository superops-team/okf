package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/embeddings"
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

// conceptText 拼接概念的编码文本（title + description + content，tokenizer 截断至 256 token）。
func conceptText(c *query.Concept) string {
	var sb strings.Builder
	sb.WriteString(c.Title)
	if c.Description != "" {
		sb.WriteString("\n")
		sb.WriteString(c.Description)
	}
	if c.Content != "" {
		sb.WriteString("\n")
		sb.WriteString(c.Content)
	}
	return sb.String()
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
	for _, c := range qb.Concepts {
		vec, err := emb.EmbedQuery(conceptText(c))
		if err != nil {
			fmt.Printf("Error: 编码概念 %s 失败: %v\n", c.Title, err)
			return 1
		}
		before := idx.Len()
		idx.Add(query.Fingerprint(c), vec)
		if idx.Len() > before {
			added++
		}
	}

	meta := vectorindex.Meta{Model: "minilm-int8", OkfVersion: Version}
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
	fmt.Printf("Indexed %d concepts (%d new, %s) in %s → %s\n",
		idx.Len(), added, mode, time.Since(start).Round(time.Millisecond), dir)
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
	fmt.Printf("  条目数:    %d\n", meta.Count)
	fmt.Printf("  向量维度:  %d\n", meta.Dims)
	fmt.Printf("  模型:      %s\n", meta.Model)
	fmt.Printf("  okf 版本:  %s\n", meta.OkfVersion)
	return 0
}

// runSemanticSearch 执行语义检索：加载索引 + MiniLM 编码 + RRF 混合检索。
// 返回 error 时调用方应回退到词法检索。
func runSemanticSearch(qb *query.KnowledgeBundle, text, path string, topK int) ([]query.SearchResult, error) {
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
	return query.SemanticSearch(qb, text, backend, query.SearchOptions{TopK: topK})
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
