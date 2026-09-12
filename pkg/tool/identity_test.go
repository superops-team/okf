package tool

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/superops-team/okf/pkg/identity"
)

var okfIDPattern = regexp.MustCompile(`(?m)^okf_id:\s*(okf_[0-9a-f]{32})\s*$`)

func readPersistedConceptID(t *testing.T, repo, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, ".okf", "knowledge", relPath))
	if err != nil {
		t.Fatalf("read persisted concept: %v", err)
	}
	m := okfIDPattern.FindSubmatch(data)
	if m == nil {
		t.Fatalf("persisted concept %s carries no okf_id:\n%s", relPath, data)
	}
	return string(m[1])
}

// S12: every owned durable writer (note/event/feedback) persists a new final
// Concept with one valid okf_id, alongside its unchanged deterministic
// concept_id.
func TestOwnedWritersPersistStableID(t *testing.T) {
	cases := []struct {
		name string
		kind string
	}{
		{"note", "note"},
		{"event", "event"},
		{"feedback", "feedback"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initToolTestRepo(t)
			svc := NewService(Config{RepoPath: repo})
			content := "Owned " + tc.kind + " content for stable identity."
			resp := svc.WriteKnowledge(context.Background(), WriteKnowledgeRequest{
				Kind:           tc.kind,
				Content:        content,
				IdempotencyKey: tc.kind + "-stable-id-v1",
			})
			if !resp.OK {
				t.Fatalf("write %s failed: %#v", tc.kind, resp.Error)
			}
			result := requireWriteResult(t, resp)
			id := readPersistedConceptID(t, repo, result.ConceptPath)
			if _, err := identity.Parse(id); err != nil {
				t.Fatalf("persisted %s okf_id %q invalid: %v", tc.kind, id, err)
			}
			// The deterministic concept_id is preserved as a separate handle.
			if result.ConceptID == "" || result.ConceptID == id {
				t.Fatalf("concept_id must remain a separate stable handle: %q", result.ConceptID)
			}
		})
	}
}

// S12: re-writing the same owned target preserves its existing okf_id.
func TestOwnedWriterRefreshPreservesStableID(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	req := WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "Refreshed durable note content.",
		IdempotencyKey: "note-refresh-preserves-id",
	}
	first := svc.WriteKnowledge(context.Background(), req)
	if !first.OK {
		t.Fatalf("first write failed: %#v", first.Error)
	}
	firstResult := requireWriteResult(t, first)
	idBefore := readPersistedConceptID(t, repo, firstResult.ConceptPath)

	// Idempotent retry of the same target reuses the existing file.
	second := svc.WriteKnowledge(context.Background(), req)
	if !second.OK {
		t.Fatalf("refresh write failed: %#v", second.Error)
	}
	idAfter := readPersistedConceptID(t, repo, firstResult.ConceptPath)
	if idBefore != idAfter {
		t.Fatalf("okf_id changed across refresh: before=%q after=%q", idBefore, idAfter)
	}
}
