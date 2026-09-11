package git

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveKnowledgeFileRemovesDerivedChunks: when a source concept is
// removed by sync, its derived __cN chunk files are removed together
// (derived: true + source_path drive the lifecycle), while unrelated files
// and human-owned concepts are preserved.
func TestRemoveKnowledgeFileRemovesDerivedChunks(t *testing.T) {
	kb := t.TempDir()

	// source concept with trusted generated metadata (okf.git)
	srcMD := "---\ntype: source\ntitle: \"big\"\ngenerated: true\ngenerator: okf.git\nsource_path: big.txt\n---\nwhole body\n"
	if err := os.WriteFile(filepath.Join(kb, "big.txt.md"), []byte(srcMD), 0o644); err != nil {
		t.Fatal(err)
	}
	// derived chunks
	chunkMD := "---\ntype: source\ntitle: \"big — part 1\"\nchunk_index: 0\nchunk_count: 2\nderived: \"true\"\nsource_path: big.txt\n---\nchunk body\n"
	if err := os.WriteFile(filepath.Join(kb, "big.txt__c1.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "big.txt__c2.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}
	// unrelated human-owned concept must survive
	other := "---\ntype: note\ntitle: \"Notes\"\n---\nhuman content\n"
	if err := os.WriteFile(filepath.Join(kb, "notes.md"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	// a different document's chunk must survive (different source_path)
	otherChunk := "---\ntype: source\ntitle: \"other — part 1\"\nderived: \"true\"\nsource_path: other.pdf\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(kb, "other.pdf__c1.md"), []byte(otherChunk), 0o644); err != nil {
		t.Fatal(err)
	}

	removeKnowledgeFile(kb, "big.txt")

	for _, gone := range []string{"big.txt.md", "big.txt__c1.md", "big.txt__c2.md"} {
		if _, err := os.Stat(filepath.Join(kb, gone)); !os.IsNotExist(err) {
			t.Errorf("%s still exists after removeKnowledgeFile", gone)
		}
	}
	for _, kept := range []string{"notes.md", "other.pdf__c1.md"} {
		if _, err := os.Stat(filepath.Join(kb, kept)); err != nil {
			t.Errorf("%s was removed, want preserved", kept)
		}
	}
}

// TestRemoveKnowledgeFileKeepsHumanChunkEdits: a chunk file whose metadata was
// edited away (derived removed) is no longer treated as derived — the file
// stays. (Lifecycle is metadata-driven, not name-driven.)
func TestRemoveKnowledgeFileKeepsEditedChunk(t *testing.T) {
	kb := t.TempDir()
	srcMD := "---\ntype: source\ntitle: \"big\"\ngenerated: true\ngenerator: okf.git\nsource_path: big.txt\n---\nwhole\n"
	if err := os.WriteFile(filepath.Join(kb, "big.txt.md"), []byte(srcMD), 0o644); err != nil {
		t.Fatal(err)
	}
	// chunk whose derived marker was removed by a user edit
	edited := "---\ntype: source\ntitle: \"big — part 1\"\nsource_path: big.txt\n---\nuser edited chunk\n"
	if err := os.WriteFile(filepath.Join(kb, "big.txt__c1.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	removeKnowledgeFile(kb, "big.txt")

	if _, err := os.Stat(filepath.Join(kb, "big.txt.md")); !os.IsNotExist(err) {
		t.Error("source concept should be removed")
	}
	if _, err := os.Stat(filepath.Join(kb, "big.txt__c1.md")); err != nil {
		t.Error("edited chunk (derived marker removed) should be preserved")
	}
}

// TestRemoveKnowledgeFileNestedSource: chunk files in a subdirectory are
// removed together with a nested source path.
func TestRemoveKnowledgeFileNestedSource(t *testing.T) {
	kb := t.TempDir()
	sub := filepath.Join(kb, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	srcMD := "---\ntype: source\ntitle: \"n\"\ngenerated: true\ngenerator: okf.git\nsource_path: docs/n.pdf\n---\nwhole\n"
	if err := os.WriteFile(filepath.Join(sub, "n.pdf.md"), []byte(srcMD), 0o644); err != nil {
		t.Fatal(err)
	}
	chunkMD := "---\ntype: source\ntitle: \"n — part 1\"\nderived: \"true\"\nsource_path: docs/n.pdf\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(sub, "n.pdf__c1.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	removeKnowledgeFile(kb, "docs/n.pdf")

	if _, err := os.Stat(filepath.Join(sub, "n.pdf.md")); !os.IsNotExist(err) {
		t.Error("nested source concept should be removed")
	}
	if _, err := os.Stat(filepath.Join(sub, "n.pdf__c1.md")); !os.IsNotExist(err) {
		t.Error("nested chunk should be removed")
	}
}
