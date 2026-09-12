package agentconfig

import (
	"fmt"
	"strings"
)

// Canonical tool and command inventory. Rendered guidance may reference only
// these currently registered OKF MCP tools and CLI commands; this is the
// executable gate for "references only currently registered OKF MCP tools".
var (
	// RegisteredTools is the allowlist of OKF MCP tool names that rendered
	// workflow guidance may reference.
	RegisteredTools = []string{
		"okf_status",
		"okf_manifest",
		"okf_query",
		"okf_context",
		"okf_note",
		"okf_feedback",
		"okf_resolve",
	}

	// RegisteredCommands is the allowlist of OKF CLI commands that rendered
	// guidance may reference.
	RegisteredCommands = []string{
		"okf status",
		"okf tool manifest",
		"okf tool query",
		"okf tool context",
		"okf identity resolve",
		"okf mcp",
	}
)

// Clause is one typed canonical workflow rule. ID is the stable clause
// identifier (W01..W07) that must appear exactly once in every applicable
// rendered output.
type Clause struct {
	ID       string
	Heading  string
	Guidance string
}

// CanonicalClauses is the fixed, ordered, seven-clause workflow model. Each
// clause has a unique ID and behavior that client wrappers may not change.
func CanonicalClauses() []Clause {
	return []Clause{
		{
			ID:      "W01",
			Heading: "Check knowledge availability",
			Guidance: "Call `okf_status` to check repository knowledge availability and freshness " +
				"before relying on the knowledge base.",
		},
		{
			ID:      "W02",
			Heading: "Ask before mutating initialization",
			Guidance: "If knowledge is unavailable, ask the user before running any mutating knowledge " +
				"init or refresh; never initialize or refresh without explicit consent.",
		},
		{
			ID:      "W03",
			Heading: "Discover, retrieve and bound evidence",
			Guidance: "Use `okf_manifest` for inventory and cold start; use `okf_query` for relevance " +
				"retrieval; use `okf_context` only to select evidence within the token budget.",
		},
		{
			ID:       "W04",
			Heading:  "Preserve stable references",
			Guidance: "Preserve stable `okf://` references when citing or passing evidence between steps.",
		},
		{
			ID:       "W05",
			Heading:  "Perform the requested task",
			Guidance: "Perform the task the user actually requested; do not start unrelated knowledge work.",
		},
		{
			ID:      "W06",
			Heading: "Persist only when requested",
			Guidance: "Use `okf_note` or `okf_feedback` only when the user asks to persist or the " +
				"workflow explicitly requires it; always supply an idempotency key.",
		},
		{
			ID:       "W07",
			Heading:  "Never store secrets or private content",
			Guidance: "Never store credentials, tokens, secrets or unrelated private content in the knowledge base.",
		},
	}
}

// clauseIDs returns the ordered list of canonical clause IDs.
func clauseIDs() []string {
	clauses := CanonicalClauses()
	ids := make([]string, 0, len(clauses))
	for _, c := range clauses {
		ids = append(ids, c.ID)
	}
	return ids
}

// renderCore renders the shared, numbered, seven-clause workflow body. Each
// clause ID is emitted as a bracketed marker exactly once so conformance tests
// can assert exact-once coverage.
func renderCore() string {
	var b strings.Builder
	for i, c := range CanonicalClauses() {
		fmt.Fprintf(&b, "%d. [%s] %s: %s\n", i+1, c.ID, c.Heading, c.Guidance)
	}
	return b.String()
}

// RenderCursorRule renders the whole-file Cursor rule document. The file is
// OKF-owned from its first line (the ownership header).
func RenderCursorRule() string {
	var b strings.Builder
	b.WriteString(PendingFileHeader)
	b.WriteString("\n\n")
	b.WriteString("# OKF Agent Knowledge Workflow\n\n")
	b.WriteString("This rule is managed by `okf agent`. It tells the AI agent how to use the OKF ")
	b.WriteString("project knowledge base. Do not edit the OKF-managed section by hand; use ")
	b.WriteString("`okf agent apply` to regenerate it.\n\n")
	b.WriteString(renderCore())
	return b.String()
}

// claudeSkillFrontmatter is the valid Claude Code skill frontmatter. It names
// the skill and its description, and carries the OKF whole-file ownership header
// after the frontmatter so ownership remains self-describing.
const claudeSkillFrontmatter = "---\n" +
	"name: okf\n" +
	"description: Use the project OKF knowledge base to answer repository questions.\n" +
	"---\n"

// RenderClaudeSkill renders the whole-file Claude Code SKILL.md.
func RenderClaudeSkill() string {
	var b strings.Builder
	b.WriteString(claudeSkillFrontmatter)
	b.WriteString(PendingFileHeader)
	b.WriteString("\n\n")
	b.WriteString("# OKF Project Knowledge Skill\n\n")
	b.WriteString("This skill is managed by `okf agent`. Follow the canonical workflow below when ")
	b.WriteString("using the project OKF knowledge base.\n\n")
	b.WriteString(renderCore())
	return b.String()
}

// RenderCodexBlock renders the Markdown body that lives inside the OKF-managed
// AGENTS.md delimited block (markers themselves are added by the adapter).
func RenderCodexBlock() string {
	var b strings.Builder
	b.WriteString("OKF project knowledge workflow (managed by `okf agent`).\n\n")
	b.WriteString(renderCore())
	return b.String()
}

// RenderCodexMCPTOML renders the body that lives inside the TOML managed block
// (markers themselves are added by the adapter). cmd is the resolved server
// command, conventionally ["okf", "mcp", "--repo", "."].
func RenderCodexMCPTOML(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[mcp_servers.okf]\n")
	fmt.Fprintf(&b, "command = %q\n", cmd[0])
	fmt.Fprintf(&b, "args = %q\n", cmd[1:])
	b.WriteString("[mcp_servers.okf.env]\n")
	fmt.Fprintf(&b, "%s = %q\n", OwnedMarkerKey, OwnedMarkerValue)
	return b.String()
}

// RenderJSONMCPEntry builds the deterministic mcpServers.okf entry as a generic
// map for encoding/json. cmd is ["okf", "mcp", "--repo", "."].
func RenderJSONMCPEntry(cmd []string) map[string]any {
	if len(cmd) == 0 {
		return nil
	}
	args := make([]any, 0, len(cmd)-1)
	for _, a := range cmd[1:] {
		args = append(args, a)
	}
	return map[string]any{
		"command": cmd[0],
		"args":    args,
		"env": map[string]any{
			OwnedMarkerKey: OwnedMarkerValue,
		},
	}
}

// toolNamesIn extracts every okf_* token from rendered text so conformance
// tests can assert they all come from the registered inventory.
func toolNamesIn(text string) []string {
	var out []string
	scan := text
	for {
		idx := strings.Index(scan, "okf_")
		if idx < 0 {
			break
		}
		rest := scan[idx:]
		end := 0
		for i := len("okf_"); i < len(rest); i++ {
			ch := rest[i]
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
				end = i + 1
				continue
			}
			break
		}
		if end > len("okf_") {
			out = append(out, rest[:end])
		}
		scan = rest[len("okf_"):]
	}
	return out
}
