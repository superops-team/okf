package memorymeta

// T0.3 / design §4: deterministic current and history projection over the
// approved-updates relation graph. Currentness is COMPUTED from stable IDs;
// no is_latest flag is persisted, and there is no second index. The graph is
// built once per call from the supplied concept slice, so it is independent of
// input ordering.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/okf"
)

// Stable topology error codes (design §9). History() wraps these so callers
// can errors.Is them.
var (
	ErrMemoryRefNotFound          = errors.New("memory_ref_not_found")
	ErrMemoryRelationCrossProject = errors.New("memory_relation_cross_project")
	ErrMemoryRelationCycle        = errors.New("memory_relation_cycle")
	ErrAmbiguousUpdateHead        = errors.New("ambiguous_update_head")
	ErrMemoryRelationDangling     = errors.New("memory_relation_dangling_target")
	ErrTemporalHistoryTooDeep     = errors.New("temporal_history_too_deep")
)

// maxHistoryItems bounds the linear update chain walked by History (design §4.3).
const maxHistoryItems = 128

// TemporalView is the deterministic projection of a concept bundle onto the
// temporal update graph.
type TemporalView struct {
	// HasTemporalData reports whether any concept in the bundle carries temporal
	// metadata. When false, every durable concept is current (legacy parity).
	HasTemporalData bool
	// Warnings accumulates conservative-read warnings (malformed state/relation).
	Warnings []string
	// Entries is keyed by stable okf id and contains durable concepts only.
	Entries map[string]*TemporalEntry
}

// TemporalEntry is one durable concept's projection state.
type TemporalEntry struct {
	Concept  *okf.Concept
	State    MemoryState
	Relation MemoryRelation
	// Current is true iff this approved durable concept is the head of its chain.
	Current bool
	// InvalidReason is "" when healthy; otherwise a stable code (design §9):
	// memory_relation_cycle / ambiguous_update_head / memory_relation_dangling_target /
	// memory_relation_cross_project.
	InvalidReason string
	// EdgeTarget is the bare okf id this approved-updates edge points to, if any.
	EdgeTarget string
}

// MemoryHistory is the ordered update chain oldest->newest for one ref.
type MemoryHistory struct {
	RequestedRef string
	CurrentRef   string
	Items        []HistoryItem
}

// HistoryItem is one member of a resolved update chain.
type HistoryItem struct {
	OKFID       string
	MemoryState string
	Current     bool
	Edge        *EdgeMeta
}

// EdgeMeta describes the outgoing temporal edge of a chain member.
type EdgeMeta struct {
	Kind   string
	Target string
}

// BuildTemporalView projects the supplied concepts onto the temporal graph.
func BuildTemporalView(concepts []*okf.Concept) *TemporalView {
	v := &TemporalView{Entries: map[string]*TemporalEntry{}}

	reg, regErr := identity.BuildRegistry(concepts)
	if regErr != nil {
		v.Warnings = append(v.Warnings, regErr.Error())
	}

	// Index durable, stable concepts and read their temporal metadata.
	for _, c := range concepts {
		if c == nil {
			continue
		}
		if hasTemporalData(c) {
			v.HasTemporalData = true
		}
		if !IsDurable(c.Type) {
			continue
		}
		id := identity.FromConcept(c).ID
		if id == "" {
			continue // not addressable by stable id
		}
		state, stateWarn := State(c)
		rel, relWarn := Relation(c)
		if stateWarn != "" {
			v.Warnings = append(v.Warnings, stateWarn)
		}
		if relWarn != "" {
			v.Warnings = append(v.Warnings, relWarn)
		}
		v.Entries[id] = &TemporalEntry{
			Concept:  c,
			State:    state,
			Relation: rel,
		}
	}

	// Build valid approved-updates edges. Only approved sources are active.
	outgoing := map[string]string{}     // source -> target
	incoming := map[string][]string{}   // target -> [sources]
	adj := map[string]map[string]bool{} // undirected component adjacency
	addAdj := func(a, b string) {
		if adj[a] == nil {
			adj[a] = map[string]bool{}
		}
		adj[a][b] = true
		if adj[b] == nil {
			adj[b] = map[string]bool{}
		}
		adj[b][a] = true
	}

	for id, e := range v.Entries {
		if e.State != MemoryApproved || e.Relation.Kind != RelationUpdates || len(e.Relation.Targets) != 1 {
			continue
		}
		tgt := e.Relation.Targets[0]

		// Validate the target.
		if tgt == id {
			e.InvalidReason = "memory_relation_cycle"
			continue
		}
		if reg == nil {
			e.InvalidReason = "memory_relation_dangling_target"
			continue
		}
		tgtConcept, err := reg.Resolve(tgt)
		if err != nil {
			e.InvalidReason = "memory_relation_dangling_target"
			continue
		}
		if !IsDurable(tgtConcept.Type) {
			e.InvalidReason = "memory_relation_dangling_target"
			continue
		}
		tgtEntry, ok := v.Entries[tgt]
		if !ok || tgtEntry.State != MemoryApproved {
			e.InvalidReason = "memory_relation_dangling_target"
			continue
		}
		if effectiveProject(e.Concept) != effectiveProject(tgtConcept) {
			e.InvalidReason = "memory_relation_cross_project"
			continue
		}

		// Valid active edge.
		outgoing[id] = tgt
		incoming[tgt] = append(incoming[tgt], id)
		e.EdgeTarget = tgt
		addAdj(id, tgt)
	}

	// Component-wide invalidation helpers.
	markReason := func(start, reason string) {
		visited := map[string]bool{}
		stack := []string{start}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if visited[n] {
				continue
			}
			visited[n] = true
			if e, ok := v.Entries[n]; ok && e.InvalidReason == "" {
				e.InvalidReason = reason
			}
			for nb := range adj[n] {
				if !visited[nb] {
					stack = append(stack, nb)
				}
			}
		}
	}

	// Ambiguous: two+ approved sources update the same target.
	for tgt := range incoming {
		if len(incoming[tgt]) >= 2 {
			markReason(tgt, "ambiguous_update_head")
		}
	}

	// Cycle: color DFS over the directed valid edges (source -> target).
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	color := map[string]int{}
	cyclic := map[string]bool{}
	var dfs func(u string)
	dfs = func(u string) {
		color[u] = visiting
		if tgt, ok := outgoing[u]; ok {
			switch color[tgt] {
			case visiting:
				cyclic[tgt] = true
			case unvisited:
				dfs(tgt)
			}
		}
		color[u] = done
	}
	for id := range v.Entries {
		if color[id] == unvisited {
			dfs(id)
		}
	}
	for start := range cyclic {
		markReason(start, "memory_relation_cycle")
	}

	// Currentness.
	for id, e := range v.Entries {
		if !v.HasTemporalData {
			e.Current = true
			continue
		}
		targeted := len(incoming[id]) > 0
		e.Current = e.State == MemoryApproved && !targeted && e.InvalidReason == ""
	}

	return v
}

