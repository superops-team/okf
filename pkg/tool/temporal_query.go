package tool

// P3 (temporal query/context views for add-temporal-memory-relations): additive
// temporal projection layered on top of the existing Query/Context pipeline.
//
// Design §7.1 validation matrix:
//
//	omitted/current | query required | refs forbidden | existing QueryResult
//	all             | query required | refs forbidden | existing QueryResult + temporal annotations
//	history         | query empty    | refs == 1       | MemoryHistoryResult envelope
//	review queue    | query empty    | refs forbidden  | MemoryReviewQueueResult envelope
//
// When a bundle carries no temporal metadata the legacy fast path is taken and
// output bytes are unchanged (S17). Temporal projection runs BEFORE ranking so
// excluded history/proposed/declined durable concepts cannot consume the limit.

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/okf"
)

// memory view mode strings accepted on QueryRequest.MemoryView /
// ContextRequest.MemoryView. The empty string is the default (== current).
const (
	memoryViewCurrent = "current"
	memoryViewAll     = "all"
	memoryViewHistory = "history"
)

// review queue pagination bounds (S34).
const (
	memoryReviewQueueDefaultLimit = 20
	memoryReviewQueueMaxLimit     = 100
)

// temporalMode is the normalized query temporal axis after validation.
type temporalMode int

const (
	// temporalModeDefault is an omitted view; it behaves like current once a
	// bundle carries temporal data, and like the legacy path otherwise (S17).
	temporalModeDefault temporalMode = iota
	temporalModeCurrent
	temporalModeAll
	temporalModeHistory
	temporalModeReviewQueue
)

// classifyTemporalQuery validates the additive temporal fields against the §7.1
// matrix. It runs BEFORE the existing empty-query guard. A zero toolError means
// the request is well-formed for its chosen mode.
func classifyTemporalQuery(req QueryRequest) (temporalMode, toolError) {
	mode := temporalModeDefault
	switch strings.TrimSpace(req.MemoryView) {
	case "":
		mode = temporalModeDefault
	case memoryViewCurrent:
		mode = temporalModeCurrent
	case memoryViewAll:
		mode = temporalModeAll
	case memoryViewHistory:
		mode = temporalModeHistory
	default:
		return 0, toolError{
			code:        ErrInvalidRequest,
			message:     fmt.Sprintf("unknown memory_view %q", req.MemoryView),
			remediation: "Use one of: current, all, history (omit for the default current view).",
		}
	}

	hasQuery := strings.TrimSpace(req.Query) != ""
	hasRefs := len(req.Refs) > 0

	// Review queue is a dedicated mode that forbids every other query axis (S35).
	if req.MemoryReviewQueue {
		switch {
		case hasQuery:
			return 0, toolError{code: ErrInvalidRequest,
				message:     "memory_review_queue cannot be combined with query",
				remediation: "Omit query when reading the review queue."}
		case mode == temporalModeHistory:
			return 0, toolError{code: ErrInvalidRequest,
				message:     "memory_review_queue cannot be combined with memory_view=history",
				remediation: "Use either the review queue or history, not both."}
		case req.MemoryCheck:
			return 0, toolError{code: ErrInvalidRequest,
				message:     "memory_review_queue cannot be combined with memory_check",
				remediation: "The review queue replaces the duplicate-detection output for this call."}
		case req.GroupBy != "":
			return 0, toolError{code: ErrInvalidRequest,
				message:     "memory_review_queue cannot be combined with group_by",
				remediation: "Omit group_by when reading the review queue."}
		case hasRefs:
			return 0, toolError{code: ErrInvalidRequest,
				message:     "memory_review_queue cannot be combined with refs",
				remediation: "Omit refs when reading the review queue."}
		}
		return temporalModeReviewQueue, toolError{}
	}

	switch mode {
	case temporalModeHistory:
		if hasQuery {
			return 0, toolError{code: ErrInvalidRequest,
				message:     "memory_view=history requires query to be empty",
				remediation: "Omit query; pass exactly one refs entry instead."}
		}
		if len(req.Refs) != 1 {
			return 0, toolError{code: ErrInvalidRequest,
				message:     fmt.Sprintf("memory_view=history requires exactly one refs entry, got %d", len(req.Refs)),
				remediation: "Pass refs with a single stable okf_id or okf://concept/<id> URI."}
		}
	default:
		if hasRefs {
			return 0, toolError{code: ErrInvalidRequest,
				message:     "refs is only valid with memory_view=history",
				remediation: "Omit refs for the current/all views, or set memory_view=history."}
		}
	}

	return mode, toolError{}
}

// MemoryHistoryItem is one member of a resolved update chain.
type MemoryHistoryItem struct {
	OKFID          string `json:"okf_id"`
	MemoryState    string `json:"memory_state"`
	Current        bool   `json:"current"`
	RelationKind   string `json:"relation_kind,omitempty"`
	RelationTarget string `json:"relation_target,omitempty"`
}

