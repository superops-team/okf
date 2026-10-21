package tool

// Temporal memory relations: stable operation name and wire error codes.
//
// This file owns every new identifier introduced by the temporal-aware durable
// write (P1) and the ReviewMemory CAS mutation (P2). It deliberately does not
// touch service.go's existing const block so the parallel Query/Context author
// and this author never collide on the same declaration.
//
// The codes below are the wire contract: they are returned verbatim in
// ToolError.Code and must not be renamed without bumping the tool protocol.

const (
	// OperationReview is the operation name for the ReviewMemory CAS mutation.
	OperationReview = "memory_review"

	// ErrInvalidMemoryState: the temporal state/confidence combination is not
	// permitted (e.g. declined created directly, proposed without confidence,
	// or approved carrying a confidence value).
	ErrInvalidMemoryState = "invalid_memory_state"
	// ErrInvalidMemoryRelation: the relation shape is wrong (unknown kind, bad
	// target count, malformed/dangling/self/duplicate target string, or a
	// non-durable / non-approved target).
	ErrInvalidMemoryRelation = "invalid_memory_relation"
	// ErrMemoryRefNotFound: a review relation target resolves to no known durable
	// concept in the loaded bundle.
	ErrMemoryRefNotFound = "memory_ref_not_found"
	// ErrMemoryRelationCrossProject: a relation edge crosses effective projects.
	ErrMemoryRelationCrossProject = "memory_relation_cross_project"
	// ErrMemoryRelationCycle: a relation edge sits inside an update cycle.
	ErrMemoryRelationCycle = "memory_relation_cycle"
	// ErrAmbiguousUpdateHead: two or more approved concepts already update the
	// same target, so the current head is ambiguous.
	ErrAmbiguousUpdateHead = "ambiguous_update_head"
	// ErrTargetNotCurrent: an approved updates edge targets a concept that is not
	// the current chain head; the remediation names the head's stable id.
	ErrTargetNotCurrent = "target_not_current"
	// ErrMemoryStateConflict: the ReviewMemory expected_state did not match the
	// concept's current state (a lost CAS race).
	ErrMemoryStateConflict = "memory_state_conflict"
	// ErrInvalidReviewTransition: the requested review action is not a legal
	// transition from the current state (or the concept is non-durable).
	ErrInvalidReviewTransition = "invalid_review_transition"
	// ErrMemoryHasApprovedUpdater: undo of an approved concept is blocked because
	// an approved updater still targets it; the remediation names the dependent ref.
	ErrMemoryHasApprovedUpdater = "memory_has_approved_updater"
	// ErrTemporalHistoryTooDeep: the update chain walked by a view exceeded its
	// safety bound.
	ErrTemporalHistoryTooDeep = "temporal_history_too_deep"
)

// temporalError builds a toolError with a stable temporal code. It is the single
// factory for these codes so messages and remediations stay consistent.
func temporalError(code, message, remediation string) toolError {
	return toolError{code: code, message: message, remediation: remediation}
}
