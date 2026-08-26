package parser

import (
	"strings"
	"testing"
)

// --- v0.2 field parsing tests ---

func TestParseV02_FullFields(t *testing.T) {
	content := `---
type: Attested Computation
title: Revenue for fiscal year
description: Recognized revenue.
tags: [finance, revenue]
status: stable
runtime: bigquery
stale_after: 2026-12-31
generated: { by: reference_agent/gemini-2.5-pro, at: 2026-06-28T14:00:00Z }
verified:
 - { by: human:ahormati, at: 2026-06-25T09:00:00Z }
sources:
 - id: rev-policy
   resource: https://wiki.acme.com/finance/revenue
   title: Revenue recognition policy
   author: team:finance-fpa
   usage_count: 5000
   last_modified: 2026-04-02
usage_window: { from: 2026-06-01, to: 2026-06-30 }
parameters:
 - { name: year, type: integer, required: true }
executor:
 resource: references/skills/run-on-bq.md
 receipt: [job_id, executed_sql, result]
attester:
 resource: references/attesters/sql-equality.py
---

# Computation

` + "```sql" + `
SELECT 1
` + "```" + `
`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Type != "Attested Computation" {
		t.Errorf("type = %q", c.Type)
	}
	if c.Runtime != "bigquery" {
		t.Errorf("runtime = %q", c.Runtime)
	}
	if c.Status != "stable" {
		t.Errorf("status = %q", c.Status)
	}
	if c.StaleAfter != "2026-12-31" {
		t.Errorf("stale_after = %q", c.StaleAfter)
	}
	if c.Generated == nil || c.Generated.By != "reference_agent/gemini-2.5-pro" {
		t.Errorf("generated = %+v", c.Generated)
	}
	if len(c.Verified) != 1 || c.Verified[0].By != "human:ahormati" {
		t.Errorf("verified = %+v", c.Verified)
	}
	if len(c.Sources) != 1 || c.Sources[0].ID != "rev-policy" {
		t.Errorf("sources = %+v", c.Sources)
	}
	if c.Sources[0].UsageCount != 5000 {
		t.Errorf("usage_count = %d", c.Sources[0].UsageCount)
	}
	if c.UsageWindow == nil || c.UsageWindow.From != "2026-06-01" {
		t.Errorf("usage_window = %+v", c.UsageWindow)
	}
	if len(c.Parameters) != 1 || c.Parameters[0].Name != "year" {
		t.Errorf("parameters = %+v", c.Parameters)
	}
	if c.Executor == nil || c.Executor.Resource != "references/skills/run-on-bq.md" {
		t.Errorf("executor = %+v", c.Executor)
	}
	if len(c.Executor.Receipt) != 3 {
		t.Errorf("executor.receipt = %+v", c.Executor.Receipt)
	}
	if c.Attester == nil || c.Attester.Resource != "references/attesters/sql-equality.py" {
		t.Errorf("attester = %+v", c.Attester)
	}
}

// --- verified flexible parsing tests (spec §5.2) ---

func TestParseVerified_List(t *testing.T) {
	content := `---
type: Metric
verified:
 - { by: human:alice, at: "2026-06-25T09:00:00Z" }
 - { by: process:nightly, at: "2026-06-26T02:00:00Z" }
---
body`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Verified) != 2 {
		t.Fatalf("expected 2 verified events, got %d", len(c.Verified))
	}
	if c.Verified[0].By != "human:alice" {
		t.Errorf("verified[0].by = %q", c.Verified[0].By)
	}
}

func TestParseVerified_SingleMapping(t *testing.T) {
	content := `---
type: Metric
verified: { by: human:alice, at: "2026-06-25T09:00:00Z" }
---
body`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Verified) != 1 {
		t.Fatalf("expected 1 verified event (bare mapping as one-element list), got %d", len(c.Verified))
	}
	if c.Verified[0].By != "human:alice" {
		t.Errorf("verified[0].by = %q", c.Verified[0].By)
	}
}

func TestParseVerified_Absent(t *testing.T) {
	content := `---
type: Metric
---
body`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Verified != nil {
		t.Errorf("expected nil verified, got %+v", c.Verified)
	}
}

// --- title optional tests (spec §4.1) ---

