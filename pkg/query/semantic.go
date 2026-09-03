package query

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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

// chunkKeySep 分隔概念指纹与块序号。
// 选择 '#'：概念指纹由 type/title/path 组成，其中 path 已被 filepath.Clean 规范化，
// 不会包含 '#'；title 若含 '#' 也不影响解析，因为 ChunkKeyConcept 从右侧截取。
const chunkKeySep = "#"

// ChunkKey 生成分块级索引 key：<概念指纹>#<块序号>。
func ChunkKey(c *Concept, ordinal int) string {
	return Fingerprint(c) + chunkKeySep + strconv.Itoa(ordinal)
}

// ChunkKeyConcept 从分块 key 反解出概念指纹；无分隔符时原样返回（兼容概念级 key）。
func ChunkKeyConcept(key string) string {
	if i := strings.LastIndex(key, chunkKeySep); i >= 0 {
		return key[:i]
	}
	return key
}

// RRF 与权重默认值。
//
// DefaultRRFK=60 取业界共识值，与 Elasticsearch rank_constant、
// Milvus RRFRanker、WeKnora GetEffectiveRRFK() 一致。
//
// 权重默认等权（1:1）。定案依据（实测，非推测）：
//   - 在 28 条语义化 golden set（pkg/eval/testdata/golden_semantic.json）上
//     扫描向量:词法比例 0.8~4.0，MRR 在 1:1 附近区间（0.8~1.2）达 0.804~0.824，
//     而 2.33:1（即 0.7/0.3）为 0.779；
//   - 区间内相邻比例的差异仅 ±1 个 case 量级（26 正样本下 1 个 case ≈ 0.019 MRR），
//     属噪声，故不选区间内的"最优单点"（会过拟合当前语料）；
//   - 另一组 16 条查询独立复测同样指向 1:1（MRR +12.5% vs 0.7/0.3 的 +1.3%），
//     两个集合结论一致，故采用等权而非业界常见的 0.7/0.3。
//
// 权重按相对比例生效（同比例缩放不改变排序），不要求权重和为 1.0。
const (
	DefaultRRFK          = 60
	DefaultVectorWeight  = 0.5
	DefaultLexicalWeight = 0.5
)

// rrfScore 计算加权 RRF 分量：weight / (k + rank)。
//
// k+rank <= 0 时返回 0：调用方（SemanticSearch）已把 RRFK<=0 归一化为默认值，
// 该分支实际不可达，此处仅防御未来新增调用点时出现除零（Inf/NaN 会污染整个排序）。
func rrfScore(weight float64, k, rank int) float32 {
	denom := k + rank
	if denom <= 0 {
		return 0
	}
	return float32(weight / float64(denom))
}

// LexicalBackend 抽象词法检索通道（生产实现：pkg/lexical.BM25 以 chunk 为索引单位）。
// 返回的 key 可为 chunk key 或概念指纹，由 ChunkKeyConcept 统一回溯。
type LexicalBackend interface {
	Search(query string, k int) []SemanticHit
}

// SearchOptions 控制语义检索行为。
type SearchOptions struct {
	// TopK 返回结果数（默认 10）。
	TopK int
	// SemanticTopK 语义召回候选数（默认 20）。
	SemanticTopK int
	// LexicalTopK 词法召回候选数（默认 20）。
	LexicalTopK int
	// CandidateFactor 块级召回放大系数（默认 4）。
	// 索引以 chunk 为单位，同一概念可能占用多个候选位；
	// 需按此系数放大底层召回量，回溯去重后才够填满 TopK 个概念。
	CandidateFactor int
	// RRFK 是 RRF 平滑常数（默认 DefaultRRFK）。
	RRFK int
	// VectorWeight 语义通道权重（默认 DefaultVectorWeight）。
	VectorWeight float64
	// LexicalWeight 词法通道权重（默认 DefaultLexicalWeight）。
	// 显式设为 0 时跳过词法检索，退化为纯语义通道。
	LexicalWeight float64
	// Lexical 是可选的 BM25 词法后端。为 nil 时回退到内置子串匹配通道。
	Lexical LexicalBackend

	// lexicalWeightSet / vectorWeightSet 记录调用方是否显式设置过对应权重，
	// 用于区分"未设置（用默认值）"与"显式设为 0（关闭该通道）"。
	lexicalWeightSet bool
	vectorWeightSet  bool
}

// WithLexicalWeight 返回显式设置词法权重后的选项副本（0 表示关闭词法通道）。
// 需要它是因为 Go 无法区分"字段为零值"和"调用方显式赋零"。
func (o SearchOptions) WithLexicalWeight(w float64) SearchOptions {
	o.LexicalWeight = w
	o.lexicalWeightSet = true
	return o
}

// WithVectorWeight 返回显式设置语义权重后的选项副本（0 表示关闭语义通道）。
// 与 WithLexicalWeight 对称：两个通道都应可被显式关闭，
// 否则"纯词法"这类配置无法表达（直接把字段设 0 会被当成未设置而回落默认值）。
func (o SearchOptions) WithVectorWeight(w float64) SearchOptions {
	o.VectorWeight = w
	o.vectorWeightSet = true
	return o
}

