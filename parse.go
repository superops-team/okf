package okf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseError represents an error during parsing with context about the file.
type ParseError struct {
	FilePath string
	Line     int
	Message  string
}

func (e *ParseError) Error() string {
	if e.FilePath != "" {
		return fmt.Sprintf("%s:%d: %s", e.FilePath, e.Line, e.Message)
	}
	return e.Message
}

// conceptFrontmatter represents the YAML structure at the start of a concept file.
// It captures all standard OKF fields plus any custom fields.
type conceptFrontmatter struct {
	Type        string   `yaml:"type"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description,omitempty"`
	Resource    string   `yaml:"resource,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Timestamp   string   `yaml:"timestamp,omitempty"`

	// Capture unknown fields for CustomFields
	Raw map[string]interface{} `yaml:",inline"`
}

// ParseConcept parses a single markdown file with YAML frontmatter.
// The file should start with --- boundaries containing YAML, followed by markdown content.
func ParseConcept(path string) (*Concept, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{FilePath: path, Message: "failed to read file: " + err.Error()}
	}

	concept, err := parseConceptBytes(path, data)
	if err != nil {
		return nil, err
	}

	return concept, nil
}

// ParseConceptBytes parses concept content from raw bytes.
// This is useful when the content is already in memory.
func ParseConceptBytes(path string, data []byte) (*Concept, error) {
	return parseConceptBytes(path, data)
}

func parseConceptBytes(path string, data []byte) (*Concept, error) {
	// Look for YAML frontmatter delimiters
	frontmatterEnd := findFrontmatterEnd(data)
	if frontmatterEnd == -1 {
		// No frontmatter found - treat entire file as content
		return &Concept{
			Type:     "concept",
			Title:    titleFromPath(path),
			Content:  string(data),
			FilePath: path,
		}, nil
	}

	// Extract and parse YAML frontmatter
	yamlContent := data[3 : frontmatterEnd+3] // Include the closing ---

	var fm conceptFrontmatter
	if err := yaml.Unmarshal(yamlContent, &fm); err != nil {
		return nil, &ParseError{
			FilePath: path,
			Message:  "failed to parse YAML frontmatter: " + err.Error(),
		}
	}

	// Extract markdown content after frontmatter
	contentStart := frontmatterEnd + 3
	// Skip leading newlines after ---
	for contentStart < len(data) && data[contentStart] == '\n' {
		contentStart++
	}
	content := string(data[contentStart:])

	// Build the concept
	concept := &Concept{
		Type:        fm.Type,
		Title:       fm.Title,
		Description: fm.Description,
		Resource:    fm.Resource,
		Tags:        fm.Tags,
		Timestamp:   fm.Timestamp,
		Content:     content,
		FilePath:    path,
	}

	// Copy unknown fields to CustomFields
	if len(fm.Raw) > 0 {
		concept.CustomFields = make(map[string]interface{})
		knownFields := map[string]bool{
			"type": true, "title": true, "description": true,
			"resource": true, "tags": true, "timestamp": true,
		}
		for k, v := range fm.Raw {
			if !knownFields[k] {
				concept.CustomFields[k] = v
			}
		}
	}

	// Validate required fields
	if concept.Title == "" {
		return nil, &ParseError{
			FilePath: path,
			Line:     1,
			Message:  "title is required in frontmatter",
		}
	}
	if concept.Type == "" {
		return nil, &ParseError{
			FilePath: path,
			Line:     1,
			Message:  "type is required in frontmatter",
		}
	}

	return concept, nil
}

// findFrontmatterEnd finds the index of the closing --- in the frontmatter block.
// Returns -1 if no valid frontmatter is found.
func findFrontmatterEnd(data []byte) int {
	// Must start with ---
	if len(data) < 4 || !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return -1
	}

	// Find the closing --- (not followed by another ---)
	for i := 3; i < len(data)-3; i++ {
		if data[i] == '-' && data[i+1] == '-' && data[i+2] == '-' {
			// Make sure it's followed by a newline
			next := i + 3
			if next < len(data) && (data[next] == '\n' || data[next] == '\r') {
				return i
			}
		}
	}
	return -1
}

// titleFromPath extracts a title candidate from a file path.
func titleFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext == ".md" {
		base = base[:len(base)-len(ext)]
	}
	// Convert underscores and hyphens to spaces
	title := strings.ReplaceAll(base, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")
	return title
}

// SerializeConcept converts a concept back to markdown with YAML frontmatter.
func SerializeConcept(c *Concept, prettyPrint bool) ([]byte, error) {
	// Build frontmatter
	fm := conceptFrontmatter{
		Type:        c.Type,
		Title:       c.Title,
		Description: c.Description,
		Resource:    c.Resource,
		Tags:        c.Tags,
		Timestamp:   c.Timestamp,
	}

	var yamlData []byte
	var err error
	if prettyPrint {
		yamlData, err = yaml.Marshal(&fm)
	} else {
		yamlData, err = yaml.Marshal(&fm)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Build output: --- + yaml + --- + content
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlData)
	buf.WriteString("---\n")
	if !strings.HasSuffix(c.Content, "\n") && c.Content != "" {
		buf.WriteString("\n")
	}
	buf.WriteString(c.Content)

	return buf.Bytes(), nil
}
