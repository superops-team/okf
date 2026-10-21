// Package memorydefense provides write-time sensitive-information scanning for
// durable OKF concepts. It is a pure-Go, local-first, deterministic regex
// catalog. No LLM, no network, no second fact source.
package memorydefense

import (
	"regexp"
	"strings"
)

// Severity levels. A blocked write only triggers on SeverityHigh; redact
// replaces every hit regardless of severity.
const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
)

// Detector is one sensitive-pattern rule.
type Detector struct {
	ID       string
	Label    string
	Severity string
	Regex    *regexp.Regexp
	// Verify, when non-nil, post-filters a regex match. Used for patterns
	// that cannot be expressed as pure regex (e.g. credit-card Luhn check).
	Verify func(candidate string) bool
}

// luhnValid reports whether the digit string passes the Luhn algorithm.
func luhnValid(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// creditCardVerify strips spaces/dashes and requires a Luhn-valid number of
// length 13, 15, or 16.
func creditCardVerify(candidate string) bool {
	digits := strings.NewReplacer(" ", "", "-", "").Replace(candidate)
	if n := len(digits); n != 13 && n != 15 && n != 16 {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return luhnValid(digits)
}

// Match reports whether content contains a valid hit for this detector,
// applying the post-filter (Verify) when present.
func (d Detector) Match(content string) bool {
	locs := d.Regex.FindAllStringIndex(content, -1)
	for _, loc := range locs {
		if d.Verify == nil || d.Verify(content[loc[0]:loc[1]]) {
			return true
		}
	}
	return false
}

// Catalog returns the built-in 16-detector catalog. The list order is the
// scan order; more specific patterns run first.
func Catalog() []Detector {
	return []Detector{
		{
			ID:       "github_pat",
			Label:    "github_pat",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
		},
		{
			ID:       "github_oauth",
			Label:    "github_oauth",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`),
		},
		{
			ID:       "aws_access_key",
			Label:    "aws_access_key",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		},
		{
			ID:       "openai_key",
			Label:    "openai_key",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`sk-(proj|srv)?-[A-Za-z0-9_\-]{32,}`),
		},
		{
			ID:       "anthropic_key",
			Label:    "anthropic_key",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`sk-ant-api03-[A-Za-z0-9_\-]{40,}`),
		},
		{
			ID:       "jwt",
			Label:    "jwt",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
		},
		{
			ID:       "pem_private_key",
			Label:    "pem_private_key",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |PGP )?PRIVATE KEY(?: BLOCK)?-----`),
		},
		{
			ID:       "postgres_dsn",
			Label:    "postgres_dsn",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`postgres(?:ql)?://[^\s:/]+:[^\s@]+@[^\s:/]+:\d+/\S+`),
		},
		{
			ID:       "mysql_dsn",
			Label:    "mysql_dsn",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`mysql://[^\s:/]+:[^\s@]+@tcp\([^)]+\)/\S+`),
		},
		{
			ID:       "slack_bot_token",
			Label:    "slack_bot_token",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`xox[bp]-[0-9A-Za-z-]{10,}`),
		},
		{
			ID:       "slack_webhook",
			Label:    "slack_webhook",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Za-z0-9]+/B[A-Za-z0-9]+/[A-Za-z0-9]+`),
		},
		{
			ID:       "stripe_live_key",
			Label:    "stripe_live_key",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(strings.Join([]string{"sk", "live", "[A-Za-z0-9]{24,}"}, "_")),
		},
		{
			ID:       "google_api_key",
			Label:    "google_api_key",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`),
		},
		{
			ID:       "generic_long_token",
			Label:    "generic_long_token",
			Severity: SeverityHigh,
			Regex:    regexp.MustCompile(`(?:api[_-]?key|api[_-]?secret|access[_-]?token)["'=\s:]+[A-Za-z0-9_\-]{32,}`),
		},
		{
			ID:       "credit_card",
			Label:    "credit_card",
			Severity: SeverityMedium,
			Regex:    regexp.MustCompile(`\b(?:\d[ -]?){13,16}\b`),
			Verify:   creditCardVerify,
		},
		{
			ID:       "ssn",
			Label:    "ssn",
			Severity: SeverityMedium,
			Regex:    regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		},
	}
}
