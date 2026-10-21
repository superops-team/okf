package tool

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/memorymeta"
	"github.com/superops-team/okf/pkg/okf"
)

// ---------------------------------------------------------------------------
// Shared temporal test helpers (S18-S31)
// ---------------------------------------------------------------------------

func floatPtr(v float64) *float64 { return &v }

func writeApprovedNote(t *testing.T, svc *Service, key, content, project string) WriteKnowledgeResult {
	t.Helper()
	resp := svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
		Kind: "note", Content: content, Project: project, IdempotencyKey: key,
	})
	if !resp.OK {
		t.Fatalf("write approved note %q failed: %#v", key, resp.Error)
	}
	return requireWriteResult(t, resp)
}

func writeRelationNote(t *testing.T, svc *Service, req WriteKnowledgeRequest) WriteKnowledgeResult {
	t.Helper()
	resp := svc.WriteKnowledge(t.Context(), req)
	if !resp.OK {
		t.Fatalf("write temporal note %q failed: %#v", req.IdempotencyKey, resp.Error)
	}
	return requireWriteResult(t, resp)
}

func failWrite(t *testing.T, svc *Service, req WriteKnowledgeRequest, wantCode string) {
	t.Helper()
	resp := svc.WriteKnowledge(t.Context(), req)
	if resp.OK || resp.Error == nil || resp.Error.Code != wantCode {
		t.Fatalf("write = %#v, want error code %q", resp, wantCode)
	}
}

func loadTemporalBundle(t *testing.T, repo string) (*identity.Registry, *memorymeta.TemporalView, []*okf.Concept) {
	t.Helper()
	bundle, err := loadKnowledgeBundleForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := identity.BuildRegistry(bundle.Concepts)
	if err != nil {
		t.Fatal(err)
	}
	view := memorymeta.BuildTemporalView(bundle.Concepts)
	return reg, view, bundle.Concepts
}

func resolveConcept(t *testing.T, reg *identity.Registry, ref string) *okf.Concept {
	t.Helper()
	c, err := reg.Resolve(ref)
	if err != nil {
		t.Fatalf("resolve %q: %v", ref, err)
	}
	return c
}

func knowledgeFileCount(t *testing.T, repo string) int {
	t.Helper()
	return len(regularFilesUnder(t, filepath.Join(repo, ".okf", "knowledge")))
}

// S18: a legacy durable write (no temporal fields) keeps its on-disk output
// free of memory_* metadata and preserves concept identity/idempotency semantics.
func TestTemporalS18LegacyWriteByteSemanticParity(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	req := WriteKnowledgeRequest{
		Kind: "note", Content: "Legacy durable fact with no temporal fields.",
		Project: "proj-a", Tags: []string{"parity"}, IdempotencyKey: "s18-legacy-v1",
	}
	first := requireWriteResult(t, svc.WriteKnowledge(t.Context(), req))
	if first.OKFID == "" || first.Ref == "" {
		t.Fatalf("expected stable okf_id/ref, got okf_id=%q ref=%q", first.OKFID, first.Ref)
	}

	_, view, concepts := loadTemporalBundle(t, repo)
	if len(concepts) != 1 {
		t.Fatalf("concept count = %d, want 1", len(concepts))
	}
	c := concepts[0]
	for _, k := range []string{"memory_state", "memory_confidence", "memory_relation", "memory_review"} {
		if _, present := c.CustomFields[k]; present {
			t.Fatalf("legacy write must not carry %q, got %#v", k, c.CustomFields[k])
		}
	}
	if view.HasTemporalData {
		t.Fatalf("legacy write must not enable temporal data")
	}

	// Idempotent retry keeps the same concept identity.
	second := requireWriteResult(t, svc.WriteKnowledge(t.Context(), req))
	if second.ConceptID != first.ConceptID || second.OKFID != first.OKFID || second.Created {
		t.Fatalf("idempotent retry changed identity: first=%#v second=%#v", first, second)
	}
	if knowledgeFileCount(t, repo) != 1 {
		t.Fatalf("idempotent retry must not create a new file")
	}
}

