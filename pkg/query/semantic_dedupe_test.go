package query

import (
	"strings"
	"testing"
)

// chunkConcept builds a chunk-derived concept sharing source_path.
func chunkConcept(title, sourcePath, filePath string) *Concept {
	return &Concept{
		Type:     "doc",
		Title:    title,
		Content:  "chunk body " + title,
		FilePath: filePath,
		CustomFields: map[string]interface{}{
			"source_path": sourcePath,
			"derived":     "true",
		},
	}
}

func TestSourceKey_PrefersSourcePath(t *testing.T) {
	c := chunkConcept("doc — part 1", "kb/doc.pdf", "kb/doc.pdf__c0.md")
	key := sourceKey(c)
	if !strings.HasPrefix(key, "src:kb/doc.pdf") {
		t.Fatalf("sourceKey = %q, want src: prefix with source_path", key)
	}
	// 同源第二个 chunk 必须得到相同 key
	c2 := chunkConcept("doc — part 2", "kb/doc.pdf", "kb/doc.pdf__c1.md")
	if sourceKey(c2) != key {
		t.Fatalf("chunks of one source differ: %q vs %q", key, sourceKey(c2))
	}
}

func TestSourceKey_StripsCNWithoutSourcePath(t *testing.T) {
	c := &Concept{Type: "doc", Title: "doc", FilePath: "kb/doc.pdf__c3.md"}
	k1 := sourceKey(c)
	c2 := &Concept{Type: "doc", Title: "doc", FilePath: "kb/doc.pdf__c4.md"}
	k2 := sourceKey(c2)
	if k1 != k2 {
		t.Fatalf("__cN variants differ: %q vs %q", k1, k2)
	}
	if strings.Contains(k1, "__c") {
		t.Fatalf("sourceKey %q still contains __cN", k1)
	}
}

func TestSourceKey_NonChunkUsesFingerprint(t *testing.T) {
	c := &Concept{Type: "doc", Title: "Alpha", FilePath: "kb/a.md"}
	k := sourceKey(c)
	if !strings.HasPrefix(k, "concept:") || !strings.Contains(k, "alpha") {
		t.Fatalf("sourceKey = %q, want concept: fingerprint", k)
	}
}

func TestSemanticSearch_DedupeBySource(t *testing.T) {
	b := &KnowledgeBundle{Concepts: []*Concept{
		chunkConcept("doc — part 1", "kb/doc.pdf", "kb/doc.pdf__c0.md"),
		chunkConcept("doc — part 2", "kb/doc.pdf", "kb/doc.pdf__c1.md"),
		chunkConcept("doc — part 3", "kb/doc.pdf", "kb/doc.pdf__c2.md"),
		{Type: "doc", Title: "Alpha", Content: "apple banana", FilePath: "kb/a.md"},
	}}
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(b.Concepts[0]), Score: 0.9},
		{Key: Fingerprint(b.Concepts[1]), Score: 0.8},
		{Key: Fingerprint(b.Concepts[2]), Score: 0.7},
		{Key: Fingerprint(b.Concepts[3]), Score: 0.6},
	}}

	res, err := SemanticSearch(b, "doc", fb, SearchOptions{TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	// 同源 3 个 chunk 合并为 1，加 Alpha = 2 条
	if len(res) != 2 {
		t.Fatalf("got %d results, want 2 (deduped)", len(res))
	}
	// survivor = 最高分 chunk（part 1），且 DuplicateCount = 2
	var survivor *SearchResult
	for i := range res {
		if strings.HasPrefix(res[i].Concept.Title, "doc —") {
			survivor = &res[i]
		}
	}
	if survivor == nil {
		t.Fatal("no chunk survivor")
	}
	if survivor.Concept.Title != "doc — part 1" {
		t.Fatalf("survivor = %q, want part 1 (top score)", survivor.Concept.Title)
	}
	if survivor.DuplicateCount != 2 {
		t.Fatalf("DuplicateCount = %d, want 2", survivor.DuplicateCount)
	}
}

func TestSemanticSearch_DedupeKeepsTopScore(t *testing.T) {
	b := &KnowledgeBundle{Concepts: []*Concept{
		chunkConcept("doc — part 1", "kb/doc.pdf", "kb/doc.pdf__c0.md"),
		chunkConcept("doc — part 2", "kb/doc.pdf", "kb/doc.pdf__c1.md"),
	}}
	// part 2 语义 rank 更高（RRF 按 rank 计分）
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(b.Concepts[1]), Score: 0.95},
		{Key: Fingerprint(b.Concepts[0]), Score: 0.3},
	}}
	res, err := SemanticSearch(b, "doc", fb, SearchOptions{TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	if res[0].Concept.Title != "doc — part 2" {
		t.Fatalf("survivor = %q, want part 2", res[0].Concept.Title)
	}
	if res[0].DuplicateCount != 1 {
		t.Fatalf("DuplicateCount = %d, want 1", res[0].DuplicateCount)
	}
}

func TestSemanticSearch_DisableDedupe(t *testing.T) {
	b := &KnowledgeBundle{Concepts: []*Concept{
		chunkConcept("doc — part 1", "kb/doc.pdf", "kb/doc.pdf__c0.md"),
		chunkConcept("doc — part 2", "kb/doc.pdf", "kb/doc.pdf__c1.md"),
	}}
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(b.Concepts[0]), Score: 0.9},
		{Key: Fingerprint(b.Concepts[1]), Score: 0.8},
	}}
	res, err := SemanticSearch(b, "doc", fb, SearchOptions{TopK: 5, DisableDedupe: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d results, want 2 (dedupe disabled)", len(res))
	}
	for _, r := range res {
		if r.DuplicateCount != 0 {
			t.Fatalf("DuplicateCount = %d, want 0 when disabled", r.DuplicateCount)
		}
	}
}

