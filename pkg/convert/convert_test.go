package convert

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/parser"
)

func testdata(name string) string {
	return filepath.Join("testdata", name)
}

// --- Task 1.1: fixtures ---

func TestFixturesPresent(t *testing.T) {
	for _, f := range []string{"sample.pdf", "blank.pdf", "sample.docx", "sample.xlsx", "sample.pptx", "sample.html", "sample.csv", "sample.txt"} {
		st, err := os.Stat(testdata(f))
		if err != nil {
			t.Fatalf("fixture %s missing: %v", f, err)
		}
		if st.Size() == 0 {
			t.Fatalf("fixture %s is empty", f)
		}
	}
}

// --- Task 1.2: Options ---

func TestDefaultOptions(t *testing.T) {
	o := DefaultOptions()
	if o.MaxInputBytes != 64<<20 {
		t.Errorf("MaxInputBytes = %d, want %d", o.MaxInputBytes, 64<<20)
	}
	if o.Timeout == 0 {
		t.Errorf("Timeout should be non-zero")
	}
}

// --- Task 1.3: format detection (S7/S8/S9) ---

func TestIsSupportedDocument(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"a.pdf", true}, {"b.docx", true}, {"c.xlsx", true}, {"d.pptx", true},
		{"e.html", true}, {"f.htm", true}, {"g.csv", true}, {"h.txt", true},
		{"i.text", true}, {"j.doc", true},
		// S8: .md is NOT routed through the conversion layer
		{"k.md", false}, {"l.markdown", false},
		{"m.exe", false}, {"n.bin", false},
	}
	for _, c := range cases {
		if got := IsSupportedDocument(c.path); got != c.want {
			t.Errorf("IsSupportedDocument(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIsSupportedDocument_CaseInsensitive(t *testing.T) {
	// S9: uppercase extensions are recognized
	for _, p := range []string{"A.PDF", "B.DOCX", "C.XLSX", "D.PPTX", "E.HTML", "F.CSV"} {
		if !IsSupportedDocument(p) {
			t.Errorf("IsSupportedDocument(%q) = false, want true", p)
		}
	}
}

func TestIsSupportedDocument_NotMarkdown(t *testing.T) {
	// S8: .md returns false (markdown uses the existing pipeline, no conversion)
	for _, p := range []string{"x.md", "x.MD", "y.markdown"} {
		if IsSupportedDocument(p) {
			t.Errorf("IsSupportedDocument(%q) = true, want false", p)
		}
	}
}

func TestFormatRoutingPriority(t *testing.T) {
	// S7: archive > document > markdown > other
	if IsSupportedDocument("x.zip") {
		t.Errorf("zip is archive, not a convertible document")
	}
	if !IsSupportedDocument("x.pdf") {
		t.Errorf("pdf is a convertible document")
	}
	if IsSupportedDocument("x.md") {
		t.Errorf("md must stay on the existing pipeline")
	}
}

func TestDocumentType(t *testing.T) {
	cases := map[string]string{
		"a.pdf": "pdf", "b.docx": "docx", "c.doc": "doc", "d.xlsx": "xlsx",
		"e.pptx": "pptx", "f.html": "html", "g.htm": "html", "h.csv": "csv",
		"i.txt": "txt", "j.text": "txt",
	}
	for p, want := range cases {
		if got := DocumentType(p); got != want {
			t.Errorf("DocumentType(%q) = %q, want %q", p, got, want)
		}
	}
	if DocumentType("x.pdf") == "" {
		t.Errorf("DocumentType should not be empty for supported files")
	}
}

// --- Task 1.4: conversion (S3/S4/S6/S16/S17/S18) ---

func TestConvertPDF(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.pdf"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown pdf: %v", err)
	}
	if !strings.Contains(r.Markdown, "OKF PDF Fixture") {
		t.Errorf("pdf markdown missing body text; got:\n%s", r.Markdown)
	}
}

func TestConvertDOCX(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.docx"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown docx: %v", err)
	}
	if !strings.Contains(r.Markdown, "OKF DOCX Fixture") {
		t.Errorf("docx markdown missing heading; got:\n%s", r.Markdown)
	}
	if !strings.Contains(r.Markdown, "Section One") {
		t.Errorf("docx markdown missing section; got:\n%s", r.Markdown)
	}
}

