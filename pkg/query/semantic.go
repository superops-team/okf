package query

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// SemanticHit 是语义召回通道返回的单条命中（key 为概念指纹）。
type SemanticHit struct {
	Key   string
	Score float32
}

// SemanticBackend 抽象语义检索所需的嵌入与近邻检索能力，便于测试注入 fake。
// 生产实现：pkg/embeddings.MiniLM + pkg/vectorindex.HNSW。
type SemanticBackend interface {
	EmbedQuery(text string) ([]float32, error)
	Search(vec []float32, k int) []SemanticHit
}

// Fingerprint 生成概念的唯一索引指纹：lower(type):lower(title):规范化相对路径。
// 与向量索引的 key 一致（CLI 构建索引与检索需使用同一函数）。
// 注意：FilePath 是相对 bundle 根的路径，这里刻意不做 filepath.Abs——
// 避免指纹依赖进程工作目录（CLI 与 MCP 进程 cwd 不同会导致 key 不一致）。
func Fingerprint(c *Concept) string {
	return strings.ToLower(c.Type) + ":" + strings.ToLower(c.Title) + ":" + filepath.ToSlash(filepath.Clean(c.FilePath))
}

// rrfK 是 Reciprocal Rank Fusion 的常数 k。
const rrfK = 60

func rrfScore(rank int) float32 { return 1.0 / (rrfK + float32(rank)) }

// SearchOptions 控制语义检索行为。
type SearchOptions struct {
	// TopK 返回结果数（默认 10）。
	TopK int
	// SemanticTopK 语义召回候选数（默认 20）。
	SemanticTopK int
	// LexicalTopK 词法召回候选数（默认 20）。
	LexicalTopK int
}

// SemanticSearch 将语义召回（backend）与既有词法召回（子串/正则）经 RRF 融合后返回结果。
// 词法通道始终参与；backend 为 nil 时退化为纯词法（等价 SearchWithMatches 行为）。
func SemanticSearch(bundle *KnowledgeBundle, text string, backend SemanticBackend, opts SearchOptions) ([]SearchResult, error) {
	if bundle == nil {
		return nil, nil
	}
	if opts.TopK <= 0 {
		opts.TopK = 10
	}
	if opts.SemanticTopK <= 0 {
		opts.SemanticTopK = 20
	}
	if opts.LexicalTopK <= 0 {
		opts.LexicalTopK = 20
	}

	// 指纹 → 概念 映射
	byFingerprint := make(map[string]*Concept, len(bundle.Concepts))
	for _, c := range bundle.Concepts {
		byFingerprint[Fingerprint(c)] = c
	}

	// 语义通道
	semRank := make(map[*Concept]int) // 1-based rank
	if backend != nil {
		vec, err := backend.EmbedQuery(text)
		if err != nil {
			return nil, fmt.Errorf("semantic embed query: %w", err)
		}
		for i, hit := range backend.Search(vec, opts.SemanticTopK) {
			if c, ok := byFingerprint[hit.Key]; ok {
				if _, seen := semRank[c]; !seen {
					semRank[c] = i + 1
				}
			}
		}
	}

	// 词法通道
	lexRank := make(map[*Concept]int) // 1-based rank
	lex := SearchWithMatches(bundle, text)
	if opts.LexicalTopK < len(lex) {
		lex = lex[:opts.LexicalTopK]
	}
	for i, r := range lex {
		if _, seen := lexRank[r.Concept]; !seen {
			lexRank[r.Concept] = i + 1
		}
	}

	// RRF 融合 + 来源标注
	score := make(map[*Concept]float32)
	source := make(map[*Concept]string)
	for c, r := range semRank {
		score[c] += rrfScore(r)
		source[c] = "semantic"
	}
	for c, r := range lexRank {
		score[c] += rrfScore(r)
		switch source[c] {
		case "semantic":
			source[c] = "both"
		default:
			source[c] = "lexical"
		}
	}

	// 排序（降序；同分按词法 rank 升序，保持稳定）
	keys := make([]*Concept, 0, len(score))
	for c := range score {
		keys = append(keys, c)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if score[keys[i]] != score[keys[j]] {
			return score[keys[i]] > score[keys[j]]
		}
		ri, iok := semRank[keys[i]]
		rj, jok := semRank[keys[j]]
		if iok && jok {
			return ri < rj
		}
		return source[keys[i]] > source[keys[j]] // both > semantic > lexical
	})

	out := make([]SearchResult, 0, len(keys))
	for i, c := range keys {
		if i >= opts.TopK {
			break
		}
		out = append(out, SearchResult{
			Concept:       c,
			Source:        source[c],
			SemanticScore: score[c],
		})
	}
	return out, nil
}
