package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/chunk"
	"github.com/superops-team/okf/pkg/eval"
	"github.com/superops-team/okf/pkg/query"
	"github.com/superops-team/okf/pkg/vectorindex"
)

// writeKB 写出一个最小知识库，返回其路径。
func writeKB(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Scenario: 索引条目数等于 chunk 总数（而非概念数），
// 且 okf vector status 同时显示概念数与 chunk 数。
//
// 该测试不启动 embedding 模型（CI 无模型时也应可跑），
// 而是直接校验「构建索引所依据的 chunk 数」与「写入 meta 的计数口径」一致。
func TestIndexEntryCountEqualsChunkCount(t *testing.T) {
	// 两个概念，各含多个 H2 小节，分块后必然多于 2 块
	long := "---\ntype: documentation\ntitle: Long\ndescription: a long document\n---\n" +
		"## S1\n\n" + strings.Repeat("alpha beta gamma delta epsilon ", 60) + "\n\n" +
		"## S2\n\n" + strings.Repeat("zeta eta theta iota kappa ", 60) + "\n"
	short := "---\ntype: documentation\ntitle: Short\ndescription: a short document\n---\n" +
		"just one line\n"
	dir := writeKB(t, map[string]string{"long.md": long, "short.md": short})

	qb, _, err := loadKnowledgeBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(qb.Concepts) != 2 {
		t.Fatalf("概念数 = %d，期望 2", len(qb.Concepts))
	}

	// 统计分块总数，并确认确实多于概念数（否则本测试无法区分两种口径）
	total := 0
	for _, c := range qb.Concepts {
		cs := chunk.Split(c.Title, conceptChunkSource(c), chunk.Options{})
		if len(cs) == 0 {
			cs = []chunk.Chunk{{Breadcrumb: c.Title, Body: c.Description}}
		}
		total += len(cs)
	}
	if total <= len(qb.Concepts) {
		t.Fatalf("前置条件不成立：chunk 数 %d 应多于概念数 %d", total, len(qb.Concepts))
	}

	// 用假向量构建索引，验证条目数按 chunk 计
	idx := vectorindex.NewHNSW(4)
	for _, c := range qb.Concepts {
		cs := chunk.Split(c.Title, conceptChunkSource(c), chunk.Options{})
		if len(cs) == 0 {
			cs = []chunk.Chunk{{Breadcrumb: c.Title, Body: c.Description}}
		}
		for _, ch := range cs {
			idx.Add(query.ChunkKey(c, ch.Ordinal), []float32{1, 0, 0, 0})
		}
	}
	if idx.Len() != total {
		t.Errorf("索引条目数 = %d，期望等于 chunk 总数 %d", idx.Len(), total)
	}

	// meta 必须同时记录 chunk 数（Count）与概念数（Concepts）
	vdir := filepath.Join(dir, ".okf", "vector")
	if err := idx.Save(vdir, vectorindex.Meta{
		Model: "test", OkfVersion: "test", Concepts: len(qb.Concepts),
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(vdir, vectorindex.MetaFileName))
	if err != nil {
		t.Fatal(err)
	}
	var meta vectorindex.Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Count != total {
		t.Errorf("meta.Count = %d，期望 chunk 总数 %d", meta.Count, total)
	}
	if meta.Concepts != len(qb.Concepts) {
		t.Errorf("meta.Concepts = %d，期望概念数 %d", meta.Concepts, len(qb.Concepts))
	}
	if meta.IndexFormatVersion != vectorindex.CurrentIndexFormatVersion {
		t.Errorf("meta.IndexFormatVersion = %d，期望 %d",
			meta.IndexFormatVersion, vectorindex.CurrentIndexFormatVersion)
	}
}

// conceptChunkSource 必须把 description 纳入分块来源，
// 否则只有 description 而无正文的概念会完全进不了索引。
func TestConceptChunkSourceIncludesDescription(t *testing.T) {
	c := &query.Concept{Title: "T", Description: "the description", Content: "the content"}
	got := conceptChunkSource(c)
	if !strings.Contains(got, "the description") {
		t.Errorf("分块来源缺少 description: %q", got)
	}
	if !strings.Contains(got, "the content") {
		t.Errorf("分块来源缺少 content: %q", got)
	}

	// 仅有 description 时也必须有内容可分块
	only := &query.Concept{Title: "T", Description: "only description"}
	if got := conceptChunkSource(only); got != "only description" {
		t.Errorf("仅有 description 时 = %q，期望 %q", got, "only description")
	}
	// 仅有 content 时不应引入多余空行
	onlyC := &query.Concept{Title: "T", Content: "only content"}
	if got := conceptChunkSource(onlyC); got != "only content" {
		t.Errorf("仅有 content 时 = %q，期望 %q", got, "only content")
	}
}

// Scenario: golden set 与知识库不匹配时必须给出警告（否则全零指标易被误读）。
func TestMissingExpectedDocsDetection(t *testing.T) {
	qb := &query.KnowledgeBundle{Concepts: []*query.Concept{
		{Title: "a", FilePath: "a.md"},
		{Title: "b", FilePath: "b.md"},
	}}

	got := missingExpectedDocs(qb, goldenCasesFor("a.md", "zzz.md", "b.md", "zzz.md"))
	if len(got) != 1 || got[0] != "zzz.md" {
		t.Errorf("缺失文档 = %v，期望 [zzz.md]（去重）", got)
	}

	// 全部存在时不得误报
	if got := missingExpectedDocs(qb, goldenCasesFor("a.md", "b.md")); len(got) != 0 {
		t.Errorf("全部存在时应返回空，实际 %v", got)
	}
}

// goldenCasesFor 把若干期望文档名包装成 golden cases（每个文档一条 case）。
func goldenCasesFor(docs ...string) []eval.EvalCase {
	out := make([]eval.EvalCase, 0, len(docs))
	for _, d := range docs {
		out = append(out, eval.EvalCase{Query: "q-" + d, ExpectedDocs: []string{d}})
	}
	return out
}
