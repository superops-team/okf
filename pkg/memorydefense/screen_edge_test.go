package memorydefense

import (
	"strings"
	"testing"
)

func TestScreenEmptyInput(t *testing.T) {
	pol := Policy{Enabled: true, Action: "redact"}
	out, hits, err := Screen("", pol)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Error("empty input should stay empty")
	}
	if len(hits) != 0 {
		t.Error("empty input should have no hits")
	}
}

func TestScreenMultipleSecretsRedact(t *testing.T) {
	pol := Policy{Enabled: true, Action: "redact"}
	content := "key1 ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234 and key2 AKIAIOSFODNN7EXAMPLE"
	out, hits, err := Screen(content, pol)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234") {
		t.Error("first secret not redacted")
	}
	if strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("second secret not redacted")
	}
	if len(hits) < 2 {
		t.Errorf("expected >=2 hits, got %d", len(hits))
	}
}

func TestScreenLargeInputNoCrash(t *testing.T) {
	pol := Policy{Enabled: true, Action: "redact"}
	// 1MB of normal text
	big := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 25000)
	out, _, err := Screen(big, pol)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(big) {
		t.Errorf("no hits expected in normal text, len mismatch: %d vs %d", len(out), len(big))
	}
}

func TestScreenInvalidConfigAction(t *testing.T) {
	// LoadPolicy with invalid action returns error.
	// (Already covered in TestLoadPolicyInvalidAction; this verifies Screen
	// itself doesn't panic on unknown action.)
	pol := Policy{Enabled: true, Action: "explode"}
	out, _, err := Screen("hello", pol)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Error("unknown action should pass content through")
	}
}

func TestScreenMultilinePEM(t *testing.T) {
	pol := Policy{Enabled: true, Action: "block"}
	pem := "some text\n-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----\nmore"
	_, _, err := Screen(pem, pol)
	if err == nil {
		t.Fatal("expected block error for PEM")
	}
}
