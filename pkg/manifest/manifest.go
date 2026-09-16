package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/memorymeta"
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

	// Governed-agent-memory extension (add-governed-agent-memory). All new
	// fields are optional; omitting them preserves the pre-change shape and
	// order byte-for-byte.
	//
	// ForPath lexically matches concept code_refs patterns (no FS access,
	// S10-S15). Governance filters by effective governance level and — when
	// set or when ForPath is set — activates the hold→constraint→context
	// stable sort. Mode projects each item (summary|hit|full; default full).
	// MaxTokens, when > 0, applies a post-limit token budget. StaleRefs runs
	// the bounded repo-root FS scan (S17/S18).
	ForPath    string   `json:"for_path,omitempty"`
	Governance []string `json:"governance,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	MaxTokens  int      `json:"max_tokens,omitempty"`
	StaleRefs  bool     `json:"stale_refs,omitempty"`

	// RepoRoot is injected by the service layer solely for the --stale-refs
	// FS scan; it is not part of the CLI/MCP surface. Empty disables the scan
	// even when StaleRefs is set.
	RepoRoot string `json:"-"`
}

// Manifest projection modes (S28-S30).
const (
	ModeSummary = "summary"
	ModeHit     = "hit"
	ModeFull    = "full"
)

// for_path input bounds (design §4.2).
const (
	maxForPathBytes    = 1024
	maxForPathSegments = 32
)

// ManifestItem is one concept's bounded metadata. It NEVER carries Markdown
// body content (S20).
type ManifestItem struct {
	OKFID           string   `json:"okf_id,omitempty"`
	Ref             string   `json:"ref,omitempty"`
	IdentityState   string   `json:"identity_state,omitempty"`
	Path            string   `json:"path,omitempty"`
	Title           string   `json:"title,omitempty"`
	Description     string   `json:"description,omitempty"`
	Type            string   `json:"type,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Status          string   `json:"status,omitempty"`
	TrustTier       string   `json:"trust_tier,omitempty"`
	Stale           bool     `json:"stale,omitempty"`
	StaleAfter      string   `json:"stale_after,omitempty"`
	GeneratedAt     string   `json:"generated_at,omitempty"`
	VerifiedAt      string   `json:"verified_at,omitempty"`
	SourceCount     int      `json:"source_count,omitempty"`
	SourceResources []string `json:"source_resources,omitempty"`
	FileSizeBytes   int64    `json:"file_size_bytes,omitempty"`
	EstimatedTokens int64    `json:"estimated_tokens,omitempty"`
	EstimateKind    string   `json:"estimate_kind,omitempty"`

	// Governed-agent-memory extension fields. Governance is the effective
	// level (context by default). CodeRefs are the normalized, validated
	// patterns declared on the concept. MatchedCodeRef is the pattern hit by
	// for_path (S11). StaleCodeRefs are code_refs matching no file on disk
	// (populated only with --stale-refs, S17).
	Governance     string   `json:"governance,omitempty"`
	CodeRefs       []string `json:"code_refs,omitempty"`
	MatchedCodeRef string   `json:"matched_code_ref,omitempty"`
	StaleCodeRefs  []string `json:"stale_code_refs,omitempty"`
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

	// Governed-agent-memory extension. All zero values are omitted, so the
	// default request serializes byte-identically to the pre-change shape.
	NextOffset          int                  `json:"next_offset,omitzero"`
	OmittedCount        ManifestOmittedCount `json:"omitted_count,omitzero"`
	Truncated           bool                 `json:"truncated,omitzero"`
	EstimatedItemTokens int                  `json:"estimated_item_tokens,omitzero"`
	// BudgetTooSmall is set when the first eligible item alone exceeds
	// MaxTokens; MinRequiredTokens is that item's dynamic estimate (S33).
	BudgetTooSmall    *ManifestBudgetTooSmall `json:"budget_too_small,omitempty"`
	Incomplete        bool                    `json:"incomplete,omitzero"`
	ScanWarnings      []string                `json:"scan_warnings,omitempty"`
	GovernanceWarning bool                    `json:"governance_warning,omitzero"`
}

