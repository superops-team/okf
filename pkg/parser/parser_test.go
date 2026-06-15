package parser

import (
	"bytes"
	"testing"
)

func TestParseConceptBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		input       string
		wantType    string
		wantTitle   string
		wantContent string
		wantErr     bool
	}{
		{
			name:        "frontmatter with content",
			path:        "concept.md",
			input:       "---\ntype: api\ntitle: Users API\ntags:\n  - go\n---\n## Body\nDetails\n",
			wantType:    "api",
			wantTitle:   "Users API",
			wantContent: "## Body\nDetails\n",
		},
		{
			name:        "markdown without frontmatter becomes concept",
			path:        "hello-world.md",
			input:       "plain body\n",
			wantType:    "concept",
			wantTitle:   "hello world",
			wantContent: "plain body\n",
		},
		{
			name:        "CRLF frontmatter",
			path:        "windows.md",
			input:       "---\r\ntype: component\r\ntitle: Windows\r\n---\r\nBody\r\n",
			wantType:    "component",
			wantTitle:   "Windows",
			wantContent: "Body\r\n",
		},
		{
			name:    "missing title returns error",
			path:    "bad.md",
			input:   "---\ntype: api\n---\nBody\n",
			wantErr: true,
		},
		{
			name:    "malformed YAML returns error",
			path:    "bad-yaml.md",
			input:   "---\ntype: [\n---\nBody\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConceptBytes(tt.path, []byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Type != tt.wantType || got.Title != tt.wantTitle || got.Content != tt.wantContent {
				t.Fatalf("got type=%q title=%q content=%q, want type=%q title=%q content=%q", got.Type, got.Title, got.Content, tt.wantType, tt.wantTitle, tt.wantContent)
			}
		})
	}
}

func TestParseConceptBytesPreservesCustomFrontmatter(t *testing.T) {
	t.Parallel()

	got, err := ParseConceptBytes("generated.md", []byte(`---
type: code_file
title: handler.go
generated: true
generator: okf.git
generator_version: 1
source_path: internal/handler.go
source_kind: file
source_commit: abc123
---
Body
`))
	if err != nil {
		t.Fatalf("ParseConceptBytes returned error: %v", err)
	}

	if got.CustomFields["generated"] != true {
		t.Fatalf("generated = %#v, want true", got.CustomFields["generated"])
	}
	if got.CustomFields["generator"] != "okf.git" {
		t.Fatalf("generator = %#v, want okf.git", got.CustomFields["generator"])
	}
	if got.CustomFields["source_path"] != "internal/handler.go" {
		t.Fatalf("source_path = %#v, want internal/handler.go", got.CustomFields["source_path"])
	}
}

func TestSerializeConceptWritesCustomFrontmatter(t *testing.T) {
	t.Parallel()

	data, err := SerializeConcept(&Concept{
		Type:  "code_file",
		Title: "handler.go",
		CustomFields: map[string]interface{}{
			"generated":         true,
			"generator":         "okf.git",
			"generator_version": 1,
			"source_path":       "internal/handler.go",
			"source_kind":       "file",
			"source_commit":     "abc123",
		},
		Content: "Body\n",
	}, true)
	if err != nil {
		t.Fatalf("SerializeConcept returned error: %v", err)
	}

	for _, want := range [][]byte{
		[]byte("generated: true"),
		[]byte("generator: okf.git"),
		[]byte("generator_version: 1"),
		[]byte("source_path: internal/handler.go"),
		[]byte("source_kind: file"),
		[]byte("source_commit: abc123"),
	} {
		if !bytes.Contains(data, want) {
			t.Fatalf("serialized concept missing %q\n%s", want, data)
		}
	}
}
