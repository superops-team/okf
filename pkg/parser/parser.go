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
// v0.2: extended with provenance, trust, lifecycle, and attested computation fields.
type Concept struct {
	// v0.1 fields
	Type        string
	Title       string
	Description string
	Resource    string
	Tags        []string
	Timestamp   string // legacy v0.1

	// v0.2 fields
	Sources     []Source
	UsageWindow *UsageWindow
	Generated   *GeneratedInfo
	Verified    []VerificationEvent
	Status      string
	StaleAfter  string
	Runtime     string
	Parameters  []Parameter
	Computation string
	Executor    *ExecutorRef
	Attester    *AttesterRef

	// internal
	Content      string
	FilePath     string
	CustomFields map[string]interface{}
}

// Source records a material a concept derives from (spec §5.1).
type Source struct {
	ID           string `yaml:"id,omitempty"`
	Resource     string `yaml:"resource"`
	Title        string `yaml:"title,omitempty"`
	Author       string `yaml:"author,omitempty"`
	UsageCount   int    `yaml:"usage_count,omitempty"`
	LastModified string `yaml:"last_modified,omitempty"`
}

// UsageWindow frames usage_count with a date range (spec §5.1).
type UsageWindow struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// GeneratedInfo records how content was produced (spec §5.2).
type GeneratedInfo struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"`
}

// VerificationEvent records a verification (spec §5.2).
type VerificationEvent struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"`
}

// Parameter is a typed hole in an Attested Computation (spec §10.2).
type Parameter struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required,omitempty"`
}

// ExecutorRef describes how a computation is run (spec §10.2).
type ExecutorRef struct {
	Resource string   `yaml:"resource"`
	Receipt  []string `yaml:"receipt,omitempty"`
}

// AttesterRef describes the deterministic check (spec §10.2).
type AttesterRef struct {
	Resource string `yaml:"resource"`
}

// reservedFilenames are filenames that MUST NOT be used for concept documents (spec §3.1).
var reservedFilenames = map[string]bool{
	"index.md": true,
	"log.md":   true,
}

// IsReservedFilename reports whether path's base name is a reserved filename (spec §3.1).
func IsReservedFilename(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return reservedFilenames[base]
}

// ParseConcept parses a single markdown file with YAML frontmatter.
// v0.2: title is optional (derived from filename if missing), supports all v0.2 fields,
// and applies v0.1 fallbacks (timestamp→generated.at, # Citations→sources).
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

	// Skip reserved filenames (index.md, log.md) — they are not concepts.
	if IsReservedFilename(path) {
		return nil, &ParseError{FilePath: path, Message: "reserved filename, not a concept document"}
	}

	endIdx := findFrontmatterEnd(data)
	if endIdx == -1 {
		// No frontmatter: v0.2 conformance requires parseable YAML frontmatter (spec §11).
		// We return a minimal concept with type derived, matching lenient consumer behavior.
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
		Sources:      fm.Sources,
		UsageWindow:  fm.UsageWindow,
		Status:       fm.Status,
		StaleAfter:   fm.StaleAfter,
		Runtime:      fm.Runtime,
		Parameters:   fm.Parameters,
		Computation:  fm.Computation,
		Executor:     fm.Executor,
		Attester:     fm.Attester,
		Content:      content,
		FilePath:     path,
		CustomFields: fm.CustomFields,
	}

	// v0.2: verified flexible parsing (list or single mapping, spec §5.2)
	concept.Verified = normalizeVerified(fm.Verified)

	// v0.2: generated flexible parsing (struct or legacy boolean, spec §5.2 + §13.1)
	concept.Generated = normalizeGenerated(fm.Generated, concept.CustomFields)

	// v0.2: title is optional; derive from filename if missing (spec §4.1)
	if concept.Title == "" {
		concept.Title = titleFromPath(path)
	}

	// v0.2: type is the only required field (spec §4.1, §11)
	if concept.Type == "" {
		return nil, &ParseError{FilePath: path, Line: 1, Message: "type is required"}
	}

	// v0.1 fallbacks (spec §13.1)
	applyV01Fallbacks(concept)

	return concept, nil
}

// normalizeVerified converts the raw verified field (which may be a list or a single mapping,
// spec §5.2) into a normalized []VerificationEvent.
func normalizeVerified(raw interface{}) []VerificationEvent {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		events := make([]VerificationEvent, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				events = append(events, mapToVerificationEvent(m))
			}
		}
		return events
	case map[string]interface{}:
		// Bare mapping: treat as one-element list (spec §5.2)
		return []VerificationEvent{mapToVerificationEvent(v)}
	default:
		return nil
	}
}

func mapToVerificationEvent(m map[string]interface{}) VerificationEvent {
	e := VerificationEvent{}
	if by, ok := m["by"].(string); ok {
		e.By = by
	}
	if at, ok := m["at"].(string); ok {
		e.At = at
	}
	return e
}

// normalizeGenerated converts the raw generated field (which may be a v0.2 struct mapping
// or a legacy v0.1 boolean, spec §5.2 + §13.1) into *GeneratedInfo.
// Legacy boolean true is preserved in customFields for backward compatibility.
func normalizeGenerated(raw interface{}, customFields map[string]interface{}) *GeneratedInfo {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case map[string]interface{}:
		gi := &GeneratedInfo{}
		if by, ok := v["by"].(string); ok {
			gi.By = by
		}
		if at, ok := v["at"].(string); ok {
			gi.At = at
		}
		return gi
	case bool:
		// Legacy v0.1: generated: true — preserve in CustomFields, return nil struct
		if customFields != nil {
			customFields["generated"] = v
		}
		return nil
	default:
		return nil
	}
}

