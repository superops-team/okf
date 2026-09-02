package lexical

import (
	"strings"
	"testing"
)

func has(toks []string, want string) bool {
	for _, t := range toks {
		if t == want {
			return true
		}
	}
	return false
}

func TestTokenizeIdentifierKeepsWholeAndSubwords(t *testing.T) {
	got := Tokenize("okf_semantic_search")
	for _, want := range []string{"okf_semantic_search", "okf", "semantic", "search"} {
		if !has(got, want) {
			t.Errorf("缺少 token %q，实际=%v", want, got)
		}
	}
}

func TestTokenizeCamelCase(t *testing.T) {
	got := Tokenize("EnableParentChild")
	for _, want := range []string{"enableparentchild", "enable", "parent", "child"} {
		if !has(got, want) {
			t.Errorf("缺少 token %q，实际=%v", want, got)
		}
	}
}

func TestTokenizeAcronymBoundary(t *testing.T) {
	got := Tokenize("HTTPServer")
	for _, want := range []string{"http", "server"} {
		if !has(got, want) {
			t.Errorf("缺少 token %q，实际=%v", want, got)
		}
	}
}

func TestTokenizeKebabAndDot(t *testing.T) {
	got := Tokenize("core-types.md")
	for _, want := range []string{"core", "types", "md"} {
		if !has(got, want) {
			t.Errorf("缺少 token %q，实际=%v", want, got)
		}
	}
}

func TestTokenizeChineseBigram(t *testing.T) {
	got := Tokenize("如何构建向量索引")
	want := []string{"如何", "何构", "构建", "建向", "向量", "量索", "索引"}
	if len(got) != len(want) {
		t.Fatalf("token 数 = %d，期望 %d；实际=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d] = %q，期望 %q", i, got[i], want[i])
		}
	}
}

func TestTokenizeSingleCJKChar(t *testing.T) {
	got := Tokenize("中 文")
	for _, want := range []string{"中", "文"} {
		if !has(got, want) {
			t.Errorf("单字应产出 unigram %q，实际=%v", want, got)
		}
	}
}

func TestTokenizeMixedLanguage(t *testing.T) {
	got := Tokenize("MCP 服务器有哪些工具")
	if !has(got, "mcp") {
		t.Errorf("缺少拉丁词 mcp，实际=%v", got)
	}
	for _, want := range []string{"服务", "务器"} {
		if !has(got, want) {
			t.Errorf("缺少中文 bigram %q，实际=%v", want, got)
		}
	}
}

func TestTokenizeDigitsAndVersion(t *testing.T) {
	got := Tokenize("okf v0.4.1 build 2026")
	for _, want := range []string{"v0", "4", "1", "2026", "build"} {
		if !has(got, want) {
			t.Errorf("缺少 token %q，实际=%v", want, got)
		}
	}
}

func TestTokenizeEmptyAndPunctuationSafe(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t", "!!!", "---", "。，、"} {
		got := Tokenize(in)
		if len(got) != 0 {
			t.Errorf("输入 %q 应返回空，实际=%v", in, got)
		}
	}
}

func TestTokenizeNoEmptyTokens(t *testing.T) {
	for _, in := range []string{"a__b", "A--B", "x..y", "中文, English!", "_leading", "trailing_"} {
		for _, tok := range Tokenize(in) {
			if tok == "" {
				t.Errorf("输入 %q 产生了空 token", in)
			}
		}
	}
}

func TestTokenizeIsLowercased(t *testing.T) {
	for _, tok := range Tokenize("MixedCase UPPER lower") {
		if tok != strings.ToLower(tok) {
			t.Errorf("token %q 未小写化", tok)
		}
	}
}

// ---------- BM25 ----------

func buildBM25(docs map[string]string) *BM25 {
	b := NewBM25()
	// 用有序 key 保证插入顺序确定
	keys := make([]string, 0, len(docs))
	for k := range docs {
		keys = append(keys, k)
	}
	// 简单排序（避免引入 sort 包依赖到测试主体逻辑之外）
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		b.Add(k, docs[k])
	}
	b.Finalize()
	return b
}

func TestBM25MultiWordQueryReturnsResults(t *testing.T) {
	b := buildBM25(map[string]string{
		"cli#3":  "okf vector index builds the semantic index; vector rebuild recreates it",
		"lint#1": "lint checks spec compliance and error codes",
		"mcp#2":  "mcp server exposes tools over stdio",
	})
	got := b.Search("vector index rebuild command", 5)
	if len(got) == 0 {
		t.Fatal("多词查询返回空（改动前的子串匹配即为此行为）")
	}
	if got[0].Key != "cli#3" {
		t.Errorf("top1 = %q，期望 cli#3；全部=%v", got[0].Key, got)
	}
}

func TestBM25ChineseQuery(t *testing.T) {
	b := buildBM25(map[string]string{
		"cli#1":  "构建向量索引的命令是 okf vector index",
		"lint#1": "校验知识库规范的命令是 okf lint",
	})
	got := b.Search("如何构建向量索引", 5)
	if len(got) == 0 {
		t.Fatal("中文查询返回空")
	}
	if got[0].Key != "cli#1" {
		t.Errorf("top1 = %q，期望 cli#1", got[0].Key)
	}
}

