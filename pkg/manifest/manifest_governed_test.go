package manifest

// Governed-agent-memory scenarios S05-S08, S11-S15, S17-S19, S28-S35 at the
// manifest.Build level. S10/S16 live in pkg/memorymeta (P0).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func governedFM(typ, title, extra string) string {
	return goodFM(typ, title, extra)
}

// writes a knowledge root with mixed-governance concepts and a repo root used
// for stale-refs scans. Returns the knowledge root (manifest root) and repo.
func governedFixture(t *testing.T) (root, repo string) {
	t.Helper()
	root = t.TempDir()
	repo = t.TempDir()
	writeKnowledgeFile(t, root, "concepts/hold.governed.md", governedFM("decision", "Hold rule",
		"okf_id: okf_11111111111111111111111111111111\n"+
			"governance: hold\ncode_refs:\n  - pkg/hold/*.go\n  - pkg/shared/*.go\n"))
	writeKnowledgeFile(t, root, "concepts/constraint.governed.md", governedFM("decision", "Constraint rule",
		"okf_id: okf_22222222222222222222222222222222\n"+
			"governance: constraint\ncode_refs:\n  - pkg/constraint/guard.go\n  - pkg/shared/*.go\n"))
	writeKnowledgeFile(t, root, "concepts/context.governed.md", governedFM("decision", "Context note",
		"okf_id: okf_33333333333333333333333333333333\n"+
			"code_refs:\n  - pkg/context/**/*.md\n  - pkg/shared/*.go\n"))
	return root, repo
}

func buildAt(t *testing.T, root string, req ManifestRequest) (*ManifestResult, error) {
	t.Helper()
	return Build(context.Background(), root, filepath.Join(root, ".okf", "vector"), req, nil,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false)
}

