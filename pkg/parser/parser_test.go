package parser

import "testing"

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
