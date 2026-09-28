// Package relationrecall provides agent-facing relation expansion: given an
// anchor okf_id, return approved extends neighbors (bidirectional) and the
// approved updates-chain members up to the current head.
package relationrecall

import (
	"fmt"
	"strings"

	"github.com/superops-team/okf/pkg/memorymeta"
)

// Hit is one recalled concept.
type Hit struct {
	OKFID       string `json:"okf_id"`
	Title       string `json:"title"`
	Edge        string `json:"edge"` // extends | updates | self
	IsChainHead bool   `json:"is_chain_head"`
	State       string `json:"state"`
}

// Result is the recall output.
type Result struct {
	Hits     []Hit    `json:"hits"`
	Warnings []string `json:"warnings,omitempty"`
}

// Recall returns approved extends neighbors (both directions) and the
// updates-chain members for anchor. proposed/declined/invalid entries are
// excluded. depth is fixed at 1 (no transitive extends recursion).
func Recall(anchorID string, view *memorymeta.TemporalView) (*Result, error) {
	id := strings.TrimSpace(strings.TrimPrefix(anchorID, "okf://concept/"))
	entry, ok := view.Entries[id]
	if !ok {
		return nil, fmt.Errorf("memory_ref_not_found: %q: %w", anchorID, memorymeta.ErrMemoryRefNotFound)
	}

	res := &Result{}
	seen := map[string]bool{}

	addHit := func(h Hit) {
		if !seen[h.OKFID] {
			seen[h.OKFID] = true
			res.Hits = append(res.Hits, h)
		}
	}

	// Anchor itself (if approved and healthy).
	if entry.State == memorymeta.MemoryApproved && entry.InvalidReason == "" {
		addHit(Hit{OKFID: id, Title: entry.Concept.Title, Edge: "self", State: string(entry.State)})
	}

	// Bidirectional extends neighbors (approved only).
	for otherID, e := range view.Entries {
		if e.State != memorymeta.MemoryApproved || e.InvalidReason != "" {
			continue
		}
		if e.Relation.Kind != memorymeta.RelationExtends {
			continue
		}
		// outgoing: other extends anchor
		for _, tgt := range e.Relation.Targets {
			if tgt == id && otherID != id {
				addHit(Hit{OKFID: otherID, Title: e.Concept.Title, Edge: "extends", State: string(e.State)})
			}
		}
	}
	// incoming: anchor extends others
	if entry.Relation.Kind == memorymeta.RelationExtends {
		for _, tgt := range entry.Relation.Targets {
			if tgt == id {
				continue
			}
			if tgtEntry, ok := view.Entries[tgt]; ok && tgtEntry.State == memorymeta.MemoryApproved && tgtEntry.InvalidReason == "" {
				addHit(Hit{OKFID: tgt, Title: tgtEntry.Concept.Title, Edge: "extends", State: string(tgtEntry.State)})
			}
		}
	}

	// Updates chain: walk to head and include all approved members.
	if history, err := view.History(id); err != nil {
		res.Warnings = append(res.Warnings, "history: "+err.Error())
	} else {
		for _, item := range history.Items {
			if item.MemoryState != string(memorymeta.MemoryApproved) {
				continue
			}
			e := view.Entries[item.OKFID]
			title := ""
			if e != nil && e.Concept != nil {
				title = e.Concept.Title
			}
			addHit(Hit{OKFID: item.OKFID, Title: title, Edge: "updates", IsChainHead: item.Current, State: item.MemoryState})
		}
	}

	return res, nil
}
