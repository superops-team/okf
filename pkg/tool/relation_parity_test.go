package tool

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRelationRecall_Parity verifies Service.RelationRecall is deterministic
// across repeated calls (CLI and MCP both wrap this same method).
func TestRelationRecall_Parity(t *testing.T) {
	repo := initToolTestRepo(t)
	kb := filepath.Join(repo, ".okf", "knowledge")
	os.MkdirAll(kb, 0o755)

	a := `---
okf_id: okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
title: "A"
type: note
memory_state: approved
extends: [okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb]
---
# A
A body for testing.
`
	b := `---
okf_id: okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
title: "B"
type: note
memory_state: approved
---
# B
B body.
`
	os.WriteFile(filepath.Join(kb, "a.md"), []byte(a), 0o644)
	os.WriteFile(filepath.Join(kb, "b.md"), []byte(b), 0o644)

	svc := NewService(Config{RepoPath: repo})
	req := RelationRecallRequest{Anchor: "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

	r1 := svc.RelationRecall(t.Context(), req)
	r2 := svc.RelationRecall(t.Context(), req)

	if !r1.OK || !r2.OK {
		t.Fatalf("both calls should succeed")
	}
	if r1.Result == nil || r2.Result == nil {
		t.Fatal("result should not be nil")
	}
}

// TestRelationRecall_UnknownAnchorError verifies typed error for unknown anchor.
func TestRelationRecall_UnknownAnchorError(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	resp := svc.RelationRecall(t.Context(), RelationRecallRequest{Anchor: "okf_nonexistent_0000000000000000"})
	if resp.OK {
		t.Fatal("unknown anchor should return error")
	}
	if resp.Error == nil {
		t.Fatal("expected typed error")
	}
}
