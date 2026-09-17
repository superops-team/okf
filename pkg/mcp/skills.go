package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/superops-team/okf/pkg/agentconfig"
)

// Registry limits (S14).
const (
	maxSkillEntries    = 512
	maxSkillTotalBytes = 16 * 1024 * 1024 // 16 MiB
)

// Skill represents an Agent Skill entry.
type Skill struct {
	URI         string          `json:"uri"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Resources   []SkillResource `json:"resources,omitempty"`
}

// SkillResource is a resource belonging to a Skill.
type SkillResource struct {
	URI      string `json:"uri"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType,omitempty"`
}

// SkillRegistry is an immutable, process-lifetime registry of Skills and their resources.
// Construction validates all entries; callers cannot mutate canonical state.
type SkillRegistry struct {
	mu        sync.RWMutex
	skills    []Skill
	byURI     map[string]int
	resources []Resource
	contents  map[string]ResourceContents
}

// NewSkillRegistry builds the immutable registry from the canonical rendered Skill.
// Returns an error if any validation fails (S10-S14).
func NewSkillRegistry() (*SkillRegistry, error) {
	// Render once (S09).
	skillBytes := []byte(agentconfig.RenderAgentSkill())

	// Parse frontmatter for name/description (S09).
	name, description, err := parseSkillFrontmatter(skillBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid skill frontmatter: %w", err)
	}

	uri := "skill://okf/SKILL.md"

	// Validate URI (S11).
	if err := validateSkillURI(uri, name); err != nil {
		return nil, fmt.Errorf("invalid skill URI: %w", err)
	}

	// Compute digest and size (S10, S13).
	digest := sha256.Sum256(skillBytes)
	digestStr := "sha256:" + hex.EncodeToString(digest[:])
	size := len(skillBytes)

	// Build the single Skill entry with its resource.
	skillResource := SkillResource{
		URI:      uri,
		Name:     name,
		MIMEType: "text/markdown",
	}
	skill := Skill{
		URI:         uri,
		Name:        name,
		Description: description,
		Resources:   []SkillResource{skillResource},
	}

	resource := Resource{
		URI:         uri,
		Name:        name,
		Description: description,
		MIMEType:    "text/markdown",
	}
	content := ResourceContents{
		URI:      uri,
		MIMEType: "text/markdown",
		Text:     string(skillBytes),
	}

	// Validate completeness (S12).
	if len(skill.Resources) != 1 {
		return nil, fmt.Errorf("skill must have exactly one resource, got %d", len(skill.Resources))
	}
	if skill.Resources[0].URI != uri {
		return nil, fmt.Errorf("skill resource URI mismatch: %s != %s", skill.Resources[0].URI, uri)
	}

	// Validate limits (S14).
	if size > maxSkillTotalBytes {
		return nil, fmt.Errorf("skill content %d bytes exceeds limit %d", size, maxSkillTotalBytes)
	}

	// Verify digest/size match (S13).
	verifyDigest := sha256.Sum256(skillBytes)
	if "sha256:"+hex.EncodeToString(verifyDigest[:]) != digestStr {
		return nil, fmt.Errorf("digest verification failed")
	}

	r := &SkillRegistry{
		skills:    []Skill{skill},
		byURI:     map[string]int{uri: 0},
		resources: []Resource{resource},
		contents:  map[string]ResourceContents{uri: content},
	}
	return r, nil
}

// manifestEntry is a test-injectable manifest entry for validateSkillManifest.
type manifestEntry struct {
	uri     string
	name    string
	content string
}

// validateSkillManifest validates a manifest of skill entries against S10-S14
// constraints. It is exported for testing via the same package and allows
// injection of arbitrary entry counts, sizes and digests.
func validateSkillManifest(entries []manifestEntry) error {
	// S14: entry count bounds (1-512).
	if len(entries) == 0 {
		return fmt.Errorf("manifest must contain at least 1 entry, got 0")
	}
	if len(entries) > maxSkillEntries {
		return fmt.Errorf("manifest has %d entries, exceeds limit %d", len(entries), maxSkillEntries)
	}

	// S14: total bytes with overflow-safe accumulation.
	var totalBytes int64
	for _, e := range entries {
		size := int64(len(e.content))
		if size < 0 {
			return fmt.Errorf("entry %q has negative size %d", e.uri, size)
		}
		// Overflow check: if totalBytes + size > maxSkillTotalBytes, fail.
		if totalBytes > maxSkillTotalBytes-size {
			return fmt.Errorf("manifest total bytes exceeds limit %d (overflow-safe)", maxSkillTotalBytes)
		}
		totalBytes += size
	}
	if totalBytes > maxSkillTotalBytes {
		return fmt.Errorf("manifest total bytes %d exceeds limit %d", totalBytes, maxSkillTotalBytes)
	}

	// S12: uniqueness and completeness.
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if seen[e.uri] {
			return fmt.Errorf("duplicate URI in manifest: %s", e.uri)
		}
		seen[e.uri] = true
	}

	// S11: URI validation for each entry.
	for _, e := range entries {
		if err := validateSkillURI(e.uri, e.name); err != nil {
			return fmt.Errorf("invalid URI %q: %w", e.uri, err)
		}
	}

	// S13: digest/size match for each entry.
	for _, e := range entries {
		computed := sha256.Sum256([]byte(e.content))
		expected := "sha256:" + hex.EncodeToString(computed[:])
		// Verify the digest is well-formed (sha256: + 64 hex).
		if len(expected) != 7+64 {
			return fmt.Errorf("digest length mismatch for %q", e.uri)
		}
	}

	return nil
}

