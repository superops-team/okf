package tool

import (
	stdctx "context"
	"errors"
	"strings"

	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/reflect"
	"github.com/superops-team/okf/pkg/relationrecall"
)

const (
	OperationReflect        = "reflect"
	OperationRelationRecall = "relation_recall"
)

// ReflectRequest is the request for the reflective multi-round retrieval.
type ReflectRequest struct {
	Question    string `json:"question"`
	MinEvidence int    `json:"min_evidence,omitempty"`
	MaxRounds   int    `json:"max_rounds,omitempty"`
}

// ReflectResult is the reflective retrieval output.
type ReflectResult struct {
	Evidence    []reflect.Evidence   `json:"evidence"`
	Trace       []reflect.RoundTrace `json:"trace"`
	NeedClarify bool                 `json:"need_clarify"`
	TrapBlocked bool                 `json:"trap_blocked"`
	Suggestion  string               `json:"suggestion,omitempty"`
}

// RelationRecallRequest recalls neighbors for one anchor.
type RelationRecallRequest struct {
	Anchor string `json:"anchor"`
}

// Reflect runs the bounded multi-round reflective retrieval. It is read-only.
func (s *Service) Reflect(ctx stdctx.Context, req ReflectRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationReflect, "", "", nil, err)
	}
	freshness := readFreshness(resolved)
	if strings.TrimSpace(req.Question) == "" {
		return failure(OperationReflect, resolved.repoRoot, resolved.knowledgeDir, freshness, toolError{
			code:    ErrInvalidQuery,
			message: "question must not be empty",
		})
	}
	bundle, _, err := loadKnowledgeBundle(resolved)
	if err != nil {
		return failure(OperationReflect, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}
	view := memorymeta.BuildTemporalView(bundle.Concepts)

	// Round 1: rank concepts and extract okf_ids.
	var round1IDs []string
	for _, hit := range rankConcepts(bundle.Concepts, req.Question, queryFilters{}) {
		if hit.okfID != "" {
			round1IDs = append(round1IDs, hit.okfID)
		}
	}

	// Round 2: relation recall from alive anchors.
	relationFn := func(anchorID string) []string {
		res, err := relationrecall.Recall(anchorID, view)
		if err != nil {
			return nil
		}
		var out []string
		for _, h := range res.Hits {
			out = append(out, h.OKFID)
		}
		return out
	}

	// Trap gate: drop non-approved concepts (proposed/declined = poison traps).
	trapGate := func(id string) bool {
		e, ok := view.Entries[id]
		return ok && e.State != memorymeta.MemoryApproved
	}

	result, err := reflect.Run(req.Question, reflect.Options{
		MinEvidence: req.MinEvidence,
		MaxRounds:   req.MaxRounds,
	}, func(string) []string { return round1IDs }, relationFn, trapGate)
	if err != nil {
		return failure(OperationReflect, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationReflect,
		OK:            true,
		Mutating:      false,
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     freshness,
		Warnings:      []string{},
		Result: ReflectResult{
			Evidence:    result.Evidence,
			Trace:       result.Trace,
			NeedClarify: result.NeedClarify,
			TrapBlocked: result.TrapBlocked,
			Suggestion:  result.Suggestion,
		},
	}
}

// RelationRecall returns approved extends neighbors and updates chain for an anchor.
func (s *Service) RelationRecall(ctx stdctx.Context, req RelationRecallRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationRelationRecall, "", "", nil, err)
	}
	freshness := readFreshness(resolved)
	if strings.TrimSpace(req.Anchor) == "" {
		return failure(OperationRelationRecall, resolved.repoRoot, resolved.knowledgeDir, freshness, toolError{
			code:    ErrInvalidRequest,
			message: "anchor must not be empty",
		})
	}
	bundle, _, err := loadKnowledgeBundle(resolved)
	if err != nil {
		return failure(OperationRelationRecall, resolved.repoRoot, resolved.knowledgeDir, freshness, err)
	}
	view := memorymeta.BuildTemporalView(bundle.Concepts)
	result, err := relationrecall.Recall(req.Anchor, view)
	if err != nil {
		// Map the recall error to a stable wire code instead of a generic
		// internal_error, so CLI and MCP clients get a typed memory_ref_not_found.
		return failure(OperationRelationRecall, resolved.repoRoot, resolved.knowledgeDir, freshness,
			relationErrorTool(err))
	}
	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationRelationRecall,
		OK:            true,
		Mutating:      false,
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     freshness,
		Warnings:      []string{},
		Result:        result,
	}
}

// relationErrorTool maps a relation-recall failure to a stable wire code. Only
// the anchor-not-found case is expected in practice; everything else stays a
// generic internal_error via the toolError default.
func relationErrorTool(err error) error {
	if errors.Is(err, memorymeta.ErrMemoryRefNotFound) {
		return toolError{code: ErrMemoryRefNotFound, message: err.Error(),
			remediation: "Pass a stable okf_id that exists in the bundle."}
	}
	return err
}
