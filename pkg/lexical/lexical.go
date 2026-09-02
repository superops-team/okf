// Package lexical 提供面向代码知识库的分词与 BM25 词法检索。
//
// 背景：改动前的词法通道是整串子串匹配（containsFold），不分词、不打分：
// 多词查询与中文查询实测 Recall 为 0（"vector index rebuild" 返回 No results found），
// 且返回顺序取决于概念遍历顺序，作为 RRF 的输入等同噪声。
//
// 设计要点：
//   - 分词对拉丁段同时产出整词与标识符子词（okf_semantic_search → 整词 + okf/semantic/search），
//     使代码符号既能整体匹配也能按子词命中；
//   - CJK 段产出重叠 bigram，无需词典、零维护（jieba 等词典方案会引入依赖与 OOV 问题）；
//   - BM25 使用 IR 领域标准参数 k1=1.2、b=0.75；
//   - 检索结果同分时按 key 升序 tie-break，保证确定可复现。
//
// 本包为纯标准库实现，不引入第三方分词或检索依赖。
package lexical

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// BM25 标准参数：k1 控制词频饱和，b 控制文档长度归一化强度。
const (
	paramK1 = 1.2
	paramB  = 0.75
)

// isCJK 判断是否为中日韩文字（含假名、谚文）。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// isWordRune 判断是否为拉丁词的组成字符（字母、数字、下划线）。
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// splitIdentifier 把 snake_case / kebab-case / dotted / camelCase / PascalCase
// 拆成子词。例："okf_semantic_search" → [okf semantic search]；
// "HTTPServer" → [http server]（连续大写后接小写视为缩写边界）。
func splitIdentifier(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	var out []string
	for _, p := range parts {
		rs := []rune(p)
		start := 0
		for i := 1; i < len(rs); i++ {
			prev, cur := rs[i-1], rs[i]
			// camelCase 边界：小写/数字 后接 大写
			lowerToUpper := (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(cur)
			// 缩写结束：连续大写后紧跟小写（HTTPServer → HTTP | Server）
			acronymEnd := i+1 < len(rs) &&
				unicode.IsUpper(prev) && unicode.IsUpper(cur) && unicode.IsLower(rs[i+1])
			if lowerToUpper || acronymEnd {
				if seg := strings.ToLower(string(rs[start:i])); seg != "" {
					out = append(out, seg)
				}
				start = i
			}
		}
		if seg := strings.ToLower(string(rs[start:])); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// Tokenize 把文本切成检索用 token。
//
// 规则：
//   - 拉丁/数字连续段 → 小写整词，若可拆分则额外产出子词（保留整词）；
//   - CJK 连续段 → 重叠 bigram（单字则 unigram）；
//   - 其他字符作为分隔符丢弃。
//
// 结果不含空字符串；对同一输入结果稳定。
func Tokenize(s string) []string {
	var out []string
	var latin, cjk []rune

	flushLatin := func() {
		if len(latin) == 0 {
			return
		}
		whole := strings.ToLower(string(latin))
		if whole != "" {
			out = append(out, whole)
		}
		if subs := splitIdentifier(string(latin)); len(subs) > 1 || (len(subs) == 1 && subs[0] != whole) {
			for _, sw := range subs {
				if sw != "" && sw != whole {
					out = append(out, sw)
				}
			}
		}
		latin = latin[:0]
	}
	flushCJK := func() {
		switch {
		case len(cjk) == 0:
		case len(cjk) == 1:
			out = append(out, string(cjk))
		default:
			for i := 0; i+1 < len(cjk); i++ {
				out = append(out, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range s {
		switch {
		case isCJK(r):
			flushLatin()
			cjk = append(cjk, r)
		case isWordRune(r):
			flushCJK()
			latin = append(latin, r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return out
}

// Hit 是一条词法检索命中。
type Hit struct {
	Key   string
	Score float64
}

// BM25 是内存中的 BM25 索引。使用流程：NewBM25 → 多次 Add → Finalize → Search。
// 非并发安全：构建完成后只读使用（okf 的索引在单次命令内构建）。
type BM25 struct {
	keys  []string
	tfs   []map[string]int
	lens  []float64
	df    map[string]int
	avgdl float64
	ready bool
}

// NewBM25 创建空索引。
func NewBM25() *BM25 {
	return &BM25{df: make(map[string]int)}
}

// Add 加入一个文档（key 为 chunk key）。重复 key 不做去重，由调用方保证唯一。
func (b *BM25) Add(key, text string) {
	toks := Tokenize(text)
	tf := make(map[string]int, len(toks))
	for _, t := range toks {
		tf[t]++
	}
	for t := range tf {
		b.df[t]++
	}
	b.keys = append(b.keys, key)
	b.tfs = append(b.tfs, tf)
	b.lens = append(b.lens, float64(len(toks)))
	b.ready = false
}

// Finalize 计算平均文档长度。必须在 Add 全部完成后、Search 之前调用一次。
func (b *BM25) Finalize() {
	var sum float64
	for _, l := range b.lens {
		sum += l
	}
	if len(b.lens) > 0 {
		b.avgdl = sum / float64(len(b.lens))
	}
	b.ready = true
}

// Len 返回索引中的文档数。
func (b *BM25) Len() int { return len(b.keys) }

// Search 返回 BM25 分数最高的 k 条命中（降序；同分按 key 升序，保证确定性）。
// 未命中任何词项的文档不返回。
func (b *BM25) Search(query string, k int) []Hit {
	if k <= 0 || len(b.keys) == 0 {
		return nil
	}
	if !b.ready {
		b.Finalize() // 容错：调用方漏调 Finalize 时不至于返回错误分数
	}
	qt := Tokenize(query)
	if len(qt) == 0 {
		return nil
	}
	// 查询词去重：同一词重复出现不应线性放大分数
	seen := make(map[string]struct{}, len(qt))
	terms := make([]string, 0, len(qt))
	for _, t := range qt {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		terms = append(terms, t)
	}

	n := float64(len(b.keys))
	hits := make([]Hit, 0, len(b.keys))
	for i := range b.keys {
		var score float64
		for _, t := range terms {
			f := float64(b.tfs[i][t])
			if f == 0 {
				continue
			}
			dfT := float64(b.df[t])
			// Lucene 风格 IDF：加 1 避免高频词得到负权重
			idf := math.Log(1 + (n-dfT+0.5)/(dfT+0.5))
			norm := f + paramK1*(1-paramB+paramB*b.lens[i]/b.avgdl)
			score += idf * f * (paramK1 + 1) / norm
		}
		if score > 0 {
			hits = append(hits, Hit{Key: b.keys[i], Score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Key < hits[j].Key
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits
}
