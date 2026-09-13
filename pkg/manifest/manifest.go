package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/vectorindex"
)

// defaultLimit is used when the request omits limit (nil pointer).
const defaultLimit = 100

// maxLimit is the inclusive upper bound on an explicitly provided limit.
const maxLimit = 500

// maxSourceResources caps the per-item source resource strings surfaced.
const maxSourceResources = 3

// IndexStatus observes the vector index from its metadata only. It never loads
// embeddings or HNSW and never writes the index (S24).
type IndexStatus string

const (
	IndexMissing      IndexStatus = "missing"
	IndexReady        IndexStatus = "ready"
	IndexStale        IndexStatus = "stale"
	IndexIncompatible IndexStatus = "incompatible"
	IndexUnreadable   IndexStatus = "unreadable"
)

// currentIndexFormatVersion is read from vectorindex.FormatVersion so there is a
// single source of truth. The accessor is a pure constant function that never
// triggers HNSW/embedding runtime initialization; a mismatch is reported as
// "incompatible".
func currentIndexFormatVersion() int { return vectorindex.FormatVersion() }

// ManifestRequest is the metadata-only discovery request (design §4.2). Limit
// is a pointer so omitted (nil → 100) is distinguishable from an explicit
// out-of-range value (which fails closed rather than being clamped).
type ManifestRequest struct {
	Offset       int      `json:"offset,omitempty"`
	Limit        *int     `json:"limit,omitempty"`
	Types        []string `json:"types,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Statuses     []string `json:"statuses,omitempty"`
	Stale        *bool    `json:"stale,omitempty"`
	FolderPrefix string   `json:"folder_prefix,omitempty"`
	IncludeTrace bool     `json:"include_trace,omitempty"`
}

// ManifestItem is one concept's bounded metadata. It NEVER carries Markdown
// body content (S20).
type ManifestItem struct {
	OKFID           string   `json:"okf_id,omitempty"`
	Ref             string   `json:"ref,omitempty"`
	IdentityState   string   `json:"identity_state"`
	Path            string   `json:"path"`
	Title           string   `json:"title,omitempty"`
	Description     string   `json:"description,omitempty"`
	Type            string   `json:"type,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Status          string   `json:"status"`
	TrustTier       string   `json:"trust_tier"`
	Stale           bool     `json:"stale"`
	StaleAfter      string   `json:"stale_after,omitempty"`
	GeneratedAt     string   `json:"generated_at,omitempty"`
	VerifiedAt      string   `json:"verified_at,omitempty"`
	SourceCount     int      `json:"source_count"`
	SourceResources []string `json:"source_resources,omitempty"`
	FileSizeBytes   int64    `json:"file_size_bytes"`
	EstimatedTokens int64    `json:"estimated_tokens"`
	EstimateKind    string   `json:"estimate_kind"`
}

// ManifestWarning records one omitted file and its stable warning code.
type ManifestWarning struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

// ManifestTraceStep is a minimal deterministic trace entry when IncludeTrace is set.
type ManifestTraceStep struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Count   int    `json:"count,omitempty"`
}

// ManifestResult is the metadata-only listing result.
type ManifestResult struct {
	Total       int                 `json:"total"`
	Offset      int                 `json:"offset"`
	Limit       int                 `json:"limit"`
	Items       []ManifestItem      `json:"items"`
	IndexStatus IndexStatus         `json:"index_status"`
	Warnings    []ManifestWarning   `json:"warnings,omitempty"`
	Trace       []ManifestTraceStep `json:"trace,omitempty"`
}

// Error is the typed manifest error carrying a stable code (e.g. invalid_request,
// duplicate_concept_id, invalid_concept_id).
type Error struct {
	Code    string
	Message string
	Paths   []string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Is matches any *Error carrying the same Code.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other != nil && e != nil && other.Code == e.Code
}

var ErrInvalidRequest = &Error{Code: "invalid_request", Message: "invalid manifest request"}

