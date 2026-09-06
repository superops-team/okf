package convert

// Chunking: pure-Go document markdown splitter with heading inheritance,
// CJK-aware word counting, and atomic handling of fenced code blocks and
// markdown table rows.
//
// Split precedence: paragraph > heading > sentence > word. Large documents are
// split into bounded chunks so each chunk fits the embedding model's token
// window (khoj-style); every chunk carries the inherited heading path so
// search results retain section context.

import (
	"strings"
	"unicode"
)

// defaultMaxWords bounds each chunk's indexed text (body + heading prefix).
const defaultMaxWords = 256

// maxHeadingPrefixLen bounds the heading prefix carried into a chunk
// (khoj keeps the last 100 chars of the heading).
const maxHeadingPrefixLen = 100

// Chunk is one bounded piece of a document with inherited heading context.
type Chunk struct {
	// Text is the chunk body. The heading prefix is NOT included.
	Text string
	// HeadingPath is the inherited heading path, e.g. "H1 > H2"
	// (empty when the document has no headings).
	HeadingPath string
	// Index is the 0-based chunk index within the document.
	Index int
}

// ChunkOptions control splitting. Zero value uses defaults.
type ChunkOptions struct {
	// MaxWords bounds the indexed words of each chunk
	// (body + heading prefix; default 256).
	MaxWords int
}

// Split splits document markdown into bounded chunks. It is deterministic,
// pure, and returns nil for empty input.
func Split(markdown string, opts *ChunkOptions) []Chunk {
	if markdown == "" {
		return nil
	}
	maxWords := defaultMaxWords
	if opts != nil && opts.MaxWords > 0 {
		maxWords = opts.MaxWords
	}

	var chunks []Chunk
	var cur []string
	var curWords int
	var headingStack []string

	flush := func() {
		if len(cur) == 0 {
			return
		}
		chunks = append(chunks, Chunk{
			Text:        strings.TrimSpace(strings.Join(cur, "\n\n")),
			HeadingPath: strings.Join(headingStack, " > "),
			Index:       len(chunks),
		})
		cur = nil
		curWords = 0
	}

	for _, seg := range splitSegments(markdown) {
		switch seg.kind {
		case segHeading:
			// A heading opens a new chunk context (split priority 2).
			if len(cur) > 0 {
				flush()
			}
			headingStack = append(headingStack, seg.text)
		case segFence, segTable:
			// Atomic blocks: never split inside; flush only if adding would
			// overflow the budget AND the current chunk is non-empty. An
			// atomic block itself may exceed MaxWords (atomicity wins).
			w := countWords(seg.text)
			if len(cur) > 0 && curWords+w > bodyBudget(headingStack, maxWords) {
				flush()
			}
			cur = append(cur, seg.text)
			curWords += w
		default: // segParagraph
			pieces := []string{seg.text}
			budget := bodyBudget(headingStack, maxWords)
			if countWords(seg.text) > budget {
				pieces = splitLongText(seg.text, budget)
			}
			for _, p := range pieces {
				w := countWords(p)
				if len(cur) > 0 && curWords+w > budget {
					flush()
					budget = bodyBudget(headingStack, maxWords)
				}
				cur = append(cur, p)
				curWords += w
			}
		}
	}
	flush()
	return chunks
}

// bodyBudget returns the word budget available for body text of a chunk under
// the current heading stack: MaxWords minus the heading prefix words. The
// prefix is bounded to maxHeadingPrefixLen before counting.
func bodyBudget(headingStack []string, maxWords int) int {
	b := maxWords - headingPrefixWords(headingStack)
	if b < 1 {
		return 1
	}
	return b
}

// headingPrefixWords counts the heading-path words the way the budget reserves
// them: one count per heading level (the ">" separator is not a word), with the
// 100-char bound applied to the joined display path first.
func headingPrefixWords(stack []string) int {
	if len(stack) == 0 {
		return 0
	}
	joined := strings.Join(stack, " > ")
	if len(joined) > maxHeadingPrefixLen {
		joined = joined[len(joined)-maxHeadingPrefixLen:]
	}
	total := 0
	for _, lvl := range strings.Split(joined, " > ") {
		total += countWords(lvl)
	}
	return total
}

// countWords counts words CJK-aware: whitespace-separated tokens count as
// words (English and other scripts); each CJK character counts as one word, so
// a Chinese paragraph with no whitespace splits correctly.
func countWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if isCJK(r) {
			count++
			inWord = false
			continue
		}
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}