// S19: an approved relation write activates immediately — the updater becomes
// the current head and the target stops being current.
func TestTemporalS19ApprovedUpdateActivatesImmediately(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	a := writeApprovedNote(t, svc, "s19-a", "Original fact A", "")

	b := writeRelationNote(t, svc, WriteKnowledgeRequest{
		Kind: "note", Content: "Updated fact B", IdempotencyKey: "s19-b",
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
	})
	if b.OKFID == "" || b.OKFID == a.OKFID {
		t.Fatalf("expected distinct updater okf_id, got %q", b.OKFID)
	}

	_, view, _ := loadTemporalBundle(t, repo)
	if !view.IsCurrent(b.OKFID) {
		t.Fatalf("updater B must be current head")
	}
	if view.IsCurrent(a.OKFID) {
		t.Fatalf("target A must no longer be current once B updates it")
	}
	if entry := view.Entries[b.OKFID]; entry == nil || entry.EdgeTarget != a.OKFID {
		t.Fatalf("expected B->A edge, view entry = %#v", view.Entries[b.OKFID])
	}
}

// S20: a proposed relation write is quarantined — it returns okf_id/ref, stores
// the relation metadata, but does NOT make the target stop being current.
func TestTemporalS20ProposedQuarantine(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})

	a := writeApprovedNote(t, svc, "s20-a", "Current fact A", "")

	c := writeRelationNote(t, svc, WriteKnowledgeRequest{
		Kind: "note", Content: "Proposed successor C", IdempotencyKey: "s20-c",
		MemoryState: "proposed", MemoryConfidence: floatPtr(0.5),
		EvidenceRefs:       []string{"evidence/s20-c"},
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
	})
	if c.OKFID == "" || c.Ref == "" {
		t.Fatalf("proposed write must return okf_id/ref, got %#v", c)
	}

	reg, view, _ := loadTemporalBundle(t, repo)
	if !view.IsCurrent(a.OKFID) {
		t.Fatalf("A must stay current while C is only proposed")
	}
	if view.IsCurrent(c.OKFID) {
		t.Fatalf("proposed C must not be current")
	}
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryProposed {
		t.Fatalf("C state = %q, want proposed", st)
	}
	if v, ok, _ := memorymeta.Confidence(cc); !ok || v != 0.5 {
		t.Fatalf("C confidence present=%v value=%v, want present 0.5", ok, v)
	}
	if rel, _ := memorymeta.Relation(cc); rel.Kind != memorymeta.RelationUpdates || len(rel.Targets) != 1 || rel.Targets[0] != a.OKFID {
		t.Fatalf("C relation metadata = %#v", rel)
	}
}

// S21: the temporal fields enter the payload hash. An identical retry after a
// review reuses the concept and keeps its reviewed state; changing the temporal
// fields under the same key surfaces as an idempotency conflict.
func TestTemporalS21IdempotencyHashAndRetryAfterReview(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s21-a", "Fact A", "")

	proposed := WriteKnowledgeRequest{
		Kind: "note", Content: "Proposed successor C", IdempotencyKey: "s21-c",
		MemoryState: "proposed", MemoryConfidence: floatPtr(0.5),
		EvidenceRefs:       []string{"evidence/s21-c"},
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
	}
	c := writeRelationNote(t, svc, proposed)

	// Review C to approved via P2.
	review := svc.ReviewMemory(t.Context(), ReviewMemoryRequest{
		Ref: c.Ref, Action: "approve", ExpectedState: "proposed",
	})
	if !review.OK {
		t.Fatalf("approve failed: %#v", review.Error)
	}

	// Identical retry must reuse C and NOT revert it to proposed.
	retry := requireWriteResult(t, svc.WriteKnowledge(t.Context(), proposed))
	if retry.Created {
		t.Fatalf("identical retry must reuse the existing concept")
	}
	reg, _, _ := loadTemporalBundle(t, repo)
	cc := resolveConcept(t, reg, c.OKFID)
	if st, _ := memorymeta.State(cc); st != memorymeta.MemoryApproved {
		t.Fatalf("C reverted to %q after retry, want approved (review preserved)", st)
	}

	// Changed confidence under the same idempotency key is a conflict.
	conflict := proposed
	conflict.MemoryConfidence = floatPtr(0.9)
	failWrite(t, svc, conflict, ErrIdempotencyConflict)

	// Changed relation target under the same key is also a conflict.
	other := writeApprovedNote(t, svc, "s21-other", "Other fact", "")
	relConflict := proposed
	relConflict.MemoryRelationTargets = []string{other.OKFID}
	failWrite(t, svc, relConflict, ErrIdempotencyConflict)
}

