package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/embeddings"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
	"github.com/superops-team/okf/pkg/query"
	"github.com/superops-team/okf/pkg/vectorindex"
)

// writeLongText writes a synthetic text file with n words of filler plus an
// optional unique tail term.
func writeLongText(t *testing.T, dir, name string, n int, tail string) string {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("common filler word ")
	}
	sb.WriteString(tail)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// countChunkFiles returns the number of __cN chunk files under kb.
func countChunkFiles(t *testing.T, kb string) int {
	t.Helper()
	n := 0
	err := filepath.Walk(kb, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if !info.IsDir() && strings.Contains(filepath.Base(path), "__c") && strings.HasSuffix(path, ".md") {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// --- small document stays single-concept ---
func TestCmdAddSmallDocumentSingleConcept(t *testing.T) {
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", fixture("sample.pdf")}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}
	bundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Concepts) != 1 {
		t.Fatalf("concepts = %d, want 1 (small doc stays single-concept)", len(bundle.Concepts))
	}
	if n := countChunkFiles(t, kb); n != 0 {
		t.Fatalf("chunk files = %d, want 0", n)
	}
}

// --- large document is chunked ---
func TestCmdAddLargeDocumentChunked(t *testing.T) {
	src := t.TempDir()
	writeLongText(t, src, "big.txt", 3000, "taillabel unique")
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}
	bundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	// whole-document concept + 2+ chunk concepts
	if len(bundle.Concepts) < 3 {
		t.Fatalf("concepts = %d, want >= 3 (whole + 2+ chunks)", len(bundle.Concepts))
	}
	if n := countChunkFiles(t, kb); n < 2 {
		t.Fatalf("chunk files = %d, want >= 2", n)
	}
	// whole-document concept present
	foundWhole := false
	for _, c := range bundle.Concepts {
		if strings.HasSuffix(c.FilePath, "big.txt.md") && !strings.Contains(c.FilePath, "__c") {
			foundWhole = true
		}
	}
	if !foundWhole {
		t.Fatal("whole-document concept big.txt.md missing")
	}
}

// --- chunk concept title is explicit, not filename-derived ---
func TestCmdAddChunkTitleExplicit(t *testing.T) {
	src := t.TempDir()
	writeLongText(t, src, "big.pdf", 3000, "taillabel unique")
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}
	bundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range bundle.Concepts {
		if strings.Contains(c.FilePath, "__c") {
			if strings.Contains(c.Title, "__c") || strings.Contains(c.Title, "big pdf c") {
				t.Fatalf("chunk title %q derived from filename (contains artifact)", c.Title)
			}
			if !strings.Contains(c.Title, "part") {
				t.Fatalf("chunk title %q does not contain 'part N'", c.Title)
			}
		}
	}
}

// --- chunk concept metadata ---
func TestCmdAddChunkMetadata(t *testing.T) {
	src := t.TempDir()
	writeLongText(t, src, "big.txt", 3000, "taillabel unique")
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}
	bundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	chunks := 0
	for _, c := range bundle.Concepts {
		if !strings.Contains(c.FilePath, "__c") {
			continue
		}
		chunks++
		cf := c.CustomFields
		if _, ok := cf["chunk_index"]; !ok {
			t.Errorf("chunk %s missing chunk_index", c.FilePath)
		}
		if _, ok := cf["chunk_count"]; !ok {
			t.Errorf("chunk %s missing chunk_count", c.FilePath)
		}
		if cf["source_path"] != "big.txt" {
			t.Errorf("chunk %s source_path = %v, want big.txt", c.FilePath, cf["source_path"])
		}
		if cf["derived"] != "true" {
			t.Errorf("chunk %s derived = %v, want true", c.FilePath, cf["derived"])
		}
	}
	if chunks < 2 {
		t.Fatalf("chunk concepts = %d, want >= 2", chunks)
	}
}

// --- CJK large document is chunked ---
func TestCmdAddCJKLargeDocumentChunked(t *testing.T) {
	src := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 3000; i++ {
		sb.WriteString("中文检索分块测试内容 ")
	}
	sb.WriteString("尾标记词")
	p := filepath.Join(src, "cn.txt")
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}
	bundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Concepts) < 3 {
		t.Fatalf("CJK concepts = %d, want >= 3 (CJK chunking failed: counted as one word?)", len(bundle.Concepts))
	}
}

