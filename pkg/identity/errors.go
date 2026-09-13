// Package identity owns the canonical, optional stable concept identity
// extension (okf_id) for OKF v0.2 bundles.
//
// okf_id is an OKF implementation extension, not a Google OKF v0.2 required
// field. It lives only in Concept.CustomFields as the string key "okf_id".
// There is no identity catalog, no alias/path-history table, and no mutable
// global registry: callers build a Registry from the concepts in memory and
// read operations never add IDs or modify user files.
//
// Package boundary: identity may depend on pkg/okf but MUST NOT depend on
// pkg/query. Consumers that only need a string-level index key call Key so
// that adapter wiring cannot create an import cycle.
package identity

import "fmt"

// ErrorCode is a stable, machine-readable identity error code. These strings
// are part of the public contract and are mirrored into pkg/tool.
type ErrorCode string

const (
	// CodeInvalidConceptID: an explicit okf_id does not match the canonical grammar.
	CodeInvalidConceptID ErrorCode = "invalid_concept_id"
	// CodeDuplicateConceptID: two concepts claim the same canonical okf_id.
	CodeDuplicateConceptID ErrorCode = "duplicate_concept_id"
	// CodeConceptRefNotFound: a syntactically valid stable ref resolves to no concept.
	CodeConceptRefNotFound ErrorCode = "concept_ref_not_found"
	// CodeIndexRebuildRequired: a v2 (or otherwise incompatible) vector index is
	// present and v3 code refuses to query it.
	CodeIndexRebuildRequired ErrorCode = "index_rebuild_required"
	// CodeIdentityMigrationPartial: a cross-file migration rolled back but at least
	// one file could not be restored to its original bytes.
	CodeIdentityMigrationPartial ErrorCode = "identity_migration_partial"
)

// Error is the typed identity error. It carries a stable Code so callers can
// match it with errors.Is (by code) or errors.As (for paths/message).
type Error struct {
	Code ErrorCode
	msg  string
	// Paths identifies the bundle-relative concepts involved (e.g. both sides of
	// a duplicate, or the files left after a partial rollback).
	Paths []string
	// cause is the optional wrapped underlying error (e.g. an entropy read error).
	cause error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As traversal.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is makes an *Error match any error carrying the same Code, so callers can
// write errors.Is(err, ErrInvalidConceptID) against a concrete instance.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other != nil && e != nil && other.Code == e.Code
}

// Sentinels. Concrete returned errors carry one of these codes plus context.
var (
	ErrInvalidConceptID         = &Error{Code: CodeInvalidConceptID, msg: "invalid concept id"}
	ErrDuplicateConceptID       = &Error{Code: CodeDuplicateConceptID, msg: "duplicate concept id"}
	ErrConceptRefNotFound       = &Error{Code: CodeConceptRefNotFound, msg: "concept ref not found"}
	ErrIndexRebuildRequired     = &Error{Code: CodeIndexRebuildRequired, msg: "vector index rebuild required"}
	ErrIdentityMigrationPartial = &Error{Code: CodeIdentityMigrationPartial, msg: "identity migration partial"}
)

// withPaths returns a copy of e annotated with the given bundle-relative paths.
func (e *Error) withPaths(paths ...string) *Error {
	copied := *e
	copied.Paths = append([]string(nil), paths...)
	return &copied
}

// withMessage returns a copy of e with a richer message, preserving the code.
func (e *Error) withMessage(format string, args ...any) *Error {
	copied := *e
	copied.msg = fmt.Sprintf(format, args...)
	return &copied
}

// withCause returns a copy of e wrapping an underlying error.
func (e *Error) withCause(cause error) *Error {
	copied := *e
	copied.cause = cause
	return &copied
}
