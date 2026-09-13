package identity

import (
	"os"
	"path/filepath"
	"testing"
)

// S16: when a vector index is present and at least one concept needs a planned
// id addition, both dry-run and apply report vector_rebuild_required=true.
func TestIdentityMigrationReportsRebuild(t *testing.T) {
	t.Run("dry_run_with_index", func(t *testing.T) {
		root := t.TempDir()
		writeConceptFile(t, root, "a.md", "")
		writeConceptFile(t, root, "stable.md", "okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e\n")
		// Simulate an existing vector index.
		metaDir := filepath.Join(root, ".okf", "vector")
		if err := os.MkdirAll(metaDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(metaDir, "index.meta.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		report, err := Ensure(root, false)
		if err != nil {
			t.Fatalf("dry-run: %v", err)
		}
		if report.Missing != 1 {
			t.Fatalf("missing=%d, want 1", report.Missing)
		}
		if !report.VectorRebuildRequired {
			t.Fatal("dry-run with present index and planned additions must report vector_rebuild_required=true")
		}
	})

	t.Run("apply_with_index", func(t *testing.T) {
		root := t.TempDir()
		writeConceptFile(t, root, "a.md", "")
		metaDir := filepath.Join(root, ".okf", "vector")
		if err := os.MkdirAll(metaDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(metaDir, "index.meta.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}

		report, err := Ensure(root, true)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if !report.VectorRebuildRequired {
			t.Fatal("apply with present index and planned additions must report vector_rebuild_required=true")
		}
	})

	t.Run("no_index_no_flag", func(t *testing.T) {
		root := t.TempDir()
		writeConceptFile(t, root, "a.md", "")
		report, err := Ensure(root, false)
		if err != nil {
			t.Fatalf("dry-run: %v", err)
		}
		if report.VectorRebuildRequired {
			t.Fatal("no vector index present: vector_rebuild_required must be false")
		}
	})

	t.Run("index_but_no_missing_no_flag", func(t *testing.T) {
		root := t.TempDir()
		writeConceptFile(t, root, "stable.md", "okf_id: okf_17a2c56db85c4889b4f8fe02ca9ac67e\n")
		metaDir := filepath.Join(root, ".okf", "vector")
		if err := os.MkdirAll(metaDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(metaDir, "index.meta.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
		report, err := Ensure(root, false)
		if err != nil {
			t.Fatalf("dry-run: %v", err)
		}
		if report.Missing != 0 {
			t.Fatalf("missing=%d, want 0", report.Missing)
		}
		if report.VectorRebuildRequired {
			t.Fatal("index present but no planned additions: vector_rebuild_required must be false")
		}
	})
}
