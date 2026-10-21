package tool

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/superops-team/okf/pkg/memorymeta"
)

// ---------------------------------------------------------------------------
// ReviewMemory CAS tests (S25-S31)
// ---------------------------------------------------------------------------

func reviewNote(t *testing.T, svc *Service, ref, action, expected string) ToolEnvelope {
	t.Helper()
	return svc.ReviewMemory(t.Context(), ReviewMemoryRequest{
		Ref: ref, Action: action, ExpectedState: expected,
	})
}

func requireReviewResult(t *testing.T, env ToolEnvelope) ReviewMemoryResult {
	t.Helper()
	if !env.OK {
		t.Fatalf("review failed: %#v", env.Error)
	}
	result, ok := env.Result.(ReviewMemoryResult)
	if !ok {
		t.Fatalf("review result type = %T, want ReviewMemoryResult", env.Result)
	}
	return result
}

func failReview(t *testing.T, svc *Service, ref, action, expected, wantCode string) {
	t.Helper()
	env := reviewNote(t, svc, ref, action, expected)
	if env.OK || env.Error == nil || env.Error.Code != wantCode {
		t.Fatalf("review = %#v, want error code %q", env, wantCode)
	}
}

func proposedNoteRequest(key, content, target string) WriteKnowledgeRequest {
	req := WriteKnowledgeRequest{
		Kind: "note", Content: content, IdempotencyKey: key,
		MemoryState: "proposed", MemoryConfidence: floatPtr(0.6),
		EvidenceRefs: []string{"evidence/" + key},
	}
	if target != "" {
		req.MemoryRelationKind = "updates"
		req.MemoryRelationTargets = []string{target}
	}
	return req
}

func TestReviewMemoryServiceClocksAreIsolated(t *testing.T) {
	repoA := initToolTestRepo(t)
	repoB := initToolTestRepo(t)
	svcA := NewService(Config{RepoPath: repoA})
	svcB := NewService(Config{RepoPath: repoB})
	timeA := time.Date(2026, time.September, 20, 1, 2, 3, 4, time.UTC)
	timeB := time.Date(2027, time.October, 21, 5, 6, 7, 8, time.UTC)
	svcA.now = func() time.Time { return timeA }
	svcB.now = func() time.Time { return timeB }

	noteA := writeRelationNote(t, svcA, proposedNoteRequest("clock-a", "Proposal A", ""))
	noteB := writeRelationNote(t, svcB, proposedNoteRequest("clock-b", "Proposal B", ""))

	var wg sync.WaitGroup
	var envA, envB ToolEnvelope
	wg.Go(func() {
		envA = svcA.ReviewMemory(t.Context(), ReviewMemoryRequest{Ref: noteA.Ref, Action: "approve", ExpectedState: "proposed"})
	})
	wg.Go(func() {
		envB = svcB.ReviewMemory(t.Context(), ReviewMemoryRequest{Ref: noteB.Ref, Action: "approve", ExpectedState: "proposed"})
	})
	wg.Wait()
	requireReviewResult(t, envA)
	requireReviewResult(t, envB)

	regA, _, _ := loadTemporalBundle(t, repoA)
	recA, _ := memorymeta.Review(resolveConcept(t, regA, noteA.OKFID))
	if recA.ReviewedAt != timeA.Format(time.RFC3339Nano) {
		t.Fatalf("service A reviewed_at = %q, want %q", recA.ReviewedAt, timeA.Format(time.RFC3339Nano))
	}
	regB, _, _ := loadTemporalBundle(t, repoB)
	recB, _ := memorymeta.Review(resolveConcept(t, regB, noteB.OKFID))
	if recB.ReviewedAt != timeB.Format(time.RFC3339Nano) {
		t.Fatalf("service B reviewed_at = %q, want %q", recB.ReviewedAt, timeB.Format(time.RFC3339Nano))
	}
}

