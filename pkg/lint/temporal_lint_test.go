package lint

import (
	"fmt"
	"strings"
	"testing"
)

// --- helpers ---------------------------------------------------------------

// tlConcept builds a minimal lint.Concept carrying temporal metadata.
func tlConcept(id, typ, project, filePath string, t *TemporalInfo) *Concept {
	return &Concept{
		OKFID:    id,
		Type:     typ,
		Project:  project,
		FilePath: filePath,
		Temporal: t,
	}
}

// tlMeta builds a TemporalInfo with the given state/kind/targets and no warnings.
func tlMeta(state, kind string, targets ...string) *TemporalInfo {
	return &TemporalInfo{
		HasTemporal:     true,
		State:           state,
		RelationKind:    kind,
		RelationTargets: targets,
	}
}

// temporalIssueCodes returns the subset of codes that belong to the temporal pass.
var temporalCodes = map[string]bool{
	tlInvalidState:    true,
	tlInvalidRelation: true,
	tlDangling:        true,
	tlCrossProject:    true,
	tlAmbiguous:       true,
	tlCycle:           true,
	tlTooDeep:         true,
}

// filterTemporal keeps only temporal-pass issues.
func filterTemporal(issues []Issue) []Issue {
	var out []Issue
	for _, is := range issues {
		if temporalCodes[is.Code] {
			out = append(out, is)
		}
	}
	return out
}

func findCode(issues []Issue, code string) []Issue {
	var out []Issue
	for _, is := range issues {
		if is.Code == code {
			out = append(out, is)
		}
	}
	return out
}

func issueForPath(issues []Issue, path string) *Issue {
	for i := range issues {
		if issues[i].FilePath == path {
			return &issues[i]
		}
	}
	return nil
}

const (
	idA = "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	idB = "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	idC = "okf_cccccccccccccccccccccccccccccccc"
	idZ = "okf_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
)

// --- per-concept malformed checks ------------------------------------------

func TestTemporalMalformedStateIsReported(t *testing.T) {
	t.Parallel()
	c := tlConcept(idA, "note", "p", "a.md", &TemporalInfo{
		HasTemporal: true,
		State:       "proposed",
		StateWarn:   "unknown memory_state \"weird\"; treating as proposed",
	})
	res := LintBundle([]*Concept{c}, DefaultConfig())
	got := findCode(res.Issues, tlInvalidState)
	if len(got) == 0 {
		t.Fatalf("expected invalid_memory_state issue, got %v", filterTemporal(res.Issues))
	}
}

func TestTemporalMalformedConfidenceIsStateCode(t *testing.T) {
	t.Parallel()
	c := tlConcept(idA, "note", "p", "a.md", &TemporalInfo{
		HasTemporal:    true,
		State:          "approved",
		ConfidenceWarn: "memory_confidence is string, not a number",
	})
	res := LintBundle([]*Concept{c}, DefaultConfig())
	got := findCode(res.Issues, tlInvalidState)
	if len(got) == 0 {
		t.Fatalf("expected invalid_memory_state for confidence warning, got %v", filterTemporal(res.Issues))
	}
}

func TestTemporalMalformedRelationIsReported(t *testing.T) {
	t.Parallel()
	c := tlConcept(idA, "note", "p", "a.md", &TemporalInfo{
		HasTemporal:  true,
		State:        "approved",
		RelationWarn: "invalid_memory_relation: memory_relation is string, not a map",
	})
	res := LintBundle([]*Concept{c}, DefaultConfig())
	got := findCode(res.Issues, tlInvalidRelation)
	if len(got) == 0 {
		t.Fatalf("expected invalid_memory_relation issue, got %v", filterTemporal(res.Issues))
	}
}

// --- graph checks -----------------------------------------------------------

