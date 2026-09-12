package tool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	querypkg "github.com/superops-team/okf/pkg/query"
)

// groupingFixture seeds three concepts: two share one source_path (src/doc1.md)
// and a third lives in src/doc2.md. Querying "ParityToken" matches all three by
// description, so source projection collapses the first two into one group.
func groupingFixture(t *testing.T) string {
	t.Helper()
	repo := initToolTestRepo(t)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "one.md"), `---
type: concept
title: Alpha Doc One
description: ParityToken first half of doc1
source_path: src/doc1.md
---
ParityToken body alpha one.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "two.md"), `---
type: concept
title: Alpha Doc Two
description: ParityToken second half of doc1
source_path: src/doc1.md
---
ParityToken body alpha two.
`)
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "concepts", "three.md"), `---
type: concept
title: Beta Doc
description: ParityToken lives in doc2
source_path: src/doc2.md
---
ParityToken body beta.
`)
	return repo
}

// TestGroupingOmittedCompatibility (S27): when group_by is omitted the grouped
// field must be absent and the results must be byte-for-byte identical to the
// pre-grouping ungrouped path.
func TestGroupingOmittedCompatibility(t *testing.T) {
	repo := groupingFixture(t)
	svc := NewService(Config{RepoPath: repo})

	resp := svc.Query(context.Background(), QueryRequest{Query: "ParityToken", Limit: 5})
	if !resp.OK {
		t.Fatalf("query OK = false, error = %#v", resp.Error)
	}
	result, ok := resp.Result.(QueryResult)
	if !ok {
		t.Fatalf("result type = %T, want QueryResult", resp.Result)
	}
	if result.Groups != nil {
		t.Fatalf("Groups = %v, want nil when group_by omitted", result.Groups)
	}
	if len(result.Results) != 3 {
		t.Fatalf("ungrouped results = %d, want 3", len(result.Results))
	}

	// Deterministic across repeated calls.
	again := svc.Query(context.Background(), QueryRequest{Query: "ParityToken", Limit: 5})
	againResult := again.Result.(QueryResult)
	for i := range result.Results {
		if result.Results[i].Location != againResult.Results[i].Location ||
			result.Results[i].Score != againResult.Results[i].Score {
			t.Fatalf("ungrouped results not deterministic at %d: %+v vs %+v",
				i, result.Results[i], againResult.Results[i])
		}
	}

	// The "groups" key must not appear in the JSON envelope (omitempty).
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var resultObj map[string]json.RawMessage
	if err := json.Unmarshal(raw["result"], &resultObj); err != nil {
		t.Fatal(err)
	}
	if _, present := resultObj["groups"]; present {
		t.Fatalf("result JSON contains \"groups\" key when omitted: %s", data)
	}
}

// TestGroupingEntryPointParity (S34): the Service path and the shared
// query.Project engine must produce identical group keys, representatives and
// counts for the same hits. The MCP and CLI entry points are covered by their
// own parity tests against this same adapter.
func TestGroupingEntryPointParity(t *testing.T) {
	repo := groupingFixture(t)
	svc := NewService(Config{RepoPath: repo})

	resp := svc.Query(context.Background(), QueryRequest{
		Query:               "ParityToken",
		Limit:               5,
		GroupBy:             "source",
		IncludeGroupMembers: true,
	})
	if !resp.OK {
		t.Fatalf("grouped query OK = false, error = %#v", resp.Error)
	}
	result := resp.Result.(QueryResult)
	if len(result.Groups) != 2 {
		t.Fatalf("source groups = %d, want 2", len(result.Groups))
	}

	// Group 0 = src/doc1.md (two hits), group 1 = src/doc2.md (one hit),
	// ordered by representative rank.
	g0, g1 := result.Groups[0], result.Groups[1]
	if g0.GroupKey != "src:src/doc1.md" {
		t.Fatalf("group0 key = %q, want src:src/doc1.md", g0.GroupKey)
	}
	if g0.HitCount != 2 || g0.SourceCount != 1 || g0.ConceptCount != 2 {
		t.Fatalf("group0 counts = hits:%d concepts:%d sources:%d, want 2/2/1",
			g0.HitCount, g0.ConceptCount, g0.SourceCount)
	}
	if g1.GroupKey != "src:src/doc2.md" {
		t.Fatalf("group1 key = %q, want src:src/doc2.md", g1.GroupKey)
	}
	if g1.HitCount != 1 {
		t.Fatalf("group1 hit_count = %d, want 1", g1.HitCount)
	}
	// Representatives equal the backward-compatible results list.
	if len(result.Results) != len(result.Groups) {
		t.Fatalf("results(%d) vs groups(%d) length mismatch", len(result.Results), len(result.Groups))
	}
	if result.Results[0].SourcePath != "src/doc1.md" {
		t.Fatalf("representative source = %q, want src/doc1.md", result.Results[0].SourcePath)
	}
	// Members are populated when requested.
	if len(g0.Members) != 2 {
		t.Fatalf("group0 members = %d, want 2", len(g0.Members))
	}

	// Parity with the raw engine: re-adapt the ungrouped hits and project
	// directly; keys and counts must match the Service output.
	ungrouped := svc.Query(context.Background(), QueryRequest{Query: "ParityToken", Limit: 5}).Result.(QueryResult)
	pool := make([]querypkg.ResultHit, len(ungrouped.Results))
	for i, h := range ungrouped.Results {
		pool[i] = querypkg.ResultHit{
			OKFID:             h.okfID,
			ParentOKFID:       h.parentOKFID,
			LegacyFingerprint: h.legacyFingerprint,
			Ref:               h.Location,
			ConceptPath:       h.ConceptPath,
			SourcePath:        h.SourcePath,
			StartLine:         h.StartLine,
			EndLine:           h.EndLine,
			Rank:              i + 1,
			Score:             float64(h.Score),
			Provenance:        h.Provenance,
		}
	}
	engineGroups, _, err := querypkg.Project(pool, querypkg.GroupSource, true, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(engineGroups) != len(result.Groups) {
		t.Fatalf("engine groups %d != service groups %d", len(engineGroups), len(result.Groups))
	}
	for i := range engineGroups {
		if engineGroups[i].GroupKey != result.Groups[i].GroupKey {
			t.Fatalf("engine group %d key %q != service %q", i, engineGroups[i].GroupKey, result.Groups[i].GroupKey)
		}
		if engineGroups[i].HitCount != result.Groups[i].HitCount {
			t.Fatalf("engine group %d hit_count %d != service %d", i, engineGroups[i].HitCount, result.Groups[i].HitCount)
		}
	}
}

// TestGroupingInvalidGroupByRejected (S33 wiring at Service layer).
func TestGroupingInvalidGroupByRejected(t *testing.T) {
	repo := groupingFixture(t)
	svc := NewService(Config{RepoPath: repo})
	resp := svc.Query(context.Background(), QueryRequest{Query: "ParityToken", GroupBy: "document"})
	if resp.OK {
		t.Fatal("invalid group_by should fail")
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidGroupBy {
		t.Fatalf("error = %#v, want code %q", resp.Error, ErrInvalidGroupBy)
	}
}