func TestParse_TitleOptional(t *testing.T) {
	content := `---
type: Metric
---
body without title`
	c, err := ParseConceptBytes("my-concept.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Title != "my concept" {
		t.Errorf("expected title derived from filename 'my concept', got %q", c.Title)
	}
}

func TestParse_TypeRequired(t *testing.T) {
	content := `---
title: No type
---
body`
	_, err := ParseConceptBytes("test.md", []byte(content))
	if err == nil {
		t.Error("expected error for missing type")
	}
}

// --- reserved filename tests (spec §3.1) ---

func TestIsReservedFilename(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"index.md", true},
		{"log.md", true},
		{"subdir/index.md", true},
		{"subdir/log.md", true},
		{"INDEX.MD", true},
		{"concept.md", false},
		{"subdir/concept.md", false},
	}
	for _, c := range cases {
		if got := IsReservedFilename(c.path); got != c.want {
			t.Errorf("IsReservedFilename(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestParse_ReservedFilename(t *testing.T) {
	content := `---
type: Metric
---
body`
	_, err := ParseConceptBytes("index.md", []byte(content))
	if err == nil {
		t.Error("expected error for reserved filename index.md")
	}
}

// --- v0.1 fallback tests (spec §13.1) ---

func TestParseV01_TimestampFallback(t *testing.T) {
	content := `---
type: Metric
title: Legacy
timestamp: "2026-05-28T22:53:05Z"
---
body`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Generated == nil {
		t.Fatal("expected Generated to be set from timestamp fallback")
	}
	if c.Generated.At != "2026-05-28T22:53:05Z" {
		t.Errorf("generated.at = %q", c.Generated.At)
	}
	if c.Generated.By != "unknown" {
		t.Errorf("generated.by = %q", c.Generated.By)
	}
	if c.Timestamp != "2026-05-28T22:53:05Z" {
		t.Errorf("timestamp should be preserved, got %q", c.Timestamp)
	}
}

func TestParseV01_TimestampNotOverwriteGenerated(t *testing.T) {
	content := `---
type: Metric
title: V02
timestamp: "2026-01-01T00:00:00Z"
generated: { by: agent/v1, at: "2026-06-20T22:53:05Z" }
---
body`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Generated.At != "2026-06-20T22:53:05Z" {
		t.Errorf("generated.at should not be overwritten by timestamp, got %q", c.Generated.At)
	}
}

func TestParseV01_CitationsFallback(t *testing.T) {
	content := `---
type: Metric
title: With citations
---
# Definition

Some text.

# Citations

- https://wiki.acme.com/finance/fpa-handbook
- https://wiki.acme.com/finance/revenue

# Other

More text.
`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Sources) != 2 {
		t.Fatalf("expected 2 sources from Citations fallback, got %d", len(c.Sources))
	}
	if c.Sources[0].Resource != "https://wiki.acme.com/finance/fpa-handbook" {
		t.Errorf("sources[0].resource = %q", c.Sources[0].Resource)
	}
	if c.Sources[1].Resource != "https://wiki.acme.com/finance/revenue" {
		t.Errorf("sources[1].resource = %q", c.Sources[1].Resource)
	}
	if c.Sources[0].ID != "citation-0" {
		t.Errorf("sources[0].id = %q", c.Sources[0].ID)
	}
}

func TestParseV01_CitationsNotOverwriteSources(t *testing.T) {
	content := `---
type: Metric
title: V02
sources:
 - id: existing
   resource: https://example.com/existing
---
# Citations

- https://wiki.acme.com/should-not-be-used
`
	c, err := ParseConceptBytes("test.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Sources) != 1 {
		t.Fatalf("expected 1 source (v0.2 sources should not be overwritten), got %d", len(c.Sources))
	}
	if c.Sources[0].ID != "existing" {
		t.Errorf("sources[0].id = %q", c.Sources[0].ID)
	}
}

// --- round-trip serialization tests ---

func TestSerializeV02_RoundTrip(t *testing.T) {
	original := `---
type: Attested Computation
title: Revenue
runtime: bigquery
generated:
  by: agent/v1
  at: "2026-06-20T22:53:05Z"
verified:
- by: human:alice
  at: "2026-06-25T09:00:00Z"
sources:
- id: src1
  resource: https://example.com
---
body`
	c, err := ParseConceptBytes("test.md", []byte(original))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	data, err := SerializeConcept(c, true)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}
	// Re-parse and verify key fields survive round-trip
	c2, err := ParseConceptBytes("test.md", data)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	if c2.Type != c.Type {
		t.Errorf("type mismatch: %q vs %q", c2.Type, c.Type)
	}
	if c2.Runtime != c.Runtime {
		t.Errorf("runtime mismatch: %q vs %q", c2.Runtime, c.Runtime)
	}
	if len(c2.Verified) != 1 || c2.Verified[0].By != "human:alice" {
		t.Errorf("verified mismatch: %+v", c2.Verified)
	}
	if len(c2.Sources) != 1 || c2.Sources[0].ID != "src1" {
		t.Errorf("sources mismatch: %+v", c2.Sources)
	}
	if c2.Generated == nil || c2.Generated.By != "agent/v1" {
		t.Errorf("generated mismatch: %+v", c2.Generated)
	}
}

