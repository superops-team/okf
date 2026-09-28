package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// writeImportFixture creates a temp txt file with optional secret content.
func writeImportFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeDefenseConfig writes .okf/config.yaml with the given action.
func writeDefenseConfig(t *testing.T, repoRoot, action string) {
	t.Helper()
	dir := filepath.Join(repoRoot, ".okf")
	os.MkdirAll(dir, 0o755)
	cfg := "memory_defense:\n  enabled: true\n  action: \"" + action + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestImportDocumentDefenseDisabled: default (no config) = secrets pass through.
func TestImportDocumentDefenseDisabled(t *testing.T) {
	kb := filepath.Join(t.TempDir(), "repo", "knowledge")
	os.MkdirAll(kb, 0o755)
	r := NewToolRegistry()
	r.SetBundle(&okf.KnowledgeBundle{}, kb)

	secret := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234"
	fixture := writeImportFixture(t, "My token is "+secret)
	res, err := r.Call("okf_import_document", map[string]interface{}{"path": fixture})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected success (defense disabled), got: %s", res.Content[0].Text)
	}
	prod := filepath.Join(kb, "note.txt.md")
	data, _ := os.ReadFile(prod)
	if !strings.Contains(string(data), secret) {
		t.Error("secret should pass through when defense is disabled")
	}
}

// TestImportDocumentDefenseBlock: enabled+block rejects import with secret.
func TestImportDocumentDefenseBlock(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	kb := filepath.Join(repoRoot, "knowledge")
	os.MkdirAll(kb, 0o755)
	writeDefenseConfig(t, repoRoot, "block")

	r := NewToolRegistry()
	r.SetBundle(&okf.KnowledgeBundle{}, kb)

	fixture := writeImportFixture(t, "token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234")
	res, err := r.Call("okf_import_document", map[string]interface{}{"path": fixture})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected block error, got success")
	}
	// Error must not leak the original secret.
	if strings.Contains(res.Content[0].Text, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Error("error message leaked the secret text")
	}
	// No file on disk.
	if _, err := os.Stat(filepath.Join(kb, "note.txt.md")); !os.IsNotExist(err) {
		t.Error("blocked import should not write markdown")
	}
}

// TestImportDocumentDefenseRedact: enabled+redact replaces secrets in output.
func TestImportDocumentDefenseRedact(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	kb := filepath.Join(repoRoot, "knowledge")
	os.MkdirAll(kb, 0o755)
	writeDefenseConfig(t, repoRoot, "redact")

	r := NewToolRegistry()
	r.SetBundle(&okf.KnowledgeBundle{}, kb)

	secret := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234"
	fixture := writeImportFixture(t, "token "+secret+" here")
	res, err := r.Call("okf_import_document", map[string]interface{}{"path": fixture})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("expected redact success, got: %s", res.Content[0].Text)
	}
	prod := filepath.Join(kb, "note.txt.md")
	data, _ := os.ReadFile(prod)
	if strings.Contains(string(data), secret) {
		t.Error("original secret leaked into markdown")
	}
	if !strings.Contains(string(data), "[REDACTED:github_pat]") {
		t.Error("expected [REDACTED:github_pat] in markdown")
	}
}

// TestImportDocumentDefenseClean: normal document unaffected when defense enabled.
func TestImportDocumentDefenseClean(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	kb := filepath.Join(repoRoot, "knowledge")
	os.MkdirAll(kb, 0o755)
	writeDefenseConfig(t, repoRoot, "redact")

	r := NewToolRegistry()
	r.SetBundle(&okf.KnowledgeBundle{}, kb)

	fixture := writeImportFixture(t, "This is a normal note about PostgreSQL deployment.")
	res, err := r.Call("okf_import_document", map[string]interface{}{"path": fixture})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("normal doc should not be blocked: %s", res.Content[0].Text)
	}
	prod := filepath.Join(kb, "note.txt.md")
	data, _ := os.ReadFile(prod)
	if !strings.Contains(string(data), "PostgreSQL deployment") {
		t.Error("normal content should be preserved")
	}
}