func TestTemporalDanglingUnresolvableTarget(t *testing.T) {
	t.Parallel()
	src := tlConcept(idA, "note", "p", "a.md", tlMeta("approved", "updates", idZ))
	res := LintBundle([]*Concept{src}, DefaultConfig())
	got := findCode(res.Issues, tlDangling)
	if len(got) == 0 {
		t.Fatalf("expected dangling issue, got %v", filterTemporal(res.Issues))
	}
	if got[0].FilePath != "a.md" {
		t.Fatalf("dangling issue must be on source path a.md, got %q", got[0].FilePath)
	}
}

func TestTemporalDanglingNonApprovedTarget(t *testing.T) {
	t.Parallel()
	src := tlConcept(idA, "note", "p", "a.md", tlMeta("approved", "updates", idB))
	tgt := tlConcept(idB, "note", "p", "b.md", tlMeta("proposed", ""))
	res := LintBundle([]*Concept{src, tgt}, DefaultConfig())
	got := findCode(res.Issues, tlDangling)
	if len(got) == 0 {
		t.Fatalf("expected dangling (non-approved target) issue, got %v", filterTemporal(res.Issues))
	}
	if !strings.Contains(got[0].Message, "not approved") {
		t.Fatalf("expected non-approved message, got %q", got[0].Message)
	}
}

func TestTemporalDanglingNonDurableTarget(t *testing.T) {
	t.Parallel()
	src := tlConcept(idA, "note", "p", "a.md", tlMeta("approved", "updates", idB))
	tgt := tlConcept(idB, "api", "p", "b.md", tlMeta("approved", "")) // api is not durable
	res := LintBundle([]*Concept{src, tgt}, DefaultConfig())
	got := findCode(res.Issues, tlDangling)
	if len(got) == 0 {
		t.Fatalf("expected dangling (non-durable target) issue, got %v", filterTemporal(res.Issues))
	}
}

func TestTemporalCrossProject(t *testing.T) {
	t.Parallel()
	src := tlConcept(idA, "note", "p1", "a.md", tlMeta("approved", "updates", idB))
	tgt := tlConcept(idB, "note", "p2", "b.md", tlMeta("approved", ""))
	res := LintBundle([]*Concept{src, tgt}, DefaultConfig())
	got := findCode(res.Issues, tlCrossProject)
	if len(got) == 0 {
		t.Fatalf("expected cross-project issue, got %v", filterTemporal(res.Issues))
	}
	if got[0].FilePath != "a.md" {
		t.Fatalf("cross-project issue must be on source a.md, got %q", got[0].FilePath)
	}
}

func TestTemporalAmbiguousFork(t *testing.T) {
	t.Parallel()
	u1 := tlConcept(idA, "note", "p", "a.md", tlMeta("approved", "updates", idB))
	u2 := tlConcept(idC, "note", "p", "c.md", tlMeta("approved", "updates", idB))
	tgt := tlConcept(idB, "note", "p", "b.md", tlMeta("approved", ""))
	res := LintBundle([]*Concept{u1, u2, tgt}, DefaultConfig())
	got := findCode(res.Issues, tlAmbiguous)
	if len(got) != 2 {
		t.Fatalf("expected ambiguous issue on both updaters, got %v", got)
	}
	if issueForPath(got, "a.md") == nil || issueForPath(got, "c.md") == nil {
		t.Fatalf("ambiguous issue must cover both updaters a.md and c.md: %v", got)
	}
}

func TestTemporalCycle(t *testing.T) {
	t.Parallel()
	a := tlConcept(idA, "note", "p", "a.md", tlMeta("approved", "updates", idB))
	b := tlConcept(idB, "note", "p", "b.md", tlMeta("approved", "updates", idA))
	res := LintBundle([]*Concept{a, b}, DefaultConfig())
	got := findCode(res.Issues, tlCycle)
	if len(got) == 0 {
		t.Fatalf("expected cycle issue, got %v", filterTemporal(res.Issues))
	}
	if issueForPath(got, "a.md") == nil || issueForPath(got, "b.md") == nil {
		t.Fatalf("cycle issue must cover both nodes a.md and b.md: %v", got)
	}
}