// ManifestOmittedCount distinguishes items dropped by the token budget from
// items outside the offset+limit window (S32).
type ManifestOmittedCount struct {
	BudgetOmitted  int `json:"budget_omitted"`
	TotalRemaining int `json:"total_remaining"`
}

// ManifestBudgetTooSmall reports the minimum budget needed to return the first
// eligible item (S33).
type ManifestBudgetTooSmall struct {
	MinRequiredTokens int `json:"min_required_tokens"`
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
	mode, err := normalizeMode(req.Mode)
	if err != nil {
		return nil, err
	}
	forPath, err := normalizeForPath(req.ForPath)
	if err != nil {
		return nil, err
	}
	govFilter := make(map[string]struct{}, len(req.Governance))
	for _, g := range req.Governance {
		if g = strings.TrimSpace(strings.ToLower(g)); g != "" {
			govFilter[g] = struct{}{}
		}
	}
	// The governance stable sort activates only when for_path or a governance
	// filter is explicitly requested; otherwise the historical order is kept
	// byte-identical (S05).
	governanceSortActive := forPath != "" || len(govFilter) > 0

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
		if !matchesFilters(item, r, req, folderPrefix) {
			continue
		}
		// Governed projection: read governance/code_refs through the typed
		// memorymeta accessors on a cheap CustomFields projection (no body read).
		proj := &okf.Concept{CustomFields: r.Custom}
		level, _ := memorymeta.Governance(proj)
		patterns, _ := memorymeta.CodeRefs(proj)
		item.Governance = string(level)
		item.CodeRefs = patterns

		if len(govFilter) > 0 {
			if _, wanted := govFilter[string(level)]; !wanted {
				continue
			}
		}
		if forPath != "" {
			matched, hit := memorymeta.MatchCodeRefs(forPath, patterns)
			if !hit {
				continue
			}
			item.MatchedCodeRef = matched
		}
		items = append(items, item)
	}

	if governanceSortActive {
		// Stable: hold → constraint → context, then original order within level.
		slices.SortStableFunc(items, func(a, b ManifestItem) int {
			return govRank(a.Governance) - govRank(b.Governance)
		})
	} else {
		// Deterministic order: path ASC, then okf_id ASC.
		slices.SortFunc(items, func(a, b ManifestItem) int {
			if c := strings.Compare(a.Path, b.Path); c != 0 {
				return c
			}
			return strings.Compare(a.OKFID, b.OKFID)
		})
	}

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

	// S17/S18: bounded repo-root FS scan annotating stale_code_refs. Fail
	// closed: symlink escapes / unreadable dirs / the 50k entry cap set
	// incomplete + warnings rather than silently returning empty.
	var scanWarnings []string
	incomplete := false
	if req.StaleRefs && req.RepoRoot != "" {
		incomplete, scanWarnings = annotateStaleRefs(req.RepoRoot, page)
	}

	// Mode projection happens before token budgeting so estimates reflect the
	// serialized shape the caller receives (S28-S31).
	projected := make([]ManifestItem, 0, len(page))
	for _, it := range page {
		projected = append(projected, projectItem(it, mode))
	}

	governanceWarning := governanceSortActive &&
		slices.ContainsFunc(projected, func(it ManifestItem) bool {
			return it.Governance == string(memorymeta.GovernanceHold)
		})

	// Token budget pipeline (S32-S34): filter → sort → offset → limit → budget.
	packed := projected
	omittedBudget := 0
	nextOffset := start + len(projected)
	var estimatedItemTokens int
	var budgetTooSmall *ManifestBudgetTooSmall
	if req.MaxTokens > 0 && len(projected) > 0 {
		ests := make([]int, len(projected))
		for i := range projected {
			ests[i] = estimateItemTokens(projected[i])
		}
		if ests[0] > req.MaxTokens {
			budgetTooSmall = &ManifestBudgetTooSmall{MinRequiredTokens: ests[0]}
			packed = nil
			omittedBudget = len(projected)
			nextOffset = req.Offset
		} else {
			used := 0
			cut := 0
			for i, est := range ests {
				if used+est > req.MaxTokens {
					omittedBudget = len(projected) - i
					break
				}
				used += est
				cut = i + 1
			}
			packed = projected[:cut]
			estimatedItemTokens = used
			nextOffset = req.Offset + cut
		}
	} else {
		for i := range packed {
			estimatedItemTokens += estimateItemTokens(packed[i])
		}
	}

	totalRemaining := total - end
	result := &ManifestResult{
		Total:       total,
		Offset:      req.Offset,
		Limit:       limit,
		Items:       packed,
		IndexStatus: ObserveIndexStatus(vectorDir, indexStale),
		Warnings:    warnings,

		NextOffset:          nextOffset,
		OmittedCount:        ManifestOmittedCount{BudgetOmitted: omittedBudget, TotalRemaining: totalRemaining},
		Truncated:           omittedBudget > 0 || totalRemaining > 0,
		EstimatedItemTokens: estimatedItemTokens,
		BudgetTooSmall:      budgetTooSmall,
		Incomplete:          incomplete,
		ScanWarnings:        scanWarnings,
		GovernanceWarning:   governanceWarning,
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

// normalizeMode validates the projection mode; empty means full (default).
func normalizeMode(mode string) (string, error) {
	switch strings.TrimSpace(mode) {
	case "", ModeFull:
		return ModeFull, nil
	case ModeSummary, ModeHit:
		return strings.TrimSpace(mode), nil
	default:
		return "", wrapInvalidRequest("mode must be summary, hit, or full")
	}
}

// normalizeForPath canonicalizes a repo-relative for_path input lexically and
// rejects absolute paths, backslashes, NUL bytes, and root escapes (S14). It
// never touches the filesystem.
func normalizeForPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if len(p) > maxForPathBytes {
		return "", wrapInvalidRequest("for_path exceeds 1024 bytes")
	}
	if strings.ContainsRune(p, 0) {
		return "", wrapInvalidRequest("for_path contains a NUL byte")
	}
	if strings.Contains(p, "\\") {
		return "", wrapInvalidRequest("for_path must use forward slashes, not backslashes")
	}
	if filepath.IsAbs(p) {
		return "", wrapInvalidRequest("for_path must be repo-relative, not absolute")
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", wrapInvalidRequest("for_path escapes the repository root")
	}
	if segs := strings.Count(cleaned, "/") + 1; segs > maxForPathSegments {
		return "", wrapInvalidRequest("for_path exceeds 32 path segments")
	}
	return cleaned, nil
}