func TestConvertXLSX(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.xlsx"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown xlsx: %v", err)
	}
	if !strings.Contains(r.Markdown, "apple") || !strings.Contains(r.Markdown, "banana") {
		t.Errorf("xlsx markdown missing sheet data; got:\n%s", r.Markdown)
	}
}

func TestConvertPPTX(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.pptx"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown pptx: %v", err)
	}
	if !strings.Contains(r.Markdown, "OKF PPTX Fixture") {
		t.Errorf("pptx markdown missing title; got:\n%s", r.Markdown)
	}
	if !strings.Contains(r.Markdown, "Second Slide") {
		t.Errorf("pptx markdown missing second slide; got:\n%s", r.Markdown)
	}
}

func TestConvertHTML(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.html"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown html: %v", err)
	}
	for _, want := range []string{"Main Heading", "item one", "item two"} {
		if !strings.Contains(r.Markdown, want) {
			t.Errorf("html markdown missing %q; got:\n%s", want, r.Markdown)
		}
	}
}

func TestConvertCSV(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.csv"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown csv: %v", err)
	}
	if !strings.Contains(r.Markdown, "foo") || !strings.Contains(r.Markdown, "bar") {
		t.Errorf("csv markdown missing rows; got:\n%s", r.Markdown)
	}
}

func TestConvertTXT(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.txt"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown txt: %v", err)
	}
	if !strings.Contains(r.Markdown, "OKF TXT Fixture") {
		t.Errorf("txt markdown missing content; got:\n%s", r.Markdown)
	}
}

func TestConvertExtractsTitle(t *testing.T) {
	// S4: HTML <title> becomes Result.Title
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.html"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown html: %v", err)
	}
	if r.Title == "" {
		t.Errorf("expected non-empty Title from HTML <title>")
	}
}

func TestConvertWarningsSummarized(t *testing.T) {
	// S4: Warnings are string summaries (may be empty for clean conversions)
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.pdf"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown pdf: %v", err)
	}
	for _, w := range r.Warnings {
		if w == "" {
			t.Errorf("warning should not be empty string")
		}
	}
}

func TestConvertUnsupportedFormat(t *testing.T) {
	// S6
	_, err := ConvertToMarkdown(context.Background(), testdata("gen_fixtures.py"), nil)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestConvertInputTooLarge(t *testing.T) {
	// S16
	_, err := ConvertToMarkdown(context.Background(), testdata("sample.pdf"), &Options{MaxInputBytes: 1})
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("err = %v, want ErrInputTooLarge", err)
	}
}

func TestConvertTimeout(t *testing.T) {
	// S17: non-positive Timeout means immediate cancellation
	ctx := context.Background()
	_, err := ConvertToMarkdown(ctx, testdata("sample.pdf"), &Options{Timeout: -1})
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context deadline/canceled", err)
	}
}

func TestConvertNoTextPDF(t *testing.T) {
	// S18: scanned/blank PDF with no text layer → ErrNoText
	_, err := ConvertToMarkdown(context.Background(), testdata("blank.pdf"), nil)
	if !errors.Is(err, ErrNoText) {
		t.Fatalf("err = %v, want ErrNoText (scanned pdf)", err)
	}
}

// --- Task 1.5: output parsable by okf parser (S5) ---

func TestConvertedOutputParsableByOKF(t *testing.T) {
	r, err := ConvertToMarkdown(context.Background(), testdata("sample.html"), nil)
	if err != nil {
		t.Fatalf("ConvertToMarkdown html: %v", err)
	}
	doc := "---\ntype: source\ntitle: " + strconvQuote(r.Title) + "\n---\n" + r.Markdown
	c, err := parser.ParseConceptBytes("sample.html.md", []byte(doc))
	if err != nil {
		t.Fatalf("ParseConceptBytes: %v", err)
	}
	if strings.TrimSpace(c.Content) == "" {
		t.Errorf("parsed Concept.Content is empty; conversion body lost")
	}
	if c.Type != "source" {
		t.Errorf("parsed Concept.Type = %q, want source", c.Type)
	}
}

func strconvQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
