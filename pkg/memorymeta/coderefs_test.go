package memorymeta

import (
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// S10: code_refs read from CustomFields, normalized.
func TestCodeRefsRead(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{
		"code_refs": []any{"pkg/mcp/*.go", "cmd/okf/main.go"},
	}}
	refs, warns := CodeRefs(c)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0] != "pkg/mcp/*.go" {
		t.Errorf("refs[0] = %q", refs[0])
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestCodeRefsEmpty(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{}}
	refs, warns := CodeRefs(c)
	if len(refs) != 0 {
		t.Errorf("expected empty, got %v", refs)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestCodeRefsNilCustomFields(t *testing.T) {
	c := &okf.Concept{}
	refs, _ := CodeRefs(c)
	if len(refs) != 0 {
		t.Errorf("expected empty for nil CustomFields")
	}
}

// S16: code_refs pattern bounds.
func TestCodeRefsBounds(t *testing.T) {
	// Too many patterns (17)
	patterns := make([]any, 17)
	for i := range patterns {
		patterns[i] = "pkg/a.go"
	}
	c := &okf.Concept{CustomFields: map[string]any{"code_refs": patterns}}
	_, warns := CodeRefs(c)
	if len(warns) == 0 {
		t.Error("17 patterns should produce warning")
	}

	// Pattern too long (>256 bytes)
	longPattern := string(make([]byte, 257))
	c2 := &okf.Concept{CustomFields: map[string]any{"code_refs": []any{longPattern}}}
	_, warns2 := CodeRefs(c2)
	if len(warns2) == 0 {
		t.Error("257-byte pattern should produce warning")
	}

	// Too many segments (>32)
	segPattern := "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/aa/bb/cc/dd/ee/ff/gg.go"
	c3 := &okf.Concept{CustomFields: map[string]any{"code_refs": []any{segPattern}}}
	_, warns3 := CodeRefs(c3)
	if len(warns3) == 0 {
		t.Error(">32 segments should produce warning")
	}

	// Too many ** operators (>2)
	doubleStar := "**/a/**/b/**/c.go"
	c4 := &okf.Concept{CustomFields: map[string]any{"code_refs": []any{doubleStar}}}
	_, warns4 := CodeRefs(c4)
	if len(warns4) == 0 {
		t.Error(">2 ** operators should produce warning")
	}
}

// S14: path canonicalization and rejection.
func TestCodeRefsPathRejection(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		wantWarn bool
	}{
		{"valid relative", "pkg/mcp/server.go", false},
		{"valid glob", "pkg/**/*.go", false},
		{"absolute path", "/etc/passwd", true},
		{"dotdot escape", "../secret/file", true},
		{"backslash", `pkg\mcp\server.go`, true},
		{"leading dot slash", "./pkg/mcp.go", false}, // normalized
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &okf.Concept{CustomFields: map[string]any{"code_refs": []any{tt.pattern}}}
			refs, warns := CodeRefs(c)
			if (len(warns) > 0) != tt.wantWarn {
				t.Errorf("pattern %q: warns=%v, wantWarn=%v, refs=%v", tt.pattern, warns, tt.wantWarn, refs)
			}
		})
	}
}

// S11: for_path exact match.
func TestMatchCodeRefsExact(t *testing.T) {
	patterns := []string{"pkg/mcp/server.go", "cmd/okf/main.go"}
	matched, ok := MatchCodeRefs("pkg/mcp/server.go", patterns)
	if !ok {
		t.Error("exact match should succeed")
	}
	if matched != "pkg/mcp/server.go" {
		t.Errorf("matched = %q", matched)
	}
	_, ok = MatchCodeRefs("pkg/mcp/other.go", patterns)
	if ok {
		t.Error("non-matching path should not match")
	}
}

// S12: for_path single-segment glob.
func TestMatchCodeRefsSingleGlob(t *testing.T) {
	patterns := []string{"pkg/mcp/*.go"}
	_, ok := MatchCodeRefs("pkg/mcp/server.go", patterns)
	if !ok {
		t.Error("single-segment glob should match")
	}
	_, ok = MatchCodeRefs("pkg/mcp/sub/server.go", patterns)
	if ok {
		t.Error("single-segment glob should NOT match subdirectory")
	}
}

// S13: for_path recursive glob depth limit (each ** max 8 segments).
func TestMatchCodeRefsRecursiveDepth(t *testing.T) {
	patterns := []string{"pkg/**/*.go"}
	// 8 segments after pkg/
	_, ok := MatchCodeRefs("pkg/a/b/c/d/e/f/g/h/file.go", patterns)
	if !ok {
		t.Error("8 segments should match (each ** max 8)")
	}
	// 9 segments after pkg/
	_, ok = MatchCodeRefs("pkg/a/b/c/d/e/f/g/h/i/file.go", patterns)
	if ok {
		t.Error("9 segments should NOT match (each ** max 8)")
	}
}

// S15: for_path supports not-yet-created paths (lexical only, no FS).
func TestMatchCodeRefsNoFS(t *testing.T) {
	patterns := []string{"pkg/newfeature/*.go"}
	_, ok := MatchCodeRefs("pkg/newfeature/widget.go", patterns)
	if !ok {
		t.Error("not-yet-created path should match lexically")
	}
}

// S09: SetCodeRefs writes to CustomFields.
func TestSetCodeRefs(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{}}
	SetCodeRefs(c, []string{"pkg/a.go", "pkg/b.go"})
	refs, _ := CodeRefs(c)
	if len(refs) != 2 {
		t.Errorf("expected 2 refs, got %d", len(refs))
	}
}