// S25: approve moves proposed -> approved, drops confidence, writes the bounded
// review record, and activates the relation edge.
func TestTemporalS25Approve(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s25-a", "Fact A", "")
	c := writeRelationNote(t, svc, proposedNoteRequest("s25-c", "Proposed successor", a.OKFID))

	res := requireReviewResult(t, reviewNote(t, svc, c.Ref, "approve", "proposed"))
	if res.PreviousState != "proposed" || res.CurrentState != "approved" {
		t.Fatalf("review result = %#v", res)
	}

	reg, view, _ := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryApproved {
		t.Fatalf("C state = %q, want approved", st)
	}
	if _, present, _ := memorymeta.Confidence(cc); present {
		t.Fatalf("approved C must not carry memory_confidence")
	}
	rec, _ := memorymeta.Review(cc)
	if rec.Action != "approve" || rec.CurrentState != memorymeta.MemoryApproved || rec.ReviewedAt == "" {
		t.Fatalf("review record = %#v", rec)
	}
	if rec.PreviousConfidence == nil || *rec.PreviousConfidence != 0.6 {
		t.Fatalf("review record previous_confidence = %v, want 0.6", rec.PreviousConfidence)
	}
	// Edge activated: C is the current head, A is no longer current.
	if !view.IsCurrent(c.OKFID) || view.IsCurrent(a.OKFID) {
		t.Fatalf("after approve: C current=%v, A current=%v; want C current, A not", view.IsCurrent(c.OKFID), view.IsCurrent(a.OKFID))
	}
}

// S26: decline moves proposed -> declined, drops confidence, and leaves the edge
// inactive (A stays current).
func TestTemporalS26Decline(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s26-a", "Fact A", "")
	c := writeRelationNote(t, svc, proposedNoteRequest("s26-c", "Declined successor", a.OKFID))

	res := requireReviewResult(t, reviewNote(t, svc, c.Ref, "decline", "proposed"))
	if res.CurrentState != "declined" {
		t.Fatalf("review result = %#v", res)
	}

	reg, view, _ := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryDeclined {
		t.Fatalf("C state = %q, want declined", st)
	}
	if _, present, _ := memorymeta.Confidence(cc); present {
		t.Fatalf("declined C must not carry memory_confidence")
	}
	rec, _ := memorymeta.Review(cc)
	if rec.Action != "decline" || rec.CurrentState != memorymeta.MemoryDeclined {
		t.Fatalf("review record = %#v", rec)
	}
	// Declined edge stays inactive: A remains current.
	if !view.IsCurrent(a.OKFID) || view.IsCurrent(c.OKFID) {
		t.Fatalf("after decline: A current=%v, C current=%v; want A current", view.IsCurrent(a.OKFID), view.IsCurrent(c.OKFID))
	}
}

// S27: undo restores the exact previous confidence, and undo of an approved
// concept that has an approved updater is rejected.
func TestTemporalS27UndoConfidenceAndHasApprovedUpdater(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s27-a", "Fact A", "")

	// Part 1: approve then undo restores the original confidence.
	c := writeRelationNote(t, svc, proposedNoteRequest("s27-c", "Successor to undo", a.OKFID))
	requireReviewResult(t, reviewNote(t, svc, c.Ref, "approve", "proposed"))
	requireReviewResult(t, reviewNote(t, svc, c.Ref, "undo", "approved"))

	reg, _, _ := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryProposed {
		t.Fatalf("C after undo state = %q, want proposed", st)
	}
	if v, present, _ := memorymeta.Confidence(cc); !present || v != 0.6 {
		t.Fatalf("C after undo confidence present=%v value=%v, want restored 0.6", present, v)
	}

	// Part 2: an approved concept with an approved updater cannot be undone.
	leaf := writeRelationNote(t, svc, proposedNoteRequest("s27-leaf", "Leaf to approve", ""))
	requireReviewResult(t, reviewNote(t, svc, leaf.Ref, "approve", "proposed"))
	_ = writeRelationNote(t, svc, WriteKnowledgeRequest{
		Kind: "note", Content: "Updater of leaf", IdempotencyKey: "s27-updater",
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{leaf.OKFID},
	})
	failReview(t, svc, leaf.Ref, "undo", "approved", ErrMemoryHasApprovedUpdater)
}

