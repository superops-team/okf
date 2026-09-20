package tool

import (
	stdctx "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

// ReviewMemoryRequest is the CAS review of one durable concept's temporal state.
type ReviewMemoryRequest struct {
	// Ref is the stable ref (okf_id or okf://concept/<id>) to review.
	Ref string `json:"ref"`
	// Action is approve, decline, or undo.
	Action string `json:"action"`
	// ExpectedState is the CAS guard: the review only applies when the concept's
	// current normalized state equals this value.
	ExpectedState string `json:"expected_state"`
}

// ReviewMemoryResult reports the before/after states of a successful review.
type ReviewMemoryResult struct {
	Ref           string `json:"ref"`
	PreviousState string `json:"previous_state"`
	CurrentState  string `json:"current_state"`
	ConceptPath   string `json:"concept_path"`
}

// reviewClock is the injectable wall clock for memory_review.reviewed_at. Tests
// swap it for determinism; production uses the UTC wall clock.
var reviewClock = func() time.Time { return time.Now().UTC() }

// ReviewMemory applies a CAS (compare-and-swap) review transition to one durable
// concept. It runs entirely under the repo-scoped temporal lock so that two
// concurrent reviews serialize and exactly one wins the CAS (S29).
//
// approve/decline move proposed -> approved/declined. undo moves
// approved|declined -> proposed (restoring the original confidence). It never
// rewrites the concept's payload_hash or idempotency_key (S21/R2.4).
func (s *Service) ReviewMemory(ctx stdctx.Context, req ReviewMemoryRequest) ToolEnvelope {
	resolved, err := s.resolve()
	if err != nil {
		return failure(OperationReview, "", "", nil, err)
	}
	if err := checkContext(ctx); err != nil {
		return reviewFailure(resolved, err)
	}

	ref := strings.TrimSpace(req.Ref)
	action := strings.ToLower(strings.TrimSpace(req.Action))
	expected := strings.ToLower(strings.TrimSpace(req.ExpectedState))
	if ref == "" {
		return reviewFailure(resolved, toolError{code: ErrInvalidRequest, message: "ref must not be empty", remediation: "Provide the okf_id or okf://concept/<id> of the concept to review."})
	}
	switch action {
	case "approve", "decline", "undo":
	default:
		return reviewFailure(resolved, toolError{code: ErrInvalidRequest, message: fmt.Sprintf("invalid action %q", req.Action), remediation: "Action must be approve, decline, or undo."})
	}
	if expected == "" {
		return reviewFailure(resolved, toolError{code: ErrInvalidRequest, message: "expected_state must not be empty", remediation: "Provide the state the concept is expected to be in."})
	}

	// Outer lock: the whole load -> CAS -> transition -> write -> verify section
	// serializes against relation writes and other reviews for this repo.
	repoLock := repositoryTemporalLock(resolved.knowledgeDir)
	repoLock.Lock()
	defer repoLock.Unlock()

	bundle, err := okf.LoadBundle(resolved.knowledgeDir, okf.DefaultLoadOptions())
	if err != nil {
		return reviewFailure(resolved, fmt.Errorf("load bundle for review: %w", err))
	}
	reg, err := identity.BuildRegistry(bundle.Concepts)
	if err != nil {
		return reviewFailure(resolved, fmt.Errorf("build registry for review: %w", err))
	}

	target, err := reg.Resolve(ref)
	if err != nil {
		return reviewFailure(resolved, temporalError(ErrMemoryRefNotFound,
			fmt.Sprintf("review target %q does not resolve to a known concept", ref),
			"Use the okf_id or okf://concept/<id> of an existing durable concept."))
	}
	if !memorymeta.IsDurable(target.Type) {
		return reviewFailure(resolved, temporalError(ErrInvalidReviewTransition,
			fmt.Sprintf("review target %q is non-durable type %q", ref, target.Type),
			"Only durable note, event, and feedback concepts can be reviewed."))
	}
	view := memorymeta.BuildTemporalView(bundle.Concepts)

	currentState, _ := memorymeta.State(target)
	if string(currentState) != expected {
		return reviewFailure(resolved, temporalError(ErrMemoryStateConflict,
			fmt.Sprintf("concept state is %q, expected %q", currentState, expected),
			"Re-read the current state and retry with the matching expected_state."))
	}

	oldRec, _ := memorymeta.Review(target)
	if err := validateReviewTransition(action, currentState, oldRec); err != nil {
		return reviewFailure(resolved, err)
	}

	targetID := identity.FromConcept(target).ID
	fullPath := filepath.Join(resolved.knowledgeDir, target.FilePath)

	// undo of an approved concept is blocked while an approved updater still points
	// at it (S27/R1.6).
	if action == "undo" && currentState == memorymeta.MemoryApproved {
		if dep := findApprovedUpdater(view, targetID); dep != "" {
			return reviewFailure(resolved, temporalError(ErrMemoryHasApprovedUpdater,
				fmt.Sprintf("approved concept %q has an approved updater %q", targetID, dep),
				fmt.Sprintf("Review the updater %q first, or undo it, before undoing %q.", dep, targetID)))
		}
	}

	// On approve, the (possibly proposed, now activating) relation is re-validated
	// against the fresh bundle: a competitor may have made the target stale.
	if action == "approve" {
		if rel, relWarn := memorymeta.Relation(target); rel.Kind != "" {
			if relWarn != "" {
				return reviewFailure(resolved, temporalError(ErrInvalidMemoryRelation, relWarn, "Repair the concept's memory_relation before approving."))
			}
			if err := preflightApprovedRelation(reg, view, conceptProject(target), rel, targetID); err != nil {
				return reviewFailure(resolved, err)
			}
		}
	}

	// Save the original bytes so a post-rename failure can be restored verbatim
	// (the reviewed concept is NEVER deleted — S31).
	originalBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return reviewFailure(resolved, fmt.Errorf("read concept for review: %w", err))
	}

	newState := applyReviewTransition(target, action, currentState, oldRec)

	data, err := serializeKnowledgeConcept(target)
	if err != nil {
		return reviewFailure(resolved, err)
	}
	if err := s.writeKnowledgeFile(fullPath, data); err != nil {
		if restoreErr := restoreOriginalBytes(fullPath, originalBytes); restoreErr != nil {
			err = fmt.Errorf("%w; restore original bytes failed: %v", err, restoreErr)
		}
		return reviewFailure(resolved, err)
	}

	persisted, err := parser.ParseConcept(fullPath)
	if err != nil {
		_ = restoreOriginalBytes(fullPath, originalBytes)
		return reviewFailure(resolved, fmt.Errorf("verify reviewed concept: %w", err))
	}
	verifyConcept := &okf.Concept{Type: persisted.Type, CustomFields: persisted.CustomFields}
	if st, _ := memorymeta.State(verifyConcept); st != newState {
		_ = restoreOriginalBytes(fullPath, originalBytes)
		return reviewFailure(resolved, temporalError(ErrInvalidReviewTransition,
			fmt.Sprintf("verified state %q does not match expected %q", st, newState),
			"The reviewed file did not serialize the expected state; original restored."))
	}
	if rec, _ := memorymeta.Review(verifyConcept); rec.Action != action || rec.CurrentState != newState {
		_ = restoreOriginalBytes(fullPath, originalBytes)
		return reviewFailure(resolved, temporalError(ErrInvalidReviewTransition,
			"verified review record is inconsistent",
			"The reviewed file did not persist the review record; original restored."))
	}

	return ToolEnvelope{
		SchemaVersion: SchemaVersion,
		Operation:     OperationReview,
		OK:            true,
		Mutating:      true,
		RepoRoot:      resolved.repoRoot,
		KnowledgeDir:  resolved.knowledgeDir,
		Freshness:     readFreshness(resolved),
		Warnings:      []string{},
		Result: ReviewMemoryResult{
			Ref:           identity.CanonicalURI(targetID),
			PreviousState: string(currentState),
			CurrentState:  string(newState),
			ConceptPath:   target.FilePath,
		},
	}
}

