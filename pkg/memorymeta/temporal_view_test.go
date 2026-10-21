package memorymeta

// T0.3 / S08–S16, S40: deterministic temporal current/history projection.
// Golden topologies are built in-memory; currentness and history must be
// independent of input order and must never silently pick a winner in a fork.

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/quick"

	"github.com/superops-team/okf/pkg/okf"
)

// mkMem builds a durable memory concept. state="" omits memory_state (defaults
// approved). relKind="" omits memory_relation. project="" omits project.
func mkMem(id, typ, project, state, relKind string, targets ...string) *okf.Concept {
	cf := map[string]any{"okf_id": id}
	if project != "" {
		cf["project"] = project
	}
	if state != "" {
		cf["memory_state"] = state
	}
	if relKind != "" {
		tgts := make([]any, len(targets))
		for i, t := range targets {
			tgts[i] = t
		}
		cf["memory_relation"] = map[string]any{"kind": relKind, "targets": tgts}
	}
	return &okf.Concept{Type: typ, CustomFields: cf}
}

// mkChain builds an approved update chain oldest->newest: ids[0] oldest, each
// ids[i] (i>0) updates ids[i-1].
func mkChain(ids []string) []*okf.Concept {
	out := make([]*okf.Concept, len(ids))
	for i, id := range ids {
		if i == 0 {
			out[i] = mkMem(id, "note", "", "", "")
		} else {
			out[i] = mkMem(id, "note", "", "", "updates", ids[i-1])
		}
	}
	return out
}

func entry(v *TemporalView, t *testing.T, id string) *TemporalEntry {
	t.Helper()
	e, ok := v.Entries[id]
	if !ok {
		t.Fatalf("Entries missing %s", id)
	}
	return e
}

// S08: legacy approved memories and extends edges all remain current.
func TestS08LegacyAndExtendsCurrent(t *testing.T) {
	a := mkMem(idA, "note", "", "", "")
	b := mkMem(idB, "note", "", "", "extends", idA)
	v := BuildTemporalView([]*okf.Concept{a, b})

	if !v.HasTemporalData {
		t.Fatal("view should report HasTemporalData")
	}
	if !entry(v, t, idA).Current {
		t.Fatal("A (legacy) must be current")
	}
	if !entry(v, t, idB).Current {
		t.Fatal("B (extends) must be current; extends never makes a target historical")
	}
	if got := v.CurrentSet()[idA]; !got {
		t.Fatal("CurrentSet must include A")
	}
}

// S09: linear chain A<-B<-C selects exactly C as current.
func TestS09LinearChainSelectsHead(t *testing.T) {
	chain := mkChain([]string{idA, idB, idC})
	v := BuildTemporalView(chain)

	if !entry(v, t, idC).Current {
		t.Fatal("C (newest) must be current")
	}
	if entry(v, t, idA).Current {
		t.Fatal("A (oldest) must be historical")
	}
	if entry(v, t, idB).Current {
		t.Fatal("B (middle) must be historical")
	}
	if got := v.IsCurrent(idA); got {
		t.Fatal("IsCurrent(A)=true, want false (not current head -> target_not_current)")
	}
	if got := v.IsCurrent(idC); !got {
		t.Fatal("IsCurrent(C)=false, want true")
	}
	if e := entry(v, t, idB); e.EdgeTarget != idA {
		t.Fatalf("B.EdgeTarget = %q, want %q", e.EdgeTarget, idA)
	}
}

// S10: proposed/declined sources are inactive; A stays current.
func TestS10ProposedDeclinedInactive(t *testing.T) {
	a := mkMem(idA, "note", "", "", "")
	p := mkMem(idP, "note", "", "proposed", "updates", idA)
	d := mkMem(idD, "note", "", "declined", "updates", idA)
	v := BuildTemporalView([]*okf.Concept{a, p, d})

	if !entry(v, t, idA).Current {
		t.Fatal("A must remain current despite proposed/declined updaters")
	}
	if entry(v, t, idP).Current {
		t.Fatal("proposed P must not be current")
	}
	if entry(v, t, idD).Current {
		t.Fatal("declined D must not be current")
	}
}

// S12: two approved sources updating the same target -> ambiguous_update_head.
func TestS12AmbiguousFork(t *testing.T) {
	a := mkMem(idA, "note", "", "", "")
	b := mkMem(idB, "note", "", "", "updates", idA)
	c := mkMem(idC, "note", "", "", "updates", idA)
	v := BuildTemporalView([]*okf.Concept{a, b, c})

	for _, id := range []string{idA, idB, idC} {
		e := entry(v, t, id)
		if e.InvalidReason != "ambiguous_update_head" {
			t.Fatalf("%s InvalidReason = %q, want ambiguous_update_head", id, e.InvalidReason)
		}
		if e.Current {
			t.Fatalf("%s must be non-current in an invalid component", id)
		}
	}
	if _, err := v.History(idB); !errors.Is(err, ErrAmbiguousUpdateHead) {
		t.Fatalf("History(B) err = %v, want ambiguous_update_head", err)
	}
}

