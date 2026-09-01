package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/superops-team/okf/pkg/okf"
)

func TestWriteKnowledgeIdempotentAndConflict(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	req := WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "Prefer structured facts over raw command strings.",
		Project:        "sample-project",
		Tags:           []string{"runtime", "runtime"},
		IdempotencyKey: "note-runtime-facts-v1",
	}

	first := svc.WriteKnowledge(t.Context(), req)
	if !first.OK {
		t.Fatalf("first write failed: %#v", first.Error)
	}
	second := svc.WriteKnowledge(t.Context(), req)
	if !second.OK {
		t.Fatalf("idempotent retry failed: %#v", second.Error)
	}
	firstResult := requireWriteResult(t, first)
	secondResult := requireWriteResult(t, second)
	if firstResult.ConceptID != secondResult.ConceptID || firstResult.ConceptPath != secondResult.ConceptPath {
		t.Fatalf("idempotent identity differs: first=%#v second=%#v", firstResult, secondResult)
	}
	if !firstResult.Created || secondResult.Created {
		t.Fatalf("created flags = %v/%v, want true/false", firstResult.Created, secondResult.Created)
	}

	conflictReq := req
	conflictReq.Content = "Different content must not overwrite the original."
	conflict := svc.WriteKnowledge(t.Context(), conflictReq)
	if conflict.OK || conflict.Error == nil || conflict.Error.Code != ErrIdempotencyConflict {
		t.Fatalf("conflict = %#v, want %q", conflict, ErrIdempotencyConflict)
	}

	query := NewService(Config{RepoPath: repo}).Query(t.Context(), QueryRequest{
		Query: "structured facts",
		Types: []string{"note"},
	})
	if !query.OK {
		t.Fatalf("query after restart failed: %#v", query.Error)
	}
	results := query.Result.(QueryResult).Results
	if len(results) != 1 || results[0].Type != "note" {
		t.Fatalf("query results = %#v, want one note", results)
	}
}

func TestWriteFeedbackEvidenceRefs(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	resp := svc.WriteKnowledge(context.Background(), WriteKnowledgeRequest{
		Kind:           "feedback",
		Content:        "Fail closed when execution semantics cannot be parsed.",
		Project:        "sample-project",
		IdempotencyKey: "feedback-fail-closed-v1",
		Metadata:       map[string]any{"category": "verification", "principle": "fail closed"},
		EvidenceRefs:   []string{"prd-spec/refactors/example.md#decision-1", "tests/verification_test.go"},
	})
	if !resp.OK {
		t.Fatalf("write feedback failed: %#v", resp.Error)
	}

	bundle, err := loadKnowledgeBundleForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Concepts) != 1 {
		t.Fatalf("concept count = %d, want 1", len(bundle.Concepts))
	}
	concept := bundle.Concepts[0]
	if concept.Type != "feedback" {
		t.Fatalf("type = %q, want feedback", concept.Type)
	}
	provenance, ok := concept.CustomFields["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("provenance = %#v", concept.CustomFields["provenance"])
	}
	if provenance["category"] != "verification" || provenance["principle"] != "fail closed" {
		t.Fatalf("provenance = %#v", provenance)
	}
	refs, ok := provenance["evidence_refs"].([]any)
	if !ok || len(refs) != 2 {
		t.Fatalf("evidence refs = %#v, want two refs", provenance["evidence_refs"])
	}
}

func TestWriteKnowledgeConcurrentSameKey(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	req := WriteKnowledgeRequest{Kind: "event", Content: "deployment finished", IdempotencyKey: "event-deploy-v1"}

	const writers = 8
	results := make(chan ToolEnvelope, writers)
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() { results <- svc.WriteKnowledge(t.Context(), req) })
	}
	wg.Wait()
	close(results)
	created := 0
	for result := range results {
		if !result.OK {
			t.Fatalf("concurrent write failed: %#v", result.Error)
		}
		if requireWriteResult(t, result).Created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created count = %d, want 1", created)
	}
	bundle, err := loadKnowledgeBundleForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Concepts) != 1 {
		t.Fatalf("concept count = %d, want 1", len(bundle.Concepts))
	}
}

func TestWriteKnowledgeCommitFailureLeavesNoHalfWrite(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	svc.writeKnowledgeFile = func(string, []byte) error { return os.ErrPermission }

	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind: "note", Content: "must remain invisible", IdempotencyKey: "failed-note-v1",
	})
	if resp.OK || resp.Error == nil {
		t.Fatalf("response = %#v, want structured failure", resp)
	}
	query := NewService(Config{RepoPath: repo}).Query(t.Context(), QueryRequest{Query: "must remain invisible", Types: []string{"note"}})
	if query.OK || query.Error == nil || query.Error.Code != ErrKnowledgeNotInitialized {
		t.Fatalf("query = %#v, want knowledge_not_initialized after failed commit", query)
	}
}

