// Package lint provides specification compliance checking for OKF concepts.
package lint

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Concept represents the minimal concept interface needed for linting.
type Concept struct {
	Type        string
	Title       string
	Description string
	Resource    string
	Tags        []string
	Timestamp   string
	Content     string
	FilePath    string
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
		Description: "Required field 'title' must not be empty",
		Severity:    Error,
		Check: func(c *Concept, cfg *Config) []Issue {
			if strings.TrimSpace(c.Title) == "" {
				return []Issue{{Code: "OKF002", Severity: Error, Message: "'title' field is required and must not be empty", Suggestion: "Provide a concise title", FilePath: c.FilePath, Line: 1}}
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
		Description: "Type should use lowercase alphanumeric",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			matched, _ := regexp.MatchString(`^[a-z][a-z0-9_]*$`, c.Type)
			if c.Type != "" && !matched {
				return []Issue{{Code: "OKF004", Severity: Warning, Message: fmt.Sprintf("'type' '%s' should use lowercase", c.Type), Suggestion: "Use lowercase letters, digits, and underscores", FilePath: c.FilePath, Line: 1}}
			}
			return nil
		},
	},
	{
		Code:        "OKF005",
		Description: "Timestamp should be in ISO 8601 format",
		Severity:    Warning,
		Check: func(c *Concept, cfg *Config) []Issue {
			if c.Timestamp == "" {
				return []Issue{{Code: "OKF005", Severity: Warning, Message: "'timestamp' is recommended but missing", Suggestion: "Set timestamp in ISO 8601 format", FilePath: c.FilePath, Line: 6}}
			}
			valid := false
			for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
				if _, err := time.Parse(f, c.Timestamp); err == nil {
					valid = true
					break
				}
			}
			if c.Timestamp != "" && !valid {
				return []Issue{{Code: "OKF005", Severity: Warning, Message: fmt.Sprintf("'timestamp' '%s' is not valid ISO 8601", c.Timestamp), Suggestion: "Use format: 2024-01-15T10:30:00Z", FilePath: c.FilePath, Line: 6}}
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
