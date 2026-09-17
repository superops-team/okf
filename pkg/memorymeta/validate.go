package memorymeta

import (
	"github.com/superops-team/okf/pkg/okf"
)

// Validate checks governance and code_refs fields on a concept.
// In strict mode, unknown governance values and invalid code_refs are errors.
// In non-strict mode, they are warnings (returned as empty error list but
// the accessor functions return warnings).
func Validate(c *okf.Concept, strict bool) []string {
	var errs []string

	// Governance validation
	_, warn := Governance(c)
	if strict && warn != "" {
		errs = append(errs, warn)
	}

	// code_refs validation
	_, codeWarns := CodeRefs(c)
	if strict {
		errs = append(errs, codeWarns...)
	}

	return errs
}
