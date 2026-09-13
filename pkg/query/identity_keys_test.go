package query

import (
	"testing"

	"github.com/superops-team/okf/pkg/identity"
)

// S14: stable and legacy index keys are deterministic. A concept carrying a
// valid okf_id is keyed v3:id:<id>; a legacy concept falls back to
// v3:legacy:<fingerprint>; chunk keys append the ordinal without losing the
// parent key.
func TestIdentityAwareIndexKeys(t *testing.T) {
	stable := &Concept{
		Type:     "note",
		Title:    "Stable Note",
		FilePath: "notes/stable.md",
		CustomFields: map[string]any{
			identity.Field: "okf_17a2c56db85c4889b4f8fe02ca9ac67e",
		},
	}
	legacy := &Concept{
		Type:     "doc",
		Title:    "Legacy Doc",
		FilePath: "docs/legacy.md",
	}

	stableKey := ConceptKey(stable)
	if stableKey != "v3:id:okf_17a2c56db85c4889b4f8fe02ca9ac67e" {
		t.Fatalf("stable concept key = %q, want v3:id:<okf_id>", stableKey)
	}

	legacyKey := ConceptKey(legacy)
	wantLegacy := "v3:legacy:" + Fingerprint(legacy)
	if legacyKey != wantLegacy {
		t.Fatalf("legacy concept key = %q, want %q", legacyKey, wantLegacy)
	}

	// Chunk keys append the ordinal without losing the parent key.
	if got := ChunkKey(stable, 0); got != stableKey+"#0" {
		t.Fatalf("chunk key = %q, want %q", got, stableKey+"#0")
	}
	if got := ChunkKey(stable, 3); got != stableKey+"#3" {
		t.Fatalf("chunk key = %q, want %q", got, stableKey+"#3")
	}
	if got := ChunkKey(legacy, 0); got != legacyKey+"#0" {
		t.Fatalf("legacy chunk key = %q, want %q", got, legacyKey+"#0")
	}

	// Deterministic: a fresh call must reproduce the key already computed above.
	if ConceptKey(stable) != stableKey {
		t.Fatal("ConceptKey not deterministic for stable concept")
	}
	if got := ChunkKey(legacy, 2); got != legacyKey+"#2" {
		t.Fatalf("ChunkKey = %q, want %q", got, legacyKey+"#2")
	}

	// ChunkKeyConcept round-trips: stripping the ordinal recovers the parent key.
	if got := ChunkKeyConcept(ChunkKey(stable, 5)); got != stableKey {
		t.Fatalf("ChunkKeyConcept = %q, want %q", got, stableKey)
	}
	if got := ChunkKeyConcept(ChunkKey(legacy, 1)); got != legacyKey {
		t.Fatalf("ChunkKeyConcept = %q, want %q", got, legacyKey)
	}

	// An invalid okf_id falls back to the legacy fingerprint rather than being
	// trusted as a stable id.
	bogus := &Concept{
		Type:     "note",
		Title:    "Bogus",
		FilePath: "notes/bogus.md",
		CustomFields: map[string]any{
			identity.Field: "NOT_A_VALID_ID",
		},
	}
	if got := ConceptKey(bogus); got != "v3:legacy:"+Fingerprint(bogus) {
		t.Fatalf("invalid okf_id should fall back to legacy key, got %q", got)
	}
}
