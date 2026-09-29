package memorydefense

import (
	"fmt"
	"strings"
)

// Policy controls Memory Defense behavior for one repository.
type Policy struct {
	Enabled   bool
	Action    string // "redact" or "block"
	Detectors []string
}

// Hit records one detector firing, without the matched secret text.
type Hit struct {
	DetectorID string `json:"detector_id"`
	Label      string `json:"label"`
	Severity   string `json:"severity"`
}

// ErrBlocked is returned when action=block and a high-severity detector fires.
type ErrBlocked struct {
	DetectorID string
}

func (e *ErrBlocked) Error() string {
	return fmt.Sprintf("memory_defense_blocked: content blocked by detector %q", e.DetectorID)
}

// activeDetectors filters the catalog by the policy whitelist. Empty whitelist
// means all detectors are active.
func (p Policy) activeDetectors() []Detector {
	cat := Catalog()
	if len(p.Detectors) == 0 {
		return cat
	}
	whitelist := make(map[string]bool, len(p.Detectors))
	for _, id := range p.Detectors {
		whitelist[strings.TrimSpace(id)] = true
	}
	out := make([]Detector, 0, len(cat))
	for _, d := range cat {
		if whitelist[d.ID] {
			out = append(out, d)
		}
	}
	return out
}

// Screen scans content against the active detectors.
//
// Returns the (possibly redacted) content, the list of hits, and an error.
// When policy.Action is "block" and any high-severity detector fires, the
// returned content is "" and error is *ErrBlocked. When action is "redact",
// every hit is replaced with [REDACTED:label] and hits are reported.
func Screen(content string, policy Policy) (string, []Hit, error) {
	if !policy.Enabled {
		return content, nil, nil
	}
	detectors := policy.activeDetectors()
	var hits []Hit
	out := content
	for _, d := range detectors {
		loc := d.Regex.FindAllStringIndex(out, -1)
		// Apply post-filter (e.g. Luhn) before counting hits.
		type match struct{ start, end int }
		var valid []match
		for _, idx := range loc {
			if d.Verify != nil && !d.Verify(out[idx[0]:idx[1]]) {
				continue
			}
			valid = append(valid, match{idx[0], idx[1]})
		}
		if len(valid) == 0 {
			continue
		}
		hits = append(hits, Hit{DetectorID: d.ID, Label: d.Label, Severity: d.Severity})
		if policy.Action == "block" && d.Severity == SeverityHigh {
			return "", hits, &ErrBlocked{DetectorID: d.ID}
		}
		// Redact: rebuild the string replacing each valid match.
		var b strings.Builder
		prev := 0
		for _, m := range valid {
			b.WriteString(out[prev:m.start])
			b.WriteString("[REDACTED:")
			b.WriteString(d.Label)
			b.WriteString("]")
			prev = m.end
		}
		b.WriteString(out[prev:])
		out = b.String()
	}
	return out, hits, nil
}
