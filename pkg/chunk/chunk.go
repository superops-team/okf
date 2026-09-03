// Package chunk 把概念内容按 Markdown 标题切分为可嵌入的片段（chunk）。
//
// 背景：语义索引使用的 MiniLM 上限为 256 token（约 1024 英文字符），
// 若把整篇概念拼成一个字符串编码，超限内容会被 tokenizer 静默截断
// （实测本仓库知识库丢失 68.7% 内容）。按标题切分后逐块编码可消除该丢失。
//
// 设计要点：
//   - 纯函数：无 I/O、无全局状态，相同输入产生相同输出，便于测试与属性验证；
//   - breadcrumb（标题路径）写入编码文本，保留层级语义；
//   - 代码围栏内的 # 不视为标题，避免注释被误判为分节；
//   - 超长块按空行段落二次切分，仍超限则硬切分，保证任何输入都不超上限；
//   - 过小的相邻同 breadcrumb 块合并，避免碎片稀释检索信号。
package chunk

import (
	"strings"
)

// 默认参数。MaxChars 对齐 MiniLM 的 256 token 上限（英文约 4 字符/token）。
const (
	DefaultMaxChars = 1024
	DefaultMinChars = 120
)

// 参与切分的标题层级：H1 视为概念标题本身，H5+ 过细不切分。
const (
	minSplitLevel = 2
	maxSplitLevel = 4
)

// Chunk 是一个可嵌入片段。
type Chunk struct {
	// Breadcrumb 是标题路径，如 "CLI > Commands > search"。
	Breadcrumb string
	// Body 是该片段的正文（不含标题行）。
	Body string
	// Ordinal 是片段在概念内的序号（从 0 起，按文档顺序）。
	Ordinal int
}

// Text 返回用于 embedding / 词法索引的完整文本（breadcrumb + 正文）。
func (c Chunk) Text() string {
	if c.Breadcrumb == "" {
		return c.Body
	}
	if c.Body == "" {
		return c.Breadcrumb
	}
	return c.Breadcrumb + "\n" + c.Body
}

// Options 控制分块行为；零值字段使用默认值。
type Options struct {
	// MaxChars 是单个 chunk 的 Text() 长度上限。
	MaxChars int
	// MinChars 低于此长度的块会与相邻同 breadcrumb 块合并。
	MinChars int
}

func (o Options) normalized() Options {
	if o.MaxChars <= 0 {
		o.MaxChars = DefaultMaxChars
	}
	if o.MinChars < 0 {
		o.MinChars = 0
	}
	if o.MinChars == 0 {
		o.MinChars = DefaultMinChars
	}
	// MinChars 不得超过 MaxChars，否则合并逻辑会无条件吞并
	if o.MinChars >= o.MaxChars {
		o.MinChars = o.MaxChars / 2
	}
	return o
}

// isFenceLine 判断是否为代码围栏行（``` 或 ~~~）。
func isFenceLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// parseHeading 解析 ATX 标题，返回层级与标题文本。
// 仅当 # 后紧跟空格或行尾时才视为标题（符合 CommonMark，避免误判 #tag）。
func parseHeading(line string) (level int, text string, ok bool) {
	if !strings.HasPrefix(line, "#") {
		return 0, "", false
	}
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i > 6 {
		return 0, "", false
	}
	rest := line[i:]
	if rest != "" && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
		return 0, "", false // 如 "#tag"，不是标题
	}
	return i, strings.TrimSpace(rest), true
}

// Split 把概念内容切分为 chunk 列表。title 作为 breadcrumb 的根。
// 输入为空或纯空白时返回 nil。
func Split(title, content string, opts Options) []Chunk {
	opts = opts.normalized()
	if strings.TrimSpace(content) == "" {
		return nil
	}

	var out []Chunk
	path := []string{title}
	var buf []string
	inFence := false

	// flush 把当前缓冲区按需切分后追加到结果。
	flush := func() {
		body := strings.TrimSpace(strings.Join(buf, "\n"))
		buf = buf[:0]
		if body == "" {
			return
		}
		bc := strings.Join(path, " > ")
		pieces, bc := fitPieces(bc, body, opts.MaxChars)
		for _, piece := range pieces {
			out = append(out, Chunk{Breadcrumb: bc, Body: piece})
		}
	}

	for _, line := range strings.Split(content, "\n") {
		if isFenceLine(line) {
			inFence = !inFence
			buf = append(buf, line)
			continue
		}
		if !inFence {
			if lv, h, ok := parseHeading(line); ok && lv >= minSplitLevel && lv <= maxSplitLevel {
				flush()
				// 回退到父层级再压入当前标题：lv=2 → path[:1]，lv=3 → path[:2]
				if lv-1 <= len(path) {
					path = path[:lv-1]
				}
				path = append(path, h)
				continue
			}
		}
		buf = append(buf, line)
	}
	flush()

	out = mergeTiny(out, opts)
	for i := range out {
		out[i].Ordinal = i
	}
	return out
}

