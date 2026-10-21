package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// proposedKBNote writes a durable note concept whose temporal state is proposed
// (with confidence and evidence_refs) so it surfaces in the review queue and can
// be approved via the memory-review subcommand.
func proposedKBNote(t *testing.T, repo, name, okfID string) {
	t.Helper()
	body := "---\n" +
		"type: note\n" +
		"title: Temporal Proposal " + name + "\n" +
		"okf_id: " + okfID + "\n" +
		"memory_state: proposed\n" +
		"memory_confidence: 0.6\n" +
		"provenance:\n" +
		"  evidence_refs:\n" +
		"    - ref-a\n" +
		"generated:\n" +
		"  by: test\n" +
		"  at: \"2024-01-01T00:00:00Z\"\n" +
		"---\n" +
		"Proposed reusable knowledge about " + name + ".\n"
	mustWriteKBFile(t, repo, "concepts/"+name+".md", body)
}

// runToolCmd runs `okf tool <args...>` and returns the exit code plus the parsed
// JSON envelope. It fatals when the output is not the expected JSON envelope.
func runToolCmdEnvelope(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	var code int
	out := captureStdout(t, func() {
		code = cmdTool(args)
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("not JSON (code=%d): %v\n%s", code, err, out)
	}
	return code, parsed
}

// S37: `okf tool query --memory-view all` keeps requiring a non-empty -q and
// parses the hyphen-named flag into the service request.
func TestToolQueryMemoryViewAllFlag(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	mustWriteKBFile(t, repo, "concepts/n.md", "---\ntype: note\ntitle: All View\n---\nBody mentioning routing policy.\n")

	code, parsed := runToolCmdEnvelope(t, "query", "--repo", repo,
		"-q", "routing policy", "--memory-view", "all", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, envelope = %v", code, parsed)
	}
	if parsed["operation"] != "query" || parsed["ok"] != true {
		t.Fatalf("envelope not a successful query: %v", parsed)
	}
	result, _ := parsed["result"].(map[string]any)
	if _, ok := result["results"]; !ok {
		t.Fatalf("all view should still return ranked results: %v", result)
	}
}

// S37: the default/current view still rejects an empty -q.
func TestToolQueryDefaultStillRequiresQuery(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	mustWriteKBFile(t, repo, "concepts/n.md", "---\ntype: note\ntitle: N\n---\nbody\n")

	code, parsed := runToolCmdEnvelope(t, "query", "--repo", repo, "-q", "", "--json")
	if code == 0 {
		t.Fatalf("default query mode must reject empty -q: %v", parsed)
	}
	errObj, _ := parsed["error"].(map[string]any)
	if errObj["code"] != "invalid_query" {
		t.Fatalf("error.code = %v, want invalid_query", errObj["code"])
	}
}

// S37: --memory-review-queue is query-optional: an empty -q is accepted and the
// body-free proposed-concept queue is returned.
func TestToolQueryMemoryReviewQueueAllowsEmptyQuery(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	proposedKBNote(t, repo, "proposal", id)

	code, parsed := runToolCmdEnvelope(t, "query", "--repo", repo, "--memory-review-queue", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, envelope = %v", code, parsed)
	}
	if parsed["operation"] != "query" || parsed["ok"] != true {
		t.Fatalf("review queue must succeed: %v", parsed)
	}
	result, _ := parsed["result"].(map[string]any)
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("review queue items = %d, want 1: %v", len(items), result)
	}
	first, _ := items[0].(map[string]any)
	if first["okf_id"] != id {
		t.Fatalf("queue item okf_id = %v, want %s", first["okf_id"], id)
	}
	// Body-free queue item: no markdown body field.
	if _, hasBody := first["body"]; hasBody {
		t.Fatalf("review queue item must be body-free: %v", first)
	}
}