// S13: directed update cycle isolates the component with memory_relation_cycle.
func TestS13Cycle(t *testing.T) {
	a := mkMem(idA, "note", "", "", "updates", idB)
	b := mkMem(idB, "note", "", "", "updates", idA)
	v := BuildTemporalView([]*okf.Concept{a, b})

	if e := entry(v, t, idA); e.InvalidReason != "memory_relation_cycle" {
		t.Fatalf("A.InvalidReason = %q, want memory_relation_cycle", e.InvalidReason)
	}
	if e := entry(v, t, idB); e.InvalidReason != "memory_relation_cycle" {
		t.Fatalf("B.InvalidReason = %q, want memory_relation_cycle", e.InvalidReason)
	}
	if _, err := v.History(idA); !errors.Is(err, ErrMemoryRelationCycle) {
		t.Fatalf("History(A) err = %v, want memory_relation_cycle", err)
	}
}

// S14: dangling target (missing / non-durable) -> memory_relation_dangling_target.
func TestS14DanglingAndNonDurable(t *testing.T) {
	missing := validID(90)
	a := mkMem(idA, "note", "", "", "updates", missing)
	v := BuildTemporalView([]*okf.Concept{a})

	if e := entry(v, t, idA); e.InvalidReason != "memory_relation_dangling_target" {
		t.Fatalf("A.InvalidReason = %q, want memory_relation_dangling_target", e.InvalidReason)
	}
	if _, err := v.History(idA); !errors.Is(err, ErrMemoryRelationDangling) {
		t.Fatalf("History(A) err = %v, want memory_relation_dangling_target", err)
	}

	// A non-durable "code" concept is never an Entry and cannot resolve a ref.
	code := &okf.Concept{Type: "code", CustomFields: map[string]any{"okf_id": idP}}
	v2 := BuildTemporalView([]*okf.Concept{code, a})
	if _, ok := v2.Entries[idP]; ok {
		t.Fatal("non-durable code concept must not appear in Entries")
	}
	if _, err := v2.History(idP); !errors.Is(err, ErrMemoryRefNotFound) {
		t.Fatalf("History(non-durable) err = %v, want memory_ref_not_found", err)
	}
}

// S14: cross-project target -> memory_relation_cross_project.
func TestS14CrossProject(t *testing.T) {
	a := mkMem(idA, "note", "proj-x", "", "")
	b := mkMem(idB, "note", "proj-y", "", "updates", idA)
	v := BuildTemporalView([]*okf.Concept{a, b})

	if e := entry(v, t, idB); e.InvalidReason != "memory_relation_cross_project" {
		t.Fatalf("B.InvalidReason = %q, want memory_relation_cross_project (got %q)",
			e.InvalidReason, e.InvalidReason)
	}
	// Same-project edge is fine; empty project == empty project.
	a2 := mkMem(idC, "note", "", "", "")
	c2 := mkMem(idD, "note", "", "", "updates", idC)
	v3 := BuildTemporalView([]*okf.Concept{a2, c2})
	if e := entry(v3, t, idD); e.InvalidReason != "" {
		t.Fatalf("same empty-project edge should be healthy, got %q", e.InvalidReason)
	}
	if !entry(v3, t, idD).Current {
		t.Fatal("head D in shared empty project must be current")
	}
}

// S15: history resolves the same ordered chain from any member; current_ref=head.
func TestS15HistoryFromAnyMember(t *testing.T) {
	chain := mkChain([]string{idA, idB, idC})
	v := BuildTemporalView(chain)

	for _, ref := range []string{idA, idB, idC} {
		h, err := v.History(ref)
		if err != nil {
			t.Fatalf("History(%s) err: %v", ref, err)
		}
		if h.RequestedRef != ref {
			t.Fatalf("RequestedRef = %q, want %q", h.RequestedRef, ref)
		}
		if h.CurrentRef != idC {
			t.Fatalf("CurrentRef = %q, want head %q", h.CurrentRef, idC)
		}
		gotOrder := []string{h.Items[0].OKFID, h.Items[1].OKFID, h.Items[2].OKFID}
		wantOrder := []string{idA, idB, idC}
		if !slices.Equal(gotOrder, wantOrder) {
			t.Fatalf("History(%s) order = %v, want %v", ref, gotOrder, wantOrder)
		}
	}
	// Head is current; middle items carry their outgoing edge.
	h, _ := v.History(idA)
	if !h.Items[2].Current || h.Items[0].Current || h.Items[1].Current {
		t.Fatalf("only the head C must be marked current: %+v", h.Items)
	}
	if h.Items[1].Edge == nil || h.Items[1].Edge.Target != idA || h.Items[1].Edge.Kind != "updates" {
		t.Fatalf("B edge = %+v, want updates->A", h.Items[1].Edge)
	}
	if h.Items[0].Edge != nil {
		t.Fatalf("oldest A must have no edge, got %+v", h.Items[0].Edge)
	}
}