func TestSemanticSearch_NonChunkConceptsUnaffected(t *testing.T) {
	// 无 source_path / __cN 的普通概念：不同文件不去重
	b := newTestBundle()
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(b.Concepts[0]), Score: 0.9},
		{Key: Fingerprint(b.Concepts[1]), Score: 0.8},
		{Key: Fingerprint(b.Concepts[2]), Score: 0.7},
	}}
	res, err := SemanticSearch(b, "apple", fb, SearchOptions{TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results, want 3 (no dedupe for distinct sources)", len(res))
	}
}

func TestSemanticSearch_DedupeStillFusesLexical(t *testing.T) {
	// 去重后 RRF 融合仍生效：Alpha(both) 排第一
	b := &KnowledgeBundle{Concepts: []*Concept{
		chunkConcept("doc — part 1", "kb/doc.pdf", "kb/doc.pdf__c0.md"),
		{Type: "doc", Title: "Alpha", Content: "apple banana fruit", FilePath: "kb/a.md"},
		{Type: "doc", Title: "Beta", Content: "unrelated topic", FilePath: "kb/b.md"},
	}}
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(b.Concepts[1]), Score: 0.95}, // Alpha rank1
		{Key: Fingerprint(b.Concepts[0]), Score: 0.8},  // chunk rank2
		{Key: Fingerprint(b.Concepts[2]), Score: 0.7},  // Beta rank3
	}}
	res, err := SemanticSearch(b, "apple", fb, SearchOptions{TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("got %d results, want 3 (chunk deduped + Alpha + Beta)", len(res))
	}
	// Alpha 双通道命中排第一
	if res[0].Concept.Title != "Alpha" || res[0].Source != "both" {
		t.Fatalf("top = %q/%q, want Alpha/both", res[0].Concept.Title, res[0].Source)
	}
}

// --- Gap 4: dedupe result ordering is stable and score-descending ---

func TestSemanticSearch_DedupeResultsSortedByScore(t *testing.T) {
	// Three distinct sources (no dedupe collapse); assert results are sorted by
	// SemanticScore descending and the order is deterministic across runs (the
	// sort is SliceStable, so equal-score ties preserve input order — this is
	// the tie-breaking contract even though RRF rank uniqueness makes exact ties
	// unreachable in practice).
	b := &KnowledgeBundle{Concepts: []*Concept{
		{Type: "doc", Title: "Low", Content: "zzz quiet", FilePath: "kb/low.md"},
		{Type: "doc", Title: "High", Content: "apple banana fruit", FilePath: "kb/high.md"},
		{Type: "doc", Title: "Mid", Content: "banana cherry", FilePath: "kb/mid.md"},
	}}
	fb := &fakeBackend{hits: []SemanticHit{
		{Key: Fingerprint(b.Concepts[1]), Score: 0.95}, // High rank1
		{Key: Fingerprint(b.Concepts[2]), Score: 0.80}, // Mid rank2
		{Key: Fingerprint(b.Concepts[0]), Score: 0.60}, // Low rank3
	}}
	// Run twice; order must be identical (deterministic).
	var first []string
	for run := 0; run < 2; run++ {
		res, err := SemanticSearch(b, "apple banana", fb, SearchOptions{TopK: 5})
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 3 {
			t.Fatalf("run %d: got %d results, want 3", run, len(res))
		}
		// score descending
		for i := 1; i < len(res); i++ {
			if res[i].SemanticScore > res[i-1].SemanticScore {
				t.Fatalf("run %d: results not score-descending: res[%d]=%.4f > res[%d]=%.4f",
					run, i, res[i].SemanticScore, i-1, res[i-1].SemanticScore)
			}
		}
		// top must be High (both channels: lexical + semantic rank1)
		if res[0].Concept.Title != "High" {
			t.Fatalf("run %d: top = %q, want High", run, res[0].Concept.Title)
		}
		titles := make([]string, len(res))
		for i, r := range res {
			titles[i] = r.Concept.Title
		}
		if run == 0 {
			first = titles
		} else if strings.Join(titles, ",") != strings.Join(first, ",") {
			t.Fatalf("non-deterministic order: run0=%v run1=%v", first, titles)
		}
	}
}
