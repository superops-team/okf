package agentconfig

import (
	"strings"
	"testing"
)

// TestCanonicalWorkflowCoverage asserts that every canonical clause ID appears
// exactly once in each applicable rendered output.
func TestCanonicalWorkflowCoverage(t *testing.T) {
	outputs := map[string]string{
		"cursor-rule":  RenderCursorRule(),
		"claude-skill": RenderClaudeSkill(),
		"codex-block":  RenderCodexBlock(),
	}
	for name, text := range outputs {
		for _, id := range clauseIDs() {
			got := strings.Count(text, id)
			if got != 1 {
				t.Errorf("%s: clause %q appears %d times, want exactly once", name, id, got)
			}
		}
	}
	// Every clause ID is unique.
	seen := map[string]int{}
	for _, id := range clauseIDs() {
		seen[id]++
		if seen[id] > 1 {
			t.Fatalf("clause ID %q duplicated", id)
		}
	}
	if len(clauseIDs()) != 7 {
		t.Fatalf("want 7 canonical clauses, got %d", len(clauseIDs()))
	}
}

// TestGeneratedToolsExist asserts that rendered guidance only references tools
// from the registered OKF MCP inventory.
func TestGeneratedToolsExist(t *testing.T) {
	allow := map[string]bool{}
	for _, tool := range RegisteredTools {
		allow[tool] = true
	}
	outputs := []string{
		RenderCursorRule(),
		RenderClaudeSkill(),
		RenderCodexBlock(),
	}
	for _, text := range outputs {
		for _, name := range toolNamesIn(text) {
			if !allow[name] {
				t.Errorf("rendered guidance references unregistered tool %q", name)
			}
		}
	}
	// The canonical clauses must actually name the expected tools.
	cursor := RenderCursorRule()
	for _, want := range []string{"okf_status", "okf_manifest", "okf_query", "okf_context", "okf_note", "okf_feedback"} {
		if !strings.Contains(cursor, want) {
			t.Errorf("cursor rule missing expected tool reference %q", want)
		}
	}
}

// TestRenderedMarkersAreStable guards drift: the ownership header and markers
// are constants the adapters depend on.
func TestRenderedMarkersAreStable(t *testing.T) {
	if PendingFileHeader == "" || tomlBegin == "" || tomlEnd == "" || agBegin == "" || agEnd == "" {
		t.Fatal("ownership markers must be non-empty")
	}
	if !strings.Contains(RenderClaudeSkill(), claudeSkillFrontmatter) {
		t.Fatal("claude skill must begin with valid frontmatter")
	}
}

// L4 (second-round review lock-in): a nil or empty server command must be a safe
// no-op for both MCP renderers — no panic, TOML renders empty string, JSON entry
// renders nil.
func TestRenderMCPEmptyCommandNoPanic(t *testing.T) {
	cases := map[string][]string{
		"nil":   nil,
		"empty": {},
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			if got := RenderCodexMCPTOML(cmd); got != "" {
				t.Fatalf("RenderCodexMCPTOML(%v) = %q, want empty", cmd, got)
			}
			if got := RenderJSONMCPEntry(cmd); got != nil {
				t.Fatalf("RenderJSONMCPEntry(%v) = %v, want nil", cmd, got)
			}
		})
	}
}
