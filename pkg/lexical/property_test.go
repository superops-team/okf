package lexical

import (
	"strings"
	"testing"
	"testing/quick"
)

var quickCfg = &quick.Config{MaxCount: 400}

// 属性 1：Tokenize 幂等确定 —— 同一输入多次调用结果一致。
func TestPropertyTokenizeDeterministic(t *testing.T) {
	f := func(s string) bool {
		a, b := Tokenize(s), Tokenize(s)
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

// 属性 2：不产生空 token。
func TestPropertyTokenizeNoEmpty(t *testing.T) {
	f := func(s string) bool {
		for _, tok := range Tokenize(s) {
			if tok == "" {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 3：所有 token 均为小写（大小写不敏感检索的前提）。
func TestPropertyTokenizeLowercase(t *testing.T) {
	f := func(s string) bool {
		for _, tok := range Tokenize(s) {
			if tok != strings.ToLower(tok) {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 4：对纯 ASCII 字母数字输入，整词集合与 strings.Fields+小写化一致。
// （子词是额外产出，故用子集关系断言：Fields 的每个词都必须出现在 token 中。）
func TestPropertyASCIIWordsArePresent(t *testing.T) {
	f := func(words []string) bool {
		var clean []string
		for _, w := range words {
			w = keepASCIIWord(w)
			if w != "" {
				clean = append(clean, w)
			}
		}
		if len(clean) == 0 {
			return true
		}
		joined := strings.Join(clean, " ")
		toks := Tokenize(joined)
		set := make(map[string]struct{}, len(toks))
		for _, tk := range toks {
			set[tk] = struct{}{}
		}
		for _, w := range strings.Fields(joined) {
			if _, ok := set[strings.ToLower(w)]; !ok {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 5：BM25 检索结果按分数降序且长度不超过 k。
func TestPropertyBM25SortedAndBounded(t *testing.T) {
	f := func(texts []string, query string) bool {
		b := NewBM25()
		for i, tx := range texts {
			b.Add(keyFor(i), tx)
		}
		b.Finalize()
		const k = 5
		hits := b.Search(query, k)
		if len(hits) > k {
			return false
		}
		for i := 1; i < len(hits); i++ {
			if hits[i-1].Score < hits[i].Score {
				return false
			}
			if hits[i-1].Score == hits[i].Score && hits[i-1].Key > hits[i].Key {
				return false // 同分必须按 key 升序
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

// 属性 6：BM25 分数恒为正（命中才返回），且检索可复现。
func TestPropertyBM25PositiveAndReproducible(t *testing.T) {
	f := func(texts []string, query string) bool {
		build := func() *BM25 {
			b := NewBM25()
			for i, tx := range texts {
				b.Add(keyFor(i), tx)
			}
			b.Finalize()
			return b
		}
		a := build().Search(query, 5)
		c := build().Search(query, 5)
		if len(a) != len(c) {
			return false
		}
		for i := range a {
			if a[i] != c[i] {
				return false
			}
			if a[i].Score <= 0 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickCfg); err != nil {
		t.Error(err)
	}
}

func keepASCIIWord(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func keyFor(i int) string {
	const d = "0123456789"
	if i == 0 {
		return "k0"
	}
	var out []byte
	for i > 0 {
		out = append([]byte{d[i%10]}, out...)
		i /= 10
	}
	return "k" + string(out)
}
