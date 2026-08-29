package okf

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// v0.2 new types: provenance, trust, lifecycle, attested computation
// ---------------------------------------------------------------------------

// Source records a material a concept derives from (spec §5.1).
type Source struct {
	// ID is an optional stable key used for per-claim attribution via markdown footnotes.
	ID string `yaml:"id,omitempty" json:"id,omitempty"`
	// Resource is REQUIRED within an entry. Names a concrete artifact (URL, bundle-relative path,
	// references/ path) or a scope descriptor (e.g. "all queries in project X").
	Resource string `yaml:"resource" json:"resource"`
	// Title is an optional human-readable label for the source.
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	// Author is who produced the source, in actor convention (spec §7).
	Author string `yaml:"author,omitempty" json:"author,omitempty"`
	// UsageCount is how often resource was exercised over usage_window. A liveness signal.
	UsageCount int `yaml:"usage_count,omitempty" json:"usage_count,omitempty"`
	// LastModified is when the source itself last changed (YYYY-MM-DD). A recency signal.
	LastModified string `yaml:"last_modified,omitempty" json:"last_modified,omitempty"`
}

// UsageWindow frames every usage_count with a {from, to} date range (spec §5.1).
type UsageWindow struct {
	From string `yaml:"from" json:"from"` // YYYY-MM-DD
	To   string `yaml:"to" json:"to"`     // YYYY-MM-DD
}

// GeneratedInfo records how the current content was produced (spec §5.2).
type GeneratedInfo struct {
	// By is REQUIRED within generated. An actor (spec §7).
	By string `yaml:"by" json:"by"`
	// At is an ISO 8601 datetime marking the content's last meaningful change.
	At string `yaml:"at,omitempty" json:"at,omitempty"`
}

// VerificationEvent records who or what confirmed the content (spec §5.2).
type VerificationEvent struct {
	// By is an actor (spec §7).
	By string `yaml:"by" json:"by"`
	// At is an ISO 8601 datetime.
	At string `yaml:"at,omitempty" json:"at,omitempty"`
}

// TrustTier is derived from a concept's verified field (spec §5.3).
type TrustTier int

const (
	// TrustUnverified: no verified key.
	TrustUnverified TrustTier = iota
	// TrustMachineConfirmed: verified by non-human: actors only.
	TrustMachineConfirmed
	// TrustHumanReviewed: verified by a human:<id> actor.
	TrustHumanReviewed
)

// String returns the human-readable trust tier name.
func (t TrustTier) String() string {
	switch t {
	case TrustUnverified:
		return "unverified"
	case TrustMachineConfirmed:
		return "machine-confirmed"
	case TrustHumanReviewed:
		return "human-reviewed"
	default:
		return "unknown"
	}
}

// ConceptStatus is the lifecycle status (spec §5.4).
type ConceptStatus string

const (
	StatusDraft      ConceptStatus = "draft"
	StatusStable     ConceptStatus = "stable"
	StatusDeprecated ConceptStatus = "deprecated"
)

// Parameter is a typed, named hole an agent may fill in an Attested Computation (spec §10.2).
type Parameter struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

// ExecutorRef describes how a computation is run (spec §10.2).
type ExecutorRef struct {
	// Resource names run instructions or code.
	Resource string `yaml:"resource" json:"resource"`
	// Receipt declares the fields a run must return (e.g. [job_id, executed_sql, result]).
	Receipt []string `yaml:"receipt,omitempty" json:"receipt,omitempty"`
}

// AttesterRef describes the deterministic check (spec §10.2).
type AttesterRef struct {
	// Resource names code (no LLM) that takes a receipt and returns a verdict.
	Resource string `yaml:"resource" json:"resource"`
}

// ---------------------------------------------------------------------------
// Concept
// ---------------------------------------------------------------------------