// minBodyBudget 是 breadcrumb 过长时保留给正文的最小预算。
// breadcrumb 会被截断以让出空间，保证 Text() 始终满足 MaxChars 约束。
const minBodyBudget = 16

// fitPieces 把 body 切成若干片，保证 len(breadcrumb)+len(piece)+1 不超过 max。
// 优先按空行段落切；单段仍超限则按行切；单行仍超限则硬切。
//
// 返回值第二项是（可能被截断后的）breadcrumb：当 breadcrumb 长到挤占全部预算时，
// 必须截断它而不是放弃长度约束——否则 Text() 会超出 MaxChars，
// 下游 embedding 的 token 上限假设随之失效（实测超长标题会让每个块都超限）。
func fitPieces(breadcrumb, body string, max int) ([]string, string) {
	budget := max - len(breadcrumb)
	if breadcrumb != "" {
		budget-- // Text() 中的换行符
	}
	if budget < minBodyBudget {
		// breadcrumb 挤占了正文预算：按 rune 边界截断，给正文留出 minBodyBudget
		keep := max - minBodyBudget - 1
		if keep < 0 {
			keep = 0
		}
		breadcrumb = truncateRunes(breadcrumb, keep)
		budget = max - len(breadcrumb)
		if breadcrumb != "" {
			budget--
		}
		if budget <= 0 {
			budget = 1
		}
	}
	if len(body) <= budget {
		return []string{body}, breadcrumb
	}

	var pieces []string
	var cur []string
	curLen := 0
	appendCur := func() {
		if len(cur) > 0 {
			pieces = append(pieces, strings.Join(cur, "\n\n"))
			cur = cur[:0]
			curLen = 0
		}
	}
	for _, para := range strings.Split(body, "\n\n") {
		if strings.TrimSpace(para) == "" {
			continue
		}
		// 段落自身超限：先冲刷已累积内容，再对该段按行/硬切
		if len(para) > budget {
			appendCur()
			pieces = append(pieces, hardSplit(para, budget)...)
			continue
		}
		sep := 0
		if curLen > 0 {
			sep = 2 // "\n\n"
		}
		if curLen+sep+len(para) > budget {
			appendCur()
		}
		if curLen > 0 {
			curLen += 2
		}
		cur = append(cur, para)
		curLen += len(para)
	}
	appendCur()
	return pieces, breadcrumb
}

// truncateRunes 把 s 截断到不超过 n 字节，且不切裂 UTF-8 字符。
// 返回值满足 len(result) <= n。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	// range string 的下标是每个 rune 的起始字节位置；
	// 第一个起始位置 > n 的 rune 意味着它无法完整放入，
	// 故截到该位置即为满足 <= n 的最长前缀。
	for i := range s {
		if i > n {
			return s[:prevRuneStart(s, i)]
		}
	}
	// 所有 rune 起始位置都 <= n，但 len(s) > n：
	// 说明最后一个 rune 跨过了 n，需要去掉它。
	return s[:prevRuneStart(s, len(s))]
}

// prevRuneStart 返回 pos 之前最后一个 rune 的起始位置（pos 本身为 rune 边界）。
func prevRuneStart(s string, pos int) int {
	prev := 0
	for i := range s {
		if i >= pos {
			break
		}
		prev = i
	}
	return prev
}

// hardSplit 把超限文本按行累积切分；单行仍超限则按字符边界硬切（不切裂 UTF-8）。
func hardSplit(s string, budget int) []string {
	var out []string
	var cur []string
	curLen := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, "\n"))
			cur = cur[:0]
			curLen = 0
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if len(line) > budget {
			flush()
			out = append(out, splitRunes(line, budget)...)
			continue
		}
		sep := 0
		if curLen > 0 {
			sep = 1
		}
		if curLen+sep+len(line) > budget {
			flush()
		}
		if curLen > 0 {
			curLen++
		}
		cur = append(cur, line)
		curLen += len(line)
	}
	flush()
	return out
}

// splitRunes 按 rune 边界把 s 切成不超过 budget 字节的片段，避免切裂多字节字符。
func splitRunes(s string, budget int) []string {
	var out []string
	var b strings.Builder
	for _, r := range s {
		if b.Len()+len(string(r)) > budget && b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// mergeTiny 合并过小的相邻同 breadcrumb 块，避免碎片稀释检索信号。
func mergeTiny(in []Chunk, opts Options) []Chunk {
	if len(in) == 0 {
		return in
	}
	out := make([]Chunk, 0, len(in))
	for _, c := range in {
		if len(out) > 0 {
			last := &out[len(out)-1]
			merged := last.Body + "\n\n" + c.Body
			if len(last.Body) < opts.MinChars && last.Breadcrumb == c.Breadcrumb &&
				len(last.Breadcrumb)+len(merged)+1 <= opts.MaxChars {
				last.Body = merged
				continue
			}
		}
		out = append(out, c)
	}
	return out
}