// SemanticSearch 将语义召回与词法召回经加权 RRF 融合后返回结果。
// backend 为 nil 时退化为纯词法；LexicalWeight 为 0 时退化为纯语义。
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
	if opts.CandidateFactor <= 0 {
		opts.CandidateFactor = 4
	}
	if opts.RRFK <= 0 {
		opts.RRFK = DefaultRRFK
	}
	if !opts.vectorWeightSet && opts.VectorWeight == 0 {
		opts.VectorWeight = DefaultVectorWeight
	}
	if opts.VectorWeight < 0 {
		opts.VectorWeight = 0 // 负权重无意义，等同关闭该通道
	}
	if !opts.lexicalWeightSet && opts.LexicalWeight == 0 {
		opts.LexicalWeight = DefaultLexicalWeight
	}
	if opts.LexicalWeight < 0 {
		opts.LexicalWeight = 0
	}

	// 指纹 → 概念 映射
	byFingerprint := make(map[string]*Concept, len(bundle.Concepts))
	for _, c := range bundle.Concepts {
		byFingerprint[Fingerprint(c)] = c
	}

	// 语义通道：索引以 chunk 为单位，需放大召回后回溯父概念。
	// 同一概念被多个 chunk 命中时分数累加（多块命中 = 更强证据），
	// rank 取最靠前的那个 chunk（用于同分 tie-break）。
	semRank := make(map[*Concept]int) // 1-based rank
	semHits := make(map[*Concept]int) // 命中块数（>1 表示多块命中）
	// 语义通道：权重为 0 时完全跳过（与词法通道对称，避免无谓的编码开销）。
	if backend != nil && opts.VectorWeight > 0 {
		vec, err := backend.EmbedQuery(text)
		if err != nil {
			return nil, fmt.Errorf("semantic embed query: %w", err)
		}
		// 候选窗口 = TopK * CandidateFactor。
		// 刻意以 TopK（而非 SemanticTopK）为基准并保持窄窗口：
		// 分数按命中块数累加，若窗口接近索引总块数，几乎所有块都会被召回，
		// 累加就退化成"按块数排名"——块最多的文档夺冠而与相关性无关
		// （实测：窗口 80 / 总块 86 时，21 块的 lint.md 压过真正相关的 cli.md）。
		want := opts.TopK * opts.CandidateFactor
		rank := 0
		for _, hit := range backend.Search(vec, want) {
			c, ok := byFingerprint[ChunkKeyConcept(hit.Key)]
			if !ok {
				continue
			}
			rank++
			semHits[c]++
			if _, seen := semRank[c]; !seen {
				semRank[c] = rank
			}
		}
	}

	// 词法通道：权重为 0 时完全跳过（避免无谓开销）。
	lexRank := make(map[*Concept]int) // 1-based rank
	lexHits := make(map[*Concept]int)
	if opts.LexicalWeight > 0 {
		if opts.Lexical != nil {
			// BM25 后端：以 chunk 为单位，同样需回溯父概念并累加。
			want := opts.TopK * opts.CandidateFactor
			rank := 0
			for _, hit := range opts.Lexical.Search(text, want) {
				c, ok := byFingerprint[ChunkKeyConcept(hit.Key)]
				if !ok {
					continue
				}
				rank++
				lexHits[c]++
				if _, seen := lexRank[c]; !seen {
					lexRank[c] = rank
				}
			}
		} else {
			// 回退：内置子串匹配（无相关性打分，仅作为无 BM25 索引时的兜底）
			lex := SearchWithMatches(bundle, text)
			if opts.LexicalTopK < len(lex) {
				lex = lex[:opts.LexicalTopK]
			}
			for i, r := range lex {
				if _, seen := lexRank[r.Concept]; !seen {
					lexRank[r.Concept] = i + 1
					lexHits[r.Concept] = 1
				}
			}
		}
	}

	// 加权 RRF 融合 + 来源标注。
	// 两侧均按命中块数累加：一个概念的多个 chunk 都命中时，证据更强。
	// 实测（docs/knowledge，13 条语义化查询）：累加 Recall@5 +8.3%，
	// 而只取最佳块（max-pooling）退化到 +0.0%，故采用累加。
	score := make(map[*Concept]float32)
	source := make(map[*Concept]string)
	for c, r := range semRank {
		hits := semHits[c]
		if hits < 1 {
			hits = 1
		}
		score[c] += rrfScore(opts.VectorWeight, opts.RRFK, r) * float32(hits)
		source[c] = "semantic"
	}
	for c, r := range lexRank {
		hits := lexHits[c]
		if hits < 1 {
			hits = 1
		}
		score[c] += rrfScore(opts.LexicalWeight, opts.RRFK, r) * float32(hits)
		switch source[c] {
		case "semantic":
			source[c] = "both"
		default:
			source[c] = "lexical"
		}
	}

	// 排序（降序）。同分时必须有确定的 tie-break，否则顺序取决于 map 遍历顺序，
	// 同一查询多次执行返回不同排列（不可复现）。
	// 依据：分数 > 语义 rank > 有无语义命中 > 来源(both>semantic>lexical) > 指纹兜底。
	keys := make([]*Concept, 0, len(score))
	for c := range score {
		keys = append(keys, c)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if score[a] != score[b] {
			return score[a] > score[b]
		}
		ra, aok := semRank[a]
		rb, bok := semRank[b]
		if aok && bok && ra != rb {
			return ra < rb
		}
		if aok != bok {
			return aok // 有语义命中的排前
		}
		if source[a] != source[b] {
			return source[a] > source[b] // both > semantic > lexical（字典序恰好满足）
		}
		return Fingerprint(a) < Fingerprint(b) // 确定性兜底
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
