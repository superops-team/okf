package vectorindex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddSearchOrder(t *testing.T) {
	idx := NewHNSW(4)
	idx.Add("doc1", []float32{1, 0, 0, 0})
	idx.Add("doc2", []float32{0.9, 0.1, 0, 0})
	idx.Add("doc3", []float32{0, 0, 1, 0})

	got := idx.Search([]float32{1, 0, 0, 0}, 3)
	if len(got) != 3 {
		t.Fatalf("got %d hits, want 3", len(got))
	}
	if got[0].Key != "doc1" {
		t.Fatalf("top = %q, want doc1", got[0].Key)
	}
	if got[1].Key != "doc2" || got[2].Key != "doc3" {
		t.Fatalf("order = %q,%q,%q", got[0].Key, got[1].Key, got[2].Key)
	}
	if got[0].Score <= got[1].Score || got[1].Score <= got[2].Score {
		t.Fatalf("scores not descending: %v %v %v", got[0].Score, got[1].Score, got[2].Score)
	}
	if idx.Len() != 3 {
		t.Fatalf("Len = %d, want 3", idx.Len())
	}
}

func TestAddSameKeyIsIdempotent(t *testing.T) {
	idx := NewHNSW(2)
	idx.Add("k", []float32{1, 0})
	idx.Add("k", []float32{0, 1}) // 重复 Add 应幂等跳过，不 panic
	if idx.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (去重)", idx.Len())
	}
	got := idx.Search([]float32{1, 0}, 1)
	if got[0].Key != "k" {
		t.Fatalf("hit = %q", got[0].Key)
	}
}

func TestRemove(t *testing.T) {
	idx := NewHNSW(2)
	idx.Add("a", []float32{1, 0})
	idx.Add("b", []float32{0, 1})
	if !idx.Remove("a") {
		t.Fatal("Remove(a) = false")
	}
	if idx.Remove("a") {
		t.Fatal("Remove(a) again = true")
	}
	if idx.Len() != 1 {
		t.Fatalf("Len = %d, want 1", idx.Len())
	}
	got := idx.Search([]float32{0, 1}, 1)
	if got[0].Key != "b" {
		t.Fatalf("hit = %q, want b", got[0].Key)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(3)
	idx.Add("x", []float32{1, 0, 0})
	idx.Add("y", []float32{0, 1, 0})
	meta := Meta{Model: "minilm-int8", OkfVersion: "0.4.0"}
	if err := idx.Save(dir, meta); err != nil {
		t.Fatal(err)
	}

	loaded := NewHNSW(3)
	gotMeta, err := loaded.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if gotMeta.Dims != 3 || gotMeta.Count != 2 || gotMeta.Model != "minilm-int8" {
		t.Fatalf("meta = %+v", gotMeta)
	}
	if loaded.Len() != 2 {
		t.Fatalf("Len = %d, want 2", loaded.Len())
	}
	got := loaded.Search([]float32{0, 1, 0}, 1)
	if got[0].Key != "y" {
		t.Fatalf("hit = %q, want y", got[0].Key)
	}
}

func TestLoadMetaMismatch(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(3)
	idx.Add("x", []float32{1, 0, 0})
	if err := idx.Save(dir, Meta{Model: "m", OkfVersion: "v"}); err != nil {
		t.Fatal(err)
	}
	// 维度不一致应报错（提示 rebuild）
	loaded := NewHNSW(384)
	_, err := loaded.Load(dir)
	if err == nil {
		t.Fatal("维度不一致未报错")
	}
}

func TestSaveLoadPersistedFiles(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(2)
	idx.Add("a", []float32{1, 0})
	if err := idx.Save(dir, Meta{Model: "m", OkfVersion: "v"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{IndexFileName, MetaFileName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s 缺失: %v", name, err)
		}
	}
}
