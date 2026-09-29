package tool

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/relationrecall"
)

// relFrontmatter renders the ONLY relation schema the temporal view parses:
// memory_relation: { kind: extends|updates, targets: [...] }. A bare top-level
// `extends: [...]` key is silently ignored by the loader — using it makes
// relation tests vacuous (self-only recall).
func relFrontmatter(kind string, targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("memory_relation:\n  kind: " + kind + "\n  targets:\n")
	for _, t := range targets {
		sb.WriteString("    - " + t + "\n")
	}
	return sb.String()
}

func writeRelConcept(t *testing.T, kb, id, title, state, kind string, targets []string) {
	t.Helper()
	content := "---\nokf_id: " + id + "\ntitle: \"" + title + "\"\ntype: note\nmemory_state: " + state + "\n" +
		relFrontmatter(kind, targets) + "---\n# " + title + "\nBody about " + title + ".\n"
	if err := os.WriteFile(filepath.Join(kb, id+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func relationHits(t *testing.T, resp ToolEnvelope) []relationrecall.Hit {
	t.Helper()
	res, ok := resp.Result.(*relationrecall.Result)
	if !ok {
		t.Fatalf("result is %T, not *relationrecall.Result", resp.Result)
	}
	return res.Hits
}

// TestRelationRecall_ServiceGraceful covers Service.RelationRecall degradation
// paths with the REAL memory_relation schema: unknown anchor, dangling extends,
// cycles, forks, empty anchor, and corrupted frontmatter. It asserts actual hit
// sets (not just OK) so a silently-ignored relation cannot make it vacuous.
func TestRelationRecall_ServiceGraceful(t *testing.T) {
	repo := initToolTestRepo(t)
	kb := filepath.Join(repo, ".okf", "knowledge")
	os.MkdirAll(kb, 0o755)

	const (
		idA = "okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		idB = "okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		idC = "okf_cccccccccccccccccccccccccccccccc"
		idD = "okf_dddddddddddddddddddddddddddddddd"
		idX = "okf_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	)

	// A extends B; B extends A (cycle) and C (fork).
	writeRelConcept(t, kb, idA, "A concept", "approved", "extends", []string{idB})
	writeRelConcept(t, kb, idB, "B concept", "approved", "extends", []string{idA, idC})
	writeRelConcept(t, kb, idC, "C concept", "approved", "", nil)
	// D extends idX which does not exist (dangling target).
	writeRelConcept(t, kb, idD, "D concept", "approved", "extends", []string{idX})

	t.Run("unknown_anchor_typed_error", func(t *testing.T) {
		resp := svcAt(repo).RelationRecall(t.Context(), RelationRecallRequest{Anchor: "okf_ffffffffffffffffffffffffffffffff"})
		if resp.OK {
			t.Fatalf("expected error for unknown anchor, got OK")
		}
		if resp.Error == nil {
			t.Fatal("expected typed error, got nil")
		}
		if !strings.Contains(resp.Error.Code, "not_found") {
			t.Fatalf("expected typed not_found error code, got %q", resp.Error.Code)
		}
	})

	t.Run("cycle_returns_both_directions", func(t *testing.T) {
		resp := svcAt(repo).RelationRecall(t.Context(), RelationRecallRequest{Anchor: idA})
		if !resp.OK {
			t.Fatalf("cycle should not fail: %+v", resp.Error)
		}
		hits := relationHits(t, resp)
		got := []string{hits[0].OKFID, hits[1].OKFID}
		// self(A) + B (both incoming A→B and outgoing B→A dedup to one hit).
		if len(hits) != 2 || got[0] != idA || got[1] != idB {
			t.Fatalf("cycle recall hits = %v, want [A B]", got)
		}
		if hits[1].Edge != "extends" {
			t.Fatalf("B edge = %q, want extends", hits[1].Edge)
		}
	})

	t.Run("fork_returns_all_neighbors", func(t *testing.T) {
		resp := svcAt(repo).RelationRecall(t.Context(), RelationRecallRequest{Anchor: idB})
		if !resp.OK {
			t.Fatalf("fork recall failed: %+v", resp.Error)
		}
		hits := relationHits(t, resp)
		got := make([]string, 0, len(hits))
		for _, h := range hits {
			got = append(got, h.OKFID)
		}
		// self(B) + A (incoming/outgoing) + C (incoming). Sorted for determinism.
		sort.Strings(got)
		want := []string{idA, idB, idC}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("fork recall hits = %v, want %v", got, want)
		}
	})

	t.Run("dangling_extends_graceful_self_only", func(t *testing.T) {
		resp := svcAt(repo).RelationRecall(t.Context(), RelationRecallRequest{Anchor: idD})
		if !resp.OK {
			t.Fatalf("dangling extends should be graceful: %+v", resp.Error)
		}
		hits := relationHits(t, resp)
		// D's extends target idX does not exist → only self hit, no panic.
		if len(hits) != 1 || hits[0].OKFID != idD {
			got := []string{}
			for _, h := range hits {
				got = append(got, h.OKFID)
			}
			t.Fatalf("dangling recall hits = %v, want [D] only", got)
		}
	})

	t.Run("empty_anchor_rejected", func(t *testing.T) {
		resp := svcAt(repo).RelationRecall(t.Context(), RelationRecallRequest{Anchor: ""})
		if resp.OK {
			t.Fatal("empty anchor should be rejected")
		}
		if !strings.Contains(resp.Error.Message, "anchor") {
			t.Fatalf("error should mention anchor, got: %+v", resp.Error)
		}
	})

	t.Run("corrupted_frontmatter_graceful", func(t *testing.T) {
		broken := `---
okf_id: okf_ffffffffffffffffffffffffffffffff
title: "Broken"
this is not valid yaml: [unclosed
---
# Broken
Body.
`
		if err := os.WriteFile(filepath.Join(kb, "broken.md"), []byte(broken), 0o644); err != nil {
			t.Fatal(err)
		}
		resp := svcAt(repo).RelationRecall(t.Context(), RelationRecallRequest{Anchor: idA})
		if !resp.OK {
			t.Fatalf("corrupted frontmatter should not break valid recall: %+v", resp.Error)
		}
		hits := relationHits(t, resp)
		if len(hits) != 2 || hits[0].OKFID != idA || hits[1].OKFID != idB {
			got := []string{}
			for _, h := range hits {
				got = append(got, h.OKFID)
			}
			t.Fatalf("recall after corrupt file = %v, want [A B]", got)
		}
	})
}

func svcAt(repo string) *Service { return NewService(Config{RepoPath: repo}) }