// History on a standalone durable concept returns a single-item chain.
func TestHistoryStandaloneAndMissing(t *testing.T) {
	a := mkMem(idA, "note", "", "", "")
	v := BuildTemporalView([]*okf.Concept{a})

	h, err := v.History(idA)
	if err != nil {
		t.Fatalf("History(standalone) err: %v", err)
	}
	if len(h.Items) != 1 || h.Items[0].OKFID != idA || !h.Items[0].Current {
		t.Fatalf("standalone history = %+v", h.Items)
	}
	if h.CurrentRef != idA {
		t.Fatalf("CurrentRef = %q, want self", h.CurrentRef)
	}

	// Missing ref.
	if _, err := v.History(validID(77)); !errors.Is(err, ErrMemoryRefNotFound) {
		t.Fatalf("History(missing) err = %v, want memory_ref_not_found", err)
	}
}

// HasTemporalData=false: a repo with no temporal fields keeps every durable
// concept current; History still works on an explicit durable ref.
func TestS17LegacyRepoAllCurrent(t *testing.T) {
	a := mkMem(idA, "note", "", "", "")
	b := mkMem(idB, "event", "", "", "")
	// Strip temporal markers: these have none, but also no memory_relation.
	v := BuildTemporalView([]*okf.Concept{a, b})
	if v.HasTemporalData {
		t.Fatal("legacy repo must report HasTemporalData=false")
	}
	if !v.IsCurrent(idA) || !v.IsCurrent(idB) {
		t.Fatal("legacy repo: every durable concept must be current")
	}
	if _, err := v.History(idA); err != nil {
		t.Fatalf("History must still work on explicit ref: %v", err)
	}
}

// S16: a chain longer than 128 fails with temporal_history_too_deep.
func TestS16HistoryDepthBound(t *testing.T) {
	n := 129
	ids := make([]string, n)
	for i := range n {
		ids[i] = validID(200 + i)
	}
	v := BuildTemporalView(mkChain(ids))

	if _, err := v.History(ids[0]); !errors.Is(err, ErrTemporalHistoryTooDeep) {
		t.Fatalf("History over 129 items err = %v, want temporal_history_too_deep", err)
	}
	// A chain exactly at the bound (128) succeeds.
	ids128 := ids[:128]
	v128 := BuildTemporalView(mkChain(ids128))
	h, err := v128.History(ids128[0])
	if err != nil {
		t.Fatalf("128-item history must succeed, got %v", err)
	}
	if len(h.Items) != 128 {
		t.Fatalf("128-item history returned %d items", len(h.Items))
	}
}

// Property: the projection is independent of input ordering.
func TestS40PermutationIndependence(t *testing.T) {
	corpus := []*okf.Concept{
		mkMem(idA, "note", "", "", ""),
		mkMem(idB, "note", "", "", "updates", idA),
		mkMem(idC, "note", "", "", "updates", idB),
		mkMem(idD, "event", "", "proposed", "extends", idA),
	}
	golden := BuildTemporalView(corpus)
	goldenSet := golden.CurrentSet()

	// quick.Check over random seeds; each seed yields a true permutation of the
	// corpus (every concept appears exactly once, order varies).
	prop := func(seed int64) bool {
		order := []int{0, 1, 2, 3}
		r := uint64(seed)
		if r == 0 {
			r = 1
		}
		for i := len(order) - 1; i > 0; i-- {
			r = r*6364136223867930055 + 1442695040888963407
			j := int(r % uint64(i+1))
			order[i], order[j] = order[j], order[i]
		}
		shuffled := make([]*okf.Concept, len(corpus))
		for i, idx := range order {
			shuffled[i] = corpus[idx]
		}
		v := BuildTemporalView(shuffled)
		if !mapsEqual(v.CurrentSet(), goldenSet) {
			return false
		}
		for id, ge := range golden.Entries {
			e, ok := v.Entries[id]
			if !ok || e.InvalidReason != ge.InvalidReason || e.Current != ge.Current {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatalf("permutation independence violated: %v", err)
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Compile-time guard that the public surface exists with the expected shapes.
var (
	_ func([]*okf.Concept) *TemporalView   = BuildTemporalView
	_ func(string) (*MemoryHistory, error) = (*TemporalView)(nil).History
	_ func(string) bool                    = (*TemporalView)(nil).IsCurrent
	_ func() map[string]bool               = (*TemporalView)(nil).CurrentSet
	_                                      = strings.Contains
)