// S22: an approved relation write that fails the bundle preflight writes ZERO
// files — the invalid concept never appears on disk.
func TestTemporalS22PreflightRollbackZeroWrite(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s22-a", "Fact A", "")

	// Dangling target: syntactically valid okf id that no concept owns.
	failWrite(t, svc, WriteKnowledgeRequest{
		Kind: "note", Content: "Dangling updater", IdempotencyKey: "s22-b",
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{"okf_00000000000000000000000000000000"},
	}, ErrMemoryRefNotFound)

	// Non-current target: A is no longer current once a valid updater exists.
	first := writeRelationNote(t, svc, WriteKnowledgeRequest{
		Kind: "note", Content: "Valid updater B", IdempotencyKey: "s22-b",
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
	})
	_ = first
	failWrite(t, svc, WriteKnowledgeRequest{
		Kind: "note", Content: "Second updater C", IdempotencyKey: "s22-c",
		MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
	}, ErrTargetNotCurrent)

	if got := knowledgeFileCount(t, repo); got != 2 {
		t.Fatalf("knowledge files = %d, want exactly A and B (zero invalid files)", got)
	}
}

// S23: concurrent approved updates to the same target serialize — at most one
// wins and the rest are rejected with target_not_current.
func TestTemporalS23ConcurrentApprovedUpdatesSerialize(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s23-a", "Fact A", "")

	const writers = 8
	var wg sync.WaitGroup
	results := make([]ToolEnvelope, writers)
	for i := range writers {
		wg.Go(func() {
			idx := i
			results[idx] = svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
				Kind: "note", Content: "Concurrent updater " + strconv.Itoa(idx), IdempotencyKey: "s23-updater-" + strconv.Itoa(idx),
				MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
			})
		})
	}
	wg.Wait()

	succeeded := 0
	for _, resp := range results {
		if resp.OK {
			succeeded++
		} else if resp.Error == nil || resp.Error.Code != ErrTargetNotCurrent {
			t.Fatalf("concurrent write error = %#v, want %q", resp.Error, ErrTargetNotCurrent)
		}
	}
	if succeeded != 1 {
		t.Fatalf("succeeded writers = %d, want exactly 1", succeeded)
	}

	_, view, _ := loadTemporalBundle(t, repo)
	if view.IsCurrent(a.OKFID) {
		t.Fatalf("A should no longer be current once one updater succeeded")
	}
	currentCount := 0
	for _, e := range view.Entries {
		if e.EdgeTarget == a.OKFID && e.Current {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("current updaters of A = %d, want 1", currentCount)
	}
}

// S24: the fixed lock order (repo temporal lock -> per-path lock) does not
// deadlock when temporal and legacy writes run concurrently; the whole mix
// completes well within a deadline.
func TestTemporalS24LockOrderNoDeadlock(t *testing.T) {
	repo := initToolTestRepo(t)
	svc := NewService(Config{RepoPath: repo})
	a := writeApprovedNote(t, svc, "s24-a", "Fact A", "")

	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for i := range 12 {
			wg.Go(func() {
				idx := i
				// Half temporal, half legacy.
				if idx%2 == 0 {
					svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
						Kind: "note", Content: "legacy " + strconv.Itoa(idx), IdempotencyKey: "s24-legacy-" + strconv.Itoa(idx),
					})
				} else {
					svc.WriteKnowledge(t.Context(), WriteKnowledgeRequest{
						Kind: "note", Content: "temporal " + strconv.Itoa(idx), IdempotencyKey: "s24-temporal-" + strconv.Itoa(idx),
						MemoryRelationKind: "updates", MemoryRelationTargets: []string{a.OKFID},
					})
				}
			})
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("temporal/legacy concurrent writes deadlocked (lock order violation)")
	}
}
