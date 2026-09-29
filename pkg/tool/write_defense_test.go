package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/memorydefense"
)

func setupDefenseRepo(t *testing.T, action string) string {
	t.Helper()
	repo := initToolTestRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".okf", "knowledge", "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "memory_defense:\n  enabled: true\n  action: " + action + "\n"
	if err := os.WriteFile(filepath.Join(repo, ".okf", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestWriteKnowledge_DefenseRedactsAndPersists(t *testing.T) {
	repo := setupDefenseRepo(t, "redact")
	svc := NewService(Config{RepoPath: repo})
	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "my token is ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234 please note",
		IdempotencyKey: "defense-redact-1",
	})
	if !resp.OK {
		t.Fatalf("write failed: %+v", resp.Error)
	}
	res := resp.Result.(WriteKnowledgeResult)
	if len(res.Redactions) == 0 || res.Redactions[0].DetectorID != "github_pat" {
		t.Fatalf("redactions = %+v", res.Redactions)
	}
	data, err := os.ReadFile(filepath.Join(repo, ".okf", "knowledge", res.ConceptPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234") {
		t.Fatal("persisted file still contains the secret")
	}
	if !strings.Contains(string(data), "[REDACTED:github_pat]") {
		t.Fatalf("persisted file missing redaction placeholder: %s", data)
	}
}

func TestWriteKnowledge_DefenseBlockZeroBytes(t *testing.T) {
	repo := setupDefenseRepo(t, "block")
	svc := NewService(Config{RepoPath: repo})
	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "use AKIAIOSFODNN7EXAMPLE for s3",
		IdempotencyKey: "defense-block-1",
	})
	if resp.OK {
		t.Fatal("expected block failure")
	}
	if resp.Error.Code != ErrMemoryDefenseBlocked {
		t.Fatalf("code = %q, want %q", resp.Error.Code, ErrMemoryDefenseBlocked)
	}
	if strings.Contains(resp.Error.Message+" "+resp.Error.Remediation, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal("block error leaked the secret")
	}
	// No file should exist.
	matches, _ := filepath.Glob(filepath.Join(repo, ".okf", "knowledge", "notes", "*.md"))
	if len(matches) != 0 {
		t.Fatalf("block left files on disk: %v", matches)
	}
}

func TestWriteKnowledge_RedactionIdempotencyRetry(t *testing.T) {
	repo := setupDefenseRepo(t, "redact")
	svc := NewService(Config{RepoPath: repo})
	req := WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234",
		IdempotencyKey: "idem-defense-1",
	}
	r1 := svc.WriteKnowledge(t.Context(), req)
	if !r1.OK {
		t.Fatalf("first write failed: %+v", r1.Error)
	}
	// Retry with the same idempotency key and same original content.
	r2 := svc.WriteKnowledge(t.Context(), req)
	if !r2.OK {
		t.Fatalf("retry failed: %+v", r2.Error)
	}
	r2res := r2.Result.(WriteKnowledgeResult)
	if r2res.Created {
		t.Fatal("retry should reuse existing concept (Created=false)")
	}
}

func TestWriteKnowledge_DefenseDisabledDefault(t *testing.T) {
	// No .okf/config.yaml — defense disabled.
	repo := initToolTestRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".okf", "knowledge", "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{RepoPath: repo})
	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234",
		IdempotencyKey: "no-defense-1",
	})
	if !resp.OK {
		t.Fatalf("write with defense disabled should succeed: %+v", resp.Error)
	}
	res := resp.Result.(WriteKnowledgeResult)
	if len(res.Redactions) != 0 {
		t.Fatalf("disabled defense should not report redactions: %+v", res.Redactions)
	}
	data, _ := os.ReadFile(filepath.Join(repo, ".okf", "knowledge", res.ConceptPath))
	if !strings.Contains(string(data), "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234") {
		t.Fatal("with defense disabled, original secret should be persisted as-is")
	}
}

func TestWriteKnowledge_RedactionEmptyRejected(t *testing.T) {
	repo := setupDefenseRepo(t, "redact")
	svc := NewService(Config{RepoPath: repo})
	// Entire content is a single GitHub PAT with no surrounding text.
	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefgh1234",
		IdempotencyKey: "redact-empty-1",
	})
	if resp.OK {
		t.Fatal("expected redaction_empty failure")
	}
	if resp.Error.Code != ErrRedactionEmpty {
		t.Fatalf("code = %q, want %q", resp.Error.Code, ErrRedactionEmpty)
	}
}

func TestWriteKnowledge_NoFalsePositiveChinese(t *testing.T) {
	repo := setupDefenseRepo(t, "redact")
	svc := NewService(Config{RepoPath: repo})
	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind:           "note",
		Content:        "用户偏好晚上开会，下载一个 1234567890123456 字节的模型",
		IdempotencyKey: "chinese-no-fp-1",
	})
	if !resp.OK {
		t.Fatalf("normal Chinese text should not trigger defense: %+v", resp.Error)
	}
	res := resp.Result.(WriteKnowledgeResult)
	if len(res.Redactions) != 0 {
		t.Fatalf("expected no redactions on normal Chinese: %+v", res.Redactions)
	}
}

// Ensure memorydefense import is used even if tests change.
var _ = memorydefense.Screen
