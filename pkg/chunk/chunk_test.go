package chunk

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode/utf8"
)

// helper：返回所有 chunk 的 breadcrumb 列表
func breadcrumbs(cs []Chunk) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Breadcrumb
	}
	return out
}

func TestSplitByHeadingBuildsBreadcrumb(t *testing.T) {
	content := `intro text

## Commands

cmd overview

### search

search details
`
	got := Split("CLI", content, Options{})
	if len(got) < 3 {
		t.Fatalf("chunk 数 = %d，期望 ≥3；breadcrumbs=%v", len(got), breadcrumbs(got))
	}
	// 首块为标题前正文，breadcrumb 只有概念标题
	if got[0].Breadcrumb != "CLI" {
		t.Errorf("首块 breadcrumb = %q，期望 %q", got[0].Breadcrumb, "CLI")
	}
	var found bool
	for _, c := range got {
		if c.Breadcrumb == "CLI > Commands > search" {
			found = true
			if !strings.Contains(c.Body, "search details") {
				t.Errorf("breadcrumb 匹配但正文不含 search details: %q", c.Body)
			}
		}
	}
	if !found {
		t.Errorf("未找到 breadcrumb %q，实际=%v", "CLI > Commands > search", breadcrumbs(got))
	}
}

func TestBreadcrumbIncludedInText(t *testing.T) {
	got := Split("Doc", "## Section\n\nbody\n", Options{})
	var target *Chunk
	for i := range got {
		if got[i].Breadcrumb == "Doc > Section" {
			target = &got[i]
		}
	}
	if target == nil {
		t.Fatalf("未找到目标块，breadcrumbs=%v", breadcrumbs(got))
	}
	text := target.Text()
	if !strings.Contains(text, "Doc > Section") {
		t.Errorf("Text() 未包含 breadcrumb: %q", text)
	}
	if !strings.Contains(text, "body") {
		t.Errorf("Text() 未包含正文: %q", text)
	}
}

func TestH1DoesNotSplit(t *testing.T) {
	content := `# Top Title

para one

# Another H1

para two
`
	got := Split("Concept", content, Options{})
	if len(got) != 1 {
		t.Fatalf("H1 不应触发切分，chunk 数 = %d，breadcrumbs=%v", len(got), breadcrumbs(got))
	}
	if got[0].Breadcrumb != "Concept" {
		t.Errorf("breadcrumb = %q，期望 %q", got[0].Breadcrumb, "Concept")
	}
}

func TestCodeFenceHashNotTreatedAsHeading(t *testing.T) {
	content := "## Real Section\n\n" +
		"```bash\n" +
		"## not a heading\n" +
		"echo hi\n" +
		"### also not\n" +
		"```\n\n" +
		"after code\n"
	got := Split("Doc", content, Options{})
	for _, c := range got {
		if strings.Contains(c.Breadcrumb, "not a heading") || strings.Contains(c.Breadcrumb, "also not") {
			t.Fatalf("代码围栏内的 # 被当作标题：breadcrumbs=%v", breadcrumbs(got))
		}
	}
	// 代码块必须完整保留在同一个 chunk 内
	var hasFullCode bool
	for _, c := range got {
		if strings.Contains(c.Body, "## not a heading") && strings.Contains(c.Body, "### also not") {
			hasFullCode = true
		}
	}
	if !hasFullCode {
		t.Errorf("代码块被拆开，breadcrumbs=%v", breadcrumbs(got))
	}
}

func TestChunkNeverExceedsMaxChars(t *testing.T) {
	// 构造超长单节内容
	var sb strings.Builder
	sb.WriteString("## Big\n\n")
	for i := 0; i < 40; i++ {
		sb.WriteString(strings.Repeat("word ", 40))
		sb.WriteString("\n\n")
	}
	opts := Options{MaxChars: 500}
	got := Split("Doc", sb.String(), opts)
	if len(got) < 2 {
		t.Fatalf("超长内容应被切分，chunk 数 = %d", len(got))
	}
	for i, c := range got {
		if n := len(c.Text()); n > opts.MaxChars {
			t.Errorf("chunk %d 长度 %d 超过上限 %d", i, n, opts.MaxChars)
		}
	}
}

func TestNoHeadingLongContentStillSplits(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString(strings.Repeat("alpha ", 30))
		sb.WriteString("\n\n")
	}
	opts := Options{MaxChars: 400}
	got := Split("Doc", sb.String(), opts)
	if len(got) < 2 {
		t.Fatalf("无标题长文应按段落兜底切分，chunk 数 = %d", len(got))
	}
	for i, c := range got {
		if n := len(c.Text()); n > opts.MaxChars {
			t.Errorf("chunk %d 长度 %d 超过上限 %d", i, n, opts.MaxChars)
		}
	}
}