// Concept represents a single unit of knowledge within a bundle.
// It corresponds to one markdown file with YAML frontmatter.
//
// v0.2: only Type is required (spec §4.1). All other fields are optional.
type Concept struct {
	// --- v0.1 fields (carried forward) ---

	// Type is the category of the concept. REQUIRED per OKF spec §4.1.
	Type string `yaml:"type" json:"type"`
	// Title is a human-readable name. v0.2: optional (spec §4.1 recommended).
	// If omitted, consumers may derive from filename.
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	// Description provides a brief summary. Optional but recommended.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Resource references the actual resource this knowledge describes. Optional.
	Resource string `yaml:"resource,omitempty" json:"resource,omitempty"`
	// Tags are arbitrary labels for categorization and discovery.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	// Timestamp is LEGACY v0.1 field. Superseded by Generated.At (spec §13.1).
	// Retained for v0.1 backward compatibility; new code should use Generated.
	Timestamp string `yaml:"timestamp,omitempty" json:"timestamp,omitempty"`

	// --- v0.2 new fields ---

	// Sources records the materials a concept derives from (spec §5.1).
	Sources []Source `yaml:"sources,omitempty" json:"sources,omitempty"`
	// UsageWindow frames every usage_count with a date range (spec §5.1).
	UsageWindow *UsageWindow `yaml:"usage_window,omitempty" json:"usage_window,omitempty"`
	// Generated records how the current content was produced (spec §5.2).
	Generated *GeneratedInfo `yaml:"generated,omitempty" json:"generated,omitempty"`
	// Verified records verification events (spec §5.2). May be a bare mapping in YAML,
	// which NormalizeVerified() converts to a one-element list.
	Verified []VerificationEvent `yaml:"verified,omitempty" json:"verified,omitempty"`
	// Status is the lifecycle status: draft|stable|deprecated. Absent => stable (spec §5.4).
	Status ConceptStatus `yaml:"status,omitempty" json:"status,omitempty"`
	// StaleAfter is an absolute date (YYYY-MM-DD). Concept is stale when today >= stale_after (spec §5.5).
	StaleAfter string `yaml:"stale_after,omitempty" json:"stale_after,omitempty"`

	// --- Attested Computation fields (spec §10) ---

	// Runtime says how to run the computation. REQUIRED for type "Attested Computation".
	Runtime string `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	// Parameters is the list of typed, named holes the agent may fill.
	Parameters []Parameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	// Computation is a path to a file holding the computation (alternative to inline body fence).
	Computation string `yaml:"computation,omitempty" json:"computation,omitempty"`
	// Executor describes how the computation is run.
	Executor *ExecutorRef `yaml:"executor,omitempty" json:"executor,omitempty"`
	// Attester describes the deterministic check.
	Attester *AttesterRef `yaml:"attester,omitempty" json:"attester,omitempty"`

	// --- internal fields ---

	// Content is the markdown body of the document.
	Content string `yaml:"-" json:"-"`
	// FilePath is the relative path to the source file.
	FilePath string `yaml:"-" json:"filePath,omitempty"`
	// CustomFields holds additional fields not defined in the OKF spec.
	CustomFields map[string]interface{} `yaml:",inline" json:"-"`
}

// TrustTier derives the trust tier from the verified field (spec §5.3).
//   - no verified or empty list => TrustUnverified
//   - only non-human: actors => TrustMachineConfirmed
//   - at least one human:<id> actor => TrustHumanReviewed
func (c *Concept) TrustTier() TrustTier {
	if len(c.Verified) == 0 {
		return TrustUnverified
	}
	for _, v := range c.Verified {
		if strings.HasPrefix(v.By, "human:") {
			return TrustHumanReviewed
		}
	}
	return TrustMachineConfirmed
}

// IsStale reports whether the concept is stale as of referenceTime (spec §5.5).
// A concept is stale when referenceTime >= stale_after.
// Empty or invalid stale_after => false (never stale).
func (c *Concept) IsStale(referenceTime time.Time) bool {
	if c.StaleAfter == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", c.StaleAfter)
	if err != nil {
		return false
	}
	// Compare at day granularity: referenceTime >= stale_after date.
	refDay := time.Date(referenceTime.Year(), referenceTime.Month(), referenceTime.Day(), 0, 0, 0, 0, time.UTC)
	return !refDay.Before(t)
}

// IsAttestedComputation reports whether this concept is an Attested Computation (spec §10.1).
func (c *Concept) IsAttestedComputation() bool {
	return c.Type == "Attested Computation"
}

// GetComputationBody extracts the first fenced code block under the "# Computation" heading
// from the markdown body (spec §4.2, §10.3). Returns (code, true) if found, ("", false) otherwise.
//
// If the Computation field (file path) is set, the caller should resolve the file separately;
// this method only handles inline body extraction.
func (c *Concept) GetComputationBody() (string, bool) {
	if c.Content == "" {
		return "", false
	}
	lines := strings.Split(c.Content, "\n")
	// Find the "# Computation" heading
	inComputation := false
	codeLines := []string{}
	inCodeBlock := false
	codeFence := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inComputation {
			// Match "# Computation" heading (case-insensitive, allow leading whitespace)
			if strings.HasPrefix(strings.ToLower(trimmed), "# computation") {
				inComputation = true
			}
			continue
		}
		// We're in the Computation section. Stop at next heading of same or higher level.
		if inCodeBlock {
			// Check for closing fence
			if strings.HasPrefix(trimmed, codeFence) {
				inCodeBlock = false
				break // Found the first code block, done
			}
			codeLines = append(codeLines, line)
			continue
		}
		// Check for opening fence
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = true
			codeFence = "```"
			continue
		}
		// Stop if we hit another heading (any level) before finding a code block
		if strings.HasPrefix(trimmed, "#") {
			break
		}
	}
	if len(codeLines) == 0 {
		return "", false
	}
	return strings.Join(codeLines, "\n"), true
}