// Build scans the knowledge root under a strict frontmatter budget, applies
// deterministic filters and pagination, and returns a metadata-only listing.
// It never reads Markdown bodies, never loads embeddings/HNSW, and never writes
// the index. vectorDir is observed read-only for index_status; indexStale
// upgrades an otherwise-ready index to "stale" (it is supplied by the caller
// from git freshness); now drives staleness (injectable for deterministic tests).
func Build(ctx context.Context, root, vectorDir string, req ManifestRequest, fr FileReader, now time.Time, indexStale bool) (*ManifestResult, error) {
	if root == "" {
		return nil, wrapInvalidRequest("knowledge root is required")
	}
	if req.Offset < 0 {
		return nil, wrapInvalidRequest("offset must be non-negative")
	}
	limit := defaultLimit
	if req.Limit != nil {
		if *req.Limit < 1 || *req.Limit > maxLimit {
			return nil, wrapInvalidRequest("limit must be between 1 and 500 when explicitly provided")
		}
		limit = *req.Limit
	}
	folderPrefix, err := normalizeFolderPrefix(req.FolderPrefix)
	if err != nil {
		return nil, err
	}

	reports, err := ScanKnowledgeFiles(ctx, root, fr)
	if err != nil {
		return nil, err
	}

	warnings := collectWarnings(reports)

	// Identity closure: validate explicit IDs and reject duplicates over the WHOLE
	// scanned set (before filtering), so refs can never be ambiguous (S25/S03).
	if err := validateIdentityRegistry(reports); err != nil {
		return nil, err
	}

	items := make([]ManifestItem, 0, len(reports))
	for _, r := range reports {
		if r.Meta == nil {
			continue
		}
		item, ok := toItem(r, now)
		if !ok {
			continue
		}
		if matchesFilters(item, r, req, folderPrefix) {
			items = append(items, item)
		}
	}

	// Deterministic order: path ASC, then okf_id ASC.
	slices.SortFunc(items, func(a, b ManifestItem) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(a.OKFID, b.OKFID)
	})

	total := len(items)
	start := req.Offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	var page []ManifestItem
	if start < end {
		page = items[start:end]
	}
	if page == nil {
		page = []ManifestItem{}
	}

	result := &ManifestResult{
		Total:       total,
		Offset:      req.Offset,
		Limit:       limit,
		Items:       page,
		IndexStatus: ObserveIndexStatus(vectorDir, indexStale),
		Warnings:    warnings,
	}
	if req.IncludeTrace {
		result.Trace = []ManifestTraceStep{
			{Type: "scan", Message: "bounded frontmatter scan", Count: len(reports)},
			{Type: "filter", Message: "applied manifest filters", Count: total},
			{Type: "pagination", Message: "applied offset/limit", Count: len(page)},
		}
	}
	return result, nil
}