// S28: invalid transitions and non-durable concepts are rejected; an MCP-authored
// durable concept is reviewable.
func TestTemporalS28InvalidTransitionsAndDurability(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	// Non-durable concept authored directly on disk.
	mustWriteToolFile(t, filepath.Join(repo, ".okf", "knowledge", "sources", "non-durable.md"),
		"---\ntype: source\ntitle: non durable\nokf_id: okf_000000000000000000000000000000aa\n---\nbody\n")
	failReview(t, svc, "okf_000000000000000000000000000000aa", "approve", "proposed", ErrInvalidReviewTransition)

	// Born-approved note: approve-on-approved and undo-on-approved are invalid.
	born := writeApprovedNote(t, svc, "s28-born", "Born approved fact", "")
	failReview(t, svc, born.Ref, "approve", "approved", ErrInvalidReviewTransition)
	failReview(t, svc, born.Ref, "undo", "approved", ErrInvalidReviewTransition)

	// MCP-authored proposed durable note is reviewable (generated.by == okf-mcp).
	c := writeRelationNote(t, svc, proposedNoteRequest("s28-c", "MCP authored proposal", born.OKFID))
	reg, _, concepts := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	_ = concepts
	if cc.Generated == nil || cc.Generated.By != "okf-mcp" {
		t.Fatalf("expected MCP-authored concept, got generated=%#v", cc.Generated)
	}
	if res := requireReviewResult(t, reviewNote(t, svc, c.Ref, "approve", "proposed")); res.CurrentState != "approved" {
		t.Fatalf("MCP-authored concept not reviewable: %#v", res)
	}
}

// S29: under the repo lock, two concurrent CAS approves with expected_state=proposed
// yield exactly one winner; the loser gets memory_state_conflict and no duplicate
// review record is written.
func TestTemporalS29CASRace(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s29-a", "Fact A", "")
	c := writeRelationNote(t, svc, proposedNoteRequest("s29-c", "Race proposal", a.OKFID))

	const racers = 2
	var wg sync.WaitGroup
	results := make([]ToolEnvelope, racers)
	for i := range racers {
		wg.Go(func() {
			results[i] = reviewNote(t, svc, c.Ref, "approve", "proposed")
		})
	}
	wg.Wait()

	wins, conflicts := 0, 0
	for _, env := range results {
		if env.OK {
			wins++
		} else if env.Error != nil && env.Error.Code == ErrMemoryStateConflict {
			conflicts++
		} else {
			t.Fatalf("unexpected race result: %#v", env)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d, want 1/1", wins, conflicts)
	}

	reg, _, _ := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryApproved {
		t.Fatalf("C state = %q, want approved", st)
	}
	rec, _ := memorymeta.Review(cc)
	if rec.Action != "approve" || rec.CurrentState != memorymeta.MemoryApproved {
		t.Fatalf("duplicate/inconsistent review record after race: %#v", rec)
	}
}

// S30: after P1 approves a new updater, approving a stale proposal that targeted
// the now-superseded head fails target_not_current.
func TestTemporalS30ApprovedThenTargetNotCurrent(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s30-a", "Fact A", "")

	// Proposal C targets A while A is still current (inactive, not preflighted).
	c := writeRelationNote(t, svc, proposedNoteRequest("s30-c", "Stale proposal", a.OKFID))

	// P1 approves B updating A; B becomes the current head and A is superseded.
	_ = writeRelationNote(t, svc, WriteKnowledgeRequest{
		Kind: "note", Content: "Approved updater B", IdempotencyKey: "s30-b",
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
	})

	// Approving C now re-runs the preflight against the fresh bundle and fails.
	failReview(t, svc, c.Ref, "approve", "proposed", ErrTargetNotCurrent)

	reg, _, _ := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryProposed {
		t.Fatalf("stale C state = %q, want still proposed", st)
	}
}

// S31: a persistence failure during review restores the original bytes verbatim;
// the reviewed concept is never deleted or half-written.
func TestTemporalS31PersistenceFailureRestoresOriginalBytes(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s31-a", "Fact A", "")
	c := writeRelationNote(t, svc, proposedNoteRequest("s31-c", "Failing review", a.OKFID))

	fullPath := filepath.Join(repo, ".okf", "knowledge", c.ConceptPath)
	original, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}

	svc.writeKnowledgeFile = func(path string, data []byte) error {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		return errors.New("simulated post-rename persistence failure")
	}

	env := reviewNote(t, svc, c.Ref, "approve", "proposed")
	if env.OK || env.Error == nil {
		t.Fatalf("review = %#v, want structured failure", env)
	}

	restored, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(original) {
		t.Fatalf("review did not restore original bytes:\nwant:\n%s\ngot:\n%s", original, restored)
	}

	reg, _, _ := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryProposed {
		t.Fatalf("C state after failed review = %q, want proposed (restored)", st)
	}
	if v, present, _ := memorymeta.Confidence(cc); !present || v != 0.6 {
		t.Fatalf("C confidence after restore present=%v value=%v, want 0.6", present, v)
	}
	if rec, _ := memorymeta.Review(cc); rec.Action != "" {
		t.Fatalf("no review record should be written on failure, got %#v", rec)
	}
}