// MemoryHistoryResult is the dedicated envelope for memory_view=history.
type MemoryHistoryResult struct {
	RequestedRef string              `json:"requested_ref"`
	CurrentRef   string              `json:"current_ref,omitempty"`
	Items        []MemoryHistoryItem `json:"items"`
}

// buildMemoryHistory maps the memorymeta history walk into the tool envelope.
func buildMemoryHistory(ref string, view *memorymeta.TemporalView) (*MemoryHistoryResult, error) {
	h, err := view.History(ref)
	if err != nil {
		return nil, err
	}
	items := make([]MemoryHistoryItem, 0, len(h.Items))
	for _, it := range h.Items {
		item := MemoryHistoryItem{
			OKFID:       it.OKFID,
			MemoryState: it.MemoryState,
			Current:     it.Current,
		}
		if it.Edge != nil {
			item.RelationKind = it.Edge.Kind
			item.RelationTarget = it.Edge.Target
		}
		items = append(items, item)
	}
	return &MemoryHistoryResult{
		RequestedRef: h.RequestedRef,
		CurrentRef:   h.CurrentRef,
		Items:        items,
	}, nil
}

// historyErrorTool maps a history-walk failure to a stable wire code with
// remediation. The codes match the §9 error contract.
func historyErrorTool(err error) toolError {
	switch {
	case errors.Is(err, memorymeta.ErrMemoryRefNotFound):
		return toolError{code: ErrMemoryRefNotFound, message: err.Error(),
			remediation: "Pass a stable okf_id that exists in the bundle."}
	case errors.Is(err, memorymeta.ErrMemoryRelationCycle):
		return toolError{code: ErrMemoryRelationCycle, message: err.Error(),
			remediation: "Break the approved-updates cycle before reading history."}
	case errors.Is(err, memorymeta.ErrAmbiguousUpdateHead):
		return toolError{code: ErrAmbiguousUpdateHead, message: err.Error(),
			remediation: "A concept has multiple approved updaters; resolve which one is current."}
	case errors.Is(err, memorymeta.ErrTemporalHistoryTooDeep):
		return toolError{code: ErrTemporalHistoryTooDeep, message: err.Error(),
			remediation: "The update chain exceeds the depth bound; request a shorter chain."}
	case errors.Is(err, memorymeta.ErrMemoryRelationCrossProject):
		return toolError{code: ErrMemoryRelationCrossProject, message: err.Error(),
			remediation: "Update edges may only point within the same project."}
	case errors.Is(err, memorymeta.ErrMemoryRelationDangling):
		return toolError{code: "memory_relation_dangling_target", message: err.Error(),
			remediation: "The update edge points at a missing or non-approved target."}
	default:
		return toolError{code: "internal_error", message: err.Error()}
	}
}