func TestTemporalHistoryTooDeep(t *testing.T) {
	t.Parallel()
	const n = maxTemporalHistory + 2 // 130 nodes -> exceeds 128
	ids := make([]string, n)
	concepts := make([]*Concept, n)
	for i := range n {
		ids[i] = okfID(i)
	}
	for i := 0; i < n; i++ {
		var m *TemporalInfo
		if i < n-1 {
			// each node updates the next; the last node has no outgoing edge.
			m = tlMeta("approved", "updates", ids[i+1])
		} else {
			m = tlMeta("approved", "")
		}
		concepts[i] = tlConcept(ids[i], "note", "p", ids[i]+".md", m)
	}
	res := LintBundle(concepts, DefaultConfig())
	got := findCode(res.Issues, tlTooDeep)
	if len(got) == 0 {
		t.Fatalf("expected temporal_history_too_deep issue, got %v", filterTemporal(res.Issues))
	}
	if got[0].FilePath != ids[0]+".md" {
		t.Fatalf("depth issue must be on the chain head %s.md, got %q", ids[0], got[0].FilePath)
	}
}

// --- clean / parity ---------------------------------------------------------

func TestTemporalCleanBundleHasNoIssues(t *testing.T) {
	t.Parallel()
	a := tlConcept(idA, "note", "p", "a.md", tlMeta("approved", "updates", idB))
	b := tlConcept(idB, "note", "p", "b.md", tlMeta("approved", ""))
	res := LintBundle([]*Concept{a, b}, DefaultConfig())
	if got := filterTemporal(res.Issues); len(got) != 0 {
		t.Fatalf("clean chain should have no temporal issues, got %v", got)
	}
}

func TestTemporalLegacyBundleParity(t *testing.T) {
	t.Parallel()
	// No concept carries Temporal at all -> the whole-bundle pass is skipped.
	concepts := []*Concept{
		{Type: "note", Title: "legacy", FilePath: "x.md"},
		{Type: "note", Title: "legacy2", FilePath: "y.md"},
	}
	res := LintBundle(concepts, DefaultConfig())
	if got := filterTemporal(res.Issues); len(got) != 0 {
		t.Fatalf("legacy bundle must not emit temporal issues, got %v", got)
	}
}

// --- severity matrix -------------------------------------------------------

func TestTemporalNonStrictIsWarning(t *testing.T) {
	t.Parallel()
	c := tlConcept(idA, "note", "p", "a.md", &TemporalInfo{
		HasTemporal: true,
		State:       "proposed",
		StateWarn:   "unknown memory_state",
	})
	res := LintBundle([]*Concept{c}, DefaultConfig()) // StrictMode off
	got := findCode(res.Issues, tlInvalidState)
	if len(got) == 0 {
		t.Fatalf("expected an issue, got %v", res.Issues)
	}
	if got[0].Severity != Warning {
		t.Fatalf("non-strict temporal issue must be Warning, got %s", got[0].Severity)
	}
	if res.HasErrors() {
		t.Fatalf("non-strict must not set HasErrors")
	}
}

func TestTemporalStrictIsError(t *testing.T) {
	t.Parallel()
	c := tlConcept(idA, "note", "p", "a.md", &TemporalInfo{
		HasTemporal: true,
		State:       "proposed",
		StateWarn:   "unknown memory_state",
	})
	res := LintBundle([]*Concept{c}, &Config{MaxLineLength: 240, MinDescriptionLength: 10, StrictMode: true})
	got := findCode(res.Issues, tlInvalidState)
	if len(got) == 0 {
		t.Fatalf("expected an issue, got %v", res.Issues)
	}
	if got[0].Severity != Error {
		t.Fatalf("strict temporal issue must be Error, got %s", got[0].Severity)
	}
	if !res.HasErrors() {
		t.Fatalf("strict must set HasErrors")
	}
}

// --- helpers ----------------------------------------------------------------

// okfID returns a unique canonical-looking okf id for index i (okf_ + 32
// lowercase hex chars). It only needs to be unique and grammar-consistent for
// the in-package graph tests, which match ids by string equality.
func okfID(i int) string {
	return fmt.Sprintf("okf_%032x", i)
}