// reviewFailure wraps a failure envelope for the review operation, forcing the
// mutating flag because the shared failure() helper does not know OperationReview.
func reviewFailure(resolved resolvedConfig, err error) ToolEnvelope {
	env := failure(OperationReview, resolved.repoRoot, resolved.knowledgeDir, readFreshness(resolved), err)
	env.Mutating = true
	return env
}

// restoreOriginalBytes atomically writes the saved original bytes back to the file
// and parses them back to confirm the restore. The reviewed concept is NEVER
// deleted — this is the rollback path for an in-place mutation (S31).
func restoreOriginalBytes(fullPath string, originalBytes []byte) error {
	if err := atomicWriteKnowledgeFile(fullPath, originalBytes); err != nil {
		return err
	}
	if _, err := parser.ParseConcept(fullPath); err != nil {
		return fmt.Errorf("post-restore parse: %w", err)
	}
	return nil
}

// validateReviewTransition enforces the legal transition table (design §6.2).
func validateReviewTransition(action string, current memorymeta.MemoryState, oldRec memorymeta.ReviewRecord) error {
	switch action {
	case "approve", "decline":
		if current != memorymeta.MemoryProposed {
			return temporalError(ErrInvalidReviewTransition,
				fmt.Sprintf("%s is only valid from %s, current is %s", action, memorymeta.MemoryProposed, current),
				"Only a proposed concept can be approved or declined.")
		}
	case "undo":
		if current != memorymeta.MemoryApproved && current != memorymeta.MemoryDeclined {
			return temporalError(ErrInvalidReviewTransition,
				fmt.Sprintf("undo is only valid from %s or %s, current is %s", memorymeta.MemoryApproved, memorymeta.MemoryDeclined, current),
				"Undo returns a reviewed approved/declined concept to proposed.")
		}
		// undo on a born-approved concept (approved at write time, never reviewed)
		// is not a legal transition: there is no original proposal to restore.
		if current == memorymeta.MemoryApproved && oldRec.Action == "" {
			return temporalError(ErrInvalidReviewTransition,
				"undo is not valid on an approved concept that was never reviewed",
				"Only a concept that was approved through a prior review can be undone.")
		}
	}
	return nil
}

