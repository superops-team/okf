package git

import (
	"path/filepath"
	"testing"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

// S12: generated code concepts persisted by SaveKnowledgeBase receive a valid
// okf_id at the final destination, and a re-save (refresh) preserves it.
func TestOwnedWritersPersistStableID(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{RepoPath: dir, KnowledgeDir: dir}

	mainPath := "code_files/main.md"
	concepts := []*okf.Concept{
		{Type: "code_file", Title: "main", FilePath: mainPath, Content: "package main",
			CustomFields: map[string]any{"source_path": "main.go", "generator": "okf.git"}},
	}
	bundle := okf.NewBundle("test")
	bundle.Concepts = concepts

	if _, err := SaveKnowledgeBase(bundle, cfg); err != nil {
		t.Fatalf("SaveKnowledgeBase: %v", err)
	}
	readID := func() string {
		t.Helper()
		pc, err := parser.ParseConcept(filepath.Join(dir, mainPath))
		if err != nil {
			t.Fatalf("parse %s: %v", mainPath, err)
		}
		raw, _ := pc.CustomFields[identity.Field].(string)
		if _, err := identity.Parse(raw); err != nil {
			t.Fatalf("persisted invalid/missing okf_id %q: %v", raw, err)
		}
		return raw
	}
	idBefore := readID()

	// Refresh: rebuild a fresh in-memory concept (no okf_id) and save to the same
	// final target; the on-disk id must be preserved.
	refreshed := okf.NewBundle("test")
	refreshed.Concepts = []*okf.Concept{
		{Type: "code_file", Title: "main", FilePath: mainPath, Content: "package main // changed",
			CustomFields: map[string]any{"source_path": "main.go", "generator": "okf.git"}},
	}
	if _, err := SaveKnowledgeBase(refreshed, cfg); err != nil {
		t.Fatalf("refresh SaveKnowledgeBase: %v", err)
	}
	if idAfter := readID(); idAfter != idBefore {
		t.Fatalf("refresh changed okf_id: before=%q after=%q", idBefore, idAfter)
	}
}
