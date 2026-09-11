package convert

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
)

// --- countWords: CJK-aware counting ---

func TestCountWords_English(t *testing.T) {
	if got := countWords("hello world foo bar"); got != 4 {
		t.Fatalf("countWords(english) = %d, want 4", got)
	}
}

func TestCountWords_ChineseOnly(t *testing.T) {
	// 10 CJK chars, no whitespace: must count per rune, not as one word.
	if got := countWords("这是一个非常长的中文句子没有空格"); got != 16 {
		t.Fatalf("countWords(chinese) = %d, want 16", got)
	}
}

func TestCountWords_Mixed(t *testing.T) {
	// "okf 支持 中文" -> 2 english tokens + 2 CJK chars = 4.
	if got := countWords("okf 支持 中文搜索"); got != 7 {
		t.Fatalf("countWords(mixed) = %d, want 7", got)
	}
}

func TestCountWords_PunctuationOnly(t *testing.T) {
	if got := countWords("!!! --- ???"); got != 0 {
		t.Fatalf("countWords(punct) = %d, want 0", got)
	}
}

func TestCountWords_Empty(t *testing.T) {
	if got := countWords(""); got != 0 {
		t.Fatalf("countWords(empty) = %d, want 0", got)
	}
}

// --- Chunk: boundaries, heading inheritance, atomic blocks ---

func TestChunk_MultiParagraph(t *testing.T) {
	// 3 paragraphs * 200 words = 600 words > default 256 -> multiple chunks.
	var sb strings.Builder
	for i := 0; i < 3; i++ {
		for j := 0; j < 200; j++ {
			sb.WriteString("word ")
		}
		sb.WriteString("\n\n")
	}
	chunks := Split(sb.String(), nil)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if n := countWords(c.Text) + prefixWords(c.HeadingPath); n > defaultMaxWords {
			t.Fatalf("chunk %d exceeds MaxWords: %d words", i, n)
		}
	}
}

func TestChunk_ParagraphBoundary(t *testing.T) {
	md := "para one\n\npara two\n\npara three"
	chunks := Split(md, &ChunkOptions{MaxWords: 2})
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %+v", len(chunks), chunks)
	}
}

func TestChunk_HeadingInheritance(t *testing.T) {
	md := "# H1\n\nbody a\n\n## H2\n\nbody b"
	chunks := Split(md, &ChunkOptions{MaxWords: 4})
	// Find the chunk containing "body b".
	var bChunk *Chunk
	for i := range chunks {
		if strings.Contains(chunks[i].Text, "body b") {
			bChunk = &chunks[i]
		}
	}
	if bChunk == nil {
		t.Fatal("chunk containing body b not found")
	}
	if bChunk.HeadingPath != "H1 > H2" {
		t.Fatalf("HeadingPath = %q, want %q", bChunk.HeadingPath, "H1 > H2")
	}
}

func TestChunk_FirstChunkHeadingPath(t *testing.T) {
	md := "# H1\n\nbody a\n\n## H2\n\nbody b"
	chunks := Split(md, &ChunkOptions{MaxWords: 10})
	if len(chunks) == 0 {
		t.Fatal("no chunks returned")
	}
	if chunks[0].HeadingPath != "H1" {
		t.Fatalf("first chunk HeadingPath = %q, want %q", chunks[0].HeadingPath, "H1")
	}
}

func TestChunk_HeadingPrefixCountsAgainstBudget(t *testing.T) {
	// Long heading chain (100+ chars) + MaxWords=32: every chunk must stay
	// within the budget including the heading prefix.
	var sb strings.Builder
	for i := 0; i < 6; i++ {
		sb.WriteString("# ")
		sb.WriteString(strings.Repeat("Heading", 6)) // 42 chars each
		sb.WriteString("\n\n")
		for j := 0; j < 40; j++ {
			sb.WriteString("word ")
		}
		sb.WriteString("\n\n")
	}
	chunks := Split(sb.String(), &ChunkOptions{MaxWords: 32})
	for i, c := range chunks {
		if n := countWords(c.Text) + prefixWords(c.HeadingPath); n > 32 {
			t.Fatalf("chunk %d exceeds budget: %d words (text=%q, path=%q)", i, n, c.Text, c.HeadingPath)
		}
	}
}