// NormalizeVerified ensures Verified is a proper list. In YAML, verified may be written as
// a bare mapping {by, at} which should be treated as a one-element list (spec §5.2).
// This method is a no-op if Verified is already a list; callers that parse via yaml.Node
// should normalize before setting the field.
func (c *Concept) NormalizeVerified() {
	// No-op for already-normalized slice. The actual normalization happens in parser
	// where yaml.Node is used to detect mapping vs sequence.
}

// EffectiveStatus returns the concept status, defaulting to stable if empty (spec §5.4).
func (c *Concept) EffectiveStatus() ConceptStatus {
	if c.Status == "" {
		return StatusStable
	}
	return c.Status
}

// ---------------------------------------------------------------------------
// KnowledgeBundle
// ---------------------------------------------------------------------------

// KnowledgeBundle represents a self-contained collection of knowledge documents.
// It is the unit of distribution in OKF - typically a directory structure.
type KnowledgeBundle struct {
	// Name is a short identifier for the bundle.
	Name string `yaml:"name" json:"name"`
	// Description provides an overview of what this bundle contains.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Version is the bundle format version (e.g., "1.0", "2.1").
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	// OKFVersion is the OKF spec version declared in bundle-root index.md (spec §12).
	// E.g. "0.2". Empty if not declared.
	OKFVersion string `yaml:"okf_version,omitempty" json:"okf_version,omitempty"`
	// Owner identifies who or what is responsible for this bundle.
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty"`
	// Concepts is the list of all concepts in this bundle.
	Concepts []*Concept `yaml:"concepts,omitempty" json:"concepts,omitempty"`
	// RootPath is the filesystem path to the bundle root.
	RootPath string `yaml:"-" json:"rootPath,omitempty"`
	// mu guards concurrent access to Concepts (bundle methods are safe for
	// concurrent use; direct field mutation by callers is not).
	mu sync.RWMutex
}

// LoadOptions contains configuration for loading bundles.
type LoadOptions struct {
	// Recursive controls whether subdirectories are scanned for concepts.
	Recursive bool
	// FilterFunc optionally filters which files are loaded.
	FilterFunc func(path string, info os.FileInfo) bool
}

// SaveOptions contains configuration for saving bundles.
type SaveOptions struct {
	// PrettyPrint controls whether YAML is formatted with indentation.
	PrettyPrint bool
	// LegacyTimestamp controls whether the legacy v0.1 'timestamp' field is output
	// alongside 'generated.at' for v0.1 consumer compatibility. Default false (v0.2 only).
	LegacyTimestamp bool
}

// DefaultLoadOptions returns the recommended default load configuration.
func DefaultLoadOptions() *LoadOptions {
	return &LoadOptions{
		Recursive: true,
	}
}

// DefaultSaveOptions returns the recommended default save configuration.
func DefaultSaveOptions() *SaveOptions {
	return &SaveOptions{
		PrettyPrint: true,
	}
}

// NewConcept creates a new concept with required fields set.
// v0.2: only Type is required; Title is optional but set here for convenience.
func NewConcept(conceptType, title string) *Concept {
	return &Concept{
		Type:      conceptType,
		Title:     title,
		Generated: &GeneratedInfo{By: "unknown", At: time.Now().UTC().Format(time.RFC3339)},
		Status:    StatusStable,
		Tags:      []string{},
	}
}

// NewBundle creates a new knowledge bundle.
func NewBundle(name string) *KnowledgeBundle {
	return &KnowledgeBundle{
		Name:       name,
		Version:    "1.0",
		OKFVersion: "0.2",
		Concepts:   []*Concept{},
	}
}

