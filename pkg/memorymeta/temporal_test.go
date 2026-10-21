package memorymeta

// T0.1 / S01–S06: typed temporal CustomFields accessors. These tests pin the
// exact API surface and the conservative read + strict-fail contract.

import (
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// Valid stable IDs used across the accessor tests. Each is okf_ + 32 hex chars.
const (
	idA = "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	idC = "okf_cccccccccccccccccccccccccccccccc"
	idD = "okf_dddddddddddddddddddddddddddddddd"
	idP = "okf_e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1e1"
)

// mkTemporalConcept builds an okf.Concept with the given durable type and an optional
// set of CustomFields.
func mkTemporalConcept(typ string, fields map[string]any) *okf.Concept {
	cf := map[string]any{}
	for k, v := range fields {
		cf[k] = v
	}
	return &okf.Concept{Type: typ, CustomFields: cf}
}

// provenanceEvidence builds a provenance CustomFields entry carrying evidence_refs.
func provenanceEvidence(refs []string) map[string]any {
	anyRefs := make([]any, len(refs))
	for i, r := range refs {
		anyRefs[i] = r
	}
	return map[string]any{"evidence_refs": anyRefs}
}

// validID returns a deterministic, unique canonical okf id (okf_ + 32 hex
// chars). A splitmix-style PRNG guarantees distinct seeds yield distinct IDs.
func validID(n int) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.WriteString("okf_")
	state := uint64(n+1) * 0x9e3779b97f4a7c15
	for i := 0; i < 32; i++ {
		state = state*6364136223867930055 + 1442695040888963407
		b.WriteByte(hex[(state>>uint(i%8*4))&0xf])
	}
	return b.String()
}

// ids builds n distinct valid ids starting at seed.
func ids(n, seed int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = validID(seed + i)
	}
	return out
}

// S01: missing memory_state defaults to approved with no warning.
func TestS01MissingStateDefaultsApproved(t *testing.T) {
	c := mkTemporalConcept("note", nil)
	got, warn := State(c)
	if got != MemoryApproved {
		t.Fatalf("State() = %q, want approved (missing)", got)
	}
	if warn != "" {
		t.Fatalf("State() warning = %q, want empty for missing state", warn)
	}
}

// S02: explicit states normalize case; unknown/wrong-type fall back to proposed
// with a warning (conservative).
func TestS02StateNormalization(t *testing.T) {
	cases := []struct {
		name  string
		raw   any
		want  MemoryState
		warns bool
	}{
		{"Approved", "Approved", MemoryApproved, false},
		{"proposed", "proposed", MemoryProposed, false},
		{"DECLINED", "DECLINED", MemoryDeclined, false},
		{"mixed", "ApPrOvEd", MemoryApproved, false},
		{"unknown", "archived", MemoryProposed, true},
		{"wrong-type", 42, MemoryProposed, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := mkTemporalConcept("note", map[string]any{"memory_state": tc.raw})
			got, warn := State(c)
			if got != tc.want {
				t.Fatalf("State() = %q, want %q", got, tc.want)
			}
			if tc.warns && warn == "" {
				t.Fatalf("State() warning empty, want a conservative warning")
			}
			if !tc.warns && warn != "" {
				t.Fatalf("State() warning = %q, want empty", warn)
			}
		})
	}
}

// S02 round-trip through SetState persists the canonical lowercase value.
func TestSetStateWritesCanonical(t *testing.T) {
	c := mkTemporalConcept("note", nil)
	SetState(c, MemoryProposed)
	if got := c.CustomFields["memory_state"]; got != "proposed" {
		t.Fatalf("memory_state = %v, want \"proposed\"", got)
	}
	got, _ := State(c)
	if got != MemoryProposed {
		t.Fatalf("State() after SetState = %q, want proposed", got)
	}
}