// S05: without --for-path / --governance the order is the historical one
// (path ASC, okf_id ASC).
func TestS05DefaultOrderUnchanged(t *testing.T) {
	root, _ := governedFixture(t)
	res, err := buildAt(t, root, ManifestRequest{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := []string{
		"concepts/constraint.governed.md",
		"concepts/context.governed.md",
		"concepts/hold.governed.md",
	}
	if len(res.Items) != len(want) {
		t.Fatalf("items = %d", len(res.Items))
	}
	for i, p := range want {
		if res.Items[i].Path != p {
			t.Fatalf("order = %v, want %v", itemPaths(res.Items), want)
		}
	}
	if res.GovernanceWarning {
		t.Fatal("GovernanceWarning must stay false when no new params")
	}
}

// S06: governance sort activates only when for_path or governance is set.
func TestS06GovernanceSortActivation(t *testing.T) {
	root, _ := governedFixture(t)

	// --for-path without --governance: hold → constraint → context.
	res, err := buildAt(t, root, ManifestRequest{ForPath: "pkg/shared/x.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got := itemPaths(res.Items)
	want := []string{
		"concepts/hold.governed.md",
		"concepts/constraint.governed.md",
		"concepts/context.governed.md",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("for_path order = %v, want %v", got, want)
	}

	// --governance filter keeps only the listed levels, sorted hold→constraint.
	res, err = buildAt(t, root, ManifestRequest{Governance: []string{"constraint", "hold"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got = itemPaths(res.Items)
	want = []string{"concepts/hold.governed.md", "concepts/constraint.governed.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("governance filter order = %v, want %v", got, want)
	}
	for _, it := range res.Items {
		if it.Governance != "hold" && it.Governance != "constraint" {
			t.Fatalf("unexpected governance %q in filtered result", it.Governance)
		}
	}
}

// S07: hold is advisory — governance_warning surfaces it, never blocks.
func TestS07HoldAdvisoryWarning(t *testing.T) {
	root, _ := governedFixture(t)
	res, err := buildAt(t, root, ManifestRequest{ForPath: "pkg/shared/x.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !res.GovernanceWarning {
		t.Fatal("governance_warning must be true when a hold concept matches")
	}
	// No hold in the filtered set → false.
	res2, err := buildAt(t, root, ManifestRequest{Governance: []string{"constraint"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res2.GovernanceWarning {
		t.Fatal("governance_warning must be false when no hold is present")
	}
}

// S08: governance filtering.
func TestS08GovernanceFilter(t *testing.T) {
	root, _ := governedFixture(t)
	res, err := buildAt(t, root, ManifestRequest{Governance: []string{"hold"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Governance != "hold" {
		t.Fatalf("hold filter = %v", itemPaths(res.Items))
	}
}

// S11: for_path exact match surfaces matched_code_ref.
func TestS11ForPathExactMatch(t *testing.T) {
	root, _ := governedFixture(t)
	res, err := buildAt(t, root, ManifestRequest{ForPath: "pkg/constraint/guard.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("exact match returned %d items, want 1: %v", len(res.Items), itemPaths(res.Items))
	}
	if res.Items[0].MatchedCodeRef != "pkg/constraint/guard.go" {
		t.Fatalf("matched_code_ref = %q", res.Items[0].MatchedCodeRef)
	}
	// Non-matching path does not return the concept.
	res, err = buildAt(t, root, ManifestRequest{ForPath: "pkg/constraint/other.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("non-match returned %d items", len(res.Items))
	}
}

// S12: single-segment * does not cross a slash.
func TestS12ForPathSingleSegmentGlob(t *testing.T) {
	root, _ := governedFixture(t)
	res, err := buildAt(t, root, ManifestRequest{ForPath: "pkg/hold/server.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].MatchedCodeRef != "pkg/hold/*.go" {
		t.Fatalf("glob match = %v", itemPaths(res.Items))
	}
	res, err = buildAt(t, root, ManifestRequest{ForPath: "pkg/hold/sub/server.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("single-segment glob crossed a slash: %v", itemPaths(res.Items))
	}
}

// S13: ** matches at most 8 segments.
func TestS13ForPathDoubleStarDepth(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/deep.md", governedFM("decision", "Deep",
		"code_refs:\n  - pkg/**/*.go\n"))
	within := "pkg/" + strings.Join([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, "/") + "/file.go"
	res, err := buildAt(t, root, ManifestRequest{ForPath: within})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("8 segments after pkg/ should match, got %d", len(res.Items))
	}
	beyond := "pkg/a/b/c/d/e/f/g/h/i/file.go"
	res, err = buildAt(t, root, ManifestRequest{ForPath: beyond})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("9 segments after pkg/ must NOT match, got %d", len(res.Items))
	}
}

// S14: for_path canonicalization and rejection.
func TestS14ForPathCanonicalization(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/x.md", governedFM("decision", "X",
		"code_refs:\n  - pkg/mcp/server.go\n"))

	// ./ prefix canonicalized and matched.
	res, err := buildAt(t, root, ManifestRequest{ForPath: "./pkg/mcp/server.go"})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("./prefixed path should match, got %d", len(res.Items))
	}
	// Inside-root .. stays lexical (canonicalized, no FS check).
	res, err = buildAt(t, root, ManifestRequest{ForPath: "pkg/mcp/../mcp/server.go"})
	if err != nil {
		t.Fatalf("inside-root .. should not error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("inside-root .. should canonicalize to a match, got %d", len(res.Items))
	}
	for _, bad := range []string{"/abs/path", `pkg\mcp\server.go`, "../secret/file", "pkg/../../secret/file"} {
		if _, err := buildAt(t, root, ManifestRequest{ForPath: bad}); err == nil {
			t.Fatalf("for_path %q must be rejected", bad)
		} else if me, ok := err.(*Error); !ok || me.Code != "invalid_request" {
			t.Fatalf("for_path %q err = %v, want invalid_request", bad, err)
		}
	}
	// NUL byte.
	if _, err := buildAt(t, root, ManifestRequest{ForPath: "pkg/mcp\x00evil.go"}); err == nil {
		t.Fatal("NUL byte must be rejected")
	}
}

// S15: lexical match needs no file on disk.
func TestS15ForPathLexicalNoFS(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/new.md", governedFM("decision", "New",
		"code_refs:\n  - pkg/newfeature/*.go\n"))
	res, err := buildAt(t, root, ManifestRequest{ForPath: "pkg/newfeature/widget.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("lexical match of not-yet-created path = %d items", len(res.Items))
	}
}

// S19: code_refs only binds the domain concept that declares them; a
// code_file-shaped concept with the same path is not auto-bound.
func TestS19NoDuplicateCodeFileBinding(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/decision.md", governedFM("decision", "Decision",
		"code_refs:\n  - pkg/server.go\n"))
	writeKnowledgeFile(t, root, "code_files/pkg/server.md", governedFM("code_file", "server.go", ""))
	res, err := buildAt(t, root, ManifestRequest{ForPath: "pkg/server.go"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Type != "decision" {
		t.Fatalf("only the decision should match: %v", itemPaths(res.Items))
	}
}

// S17: stale-refs lists concepts whose code_refs match no file on disk.
func TestS17StaleRefsScan(t *testing.T) {
	root, repo := governedFixture(t)
	// Repo actually contains pkg/hold/a.go and pkg/constraint/guard.go;
	// the context pattern has no md files.
	mkRepoFile(t, filepath.Join(repo, "pkg", "hold", "a.go"), "package hold\n")
	mkRepoFile(t, filepath.Join(repo, "pkg", "shared", "x.go"), "package shared\n")
	mkRepoFile(t, filepath.Join(repo, "pkg", "constraint", "guard.go"), "package constraint\n")
	res, err := buildAt(t, root, ManifestRequest{StaleRefs: true, RepoRoot: repo})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var stale []string
	for _, it := range res.Items {
		stale = append(stale, it.StaleCodeRefs...)
	}
	if len(stale) == 0 {
		t.Fatalf("expected stale_code_refs on some item, items=%+v", res.Items)
	}
	for _, s := range stale {
		if s != "pkg/context/**/*.md" && s != "pkg/constraint/guard.go" {
			// pkg/hold/*.go has a match → not stale.
			t.Fatalf("unexpected stale ref %q", s)
		}
	}
}

// S28: summary mode strips everything but okf_id/title/type/governance/desc.
func TestS28SummaryModeShape(t *testing.T) {
	root, _ := governedFixture(t)
	res, err := buildAt(t, root, ManifestRequest{Mode: ModeSummary})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, it := range res.Items {
		data, err := json.Marshal(it)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// Must NOT contain legacy full-mode fields
		for _, forbidden := range []string{"path", "ref", "tags", "code_refs", "status", "trust_tier", "stale", "source_count", "file_size_bytes", "estimated_tokens", "estimate_kind", "identity_state"} {
			if _, ok := m[forbidden]; ok {
				t.Fatalf("summary JSON leaked field %q: %s", forbidden, string(data))
			}
		}
		// Must contain at least one identifying field
		if m["okf_id"] == nil && m["title"] == nil {
			t.Fatalf("summary item missing required fields: %s", string(data))
		}
		if len([]rune(it.Description)) > 80 {
			t.Fatalf("description longer than 80 runes: %q", it.Description)
		}
	}
}

// S29: hit mode adds tags, code_refs, status, stale_after.
func TestS29HitModeShape(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/hit.md", governedFM("note", "Hit",
		"tags: [t1, t2]\nstatus: stable\nstale_after: 2027-01-01\ncode_refs:\n  - pkg/*.go\n"))
	res, err := buildAt(t, root, ManifestRequest{Mode: ModeHit})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d", len(res.Items))
	}
	it := res.Items[0]
	data, err := json.Marshal(it)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Hit mode must include these
	for _, required := range []string{"tags", "code_refs", "status", "stale_after"} {
		if _, ok := m[required]; !ok {
			t.Fatalf("hit JSON missing field %q: %s", required, string(data))
		}
	}
	// Hit mode must NOT include full-mode-only fields
	for _, forbidden := range []string{"path", "ref", "trust_tier", "stale", "source_count", "file_size_bytes", "estimated_tokens", "estimate_kind", "identity_state"} {
		if _, ok := m[forbidden]; ok {
			t.Fatalf("hit JSON leaked full-mode field %q: %s", forbidden, string(data))
		}
	}
}

// S30: full mode (default) keeps the pre-change shape.
func TestS30FullModeBackwardCompatible(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/a.md", governedFM("concept", "A", ""))
	writeKnowledgeFile(t, root, "concepts/b.md", governedFM("source", "B", "tags: [x]\n"))
	for _, mode := range []string{"", "full"} {
		res, err := buildAt(t, root, ManifestRequest{Mode: mode})
		if err != nil {
			t.Fatalf("build mode %q: %v", mode, err)
		}
		for _, it := range res.Items {
			if it.Path == "" || it.IdentityState == "" || it.Status == "" || it.EstimateKind == "" {
				t.Fatalf("full item lost a legacy field: %+v", it)
			}
		}
	}
}

// S31: ID parity across modes without max_tokens.
func TestS31ModeIDParity(t *testing.T) {
	root, _ := governedFixture(t)
	var ids [][]string
	for _, mode := range []string{ModeSummary, ModeHit, ModeFull} {
		res, err := buildAt(t, root, ManifestRequest{Mode: mode, ForPath: "pkg/shared/x.go"})
		if err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
		var list []string
		for _, it := range res.Items {
			list = append(list, it.OKFID)
		}
		ids = append(ids, list)
	}
	for i := 1; i < len(ids); i++ {
		if strings.Join(ids[0], ",") != strings.Join(ids[i], ",") {
			t.Fatalf("ID parity broken: %v vs %v", ids[0], ids[i])
		}
	}
}

// S32: pipeline order and response fields.
func TestS32TokenBudgetPipeline(t *testing.T) {
	root := t.TempDir()
	for i, g := range []string{"hold", "constraint", "context", "hold", "context"} {
		writeKnowledgeFile(t, root, fmt.Sprintf("concepts/c%d.md", i), governedFM("note", fmt.Sprintf("C%d", i),
			"governance: "+g+"\ndescription: A moderately long description text for item number.\n"))
	}
	res, err := buildAt(t, root, ManifestRequest{
		Offset:    1,
		Limit:     newInt(3),
		MaxTokens: 1, // far too small for any item
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.BudgetTooSmall == nil {
		t.Fatal("expected budget_too_small")
	}
	if res.NextOffset != 1 {
		t.Fatalf("next_offset = %d, must not advance", res.NextOffset)
	}
	if len(res.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(res.Items))
	}
	if res.BudgetTooSmall.MinRequiredTokens <= 0 {
		t.Fatal("min_required_tokens must be dynamic and positive")
	}

	// Generous budget: whole page returned, next_offset points past page,
	// total_remaining = items after the window.
	large := 1000000
	res, err = buildAt(t, root, ManifestRequest{Offset: 1, Limit: newInt(2), MaxTokens: large})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(res.Items))
	}
	if res.NextOffset != 3 {
		t.Fatalf("next_offset = %d, want 3", res.NextOffset)
	}
	if res.OmittedCount.TotalRemaining != 2 {
		t.Fatalf("total_remaining = %d, want 2", res.OmittedCount.TotalRemaining)
	}
	if res.OmittedCount.BudgetOmitted != 0 {
		t.Fatalf("budget_omitted = %d, want 0", res.OmittedCount.BudgetOmitted)
	}
	if !res.Truncated {
		t.Fatal("truncated must be true when items remain beyond the page")
	}
}

// S33: budget_too_small signals the first eligible item's dynamic estimate.
func TestS33BudgetTooSmall(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/a.md", governedFM("note", "A", "description: hello world\n"))
	full, err := buildAt(t, root, ManifestRequest{Mode: ModeFull})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := estimateJSONTokens(full.Items[0])
	small := want - 1
	res, err := buildAt(t, root, ManifestRequest{MaxTokens: small})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.BudgetTooSmall == nil || res.BudgetTooSmall.MinRequiredTokens != want {
		t.Fatalf("min_required_tokens = %+v, want %d", res.BudgetTooSmall, want)
	}
	if len(res.Items) != 0 || res.NextOffset != 0 {
		t.Fatalf("items=%d next_offset=%d", len(res.Items), res.NextOffset)
	}
}

// S34: per-item estimate is ceil(go json bytes / 4) on the projected item.
func TestS34TokenEstimateDefinition(t *testing.T) {
	root := t.TempDir()
	writeKnowledgeFile(t, root, "concepts/a.md", governedFM("note", "A", "description: hello world\n"))
	res, err := buildAt(t, root, ManifestRequest{Mode: ModeSummary, MaxTokens: 100000})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d", len(res.Items))
	}
	want := estimateJSONTokens(res.Items[0])
	if res.EstimatedItemTokens != want {
		t.Fatalf("estimated_item_tokens = %d, want %d", res.EstimatedItemTokens, want)
	}
	// Go json.Marshal escapes <, >, & by default — assert no custom encoder.
	if !strings.Contains(string(mustMarshal(t, res.Items[0])), `"`) {
		t.Fatal("expected JSON output")
	}
}

// S32 helper: dropped-by-budget distinguishes from total_remaining.
func TestS32BudgetOmittedVsTotalRemaining(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 4; i++ {
		writeKnowledgeFile(t, root, fmt.Sprintf("concepts/c%d.md", i), governedFM("note", fmt.Sprintf("C%d", i),
			"description: text that makes each item non-trivial.\n"))
	}
	res, err := buildAt(t, root, ManifestRequest{Limit: newInt(3), MaxTokens: 1})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.BudgetTooSmall == nil {
		t.Fatal("first item alone exceeds budget → budget_too_small")
	}
	// Budget that fits exactly one item: second dropped by budget.
	// Compute a realistic budget from the first item.
	full, _ := buildAt(t, root, ManifestRequest{})
	one := estimateJSONTokens(full.Items[0])
	res, err = buildAt(t, root, ManifestRequest{Limit: newInt(3), MaxTokens: one})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(res.Items))
	}
	if res.OmittedCount.BudgetOmitted != 2 {
		t.Fatalf("budget_omitted = %d, want 2 (page items 1,2 dropped by budget)", res.OmittedCount.BudgetOmitted)
	}
	if res.OmittedCount.TotalRemaining != 1 {
		t.Fatalf("total_remaining = %d, want 1 (item 3 beyond limit window)", res.OmittedCount.TotalRemaining)
	}
	if !res.Truncated {
		t.Fatal("truncated must be true")
	}
	if res.NextOffset != 1 {
		t.Fatalf("next_offset = %d, want 1", res.NextOffset)
	}
	if res.EstimatedItemTokens != one {
		t.Fatalf("estimated_item_tokens = %d, want %d", res.EstimatedItemTokens, one)
	}
}

func newInt(v int) *int { return &v }

func itemPaths(items []ManifestItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Path)
	}
	return out
}

func estimateJSONTokens(it ManifestItem) int {
	data, _ := json.Marshal(it)
	return (len(data) + 3) / 4
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func mkRepoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// S18: the --stale-refs walk is bounded by an injectable entry cap. With a
// tiny injected cap the walk completes exactly at the boundary but marks
// incomplete with a warning once entries exceed it; it never visits more than
// the cap (no 50k-file fixture required).
func TestStaleRefsScanEntryLimit(t *testing.T) {
	old := maxStaleScanEntries
	maxStaleScanEntries = 3
	defer func() { maxStaleScanEntries = old }()

	// Entry accounting: the repo root itself is entry #1; each file after it
	// counts one. So cap=3 admits root + 2 files before the 4th entry trips
	// the cap.
	//
	// (a) Exactly at the boundary: 2 files => 3 entries visited, walk finishes
	//     naturally => complete, no warning.
	atLimit := t.TempDir()
	mkRepoFile(t, filepath.Join(atLimit, "a.txt"), "a")
	mkRepoFile(t, filepath.Join(atLimit, "b.txt"), "b")
	files, incomplete, warnings := walkStaleScan(atLimit)
	if incomplete {
		t.Fatalf("at-limit walk must be complete, got warnings=%v", warnings)
	}
	if len(warnings) != 0 {
		t.Fatalf("at-limit walk must have no warnings, got %v", warnings)
	}
	if len(files) != 2 {
		t.Fatalf("at-limit files = %d, want 2 (root is not a file): %v", len(files), files)
	}

	// (b) Over the boundary: 5 files => root + a + b processed, c trips the
	//     cap. The walk stops, marks incomplete with a warning, and never
	//     observes files beyond the cap.
	overLimit := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"} {
		mkRepoFile(t, filepath.Join(overLimit, name), name)
	}
	files, incomplete, warnings = walkStaleScan(overLimit)
	if !incomplete {
		t.Fatal("over-limit walk must be marked incomplete")
	}
	if len(warnings) == 0 {
		t.Fatal("over-limit walk must carry at least one warning")
	}
	// Only root + a + b were observed; c/d/e must never have been read.
	for _, late := range []string{"c.txt", "d.txt", "e.txt"} {
		if _, ok := files[late]; ok {
			t.Fatalf("walk visited %q beyond the entry cap; files=%v", late, files)
		}
	}
	if len(files) != 2 {
		t.Fatalf("over-limit observed %d files, want exactly 2 (a,b): %v", len(files), files)
	}
}
