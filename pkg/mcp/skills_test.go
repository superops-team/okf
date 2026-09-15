package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

func TestNewSkillRegistry(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatalf("NewSkillRegistry: %v", err)
	}
	skills := r.List()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].URI != "skill://okf/SKILL.md" {
		t.Errorf("expected skill://okf/SKILL.md, got %s", skills[0].URI)
	}
	if skills[0].Name != "okf" {
		t.Errorf("expected name okf, got %s", skills[0].Name)
	}
}

func TestSkillRegistryDigest(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatal(err)
	}
	d := r.Digest()
	if !strings.HasPrefix(d, "sha256:") {
		t.Errorf("digest must start with sha256:, got %s", d)
	}
	if len(d) != 7+64 { // "sha256:" + 64 hex
		t.Errorf("digest length wrong: %d", len(d))
	}
	// Verify digest matches actual content
	content, err := r.Read("skill://okf/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	actualHash := sha256.Sum256([]byte(content.Text))
	expected := "sha256:" + hex.EncodeToString(actualHash[:])
	if d != expected {
		t.Errorf("digest mismatch: got %s, expected %s", d, expected)
	}
}

func TestSkillRegistrySize(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if r.Size() <= 0 {
		t.Error("size must be positive")
	}
}

func TestSkillRegistryGet(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.Get("skill://okf/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "okf" {
		t.Errorf("expected name okf, got %s", s.Name)
	}
	_, err = r.Get("skill://okf/other.md")
	if err == nil {
		t.Error("expected error for unknown URI")
	}
}

func TestSkillRegistryRead(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Read("skill://okf/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if c.MIMEType != "text/markdown" {
		t.Errorf("expected text/markdown, got %s", c.MIMEType)
	}
	if !strings.Contains(c.Text, "W01") {
		t.Error("content must contain W01")
	}
}

func TestSkillRegistryDefensiveCopy(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Mutate returned slice
	skills := r.List()
	skills[0].Name = "hacked"
	// Original should be unchanged
	skills2 := r.List()
	if skills2[0].Name != "okf" {
		t.Errorf("registry was mutated: %s", skills2[0].Name)
	}
}

func TestSkillRegistryConcurrentReads(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
			_, _ = r.Get("skill://okf/SKILL.md")
			_ = r.Resources()
			_, _ = r.Read("skill://okf/SKILL.md")
			_ = r.Digest()
			_ = r.Size()
		}()
	}
	wg.Wait()
}

func TestValidateSkillURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{"valid", "skill://okf/SKILL.md", false},
		{"wrong scheme", "http://okf/SKILL.md", true},
		{"userinfo", "skill://user@okf/SKILL.md", true},
		{"port", "skill://okf:8080/SKILL.md", true},
		{"query", "skill://okf/SKILL.md?q=1", true},
		{"fragment", "skill://okf/SKILL.md#frag", true},
		{"dot segment", "skill://okf/./SKILL.md", true},
		{"dotdot segment", "skill://okf/../SKILL.md", true},
		{"missing SKILL.md", "skill://okf/OTHER.md", true},
		{"wrong host", "skill://other/SKILL.md", true},
		{"encoded dot traversal", "skill://okf/%2e%2e/SKILL.md", true},
		{"encoded slash traversal", "skill://okf/..%2fSKILL.md", true},
		{"encoded backslash", "skill://okf/%5cSKILL.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSkillURI(tt.uri, "okf")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSkillURI(%q) error = %v, wantErr %v", tt.uri, err, tt.wantErr)
			}
		})
	}
}

func TestSkillRegistryResources(t *testing.T) {
	r, err := NewSkillRegistry()
	if err != nil {
		t.Fatal(err)
	}
	res := r.Resources()
	if len(res) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(res))
	}
	if res[0].URI != "skill://okf/SKILL.md" {
		t.Errorf("expected skill URI, got %s", res[0].URI)
	}
	if res[0].MIMEType != "text/markdown" {
		t.Errorf("expected text/markdown, got %s", res[0].MIMEType)
	}
}
