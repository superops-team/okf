package memorydefense

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogSizeFloor(t *testing.T) {
	cat := Catalog()
	if len(cat) < 16 {
		t.Fatalf("catalog size = %d, want >= 16", len(cat))
	}
	for _, d := range cat {
		if d.ID == "" || d.Label == "" || d.Severity == "" {
			t.Fatalf("detector missing field: %+v", d)
		}
		if d.Severity != "high" && d.Severity != "medium" {
			t.Fatalf("detector %q severity = %q, want high|medium", d.ID, d.Severity)
		}
		if d.Regex == nil {
			t.Fatalf("detector %q regex not compiled", d.ID)
		}
	}
}

func TestDetectorPositiveHits(t *testing.T) {
	cases := []struct {
		id     string
		sample string
	}{
		{"github_pat", "token=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234"},
		{"github_oauth", "token=gho_ABCDEFGHIJKLMNOPQRSTUVWXYZabcd1234567890"},
		{"aws_access_key", "AKIAIOSFODNN7EXAMPLE"},
		{"openai_key", "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234567890"},
		{"anthropic_key", "sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234567890abcdEFGH"},
		{"jwt", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTYifQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
		{"pem_private_key", "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhk...\n-----END PRIVATE KEY-----"},
		{"postgres_dsn", "postgresql://user:pass@localhost:5432/mydb"},
		{"mysql_dsn", "mysql://user:pass@tcp(localhost:3306)/mydb"},
		{"slack_bot_token", "xoxb-INVALID-EXAMPLE-TOKEN-000000000000000000000000"},
		{"slack_webhook", "https://hooks.slack.com/services/T00000000/B00000000/INVALIDEXAMPLETOKEN0000000000"},
		{"stripe_live_key", strings.Join([]string{"sk", "live", ""}, "_") + "INVALIDEXAMPLETOKENPLACEHOLDER0000000000"},
		{"credit_card", "4111-1111-1111-1111"},
		{"ssn", "123-45-6789"},
		{"google_api_key", "AIzaSyA1234567890abcdefghijklmnopqrstuvw"},
		{"generic_long_token", "api_key=abcdef1234567890abcdef1234567890abcdEFGH"},
	}
	cat := Catalog()
	byID := make(map[string]Detector, len(cat))
	for _, d := range cat {
		byID[d.ID] = d
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			d, ok := byID[tc.id]
			if !ok {
				t.Fatalf("detector %q not in catalog", tc.id)
			}
			if !d.Match(tc.sample) {
				t.Fatalf("detector %q did not match positive sample %q", tc.id, tc.sample)
			}
		})
	}
}

func TestNoFalsePositiveOnNormalText(t *testing.T) {
	normal := []string{
		"the project uses github actions to run tests on every push",
		"remember to call the customer tomorrow at 3pm",
		"下载一个 1234567890123456 字节的模型文件到本地缓存",
		"the order number is 1234-5678 and it arrived on tuesday",
		"user prefers morning meetings over evening standups",
		"the build outputs a 41111 file with checksum abcd1234",
		"we use postgresql in development and sqlite for tests",
	}
	cat := Catalog()
	for _, text := range normal {
		for _, d := range cat {
			if d.Match(text) {
				t.Fatalf("detector %q falsely matched normal text %q", d.ID, text)
			}
		}
	}
}

func TestCreditCardRequiresLuhn(t *testing.T) {
	cat := Catalog()
	var cc Detector
	for _, d := range cat {
		if d.ID == "credit_card" {
			cc = d
		}
	}
	if cc.ID == "" {
		t.Skip("credit_card detector not present")
	}
	// Fails Luhn (byte count shape) — must NOT match.
	if cc.Match("1234567890123456") {
		t.Fatal("credit_card matched non-Luhn number (false positive)")
	}
	// Luhn-valid test number — must match.
	if !cc.Match("4111-1111-1111-1111") {
		t.Fatal("credit_card did not match Luhn-valid 4111-1111-1111-1111")
	}
}

func TestScreenRedactReplacesInPlace(t *testing.T) {
	pol := Policy{Enabled: true, Action: "redact"}
	in := "my key is ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234 and nothing else"
	out, hits, err := Screen(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234") {
		t.Fatalf("redacted output still contains original token: %q", out)
	}
	if !strings.Contains(out, "[REDACTED:github_pat]") {
		t.Fatalf("redacted output missing placeholder: %q", out)
	}
	if len(hits) == 0 || hits[0].DetectorID != "github_pat" {
		t.Fatalf("hits = %+v, want github_pat", hits)
	}
}

func TestScreenBlockReturnsError(t *testing.T) {
	pol := Policy{Enabled: true, Action: "block"}
	in := "use AKIAIOSFODNN7EXAMPLE for the s3 access"
	_, _, err := Screen(in, pol)
	if err == nil {
		t.Fatal("expected block error, got nil")
	}
	if !strings.Contains(err.Error(), "memory_defense_blocked") {
		t.Fatalf("error = %q, want memory_defense_blocked", err.Error())
	}
	if strings.Contains(err.Error(), "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal("block error leaked the original secret")
	}
}

func TestScreenEmptyAfterRedaction(t *testing.T) {
	pol := Policy{Enabled: true, Action: "redact"}
	// Entire content is a PEM block.
	in := "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhk...\n-----END PRIVATE KEY-----"
	out, _, err := Screen(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected non-empty redacted output, got empty (caller must reject)")
	}
}

func TestScreenDisabledIsNoop(t *testing.T) {
	pol := Policy{Enabled: false}
	in := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234"
	out, hits, err := Screen(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("disabled screen should be noop: got %q", out)
	}
	if len(hits) != 0 {
		t.Fatalf("disabled screen should report no hits: %+v", hits)
	}
}

func TestLoadPolicyMissingFile(t *testing.T) {
	dir := t.TempDir()
	pol, err := LoadPolicy(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pol.Enabled {
		t.Fatal("missing config should default to disabled")
	}
}

func TestLoadPolicyValid(t *testing.T) {
	dir := t.TempDir()
	content := `memory_defense:
  enabled: true
  action: block
  detectors: [github_pat, aws_access_key]
`
	if err := os.MkdirAll(filepath.Join(dir, ".okf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".okf", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	pol, err := LoadPolicy(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !pol.Enabled || pol.Action != "block" || len(pol.Detectors) != 2 {
		t.Fatalf("policy = %+v", pol)
	}
}

func TestLoadPolicyInvalidAction(t *testing.T) {
	dir := t.TempDir()
	content := "memory_defense:\n  enabled: true\n  action: bogus\n"
	if err := os.MkdirAll(filepath.Join(dir, ".okf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".okf", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(dir); err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestScreenWhitelist(t *testing.T) {
	pol := Policy{Enabled: true, Action: "redact", Detectors: []string{"github_pat"}}
	// AWS key present but whitelist only has github_pat → no hit.
	in := "AKIAIOSFODNN7EXAMPLE"
	out, hits, err := Screen(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("whitelist should not redact non-whitelisted detector: %q", out)
	}
	if len(hits) != 0 {
		t.Fatalf("whitelist should report no hits for non-whitelisted: %+v", hits)
	}
}
