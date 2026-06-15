package okf

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var benchmarkBundleSink *KnowledgeBundle

func TestSaveBundleLoadBundleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	bundle := NewBundle("demo")
	first := NewConcept("api", "Users API")
	first.Description = "Handles users"
	first.Resource = "api/users"
	first.Tags = []string{"go"}
	first.Content = "## Users\nBody"
	second := NewConcept("api", "Users API")
	second.Description = "Duplicate title with unique path"
	second.Content = "second body"
	bundle.AddConcept(first)
	bundle.AddConcept(second)

	if err := SaveBundle(bundle, dir, DefaultSaveOptions()); err != nil {
		t.Fatalf("SaveBundle returned error: %v", err)
	}

	loaded, err := LoadBundle(dir, DefaultLoadOptions())
	if err != nil {
		t.Fatalf("LoadBundle returned error: %v", err)
	}
	if len(loaded.Concepts) != 2 {
		t.Fatalf("loaded %d concepts, want 2", len(loaded.Concepts))
	}
	if _, err := os.Stat(filepath.Join(dir, "apis", "Users API.md")); err != nil {
		t.Fatalf("expected first file to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "apis", "Users API_1.md")); err != nil {
		t.Fatalf("expected duplicate file to exist: %v", err)
	}
}

func TestSaveBundleLoadBundlePreservesCustomFields(t *testing.T) {
	dir := t.TempDir()
	bundle := NewBundle("generated")
	concept := NewConcept("code_file", "handler.go")
	concept.CustomFields = map[string]interface{}{
		"generated":         true,
		"generator":         "okf.git",
		"generator_version": 1,
		"source_path":       "internal/handler.go",
		"source_kind":       "file",
		"source_commit":     "abc123",
	}
	bundle.AddConcept(concept)

	if err := SaveBundle(bundle, dir, DefaultSaveOptions()); err != nil {
		t.Fatalf("SaveBundle returned error: %v", err)
	}

	loaded, err := LoadBundle(dir, DefaultLoadOptions())
	if err != nil {
		t.Fatalf("LoadBundle returned error: %v", err)
	}
	if len(loaded.Concepts) != 1 {
		t.Fatalf("loaded %d concepts, want 1", len(loaded.Concepts))
	}
	fields := loaded.Concepts[0].CustomFields
	if fields["generated"] != true || fields["generator"] != "okf.git" || fields["source_path"] != "internal/handler.go" {
		t.Fatalf("custom fields = %#v, want generated okf.git internal/handler.go", fields)
	}
}

func BenchmarkSaveLoadRoundTrip(b *testing.B) {
	bundle := NewBundle("bench")
	for i := 0; i < 200; i++ {
		concept := NewConcept("component", fmt.Sprintf("Component %03d", i))
		concept.Description = fmt.Sprintf("Benchmark concept %03d", i)
		concept.Resource = fmt.Sprintf("src/component_%03d.go", i)
		concept.Tags = []string{"go", "benchmark"}
		concept.Content = fmt.Sprintf("## Component %03d\n\nGenerated benchmark content.\n", i)
		bundle.AddConcept(concept)
	}
	root := b.TempDir()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dir := filepath.Join(root, fmt.Sprintf("round_%06d", i))
		if err := SaveBundle(bundle, dir, DefaultSaveOptions()); err != nil {
			b.Fatal(err)
		}
		loaded, err := LoadBundle(dir, DefaultLoadOptions())
		if err != nil {
			b.Fatal(err)
		}
		if len(loaded.Concepts) != len(bundle.Concepts) {
			b.Fatalf("loaded %d concepts, want %d", len(loaded.Concepts), len(bundle.Concepts))
		}
		benchmarkBundleSink = loaded
	}
}
