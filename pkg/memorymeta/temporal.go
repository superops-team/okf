package memorymeta

// T0.1 / design §3: typed temporal CustomFields accessors. Temporal metadata
// (memory_state, memory_confidence, memory_relation, memory_review) lives
// ONLY in Concept.CustomFields under the memory_ namespace. The core okf.Concept
// and Status structs are never modified.
//
// Read accessors are conservative: malformed input degrades to a safe default
// (proposed state) plus a warning rather than panicking or silently entering the
// current view. ValidateTemporal(c, true) turns those warnings into hard errors.

import (
	"fmt"
	"math"
	"strings"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/okf"
)

// MemoryState is the review/lifecycle state of a durable concept.
type MemoryState string

const (
	// MemoryApproved is the default, current state.
	MemoryApproved MemoryState = "approved"
	// MemoryProposed is a quarantined proposal that never affects current view.
	MemoryProposed MemoryState = "proposed"
	// MemoryDeclined is a rejected proposal; excluded from current view.
	MemoryDeclined MemoryState = "declined"
)

// RelationKind is the temporal relation type (distinct from generated code
// relation_kind/source/target).
type RelationKind string

const (
	// RelationUpdates marks this concept as the newer revision of its target.
	RelationUpdates RelationKind = "updates"
	// RelationExtends marks this concept as an elaboration of its target.
	RelationExtends RelationKind = "extends"
)

// MemoryRelation is a typed temporal relation stored under memory_relation.
type MemoryRelation struct {
	Kind    RelationKind
	Targets []string
}

// ReviewRecord is the bounded latest transition stored under memory_review.
// Git history remains the complete audit log; only the latest transition is kept.
type ReviewRecord struct {
	ReviewedAt         string
	PreviousState      MemoryState
	CurrentState       MemoryState
	Action             string
	PreviousConfidence *float64
}

// maxRelationTargetBytes bounds each target string length (design §3.3).
const maxRelationTargetBytes = 64

// durableTypes are the concept types that may carry temporal memory metadata.
var durableTypes = map[string]bool{
	"note":     true,
	"event":    true,
	"feedback": true,
}

// IsDurable reports whether t is a durable memory concept type.
func IsDurable(t string) bool { return durableTypes[t] }

// State reads and normalizes memory_state. Missing/empty defaults to approved.
// Unknown or wrong-type values degrade to proposed with a warning (S01,S02,S06).
func State(c *okf.Concept) (MemoryState, string) {
	if c == nil || c.CustomFields == nil {
		return MemoryApproved, ""
	}
	raw, ok := c.CustomFields["memory_state"]
	if !ok {
		return MemoryApproved, ""
	}
	s, ok := raw.(string)
	if !ok {
		return MemoryProposed, fmt.Sprintf("invalid_memory_state: memory_state is %T, not a string; treating as proposed", raw)
	}
	norm := strings.ToLower(strings.TrimSpace(s))
	if norm == "" {
		return MemoryApproved, ""
	}
	switch norm {
	case "approved":
		return MemoryApproved, ""
	case "proposed":
		return MemoryProposed, ""
	case "declined":
		return MemoryDeclined, ""
	}
	return MemoryProposed, fmt.Sprintf("invalid_memory_state: unknown memory_state %q; treating as proposed", s)
}

// SetState writes memory_state to CustomFields, initializing the map if needed.
func SetState(c *okf.Concept, state MemoryState) {
	if c == nil {
		return
	}
	if c.CustomFields == nil {
		c.CustomFields = make(map[string]any)
	}
	c.CustomFields["memory_state"] = string(state)
}

// Confidence reads memory_confidence. Returns (value, present, warning).
// Absent -> (0, false, ""). A present-but-malformed value returns present=true
// with a warning (S06).
func Confidence(c *okf.Concept) (float64, bool, string) {
	if c == nil || c.CustomFields == nil {
		return 0, false, ""
	}
	raw, ok := c.CustomFields["memory_confidence"]
	if !ok {
		return 0, false, ""
	}
	switch v := raw.(type) {
	case float64:
		return v, true, ""
	case int:
		return float64(v), true, ""
	case int64:
		return float64(v), true, ""
	}
	return 0, true, fmt.Sprintf("memory_confidence is %T, not a number", raw)
}

