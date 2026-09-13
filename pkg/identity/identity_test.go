package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

func conceptWithID(id, path string) *okf.Concept {
	c := &okf.Concept{Type: "source", FilePath: path}
	if id != "" {
		c.CustomFields = map[string]any{Field: id}
	}
	return c
}

// S01: a legacy concept (no okf_id) remains valid and is legacy-unstable.
func TestLegacyIdentityState(t *testing.T) {
	ref := FromConcept(&okf.Concept{Type: "source", FilePath: "notes/a.md"})
	if ref.State != StateLegacyUnstable {
		t.Fatalf("expected legacy-unstable, got %q", ref.State)
	}
	if ref.ID != "" || ref.URI != "" {
		t.Fatalf("legacy concept must carry no id/uri, got %q %q", ref.ID, ref.URI)
	}
	if ref.FilePath != "notes/a.md" {
		t.Fatalf("file path not preserved: %q", ref.FilePath)
	}
}

// S02: a valid stable id round-trips through parse and exposes its URI.
func TestStableIDRoundTrip(t *testing.T) {
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	got, err := Parse(id)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", id, err)
	}
	if got != id {
		t.Fatalf("round trip changed id: %q", got)
	}
	c := conceptWithID(id, "docs/x.md")
	ref := FromConcept(c)
	if ref.State != StateStable {
		t.Fatalf("expected stable, got %q", ref.State)
	}
	if ref.URI != "okf://concept/"+id {
		t.Fatalf("unexpected URI %q", ref.URI)
	}
	resolved, err := Resolve([]*okf.Concept{c}, "okf://concept/"+id)
	if err != nil {
		t.Fatalf("Resolve(uri) error: %v", err)
	}
	if resolved != c {
		t.Fatalf("Resolve did not return the original concept")
	}
	// Bare ID also resolves.
	if _, err := Resolve([]*okf.Concept{c}, id); err != nil {
		t.Fatalf("Resolve(bare) error: %v", err)
	}
}

// S03: uppercase / truncated / overlong / non-hex ids fail identity use but
// generic parsing is untouched (we only assert our validator).
func TestInvalidStableID(t *testing.T) {
	cases := map[string]string{
		"uppercase": "okf_17A2C56DB85C4889B4F8FE02CA9AC67E",
		"truncated": "okf_17a2c56db85c4889b4f8fe02ca9ac67",
		"overlong":  "okf_17a2c56db85c4889b4f8fe02ca9ac67e00",
		"non-hex":   "okf_17a2c56db85c4889b4f8fe02ca9ac67g",
		"no-prefix": "17a2c56db85c4889b4f8fe02ca9ac67e",
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(v); !errors.Is(err, ErrInvalidConceptID) {
				t.Fatalf("Parse(%q): expected invalid_concept_id, got %v", v, err)
			}
			// FromConcept classifies it invalid.
			ref := FromConcept(conceptWithID(v, "x.md"))
			if ref.State != StateInvalid {
				t.Fatalf("expected invalid state, got %q", ref.State)
			}
			// BuildRegistry fails closed on an explicit invalid id.
			if _, err := BuildRegistry([]*okf.Concept{conceptWithID(v, "x.md")}); !errors.Is(err, ErrInvalidConceptID) {
				t.Fatalf("BuildRegistry: expected invalid_concept_id, got %v", err)
			}
		})
	}
}

// S04: duplicate stable ids fail closed and identify both bundle-relative paths.
func TestDuplicateStableID(t *testing.T) {
	const id = "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	a := conceptWithID(id, "alpha/a.md")
	b := conceptWithID(id, "beta/b.md")
	_, err := BuildRegistry([]*okf.Concept{a, b})
	if !errors.Is(err, ErrDuplicateConceptID) {
		t.Fatalf("expected duplicate_concept_id, got %v", err)
	}
	var identErr *Error
	if !errors.As(err, &identErr) {
		t.Fatalf("expected *identity.Error, got %T", err)
	}
	if len(identErr.Paths) != 2 {
		t.Fatalf("expected two paths in duplicate error, got %v", identErr.Paths)
	}
	// And resolution must fail closed, not pick arbitrarily.
	if _, err := Resolve([]*okf.Concept{a, b}, id); !errors.Is(err, ErrDuplicateConceptID) {
		t.Fatalf("Resolve over duplicates must fail closed, got %v", err)
	}
}

// S05: generated ids have canonical grammar and are unique over a large sample.
func TestNewStableID(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for range n {
		id, err := New()
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		if !idPattern.MatchString(id) {
			t.Fatalf("generated id %q fails grammar", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate generated id %q", id)
		}
		seen[id] = struct{}{}
	}
}

// S05: a controllable failing entropy reader produces a non-nil error with no
// fallback id.
func TestStableIDEntropyFailure(t *testing.T) {
	orig := randomReader
	t.Cleanup(func() { randomReader = orig })
	wantErr := errors.New("entropy unavailable")
	randomReader = &failingReader{err: wantErr}
	id, err := New()
	if err == nil {
		t.Fatalf("expected entropy failure, got id %q", id)
	}
	if !errors.Is(err, ErrInvalidConceptID) {
		t.Fatalf("expected invalid_concept_id code, got %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected underlying entropy error wrapped, got %v", err)
	}
	if id != "" {
		t.Fatalf("must not fall back to an id, got %q", id)
	}
}

type failingReader struct{ err error }

func (f *failingReader) Read(p []byte) (int, error) {
	return 0, f.err
}

// S11: a syntactically valid but unknown ref fails with concept_ref_not_found.
func TestStableRefNotFound(t *testing.T) {
	legacy := conceptWithID("", "only/legacy.md")
	_, err := Resolve([]*okf.Concept{legacy}, "okf://concept/okf_11111111111111111111111111111111")
	if !errors.Is(err, ErrConceptRefNotFound) {
		t.Fatalf("expected concept_ref_not_found, got %v", err)
	}
	// An invalid ref is invalid, not not-found.
	_, err = Resolve([]*okf.Concept{legacy}, "okf://concept/not-a-ref")
	if !errors.Is(err, ErrInvalidConceptID) {
		t.Fatalf("expected invalid_concept_id, got %v", err)
	}
}

func TestKey(t *testing.T) {
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	if got := Key(id, "legacyfp"); got != "v3:id:"+id {
		t.Fatalf("stable key mismatch: %q", got)
	}
	if got := Key("", "type:title:path"); got != "v3:legacy:type:title:path" {
		t.Fatalf("legacy key mismatch: %q", got)
	}
	// An invalid id falls back to legacy rather than emitting a bogus id key.
	if got := Key("OKF_UPPER", "legacyfp"); !strings.HasPrefix(got, "v3:legacy:") {
		t.Fatalf("invalid id must fall back to legacy key, got %q", got)
	}
	if got := ChunkKey("v3:id:"+id, 3); got != "v3:id:"+id+"#3" {
		t.Fatalf("chunk key mismatch: %q", got)
	}
}
