package memorymeta

import (
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// S01: Governance field read from CustomFields, normalized lowercase.
func TestGovernanceReadAndNormalize(t *testing.T) {
	tests := []struct {
		name     string
		raw      any
		expected GovernanceLevel
		wantWarn bool
	}{
		{"lowercase constraint", "constraint", GovernanceConstraint, false},
		{"mixed case Constraint", "Constraint", GovernanceConstraint, false},
		{"uppercase HOLD", "HOLD", GovernanceHold, false},
		{"context", "context", GovernanceContext, false},
		{"empty string", "", GovernanceContext, false},
		{"unknown value", "weird", GovernanceContext, true},
		{"non-string", 123, GovernanceContext, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &okf.Concept{CustomFields: map[string]any{}}
			if tt.raw != nil {
				c.CustomFields["governance"] = tt.raw
			}
			got, warn := Governance(c)
			if got != tt.expected {
				t.Errorf("Governance() = %q, want %q", got, tt.expected)
			}
			if (warn != "") != tt.wantWarn {
				t.Errorf("warn = %q, wantWarn=%v", warn, tt.wantWarn)
			}
		})
	}
}

// S02: Default governance is context for all concepts (no path inference).
func TestGovernanceDefaultContext(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{}}
	got, warn := Governance(c)
	if got != GovernanceContext {
		t.Errorf("default = %q, want context", got)
	}
	if warn != "" {
		t.Errorf("default should have no warning, got %q", warn)
	}
	// nil CustomFields also defaults to context
	c2 := &okf.Concept{}
	got2, _ := Governance(c2)
	if got2 != GovernanceContext {
		t.Errorf("nil CustomFields default = %q, want context", got2)
	}
}

// S03: Explicit governance takes effect.
func TestGovernanceExplicit(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{"governance": "hold"}}
	got, _ := Governance(c)
	if got != GovernanceHold {
		t.Errorf("explicit hold = %q, want hold", got)
	}
}

// S04: Unknown value handling — non-strict returns context+warning.
func TestGovernanceUnknownNonStrict(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{"governance": "review"}}
	got, warn := Governance(c)
	if got != GovernanceContext {
		t.Errorf("unknown = %q, want context", got)
	}
	if warn == "" {
		t.Error("unknown should produce warning")
	}
}

// S09: SetGovernance writes to CustomFields.
func TestSetGovernance(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{}}
	SetGovernance(c, GovernanceConstraint)
	if c.CustomFields["governance"] != "constraint" {
		t.Errorf("CustomFields[governance] = %v, want constraint", c.CustomFields["governance"])
	}
	// Round-trip
	got, _ := Governance(c)
	if got != GovernanceConstraint {
		t.Errorf("round-trip = %q, want constraint", got)
	}
	// Set on nil CustomFields initializes map
	c2 := &okf.Concept{}
	SetGovernance(c2, GovernanceHold)
	if c2.CustomFields["governance"] != "hold" {
		t.Errorf("nil CustomFields set failed")
	}
}

// S04: Validate strict mode rejects unknown governance.
func TestValidateGovernanceStrict(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{"governance": "invalid"}}
	errs := Validate(c, true)
	if len(errs) == 0 {
		t.Error("strict mode should reject unknown governance")
	}
}

func TestValidateGovernanceNonStrict(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{"governance": "invalid"}}
	errs := Validate(c, false)
	if len(errs) != 0 {
		t.Errorf("non-strict should not error on unknown governance, got %v", errs)
	}
}

func TestValidateValidGovernance(t *testing.T) {
	c := &okf.Concept{CustomFields: map[string]any{"governance": "constraint"}}
	errs := Validate(c, true)
	if len(errs) != 0 {
		t.Errorf("valid governance should not error, got %v", errs)
	}
}