// applyReviewTransition mutates target in place and returns the new state. It
// stashes (on approve/decline) or restores (on undo) memory_confidence and writes
// the bounded latest memory_review record.
func applyReviewTransition(target *okf.Concept, action string, current memorymeta.MemoryState, oldRec memorymeta.ReviewRecord) memorymeta.MemoryState {
	reviewedAt := reviewClock().Format(time.RFC3339Nano)

	switch action {
	case "approve", "decline":
		// proposed -> approved|declined: stash the confidence, then drop it.
		prevConf, _, _ := memorymeta.Confidence(target)
		newState := memorymeta.MemoryApproved
		if action == "decline" {
			newState = memorymeta.MemoryDeclined
		}
		memorymeta.SetState(target, newState)
		delete(target.CustomFields, "memory_confidence")
		setReviewRecord(target, memorymeta.ReviewRecord{
			ReviewedAt:         reviewedAt,
			PreviousState:      memorymeta.MemoryProposed,
			CurrentState:       newState,
			Action:             action,
			PreviousConfidence: new(prevConf),
		})
		return newState
	default: // undo: approved|declined -> proposed, restore the prior confidence.
		memorymeta.SetState(target, memorymeta.MemoryProposed)
		if oldRec.PreviousConfidence != nil {
			target.CustomFields["memory_confidence"] = *oldRec.PreviousConfidence
		} else {
			delete(target.CustomFields, "memory_confidence")
		}
		setReviewRecord(target, memorymeta.ReviewRecord{
			ReviewedAt:         reviewedAt,
			PreviousState:      current,
			CurrentState:       memorymeta.MemoryProposed,
			Action:             "undo",
			PreviousConfidence: oldRec.PreviousConfidence,
		})
		return memorymeta.MemoryProposed
	}
}

// setReviewRecord writes the bounded latest memory_review record onto a concept.
func setReviewRecord(c *okf.Concept, rec memorymeta.ReviewRecord) {
	if c.CustomFields == nil {
		c.CustomFields = map[string]any{}
	}
	m := map[string]any{
		"reviewed_at":    rec.ReviewedAt,
		"previous_state": string(rec.PreviousState),
		"current_state":  string(rec.CurrentState),
		"action":         rec.Action,
	}
	if rec.PreviousConfidence != nil {
		m["previous_confidence"] = *rec.PreviousConfidence
	}
	c.CustomFields["memory_review"] = m
}

// findApprovedUpdater returns the canonical ref of an approved concept that has an
// active updates edge pointing at targetID, or "" when none exists.
func findApprovedUpdater(view *memorymeta.TemporalView, targetID string) string {
	for id, e := range view.Entries {
		if id == targetID {
			continue
		}
		if e.State != memorymeta.MemoryApproved || e.Relation.Kind != memorymeta.RelationUpdates || len(e.Relation.Targets) != 1 {
			continue
		}
		if e.Relation.Targets[0] == targetID {
			return identity.CanonicalURI(id)
		}
	}
	return ""
}
