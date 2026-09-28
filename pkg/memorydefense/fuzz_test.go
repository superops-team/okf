package memorydefense

import (
	"strings"
	"testing"
)

// FuzzScreenNeverCrashes ensures Screen handles arbitrary input without panicking.
func FuzzScreenNeverCrashes(f *testing.F) {
	f.Add("hello world")
	f.Add("ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234")
	f.Add("4111-1111-1111-1111")
	f.Fuzz(func(t *testing.T, content string) {
		pol := Policy{Enabled: true, Action: "redact"}
		_, _, _ = Screen(content, pol)
		pol.Action = "block"
		_, _, _ = Screen(content, pol)
	})
}

// TestCatalogNoDuplicateIDs ensures no detector ID is duplicated.
func TestCatalogNoDuplicateIDs(t *testing.T) {
	cat := Catalog()
	seen := map[string]bool{}
	for _, d := range cat {
		if seen[d.ID] {
			t.Fatalf("duplicate detector ID %q", d.ID)
		}
		seen[d.ID] = true
	}
}

// TestRedactionDeterministic ensures same input → same output.
func TestRedactionDeterministic(t *testing.T) {
	pol := Policy{Enabled: true, Action: "redact"}
	in := "key ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234 and aws AKIAIOSFODNN7EXAMPLE"
	out1, _, _ := Screen(in, pol)
	out2, _, _ := Screen(in, pol)
	if out1 != out2 {
		t.Fatalf("non-deterministic: %q vs %q", out1, out2)
	}
	if strings.Contains(out1, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234") {
		t.Fatal("original github_pat leaked")
	}
	if strings.Contains(out1, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal("original aws key leaked")
	}
}
