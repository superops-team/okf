package query

import (
	"math"
	"strings"
	"testing"
)

// fakeLexical 是可注入的词法后端，按预设顺序返回 key。
type fakeLexical struct {
	hits  []SemanticHit
	calls int
}

func (f *fakeLexical) Search(_ string, k int) []SemanticHit {
	f.calls++
	if k < len(f.hits) {
		return f.hits[:k]
	}
	return f.hits
}

// fakeSemantic 按预设顺序返回 key，EmbedQuery 恒成功。
type fakeSemantic struct{ hits []SemanticHit }

func (f *fakeSemantic) EmbedQuery(string) ([]float32, error) { return []float32{1}, nil }
func (f *fakeSemantic) Search(_ []float32, k int) []SemanticHit {
	if k < len(f.hits) {
		return f.hits[:k]
	}
	return f.hits
}

func weightBundle(t *testing.T) (*KnowledgeBundle, map[string]*Concept) {
	t.Helper()
	b := &KnowledgeBundle{}
	byName := map[string]*Concept{}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		c := &Concept{Type: "documentation", Title: name, FilePath: name + ".md", Content: name + " body"}
		b.Concepts = append(b.Concepts, c)
		byName[name] = c
	}
	return b, byName
}

func fpOf(c *Concept) string { return Fingerprint(c) }

func TestWeightedRRFVectorWeightDominates(t *testing.T) {
	b, by := weightBundle(t)
	sem := &fakeSemantic{hits: []SemanticHit{{Key: fpOf(by["alpha"]) + "#0"}}}
	lex := &fakeLexical{hits: []SemanticHit{{Key: fpOf(by["beta"]) + "#0"}}}

	// 语义权重远大于词法 → alpha 必须在 beta 之前
	opts := SearchOptions{TopK: 3, VectorWeight: 0.9, Lexical: lex}.WithLexicalWeight(0.1)
	got, err := SemanticSearch(b, "q", sem, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("返回 %d 条，期望 ≥2", len(got))
	}
	if got[0].Concept.Title != "alpha" {
		t.Errorf("top1 = %q，期望 alpha（语义权重 0.9）", got[0].Concept.Title)
	}
}

