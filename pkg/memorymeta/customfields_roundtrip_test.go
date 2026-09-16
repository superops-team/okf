package memorymeta

// S37: governance and code_refs are extension fields stored inside
// Concept.CustomFields, NOT named struct fields. This round-trip test proves
// that (1) the parser keeps unknown frontmatter keys (governance, code_refs,
// my_custom) in CustomFields, (2) SetGovernance/SetCodeRefs mutate the same
// inline map and survive a re-serialization to YAML, and (3) the Concept struct
// itself carries no dedicated governance/code_refs field (enforced via
// reflection below).

import (
	"reflect"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

const s37RoundTripMarkdown = `---
type: decision
title: Governed Concept
governance: hold
code_refs:
  - pkg/original.go
my_custom: hello
---
Body content for the custom-fields round-trip.
`

// TestS37CustomFieldsParseRoundTrip parses a Markdown concept carrying
// governance/code_refs/my_custom, mutates governance and code_refs through the
// typed accessors, and re-serializes to confirm the extension fields survive.
func TestS37CustomFieldsParseRoundTrip(t *testing.T) {
	pc, err := parser.ParseConceptBytes("concepts/governed.md", []byte(s37RoundTripMarkdown))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// (1) The parser parked all three extension keys in CustomFields.
	if got, _ := pc.CustomFields["governance"].(string); got != "hold" {
		t.Fatalf("CustomFields[governance] = %v, want hold", pc.CustomFields["governance"])
	}
	rawRefs, ok := pc.CustomFields["code_refs"]
	if !ok {
		t.Fatal("CustomFields missing code_refs")
	}
	refs := toStringSlice(t, rawRefs)
	if len(refs) != 1 || refs[0] != "pkg/original.go" {
		t.Fatalf("CustomFields[code_refs] = %v, want [pkg/original.go]", rawRefs)
	}
	if got, _ := pc.CustomFields["my_custom"].(string); got != "hello" {
		t.Fatalf("CustomFields[my_custom] = %v, want hello", pc.CustomFields["my_custom"])
	}

	// (2) Build an okf.Concept that SHARES the CustomFields map, then mutate it
	// through the typed accessors. Sharing the map means the parser.Concept
	// below observes the mutation on re-serialization.
	oc := &okf.Concept{
		Type:         pc.Type,
		Title:        pc.Title,
		Content:      pc.Content,
		CustomFields: pc.CustomFields,
	}
	SetGovernance(oc, GovernanceConstraint)
	SetCodeRefs(oc, []string{"pkg/changed/*.go"})

	// The accessors wrote back into the shared map.
	if got, _ := oc.CustomFields["governance"].(string); got != "constraint" {
		t.Fatalf("SetGovernance did not persist: %v", oc.CustomFields["governance"])
	}

	// (3) Re-serialize the parser concept; extension fields must be retained.
	out, err := parser.SerializeConcept(pc, false)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	serialized := string(out)
	for _, want := range []string{
		"governance: constraint",
		"my_custom: hello",
		"pkg/changed/*.go",
	} {
		if !strings.Contains(serialized, want) {
			t.Fatalf("serialized YAML missing %q:\n%s", want, serialized)
		}
	}
	// The original code_refs value must have been replaced, not duplicated.
	if strings.Contains(serialized, "pkg/original.go") {
		t.Fatalf("stale original code_refs survived re-serialization:\n%s", serialized)
	}
}

// TestS37ConceptHasNoGovernedStructFields locks the S37 architecture: neither
// the bundle okf.Concept nor the parser parser.Concept exposes governance /
// code_refs as named struct fields. All such data lives in CustomFields.
func TestS37ConceptHasNoGovernedStructFields(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[okf.Concept](),
		reflect.TypeFor[parser.Concept](),
	} {
		for _, forbidden := range []string{"Governance", "CodeRefs"} {
			if f, found := typ.FieldByName(forbidden); found {
				t.Fatalf("%s must not declare a %q struct field (yaml tag %q); "+
					"governed extension data lives in CustomFields", typ, forbidden, f.Tag)
			}
		}
	}
}

func toStringSlice(t *testing.T, v any) []string {
	t.Helper()
	switch typed := v.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				t.Fatalf("code_refs element is %T, not string", item)
			}
			out = append(out, s)
		}
		return out
	case []string:
		return typed
	default:
		t.Fatalf("code_refs is %T, not a list", v)
		return nil
	}
}
