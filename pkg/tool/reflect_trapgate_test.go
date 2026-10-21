package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReflect_TrapGateDropsProposed verifies Service.Reflect drops non-approved
// concepts from evidence. The concepts are written with rich content so the
// lexical ranker actually returns them in round1, making the trapGate filter
// observable.
func TestReflect_TrapGateDropsProposed(t *testing.T) {
	repo := initToolTestRepo(t)
	kb := filepath.Join(repo, ".okf", "knowledge")
	os.MkdirAll(kb, 0o755)

	approved := `---
okf_id: okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
title: "PostgreSQL deployment guide"
type: note
memory_state: approved
---
# PostgreSQL deployment guide
How to deploy PostgreSQL in production. PostgreSQL deployment requires tuning shared_buffers, setting up replication, backing up PostgreSQL data, and configuring PostgreSQL security. This PostgreSQL deployment guide covers PostgreSQL setup.
`
	proposed := `---
okf_id: okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
title: "PostgreSQL poison note"
type: note
memory_state: proposed
---
# PostgreSQL poison note
This is a proposed untrusted PostgreSQL note about PostgreSQL deployment that should not be trusted. Poison PostgreSQL content.
`
	os.WriteFile(filepath.Join(kb, "approved.md"), []byte(approved), 0o644)
	os.WriteFile(filepath.Join(kb, "proposed.md"), []byte(proposed), 0o644)

	svc := NewService(Config{RepoPath: repo})
	resp := svc.Reflect(t.Context(), ReflectRequest{
		Question:    "PostgreSQL deployment guide",
		MinEvidence: 1,
	})
	if !resp.OK {
		t.Fatalf("reflect failed: %+v", resp.Error)
	}
	result, ok := resp.Result.(ReflectResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resp.Result)
	}
	ids := []string{}
	for _, ev := range result.Evidence {
		ids = append(ids, ev.ID)
	}
	t.Logf("evidence IDs: %v", ids)
	if !strings.Contains(strings.Join(ids, ","), "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatal("approved concept missing from evidence — test setup broken")
	}
	if strings.Contains(strings.Join(ids, ","), "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Error("proposed (poison) concept leaked into evidence!")
	}
}
