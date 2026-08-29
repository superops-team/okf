package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

const mcpFixtureDir = "../../pkg/convert/testdata"

func mcpFixture(name string) string { return filepath.Join(mcpFixtureDir, name) }

// S22: the tool is registered with the right schema
func TestMCPSchemaHasImportDocument(t *testing.T) {
	r := NewToolRegistry()
	found := false
	for _, tool := range r.List() {
		if tool.Name == "okf_import_document" {
			found = true
			props, _ := tool.InputSchema["properties"].(map[string]interface{})
			if props == nil {
				t.Fatalf("input schema properties missing")
			}
			for _, p := range []string{"path", "title", "type"} {
				if _, ok := props[p]; !ok {
					t.Errorf("schema missing property %q", p)
				}
			}
			req, _ := tool.InputSchema["required"].([]string)
			if len(req) != 1 || req[0] != "path" {
				t.Errorf("required = %v, want [path]", req)
			}
		}
	}
	if !found {
		t.Fatal("okf_import_document not registered")
	}
}

// S23: importing without a loaded bundle fails clearly
func TestMCPImportRequiresBundle(t *testing.T) {
	r := NewToolRegistry()
	res, err := r.Call("okf_import_document", map[string]interface{}{"path": mcpFixture("sample.pdf")})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Errorf("expected IsError when no bundle is loaded")
	}
	if !strings.Contains(res.Content[0].Text, "okf_load_bundle") {
		t.Errorf("error should hint at okf_load_bundle, got: %s", res.Content[0].Text)
	}
}

// S22: importing a real document writes into the loaded bundle
func TestMCPImportSuccess(t *testing.T) {
	kb := filepath.Join(t.TempDir(), "kb")
	if err := os.MkdirAll(kb, 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewToolRegistry()
	r.SetBundle(&okf.KnowledgeBundle{}, kb)
	res, err := r.Call("okf_import_document", map[string]interface{}{"path": mcpFixture("sample.docx")})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content[0].Text)
	}
	prod := filepath.Join(kb, "sample.docx.md")
	data, rerr := os.ReadFile(prod)
	if rerr != nil {
		t.Fatalf("product %s missing: %v", prod, rerr)
	}
	if !strings.Contains(string(data), "OKF DOCX Fixture") {
		t.Errorf("converted content missing in %s", prod)
	}
	if !strings.Contains(string(data), "type: source") {
		t.Errorf("frontmatter type not source:\n%s", data)
	}
}

// S24: title/type overrides are honored
func TestMCPImportWithOverrides(t *testing.T) {
	kb := filepath.Join(t.TempDir(), "kb")
	if err := os.MkdirAll(kb, 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewToolRegistry()
	r.SetBundle(&okf.KnowledgeBundle{}, kb)
	res, err := r.Call("okf_import_document", map[string]interface{}{
		"path":  mcpFixture("sample.html"),
		"title": "Custom Title",
		"type":  "reference",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content[0].Text)
	}
	data, rerr := os.ReadFile(filepath.Join(kb, "sample.html.md"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(string(data), `title: "Custom Title"`) {
		t.Errorf("title override not applied:\n%s", data)
	}
	if !strings.Contains(string(data), "type: reference") {
		t.Errorf("type override not applied:\n%s", data)
	}
}

// S25: conversion failure returns an error result
func TestMCPImportError(t *testing.T) {
	kb := filepath.Join(t.TempDir(), "kb")
	if err := os.MkdirAll(kb, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad.exe")
	os.WriteFile(bad, []byte("MZ"), 0o644)
	r := NewToolRegistry()
	r.SetBundle(&okf.KnowledgeBundle{}, kb)
	res, err := r.Call("okf_import_document", map[string]interface{}{"path": bad})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Errorf("expected IsError for unsupported format")
	}
}