// S03: proposed requires finite confidence in [0,1] AND >=1 evidence_ref;
// approved/declined carrying confidence fails strict.
func TestS03ProposedConfidenceAndEvidence(t *testing.T) {
	goodProposed := mkTemporalConcept("note", map[string]any{
		"memory_state":      "proposed",
		"memory_confidence": 0.72,
		"provenance":        provenanceEvidence([]string{idA}),
	})
	if errs := ValidateTemporal(goodProposed, true); len(errs) != 0 {
		t.Fatalf("valid proposed should pass strict, got %v", errs)
	}

	// proposed without confidence -> strict failure.
	noConf := mkTemporalConcept("note", map[string]any{
		"memory_state": "proposed",
		"provenance":   provenanceEvidence([]string{idA}),
	})
	if errs := ValidateTemporal(noConf, true); len(errs) == 0 {
		t.Fatal("proposed without confidence must fail strict")
	}

	// proposed with out-of-range confidence -> strict failure.
	badConf := mkTemporalConcept("note", map[string]any{
		"memory_state":      "proposed",
		"memory_confidence": 1.5,
		"provenance":        provenanceEvidence([]string{idA}),
	})
	if errs := ValidateTemporal(badConf, true); len(errs) == 0 {
		t.Fatal("proposed with confidence 1.5 must fail strict")
	}

	// proposed with no evidence_ref -> strict failure.
	noEvidence := mkTemporalConcept("note", map[string]any{
		"memory_state":      "proposed",
		"memory_confidence": 0.5,
	})
	if errs := ValidateTemporal(noEvidence, true); len(errs) == 0 {
		t.Fatal("proposed without evidence_ref must fail strict")
	}

	// approved carrying confidence fails strict.
	approvedWithConf := mkTemporalConcept("note", map[string]any{
		"memory_state":      "approved",
		"memory_confidence": 0.5,
	})
	errs := ValidateTemporal(approvedWithConf, true)
	if len(errs) == 0 {
		t.Fatal("approved carrying memory_confidence must fail strict")
	}

	// Confidence accessor: absent -> 0,false.
	v, present, warn := Confidence(mkTemporalConcept("note", nil))
	if present || v != 0 || warn != "" {
		t.Fatalf("Confidence absent = (%v,%v,%q), want (0,false,empty)", v, present, warn)
	}
	// Confidence accessor: present number -> value,true.
	v, present, _ = Confidence(goodProposed)
	if !present || v != 0.72 {
		t.Fatalf("Confidence present = (%v,%v), want (0.72,true)", v, present)
	}
}

// S04: relation is namespaced under memory_relation; targets normalized to bare
// okf id; code relation_kind/source/target are neither read nor written.
func TestS04RelationNamespacingAndNormalization(t *testing.T) {
	c := mkTemporalConcept("note", map[string]any{
		"memory_relation": map[string]any{
			"kind":    "updates",
			"targets": []any{"okf://concept/" + idA, idB},
		},
	})
	rel, warn := Relation(c)
	if warn != "" {
		t.Fatalf("Relation() warning = %q, want empty", warn)
	}
	if rel.Kind != RelationUpdates {
		t.Fatalf("Kind = %q, want updates", rel.Kind)
	}
	if len(rel.Targets) != 2 || rel.Targets[0] != idA || rel.Targets[1] != idB {
		t.Fatalf("Targets = %v, want [%s %s] (URI stripped)", rel.Targets, idA, idB)
	}

	// Unknown kind / non-list targets / non-string entries warn.
	rel, warn = Relation(mkTemporalConcept("note", map[string]any{
		"memory_relation": map[string]any{"kind": "contradicts", "targets": []any{idA}},
	}))
	if warn == "" {
		t.Fatal("unknown relation kind must warn")
	}
	rel, warn = Relation(mkTemporalConcept("note", map[string]any{
		"memory_relation": map[string]any{"kind": "updates", "targets": idA},
	}))
	if warn == "" {
		t.Fatal("non-list targets must warn")
	}
	rel, warn = Relation(mkTemporalConcept("note", map[string]any{
		"memory_relation": map[string]any{"kind": "updates", "targets": []any{42}},
	}))
	if warn == "" {
		t.Fatal("non-string target entry must warn")
	}

	// SetRelation writes a nested map under the memory_ namespace only.
	out := mkTemporalConcept("note", nil)
	SetRelation(out, MemoryRelation{Kind: RelationExtends, Targets: []string{idA, idB}})
	if _, ok := out.CustomFields["relation_kind"]; ok {
		t.Fatal("SetRelation must NOT write code relation_kind field")
	}
	nested, ok := out.CustomFields["memory_relation"].(map[string]any)
	if !ok {
		t.Fatal("SetRelation must write nested memory_relation map")
	}
	if nested["kind"] != "extends" {
		t.Fatalf("nested kind = %v, want extends", nested["kind"])
	}
}

