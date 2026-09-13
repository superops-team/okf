package convert

import (
	"strings"
	"testing"
)

// S12: derived chunks persist the parent concept's stable okf_id under
// parent_okf_id, so grouped concept projection can key them to the parent.
func TestDerivedChunkHasParentID(t *testing.T) {
	const parent = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	md := WrapChunkConcept("Deep Dive", "doc.pdf", "pdf", "doc.pdf", 1, 3, "Intro", "body", parent)
	if !strings.Contains(md, "parent_okf_id:") || !strings.Contains(md, parent) {
		t.Fatalf("chunk concept missing parent_okf_id:\n%s", md)
	}
	// The parent id itself must not be stamped on the chunk.
	if strings.Contains(md, "okf_id: "+parent) {
		t.Fatalf("chunk must not carry its own okf_id:\n%s", md)
	}
}

// S12: temporary conversion / staging artifacts receive NO random okf_id. The
// staging wrap helpers are called with an empty id so frontmatter carries none.
func TestConversionStagingHasNoRandomID(t *testing.T) {
	parentBody := WrapConcept("Doc", "doc.pdf", "pdf", "source", "# body", "")
	if strings.Contains(parentBody, "okf_id:") {
		t.Fatalf("staging parent body must not carry okf_id:\n%s", parentBody)
	}
	chunkBody := WrapChunkConcept("Part 1", "doc.pdf", "pdf", "doc.pdf", 0, 1, "", "body", "")
	if strings.Contains(chunkBody, "okf_id:") || strings.Contains(chunkBody, "parent_okf_id:") {
		t.Fatalf("staging chunk must not carry identity fields:\n%s", chunkBody)
	}
}
