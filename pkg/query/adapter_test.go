package query

import (
	"reflect"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// S13: the canonical conversion preserves every custom field, including the
// stable okf_id, source_path, and arbitrary user fields.
func TestConceptAdapterPreservesCustomFields(t *testing.T) {
	src := &okf.Concept{
		Type:        "code_file",
		Title:       "demo",
		Description: "desc",
		Resource:    "code://repo/a.go",
		Tags:        []string{"go", "generated"},
		Content:     "package main",
		FilePath:    "code_files/a.go.md",
		CustomFields: map[string]any{
			"okf_id":      "okf_17a2c56db85c4889b4f8fe02ca9ac67e",
			"source_path": "a.go",
			"language":    "go",
			"arbitrary":   "kept",
			"count":       float64(3),
		},
	}

	dst := ConceptFromOKF(src)

	if dst.Type != src.Type || dst.Title != src.Title || dst.Description != src.Description ||
		dst.Resource != src.Resource || dst.Content != src.Content || dst.FilePath != src.FilePath {
		t.Fatalf("scalar fields not preserved: %+v", dst)
	}
	if !reflect.DeepEqual(dst.Tags, src.Tags) {
		t.Fatalf("tags not preserved: %v", dst.Tags)
	}
	for _, key := range []string{"okf_id", "source_path", "language", "arbitrary"} {
		if dst.CustomFields[key] != src.CustomFields[key] {
			t.Fatalf("custom field %q not preserved: %v", key, dst.CustomFields[key])
		}
	}
	if dst.CustomFields["count"] != float64(3) {
		t.Fatalf("non-string custom field not preserved: %v", dst.CustomFields["count"])
	}
}

// S13: mutating the destination CustomFields map or its nested structures must
// not alias or mutate the source concept.
func TestConceptAdapterDoesNotAliasMaps(t *testing.T) {
	nested := map[string]any{"inner": "keep"}
	src := &okf.Concept{
		Type: "source",
		Tags: []string{"a"},
		CustomFields: map[string]any{
			"okf_id": "okf_17a2c56db85c4889b4f8fe02ca9ac67e",
			"nested": nested,
		},
	}

	dst := ConceptFromOKF(src)

	// Mutate destination top-level map.
	dst.CustomFields["okf_id"] = "okf_00000000000000000000000000000000"
	dst.CustomFields["added"] = "new"
	// Mutate destination nested map.
	dst.CustomFields["nested"].(map[string]any)["inner"] = "changed"
	// Mutate destination tags slice.
	dst.Tags[0] = "mutated"

	if src.CustomFields["okf_id"] != "okf_17a2c56db85c4889b4f8fe02ca9ac67e" {
		t.Fatalf("source okf_id mutated via destination")
	}
	if _, leaked := src.CustomFields["added"]; leaked {
		t.Fatalf("destination key leaked into source")
	}
	if src.CustomFields["nested"].(map[string]any)["inner"] != "keep" {
		t.Fatalf("source nested map mutated via destination")
	}
	if src.Tags[0] != "a" {
		t.Fatalf("source tags mutated via destination")
	}
}

func TestConceptAdapterNilSafe(t *testing.T) {
	if ConceptFromOKF(nil) != nil {
		t.Fatalf("nil concept must map to nil")
	}
	if BundleFromOKF(nil) != nil {
		t.Fatalf("nil bundle must map to nil")
	}
	empty := BundleFromOKF(&okf.KnowledgeBundle{})
	if empty == nil || len(empty.Concepts) != 0 {
		t.Fatalf("empty bundle must map to empty non-nil slice")
	}
}