func TestWeightedRRFLexicalWeightDominates(t *testing.T) {
	b, by := weightBundle(t)
	sem := &fakeSemantic{hits: []SemanticHit{{Key: fpOf(by["alpha"]) + "#0"}}}
	lex := &fakeLexical{hits: []SemanticHit{{Key: fpOf(by["beta"]) + "#0"}}}

	opts := SearchOptions{TopK: 3, VectorWeight: 0.1, Lexical: lex}.WithLexicalWeight(0.9)
	got, err := SemanticSearch(b, "q", sem, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Concept.Title != "beta" {
		t.Errorf("top1 = %q，期望 beta（词法权重 0.9）", got[0].Concept.Title)
	}
}

// 权重按相对比例生效：等比例缩放不改变排序（RRF 加权的性质，spec 据此不要求权重和为 1）。
func TestWeightedRRFScaleInvariant(t *testing.T) {
	run := func(v, l float64) []string {
		b, by := weightBundle(t)
		sem := &fakeSemantic{hits: []SemanticHit{
			{Key: fpOf(by["alpha"]) + "#0"}, {Key: fpOf(by["gamma"]) + "#0"},
		}}
		lex := &fakeLexical{hits: []SemanticHit{
			{Key: fpOf(by["beta"]) + "#0"}, {Key: fpOf(by["alpha"]) + "#1"},
		}}
		got, err := SemanticSearch(b, "q", sem, SearchOptions{
			TopK: 3, VectorWeight: v, Lexical: lex,
		}.WithLexicalWeight(l))
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		for _, r := range got {
			order = append(order, r.Concept.Title)
		}
		return order
	}
	a := run(1.0, 0.5)
	c := run(0.667, 0.3335) // 同比例缩放
	if len(a) != len(c) {
		t.Fatalf("长度不同: %v vs %v", a, c)
	}
	for i := range a {
		if a[i] != c[i] {
			t.Errorf("等比例缩放改变了排序: %v vs %v", a, c)
			break
		}
	}
}

// 词法权重显式设为 0 → 跳过词法检索（后端不被调用），退化为纯语义。
func TestZeroLexicalWeightSkipsLexicalChannel(t *testing.T) {
	b, by := weightBundle(t)
	sem := &fakeSemantic{hits: []SemanticHit{{Key: fpOf(by["alpha"]) + "#0"}}}
	lex := &fakeLexical{hits: []SemanticHit{{Key: fpOf(by["beta"]) + "#0"}}}

	got, err := SemanticSearch(b, "q", sem, SearchOptions{
		TopK: 3, Lexical: lex,
	}.WithLexicalWeight(0))
	if err != nil {
		t.Fatal(err)
	}
	if lex.calls != 0 {
		t.Errorf("词法权重为 0 仍调用了词法后端 %d 次", lex.calls)
	}
	for _, r := range got {
		if r.Source != "semantic" {
			t.Errorf("概念 %q 来源 = %q，期望全部 semantic", r.Concept.Title, r.Source)
		}
	}
}

// 未显式设置权重时使用默认值（等权 0.5/0.5）。
func TestDefaultWeightsApplied(t *testing.T) {
	b, by := weightBundle(t)
	sem := &fakeSemantic{hits: []SemanticHit{{Key: fpOf(by["alpha"]) + "#0"}}}
	lex := &fakeLexical{hits: []SemanticHit{{Key: fpOf(by["beta"]) + "#0"}}}

	got, err := SemanticSearch(b, "q", sem, SearchOptions{TopK: 3, Lexical: lex})
	if err != nil {
		t.Fatal(err)
	}
	if lex.calls == 0 {
		t.Error("默认词法权重应 >0，但词法后端未被调用")
	}
	// 默认等权：两侧 rank 1 的分数应相等（各为 w/(k+1)）
	if DefaultVectorWeight != DefaultLexicalWeight {
		t.Fatalf("本测试假设默认等权，实际 %v/%v", DefaultVectorWeight, DefaultLexicalWeight)
	}
	wantScore := float32(DefaultVectorWeight / float64(DefaultRRFK+1))
	for _, r := range got {
		if math.Abs(float64(r.SemanticScore-wantScore)) > 1e-6 {
			t.Errorf("概念 %q 分数 = %v，期望 %v", r.Concept.Title, r.SemanticScore, wantScore)
		}
	}
	// 同分时按 tie-break 规则：有语义命中的排前
	if got[0].Concept.Title != "alpha" {
		t.Errorf("top1 = %q，期望 alpha（同分时有语义命中者优先）", got[0].Concept.Title)
	}
}

// 自定义 RRFK 生效。
func TestCustomRRFK(t *testing.T) {
	b, by := weightBundle(t)
	sem := &fakeSemantic{hits: []SemanticHit{{Key: fpOf(by["alpha"]) + "#0"}}}
	got, err := SemanticSearch(b, "q", sem, SearchOptions{
		TopK: 3, RRFK: 10, VectorWeight: 1.0,
	}.WithLexicalWeight(0))
	if err != nil {
		t.Fatal(err)
	}
	want := float32(1.0 / float64(10+1))
	if math.Abs(float64(got[0].SemanticScore-want)) > 1e-6 {
		t.Errorf("分数 = %v，期望 %v（RRFK=10）", got[0].SemanticScore, want)
	}
}

// 多块命中累加：同一概念占据两个候选位，分数应为两个 rank 分量之和的等价放大。
func TestMultiChunkHitsAccumulate(t *testing.T) {
	b, by := weightBundle(t)
	// alpha 命中 rank1 与 rank2；beta 只命中 rank3
	sem := &fakeSemantic{hits: []SemanticHit{
		{Key: fpOf(by["alpha"]) + "#0"},
		{Key: fpOf(by["alpha"]) + "#1"},
		{Key: fpOf(by["beta"]) + "#0"},
	}}
	got, err := SemanticSearch(b, "q", sem, SearchOptions{
		TopK: 3, VectorWeight: 1.0,
	}.WithLexicalWeight(0))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Concept.Title != "alpha" {
		t.Fatalf("top1 = %q，期望 alpha（命中 2 块）", got[0].Concept.Title)
	}
	// alpha: 1/(60+1) * 2 块
	want := float32(2.0 / 61.0)
	if math.Abs(float64(got[0].SemanticScore-want)) > 1e-6 {
		t.Errorf("alpha 分数 = %v，期望 %v（2 块累加）", got[0].SemanticScore, want)
	}
}

// 无 BM25 后端时回退到内置子串通道（向下兼容）。
func TestFallsBackToSubstringLexicalWhenNoBackend(t *testing.T) {
	b, by := weightBundle(t)
	sem := &fakeSemantic{hits: []SemanticHit{{Key: fpOf(by["gamma"]) + "#0"}}}
	// Lexical 为 nil → 走 SearchWithMatches，"alpha" 应能被子串命中
	got, err := SemanticSearch(b, "alpha", sem, SearchOptions{TopK: 3})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range got {
		if r.Concept.Title == "alpha" {
			found = true
		}
	}
	if !found {
		t.Errorf("回退子串通道未命中 alpha，结果=%v", got)
	}
}

// 融合结果必须可复现（多次执行顺序与分数一致）。
func TestWeightedFusionIsReproducible(t *testing.T) {
	type row struct {
		title string
		score float32
		src   string
	}
	snapshot := func() []row {
		b, by := weightBundle(t)
		sem := &fakeSemantic{hits: []SemanticHit{
			{Key: fpOf(by["alpha"]) + "#0"}, {Key: fpOf(by["beta"]) + "#0"},
		}}
		lex := &fakeLexical{hits: []SemanticHit{
			{Key: fpOf(by["beta"]) + "#1"}, {Key: fpOf(by["gamma"]) + "#0"},
		}}
		got, err := SemanticSearch(b, "q", sem, SearchOptions{TopK: 5, Lexical: lex})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]row, 0, len(got))
		for _, r := range got {
			out = append(out, row{r.Concept.Title, r.SemanticScore, r.Source})
		}
		return out
	}
	first := snapshot()
	for i := 0; i < 5; i++ {
		cur := snapshot()
		if len(cur) != len(first) {
			t.Fatalf("第 %d 次长度 %d，首次 %d", i, len(cur), len(first))
		}
		for j := range cur {
			if cur[j] != first[j] {
				t.Fatalf("第 %d 次 rank %d 不一致: %+v vs %+v", i, j, cur[j], first[j])
			}
		}
	}
}

