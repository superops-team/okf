package agentconfig

import (
	"strings"
	"testing"
)

func TestRenderAgentSkillFrontmatter(t *testing.T) {
	skill := RenderAgentSkill()
	if !strings.HasPrefix(skill, "---\n") {
		t.Fatal("skill must start with YAML frontmatter")
	}
	// Extract frontmatter
	parts := strings.SplitN(skill, "---\n", 3)
	if len(parts) < 3 {
		t.Fatal("skill must have closed frontmatter")
	}
	frontmatter := parts[1]
	if !strings.Contains(frontmatter, "name: okf") {
		t.Error("frontmatter must contain name: okf")
	}
	if !strings.Contains(frontmatter, "description:") {
		t.Error("frontmatter must contain description")
	}
	// Must have markdown body
	body := parts[2]
	if strings.TrimSpace(body) == "" {
		t.Error("skill must have markdown body")
	}
}

func TestRenderAgentSkillWorkflowOnce(t *testing.T) {
	skill := RenderAgentSkill()
	for _, id := range []string{"W01", "W02", "W03", "W04", "W05", "W06", "W07", "W08"} {
		count := strings.Count(skill, "["+id+"]")
		if count != 1 {
			t.Errorf("clause %s appears %d times, expected exactly 1", id, count)
		}
	}
}

func TestRenderAgentSkillNoOwnershipMarkers(t *testing.T) {
	skill := RenderAgentSkill()
	forbidden := []string{
		"OKF-MANAGED",
		"OKF_MANAGED",
		"BEGIN OKF MANAGED",
		"END OKF MANAGED",
		"agent apply",
		"agent remove",
		"agent plan",
		"allowed-tools",
		"hook:",
		"#!/bin",
	}
	for _, f := range forbidden {
		if strings.Contains(skill, f) {
			t.Errorf("portable skill must not contain %q", f)
		}
	}
}

func TestRenderAgentSkillReferencesRegisteredTools(t *testing.T) {
	skill := RenderAgentSkill()
	// All okf_ references must be in registered inventory
	refs := toolNamesIn(skill)
	registered := make(map[string]bool)
	for _, rt := range RegisteredTools {
		registered[rt] = true
	}
	for _, ref := range refs {
		if !registered[ref] {
			t.Errorf("skill references unregistered tool %q", ref)
		}
	}
}

func TestRenderAgentSkillDeterministic(t *testing.T) {
	a := RenderAgentSkill()
	b := RenderAgentSkill()
	if a != b {
		t.Error("RenderAgentSkill must be deterministic")
	}
}