// History walks the approved-updates chain containing ref and returns it
// oldest->newest. The head (newest) is CurrentRef. It fails closed with a
// precise stable code for refs in invalid components, missing/non-durable refs,
// and chains longer than the bound.
func (v *TemporalView) History(ref string) (*MemoryHistory, error) {
	id := strings.TrimSpace(ref)
	id = strings.TrimPrefix(id, identity.URIScheme)

	e, ok := v.Entries[id]
	if !ok {
		return nil, fmt.Errorf("memory_ref_not_found: %q: %w", ref, ErrMemoryRefNotFound)
	}
	if e.InvalidReason != "" {
		return nil, fmt.Errorf("%s: %w", e.InvalidReason, reasonToError(e.InvalidReason))
	}

	// Walk up to the head (the newest, with no approved updater). A healthy
	// component is an acyclic chain, so each node has at most one incoming edge.
	head := id
	for {
		srcs := incomingOf(v, head)
		if len(srcs) == 0 {
			break
		}
		head = srcs[0]
	}

	// Walk down from the head to the oldest, bounding the chain length.
	chain := []string{head}
	cur := head
	for {
		tgt, ok := outgoingOf(v, cur)
		if !ok {
			break
		}
		cur = tgt
		chain = append(chain, cur)
		if len(chain) > maxHistoryItems {
			return nil, fmt.Errorf("temporal_history_too_deep: chain exceeds %d items: %w",
				maxHistoryItems, ErrTemporalHistoryTooDeep)
		}
	}

	// chain is newest->oldest; output oldest->newest.
	items := make([]HistoryItem, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		member := chain[i]
		me := v.Entries[member]
		item := HistoryItem{
			OKFID:       member,
			MemoryState: string(me.State),
			Current:     member == head,
		}
		if tgt, ok := outgoingOf(v, member); ok {
			item.Edge = &EdgeMeta{Kind: "updates", Target: tgt}
		}
		items = append(items, item)
	}

	return &MemoryHistory{
		RequestedRef: ref,
		CurrentRef:   head,
		Items:        items,
	}, nil
}

// IsCurrent reports whether the given stable id is a current head.
func (v *TemporalView) IsCurrent(id string) bool {
	e, ok := v.Entries[id]
	return ok && e.Current
}

// CurrentSet returns the set of current stable ids.
func (v *TemporalView) CurrentSet() map[string]bool {
	out := make(map[string]bool)
	for id, e := range v.Entries {
		if e.Current {
			out[id] = true
		}
	}
	return out
}

// incomingOf / outgoingOf recompute the adjacency for History from the stored
// edges. They are derived from the view's entries so History stays a pure read.
func incomingOf(v *TemporalView, id string) []string {
	var srcs []string
	for sid, e := range v.Entries {
		if e.InvalidReason != "" || e.State != MemoryApproved {
			continue
		}
		if e.Relation.Kind == RelationUpdates && len(e.Relation.Targets) == 1 && e.Relation.Targets[0] == id {
			srcs = append(srcs, sid)
		}
	}
	return srcs
}

func outgoingOf(v *TemporalView, id string) (string, bool) {
	e, ok := v.Entries[id]
	if !ok || e.InvalidReason != "" || e.State != MemoryApproved {
		return "", false
	}
	if e.Relation.Kind == RelationUpdates && len(e.Relation.Targets) == 1 {
		return e.Relation.Targets[0], true
	}
	return "", false
}

// reasonToError maps a stored InvalidReason code to its sentinel error.
func reasonToError(reason string) error {
	switch reason {
	case "memory_relation_cycle":
		return ErrMemoryRelationCycle
	case "ambiguous_update_head":
		return ErrAmbiguousUpdateHead
	case "memory_relation_dangling_target":
		return ErrMemoryRelationDangling
	case "memory_relation_cross_project":
		return ErrMemoryRelationCrossProject
	default:
		return ErrMemoryRefNotFound
	}
}

func hasTemporalData(c *okf.Concept) bool {
	if c == nil || c.CustomFields == nil {
		return false
	}
	for _, k := range []string{"memory_state", "memory_confidence", "memory_relation", "memory_review"} {
		if _, ok := c.CustomFields[k]; ok {
			return true
		}
	}
	return false
}

func effectiveProject(c *okf.Concept) string {
	if c == nil {
		return ""
	}
	if v, ok := c.CustomFields["project"].(string); ok {
		return v
	}
	return ""
}