// Relation reads and normalizes memory_relation. Targets are normalized to bare
// okf IDs (the okf://concept/ URI prefix is stripped). Unknown kinds, non-list
// targets, and non-string entries each produce a warning (S04). The generated
// code relation_kind/source/target fields are neither read nor written.
func Relation(c *okf.Concept) (MemoryRelation, string) {
	rel := MemoryRelation{}
	if c == nil || c.CustomFields == nil {
		return rel, ""
	}
	raw, ok := c.CustomFields["memory_relation"]
	if !ok {
		return rel, ""
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return rel, fmt.Sprintf("invalid_memory_relation: memory_relation is %T, not a map", raw)
	}

	var warns []string

	kindRaw, hasKind := m["kind"]
	if !hasKind {
		warns = append(warns, "invalid_memory_relation: missing kind")
	} else if ks, ok := kindRaw.(string); !ok {
		warns = append(warns, fmt.Sprintf("invalid_memory_relation: kind is %T, not a string", kindRaw))
	} else {
		norm := RelationKind(strings.ToLower(strings.TrimSpace(ks)))
		if norm != RelationUpdates && norm != RelationExtends {
			warns = append(warns, fmt.Sprintf("invalid_memory_relation: unknown kind %q", ks))
		} else {
			rel.Kind = norm
		}
	}

	targetsRaw, hasTargets := m["targets"]
	if !hasTargets {
		warns = append(warns, "invalid_memory_relation: missing targets")
	} else if list, ok := toStringAnySlice(targetsRaw); !ok {
		warns = append(warns, fmt.Sprintf("invalid_memory_relation: targets is %T, not a list", targetsRaw))
	} else {
		for i, item := range list {
			s, ok := item.(string)
			if !ok {
				warns = append(warns, fmt.Sprintf("invalid_memory_relation: targets[%d] is %T, not a string", i, item))
				continue
			}
			rel.Targets = append(rel.Targets, normalizeTarget(s))
		}
	}

	// The relation object accepts only kind and targets; unknown nested fields
	// warn in non-strict reads and fail strict (relationUnknownFields).
	for k := range m {
		if k != "kind" && k != "targets" {
			warns = append(warns, fmt.Sprintf("invalid_memory_relation: unknown field %q in memory_relation", k))
		}
	}

	return rel, strings.Join(warns, "; ")
}

// SetRelation writes memory_relation as a nested {kind,targets} map.
func SetRelation(c *okf.Concept, rel MemoryRelation) {
	if c == nil {
		return
	}
	if c.CustomFields == nil {
		c.CustomFields = make(map[string]any)
	}
	targets := make([]any, len(rel.Targets))
	for i, t := range rel.Targets {
		targets[i] = t
	}
	c.CustomFields["memory_relation"] = map[string]any{
		"kind":    string(rel.Kind),
		"targets": targets,
	}
}

// Review reads the bounded latest memory_review record. Malformed objects warn.
func Review(c *okf.Concept) (ReviewRecord, string) {
	rec := ReviewRecord{}
	if c == nil || c.CustomFields == nil {
		return rec, ""
	}
	raw, ok := c.CustomFields["memory_review"]
	if !ok {
		return rec, ""
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return rec, fmt.Sprintf("memory_review is %T, not a map", raw)
	}
	if v, ok := m["reviewed_at"].(string); ok {
		rec.ReviewedAt = v
	}
	if v, ok := m["previous_state"].(string); ok {
		rec.PreviousState = MemoryState(strings.ToLower(strings.TrimSpace(v)))
	}
	if v, ok := m["current_state"].(string); ok {
		rec.CurrentState = MemoryState(strings.ToLower(strings.TrimSpace(v)))
	}
	if v, ok := m["action"].(string); ok {
		rec.Action = v
	}
	switch v := m["previous_confidence"].(type) {
	case float64:
		rec.PreviousConfidence = &v
	case int:
		f := float64(v)
		rec.PreviousConfidence = &f
	}
	return rec, ""
}

