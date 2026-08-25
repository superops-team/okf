package parser

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// TestStabilityRandomRoundTrip generates 100 random v0.2 concepts, serializes them,
// re-parses them, and verifies key fields are preserved. This catches serialization
// edge cases and yaml.v3 panics.
func TestStabilityRandomRoundTrip(t *testing.T) {
	t.Parallel()

	types := []string{"concept", "api", "metric", "code_file", "Attested Computation", "Metric", "component"}
	runtimes := []string{"", "bigquery", "dbt", "python", "postgres"}
	statuses := []string{"", "stable", "draft", "deprecated"}
	actors := []string{"human:alice", "process:etl-pipeline", "dbt/1.5", "bigquery/2.0"}

	for i := 0; i < 100; i++ {
		c := &Concept{
			Type:    types[rand.Intn(len(types))],
			Title:   fmt.Sprintf("Concept %d", i),
			Content: fmt.Sprintf("# Concept %d\n\nThis is concept number %d.\n", i, i),
			Tags:    []string{"tag1", "tag2"},
		}

		// Randomly add v0.2 fields
		if rand.Intn(2) == 0 {
			c.Description = "A test concept with a reasonably long description."
		}
		if rand.Intn(2) == 0 {
			c.Resource = fmt.Sprintf("resource://%d", i)
		}
		if rand.Intn(3) == 0 {
			c.Sources = []Source{
				{ID: fmt.Sprintf("src-%d", i), Resource: fmt.Sprintf("https://example.com/%d", i), Title: "Source"},
			}
		}
		if rand.Intn(3) == 0 {
			c.Generated = &GeneratedInfo{By: actors[rand.Intn(len(actors))], At: "2026-01-15T10:30:00Z"}
		}
		if rand.Intn(3) == 0 {
			c.Verified = []VerificationEvent{{By: actors[rand.Intn(len(actors))], At: "2026-02-01T00:00:00Z"}}
		}
		if rand.Intn(3) == 0 {
			c.Status = statuses[rand.Intn(len(statuses))]
		}
		if rand.Intn(3) == 0 {
			c.StaleAfter = "2026-12-31"
		}
		if c.Type == "Attested Computation" {
			c.Runtime = runtimes[rand.Intn(len(runtimes))]
			if c.Runtime == "" {
				c.Runtime = "bigquery"
			}
			c.Computation = "SELECT * FROM table"
		}
		if rand.Intn(3) == 0 {
			c.CustomFields = map[string]interface{}{
				"custom_key": "custom_value",
				"custom_num": 42,
			}
		}

		// Round-trip
		data, err := SerializeConcept(c, true)
		if err != nil {
			t.Fatalf("iteration %d: serialize error: %v", i, err)
		}

		c2, err := ParseConceptBytes(fmt.Sprintf("concept-%d.md", i), data)
		if err != nil {
			t.Fatalf("iteration %d: re-parse error: %v\n--- output ---\n%s", i, err, string(data))
		}

		// Verify key fields
		if c2.Type != c.Type {
			t.Errorf("iteration %d: type mismatch: %q != %q", i, c2.Type, c.Type)
		}
		if c2.Title != c.Title {
			t.Errorf("iteration %d: title mismatch: %q != %q", i, c2.Title, c.Title)
		}
		if len(c.Sources) > 0 && len(c2.Sources) != len(c.Sources) {
			t.Errorf("iteration %d: sources count mismatch: %d != %d", i, len(c2.Sources), len(c.Sources))
		}
		if c.Generated != nil && c2.Generated == nil {
			t.Errorf("iteration %d: generated lost", i)
		}
		if len(c.Verified) > 0 && len(c2.Verified) != len(c.Verified) {
			t.Errorf("iteration %d: verified count mismatch: %d != %d", i, len(c2.Verified), len(c.Verified))
		}
		if c.StaleAfter != "" && c2.StaleAfter != c.StaleAfter {
			t.Errorf("iteration %d: stale_after mismatch: %q != %q", i, c2.StaleAfter, c.StaleAfter)
		}
		if c.Runtime != "" && c2.Runtime != c.Runtime {
			t.Errorf("iteration %d: runtime mismatch: %q != %q", i, c2.Runtime, c.Runtime)
		}
	}
}

// TestStabilityLegacyGeneratedBool verifies that legacy "generated: true" (boolean)
// round-trips correctly without panic. This is critical for backward compatibility.
func TestStabilityLegacyGeneratedBool(t *testing.T) {
	t.Parallel()

	input := `---
type: code_file
title: handler.go
generated: true
generator: okf.git
source_path: internal/handler.go
---
Body
`
	c, err := ParseConceptBytes("handler.go.md", []byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if c.CustomFields["generated"] != true {
		t.Errorf("generated = %#v, want true", c.CustomFields["generated"])
	}

	// Round-trip
	data, err := SerializeConcept(c, true)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}
	if !strings.Contains(string(data), "generated: true") {
		t.Errorf("serialized output missing 'generated: true':\n%s", string(data))
	}

	// Re-parse
	c2, err := ParseConceptBytes("handler.go.md", data)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	if c2.CustomFields["generated"] != true {
		t.Errorf("re-parsed generated = %#v, want true", c2.CustomFields["generated"])
	}
}

// TestStabilityV02GeneratedStruct verifies that v0.2 "generated: {by, at}" round-trips.
func TestStabilityV02GeneratedStruct(t *testing.T) {
	t.Parallel()

	input := `---
type: Attested Computation
title: Revenue
runtime: bigquery
generated:
  by: process:etl-pipeline
  at: 2026-01-15T10:30:00Z
---
Body
`
	c, err := ParseConceptBytes("revenue.md", []byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if c.Generated == nil {
		t.Fatal("generated is nil")
	}
	if c.Generated.By != "process:etl-pipeline" {
		t.Errorf("generated.by = %q, want process:etl-pipeline", c.Generated.By)
	}

	// Round-trip
	data, err := SerializeConcept(c, true)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}

	c2, err := ParseConceptBytes("revenue.md", data)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	if c2.Generated == nil || c2.Generated.By != "process:etl-pipeline" {
		t.Errorf("re-parsed generated.by = %#v, want process:etl-pipeline", c2.Generated)
	}
}