func TestWriteKnowledgePostRenameFailureRollsBackCommittedFile(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	svc.writeKnowledgeFile = func(path string, data []byte) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		return errors.New("simulated post-rename sync failure")
	}

	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind: "note", Content: "must roll back after commit-stage failure", IdempotencyKey: "post-rename-failure-v1",
	})
	if resp.OK || resp.Error == nil {
		t.Fatalf("response = %#v, want structured failure", resp)
	}
	if entries := regularFilesUnder(t, filepath.Join(repo, ".okf", "knowledge")); len(entries) != 0 {
		t.Fatalf("persisted files after post-rename failure = %v, want none", entries)
	}
}

func TestWriteKnowledgeVerificationFailureRollsBackCommittedFile(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	svc.writeKnowledgeFile = func(path string, _ []byte) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte("not valid OKF frontmatter"), 0o644)
	}

	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind: "note", Content: "must roll back after verification failure", IdempotencyKey: "verify-failure-v1",
	})
	if resp.OK || resp.Error == nil {
		t.Fatalf("response = %#v, want structured verification failure", resp)
	}
	knowledgeRoot := filepath.Join(repo, ".okf", "knowledge")
	if entries := regularFilesUnder(t, knowledgeRoot); len(entries) != 0 {
		t.Fatalf("persisted files after verification failure = %v, want none", entries)
	}
	query := NewService(Config{RepoPath: repo}).Query(t.Context(), QueryRequest{Query: "roll back", Types: []string{"note"}})
	if !query.OK {
		t.Fatalf("query after rollback failed: %#v", query.Error)
	}
	if results := query.Result.(QueryResult).Results; len(results) != 0 {
		t.Fatalf("query results after rollback = %#v, want none", results)
	}
}

func regularFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk knowledge root: %v", err)
	}
	return files
}

func TestWriteKnowledgeRejectsSymlinkKnowledgeRoot(t *testing.T) {
	repo := initToolTestRepo(t)
	outside := t.TempDir()
	link := filepath.Join(repo, "linked-knowledge")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	resp := NewService(Config{RepoPath: repo, KnowledgeDir: link}).WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind: "note", Content: "must not escape", IdempotencyKey: "escape-v1",
	})
	if resp.OK || resp.Error == nil || resp.Error.Code != ErrKnowledgePathOutside {
		t.Fatalf("response = %#v, want %q", resp, ErrKnowledgePathOutside)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory contains %d entries, want none", len(entries))
	}
}

func TestWriteKnowledgePreservesValidUTF8InDerivedFields(t *testing.T) {
	repo := initToolTestRepo(t)
	content := "x" + strings.Repeat("知识", 100) + "\n正文"
	resp := NewService(Config{RepoPath: repo}).WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind: "note", Content: content, IdempotencyKey: "utf8-derived-fields-v1",
	})
	if !resp.OK {
		t.Fatalf("write failed: %#v", resp.Error)
	}
	bundle, err := loadKnowledgeBundleForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Concepts) != 1 {
		t.Fatalf("concept count = %d, want 1", len(bundle.Concepts))
	}
	concept := bundle.Concepts[0]
	if !utf8.ValidString(concept.Title) || !utf8.ValidString(concept.Description) {
		t.Fatalf("derived fields contain invalid UTF-8: title=%q description=%q", concept.Title, concept.Description)
	}
}

func TestWriteKnowledgeValidationMatrix(t *testing.T) {
	repo := initToolTestRepo(t)
	tests := []struct {
		name string
		req  WriteKnowledgeRequest
	}{
		{name: "empty content", req: WriteKnowledgeRequest{Kind: "note", IdempotencyKey: "k"}},
		{name: "unknown kind", req: WriteKnowledgeRequest{Kind: "memo", Content: "x", IdempotencyKey: "k"}},
		{name: "missing key", req: WriteKnowledgeRequest{Kind: "note", Content: "x"}},
		{name: "credential metadata", req: WriteKnowledgeRequest{Kind: "note", Content: "x", IdempotencyKey: "k", Metadata: map[string]any{"api_token": "secret"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewService(Config{RepoPath: repo}).WriteKnowledge(t.Context(), tt.req)
			if resp.OK || resp.Error == nil || resp.Error.Code != ErrInvalidRequest {
				t.Fatalf("response = %#v, want invalid_request", resp)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(repo, ".okf", "knowledge")); !os.IsNotExist(err) {
		t.Fatalf("invalid writes created knowledge dir: %v", err)
	}
}

func requireWriteResult(t *testing.T, envelope ToolEnvelope) WriteKnowledgeResult {
	t.Helper()
	result, ok := envelope.Result.(WriteKnowledgeResult)
	if !ok {
		t.Fatalf("result type = %T, want WriteKnowledgeResult", envelope.Result)
	}
	return result
}

func loadKnowledgeBundleForTest(repo string) (*okf.KnowledgeBundle, error) {
	return okf.LoadBundle(filepath.Join(repo, ".okf", "knowledge"), okf.DefaultLoadOptions())
}