// govRank orders governance levels for the optional sort (hold first).
func govRank(level string) int {
	switch memorymeta.GovernanceLevel(level) {
	case memorymeta.GovernanceHold:
		return 0
	case memorymeta.GovernanceConstraint:
		return 1
	default:
		return 2
	}
}

// projectItem applies the progressive-disclosure projection (S28/S29/S30).
// StaleCodeRefs survives projection whenever it is set (S17).
func projectItem(it ManifestItem, mode string) ManifestItem {
	switch mode {
	case ModeSummary:
		return ManifestItem{
			OKFID:         it.OKFID,
			Title:         it.Title,
			Type:          it.Type,
			Governance:    it.Governance,
			Description:   oneLineDescription(it.Description),
			StaleCodeRefs: it.StaleCodeRefs,
		}
	case ModeHit:
		out := projectItem(it, ModeSummary)
		out.Tags = it.Tags
		out.CodeRefs = it.CodeRefs
		out.Status = it.Status
		out.StaleAfter = it.StaleAfter
		return out
	default:
		return it
	}
}

// oneLineDescription returns the first line of desc truncated to at most 80
// runes (S28).
func oneLineDescription(desc string) string {
	desc = strings.TrimSpace(strings.TrimSpace(strings.Split(desc, "\n")[0]))
	if r := []rune(desc); len(r) > 80 {
		desc = string(r[:80])
	}
	return desc
}