// AddConcept adds a concept to the bundle and returns the concept.
func (b *KnowledgeBundle) AddConcept(c *Concept) *Concept {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c.FilePath == "" {
		filename := sanitizeFilename(c.Title) + ".md"
		if c.Type != "" {
			c.FilePath = filepath.Join(c.Type+"s", filename)
		} else {
			c.FilePath = filepath.Join("concepts", filename)
		}
	}
	b.Concepts = append(b.Concepts, c)
	return c
}

// RemoveConcept removes a concept from the bundle by title.
// Returns true if the concept was found and removed.
func (b *KnowledgeBundle) RemoveConcept(title string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, c := range b.Concepts {
		if c == nil {
			continue
		}
		if c.Title == title {
			b.Concepts = append(b.Concepts[:i], b.Concepts[i+1:]...)
			return true
		}
	}
	return false
}

// GetConcept returns a concept by title, or nil if not found.
func (b *KnowledgeBundle) GetConcept(title string) *Concept {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, c := range b.Concepts {
		if c == nil {
			continue
		}
		if c.Title == title {
			return c
		}
	}
	return nil
}

// FilterConcepts returns all concepts matching the given predicate.
func (b *KnowledgeBundle) FilterConcepts(pred func(*Concept) bool) []*Concept {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var result []*Concept
	for _, c := range b.Concepts {
		if c == nil {
			continue
		}
		if pred(c) {
			result = append(result, c)
		}
	}
	return result
}

// FilterByType returns all concepts of the given type.
func (b *KnowledgeBundle) FilterByType(conceptType string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return c.Type == conceptType
	})
}

// FilterByTag returns all concepts containing the given tag.
func (b *KnowledgeBundle) FilterByTag(tag string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		for _, t := range c.Tags {
			if t == tag {
				return true
			}
		}
		return false
	})
}

// FilterByResource returns all concepts referencing the given resource.
func (b *KnowledgeBundle) FilterByResource(resource string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return c.Resource == resource
	})
}

// FilterByTrustTier returns all concepts with the given trust tier (spec §5.3).
func (b *KnowledgeBundle) FilterByTrustTier(tier TrustTier) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return c.TrustTier() == tier
	})
}

// FilterByTrustTiers returns all concepts matching any of the given trust tiers.
func (b *KnowledgeBundle) FilterByTrustTiers(tiers []TrustTier) []*Concept {
	if len(tiers) == 0 {
		return b.Concepts
	}
	tierSet := make(map[TrustTier]bool, len(tiers))
	for _, t := range tiers {
		tierSet[t] = true
	}
	return b.FilterConcepts(func(c *Concept) bool {
		return tierSet[c.TrustTier()]
	})
}

// FilterByStatus returns all concepts with the given status (spec §5.4).
// Concepts with empty status are treated as stable.
func (b *KnowledgeBundle) FilterByStatus(status ConceptStatus) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return c.EffectiveStatus() == status
	})
}

// FilterByStatuses returns all concepts matching any of the given statuses.
func (b *KnowledgeBundle) FilterByStatuses(statuses []ConceptStatus) []*Concept {
	if len(statuses) == 0 {
		return b.Concepts
	}
	statusSet := make(map[ConceptStatus]bool, len(statuses))
	for _, s := range statuses {
		statusSet[s] = true
	}
	return b.FilterConcepts(func(c *Concept) bool {
		return statusSet[c.EffectiveStatus()]
	})
}

// FilterFresh returns all concepts that are NOT stale as of referenceTime (spec §5.5).
func (b *KnowledgeBundle) FilterFresh(referenceTime time.Time) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		return !c.IsStale(referenceTime)
	})
}

// FilterBySource returns all concepts whose sources contain the given id or resource.
func (b *KnowledgeBundle) FilterBySource(sourceIDOrResource string) []*Concept {
	return b.FilterConcepts(func(c *Concept) bool {
		for _, s := range c.Sources {
			if s.ID == sourceIDOrResource || s.Resource == sourceIDOrResource {
				return true
			}
		}
		return false
	})
}

// Search performs a full-text search across concept titles, descriptions, content,
// and sources[].title / sources[].author. Returns matching concepts.
func (b *KnowledgeBundle) Search(query string) []*Concept {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	return b.FilterConcepts(func(c *Concept) bool {
		if c == nil {
			return false
		}
		if containsFold(c.Title, query) ||
			containsFold(c.Description, query) ||
			containsFold(c.Content, query) {
			return true
		}
		for _, s := range c.Sources {
			if containsFold(s.Title, query) || containsFold(s.Author, query) {
				return true
			}
		}
		return false
	})
}