func TestSerializeV02_OmitEmpty(t *testing.T) {
	c := &Concept{Type: "Metric", Title: "Minimal"}
	data, err := SerializeConcept(c, true)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}
	s := string(data)
	// v0.2 optional fields should not appear
	for _, field := range []string{"sources:", "generated:", "verified:", "status:", "stale_after:", "runtime:", "parameters:", "executor:", "attester:"} {
		if strings.Contains(s, field) {
			t.Errorf("serialized output should not contain %q, got:\n%s", field, s)
		}
	}
}

// --- Appendix A official example tests ---

func TestParseAppendixA_Revenue(t *testing.T) {
	content := `---
type: Attested Computation
title: Revenue for fiscal year
description: Recognized revenue for a fiscal year, per Finance's definition.
tags: [finance, revenue]
status: stable
runtime: bigquery
parameters:
 - { name: year, type: integer, required: true }
executor:
 resource: references/skills/run-on-bq.md
 receipt: [job_id, executed_sql, result]
attester:
 resource: references/attesters/sql-equality.py
generated: { by: reference_agent/gemini-2.5-pro, at: 2026-06-28T14:00:00Z }
verified: { by: human:ahormati, at: 2026-06-25T09:00:00Z }
stale_after: 2026-12-31
sources:
 - id: rev-policy
   resource: https://wiki.acme.com/finance/revenue-recognition
   title: Revenue recognition policy
   author: team:finance-fpa
   last_modified: 2026-04-02
 - id: exec-rev-dash
   resource: dashboards/exec-revenue
   title: Executive revenue dashboard
   author: team:finance-fpa
   usage_count: 5000
   last_modified: 2026-06-18
usage_window: { from: 2026-06-01, to: 2026-06-30 }
---

# Computation

` + "```sql" + `
 SELECT SUM(amount) AS revenue
 FROM finance.recognized_revenue
 WHERE fiscal_year = @year
` + "```" + `
`
	c, err := ParseConceptBytes("revenue.md", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify all key fields from Appendix A
	if c.Type != "Attested Computation" {
		t.Errorf("type = %q", c.Type)
	}
	if c.Runtime != "bigquery" {
		t.Errorf("runtime = %q", c.Runtime)
	}
	if len(c.Parameters) != 1 {
		t.Errorf("parameters count = %d", len(c.Parameters))
	}
	if c.Executor == nil || len(c.Executor.Receipt) != 3 {
		t.Errorf("executor receipt = %+v", c.Executor)
	}
	if c.Attester == nil {
		t.Error("attester is nil")
	}
	if c.Generated == nil || c.Generated.By != "reference_agent/gemini-2.5-pro" {
		t.Errorf("generated = %+v", c.Generated)
	}
	if len(c.Verified) != 1 || c.Verified[0].By != "human:ahormati" {
		t.Errorf("verified = %+v", c.Verified)
	}
	if c.StaleAfter != "2026-12-31" {
		t.Errorf("stale_after = %q", c.StaleAfter)
	}
	if len(c.Sources) != 2 {
		t.Errorf("sources count = %d", len(c.Sources))
	}
	if c.Sources[1].UsageCount != 5000 {
		t.Errorf("usage_count = %d", c.Sources[1].UsageCount)
	}
	if c.UsageWindow == nil || c.UsageWindow.To != "2026-06-30" {
		t.Errorf("usage_window = %+v", c.UsageWindow)
	}
}