// 两侧都命中的概念来源应标为 both。
func TestBothSourcesLabeled(t *testing.T) {
	b, by := weightBundle(t)
	sem := &fakeSemantic{hits: []SemanticHit{{Key: fpOf(by["alpha"]) + "#0"}}}
	lex := &fakeLexical{hits: []SemanticHit{{Key: fpOf(by["alpha"]) + "#1"}}}
	got, err := SemanticSearch(b, "q", sem, SearchOptions{TopK: 3, Lexical: lex})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Source != "both" {
		t.Errorf("来源 = %q，期望 both", got[0].Source)
	}
}

// stubNoSemantic 是恒不召回的语义后端，用于隔离出词法通道的行为。
type stubNoSemantic struct{}

func (stubNoSemantic) EmbedQuery(string) ([]float32, error) { return []float32{1}, nil }
func (stubNoSemantic) Search([]float32, int) []SemanticHit  { return nil }

// 回归：MCP 的 handleSemanticSearch 曾未注入词法后端，导致 SemanticSearch
// 静默退化为内置子串通道（整串匹配，实测 Recall@5 仅 0.0769），
// 使 MCP 消费方拿到的结果显著劣于 CLI。此处固定「多词查询在
// 子串通道必然落空、在 BM25 通道可召回」这一差异。
func TestLexicalBackendChangesRecallForMultiWordQuery(t *testing.T) {
	b := &KnowledgeBundle{Concepts: []*Concept{
		{Title: "Vector Index Rebuild", Content: "run okf vector rebuild to refresh embeddings", FilePath: "a.md"},
		{Title: "Unrelated", Content: "nothing here", FilePath: "b.md"},
	}}
	const q = "rebuild embeddings index" // 该整串不是任何概念的子串

	without, err := SemanticSearch(b, q, stubNoSemantic{}, SearchOptions{TopK: 5})
	if err != nil {
		t.Fatalf("未接后端时报错: %v", err)
	}
	if len(without) != 0 {
		t.Fatalf("前置条件不成立：子串通道本应召回 0 条，实际 %d 条", len(without))
	}

	with, err := SemanticSearch(b, q, stubNoSemantic{}, SearchOptions{
		TopK: 5, Lexical: BuildLexicalBackend(b),
	})
	if err != nil {
		t.Fatalf("接入 BM25 后报错: %v", err)
	}
	if len(with) == 0 {
		t.Error("接入 BM25 后仍召回 0 条，词法通道未生效")
	}
	if len(with) > 0 && with[0].Concept.Title != "Vector Index Rebuild" {
		t.Errorf("首位命中 = %q，期望 Vector Index Rebuild", with[0].Concept.Title)
	}
}

// BuildLexicalBackend 对 nil bundle 必须安全返回 nil，而不是 panic。
func TestBuildLexicalBackendNilBundle(t *testing.T) {
	if got := BuildLexicalBackend(nil); got != nil {
		t.Errorf("nil bundle 应返回 nil，实际 %v", got)
	}
}