// Stats returns statistics about the bundle, including v0.2 trust tier and status counts.
func (b *KnowledgeBundle) Stats() BundleStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	stats := BundleStats{
		TotalConcepts:   len(b.Concepts),
		TypeCounts:      make(map[string]int),
		TagCounts:       make(map[string]int),
		TrustTierCounts: make(map[TrustTier]int),
		StatusCounts:    make(map[ConceptStatus]int),
	}
	typeSet := make(map[string]struct{})
	tagSet := make(map[string]struct{})
	now := time.Now()
	for _, c := range b.Concepts {
		if c == nil {
			continue
		}
		if c.Type != "" {
			typeSet[c.Type] = struct{}{}
			stats.TypeCounts[c.Type]++
		}
		for _, tag := range c.Tags {
			tagSet[tag] = struct{}{}
			stats.TagCounts[tag]++
		}
		// v0.2 stats
		tier := c.TrustTier()
		stats.TrustTierCounts[tier]++
		status := c.EffectiveStatus()
		stats.StatusCounts[status]++
		if c.IsStale(now) {
			stats.StaleCount++
		}
		if c.IsAttestedComputation() {
			stats.AttestedComputationCount++
		}
	}
	stats.UniqueTypes = len(typeSet)
	stats.UniqueTags = len(tagSet)
	return stats
}

// BundleStats contains aggregate information about a bundle, including v0.2 dimensions.
type BundleStats struct {
	TotalConcepts int            `json:"totalConcepts"`
	UniqueTypes   int            `json:"uniqueTypes"`
	UniqueTags    int            `json:"uniqueTags"`
	TypeCounts    map[string]int `json:"typeCounts"`
	TagCounts     map[string]int `json:"tagCounts"`
	// v0.2 additions
	TrustTierCounts          map[TrustTier]int     `json:"trustTierCounts"`
	StatusCounts             map[ConceptStatus]int `json:"statusCounts"`
	StaleCount               int                   `json:"staleCount"`
	AttestedComputationCount int                   `json:"attestedComputationCount"`
}

// RelatedConcepts finds concepts that share tags or resources with the given concept.
func (b *KnowledgeBundle) RelatedConcepts(c *Concept) []*Concept {
	if c == nil {
		return nil
	}
	tagSet := make(map[string]bool)
	for _, t := range c.Tags {
		tagSet[t] = true
	}
	return b.FilterConcepts(func(other *Concept) bool {
		if other == c {
			return false
		}
		for _, t := range other.Tags {
			if tagSet[t] {
				return true
			}
		}
		if c.Resource != "" && other.Resource == c.Resource {
			return true
		}
		return false
	})
}

// === v0.2: index.md / log.md support (spec §3.1, §7) ===

// IndexEntry represents a single entry in an index.md directory listing.
type IndexEntry struct {
	Path    string `yaml:"path"`
	Title   string `yaml:"title,omitempty"`
	Type    string `yaml:"type,omitempty"`
	Summary string `yaml:"summary,omitempty"`
}

// IndexFile represents a directory index document (index.md, spec §3.1).
// At bundle root, it may include okf_version frontmatter.
type IndexFile struct {
	OKFVersion string       `yaml:"okf_version,omitempty"`
	Title      string       `yaml:"title,omitempty"`
	Entries    []IndexEntry `yaml:"entries,omitempty"`
	Content    string       `yaml:"-"`
	FilePath   string       `yaml:"-"`
}

// LogEntry represents a single update history entry in log.md (spec §7).
type LogEntry struct {
	Date    string `yaml:"date"`
	Action  string `yaml:"action"`
	Target  string `yaml:"target,omitempty"`
	By      string `yaml:"by,omitempty"`
	Details string `yaml:"details,omitempty"`
}

// LogFile represents an update history document (log.md, spec §7).
type LogFile struct {
	Title    string     `yaml:"title,omitempty"`
	Entries  []LogEntry `yaml:"entries,omitempty"`
	Content  string     `yaml:"-"`
	FilePath string     `yaml:"-"`
}

// IsIndexFile reports whether path's base name is index.md.
func IsIndexFile(path string) bool {
	return strings.ToLower(filepath.Base(path)) == "index.md"
}

// IsLogFile reports whether path's base name is log.md.
func IsLogFile(path string) bool {
	return strings.ToLower(filepath.Base(path)) == "log.md"
}
