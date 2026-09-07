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
// Gap 3: okf sync -prune end-to-end (metadata cleared for missing source)
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

	// Known spec↔implementation gap (recorded, not asserted here):
	// sync -prune clears metadata but does NOT delete disk files (whole concept
	// or derived __cN chunks). Disk-file deletion happens via the SaveKnowledgeBase
	// incremental-update path (removeKnowledgeFile + removeDerivedChunks), not via
	// sync. Spec scenario "chunk files are derived artifacts — okf sync removes them"
	// is therefore only verified at the removeDerivedChunks unit layer
	// (TestRemoveKnowledgeFileRemovesDerivedChunks), not end-to-end through sync.
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
