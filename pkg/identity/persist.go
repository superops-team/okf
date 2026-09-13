package identity

import (
	"os"

	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

// This file implements the shared final-destination identity helper that every
// OKF-owned Concept writer calls immediately before persistence.
//
// Rules (design §3.3):
//   - temporary conversion / staging files MUST NOT receive a random ID;
//   - at the final target, a writer preserves an existing valid okf_id and only
//     generates a new one for a genuinely new owned target;
//   - derived chunks persist their parent's okf_id under parent_okf_id;
//   - durable capture keeps its own deterministic concept_id untouched.

// EnsureFinalID ensures c carries a canonical okf_id for its final destination.
//
// Resolution order:
//  1. a valid okf_id already present on c.CustomFields is preserved;
//  2. otherwise, a valid okf_id already on disk at diskPath (a refresh or
//     re-import of the same owned target) is adopted into c;
//  3. otherwise a fresh random id is generated and assigned.
//
// diskPath may be empty for an in-memory concept with no on-disk counterpart.
// It returns the resulting id.
func EnsureFinalID(c *okf.Concept, diskPath string) (string, error) {
	if c == nil {
		return "", ErrInvalidConceptID.withMessage("cannot assign id to nil concept")
	}
	if existing := stringCustomField(c, Field); existing != "" {
		if id, err := Parse(existing); err == nil {
			return id, nil
		}
		// An explicit invalid id is preserved (not silently rewritten) so that a
		// later registry/manifest call fails closed with invalid_concept_id.
		return "", ErrInvalidConceptID.withMessage("concept %s carries invalid okf_id %q", c.FilePath, existing)
	}
	if diskPath != "" {
		if id, ok := readExistingID(diskPath); ok {
			setCustomField(c, Field, id)
			return id, nil
		}
	}
	id, err := New()
	if err != nil {
		return "", err
	}
	setCustomField(c, Field, id)
	return id, nil
}

// AssignParentID records the parent concept's okf_id on a derived chunk concept.
func AssignParentID(chunk *okf.Concept, parentID string) {
	if chunk == nil {
		return
	}
	if _, err := Parse(parentID); err != nil {
		return
	}
	if chunk.CustomFields == nil {
		chunk.CustomFields = map[string]any{}
	}
	chunk.CustomFields[ParentField] = parentID
}

// readExistingID parses the file at path and returns a valid okf_id if it
// already carries one. A missing file, parse error, or absent/invalid id
// reports (ok=false) so the caller falls through to generation.
func readExistingID(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	pc, err := parser.ParseConcept(path)
	if err != nil {
		return "", false
	}
	raw, _ := pc.CustomFields[Field].(string)
	if id, err := Parse(raw); err == nil {
		return id, true
	}
	return "", false
}

func setCustomField(c *okf.Concept, key, value string) {
	if c.CustomFields == nil {
		c.CustomFields = map[string]any{}
	}
	c.CustomFields[key] = value
}
