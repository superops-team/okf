// Package lint provides specification compliance checking for OKF concepts.
package lint

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Concept represents the minimal concept interface needed for linting.
// v0.2: extended with all v0.2 fields for spec-aligned linting.
type Concept struct {
	Type        string
	Title       string
	Description string
	Resource    string
	Tags        []string
	Timestamp   string // legacy v0.1 field
	Content     string
	FilePath    string
	// v0.2 fields
	GeneratedBy string // generated.by
	GeneratedAt string // generated.at
	HasSources  bool
	HasVerified bool
	Status      string
	StaleAfter  string
	Runtime     string // for Attested Computation
}

// Severity represents lint warning severity levels.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "INFO"
	case Warning:
		return "WARNING"
	case Error:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Issue represents a single lint check result.
type Issue struct {
	FilePath   string
	Line       int
	Severity   Severity
	Code       string
	Message    string
	Suggestion string
}

func (i Issue) String() string {
	loc := i.FilePath
	if i.Line > 0 {
		loc = fmt.Sprintf("%s:%d", i.FilePath, i.Line)
	}
	return fmt.Sprintf("[%s] %s %s - %s", i.Severity, loc, i.Code, i.Message)
}

// Config contains lint configuration.
type Config struct {
	MaxLineLength        int
	MinDescriptionLength int
	RequiredTags         []string
	StrictMode           bool
}

// DefaultConfig returns the default lint configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxLineLength:        240,
		MinDescriptionLength: 10,
	}
}

// Result contains the complete lint result.
type Result struct {
	ConceptsChecked int
	Issues          []Issue
	Errors          int
	Warnings        int
	Infos           int
	Duration        time.Duration
}

// HasErrors returns true if there are any errors.
func (r *Result) HasErrors() bool {
	return r.Errors > 0
}

// Summary returns a human-readable summary.
func (r *Result) Summary() string {
	return fmt.Sprintf("Checked %d concepts: %d errors, %d warnings, %d infos (took %v)",
		r.ConceptsChecked, r.Errors, r.Warnings, r.Infos, r.Duration)
}

// Rule defines a single lint rule.
type Rule struct {
	Code        string
	Description string
	Severity    Severity
	Check       func(*Concept, *Config) []Issue
}