// S05: relation bounds and stable-id validation reject malformed relations with
// invalid_memory_relation in strict mode.
func TestS05RelationBoundsAndIDs(t *testing.T) {
	mkRel := func(kind string, targets ...any) *okf.Concept {
		return mkTemporalConcept("note", map[string]any{
			"memory_state": "approved",
			"memory_relation": map[string]any{
				"kind":    kind,
				"targets": targets,
			},
		})
	}
	anyIDs := func(n, seed int) []any {
		v := ids(n, seed)
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out
	}
	hasCode := func(errs []string) bool {
		for _, e := range errs {
			if strings.Contains(e, "invalid_memory_relation") {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name    string
		concept *okf.Concept
	}{
		{"updates-zero-targets", mkRel("updates")},
		{"updates-two-targets", mkRel("updates", idA, idB)},
		{"extends-zero-targets", mkRel("extends")},
		{"extends-nine-targets", mkRel("extends", anyIDs(9, 10)...)},
		{"extends-duplicate-targets", mkRel("extends", idA, idA)},
		{"self-edge", func() *okf.Concept {
			c := mkTemporalConcept("note", map[string]any{"memory_state": "approved"})
			c.CustomFields["okf_id"] = idA
			return mkRelWithID(c, "updates", []any{idA})
		}()},
		{"path-target", mkRel("updates", "notes/old.md")},
		{"title-target", mkRel("updates", "Some Title")},
		{"invalid-id", mkRel("updates", "okf_nothex")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateTemporal(tc.concept, true)
			if len(errs) == 0 {
				t.Fatalf("%s: expected strict failure, got none", tc.name)
			}
			if !hasCode(errs) {
				t.Fatalf("%s: errors %v must contain invalid_memory_relation", tc.name, errs)
			}
		})
	}

	// A valid extends with 8 unique IDs passes.
	okExtends := mkRel("extends", anyIDs(8, 10)...)
	if errs := ValidateTemporal(okExtends, true); len(errs) != 0 {
		t.Fatalf("valid 8-target extends must pass, got %v", errs)
	}

	// Unknown nested field in memory_relation fails strict.
	unknownField := mkTemporalConcept("note", map[string]any{
		"memory_state": "approved",
		"memory_relation": map[string]any{
			"kind":    "updates",
			"targets": []any{idA},
			"bogus":   true,
		},
	})
	if errs := ValidateTemporal(unknownField, true); len(errs) == 0 {
		t.Fatal("unknown nested memory_relation field must fail strict")
	}
}

// mkRelWithID attaches an okf_id and a relation to an existing concept.
func mkRelWithID(c *okf.Concept, kind string, targets []any) *okf.Concept {
	c.CustomFields["memory_relation"] = map[string]any{
		"kind":    kind,
		"targets": targets,
	}
	return c
}

// S06: malformed metadata is conservative in non-strict reads and fails strict.
func TestS06MalformedConservativeAndStrict(t *testing.T) {
	// Unknown state: non-strict read falls back to proposed + warning.
	bad := mkTemporalConcept("note", map[string]any{"memory_state": "weird"})
	got, warn := State(bad)
	if got != MemoryProposed || warn == "" {
		t.Fatalf("non-strict malformed state = (%q,%q), want (proposed, warning)", got, warn)
	}
	// Strict validation fails.
	if errs := ValidateTemporal(bad, true); len(errs) == 0 {
		t.Fatal("malformed state must fail strict")
	}

	// Non-strict ValidateTemporal returns no hard errors (conservative; warnings
	// surface through the accessors instead), mirroring package Validate().
	if errs := ValidateTemporal(bad, false); len(errs) != 0 {
		t.Fatalf("non-strict ValidateTemporal must be conservative, got %v", errs)
	}

	// Malformed confidence: accessor warns.
	badConf := mkTemporalConcept("note", map[string]any{"memory_confidence": "not-a-number"})
	if _, present, warn := Confidence(badConf); !present || warn == "" {
		t.Fatalf("malformed confidence accessor = (present=%v,warn=%q), want warning", present, warn)
	}
}

// Review accessor reads the bounded latest record and warns on malformed input.
func TestReviewAccessor(t *testing.T) {
	c := mkTemporalConcept("note", map[string]any{
		"memory_review": map[string]any{
			"reviewed_at":         "2026-09-19T12:00:00Z",
			"previous_state":      "proposed",
			"current_state":       "approved",
			"action":              "approve",
			"previous_confidence": 0.4,
		},
	})
	rec, warn := Review(c)
	if warn != "" {
		t.Fatalf("Review() warning = %q, want empty", warn)
	}
	if rec.ReviewedAt != "2026-09-19T12:00:00Z" ||
		rec.PreviousState != MemoryProposed ||
		rec.CurrentState != MemoryApproved ||
		rec.Action != "approve" ||
		rec.PreviousConfidence == nil || *rec.PreviousConfidence != 0.4 {
		t.Fatalf("Review() = %+v, unexpected", rec)
	}

	// Malformed review object warns.
	if _, warn := Review(mkTemporalConcept("note", map[string]any{"memory_review": "oops"})); warn == "" {
		t.Fatal("malformed memory_review must warn")
	}
}