// isCJK reports whether r is a CJK character (Han, Kana, Hangul).
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Ext A
		(r >= 0x3040 && r <= 0x30FF) || // Hiragana + Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Hangul Syllables
}

// segKind classifies a markdown segment for splitting.
type segKind int

const (
	segParagraph segKind = iota
	segHeading
	segFence
	segTable
)

type segment struct {
	kind segKind
	text string
}

// splitSegments slices markdown into atomic-ish segments: headings, fenced
// code blocks (whole), table row groups (whole), and paragraphs (runs of
// non-blank lines). Fence/table/heading boundaries never split inside.
func splitSegments(markdown string) []segment {
	lines := strings.Split(markdown, "\n")
	var segs []segment
	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Fenced code block: consume until the closing fence.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			var buf []string
			for i < len(lines) {
				buf = append(buf, lines[i])
				lt := strings.TrimSpace(lines[i])
				i++
				if len(buf) > 1 && (strings.HasPrefix(lt, "```") || strings.HasPrefix(lt, "~~~")) {
					break
				}
			}
			segs = append(segs, segment{kind: segFence, text: strings.Join(buf, "\n")})
			continue
		}

		if isHeading(line) {
			segs = append(segs, segment{kind: segHeading, text: headingText(line)})
			i++
			continue
		}

		// Table: consecutive table lines are atomic together.
		if isTableLine(line) {
			var buf []string
			for i < len(lines) && isTableLine(lines[i]) {
				buf = append(buf, lines[i])
				i++
			}
			segs = append(segs, segment{kind: segTable, text: strings.Join(buf, "\n")})
			continue
		}

		// Paragraph: consume until blank line or a structural boundary.
		var buf []string
		for i < len(lines) {
			l := lines[i]
			lt := strings.TrimSpace(l)
			if lt == "" || isHeading(l) || isTableLine(l) ||
				strings.HasPrefix(lt, "```") || strings.HasPrefix(lt, "~~~") {
				break
			}
			buf = append(buf, l)
			i++
		}
		if len(buf) > 0 {
			segs = append(segs, segment{kind: segParagraph, text: strings.Join(buf, "\n")})
		} else {
			i++ // blank line
		}
	}
	return segs
}

// isHeading reports whether line is an ATX heading (`#{1,6} `).
func isHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	for i := 0; i < len(trimmed) && i < 6; i++ {
		if trimmed[i] != '#' {
			return i > 0 && i < len(trimmed) && trimmed[i] == ' '
		}
		if i == len(trimmed)-1 {
			return false // bare "####" with no text
		}
	}
	return false
}

// headingText strips the leading `#`s and space from an ATX heading.
func headingText(line string) string {
	t := strings.TrimSpace(line)
	t = strings.TrimLeft(t, "#")
	return strings.TrimSpace(t)
}

// isTableLine reports whether line looks like a markdown table row
// (`| ... |`), including the separator row `|---|---|`.
func isTableLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "|")
}

// splitLongText splits an over-budget paragraph at sentence/word boundaries
// (priority: newline > CJK/English sentence enders > space > tab), keeping
// separators, into pieces of at most maxWords words.
func splitLongText(text string, maxWords int) []string {
	if maxWords < 1 {
		maxWords = 1
	}
	var pieces []string
	var cur strings.Builder
	var curWords int
	for _, tok := range tokenize(text) {
		if isSeparatorToken(tok) {
			cur.WriteString(tok)
			continue
		}
		if curWords >= maxWords && cur.Len() > 0 {
			pieces = append(pieces, strings.TrimSpace(cur.String()))
			cur.Reset()
			curWords = 0
		}
		cur.WriteString(tok)
		curWords++
	}
	if strings.TrimSpace(cur.String()) != "" {
		pieces = append(pieces, strings.TrimSpace(cur.String()))
	}
	return pieces
}

// tokenize slices text into word tokens and separator tokens (separators kept).
func tokenize(text string) []string {
	var toks []string
	var cur strings.Builder
	for _, r := range text {
		if isSeparatorRune(r) {
			if cur.Len() > 0 {
				toks = append(toks, cur.String())
				cur.Reset()
			}
			toks = append(toks, string(r))
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		toks = append(toks, cur.String())
	}
	return toks
}

func isSeparatorRune(r rune) bool {
	switch r {
	case '\n', '。', '！', '？', '!', '?', '.', ' ', '\t':
		return true
	}
	return false
}

func isSeparatorToken(tok string) bool {
	if len(tok) != 1 {
		return false
	}
	return isSeparatorRune(rune(tok[0]))
}