func TestSingleOversizedParagraphIsHardSplit(t *testing.T) {
	// 单个段落本身就超限（无空行可切）
	content := "## S\n\n" + strings.Repeat("x", 2000) + "\n"
	opts := Options{MaxChars: 300}
	got := Split("Doc", content, opts)
	if len(got) < 2 {
		t.Fatalf("超限单段应被硬切分，chunk 数 = %d", len(got))
	}
	for i, c := range got {
		if n := len(c.Text()); n > opts.MaxChars {
			t.Errorf("chunk %d 长度 %d 超过上限 %d", i, n, opts.MaxChars)
		}
	}
}

func TestTinyAdjacentChunksMerged(t *testing.T) {
	// spec: 相邻两个 chunk breadcrumb 相同、且前一个 body < MinChars 时应合并，
	// 合并后不得超过 MaxChars。
	//
	// 这个窗口很窄：fitPieces 是贪心装填，只在"下一段装不下"时才封块，
	// 所以前一块通常已接近 MaxChars（即 ≥ MinChars）。实测随机语料中
	// 仅约 2% 的参数组合能触发。下面的参数由实证探测得出，能真正走进
	// mergeTiny 的合并分支——若换成"几个短段落 + 大 MaxChars"，
	// fitPieces 会把它们装进同一个块，mergeTiny 无事可做，
	// 删掉 mergeTiny 测试依然通过（假绿）。
	const maxc, minc = 30, 6
	content := "## S\n\nshort\n\n" + strings.Repeat("a bit longer paragraph here\n\n", 4) + "tiny\n\n"

	// 前置条件：不合并时必须是多块，否则本测试无法证明合并生效
	raw := Split("Doc", content, Options{MaxChars: maxc, MinChars: 1})
	if len(raw) < 2 {
		t.Fatalf("前置条件不成立：MinChars=1 时应产出多块，实际 %d 块", len(raw))
	}

	got := Split("Doc", content, Options{MaxChars: maxc, MinChars: minc})
	if len(got) >= len(raw) {
		t.Errorf("MinChars=%d 应合并掉至少一个块：未合并=%d 块，合并后=%d 块",
			minc, len(raw), len(got))
	}
	// 合并后仍须满足长度上限
	for i, c := range got {
		if n := len(c.Text()); n > maxc {
			t.Errorf("合并后 chunk %d 长度 %d 超过上限 %d", i, n, maxc)
		}
	}
}

func TestOrdinalIsSequential(t *testing.T) {
	content := "## A\n\nbody a\n\n## B\n\nbody b\n\n## C\n\nbody c\n"
	got := Split("Doc", content, Options{})
	for i, c := range got {
		if c.Ordinal != i {
			t.Errorf("chunk %d 的 Ordinal = %d，期望 %d", i, c.Ordinal, i)
		}
	}
}

func TestEmptyAndWhitespaceInputSafe(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\n\n", "\t"} {
		got := Split("Doc", in, Options{})
		if len(got) != 0 {
			t.Errorf("输入 %q 应返回空切片，实际 %d 个块", in, len(got))
		}
	}
}

func TestSplitIsDeterministic(t *testing.T) {
	content := "## A\n\nbody\n\n### B\n\nmore\n"
	first := Split("Doc", content, Options{})
	for i := 0; i < 3; i++ {
		got := Split("Doc", content, Options{})
		if len(got) != len(first) {
			t.Fatalf("第 %d 次块数 %d != 首次 %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("第 %d 次 chunk %d 不一致", i, j)
			}
		}
	}
}

func TestTableNotSplitAcrossChunks(t *testing.T) {
	content := "## Data\n\n| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n"
	got := Split("Doc", content, Options{})
	var tableChunks int
	for _, c := range got {
		if strings.Contains(c.Body, "|---|") {
			tableChunks++
			if !strings.Contains(c.Body, "| 1 | 2 |") || !strings.Contains(c.Body, "| 3 | 4 |") {
				t.Errorf("表格被拆开: %q", c.Body)
			}
		}
	}
	if tableChunks != 1 {
		t.Errorf("表格应在单个 chunk 内，实际分布在 %d 个块", tableChunks)
	}
}

func TestDeepHeadingLevels(t *testing.T) {
	content := "## L2\n\na\n\n### L3\n\nb\n\n#### L4\n\nc\n\n##### L5 not split\n\nd\n"
	got := Split("Doc", content, Options{})
	var l4, l5 bool
	for _, c := range got {
		if c.Breadcrumb == "Doc > L2 > L3 > L4" {
			l4 = true
			// H5 不切分，故其内容应留在 L4 块内
			if !strings.Contains(c.Body, "L5 not split") {
				t.Errorf("H5 应不触发切分，但内容不在 L4 块内: %q", c.Body)
			}
		}
		if strings.Contains(c.Breadcrumb, "L5") {
			l5 = true
		}
	}
	if !l4 {
		t.Errorf("未找到 L4 breadcrumb，实际=%v", breadcrumbs(got))
	}
	if l5 {
		t.Errorf("H5 不应进入 breadcrumb，实际=%v", breadcrumbs(got))
	}
}

