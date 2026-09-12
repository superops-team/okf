package query

import (
	"encoding/json"
	"testing"
)

const testOKFID = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"

func hit(rank int, score float64, fn func(*ResultHit)) ResultHit {
	h := ResultHit{Rank: rank, Score: score, Provenance: "semantic"}
	if fn != nil {
		fn(&h)
	}
	return h
}

func TestProjectChunk(t *testing.T) {
	hits := []ResultHit{
		hit(1, 0.9, func(h *ResultHit) { h.OKFID = testOKFID; h.ConceptPath = "c/a.md"; h.StartLine = 1; h.EndLine = 10 }),
		hit(2, 0.8, func(h *ResultHit) { h.OKFID = testOKFID; h.ConceptPath = "c/a.md"; h.StartLine = 11; h.EndLine = 20 }),
	}
	groups, warns, err := Project(hits, GroupChunk, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("chunk groups = %d, want 2 (each exposed hit distinct)", len(groups))
	}
	if groups[0].GroupKey != "v3:id:"+testOKFID+"@1:10" {
		t.Fatalf("key0 = %q", groups[0].GroupKey)
	}
	if groups[1].GroupKey != "v3:id:"+testOKFID+"@11:20" {
		t.Fatalf("key1 = %q", groups[1].GroupKey)
	}
	if groups[0].HitCount != 1 || groups[1].HitCount != 1 {
		t.Fatal("ranges must remain distinct chunk groups")
	}
	// Representative preserves location and score.
	if groups[0].Representative.StartLine != 1 || groups[0].Representative.Score != 0.9 {
		t.Fatalf("representative lost location/score: %+v", groups[0].Representative)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
}

func TestProjectConcept(t *testing.T) {
	// A parent concept plus its derived chunk (carrying parent_okf_id) collapse
	// into one concept group keyed by the parent id.
	hits := []ResultHit{
		hit(1, 0.9, func(h *ResultHit) { h.OKFID = testOKFID; h.ConceptPath = "c/a.md" }),
		hit(2, 0.8, func(h *ResultHit) {
			h.ParentOKFID = testOKFID
			h.LegacyFingerprint = "x:y:c/a__c1.md"
			h.ConceptPath = "c/a__c1.md"
		}),
		hit(3, 0.7, func(h *ResultHit) {
			h.LegacyFingerprint = "src:c/b.md"
			h.ConceptPath = "c/b.md"
		}),
	}
	groups, _, err := Project(hits, GroupConcept, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("concept groups = %d, want 2", len(groups))
	}
	if groups[0].GroupKey != "id:"+testOKFID {
		t.Fatalf("group0 key = %q", groups[0].GroupKey)
	}
	if groups[0].HitCount != 2 {
		t.Fatalf("parent+chunk hit_count = %d, want 2", groups[0].HitCount)
	}
	if groups[1].GroupKey != "legacy:src:c/b.md" {
		t.Fatalf("legacy fallback key = %q", groups[1].GroupKey)
	}
	// Representative is the first raw member (rank 1).
	if groups[0].Representative.Rank != 1 {
		t.Fatalf("representative rank = %d, want 1", groups[0].Representative.Rank)
	}
}

func TestProjectSource(t *testing.T) {
	hits := []ResultHit{
		hit(1, 0.9, func(h *ResultHit) { h.OKFID = testOKFID; h.ConceptPath = "c/a.md"; h.SourcePath = "src/doc1.md" }),
		hit(2, 0.85, func(h *ResultHit) {
			h.ParentOKFID = testOKFID
			h.ConceptPath = "c/a__c1.md"
			h.SourcePath = "src/doc1.md"
		}),
		hit(3, 0.7, func(h *ResultHit) {
			h.LegacyFingerprint = "x:y:src/doc2.md"
			h.ConceptPath = "c/b.md"
			h.SourcePath = "src/doc2.md"
		}),
	}
	groups, _, err := Project(hits, GroupSource, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("source groups = %d, want 2", len(groups))
	}
	if groups[0].GroupKey != "src:src/doc1.md" {
		t.Fatalf("source key0 = %q", groups[0].GroupKey)
	}
	if groups[0].HitCount != 2 {
		t.Fatalf("same-source group hit_count = %d, want 2", groups[0].HitCount)
	}
	if groups[0].SourceCount != 1 {
		t.Fatalf("source_count = %d, want 1", groups[0].SourceCount)
	}
}

func TestProjectFolder(t *testing.T) {
	hits := []ResultHit{
		hit(1, 0.9, func(h *ResultHit) { h.ConceptPath = "notes/alpha.md"; h.SourcePath = "refs/a.md" }),
		hit(2, 0.8, func(h *ResultHit) { h.ConceptPath = "notes/sub/beta.md"; h.SourcePath = "refs/sub/b.md" }),
		hit(3, 0.7, func(h *ResultHit) { h.ConceptPath = "root.md" }),
		// Absolute path must not become a folder key → fallback + warning.
		hit(4, 0.6, func(h *ResultHit) { h.ConceptPath = "/etc/passwd.md" }),
	}
	groups, warns, err := Project(hits, GroupFolder, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	// refs/a.md -> refs; refs/sub/b.md -> refs/sub; root.md -> .; the absolute
	// path falls back to concept grouping (its own key), so 4 groups.
	if len(groups) != 4 {
		t.Fatalf("folder groups = %d, want 4", len(groups))
	}
	if groups[0].GroupKey != "folder:refs" {
		t.Fatalf("key0 = %q", groups[0].GroupKey)
	}
	if groups[1].GroupKey != "folder:refs/sub" {
		t.Fatalf("key1 = %q", groups[1].GroupKey)
	}
	if groups[2].GroupKey != "folder:." {
		t.Fatalf("root folder key = %q, want '.'", groups[2].GroupKey)
	}
	if len(warns) == 0 {
		t.Fatal("absolute path fallback should emit a warning")
	}
}

func TestProjectDeterministic(t *testing.T) {
	hits := []ResultHit{
		hit(5, 0.5, func(h *ResultHit) { h.LegacyFingerprint = "d:fourth:z"; h.ConceptPath = "z.md" }),
		hit(1, 0.9, func(h *ResultHit) { h.LegacyFingerprint = "a:first:a"; h.ConceptPath = "a.md"; h.SourcePath = "s/x" }),
		hit(2, 0.8, func(h *ResultHit) { h.LegacyFingerprint = "b:second:b"; h.ConceptPath = "b.md"; h.SourcePath = "s/x" }),
		hit(3, 0.7, func(h *ResultHit) { h.LegacyFingerprint = "c:third:c"; h.ConceptPath = "c.md" }),
	}
	run := func() []GroupedHit {
		g, _, err := Project(hits, GroupConcept, true, 10)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	first := run()
	for i := 0; i < 5; i++ {
		again := run()
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(again)
		if string(a) != string(b) {
			t.Fatalf("projection not deterministic:\n%s\n%s", a, b)
		}
	}
	// Representative rank ordering: groups ordered by first member rank.
	if first[0].Representative.Rank != 1 {
		t.Fatalf("first group rep rank = %d, want 1", first[0].Representative.Rank)
	}
}

func TestProjectLimitAndOmittedMembers(t *testing.T) {
	hits := []ResultHit{
		hit(1, 0.9, func(h *ResultHit) { h.LegacyFingerprint = "k1"; h.ConceptPath = "a.md" }),
		hit(2, 0.8, func(h *ResultHit) { h.LegacyFingerprint = "k2"; h.ConceptPath = "b.md" }),
		hit(3, 0.7, func(h *ResultHit) { h.LegacyFingerprint = "k3"; h.ConceptPath = "c.md" }),
	}
	groups, _, err := Project(hits, GroupConcept, false, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("limited groups = %d, want 2", len(groups))
	}
	if groups[0].Members != nil {
		t.Fatal("includeMembers=false must omit member list")
	}
	if groups[0].HitCount != 1 {
		t.Fatal("counts must still be computed when members omitted")
	}
}

func TestProjectInvalidGroup(t *testing.T) {
	_, _, err := Project(nil, GroupBy("document"), false, 10)
	if err == nil {
		t.Fatal("invalid group_by should error")
	}
	var ge *GroupError
	if !asGroupError(err, &ge) || ge.GroupBy != "document" {
		t.Fatalf("err = %v, want invalid_group_by", err)
	}
}

// Property: path normalization is idempotent and rejects unsafe paths.
func TestProjectPathNormalizationProperties(t *testing.T) {
	normal := []string{"a/b.md", "a", "./c/d.md", "a/b/../c.md"}
	for _, p := range normal {
		got, ok := normalizeRelPath(p)
		if !ok {
			t.Errorf("normalizeRelPath(%q) rejected", p)
			continue
		}
		again, ok2 := normalizeRelPath(got)
		if !ok2 || again != got {
			t.Errorf("normalizeRelPath not idempotent on %q -> %q -> %q", p, got, again)
		}
	}
	unsafe := []string{"/abs/path.md", "../escape.md", "a/../../escape.md", ".."}
	for _, p := range unsafe {
		if _, ok := normalizeRelPath(p); ok {
			t.Errorf("normalizeRelPath(%q) should be rejected", p)
		}
	}
}

// Property: projection is idempotent — projecting the projected representatives
// back over the same pool yields the same group keys for each representative.
func TestProjectIdempotenceProperty(t *testing.T) {
	hits := []ResultHit{
		hit(1, 0.9, func(h *ResultHit) { h.LegacyFingerprint = "k1"; h.ConceptPath = "a.md"; h.SourcePath = "s/x" }),
		hit(2, 0.8, func(h *ResultHit) { h.LegacyFingerprint = "k1"; h.ConceptPath = "a.md"; h.SourcePath = "s/x" }),
		hit(3, 0.7, func(h *ResultHit) { h.LegacyFingerprint = "k2"; h.ConceptPath = "b.md"; h.SourcePath = "s/y" }),
	}
	groups, _, err := Project(hits, GroupSource, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	reps := make([]ResultHit, len(groups))
	for i, g := range groups {
		reps[i] = g.Representative
	}
	again, _, err := Project(reps, GroupSource, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(groups) {
		t.Fatalf("idempotence: group count %d vs %d", len(again), len(groups))
	}
	for i := range groups {
		if again[i].GroupKey != groups[i].GroupKey {
			t.Fatalf("idempotence: key %q vs %q", again[i].GroupKey, groups[i].GroupKey)
		}
	}
}

func asGroupError(err error, target **GroupError) bool {
	for err != nil {
		if e, ok := err.(*GroupError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