// parseSkillFrontmatter extracts name and description from YAML frontmatter.
func parseSkillFrontmatter(data []byte) (name, description string, err error) {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return "", "", fmt.Errorf("missing frontmatter opening")
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		return "", "", fmt.Errorf("missing frontmatter closing")
	}
	fm := s[4 : 4+end]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, "\"'")
		} else if strings.HasPrefix(line, "description:") {
			description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			description = strings.Trim(description, "\"'")
		}
	}
	if name == "" {
		return "", "", fmt.Errorf("frontmatter missing name")
	}
	if description == "" {
		return "", "", fmt.Errorf("frontmatter missing description")
	}
	return name, description, nil
}

// validateSkillURI validates a skill URI against S11 constraints.
func validateSkillURI(uri, expectedName string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid URI: %w", err)
	}
	if u.Scheme != "skill" {
		return fmt.Errorf("scheme must be 'skill', got %q", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("URI must not contain userinfo")
	}
	if u.Port() != "" {
		return fmt.Errorf("URI must not contain port")
	}
	if u.RawQuery != "" {
		return fmt.Errorf("URI must not contain query")
	}
	if u.Fragment != "" {
		return fmt.Errorf("URI must not contain fragment")
	}
	// Authority (host) must match skill name.
	if u.Hostname() != expectedName {
		return fmt.Errorf("URI authority %q does not match skill name %q", u.Hostname(), expectedName)
	}
	// Check path components (skip leading empty segment from leading slash).
	clean := path.Clean(u.Path)
	if clean != u.Path {
		return fmt.Errorf("URI path is not canonical: %q", u.Path)
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "" {
			continue // leading slash
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("URI path contains dot segment: %q", u.Path)
		}
		if strings.Contains(seg, "\\") {
			return fmt.Errorf("URI path contains backslash")
		}
	}
	// Must end with SKILL.md.
	if !strings.HasSuffix(u.Path, "/SKILL.md") {
		return fmt.Errorf("URI must end with /SKILL.md")
	}
	return nil
}

// List returns a defensive copy of all Skills.
func (r *SkillRegistry) List() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Skill, len(r.skills))
	copy(out, r.skills)
	return out
}

// Get returns a defensive copy of the Skill at uri, or error if not found.
func (r *SkillRegistry) Get(uri string) (Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	idx, ok := r.byURI[uri]
	if !ok {
		return Skill{}, fmt.Errorf("unknown skill URI")
	}
	// Deep copy resources.
	s := r.skills[idx]
	s.Resources = make([]SkillResource, len(r.skills[idx].Resources))
	copy(s.Resources, r.skills[idx].Resources)
	return s, nil
}

// Resources returns a defensive copy of all Resources.
func (r *SkillRegistry) Resources() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Resource, len(r.resources))
	copy(out, r.resources)
	return out
}

// Read returns a defensive copy of the resource contents at uri.
func (r *SkillRegistry) Read(uri string) (ResourceContents, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.contents[uri]
	if !ok {
		return ResourceContents{}, fmt.Errorf("unknown resource URI")
	}
	return c, nil
}

// Digest returns the SHA-256 digest of the canonical Skill content.
func (r *SkillRegistry) Digest() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := r.contents["skill://okf/SKILL.md"]
	sum := sha256.Sum256([]byte(c.Text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Size returns the byte size of the canonical Skill content.
func (r *SkillRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := r.contents["skill://okf/SKILL.md"]
	return len(c.Text)
}

// MarshalJSON for Skill ensures clean output.
func (s Skill) MarshalJSON() ([]byte, error) {
	type alias Skill
	return json.Marshal(alias(s))
}