// S37: --memory-view=history is query-optional but requires exactly one --refs
// entry; it returns the MemoryHistoryResult envelope.
func TestToolQueryHistoryAllowsEmptyQueryWithRefs(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	mustWriteKBFile(t, repo, "concepts/one.md",
		"---\ntype: note\ntitle: History Node\nokf_id: "+id+"\nmemory_state: approved\n---\nBody mentions routing.\n")

	code, parsed := runToolCmdEnvelope(t, "query", "--repo", repo,
		"--memory-view", "history", "--refs", id, "--json")
	if code != 0 {
		t.Fatalf("exit = %d, envelope = %v", code, parsed)
	}
	result, _ := parsed["result"].(map[string]any)
	if result["requested_ref"] != id {
		t.Fatalf("requested_ref = %v, want %s: %v", result["requested_ref"], id, result)
	}
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("history items = %d, want 1: %v", len(items), result)
	}
}

// S37: --memory-view=history with no refs is rejected by the service (the CLI
// only relaxes the empty-query guard; the rest is validated downstream).
func TestToolQueryHistoryRequiresRefs(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	code, parsed := runToolCmdEnvelope(t, "query", "--repo", repo,
		"--memory-view", "history", "--json")
	if code == 0 {
		t.Fatalf("history without refs must fail: %v", parsed)
	}
}

// S37: `okf tool context --memory-view` accepts current|all and maps through.
func TestToolContextMemoryViewFlag(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	mustWriteKBFile(t, repo, "concepts/n.md", "---\ntype: note\ntitle: Ctx\n---\nBody about caching.\n")

	code, parsed := runToolCmdEnvelope(t, "context", "--repo", repo,
		"-q", "caching", "--memory-view", "all", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, envelope = %v", code, parsed)
	}
	if parsed["operation"] != "context" || parsed["ok"] != true {
		t.Fatalf("context with memory-view=all must succeed: %v", parsed)
	}
}

// S37: `okf tool memory-review` approves a proposed note and reports the
// before/after states in its result envelope.
func TestToolMemoryReviewApproveEnvelope(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	const id = "okf_17a2c56db85c4889b4f8fe02ca9ac67e"
	proposedKBNote(t, repo, "reviewme", id)

	code, parsed := runToolCmdEnvelope(t, "memory-review", "--repo", repo,
		"--ref", id, "--action", "approve", "--expected-state", "proposed", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, envelope = %v", code, parsed)
	}
	if parsed["operation"] != "memory_review" {
		t.Fatalf("operation = %v, want memory_review", parsed["operation"])
	}
	if parsed["ok"] != true || parsed["mutating"] != true {
		t.Fatalf("approve must be ok and mutating: %v", parsed)
	}
	result, _ := parsed["result"].(map[string]any)
	if result["previous_state"] != "proposed" || result["current_state"] != "approved" {
		t.Fatalf("review result states = %v, want proposed->approved", result)
	}
}

// S37: memory-review requires --ref, --action and --expected-state.
func TestToolMemoryReviewMissingFlags(t *testing.T) {
	repo := initIdentityCLITestRepo(t)
	code, parsed := runToolCmdEnvelope(t, "memory-review", "--repo", repo, "--json")
	if code == 0 {
		t.Fatalf("memory-review with no flags must fail: %v", parsed)
	}
	errObj, _ := parsed["error"].(map[string]any)
	if errObj["code"] != "invalid_request" {
		t.Fatalf("error.code = %v, want invalid_request", errObj["code"])
	}
}

// S37: the top-level help contract documents the new subcommand and modes, and
// the unknown-subcommand path lists memory-review as valid.
func TestToolHelpDocumentsTemporalModes(t *testing.T) {
	out := captureStdout(t, func() {
		code := cmdTool(nil)
		if code != 0 {
			t.Fatalf("help exit = %d", code)
		}
	})
	for _, want := range []string{"memory-review", "--memory-view", "--memory-review-queue", "--refs", "expected-state"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tool help missing %q:\n%s", want, out)
		}
	}
}
