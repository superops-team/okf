package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runTrapCapture runs cmdEvalTrap and captures stdout, returning the exit code
// and captured output. This lets tests assert on human/JSON field content, not
// just the exit code.
func runTrapCapture(t *testing.T, args []string) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := cmdEvalTrap(args)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := ioCopy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return code, buf.String()
}

func ioCopy(dst *bytes.Buffer, src *os.File) (int64, error) {
	b := make([]byte, 4096)
	for {
		n, err := src.Read(b)
		if n > 0 {
			dst.Write(b[:n])
		}
		if err != nil {
			break
		}
	}
	return int64(dst.Len()), nil
}

func initTrapTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	// Service.resolve() requires a real git repo. Without this, Reflect returns
	// ErrNotGitRepository and every trap case degrades to abstain — making the
	// whole suite vacuous (fake green).
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	exec.Command("git", "-C", repo, "config", "user.email", "t@t.com").Run()
	exec.Command("git", "-C", repo, "config", "user.name", "T").Run()
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
	exec.Command("git", "-C", repo, "add", "-A").Run()
	exec.Command("git", "-C", repo, "commit", "-q", "-m", "init").Run()
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

func TestCmdEvalTrap_ApprovedForbiddenLeak_NonZero(t *testing.T) {
	repo := initTrapTestRepo(t)
	kb := filepath.Join(repo, ".okf", "knowledge")
	// An APPROVED doc that the golden set forbids must not be cited. If Reflect
	// returns it, the gate MUST go non-zero. This is the security assertion the
	// old dead-gate missed: IsPoison must be derived from forbidden_evidence.
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
	code, out := runTrapCapture(t, []string{"-golden", p, "-repo", repo})
	if code != 1 {
		t.Fatalf("approved forbidden doc leaked into evidence -> expected exit 1, got %d (out=%s)", code, out)
	}
	if !strings.Contains(out, "poison_blocked=0.00") && !strings.Contains(out, `"poison_blocked":0`) {
		t.Fatalf("expected poison_blocked=0 on leak, out=%q", out)
	}
}

func TestCmdEvalTrap_ProposedPoisonFiltered_Zero(t *testing.T) {
	repo := initTrapTestRepo(t)
	// proposed poison is dropped by the trapGate → never cited → blocked → exit 0.
	golden := `[
		{
			"question": "PostgreSQL deployment",
			"case_type": "single-hop",
			"expected_evidence": ["okf_approved_0000000000000000000001"],
			"forbidden_evidence": ["okf_poison_00000000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code, out := runTrapCapture(t, []string{"-golden", p, "-repo", repo})
	if code != 0 {
		t.Fatalf("proposed poison should be filtered (blocked), expected exit 0, got %d (out=%s)", code, out)
	}
	if !strings.Contains(out, "poison_blocked=1.00") {
		t.Fatalf("expected poison_blocked=1.00 when proposed filtered, out=%q", out)
	}
}

func TestCmdEvalTrap_EmptyBundle_Abstains(t *testing.T) {
	// Empty knowledge bundle (not empty golden): question matches nothing →
	// Reflect abstains (need_clarify) on an abstain_ok case → rewarded, exit 0.
	repo := t.TempDir()
	kb := filepath.Join(repo, ".okf", "knowledge")
	os.MkdirAll(kb, 0o755)
	golden := `[
		{
			"question": "quantum gravity string theory",
			"case_type": "abstain",
			"abstain_ok": true
		}
	]`
	p := writeGolden(t, repo, golden)
	code, out := runTrapCapture(t, []string{"-golden", p, "-repo", repo})
	if code != 0 {
		t.Fatalf("empty bundle + abstain_ok should exit 0, got %d (out=%s)", code, out)
	}
	if !strings.Contains(out, "abstain_score=1.00") {
		t.Fatalf("expected abstain rewarded on empty bundle, out=%q", out)
	}
}

func TestCmdEvalTrap_MissingExpectedEvidence_ScoresHalf(t *testing.T) {
	repo := initTrapTestRepo(t)
	// Expected evidence exists but question does not surface it → evidence score
	// must be 0.5 (partial), not fabricated 1.0. No forbidden → no gate fire.
	golden := `[
		{
			"question": "unrelated quantum topic",
			"case_type": "single-hop",
			"expected_evidence": ["okf_approved_0000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code, out := runTrapCapture(t, []string{"-golden", p, "-repo", repo})
	if code != 0 {
		t.Fatalf("no forbidden -> exit 0, got %d (out=%s)", code, out)
	}
	if !strings.Contains(out, "evidence_support_mean=0.50") {
		t.Fatalf("missing expected evidence should score 0.50, out=%q", out)
	}
}

func TestCmdEvalTrap_JSONOutput_FieldsComplete(t *testing.T) {
	repo := initTrapTestRepo(t)
	golden := `[
		{
			"question": "PostgreSQL deployment",
			"case_type": "single-hop",
			"expected_evidence": ["okf_approved_0000000000000000000001"],
			"forbidden_evidence": ["okf_poison_00000000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code, out := runTrapCapture(t, []string{"-golden", p, "-repo", repo, "-json"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (out=%s)", code, out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v (out=%q)", err, out)
	}
	for _, field := range []string{"answer_hit_mean", "evidence_support_mean", "abstain_score", "poison_blocked"} {
		if _, ok := m[field]; !ok {
			t.Fatalf("JSON missing required field %q; keys=%v", field, m)
		}
	}
	if m["poison_blocked"].(float64) != 1.0 {
		t.Fatalf("proposed filtered -> poison_blocked=1.0, got %v", m["poison_blocked"])
	}
}

func TestCmdEvalTrap_HumanOutput_FieldsPresent(t *testing.T) {
	repo := initTrapTestRepo(t)
	golden := `[
		{
			"question": "PostgreSQL deployment",
			"case_type": "single-hop",
			"expected_evidence": ["okf_approved_0000000000000000000001"]
		}
	]`
	p := writeGolden(t, repo, golden)
	code, out := runTrapCapture(t, []string{"-golden", p, "-repo", repo})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, want := range []string{"answer_hit_mean=", "evidence_support_mean=", "abstain_score=", "poison_blocked="} {
		if !strings.Contains(out, want) {
			t.Fatalf("human output missing %q; out=%q", want, out)
		}
	}
}
