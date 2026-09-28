package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/memorydefense"
)

func writeMD(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScreenImportTreeClean(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "a.md", "# Hello\nNormal content about PostgreSQL.")
	pol := memorydefense.Policy{Enabled: true, Action: "redact"}
	if err := screenImportTree(dir, pol); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	if !strings.Contains(string(data), "PostgreSQL") {
		t.Error("normal content lost")
	}
}

func TestScreenImportTreeRedact(t *testing.T) {
	dir := t.TempDir()
	secret := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234"
	writeMD(t, dir, "a.md", "Token: "+secret)
	pol := memorydefense.Policy{Enabled: true, Action: "redact"}
	if err := screenImportTree(dir, pol); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	if strings.Contains(string(data), secret) {
		t.Error("original secret leaked")
	}
	if !strings.Contains(string(data), "[REDACTED:github_pat]") {
		t.Error("expected redaction placeholder")
	}
}

func TestScreenImportTreeBlock(t *testing.T) {
	dir := t.TempDir()
	secret := "AKIAIOSFODNN7EXAMPLE"
	writeMD(t, dir, "a.md", "AWS key: "+secret)
	pol := memorydefense.Policy{Enabled: true, Action: "block"}
	err := screenImportTree(dir, pol)
	if err == nil {
		t.Fatal("expected block error")
	}
	var blocked *memorydefense.ErrBlocked
	if !asBlocked(err, &blocked) {
		t.Fatalf("expected ErrBlocked, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Error("error leaked secret text")
	}
}

func TestScreenImportTreeDisabled(t *testing.T) {
	dir := t.TempDir()
	secret := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234"
	path := writeMD(t, dir, "a.md", "Token: "+secret)
	original, _ := os.ReadFile(path)
	pol := memorydefense.Policy{Enabled: false, Action: "redact"}
	if err := screenImportTree(dir, pol); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Error("disabled policy must be byte-for-byte identical")
	}
}

func TestScreenImportTreeMixedBatch(t *testing.T) {
	dir := t.TempDir()
	// File 1: needs redact (medium severity credit card)
	writeMD(t, dir, "ok.md", "Card 4111-1111-1111-1111")
	// File 2: needs block (high severity AWS key)
	writeMD(t, dir, "bad.md", "AWS AKIAIOSFODNN7EXAMPLE")
	pol := memorydefense.Policy{Enabled: true, Action: "block"}
	err := screenImportTree(dir, pol)
	if err == nil {
		t.Fatal("expected block error from bad.md")
	}
	// In block mode, first error aborts; ok.md may or may not be screened.
	// The staging dir cleanup (defer) handles rollback at the caller level.
}

func TestScreenImportTreeSingleFile(t *testing.T) {
	dir := t.TempDir()
	secret := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234"
	path := writeMD(t, dir, "single.md", "Token "+secret)
	pol := memorydefense.Policy{Enabled: true, Action: "redact"}
	if err := screenImportTree(path, pol); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), secret) {
		t.Error("single-file redact leaked")
	}
}

// asBlocked checks if err wraps *ErrBlocked.
func asBlocked(err error, target **memorydefense.ErrBlocked) bool {
	for err != nil {
		if b, ok := err.(*memorydefense.ErrBlocked); ok {
			*target = b
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