// ValidateTemporal checks the temporal metadata contract. In strict mode it
// returns every violation as a stable-code string; in non-strict mode it returns
// nil (the accessors surface warnings instead), mirroring package Validate.
//
// Violations covered: invalid memory_state; proposed without finite [0,1]
// confidence or without provenance.evidence_ref; approved/declined carrying
// confidence; relation bounds (updates exactly 1, extends 1-8 unique canonical
// IDs); self-edge, duplicate, path/title, and non-canonical targets; target byte
// bound; unknown nested memory_relation fields.
func ValidateTemporal(c *okf.Concept, strict bool) []string {
	var errs []string

	st, stateWarn := State(c)
	if stateWarn != "" {
		errs = append(errs, stateWarn)
	}

	confVal, confPresent, confWarn := Confidence(c)
	if confWarn != "" {
		errs = append(errs, confWarn)
	}

	if st == MemoryProposed {
		if !confPresent {
			errs = append(errs, "invalid_memory_state: proposed requires memory_confidence")
		} else if math.IsNaN(confVal) || math.IsInf(confVal, 0) || confVal < 0 || confVal > 1 {
			errs = append(errs, fmt.Sprintf("invalid_memory_state: memory_confidence %v must be finite in [0,1]", confVal))
		}
		if !hasEvidenceRef(c) {
			errs = append(errs, "invalid_memory_state: proposed requires at least one provenance.evidence_ref")
		}
	}
	if (st == MemoryApproved || st == MemoryDeclined) && confPresent {
		errs = append(errs, fmt.Sprintf("invalid_memory_state: %s memory must not carry memory_confidence", st))
	}

	rel, relWarn := Relation(c)
	if relWarn != "" {
		errs = append(errs, relWarn)
	}
	errs = append(errs, validateRelationShape(c, rel)...)

	if !strict {
		return nil
	}
	return errs
}

// validateRelationShape enforces per-kind target counts, canonical IDs, the
// self-edge and duplicate rules, and the target byte bound.
func validateRelationShape(c *okf.Concept, rel MemoryRelation) []string {
	if rel.Kind == "" {
		return nil // a malformed/unknown kind is already reported by Relation()
	}
	var errs []string
	selfID := identity.FromConcept(c).ID
	seen := make(map[string]bool)

	for i, t := range rel.Targets {
		if len(t) > maxRelationTargetBytes {
			errs = append(errs, fmt.Sprintf("invalid_memory_relation: target %d is %d bytes, exceeds %d", i, len(t), maxRelationTargetBytes))
		}
		if _, err := identity.Parse(t); err != nil {
			errs = append(errs, fmt.Sprintf("invalid_memory_relation: target %q is not a canonical okf id", t))
			continue
		}
		if t == selfID {
			errs = append(errs, fmt.Sprintf("invalid_memory_relation: self-edge to %q", t))
		}
		if seen[t] {
			errs = append(errs, fmt.Sprintf("invalid_memory_relation: duplicate target %q", t))
		}
		seen[t] = true
	}

	switch rel.Kind {
	case RelationUpdates:
		if len(rel.Targets) != 1 {
			errs = append(errs, fmt.Sprintf("invalid_memory_relation: updates requires exactly 1 target, got %d", len(rel.Targets)))
		}
	case RelationExtends:
		if len(rel.Targets) < 1 || len(rel.Targets) > 8 {
			errs = append(errs, fmt.Sprintf("invalid_memory_relation: extends requires 1-8 targets, got %d", len(rel.Targets)))
		}
	}
	return errs
}

// hasEvidenceRef reports whether provenance.evidence_refs is a non-empty list.
func hasEvidenceRef(c *okf.Concept) bool {
	prov, ok := c.CustomFields["provenance"].(map[string]any)
	if !ok {
		return false
	}
	raw, ok := prov["evidence_refs"]
	if !ok {
		return false
	}
	list, ok := toStringAnySlice(raw)
	return ok && len(list) > 0
}

// normalizeTarget strips the canonical okf://concept/ URI prefix and trims.
func normalizeTarget(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, identity.URIScheme)
	return ref
}

// toStringAnySlice normalizes []any and []string into []any.
func toStringAnySlice(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}
