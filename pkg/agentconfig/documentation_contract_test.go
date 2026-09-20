package agentconfig

import (
	"strings"
	"testing"
)

// TestDocumentationContracts locks the governed-memory wording contract that
// every rendered Agent Skill / workflow guidance must satisfy. It guards that
// hold is surfaced as an advisory, the memory_check / for_path / refs knobs
// are named, and that banned legacy phrasings never leak back in.
func TestDocumentationContracts(t *testing.T) {
	// The portable Agent Skill guidance is the canonical target.
	skill := RenderAgentSkill()

	// Required governed-memory guidance tokens.
	for _, want := range []string{
		"hold",              // W02 surfaces a matching governance hold
		"advisory",          // hold is advisory, never a hard block
		"memory_check",      // W06 search-before-write duplicate check
		"for_path",          // W02/W03 lexical code-path governance lookup
		"refs",              // W03 okf_context refs for full body
		"proposed",          // W08: inferred knowledge is written as proposed
		"memory_confidence", // W08: proposals carry a confidence
		"evidence_refs",     // W08: proposals cite their evidence
		"okf_memory_review", // W08: the review tool
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("Agent Skill guidance missing required token %q", want)
		}
	}

	// W08: the agent must never self-approve its own proposals and may review
	// only after an explicit user instruction.
	for _, want := range []string{"explicit user instruction", "never", "proposed"} {
		if !strings.Contains(skill, want) {
			t.Errorf("Agent Skill guidance missing W08 token %q", want)
		}
	}
	if !strings.Contains(skill, "never") {
		t.Errorf("Agent Skill guidance must forbid self-approval")
	}

	// hold is advisory: the guidance must clarify the server does not block
	// writes. "block writes" may ONLY appear inside this negated sentence; it
	// is never issued as a directive to block.
	if !strings.Contains(skill, "does not block writes") {
		t.Errorf("Agent Skill guidance must state hold is advisory and writes are not blocked")
	}
	for _, directive := range []string{"must block writes", "block all writes", "block writes:"} {
		if strings.Contains(skill, directive) {
			t.Errorf("Agent Skill guidance must not issue a hard-block directive %q", directive)
		}
	}

	// Banned legacy phrasings must never appear.
	for _, banned := range []string{"audit trace", "allow_duplicate"} {
		if strings.Contains(skill, banned) {
			t.Errorf("Agent Skill guidance must not contain banned phrasing %q", banned)
		}
	}

	// The same contract holds across every renderer (cursor/claude/codex),
	// not just the portable skill.
	for name, text := range map[string]string{
		"cursor": RenderCursorRule(),
		"claude": RenderClaudeSkill(),
		"codex":  RenderCodexBlock(),
		"agent":  RenderAgentSkill(),
	} {
		for _, banned := range []string{"audit trace", "allow_duplicate"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s guidance must not contain banned phrasing %q", name, banned)
			}
		}
		if !strings.Contains(text, "memory_check") {
			t.Errorf("%s guidance must name memory_check", name)
		}
		if !strings.Contains(text, "for_path") {
			t.Errorf("%s guidance must name for_path", name)
		}
	}
}

// TestDocumentationClausesOnceInAgentSkill supplements TestCanonicalWorkflowCoverage
// by pinning W01..W08 exactly-once coverage specifically in the portable Agent
// Skill rendering.
func TestDocumentationClausesOnceInAgentSkill(t *testing.T) {
	skill := RenderAgentSkill()
	for _, id := range clauseIDs() {
		if got := strings.Count(skill, id); got != 1 {
			t.Errorf("Agent Skill guidance: clause %q appears %d times, want exactly once", id, got)
		}
	}
	if len(clauseIDs()) != 8 {
		t.Fatalf("expected 8 canonical clauses, got %d", len(clauseIDs()))
	}
}
