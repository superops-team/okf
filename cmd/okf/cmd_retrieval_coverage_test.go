package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// Coverage-gap tests for M1 retrieval quality closed loop.
//
// These tests anchor behaviors that were implemented but lacked dedicated
// test wiring (AGENTS.md §17: no wiring + test = not done).

// =============================================================================
// Gap 5: ChunkThreshold boundary (exactly 2000 = not chunked; 2001 = chunked)
// =============================================================================

// writeExactWords writes exactly n English words (space-separated) so
// convert.countWords returns n precisely (no punctuation, no CJK).
func writeExactWords(t *testing.T, dir, name string, n int) string {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString("word")
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCmdAddChunkThresholdBoundary(t *testing.T) {
	// 2000 words: exactly at threshold — NOT chunked (Words > ChunkThreshold is false)
	src := t.TempDir()
	writeExactWords(t, src, "exact2000.txt", 2000)
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd(2000 words) = %d, want 0", code)
	}
	if n := countChunkFiles(t, kb); n != 0 {
		t.Fatalf("2000 words (at threshold): chunk files = %d, want 0", n)
	}

	// 2001 words: one over threshold — MUST be chunked
	src2 := t.TempDir()
	writeExactWords(t, src2, "over2001.txt", 2001)
	kb2 := newKB(t)
	if code := cmdAdd([]string{"-dir", kb2, "-silent", src2}); code != 0 {
		t.Fatalf("cmdAdd(2001 words) = %d, want 0", code)
	}
	if n := countChunkFiles(t, kb2); n < 1 {
		t.Fatalf("2001 words (over threshold): chunk files = %d, want >= 1", n)
	}
}

// =============================================================================
// Gap 3: okf sync -prune end-to-end (metadata cleared + generated disk files
// removed: whole concept AND derived __cN chunks)
// =============================================================================

func TestCmdSyncPruneClearsMetadataForMissingSource(t *testing.T) {
	src := t.TempDir()
	writeLongText(t, src, "big.txt", 3000, "tailword")
	kb := newKB(t)
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}

	// metadata has the source after add
	metaPath := okf.KnowledgeMetadataPath(kb)
	idx := okf.NewMetadataIndex()
	if err := idx.Load(metaPath); err != nil {
		t.Fatal(err)
	}
	if idx.Len() == 0 {
		t.Fatal("metadata empty after add")
	}
	// disk has whole + chunk files after add
	wholePath := filepath.Join(kb, "big.txt.md")
	if _, err := os.Stat(wholePath); err != nil {
		t.Fatalf("whole concept %s missing after add: %v", wholePath, err)
	}
	if n := countChunkFiles(t, kb); n < 1 {
		t.Fatalf("chunk files after add = %d, want >= 1", n)
	}

	// remove the source file from disk
	if err := os.Remove(filepath.Join(src, "big.txt")); err != nil {
		t.Fatal(err)
	}

	// sync -prune clears metadata for the missing source
	if code := cmdSync([]string{"-dir", kb, "-prune", "-silent"}); code != 0 {
		t.Fatalf("cmdSync -prune = %d, want 0", code)
	}
	idx2 := okf.NewMetadataIndex()
	if err := idx2.Load(metaPath); err != nil {
		t.Fatal(err)
	}
	if idx2.Len() != 0 {
		t.Fatalf("metadata after sync -prune = %d entries, want 0", idx2.Len())
	}
	// generated disk files removed too (whole concept + derived chunks)
	if _, err := os.Stat(wholePath); !os.IsNotExist(err) {
		t.Fatalf("whole concept %s still on disk after sync -prune (want removed)", wholePath)
	}
	if n := countChunkFiles(t, kb); n != 0 {
		t.Fatalf("chunk files after sync -prune = %d, want 0 (derived chunks removed with source)", n)
	}
}

// TestCmdSyncPruneKeepsAuthorOwnedFiles: sync -prune must NOT delete concepts
// without trusted generated metadata (hand-written knowledge is author-owned).
func TestCmdSyncPruneKeepsAuthorOwnedFiles(t *testing.T) {
	kb := newKB(t)
	// hand-written concept (no generated marker)
	authorFile := filepath.Join(kb, "notes.md")
	if err := os.WriteFile(authorFile, []byte("---\ntitle: My Notes\n---\nhand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// an indexed source that disappears
	src := t.TempDir()
	writeLongText(t, src, "big.txt", 3000, "tailword")
	if code := cmdAdd([]string{"-dir", kb, "-silent", src}); code != 0 {
		t.Fatalf("cmdAdd = %d, want 0", code)
	}
	if err := os.Remove(filepath.Join(src, "big.txt")); err != nil {
		t.Fatal(err)
	}
	if code := cmdSync([]string{"-dir", kb, "-prune", "-silent"}); code != 0 {
		t.Fatalf("cmdSync -prune = %d, want 0", code)
	}
	if _, err := os.Stat(authorFile); err != nil {
		t.Fatalf("author-owned notes.md was deleted by sync -prune (want kept): %v", err)
	}
}

// =============================================================================
// Gap 2: CLI semantic search outputs dup=N for chunked documents
// =============================================================================

// TestCLISemanticSearchOutputsDupCount uses the real binary (buildOKF
// singleton) + real MiniLM vector index to assert the CLI text output contains
// "dup=N" when a chunked document is retrieved (DuplicateCount > 0).
func TestCLISemanticSearchOutputsDupCount(t *testing.T) {
	bin := buildOKF(t)
	kb := filepath.Join(t.TempDir(), "kb")
	if err := os.MkdirAll(kb, 0o755); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	writeLongText(t, src, "big.txt", 3000, "zebraquasar42x7")

	runOKF(t, bin, "add", "-dir", kb, "-silent", src)
	runOKF(t, bin, "vector", "index", "-path", kb)

	out := runOKF(t, bin, "search", "-path", kb, "-q", "zebraquasar42x7", "-semantic")
	if !strings.Contains(out, "dup=") {
		t.Fatalf("semantic search output missing 'dup=' for chunked doc:\n%s", out)
	}
}
