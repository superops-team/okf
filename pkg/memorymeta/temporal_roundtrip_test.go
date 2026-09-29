package memorymeta

// T0.2 / S07: temporal CustomFields round-trip losslessly, and the core
// okf.Concept / parser.Concept structs carry no named temporal fields.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

const s07Frontmatter = `---
type: note
title: Temporal Round Trip
memory_state: approved
memory_confidence: 0.8
memory_relation:
  kind: updates
  targets:
    - okf://concept/okf_0123456789abcdef0123456789abcdef
memory_review:
  reviewed_at: "2026-09-19T12:00:00Z"
  previous_state: proposed
  current_state: approved
  action: approve
arbitrary_user_field: keep-me
---
Body content.
`

// TestS07TemporalRoundTrip parses frontmatter through the real parser, reads
// the typed accessors off a shared okf.Concept view, re-serializes, re-parses,
// and asserts byte/value stability plus preservation of the arbitrary field.
func TestS07TemporalRoundTrip(t *testing.T) {
	pc, err := parser.ParseConceptBytes("concepts/rt.md", []byte(s07Frontmatter))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Shared CustomFields map between parser.Concept and okf.Concept view.
	oc := &okf.Concept{
		Type:         pc.Type,
		Title:        pc.Title,
		Content:      pc.Content,
		CustomFields: pc.CustomFields,
	}

	// (1) Accessors read the parsed frontmatter.
	if st, _ := State(oc); st != MemoryApproved {
		t.Fatalf("State = %q, want approved", st)
	}
	if v, present, _ := Confidence(oc); !present || v != 0.8 {
		t.Fatalf("Confidence = (%v,%v), want (0.8,true)", v, present)
	}
	rel, warn := Relation(oc)
	if warn != "" {
		t.Fatalf("Relation warning = %q", warn)
	}
	if rel.Kind != RelationUpdates || len(rel.Targets) != 1 ||
		rel.Targets[0] != "okf_0123456789abcdef0123456789abcdef" {
		t.Fatalf("Relation = %+v, want updates->bare id", rel)
	}
	if rec, _ := Review(oc); rec.Action != "approve" || rec.CurrentState != MemoryApproved {
		t.Fatalf("Review = %+v, unexpected", rec)
	}
	if got, _ := oc.CustomFields["arbitrary_user_field"].(string); got != "keep-me" {
		t.Fatalf("arbitrary field = %v, want keep-me", oc.CustomFields["arbitrary_user_field"])
	}

	// (2) Re-serialize and re-parse.
	out, err := parser.SerializeConcept(pc, false)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	pc2, err := parser.ParseConceptBytes("concepts/rt.md", out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	oc2 := &okf.Concept{
		Type:         pc2.Type,
		Title:        pc2.Title,
		Content:      pc2.Content,
		CustomFields: pc2.CustomFields,
	}

	// (3) Values stable across the round trip.
	if st, _ := State(oc2); st != MemoryApproved {
		t.Fatalf("post-roundtrip State = %q", st)
	}
	rel2, _ := Relation(oc2)
	if rel2.Kind != rel.Kind || len(rel2.Targets) != 1 || rel2.Targets[0] != rel.Targets[0] {
		t.Fatalf("post-roundtrip Relation = %+v, want %+v", rel2, rel)
	}
	if got, _ := oc2.CustomFields["arbitrary_user_field"].(string); got != "keep-me" {
		t.Fatalf("arbitrary field not preserved: %v", oc2.CustomFields["arbitrary_user_field"])
	}

	// (4) Re-serialized output carries every temporal key.
	res := string(out)
	for _, want := range []string{
		"memory_state: approved",
		"memory_confidence: 0.8",
		"kind: updates",
		"action: approve",
		"arbitrary_user_field: keep-me",
	} {
		if !strings.Contains(res, want) {
			t.Fatalf("re-serialized output missing %q:\n%s", want, res)
		}
	}
}

// TestS07NoNamedTemporalStructFields locks the S07 architecture: neither
// okf.Concept nor parser.Concept exposes memory state / code refs / governance
// as named struct fields. All such data lives in CustomFields.
func TestS07NoNamedTemporalStructFields(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[okf.Concept](),
		reflect.TypeFor[parser.Concept](),
	} {
		for _, forbidden := range []string{"MemoryState", "CodeRefs", "Governance"} {
			if f, found := typ.FieldByName(forbidden); found {
				t.Fatalf("%s must not declare %q struct field (yaml tag %q); "+
					"extension data lives in CustomFields", typ, forbidden, f.Tag)
			}
		}
	}
}
