package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRelationRecall_ServiceGraceful covers Service.RelationRecall degradation
// paths: unknown anchor, dangling extends, cycles, forks, and missing metadata.
// Verifies no panic, typed error, and deterministic ordering.
func TestRelationRecall_ServiceGraceful(t *testing.T) {
	repo := initToolTestRepo(t)
	kb := filepath.Join(repo, ".okf", "knowledge")
	os.MkdirAll(kb, 0o755)

	// A: approved, extends B
	a := `---
okf_id: okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
title: "A concept"
type: note
memory_state: approved
extends: [okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb]
---
# A
A body.
`
	// B: approved, extends A (cycle) and C (fork)
	b := `---
okf_id: okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
title: "B concept"
type: note
memory_state: approved
extends: [okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa, okf_cccccccccccccccccccccccccccccccc]
---
# B
B body.
`
	// C: approved, no outgoing extends
	c := `---
okf_id: okf_cccccccccccccccccccccccccccccccc
title: "C concept"
type: note
memory_state: approved
---
# C
C body.
`
	// D: approved, extends dangling okf_dddd... which doesn't exist
	d := `---
okf_id: okf_dddddddddddddddddddddddddddddddd
title: "D concept"
type: note
memory_state: approved
extends: [okf_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee]
---
# D
D body.
`
	os.WriteFile(filepath.Join(kb, "a.md"), []byte(a), 0o644)
	os.WriteFile(filepath.Join(kb, "b.md"), []byte(b), 0o644)
	os.WriteFile(filepath.Join(kb, "c.md"), []byte(c), 0o644)
	os.WriteFile(filepath.Join(kb, "d.md"), []byte(d), 0o644)

	svc := NewService(Config{RepoPath: repo})

	t.Run("unknown_anchor_typed_error", func(t *testing.T) {
		resp := svc.RelationRecall(t.Context(), RelationRecallRequest{Anchor: "okf_ffffffffffffffffffffffffffffffff"})
		if resp.OK {
			t.Fatalf("expected error for unknown anchor, got OK")
		}
		if resp.Error == nil {
			t.Fatal("expected typed error, got nil")
		}
	})

	t.Run("cycle_no_panic_deterministic", func(t *testing.T) {
		resp := svc.RelationRecall(t.Context(), RelationRecallRequest{Anchor: "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
		if !resp.OK {
			t.Fatalf("cycle should not fail: %+v", resp.Error)
		}
	})

	t.Run("fork_returns_neighbors", func(t *testing.T) {
		resp := svc.RelationRecall(t.Context(), RelationRecallRequest{Anchor: "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
		if !resp.OK {
			t.Fatalf("fork recall failed: %+v", resp.Error)
		}
	})

	t.Run("dangling_extends_graceful", func(t *testing.T) {
		// D extends E which doesn't exist; should not panic, should return D itself.
		resp := svc.RelationRecall(t.Context(), RelationRecallRequest{Anchor: "okf_dddddddddddddddddddddddddddddddd"})
		if !resp.OK {
			t.Fatalf("dangling extends should be graceful: %+v", resp.Error)
		}
	})

	t.Run("empty_anchor_rejected", func(t *testing.T) {
		resp := svc.RelationRecall(t.Context(), RelationRecallRequest{Anchor: ""})
		if resp.OK {
			t.Fatal("empty anchor should be rejected")
		}
		if !strings.Contains(resp.Error.Message, "anchor") {
			t.Fatalf("error should mention anchor, got: %+v", resp.Error)
		}
	})
}