func TestBM25IdentifierSubwordMatch(t *testing.T) {
	b := buildBM25(map[string]string{
		"mcp#5": "the okf_semantic_search tool performs hybrid retrieval",
		"cli#0": "unrelated content about formatting",
	})
	got := b.Search("semantic search", 5)
	if len(got) == 0 {
		t.Fatal("标识符子词查询返回空")
	}
	if got[0].Key != "mcp#5" {
		t.Errorf("top1 = %q，期望 mcp#5", got[0].Key)
	}
}

func TestBM25TermFrequencyAndLengthNormalization(t *testing.T) {
	// A：词频高且更短；B：词频低且更长 → A 分数必须更高
	b := NewBM25()
	b.Add("A", "alpha alpha alpha")
	b.Add("B", "alpha "+strings.Repeat("filler ", 60))
	b.Finalize()
	got := b.Search("alpha", 2)
	if len(got) != 2 {
		t.Fatalf("返回 %d 条，期望 2", len(got))
	}
	if got[0].Key != "A" {
		t.Errorf("top1 = %q，期望 A（词频更高、文档更短）", got[0].Key)
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("分数未体现差异: A=%v B=%v", got[0].Score, got[1].Score)
	}
}

// 单独隔离长度归一化：两个文档词频完全相同，仅长度不同。
// 若去掉 b 参数的长度归一化项，二者分数将完全相等，本测试即失败。
// （上一个测试同时改变了词频与长度，词频一项就能决定胜负，
// 因此无法证明长度归一化真的生效。）
func TestBM25LengthNormalizationIsolated(t *testing.T) {
	b := NewBM25()
	b.Add("short", "alpha beta")
	b.Add("long", "alpha "+strings.Repeat("filler ", 80))
	b.Finalize()

	got := b.Search("alpha", 2)
	if len(got) != 2 {
		t.Fatalf("返回 %d 条，期望 2", len(got))
	}
	// 两文档中 "alpha" 的词频都是 1，唯一差异是文档长度
	if got[0].Key != "short" {
		t.Errorf("top1 = %q，期望 short（同词频下短文档得分更高）", got[0].Key)
	}
	if got[0].Score == got[1].Score {
		t.Errorf("同词频、不同长度的文档得分相同（长度归一化未生效）: %v vs %v",
			got[0].Score, got[1].Score)
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("短文档得分 %v 未高于长文档 %v", got[0].Score, got[1].Score)
	}
}

func TestBM25NoMatchReturnsEmpty(t *testing.T) {
	b := buildBM25(map[string]string{"a": "alpha beta", "b": "gamma delta"})
	if got := b.Search("zzz_no_such_token", 5); len(got) != 0 {
		t.Errorf("未命中应返回空，实际=%v", got)
	}
}

func TestBM25EmptyQueryAndEmptyIndex(t *testing.T) {
	empty := NewBM25()
	empty.Finalize()
	if got := empty.Search("anything", 5); len(got) != 0 {
		t.Errorf("空索引应返回空，实际=%v", got)
	}
	b := buildBM25(map[string]string{"a": "alpha"})
	if got := b.Search("", 5); len(got) != 0 {
		t.Errorf("空查询应返回空，实际=%v", got)
	}
}

func TestBM25TieBreakIsStable(t *testing.T) {
	// 两个完全相同内容的文档 → 分数相同，顺序必须按 key 升序且多次一致
	b := NewBM25()
	b.Add("zebra", "same content here")
	b.Add("alpha", "same content here")
	b.Finalize()
	for i := 0; i < 5; i++ {
		got := b.Search("same content", 2)
		if len(got) != 2 {
			t.Fatalf("返回 %d 条，期望 2", len(got))
		}
		if got[0].Key != "alpha" || got[1].Key != "zebra" {
			t.Fatalf("第 %d 次顺序 = %q,%q，期望 alpha,zebra（同分按 key 升序）", i, got[0].Key, got[1].Key)
		}
	}
}

func TestBM25RespectsK(t *testing.T) {
	b := buildBM25(map[string]string{
		"a": "alpha", "b": "alpha", "c": "alpha", "d": "alpha",
	})
	if got := b.Search("alpha", 2); len(got) != 2 {
		t.Errorf("k=2 应返回 2 条，实际 %d", len(got))
	}
	if got := b.Search("alpha", 0); len(got) != 0 {
		t.Errorf("k=0 应返回空，实际 %d", len(got))
	}
}

func TestBM25IsDeterministic(t *testing.T) {
	docs := map[string]string{
		"a#0": "vector index build", "a#1": "vector search query",
		"b#0": "lint rules and codes", "b#1": "spec compliance check",
	}
	first := buildBM25(docs).Search("vector index", 4)
	for i := 0; i < 3; i++ {
		got := buildBM25(docs).Search("vector index", 4)
		if len(got) != len(first) {
			t.Fatalf("第 %d 次返回 %d 条，首次 %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j].Key != first[j].Key || got[j].Score != first[j].Score {
				t.Fatalf("第 %d 次 rank %d 不一致: %+v vs %+v", i, j, got[j], first[j])
			}
		}
	}
}
