package okf

import (
	"strings"
	"testing"
	"testing/quick"
)

// Property-based tests for the hand-rolled string helpers (helpers.go).
// These encode invariants that ordinary example-based tests cannot enumerate:
// ASCII case-fold must agree with strings.EqualFold, and sanitizeFilename must
// never emit a path-unsafe character and must be idempotent.

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// equalFold must agree with strings.EqualFold on ASCII inputs and be symmetric.
func TestPropertyEqualFoldMatchesStdlib(t *testing.T) {
	prop := func(a, b string) bool {
		if !isASCII(a) || !isASCII(b) {
			return true // only ASCII is in scope for the hand-rolled fold
		}
		if equalFold(a, b) != strings.EqualFold(a, b) {
			return false
		}
		// symmetry
		return equalFold(a, b) == equalFold(b, a)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// containsFold is true exactly when indexFold finds a match.
func TestPropertyContainsFoldMatchesIndexFold(t *testing.T) {
	prop := func(s, sub string) bool {
		if !isASCII(s) || !isASCII(sub) {
			return true
		}
		return containsFold(s, sub) == (indexFold(s, sub) >= 0)
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// sanitizeFilename must never emit a path-unsafe character and must be idempotent.
func TestPropertySanitizeFilenameSafeAndIdempotent(t *testing.T) {
	unsafe := "/\\:*?\"<>|"
	prop := func(s string) bool {
		clean := sanitizeFilename(s)
		if strings.ContainsAny(clean, unsafe) {
			return false
		}
		// idempotent: cleaning an already-clean name is a no-op
		return sanitizeFilename(clean) == clean
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}