func TestHeadingLevelJumpBack(t *testing.T) {
	// L3 之后回到 L2，breadcrumb 必须正确回退而非累积
	content := "## A\n\na\n\n### A1\n\na1\n\n## B\n\nb\n"
	got := Split("Doc", content, Options{})
	for _, c := range got {
		if strings.Contains(c.Body, "b") && strings.Contains(c.Breadcrumb, "A1") {
			t.Errorf("breadcrumb 未回退：%q（body=%q）", c.Breadcrumb, c.Body)
		}
	}
	var foundB bool
	for _, c := range got {
		if c.Breadcrumb == "Doc > B" {
			foundB = true
		}
	}
	if !foundB {
		t.Errorf("未找到 %q，实际=%v", "Doc > B", breadcrumbs(got))
	}
}

func TestOverlongBreadcrumbStillRespectsMaxChars(t *testing.T) {
	// 回归：breadcrumb 长到挤占全部正文预算时，早期实现退化为 budget=1
	// 但仍拼上完整 breadcrumb，导致每个块都超过 MaxChars
	// （下游 embedding 的 token 上限假设随之失效）。
	// 正确行为：截断 breadcrumb，保证 Text() 始终满足约束。
	cases := []struct {
		name  string
		title string
		max   int
	}{
		{"ascii 超长标题", strings.Repeat("x", 250), 200},
		{"多字节超长标题", strings.Repeat("字", 100), 200},
		{"标题恰好等于上限", strings.Repeat("y", 200), 200},
		{"极小上限", "some title", 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Split(tc.title, strings.Repeat("body content here\n\n", 20),
				Options{MaxChars: tc.max})
			if len(got) == 0 {
				t.Fatal("应产出至少一个块")
			}
			for i, c := range got {
				if n := len(c.Text()); n > tc.max {
					t.Errorf("块 %d 长度 %d 超过上限 %d", i, n, tc.max)
				}
				if strings.TrimSpace(c.Body) == "" {
					t.Errorf("块 %d 正文为空（截断后未给正文留出空间）", i)
				}
			}
		})
	}
}

func TestTruncatedBreadcrumbKeepsValidUTF8(t *testing.T) {
	// 截断 breadcrumb 不得切裂多字节字符
	got := Split(strings.Repeat("中文标题", 40), "body text", Options{MaxChars: 60})
	for i, c := range got {
		if !utf8.ValidString(c.Breadcrumb) {
			t.Errorf("块 %d 的 breadcrumb 不是合法 UTF-8: %q", i, c.Breadcrumb)
		}
		if !utf8.ValidString(c.Text()) {
			t.Errorf("块 %d 的 Text() 不是合法 UTF-8", i)
		}
	}
}

func TestTruncateRunesRespectsByteLimit(t *testing.T) {
	// 回归：早期实现用 i > n 且返回 rune 起始位置（而非结束位置），
	// 导致结果总比 n 多出一个完整 rune，例如
	// truncateRunes("𝄞𝄞", 5) 返回 8 字节。当时未造成实际越界，
	// 仅因 minBodyBudget=16 的余量恰好吸收了超额——属侥幸而非设计。
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"abcdef", 3, "abc"},
		{"字字字", 4, "字"}, // 3 字节/字：放不下第二个
		{"字字字", 1, ""},  // 一个都放不下
		{"a字b", 2, "a"}, // a(1) + 字(3) = 4 > 2
		{"𝄞𝄞", 5, "𝄞"},  // 4 字节/字符
		{"字", 3, "字"},   // 恰好放下
		{"字a", 4, "字a"}, // 恰好放下
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"", 5, ""},
		{"short", 100, "short"},
	}
	for _, c := range cases {
		got := truncateRunes(c.s, c.n)
		if got != c.want {
			t.Errorf("truncateRunes(%q, %d) = %q，期望 %q", c.s, c.n, got, c.want)
		}
		if c.n >= 0 && len(got) > c.n {
			t.Errorf("truncateRunes(%q, %d) 返回 %d 字节，超过上限", c.s, c.n, len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("truncateRunes(%q, %d) 返回非法 UTF-8", c.s, c.n)
		}
		if !strings.HasPrefix(c.s, got) {
			t.Errorf("truncateRunes(%q, %d) = %q 不是原串前缀", c.s, c.n, got)
		}
	}
}

func TestPropertyTruncateRunesInvariants(t *testing.T) {
	// 属性：结果 <= n 字节、合法 UTF-8、且为原串前缀
	f := func(s string, n uint8) bool {
		lim := int(n)
		got := truncateRunes(s, lim)
		return len(got) <= lim && utf8.ValidString(got) && strings.HasPrefix(s, got)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}
