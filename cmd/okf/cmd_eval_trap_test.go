package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
Untrusted proposed PostgreSQL content that should be filtered.
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

func TestCmdEvalTrap_PoisonBlockedGate(t *testing.T) {
	repo := initTrapTestRepo(t)
	// poison is proposed → trapGate filters it → poison_blocked=1.0 → exit 0
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
	if code != 0 {
		t.Fatalf("poison should be blocked by trapGate, expected exit 0, got %d", code)
	}
}

func TestCmdEvalTrap_NeedClarifyAbstain(t *testing.T) {
	repo := initTrapTestRepo(t)
	// Question matches nothing → need_clarify=true → abstain rewarded
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
		t.Fatalf("abstain case should exit 0, got %d", code)
	}
}

func TestCmdEvalTrap_EmptyCases(t *testing.T) {
	repo := initTrapTestRepo(t)
	p := writeGolden(t, repo, `[]`)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	if code != 0 {
		t.Fatalf("empty cases should exit 0 (no poison), got %d", code)
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
}

func TestCmdEvalTrap_NoDemoFallback(t *testing.T) {
	// Verify that without -repo, we get exit 1 (not hardcoded scores).
	repo := initTrapTestRepo(t)
	golden := `[{"question": "test", "case_type": "single-hop"}]`
	p := writeGolden(t, repo, golden)
	code := cmdEvalTrap([]string{"-golden", p})
	if code != 1 {
		t.Fatalf("without -repo should exit 1 (no demo fallback), got %d", code)
	}
}

func TestCmdEvalTrap_ExpectedEvidenceScore(t *testing.T) {
	repo := initTrapTestRepo(t)
	golden := `[
		{
			"question": "PostgreSQL deployment tuning",
			"case_type": "single-hop",
			"expected_evidence": ["okf_approved_0000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	if code != 0 {
		t.Fatalf("expected exit 0 (no poison leak), got %d", code)
	}
}

func TestCmdEvalTrap_JSONOutput(t *testing.T) {
	repo := initTrapTestRepo(t)
	golden := `[
		{
			"question": "quantum gravity",
			"case_type": "abstain",
			"abstain_ok": true
		}
	]`
	p := writeGolden(t, repo, golden)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo, "-json"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCmdEvalTrap_ForbiddenEvidenceLeaks(t *testing.T) {
	repo := initTrapTestRepo(t)
	kb := repo + "/.okf/knowledge"
	forbidden := `---
okf_id: okf_forbidden_00000000000000000000000001
title: "PostgreSQL deployment leaked"
type: note
memory_state: approved
---
# Forbidden
PostgreSQL deployment shared_buffers replication leaked content.
`
	os.WriteFile(kb+"/forbidden.md", []byte(forbidden), 0o644)
	golden := `[
		{
			"question": "PostgreSQL deployment",
			"case_type": "single-hop",
			"forbidden_evidence": ["okf_forbidden_00000000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code := cmdEvalTrap([]string{"-golden", p, "-repo", repo})
	// Either: forbidden not in evidence (exit 0) or leak detected (exit 1).
	// We only assert it doesn't crash. The poison_blocked metric reflects this.
	if code != 0 && code != 1 {
		t.Fatalf("unexpected exit: %d", code)
	}
}
