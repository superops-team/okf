package lint

import "testing"

func TestLintBundleReportsCoreIssues(t *testing.T) {
	t.Parallel()

	concepts := []*Concept{
		{
			Type:        "API",
			Title:       "Duplicate",
			Description: "short",
			Tags:        []string{"Go Lang", "Go Lang"},
			Timestamp:   "not-a-date",
			Content:     "",
			FilePath:    "bad.md",
		},
		{
			Type:        "api",
			Title:       "Duplicate",
			Description: "This concept has enough context",
			Tags:        []string{"go"},
			Timestamp:   "2024-01-15T10:30:00Z",
			Content:     "body",
			FilePath:    "duplicate.md",
		},
	}

	result := LintBundle(concepts, DefaultConfig())

	assertIssueCode(t, result.Issues, "OKF003")
	assertIssueCode(t, result.Issues, "OKF004")
	assertIssueCode(t, result.Issues, "OKF005")
	assertIssueCode(t, result.Issues, "OKF006")
	assertIssueCode(t, result.Issues, "OKF007")
	assertIssueCode(t, result.Issues, "OKF010")
	assertIssueCode(t, result.Issues, "OKF013")
}

func TestLintConceptReportsRequiredTags(t *testing.T) {
	t.Parallel()

	issues := LintConcept(&Concept{
		Type:        "api",
		Title:       "Users",
		Description: "User API endpoint",
		Tags:        []string{"go"},
		Timestamp:   "2024-01-15T10:30:00Z",
		Content:     "body",
		FilePath:    "users.md",
	}, &Config{
		MaxLineLength:        240,
		MinDescriptionLength: 10,
		RequiredTags:         []string{"production", "go"},
	})

	assertIssueCode(t, issues, "OKF011")
}

func assertIssueCode(t *testing.T, issues []Issue, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("issues %#v do not contain code %s", issues, code)
}