func TestChunk_FencedCodeAtomic(t *testing.T) {
	md := "intro\n\n```\n" + strings.Repeat("code word ", 300) + "\n```\n\noutro"
	chunks := Split(md, &ChunkOptions{MaxWords: 32})
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1].Text
		cur := chunks[i].Text
		// A boundary is legal only at the fence edges: if the previous chunk
		// contains the opening fence but not the closing one, it must contain
		// the whole block; we assert no chunk starts/ends inside the fence by
		// checking backtick parity inside each chunk is never "open".
		opens := strings.Count(cur, "```")
		if opens%2 == 1 {
			t.Fatalf("chunk %d has unbalanced fence markers -> boundary inside code block", i)
		}
		if opens == 0 && strings.Contains(cur, "code word") && !strings.Contains(cur, "```") {
			t.Fatalf("chunk %d contains code content without fence markers", i)
		}
		_ = prev
	}
}

func TestChunk_TableRowsAtomic(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("| A | B |\n")
	sb.WriteString("|---|---|\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("| row ")
		sb.WriteString(string(rune('a' + i%26)))
		sb.WriteString(" | value |\n")
	}
	chunks := Split(sb.String(), &ChunkOptions{MaxWords: 5})
	for _, c := range chunks {
		// A chunk that contains any "| ... |" line must not begin or end
		// mid-row: each line in the chunk that starts with "|" must end with
		// "|" too (whole rows only).
		for _, line := range strings.Split(c.Text, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "|") && !strings.HasSuffix(line, "|") {
				t.Fatalf("table row split mid-line: %q", line)
			}
		}
	}
}

func TestChunk_EmptyInput(t *testing.T) {
	chunks := Split("", nil)
	if len(chunks) != 0 {
		t.Fatalf("expected empty slice, got %d chunks", len(chunks))
	}
}

func TestChunk_Deterministic(t *testing.T) {
	md := "# T\n\n" + strings.Repeat("body text ", 300) + "\n\n## S\n\n" + strings.Repeat("more words ", 300)
	first := Split(md, nil)
	second := Split(md, nil)
	if len(first) != len(second) {
		t.Fatalf("determinism broken: %d vs %d chunks", len(first), len(second))
	}
	for i := range first {
		if first[i].Text != second[i].Text || first[i].HeadingPath != second[i].HeadingPath || first[i].Index != second[i].Index {
			t.Fatalf("chunk %d differs between runs: %+v vs %+v", i, first[i], second[i])
		}
	}
}

func TestChunk_IndicesSequential(t *testing.T) {
	md := strings.Repeat("word ", 600)
	chunks := Split(md, nil)
	for i, c := range chunks {
		if c.Index != i {
			t.Fatalf("chunk %d has Index %d, want %d", i, c.Index, i)
		}
	}
}

// TestChunk_WordBudgetProperty runs the invariant
// words(Text) + words(HeadingPath) <= MaxWords over random documents,
// including CJK fixtures.
func TestChunk_WordBudgetProperty(t *testing.T) {
	prop := func(seed uint32) bool {
		rng := rand.New(rand.NewSource(int64(seed)))
		md := randomMarkdown(rng)
		opts := &ChunkOptions{MaxWords: 32}
		chunks := Split(md, opts)
		for i, c := range chunks {
			if n := countWords(c.Text) + prefixWords(c.HeadingPath); n > 32 {
				t.Logf("seed=%d chunk %d exceeds budget: %d words (text=%q path=%q)", seed, i, n, c.Text, c.HeadingPath)
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

// randomMarkdown builds a deterministic pseudo-random markdown doc mixing
// English words, CJK runs, headings, and code fences.
func randomMarkdown(rng *rand.Rand) string {
	var sb strings.Builder
	enWords := []string{"alpha", "beta", "gamma", "delta", "epsilon", "okf", "search", "retrieval", "chunk", "index"}
	cjkChars := []rune{'中', '文', '检', '索', '分', '块', '测', '试', '内', '容', '标', '题'}
	for i := 0; i < 6; i++ {
		switch rng.Intn(3) {
		case 0:
			sb.WriteString("# 标题 ")
			sb.WriteString(strings.Repeat("Heading", 2+rng.Intn(4)))
			sb.WriteString("\n\n")
		case 1:
			for j := 0; j < 10+rng.Intn(40); j++ {
				sb.WriteString(enWords[rng.Intn(len(enWords))])
				sb.WriteString(" ")
			}
			sb.WriteString("\n\n")
		case 2:
			for j := 0; j < 5+rng.Intn(40); j++ {
				sb.WriteRune(cjkChars[rng.Intn(len(cjkChars))])
			}
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

// prefixWords counts heading-path words the way the budget reserves them:
// one count per heading level (the ">" separator is NOT a word), with the
// 100-char bound applied to the joined display path first — mirrors the
// production headingPrefixWords.
func prefixWords(path string) int {
	if path == "" {
		return 0
	}
	joined := path
	if len(joined) > 100 {
		joined = joined[len(joined)-100:]
	}
	total := 0
	for _, lvl := range strings.Split(joined, " > ") {
		total += countWords(lvl)
	}
	return total
}
