package convert

import (
	"fmt"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/parser"
)

func TestWords_Exported(t *testing.T) {
	if got := Words("hello world"); got != 2 {
		t.Fatalf("Words = %d, want 2", got)
	}
	if got := Words("中文检索测试"); got != 6 {
		t.Fatalf("Words(cjk) = %d, want 6", got)
	}
}

func TestWrapChunkConcept_AllFields(t *testing.T) {
	md := WrapChunkConcept("Deep Dive — part 2", "doc.pdf", "pdf", "sub/doc.pdf", 1, 3, "Intro > Results", "chunk body")
	for _, want := range []string{
		"type: source",
		`title: "Deep Dive — part 2"`,
		"chunk_index: 1",
		"chunk_count: 3",
		`source_path: "sub/doc.pdf"`,
		`heading_path: "Intro > Results"`,
		`derived: "true"`,
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("WrapChunkConcept missing %q in:\n%s", want, md)
		}
	}
}

func TestWrapChunkConcept_NoHeadingPath(t *testing.T) {
	md := WrapChunkConcept("Doc — part 1", "a.pdf", "pdf", "a.pdf", 0, 2, "", "body")
	if strings.Contains(md, "heading_path") {
		t.Fatalf("heading_path present without headings:\n%s", md)
	}
}

func TestWrapChunkConcept_ParserRoundTrip(t *testing.T) {
	md := WrapChunkConcept("Doc — part 3", "doc.pdf", "pdf", "sub/doc.pdf", 2, 3, "H1 > H2", "tail body")
	concept, err := parser.ParseConceptBytes("sub/doc.pdf__c3.md", []byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if concept.Title != "Doc — part 3" {
		t.Fatalf("title = %q, want %q", concept.Title, "Doc — part 3")
	}
	cf := concept.CustomFields
	if fmt.Sprint(cf["chunk_index"]) != "2" {
		t.Fatalf("chunk_index = %v, want 2", cf["chunk_index"])
	}
	if fmt.Sprint(cf["chunk_count"]) != "3" {
		t.Fatalf("chunk_count = %v, want 3", cf["chunk_count"])
	}
	if cf["source_path"] != "sub/doc.pdf" {
		t.Fatalf("source_path = %v, want sub/doc.pdf", cf["source_path"])
	}
	if cf["derived"] != "true" {
		t.Fatalf("derived = %v, want true", cf["derived"])
	}
	if cf["heading_path"] != "H1 > H2" {
		t.Fatalf("heading_path = %v, want H1 > H2", cf["heading_path"])
	}
}

func TestWrapChunkConcept_ReservedNameSafe(t *testing.T) {
	// chunk filenames like doc.pdf__c1.md must not collide with parser
	// reserved names (index.md / log.md).
	for _, fn := range []string{"doc.pdf__c1.md", "doc.pdf__c12.md"} {
		if parser.IsReservedFilename(fn) {
			t.Fatalf("%q treated as reserved filename", fn)
		}
	}
	if !parser.IsReservedFilename("index.md") || !parser.IsReservedFilename("log.md") {
		t.Fatal("reserved filename check itself regressed")
	}
}
