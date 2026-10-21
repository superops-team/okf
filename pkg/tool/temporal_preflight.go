package tool

import (
	"fmt"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/okf"
)

// Approved-relation preflight.
//
// Before an APPROVED relation write commits, P1 loads the whole bundle, builds
// the identity registry and the memorymeta view, and validates the would-be edge
// against the EXISTING graph. If the topology is invalid the write aborts with
// zero files on disk (S22). Proposed relation metadata is NOT preflighted here:
// it is stored inactive and validated again at approve time (P2).
//
// A would-be updates edge to target t is legal only when:
//   - t resolves to a known concept (memory_ref_not_found otherwise);
//   - t is durable (note/event/feedback);
//   - t is currently approved;
//   - t does not cross effective project;
//   - t is not the source itself;
//   - t's component is healthy (not ambiguous / cyclic / cross-project);
//   - t is the CURRENT chain head for an updates edge, i.e. no approved updater
//     already points at t (otherwise target_not_current, naming the real head).
//
// An extends edge shares all of the above except the chain-head requirement,
// because extends is a side relation that never changes currentness.

// preflightApprovedRelation validates a would-be approved relation write. The
// targets on rel are already normalized to bare okf ids. srcProject is the new
// source concept's effective project; srcSelfID is its freshly assigned okf id
// (empty when not yet assigned, which disables the self-edge check).
func preflightApprovedRelation(
	reg *identity.Registry,
	view *memorymeta.TemporalView,
	srcProject string,
	rel memorymeta.MemoryRelation,
	srcSelfID string,
) error {
	for _, targetID := range rel.Targets {
		target, err := reg.Resolve(targetID)
		if err != nil {
			return temporalError(ErrMemoryRefNotFound,
				fmt.Sprintf("relation target %q does not resolve to a known durable concept", targetID),
				"Use the okf_id (or okf://concept/<id>) of an existing durable concept.")
		}
		if !memorymeta.IsDurable(target.Type) {
			return temporalError(ErrInvalidMemoryRelation,
				fmt.Sprintf("relation target %q is a non-durable %q concept", targetID, target.Type),
				"Relation targets must be durable note, event, or feedback concepts.")
		}
		entry, ok := view.Entries[targetID]
		if !ok {
			return temporalError(ErrInvalidMemoryRelation,
				fmt.Sprintf("relation target %q is not present in the temporal view", targetID),
				"Target must be a stable, addressable durable concept.")
		}
		if entry.State != memorymeta.MemoryApproved {
			return temporalError(ErrInvalidMemoryRelation,
				fmt.Sprintf("relation target %q is %s, but approved relations require an approved target", targetID, entry.State),
				"Approve the target first, or write this relation as proposed metadata.")
		}
		if targetID == srcSelfID {
			return temporalError(ErrInvalidMemoryRelation,
				fmt.Sprintf("relation target %q is the concept itself", targetID),
				"A concept cannot relate to itself.")
		}
		if conceptProject(srcProject) != conceptProject(target) {
			return temporalError(ErrMemoryRelationCrossProject,
				fmt.Sprintf("relation target %q belongs to a different effective project", targetID),
				"Relation edges may only connect concepts within the same project.")
		}
		if entry.InvalidReason != "" {
			return mapInvalidReason(entry.InvalidReason, targetID)
		}
		if rel.Kind == memorymeta.RelationUpdates && !view.IsCurrent(targetID) {
			head := targetID
			if history, err := view.History(targetID); err == nil {
				head = history.CurrentRef
			}
			return temporalError(ErrTargetNotCurrent,
				fmt.Sprintf("updates target %q is not the current chain head", targetID),
				fmt.Sprintf("Update the current head %q instead of the stale target %q.", head, targetID))
		}
	}
	return nil
}

// mapInvalidReason translates a memorymeta InvalidReason into a stable tool code.
func mapInvalidReason(reason, targetID string) error {
	switch reason {
	case "ambiguous_update_head":
		return temporalError(ErrAmbiguousUpdateHead,
			fmt.Sprintf("relation target %q sits behind an ambiguous update head", targetID),
			"Resolve the competing approved updaters before targeting this concept.")
	case "memory_relation_cycle":
		return temporalError(ErrMemoryRelationCycle,
			fmt.Sprintf("relation target %q is part of an update cycle", targetID),
			"Break the cycle before adding a new updates edge.")
	case "memory_relation_cross_project":
		return temporalError(ErrMemoryRelationCrossProject,
			fmt.Sprintf("relation target %q crosses projects", targetID),
			"Relation edges may only connect concepts within the same project.")
	default:
		return temporalError(ErrInvalidMemoryRelation,
			fmt.Sprintf("relation target %q is in an invalid component (%s)", targetID, reason),
			"Repair the target's component before adding a relation.")
	}
}

// conceptProject returns the effective project of a string (the source payload
// project is already a string) or of a concept's CustomFields["project"].
func conceptProject(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case *okf.Concept:
		if v == nil || v.CustomFields == nil {
			return ""
		}
		if p, ok := v.CustomFields["project"].(string); ok {
			return p
		}
	}
	return ""
}