// MemoryReviewQueueItem is a body-free proposed durable concept in the queue.
type MemoryReviewQueueItem struct {
	OKFID        string   `json:"okf_id"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Confidence   float64  `json:"confidence"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	Relation     string   `json:"relation,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
}

// MemoryReviewQueueResult is the dedicated envelope for memory_review_queue.
type MemoryReviewQueueResult struct {
	Items []MemoryReviewQueueItem `json:"items"`
}

// buildMemoryReviewQueue lists proposed durable concepts, body-free. Order is
// confidence DESC, then generated.at ASC (missing sorts last), then okf_id ASC.
// Limit defaults to 20 and is capped at 100 (S34).
func buildMemoryReviewQueue(concepts []*okf.Concept, limit int) MemoryReviewQueueResult {
	if limit <= 0 {
		limit = memoryReviewQueueDefaultLimit
	}
	limit = min(limit, memoryReviewQueueMaxLimit)

	var candidates []*okf.Concept
	for _, c := range concepts {
		if c == nil || !memorymeta.IsDurable(c.Type) {
			continue
		}
		if st, _ := memorymeta.State(c); st != memorymeta.MemoryProposed {
			continue
		}
		if identity.FromConcept(c).ID == "" {
			continue
		}
		candidates = append(candidates, c)
	}

	slices.SortFunc(candidates, func(a, b *okf.Concept) int {
		ca, _, _ := memorymeta.Confidence(a)
		cb, _, _ := memorymeta.Confidence(b)
		if ca != cb {
			return cmp.Compare(cb, ca) // confidence DESC
		}
		aa, ab := generatedAt(a), generatedAt(b)
		if aa != ab {
			switch {
			case aa == "":
				return 1
			case ab == "":
				return -1
			default:
				return strings.Compare(aa, ab) // generated.at ASC
			}
		}
		return strings.Compare(identity.FromConcept(a).ID, identity.FromConcept(b).ID)
	})

	items := make([]MemoryReviewQueueItem, 0, min(len(candidates), limit))
	for _, c := range candidates {
		if len(items) >= limit {
			break
		}
		conf, _, _ := memorymeta.Confidence(c)
		rel, _ := memorymeta.Relation(c)
		items = append(items, MemoryReviewQueueItem{
			OKFID:        identity.FromConcept(c).ID,
			Title:        c.Title,
			Type:         c.Type,
			Confidence:   conf,
			EvidenceRefs: evidenceRefs(c),
			Relation:     string(rel.Kind),
			CreatedAt:    generatedAt(c),
		})
	}
	return MemoryReviewQueueResult{Items: items}
}

// generatedAt returns the concept's generated.at timestamp, or "" when absent.
func generatedAt(c *okf.Concept) string {
	if c != nil && c.Generated != nil {
		return c.Generated.At
	}
	return ""
}

// evidenceRefs reads provenance.evidence_refs as a []string.
func evidenceRefs(c *okf.Concept) []string {
	if c == nil || c.CustomFields == nil {
		return nil
	}
	prov, _ := c.CustomFields["provenance"].(map[string]any)
	if prov == nil {
		return nil
	}
	raw, ok := prov["evidence_refs"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// filterConceptsForView drops non-current durable concepts for the current view
// so they cannot consume the TopK limit (S32). The all view and the no-temporal
// fast path return the input unchanged. Non-durable and untracked concepts are
// always unaffected.
func filterConceptsForView(concepts []*okf.Concept, view *memorymeta.TemporalView, all bool) []*okf.Concept {
	if !view.HasTemporalData || all {
		return concepts
	}
	currentSet := view.CurrentSet()
	out := make([]*okf.Concept, 0, len(concepts))
	for _, c := range concepts {
		if c == nil {
			continue
		}
		if !memorymeta.IsDurable(c.Type) {
			out = append(out, c)
			continue
		}
		id := identity.FromConcept(c).ID
		_, tracked := view.Entries[id]
		if !tracked || currentSet[id] {
			out = append(out, c)
			continue
		}
		// Tracked durable concept that is not the current head: excluded.
	}
	return out
}

// annotateQueryHit adds temporal annotations when the bundle carries temporal
// data and the hit resolves to a tracked durable concept (S32/S36).
func annotateQueryHit(hit *QueryHit, view *memorymeta.TemporalView) {
	if !view.HasTemporalData || hit == nil || hit.okfID == "" {
		return
	}
	entry, ok := view.Entries[hit.okfID]
	if !ok {
		return
	}
	hit.MemoryState = string(entry.State)
	current := entry.Current
	hit.MemoryCurrent = &current
	if entry.Relation.Kind != "" {
		hit.MemoryRelationKind = string(entry.Relation.Kind)
	}
	if len(entry.Relation.Targets) > 0 {
		hit.MemoryRelationTargets = append([]string{}, entry.Relation.Targets...)
	}
}

// annotateContextItem adds temporal annotations for a packed context item when
// its source concept is a tracked durable concept (S36). Explicitly requested
// refs are always packed, regardless of state.
func annotateContextItem(item *ContextItem, concept *okf.Concept, view *memorymeta.TemporalView) {
	if !view.HasTemporalData || item == nil || concept == nil {
		return
	}
	entry, ok := view.Entries[identity.FromConcept(concept).ID]
	if !ok {
		return
	}
	item.MemoryState = string(entry.State)
	current := entry.Current
	item.MemoryCurrent = &current
	if entry.Relation.Kind != "" {
		item.MemoryRelationKind = string(entry.Relation.Kind)
	}
	if len(entry.Relation.Targets) > 0 {
		item.MemoryRelationTargets = append([]string{}, entry.Relation.Targets...)
	}
}

// TemporalLintIssue is one strict temporal-contract violation with a safe
// bundle-relative location (T3.3).
type TemporalLintIssue struct {
	Path    string `json:"path"`
	OKFID   string `json:"okf_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidateTemporalBundle runs memorymeta.ValidateTemporal(c, true) over every
// durable concept in the bundle. FilePath is already bundle-relative, so the
// paths are safe to surface. Remaining wiring into pkg/lint's Issue pipeline is
// tracked for P4.
func ValidateTemporalBundle(concepts []*okf.Concept) []TemporalLintIssue {
	var out []TemporalLintIssue
	for _, c := range concepts {
		if c == nil || !memorymeta.IsDurable(c.Type) {
			continue
		}
		for _, msg := range memorymeta.ValidateTemporal(c, true) {
			out = append(out, TemporalLintIssue{
				Path:    c.FilePath,
				OKFID:   identity.FromConcept(c).ID,
				Code:    temporalErrorCode(msg),
				Message: msg,
			})
		}
	}
	return out
}

// temporalErrorCode extracts the leading stable code prefix from a validation
// message (e.g. "invalid_memory_state: ...").
func temporalErrorCode(msg string) string {
	if i := strings.Index(msg, ":"); i > 0 {
		return msg[:i]
	}
	return ErrInvalidMemoryState
}
