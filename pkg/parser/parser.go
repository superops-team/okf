// Package parser provides parsing and serialization for OKF concepts.
package parser

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Concept represents a parsed concept with YAML frontmatter parsed.
type Concept struct {
	Type         string
	Title        string
	Description  string
	Resource     string
	Tags         []string
	Timestamp    string
	Content      string
	FilePath     string
	CustomFields map[string]interface{}
}

// ParseConcept parses a single markdown file with YAML frontmatter.
func ParseConcept(path string) (*Concept, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{FilePath: path, Message: "failed to read file: " + err.Error()}
	}

	return ParseConceptBytes(path, data)
}

// ParseConceptBytes parses concept content from raw bytes.
func ParseConceptBytes(path string, data []byte) (*Concept, error) {
	if data == nil {
		return nil, &ParseError{FilePath: path, Message: "data is nil"}
	}
	if len(data) == 0 {
		return nil, &ParseError{FilePath: path, Message: "data is empty"}
	}
	endIdx := findFrontmatterEnd(data)
	if endIdx == -1 {
		return &Concept{
			Type:     "concept",
			Title:    titleFromPath(path),
			Content:  string(data),
			FilePath: path,
		}, nil
	}

	yamlContent := data[3 : endIdx+3]
	var fm frontmatter
	if err := yaml.Unmarshal(yamlContent, &fm); err != nil {
		return nil, &ParseError{FilePath: path, Line: 1, Message: "failed to parse YAML: " + err.Error()}
	}

	contentStart := endIdx + 3
	for contentStart < len(data) && (data[contentStart] == '\n' || data[contentStart] == '\r') {
		contentStart++
	}
	content := string(data[contentStart:])

	concept := &Concept{
		Type:         fm.Type,
		Title:        fm.Title,
		Description:  fm.Description,
		Resource:     fm.Resource,
		Tags:         fm.Tags,
		Timestamp:    fm.Timestamp,
		Content:      content,
		FilePath:     path,
		CustomFields: fm.CustomFields,
	}

	if concept.Title == "" {
		return nil, &ParseError{FilePath: path, Line: 1, Message: "title is required"}
	}
	if concept.Type == "" {
		return nil, &ParseError{FilePath: path, Line: 1, Message: "type is required"}
	}

	return concept, nil
}

// SerializeConcept converts a concept back to markdown with YAML frontmatter.
func SerializeConcept(c *Concept, prettyPrint bool) ([]byte, error) {
	fm := frontmatter{
		Type:         c.Type,
		Title:        c.Title,
		Description:  c.Description,
		Resource:     c.Resource,
		Tags:         c.Tags,
		Timestamp:    c.Timestamp,
		CustomFields: c.CustomFields,
	}

	yamlData, err := yaml.Marshal(&fm)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

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

// ParseError represents a parsing error.
type ParseError struct {
	FilePath string
	Line     int
	Message  string
}

func (e *ParseError) Error() string {
	loc := e.FilePath
	if e.Line > 0 {
		loc = fmt.Sprintf("%s:%d", e.FilePath, e.Line)
	}
	return fmt.Sprintf("%s: %s", loc, e.Message)
}

// frontmatter represents the YAML structure at the start of a concept file.
type frontmatter struct {
	Type         string                 `yaml:"type"`
	Title        string                 `yaml:"title"`
	Description  string                 `yaml:"description,omitempty"`
	Resource     string                 `yaml:"resource,omitempty"`
	Tags         []string               `yaml:"tags,omitempty"`
	Timestamp    string                 `yaml:"timestamp,omitempty"`
	CustomFields map[string]interface{} `yaml:",inline"`
}

func findFrontmatterEnd(data []byte) int {
	if len(data) < 4 || !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return -1
	}

	for i := 3; i < len(data)-3; i++ {
		if data[i] == '-' && data[i+1] == '-' && data[i+2] == '-' {
			next := i + 3
			if next < len(data) && (data[next] == '\n' || data[next] == '\r') {
				return i
			}
		}
	}
	return -1
}

func titleFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext == ".md" {
		base = base[:len(base)-len(ext)]
	}
	return strings.ReplaceAll(strings.ReplaceAll(base, "_", " "), "-", " ")
}