var rules = []Rule{
	{
		Code:        "OKF001",
		Description: "Required field 'type' must not be empty",
		Severity:    Error,
		Check: func(c *Concept, cfg *Config) []Issue {
			if strings.TrimSpace(c.Type) == "" {
				return []Issue{{Code: "OKF001", Severity: Error, Message: "'type' field is required and must not be empty", Suggestion: "Set type to one of: table, api, metric, concept, component, project, system, service", FilePath: c.FilePath, Line: 1}}
			}
			return nil
		},
	},
	{
		Code:        "OKF002",
		Description: "'title' is recommended but optional (v0.2)",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			if strings.TrimSpace(c.Title) == "" {
				return []Issue{{Code: "OKF002", Severity: Warning, Message: "'title' is recommended but missing (will be derived from filename)", Suggestion: "Provide a concise title for better discoverability", FilePath: c.FilePath, Line: 1}}
			}
			return nil
		},
	},
	{
		Code:        "OKF003",
		Description: "'description' should provide meaningful context",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			if len(strings.TrimSpace(c.Description)) < cfg.MinDescriptionLength {
				return []Issue{{Code: "OKF003", Severity: Warning, Message: fmt.Sprintf("'description' should be at least %d characters", cfg.MinDescriptionLength), Suggestion: "Add a brief description", FilePath: c.FilePath, Line: 3}}
			}
			return nil
		},
	},
	{
		Code:        "OKF004",
		Description: "Type naming convention (informational, v0.2 allows mixed-case types)",
		Severity:    Info,
		Check: func(c *Concept, cfg *Config) []Issue {
			matched, _ := regexp.MatchString(`^[a-z][a-z0-9_]*$`, c.Type)
			if c.Type != "" && !matched {
				return []Issue{{Code: "OKF004", Severity: Info, Message: fmt.Sprintf("'type' '%s' uses mixed case (allowed in v0.2, e.g. 'Attested Computation')", c.Type), Suggestion: "Lowercase is recommended for simple types; mixed-case is valid for spec-defined types", FilePath: c.FilePath, Line: 1}}
			}
			return nil
		},
	},
	{
		Code:        "OKF005",
		Description: "'generated.at' should be in ISO 8601 format (v0.2; legacy timestamp still accepted)",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			// v0.2: prefer generated.at, fall back to legacy timestamp
			ts := c.GeneratedAt
			fieldName := "generated.at"
			if ts == "" {
				ts = c.Timestamp
				fieldName = "timestamp"
			}
			if ts == "" {
				return []Issue{{Code: "OKF005", Severity: Warning, Message: "'generated.at' is recommended but missing", Suggestion: "Set generated.at in ISO 8601 format, e.g. 2024-01-15T10:30:00Z", FilePath: c.FilePath, Line: 6}}
			}
			valid := false
			for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
				if _, err := time.Parse(f, ts); err == nil {
					valid = true
					break
				}
			}
			if !valid {
				return []Issue{{Code: "OKF005", Severity: Warning, Message: fmt.Sprintf("'%s' '%s' is not valid ISO 8601", fieldName, ts), Suggestion: "Use format: 2024-01-15T10:30:00Z", FilePath: c.FilePath, Line: 6}}
			}
			return nil
		},
	},
	{
		Code:        "OKF006",
		Description: "Tags should be lowercase",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			var issues []Issue
			for _, tag := range c.Tags {
				if strings.ToLower(tag) != tag {
					issues = append(issues, Issue{Code: "OKF006", Severity: Warning, Message: fmt.Sprintf("Tag '%s' should be lowercase", tag), Suggestion: "Use lowercase tags", FilePath: c.FilePath, Line: 5})
				}
				if strings.Contains(tag, " ") {
					issues = append(issues, Issue{Code: "OKF006", Severity: Warning, Message: fmt.Sprintf("Tag '%s' should not contain spaces", tag), Suggestion: "Use hyphens or underscores", FilePath: c.FilePath, Line: 5})
				}
			}
			return issues
		},
	},
	{
		Code:        "OKF007",
		Description: "Content body should not be empty",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			if len(strings.TrimSpace(c.Content)) == 0 {
				return []Issue{{Code: "OKF007", Severity: Warning, Message: "Content body is empty", Suggestion: "Add markdown content", FilePath: c.FilePath, Line: 8}}
			}
			return nil
		},
	},
	{
		Code:        "OKF009",
		Description: "Long lines in content",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			var issues []Issue
			lines := strings.Split(c.Content, "\n")
			for i, line := range lines {
				if len(line) > cfg.MaxLineLength {
					issues = append(issues, Issue{Code: "OKF009", Severity: Warning, Message: fmt.Sprintf("Line %d exceeds %d chars", i+1, cfg.MaxLineLength), Suggestion: "Wrap lines for readability", FilePath: c.FilePath, Line: 8 + i})
					if len(issues) > 5 {
						break
					}
				}
			}
			return issues
		},
	},
	{
		Code:        "OKF010",
		Description: "Duplicate tags",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			seen := make(map[string]bool)
			for _, tag := range c.Tags {
				if seen[tag] {
					return []Issue{{Code: "OKF010", Severity: Warning, Message: fmt.Sprintf("Duplicate tag '%s'", tag), Suggestion: "Remove duplicate tags", FilePath: c.FilePath, Line: 5}}
				}
				seen[tag] = true
			}
			return nil
		},
	},
	{
		Code:        "OKF011",
		Description: "Required tags must be present",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			if len(cfg.RequiredTags) == 0 {
				return nil
			}
			tagSet := make(map[string]bool)
			for _, tag := range c.Tags {
				tagSet[tag] = true
			}
			var issues []Issue
			for _, requiredTag := range cfg.RequiredTags {
				if !tagSet[requiredTag] {
					issues = append(issues, Issue{Code: "OKF011", Severity: Warning, Message: fmt.Sprintf("Required tag '%s' is missing", requiredTag), Suggestion: "Add the required tag", FilePath: c.FilePath, Line: 5})
				}
			}
			return issues
		},
	},
	// === v0.2 rules ===
	{
		Code:        "OKF012",
		Description: "'sources' is recommended for provenance (v0.2 §5.1)",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			if !c.HasSources {
				return []Issue{{Code: "OKF012", Severity: Warning, Message: "'sources' is recommended but missing", Suggestion: "Add sources to record provenance and credibility signals", FilePath: c.FilePath, Line: 7}}
			}
			return nil
		},
	},
	{
		Code:        "OKF014",
		Description: "Attested Computation requires 'runtime' field (v0.2 §10.2)",
		Severity:    Error,
		Check: func(c *Concept, cfg *Config) []Issue {
			if c.Type == "Attested Computation" && c.Runtime == "" {
				return []Issue{{Code: "OKF014", Severity: Error, Message: "Attested Computation requires 'runtime' field", Suggestion: "Set runtime to the computation runtime identifier (e.g. bigquery, dbt, python)", FilePath: c.FilePath, Line: 1}}
			}
			return nil
		},
	},
	{
		Code:        "OKF015",
		Description: "'stale_after' should be valid ISO date YYYY-MM-DD (v0.2 §6.2)",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			if c.StaleAfter == "" {
				return nil
			}
			if _, err := time.Parse("2006-01-02", c.StaleAfter); err != nil {
				return []Issue{{Code: "OKF015", Severity: Warning, Message: fmt.Sprintf("'stale_after' '%s' is not valid YYYY-MM-DD", c.StaleAfter), Suggestion: "Use format: 2026-12-31", FilePath: c.FilePath, Line: 9}}
			}
			return nil
		},
	},
	{
		Code:        "OKF016",
		Description: "Legacy 'timestamp' should migrate to 'generated.at' (v0.2 §13.1)",
		Severity:    Info,
		Check: func(c *Concept, cfg *Config) []Issue {
			if c.Timestamp != "" && c.GeneratedAt == "" {
				return []Issue{{Code: "OKF016", Severity: Info, Message: "Legacy 'timestamp' detected; consider migrating to 'generated.at'", Suggestion: "Replace timestamp with generated: {by: <actor>, at: <ISO8601>}", FilePath: c.FilePath, Line: 6}}
			}
			return nil
		},
	},
	{
		Code:        "OKF017",
		Description: "'verified' is recommended for trust tier elevation (v0.2 §5.2)",
		Severity:    Info,
		Check: func(c *Concept, cfg *Config) []Issue {
			if !c.HasVerified {
				return []Issue{{Code: "OKF017", Severity: Info, Message: "'verified' is recommended to elevate trust tier beyond unverified", Suggestion: "Add verified events with human:<id> or process:<id> actors", FilePath: c.FilePath, Line: 8}}
			}
			return nil
		},
	},
}