// estimateItemTokens is the per-item token estimate: ceil(go encoding/json
// default bytes(item) / 4) (S34). Go's default encoder escapes <, >, & and
// emits fields in struct order; no custom SetEscapeHTML is used.
func estimateItemTokens(it ManifestItem) int {
	data, err := json.Marshal(it)
	if err != nil {
		return 0
	}
	return (len(data) + 3) / 4
}

// maxStaleScanEntries caps the --stale-refs repo walk (S18: entries, not
// patterns).
const maxStaleScanEntries = 50000

// errStaleScanCap stops the bounded walk once the entry cap is reached.
var errStaleScanCap = errors.New("stale-refs entry cap reached")

// annotateStaleRefs walks repoRoot bounded by 50,000 entries and annotates
// each item's code_refs with the patterns that match no existing file
// (S17/S18). It fails closed: symlink escapes, unreadable directories, and
// the entry cap mark incomplete with human-readable warnings rather than
// silently returning empty.
func annotateStaleRefs(repoRoot string, items []ManifestItem) (bool, []string) {
	files, incomplete, warnings := walkStaleScan(repoRoot)
	// Annotate with whatever the walk found even when incomplete; incomplete
	// itself tells the caller the result is not trustworthy.
	for i := range items {
		for _, pattern := range items[i].CodeRefs {
			if !patternWalksAnyFile(pattern, files) {
				items[i].StaleCodeRefs = append(items[i].StaleCodeRefs, pattern)
			}
		}
	}
	return incomplete, warnings
}

// walkStaleScan returns the set of slash-normalized repo-relative file paths
// seen, plus the incomplete/warnings pair describing any scan problems.
func walkStaleScan(repoRoot string) (map[string]struct{}, bool, []string) {
	files := make(map[string]struct{})
	var warnings []string
	incomplete := false
	count := 0

	markIncomplete := func(msg string) {
		incomplete = true
		warnings = append(warnings, msg)
	}

	err := filepath.WalkDir(repoRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable directory entry: fail closed.
			markIncomplete("unreadable directory entry: " + p + ": " + err.Error())
			return nil
		}
		if count >= maxStaleScanEntries {
			return errStaleScanCap
		}
		count++
		if d.IsDir() && p != repoRoot && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if escapesRepoRoot(repoRoot, p) {
				markIncomplete("symlink escapes repository root: " + p)
			}
			return nil
		}
		if !d.IsDir() {
			rel, rerr := filepath.Rel(repoRoot, p)
			if rerr == nil {
				files[filepath.ToSlash(rel)] = struct{}{}
			}
		}
		return nil
	})
	switch {
	case errors.Is(err, errStaleScanCap):
		markIncomplete("stale-refs scan stopped after 50000 entries")
	case err != nil:
		markIncomplete("stale-refs scan aborted: " + err.Error())
	}
	if len(warnings) == 0 {
		warnings = nil
	}
	return files, incomplete, warnings
}

// escapesRepoRoot reports whether the symlink at link resolves outside
// repoRoot. Relative readlinks are resolved from the link's own directory.
func escapesRepoRoot(repoRoot, link string) bool {
	repoAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return true
	}
	target, err := os.Readlink(link)
	if err != nil {
		return false
	}
	resolved := target
	if !filepath.IsAbs(target) {
		resolved = filepath.Join(filepath.Dir(link), target)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return true
	}
	rel, err := filepath.Rel(repoAbs, resolved)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// patternWalksAnyFile reports whether pattern matches at least one walked file.
// Literal patterns are exact lookups; glob patterns delegate the lexical match
// to memorymeta (single-segment *, bounded **, ?, char classes).
func patternWalksAnyFile(pattern string, files map[string]struct{}) bool {
	if !strings.ContainsAny(pattern, "*?[") {
		_, ok := files[pattern]
		return ok
	}
	for file := range files {
		if _, ok := memorymeta.MatchCodeRefs(file, []string{pattern}); ok {
			return true
		}
	}
	return false
}