// applyV01Fallbacks applies v0.1→v0.2 migrations (spec §13.1):
// 1. timestamp → generated.at (when generated is absent)
// 2. body # Citations → sources (when sources is empty)
func applyV01Fallbacks(c *Concept) {
	// Fallback 1: timestamp → generated.at
	if c.Generated == nil && c.Timestamp != "" {
		c.Generated = &GeneratedInfo{By: "unknown", At: c.Timestamp}
	}

	// Fallback 2: body # Citations → sources
	if len(c.Sources) == 0 && c.Content != "" {
		if citations := extractCitationsFromBody(c.Content); len(citations) > 0 {
			c.Sources = citations
		}
	}
}

// extractCitationsFromBody extracts URLs from a legacy "# Citations" body section (spec §13.1).
func extractCitationsFromBody(content string) []Source {
	lines := strings.Split(content, "\n")
	inCitations := false
	var sources []Source
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inCitations {
			if strings.HasPrefix(strings.ToLower(trimmed), "# citations") {
				inCitations = true
			}
			continue
		}
		// Stop at next heading
		if strings.HasPrefix(trimmed, "#") {
			break
		}
		// Extract URL from list items: "- https://..." or "* https://..."
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			url := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* "))
			if url != "" {
				sources = append(sources, Source{
					ID:       fmt.Sprintf("citation-%d", len(sources)),
					Resource: url,
				})
			}
		}
	}
	return sources
}

// SerializeConcept converts a concept back to markdown with YAML frontmatter.
// v0.2: supports all v0.2 fields with omitempty.
func SerializeConcept(c *Concept, prettyPrint bool) ([]byte, error) {
	// Build custom fields. Keys that conflict with frontmatter struct fields
	// (generated, verified, etc.) MUST be removed from the inline map to avoid
	// yaml.v3 panic. Legacy boolean values are routed through the struct field instead.
	customFields := make(map[string]interface{}, len(c.CustomFields))
	legacyGeneratedBool := false
	hasLegacyGenerated := false
	isReserved := func(key string) bool {
		switch key {
		case "type", "title", "description", "resource", "tags",
			"timestamp", "sources", "usage_window", "generated", "verified",
			"status", "stale_after", "runtime", "parameters", "computation",
			"executor", "attester":
			return true
		default:
			return false
		}
	}
	for k, v := range c.CustomFields {
		if k == "generated" {
			// Capture legacy boolean for routing through struct field
			if b, ok := v.(bool); ok {
				legacyGeneratedBool = b
				hasLegacyGenerated = true
			}
			continue
		}
		if !isReserved(k) {
			customFields[k] = v
		}
	}

	fm := frontmatter{
		Type:         c.Type,
		Title:        c.Title,
		Description:  c.Description,
		Resource:     c.Resource,
		Tags:         c.Tags,
		Timestamp:    c.Timestamp,
		Sources:      c.Sources,
		UsageWindow:  c.UsageWindow,
		Status:       c.Status,
		StaleAfter:   c.StaleAfter,
		Runtime:      c.Runtime,
		Parameters:   c.Parameters,
		Computation:  c.Computation,
		Executor:     c.Executor,
		Attester:     c.Attester,
		CustomFields: customFields,
	}
	// Only set verified if non-empty (interface{} zero-value would serialize as "verified: []")
	if len(c.Verified) > 0 {
		fm.Verified = c.Verified
	}
	// Only set generated if non-nil (interface{} zero-value would serialize as "generated: null")
	// Convert to map for safe interface{} serialization.
	if c.Generated != nil {
		fm.Generated = map[string]interface{}{
			"by": c.Generated.By,
		}
		if c.Generated.At != "" {
			fm.Generated.(map[string]interface{})["at"] = c.Generated.At
		}
	} else if hasLegacyGenerated {
		// Legacy v0.1: generated: true — output as boolean via struct field
		fm.Generated = legacyGeneratedBool
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
// v0.2: extended with all v0.2 fields. Verified uses yaml.Node for flexible list/mapping parsing.
type frontmatter struct {
	// v0.1
	Type        string   `yaml:"type"`
	Title       string   `yaml:"title,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Resource    string   `yaml:"resource,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Timestamp   string   `yaml:"timestamp,omitempty"`

	// v0.2 provenance / trust / lifecycle
	Sources     []Source     `yaml:"sources,omitempty"`
	UsageWindow *UsageWindow `yaml:"usage_window,omitempty"`
	Generated   interface{}  `yaml:"generated,omitempty"`
	Verified    interface{}  `yaml:"verified,omitempty"`
	Status      string       `yaml:"status,omitempty"`
	StaleAfter  string       `yaml:"stale_after,omitempty"`

	// v0.2 Attested Computation
	Runtime     string       `yaml:"runtime,omitempty"`
	Parameters  []Parameter  `yaml:"parameters,omitempty"`
	Computation string       `yaml:"computation,omitempty"`
	Executor    *ExecutorRef `yaml:"executor,omitempty"`
	Attester    *AttesterRef `yaml:"attester,omitempty"`

	CustomFields map[string]interface{} `yaml:",inline"`
}

func findFrontmatterEnd(data []byte) int {
	if len(data) < 4 {
		return -1
	}
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return -1
	}
	for i := 3; i <= len(data)-3; i++ {
		if data[i] == '-' && data[i+1] == '-' && data[i+2] == '-' {
			next := i + 3
			if next >= len(data) {
				return i
			}
			if data[next] == '\n' || data[next] == '\r' {
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
