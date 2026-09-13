package main

// Command-help golden assertions (T4.1). These pin the public help/usage text
// so that the new commands introduced by agent-knowledge-discovery cannot be
// silently renamed or dropped without a deliberate golden update.

import (
	"strings"
	"testing"
)

// TestHelpGoldenTopLevel lists every command the change introduces.
func TestHelpGoldenTopLevel(t *testing.T) {
	bin := buildOKF(t)
	out := runOKF(t, bin, "help")
	for _, want := range []string{
		"identity",
		"agent",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("okf help missing %q\n%s", want, out)
		}
	}
}

// TestHelpGoldenIdentityUsage pins the identity subcommand surface.
func TestHelpGoldenIdentityUsage(t *testing.T) {
	bin := buildOKF(t)
	out, code := runOKFExpectExit(t, bin, 0, "identity")
	if !strings.Contains(out, "ensure") || !strings.Contains(out, "resolve") {
		t.Fatalf("identity usage = %q, want ensure/resolve", out)
	}
	if code != 0 {
		t.Fatalf("identity (no args) exit = %d, want 0 (help)", code)
	}
}

// TestHelpGoldenAgentUsage pins the agent subcommand surface and clients.
func TestHelpGoldenAgentUsage(t *testing.T) {
	bin := buildOKF(t)
	out, code := runOKFExpectExit(t, bin, 0, "agent")
	for _, want := range []string{"plan", "apply", "status", "remove"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent usage = %q, want %s", out, want)
		}
	}
	if code != 0 {
		t.Fatalf("agent (no args) exit = %d, want 0 (help)", code)
	}
}

// TestHelpGoldenToolUsage pins manifest among the tool subcommands.
func TestHelpGoldenToolUsage(t *testing.T) {
	bin := buildOKF(t)
	out, code := runOKFExpectExit(t, bin, 0, "tool")
	if !strings.Contains(out, "manifest") {
		t.Fatalf("tool usage = %q, want manifest", out)
	}
	if code != 0 {
		t.Fatalf("tool (no args) exit = %d, want 0 (help)", code)
	}
}

// TestHelpGoldenUnknownAgentSubcommandRejected proves unknown agent verbs fail
// explicitly (S45 adjacent contract on the CLI surface).
func TestHelpGoldenUnknownAgentSubcommandRejected(t *testing.T) {
	bin := buildOKF(t)
	out, code := runOKFExpectExit(t, bin, 1, "agent", "frobnicate")
	if !strings.Contains(out, "unknown agent subcommand") {
		t.Fatalf("unknown agent subcommand output = %q", out)
	}
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

// TestHelpFlagExplicitHelp proves --help/-h prints usage and exits 0 for all
// new subcommand groups (usability: users expect --help to work everywhere).
func TestHelpFlagExplicitHelp(t *testing.T) {
	bin := buildOKF(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"identity", "--help"}, "ensure"},
		{[]string{"identity", "-h"}, "resolve"},
		{[]string{"agent", "--help"}, "plan"},
		{[]string{"agent", "-h"}, "remove"},
		{[]string{"tool", "--help"}, "manifest"},
		{[]string{"tool", "-h"}, "query"},
	} {
		out, code := runOKFExpectExit(t, bin, 0, tc.args...)
		if !strings.Contains(out, tc.want) {
			t.Fatalf("%v output = %q, want %q", tc.args, out, tc.want)
		}
		if code != 0 {
			t.Fatalf("%v exit = %d, want 0", tc.args, code)
		}
	}
}
