package identity

import (
	cryptorand "crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/superops-team/okf/pkg/okf"
)

// Field is the CustomFields key under which the canonical stable concept ID is stored.
const Field = "okf_id"

// ParentField is the CustomFields key derived chunk concepts use to point back
// at their parent concept's stable okf_id.
const ParentField = "parent_okf_id"

// IDPrefix is the canonical literal prefix of every okf_id.
const IDPrefix = "okf_"

// IDHexLen is the number of lowercase hex digits after the prefix (128 bits).
const IDHexLen = 32

// URIScheme is the canonical stable-ref URI scheme prefix.
const URIScheme = "okf://concept/"

// idPattern enforces ^okf_[0-9a-f]{32}$.
var idPattern = regexp.MustCompile(`^okf_[0-9a-f]{32}$`)

// State describes the identity state of a concept relative to stable identity.
type State string

const (
	// StateStable: the concept carries a valid canonical okf_id.
	StateStable State = "stable"
	// StateLegacyUnstable: the concept has no okf_id (valid v0.2, identity moves with rename).
	StateLegacyUnstable State = "legacy-unstable"
	// StateInvalid: the concept carries an okf_id that fails the canonical grammar.
	StateInvalid State = "invalid"
)

// Ref is the resolved stable identity of one concept file.
type Ref struct {
	ID       string
	URI      string
	State    State
	FilePath string
}

// randomReader is the entropy source used by New. Tests swap it to inject
// deterministic or failing readers; production always uses crypto/rand.Reader.
var randomReader io.Reader = cryptorand.Reader

// Parse validates value against the canonical grammar and returns the bare ID
// (it is already just the ID). An invalid value returns ErrInvalidConceptID.
func Parse(value string) (string, error) {
	if !idPattern.MatchString(value) {
		return "", ErrInvalidConceptID.withMessage("invalid_concept_id: %q must match ^okf_[0-9a-f]{%d}$", value, IDHexLen)
	}
	return value, nil
}

// New generates a fresh canonical 128-bit random ID. It consumes from the
// production crypto/rand reader and NEVER falls back to a deterministic or
// weak value: a read failure is returned as an error.
func New() (string, error) {
	return newID(randomReader)
}

// newID reads exactly 16 random bytes from r and hex-encodes them. Exposed for
// entropy-failure tests via the package-level randomReader swap.
func newID(r io.Reader) (string, error) {
	buf := make([]byte, IDHexLen/2) // 16 bytes = 128 bits
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", ErrInvalidConceptID.
			withMessage("generate okf_id: read entropy: %v", err).
			withCause(err)
	}
	return IDPrefix + hex.EncodeToString(buf), nil
}

// CanonicalURI builds the stable URI for a bare ID (already grammar-checked).
func CanonicalURI(id string) string {
	return URIScheme + id
}

// FromConcept reads the okf_id extension from a concept and classifies its state.
// It never mutates the concept.
func FromConcept(c *okf.Concept) Ref {
	if c == nil {
		return Ref{State: StateInvalid}
	}
	ref := Ref{FilePath: c.FilePath}
	raw := stringCustomField(c, Field)
	if raw == "" {
		ref.State = StateLegacyUnstable
		return ref
	}
	if id, err := Parse(raw); err == nil {
		ref.ID = id
		ref.URI = CanonicalURI(id)
		ref.State = StateStable
		return ref
	}
	ref.State = StateInvalid
	return ref
}

// Resolve resolves a stable ref (bare ID or okf://concept/<id> URI) against the
// given concepts. A syntactically invalid ref fails with ErrInvalidConceptID;
// a valid ref with no match fails with ErrConceptRefNotFound.
func Resolve(concepts []*okf.Concept, ref string) (*okf.Concept, error) {
	registry, err := BuildRegistry(concepts)
	if err != nil {
		return nil, err
	}
	return registry.Resolve(ref)
}

// Registry is an in-memory, immutable index of stable okf_id -> concept built
// once from a concept set. There is no mutable global state.
type Registry struct {
	byID map[string]*okf.Concept
}

// BuildRegistry validates every explicit okf_id in concepts and returns a
// resolver. Invalid explicit IDs and duplicates fail closed before any lookup.
func BuildRegistry(concepts []*okf.Concept) (*Registry, error) {
	byID := make(map[string]*okf.Concept)
	for _, c := range concepts {
		if c == nil {
			continue
		}
		raw := stringCustomField(c, Field)
		if raw == "" {
			continue // legacy-unstable: legal, not indexed
		}
		id, err := Parse(raw)
		if err != nil {
			var identErr *Error
			if errors.As(err, &identErr) {
				return nil, identErr.withMessage("%s (concept %s)", identErr.msg, c.FilePath)
			}
			return nil, err
		}
		if existing, dup := byID[id]; dup {
			return nil, ErrDuplicateConceptID.withPaths(existing.FilePath, c.FilePath).
				withMessage("duplicate_concept_id: okf_id %s is claimed by %q and %q", id, existing.FilePath, c.FilePath)
		}
		byID[id] = c
	}
	return &Registry{byID: byID}, nil
}

// Resolve resolves a bare ID or okf://concept/<id> URI against the registry.
func (r *Registry) Resolve(ref string) (*okf.Concept, error) {
	id, err := normalizeRef(ref)
	if err != nil {
		return nil, err
	}
	if c, ok := r.byID[id]; ok {
		return c, nil
	}
	return nil, ErrConceptRefNotFound.withMessage("concept_ref_not_found: no concept has stable id %q", id)
}

// normalizeRef strips the canonical URI prefix and validates the bare ID.
func normalizeRef(ref string) (string, error) {
	id := strings.TrimSpace(ref)
	id = strings.TrimPrefix(id, URIScheme)
	return Parse(id)
}

// Key builds the identity-aware vector index key for a concept.
//
// A valid stable okf_id maps to "v3:id:<id>"; a legacy concept falls back to
// "v3:legacy:<legacyFingerprint>". This is a pure string-level helper so the
// vector index (pkg/query) can key concepts without importing the concept
// model or creating an import cycle.
//
// The silent fallback for a non-empty but grammar-invalid id is intentional:
// invalid explicit IDs are rejected at load time by BuildRegistry and by the
// Manifest validator, so by the time a concept reaches Key construction it has
// already passed validation. A plain "" (no id) is the normal legacy case and
// is expected to fall through to the legacy key; a malformed id here can only
// come from a code path that skipped registry validation, where treating it as
// legacy is the safe non-panicking default rather than crashing indexing.
func Key(id, legacyFingerprint string) string {
	if _, err := Parse(id); err == nil {
		return "v3:id:" + id
	}
	return "v3:legacy:" + legacyFingerprint
}

// ChunkKey appends a chunk ordinal to a concept key without losing it.
func ChunkKey(conceptKey string, ordinal int) string {
	return conceptKey + "#" + strconv.Itoa(ordinal)
}

// SecureEqual constant-time compares two already-validated IDs (defensive; IDs
// are random anyway).
func SecureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func stringCustomField(c *okf.Concept, key string) string {
	if c == nil || c.CustomFields == nil {
		return ""
	}
	v, ok := c.CustomFields[key].(string)
	if !ok {
		return ""
	}
	return v
}