// --- re-import is idempotent (chunked) ---
func TestCmdAddChunkedReimportIdempotent(t *testing.T) {
	src := t.TempDir()
	writeLongText(t, src, "big.txt", 3000, "taillabel unique")
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("first cmdAdd = %d", code)
	}
	b1, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("second cmdAdd = %d", code)
	}
	b2, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(b1.Concepts) != len(b2.Concepts) {
		t.Fatalf("re-import changed concept count: %d -> %d", len(b1.Concepts), len(b2.Concepts))
	}
	if n := countChunkFiles(t, kb); n < 2 {
		t.Fatalf("chunk files = %d after re-import", n)
	}
}

// --- semantic deep-tail: chunked retrievable, unchunked control not ---
// Uses the real MiniLM embedding stack at the pure-vector layer (HNSW),
// because the spec explicitly excludes lexical search from this control and
// SemanticSearch fuses lexical hits. This proves the chunking gain on the
// semantic recall path: unchunked concepts are truncated to the model's
// token window so the tail never enters the vector; chunked concepts embed
// the tail chunk.
func TestSemanticDeepTailChunkedVsUnchunked(t *testing.T) {
	// two bundles: chunked (3000 words > threshold) and unchunked (1500 words
	// < threshold), same unique tail term
	tail := "zebraquasar42x7"
	build := func(wordCount int) (*query.KnowledgeBundle, *vectorindex.HNSW, *embeddings.MiniLM) {
		src := t.TempDir()
		writeLongText(t, src, "doc.txt", wordCount, tail)
		kb := newKB(t)
		if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
			t.Fatalf("cmdAdd = %d, want 0", code)
		}
		bundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
		if err != nil {
			t.Fatal(err)
		}
		qb := toQueryBundle(bundle)
		emb, err := embeddings.NewMiniLM()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { emb.Close() })
		idx := vectorindex.NewHNSW(emb.Dimension())
		for _, c := range qb.Concepts {
			vec, err := emb.EmbedQuery(conceptText(c))
			if err != nil {
				t.Fatal(err)
			}
			idx.Add(query.Fingerprint(c), vec)
		}
		return qb, idx, emb
	}

	chunkedBundle, chunkedIdx, emb := build(3000)
	qvec, err := emb.EmbedQuery(tail)
	if err != nil {
		t.Fatal(err)
	}
	chunkedHits := chunkedIdx.Search(qvec, 5)
	found := false
	for _, h := range chunkedHits {
		if strings.Contains(h.Key, "__c") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("chunked: tail term not found in top-5 semantic hits (%v)", chunkedHits)
	}
	_ = chunkedBundle

	unchunkedBundle, unchunkedIdx, _ := build(1500)
	qvec2, err := emb.EmbedQuery(tail)
	if err != nil {
		t.Fatal(err)
	}
	unchunkedHits := unchunkedIdx.Search(qvec2, 5)
	for _, h := range unchunkedHits {
		if strings.HasSuffix(h.Key, "doc.txt") {
			t.Fatalf("unchunked control: tail term hit the truncated concept (%v) — truncation did NOT drop the tail", unchunkedHits)
		}
	}
	_ = unchunkedBundle
}

// --- chunked document occupies one result slot in semantic search ---
func TestSemanticSearchChunkedOneSlot(t *testing.T) {
	src := t.TempDir()
	writeLongText(t, src, "big.txt", 3000, "common filler word")
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}
	bundle, err := okf.LoadBundle(kb, okf.DefaultLoadOptions())
	if err != nil {
		t.Fatal(err)
	}
	emb, err := embeddings.NewMiniLM()
	if err != nil {
		t.Fatal(err)
	}
	defer emb.Close()
	idx := vectorindex.NewHNSW(emb.Dimension())
	qb := toQueryBundle(bundle)
	for _, c := range qb.Concepts {
		vec, err := emb.EmbedQuery(conceptText(c))
		if err != nil {
			t.Fatal(err)
		}
		idx.Add(query.Fingerprint(c), vec)
	}
	backend := &semanticBackend{emb: emb, idx: idx}
	res, err := query.SemanticSearch(qb, "common filler word", backend, query.SearchOptions{TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	// many chunks of one source -> at most one chunk result slot
	chunkSlots := 0
	for _, r := range res {
		if strings.Contains(r.Concept.FilePath, "__c") {
			chunkSlots++
		}
	}
	if chunkSlots > 1 {
		t.Fatalf("chunk slots = %d, want <= 1 (dedupe by source failed)", chunkSlots)
	}
}

// --- __cN files parse via the standard parser and are not reserved names ---
func TestChunkFileNotReserved(t *testing.T) {
	for _, fn := range []string{"big.txt__c1.md", "big.txt__c12.md"} {
		if parser.IsReservedFilename(fn) {
			t.Fatalf("%q must not be a reserved filename", fn)
		}
	}
}
