// Package memorymeta provides typed accessors for governance and code_refs
// extension fields stored in Concept.CustomFields. The core Concept struct
// is NOT modified; all extension data lives in CustomFields (OKF v0.2
// extension mechanism, spec §7 open family).
package memorymeta

import (
	"fmt"
	"strings"

	"github.com/superops-team/okf/pkg/okf"
)

// GovernanceLevel is the effective governance of a concept.
type GovernanceLevel string

const (
	// GovernanceConstraint is a mandatory guardrail; agent must adhere.
	GovernanceConstraint GovernanceLevel = "constraint"
	// GovernanceHold is an execution freeze; agent SHOULD request user confirmation.
	GovernanceHold GovernanceLevel = "hold"
	// GovernanceContext is informative domain knowledge (default).
	GovernanceContext GovernanceLevel = "context"
)

// validGovernance maps normalized values to their canonical form.
var validGovernance = map[string]GovernanceLevel{
	"constraint": GovernanceConstraint,
	"hold":       GovernanceHold,
	"context":    GovernanceContext,
}

// Governance reads and normalizes the governance field from CustomFields.
// Default (missing/empty) is context. Unknown values return context + warning.
func Governance(c *okf.Concept) (GovernanceLevel, string) {
	if c == nil || c.CustomFields == nil {
		return GovernanceContext, ""
	}
	raw, ok := c.CustomFields["governance"]
	if !ok {
		return GovernanceContext, ""
	}
	s, ok := raw.(string)
	if !ok {
		return GovernanceContext, fmt.Sprintf("governance field is not a string: %T", raw)
	}
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return GovernanceContext, ""
	}
	if level, ok := validGovernance[s]; ok {
		return level, ""
	}
	return GovernanceContext, fmt.Sprintf("unknown governance value %q; treating as context", s)
}

// SetGovernance writes governance to CustomFields. Initializes map if nil.
func SetGovernance(c *okf.Concept, level GovernanceLevel) {
	if c == nil {
		return
	}
	if c.CustomFields == nil {
		c.CustomFields = make(map[string]any)
	}
	c.CustomFields["governance"] = string(level)
}