// 概念正文与描述皆空时仍应以标题建立词法条目，保证可被召回。
func TestBuildLexicalBackendIndexesTitleOnlyConcept(t *testing.T) {
	b := &KnowledgeBundle{Concepts: []*Concept{
		{Title: "Solitary Title Concept", FilePath: "t.md"},
	}}
	lex := BuildLexicalBackend(b)
	if hits := lex.Search("solitary title", 5); len(hits) == 0 {
		t.Error("仅有标题的概念未被词法索引，无法召回")
	}
}

// 两个检索通道必须共用同一分块口径：若向量侧与词法侧各自 chunk.Split，
// 同一 ordinal 会指向不同文本，ChunkKey 对不齐，RRF 会把不相干的命中
// 当成同一块融合。此测试固定「共享入口」这一不变量。
func TestVectorAndLexicalChannelsShareChunkKeys(t *testing.T) {
	c := &Concept{
		Title:       "Alignment",
		Description: "desc line",
		Content:     "## S1\n\n" + strings.Repeat("alpha ", 300) + "\n\n## S2\n\n" + strings.Repeat("beta ", 300),
		FilePath:    "a.md",
	}
	b := &KnowledgeBundle{Concepts: []*Concept{c}}

	// 向量侧（cmd_vector.go 建索引时）使用的 key 集合
	vectorKeys := map[string]bool{}
	for _, ch := range ConceptChunks(c) {
		vectorKeys[ChunkKey(c, ch.Ordinal)] = true
	}
	if len(vectorKeys) < 2 {
		t.Fatalf("前置条件不成立：应产生多个块，实际 %d", len(vectorKeys))
	}

	// 词法侧命中的 key 必须都落在向量侧 key 集合内
	lex := BuildLexicalBackend(b)
	hits := lex.Search("alpha beta", 50)
	if len(hits) == 0 {
		t.Fatal("词法通道未召回任何块")
	}
	for _, h := range hits {
		if !vectorKeys[h.Key] {
			t.Errorf("词法 key %q 不在向量 key 集合中，两通道分块口径已漂移", h.Key)
		}
		if ChunkKeyConcept(h.Key) != Fingerprint(c) {
			t.Errorf("key %q 无法回溯到父概念", h.Key)
		}
	}
}

// rrfScore 在 k+rank<=0 时必须返回 0 而非 Inf/NaN。
// 该分支经公开 API 不可达（SemanticSearch 已把 RRFK<=0 归一化为默认值），
// 属防御性约束：Inf/NaN 一旦进入分数累加会污染整个排序。
func TestRRFScoreNeverInfOrNaN(t *testing.T) {
	for _, tc := range []struct{ k, rank int }{{0, 0}, {-1, 1}, {-2, 1}, {-60, 60}, {0, 1}, {60, 1}} {
		s := rrfScore(1.0, tc.k, tc.rank)
		f := float64(s)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			t.Errorf("rrfScore(1.0, %d, %d) = %v，非法", tc.k, tc.rank, s)
		}
	}
}

// 极端 / 非法选项下不得 panic，且不得返回 nil Concept 或超出 TopK。
func TestSemanticSearchHandlesDegenerateOptions(t *testing.T) {
	c := &Concept{Title: "T", Content: "alpha beta", FilePath: "a.md"}
	b := &KnowledgeBundle{Concepts: []*Concept{c}}
	for _, tc := range []struct {
		name string
		opts SearchOptions
		q    string
	}{
		{"negative topk", SearchOptions{TopK: -5}, "alpha"},
		{"zero topk", SearchOptions{TopK: 0}, "alpha"},
		{"negative rrfk", SearchOptions{TopK: 5, RRFK: -60}, "alpha"},
		{"negative candidate factor", SearchOptions{TopK: 5, CandidateFactor: -3}, "alpha"},
		{"empty query", SearchOptions{TopK: 5}, ""},
		{"whitespace query", SearchOptions{TopK: 5}, "   \t\n "},
		{"very long query", SearchOptions{TopK: 5}, strings.Repeat("x", 100000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			opts.Lexical = BuildLexicalBackend(b)
			res, err := SemanticSearch(b, tc.q, stubNoSemantic{}, opts)
			if err != nil {
				t.Fatalf("非预期错误: %v", err)
			}
			if tc.opts.TopK > 0 && len(res) > tc.opts.TopK {
				t.Errorf("返回 %d 条，超过 TopK=%d", len(res), tc.opts.TopK)
			}
			for _, r := range res {
				if r.Concept == nil {
					t.Error("返回了 nil Concept")
				}
			}
		})
	}
}