// LintConcept checks a single concept.
func LintConcept(c *Concept, cfg *Config) []Issue {
	var allIssues []Issue
	for _, rule := range rules {
		if rule.Check == nil {
			continue
		}
		issues := rule.Check(c, cfg)
		for i := range issues {
			if issues[i].FilePath == "" {
				issues[i].FilePath = c.FilePath
			}
		}
		allIssues = append(allIssues, issues...)
	}
	return allIssues
}

// LintBundle checks a slice of concepts.
func LintBundle(concepts []*Concept, cfg *Config) *Result {
	start := time.Now()

	titleCounts := make(map[string]int)
	for _, c := range concepts {
		titleCounts[c.Title]++
	}

	result := &Result{ConceptsChecked: len(concepts)}

	for _, c := range concepts {
		issues := LintConcept(c, cfg)
		result.Issues = append(result.Issues, issues...)

		if titleCounts[c.Title] > 1 {
			result.Issues = append(result.Issues, Issue{
				FilePath:   c.FilePath,
				Severity:   Warning,
				Code:       "OKF013",
				Message:    fmt.Sprintf("Duplicate title '%s'", c.Title),
				Suggestion: "Each concept should have a unique title",
			})
		}
	}

	for _, issue := range result.Issues {
		switch issue.Severity {
		case Error:
			result.Errors++
		case Warning:
			result.Warnings++
		case Info:
			result.Infos++
		}
	}

	result.Duration = time.Since(start)
	return result
}
