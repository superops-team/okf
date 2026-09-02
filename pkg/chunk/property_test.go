package chunk

import (
	"strings"
	"testing"
	"testing/quick"
	"unicode"
)

// 属性测试使用标准库 testing/quick（不引入第三方依赖，遵循 AGENTS.md）。

var quickCfg = &quick.Config{MaxCount: 300}

// 属性 1：分块不丢失内容 —— 所有 body 的非空白字符集合必须覆盖原内容。
func TestPropertySplitPreservesNonSpaceContent(t *testing.T) {
	f := func(title, content string) bool {
		cs := Split(title, content, Options{})
		var joined strings.Builder
		for _, c := range cs {
			joined.WriteString(c.Body)
		}
		return stripSpace(joined.String()) == stripSpaceExcludingHeadings(content)
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 2：任何 chunk 的 Text() 都不超过 MaxChars。
func TestPropertyChunkWithinMaxChars(t *testing.T) {
	f := func(title, content string) bool {
		const max = 200
		for _, c := range Split(title, content, Options{MaxChars: max}) {
			if len(c.Text()) > max {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 3：Split 幂等确定 —— 同一输入多次调用结果一致。
func TestPropertySplitDeterministic(t *testing.T) {
	f := func(title, content string) bool {
		a := Split(title, content, Options{})
		b := Split(title, content, Options{})
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 4：Ordinal 必须是 0..n-1 的连续序列。
func TestPropertyOrdinalSequential(t *testing.T) {
	f := func(title, content string) bool {
		for i, c := range Split(title, content, Options{}) {
			if c.Ordinal != i {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 5：body 不为空白 —— 不产生无意义的空块。
func TestPropertyNoBlankChunks(t *testing.T) {
	f := func(title, content string) bool {
		for _, c := range Split(title, content, Options{}) {
			if strings.TrimSpace(c.Body) == "" {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func stripSpace(s string) string {
	var b strings.Builder
	for _, r := range s {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// 标题行本身被提取为 breadcrumb，不计入 body，比较时需同样剔除。
func stripSpaceExcludingHeadings(content string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		if isFenceLine(line) {
			inFence = !inFence
			b.WriteString(stripSpace(line))
			continue
		}
		if !inFence {
			if lv, _, ok := parseHeading(line); ok && lv >= minSplitLevel && lv <= maxSplitLevel {
				continue // 该行转为 breadcrumb
			}
		}
		b.WriteString(stripSpace(line))
	}
	return b.String()
}
