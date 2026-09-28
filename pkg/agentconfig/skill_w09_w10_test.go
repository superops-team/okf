package agentconfig

import "strings"
import "testing"

func TestW09ReflectClausePresent(t *testing.T) {
	text := RenderAgentSkill()
	if !strings.Contains(text, "W09") {
		t.Fatal("W09 clause missing from Agent Skill")
	}
	if !strings.Contains(text, "okf_reflect") {
		t.Fatal("W09 must reference okf_reflect")
	}
	if !strings.Contains(text, "need_clarify") {
		t.Fatal("W09 must explain abstention")
	}
}

func TestW10DefenseClausePresent(t *testing.T) {
	text := RenderAgentSkill()
	if !strings.Contains(text, "W10") {
		t.Fatal("W10 clause missing from Agent Skill")
	}
	if !strings.Contains(text, "Memory Defense") {
		t.Fatal("W10 must reference Memory Defense")
	}
	if !strings.Contains(text, "REDACTED") {
		t.Fatal("W10 must mention redaction output")
	}
}

func TestRegisteredToolsIncludeReflect(t *testing.T) {
	tools := map[string]bool{}
	for _, t := range RegisteredTools {
		tools[t] = true
	}
	if !tools["okf_reflect"] {
		t.Fatal("okf_reflect not in RegisteredTools")
	}
	if !tools["okf_relation_recall"] {
		t.Fatal("okf_relation_recall not in RegisteredTools")
	}
}

func TestW09W10InAllRenderers(t *testing.T) {
	for _, text := range []string{
		RenderAgentSkill(),
		RenderClaudeSkill(),
		RenderCursorRule(),
		RenderCodexBlock(),
	} {
		if !strings.Contains(text, "W09") || !strings.Contains(text, "W10") {
			t.Fatal("W09/W10 missing from one renderer")
		}
	}
}
