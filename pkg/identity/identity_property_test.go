package identity

import (
	"regexp"
	"testing"
	"testing/quick"

	"github.com/superops-team/okf/pkg/okf"
)

// TestStableIDProperties uses testing/quick to check invariants that hold for
// arbitrary generated ids and arbitrary custom-field round trips:
//   - every New() id matches the canonical grammar;
//   - parsing a generated id returns the same id (round trip);
//   - FromConcept on a concept carrying that id classifies it stable with a
//     canonical URI;
//   - two independently generated ids are never equal.
func TestStableIDProperties(t *testing.T) {
	grammar := regexp.MustCompile(`^okf_[0-9a-f]{32}$`)

	roundTrip := func(_ int) bool {
		id, err := New()
		if err != nil {
			t.Errorf("New() error: %v", err)
			return false
		}
		if !grammar.MatchString(id) {
			t.Errorf("id %q fails grammar", id)
			return false
		}
		parsed, err := Parse(id)
		if err != nil || parsed != id {
			t.Errorf("round trip failed: parsed=%q err=%v", parsed, err)
			return false
		}
		ref := FromConcept(&okf.Concept{
			Type:         "source",
			FilePath:     "p.md",
			CustomFields: map[string]any{Field: id},
		})
		if ref.State != StateStable || ref.ID != id || ref.URI != "okf://concept/"+id {
			t.Errorf("FromConcept mismatch: %+v", ref)
			return false
		}
		return true
	}
	if err := quick.Check(roundTrip, nil); err != nil {
		t.Fatal(err)
	}

	uniqueness := func(_ int) bool {
		a, err := New()
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		b, err := New()
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		return a != b
	}
	if err := quick.Check(uniqueness, nil); err != nil {
		t.Fatal(err)
	}
}