// ObserveIndexStatus reads only the vector index metadata file and Stat()s the
// binary; it never loads embeddings or HNSW. The stale flag is supplied by the
// caller (from git freshness) and only upgrades an otherwise-ready index.
func ObserveIndexStatus(vectorDir string, stale bool) IndexStatus {
	metaPath := filepath.Join(vectorDir, "index.meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return IndexMissing
		}
		return IndexUnreadable
	}
	var meta struct {
		IndexFormatVersion int `json:"index_format_version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return IndexUnreadable
	}
	if meta.IndexFormatVersion != currentIndexFormatVersion() {
		return IndexIncompatible
	}
	if _, err := os.Stat(filepath.Join(vectorDir, "index.bin")); err != nil {
		return IndexUnreadable
	}
	if stale {
		return IndexStale
	}
	return IndexReady
}

func collectWarnings(reports []FileReport) []ManifestWarning {
	var out []ManifestWarning
	for _, r := range reports {
		if r.Warning != "" {
			out = append(out, ManifestWarning{Code: r.Warning, Path: r.Path})
		}
	}
	return out
}

// validateIdentityRegistry reuses pkg/identity.BuildRegistry over lightweight
// concept projections so invalid and duplicate explicit IDs fail closed.
func validateIdentityRegistry(reports []FileReport) error {
	concepts := make([]*okf.Concept, 0, len(reports))
	for _, r := range reports {
		if r.Meta == nil || r.Meta.OKFID == "" {
			continue
		}
		concepts = append(concepts, &okf.Concept{
			FilePath:     r.Path,
			CustomFields: map[string]any{identity.Field: r.Meta.OKFID},
		})
	}
	if _, err := identity.BuildRegistry(concepts); err != nil {
		var identErr *identity.Error
		if errors.As(err, &identErr) {
			return &Error{Code: string(identErr.Code), Message: identErr.Error(), Paths: identErr.Paths}
		}
		return &Error{Code: "internal_error", Message: err.Error()}
	}
	return nil
}

func toItem(r FileReport, now time.Time) (ManifestItem, bool) {
	m := r.Meta
	item := ManifestItem{
		Path:            r.Path,
		Title:           m.Title,
		Description:     m.Description,
		Type:            m.Type,
		Tags:            cloneStrings(m.Tags),
		Status:          effectiveStatus(m.Status),
		TrustTier:       trustTier(m.Verified),
		StaleAfter:      m.StaleAfter,
		Stale:           isStale(m.StaleAfter, now),
		SourceCount:     len(m.Sources),
		FileSizeBytes:   r.Size,
		EstimatedTokens: (r.Size + 3) / 4,
		EstimateKind:    "file_bytes_div4",
	}
	if m.Generated != nil {
		item.GeneratedAt = m.Generated.At
	}
	if at := latestVerifiedAt(m.Verified); at != "" {
		item.VerifiedAt = at
	}
	item.SourceResources = sourceResources(m.Sources)

	switch _, err := identity.Parse(m.OKFID); {
	case m.OKFID == "":
		item.IdentityState = string(identity.StateLegacyUnstable)
	case err == nil:
		item.OKFID = m.OKFID
		item.Ref = identity.CanonicalURI(m.OKFID)
		item.IdentityState = string(identity.StateStable)
	default:
		item.IdentityState = string(identity.StateInvalid)
	}
	return item, true
}

func sourceResources(sources []SourceRef) []string {
	var out []string
	for _, s := range sources {
		if s.Resource == "" {
			continue
		}
		out = append(out, s.Resource)
		if len(out) >= maxSourceResources {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func effectiveStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return string(okf.StatusStable)
	}
	return status
}

func trustTier(verified []VerifiedMeta) string {
	if len(verified) == 0 {
		return "unverified"
	}
	for _, v := range verified {
		if strings.HasPrefix(v.By, "human:") {
			return "human-reviewed"
		}
	}
	return "machine-confirmed"
}

func isStale(staleAfter string, now time.Time) bool {
	if staleAfter == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", staleAfter)
	if err != nil {
		return false
	}
	refDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return !refDay.Before(t)
}

// latestVerifiedAt returns the most recent parseable verified timestamp, or "".
func latestVerifiedAt(verified []VerifiedMeta) string {
	var latest time.Time
	found := false
	for _, v := range verified {
		t, ok := parseEventTime(v.At)
		if !ok {
			continue
		}
		if !found || t.After(latest) {
			latest = t
			found = true
		}
	}
	if !found {
		return ""
	}
	return latest.Format(time.RFC3339)
}

func parseEventTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// matchesFilters applies AND across dimensions and OR within each dimension.
// Empty lists match all.
func matchesFilters(item ManifestItem, r FileReport, req ManifestRequest, folderPrefix string) bool {
	if len(req.Types) > 0 && !slices.Contains(req.Types, item.Type) {
		return false
	}
	if len(req.Statuses) > 0 && !slices.Contains(req.Statuses, item.Status) {
		return false
	}
	if len(req.Tags) > 0 && !tagIntersects(item.Tags, req.Tags) {
		return false
	}
	if req.Stale != nil && item.Stale != *req.Stale {
		return false
	}
	if folderPrefix != "" && !strings.HasPrefix(item.Path, folderPrefix) {
		return false
	}
	return true
}

func tagIntersects(itemTags, wanted []string) bool {
	for _, w := range wanted {
		if slices.Contains(itemTags, w) {
			return true
		}
	}
	return false
}

// normalizeFolderPrefix enforces the portable, bundle-relative prefix contract:
// absolute paths and ".." escapes are rejected.
func normalizeFolderPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", nil
	}
	clean := filepath.ToSlash(filepath.Clean(prefix))
	if filepath.IsAbs(prefix) || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", wrapInvalidRequest("folder_prefix must be a bundle-relative path without .. escapes")
	}
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." {
		return "", nil
	}
	if !strings.HasSuffix(clean, "/") {
		clean += "/"
	}
	return clean, nil
}

func wrapInvalidRequest(message string) error {
	return &Error{Code: ErrInvalidRequest.Code, Message: message}
}
