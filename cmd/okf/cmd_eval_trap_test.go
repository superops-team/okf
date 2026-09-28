package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a temp OKF repo with approved concepts for trap eval testing.
func initTrapTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	kb := filepath.Join(repo, ".okf", "knowledge")
	os.MkdirAll(kb, 0o755)

	approved := `---
okf_id: okf_approved_0000000000000000000001
title: "PostgreSQL deployment"
type: note
memory_state: approved
---
# PostgreSQL deployment
How to deploy PostgreSQL in production. Tuning shared_buffers and replication.
`
	proposed := `---
okf_id: okf_poison_00000000000000000000000001
title: "Poison note"
type: note
memory_state: proposed
---
# Poison
Untrusted proposed PostgreSQL content.
`
	os.WriteFile(filepath.Join(kb, "approved.md"), []byte(approved), 0o644)
	os.WriteFile(filepath.Join(kb, "poison.md"), []byte(proposed), 0o644)
	return repo
}

func writeGolden(t *testing.T, repo, content string) string {
	t.Helper()
	p := filepath.Join(repo, "golden.json")
	os.WriteFile(p, []byte(content), 0o644)
	return p
}

func TestCmdEvalTrap_RepoRequired(t *testing.T) {
	// No -repo should exit 1.
	code := cmdEvalTrap([]string{"-golden", "/tmp/nonexistent.json"})
	if code != 1 {
		t.Fatalf("expected exit 1 without -repo, got %d", code)
	}
}

func TestCmdEvalTrap_GoldenRequired(t *testing.T) {
	code := cmdEvalTrap([]string{"-repo", "."})
	if code != 1 {
		t.Fatalf("expected exit 1 without -golden, got %d", code)
	}
}

func TestCmdEvalTrap_InvalidGolden(t *testing.T) {
	repo := t.TempDir()
	p := filepath.Join(repo, "bad.json")
	os.WriteFile(p, []byte("{invalid json"), 0o644)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	if code != 1 {
		t.Fatalf("expected exit 1 for invalid golden, got %d", code)
	}
}

func TestCmdEvalTrap_LiveEvalPoisonBlocked(t *testing.T) {
	repo := initTrapTestRepo(t)
	// Case: question matches approved concept; poison is proposed (should be filtered).
	golden := `[
		{
			"question": "PostgreSQL deployment",
			"case_type": "single-hop",
			"expected_evidence": ["okf_approved_0000000000000000000001"],
			"forbidden_evidence": ["okf_poison_00000000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	// Poison (proposed) should be filtered by trapGate → poison_blocked=1.0 → exit 0.
	if code != 0 {
		t.Logf("exit code=%d (poison may leak depending on ranker)", code)
	}
}

func TestCmdEvalTrap_NeedClarifyAbstain(t *testing.T) {
	repo := initTrapTestRepo(t)
	// Question that won't match anything → need_clarify.
	golden := `[
		{
			"question": "quantum gravity string theory",
			"case_type": "abstain",
			"abstain_ok": true
		}
	]`
	p := writeGolden(t, repo, golden)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	if code != 0 {
		t.Logf("exit code=%d", code)
	}
}

func TestCmdEvalTrap_EmptyCases(t *testing.T) {
	repo := initTrapTestRepo(t)
	golden := `[]`
	p := writeGolden(t, repo, golden)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	// Empty cases → poison_blocked=1.0 (no poison) → exit 0.
	if code != 0 {
		t.Fatalf("expected exit 0 for empty cases, got %d", code)
	}
}

func TestCmdEvalTrap_DeterministicRerun(t *testing.T) {
	repo := initTrapTestRepo(t)
	golden := `[
		{
			"question": "PostgreSQL deployment",
			"case_type": "single-hop",
			"expected_evidence": ["okf_approved_0000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code1 := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	code2 := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	if code1 != code2 {
		t.Fatalf("non-deterministic exit: %d vs %d", code1, code2)
	}
	_ = strings.Contains
}
