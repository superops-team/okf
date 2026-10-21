package lint

// Whole-bundle strict temporal validation (S06 / T3.3).
//
// pkg/lint deliberately does NOT import pkg/memorymeta (that would create an
// import cycle: the conversion sites in cmd/okf and pkg/mcp parse temporal data
// via memorymeta and project it onto the additive TemporalInfo fields of
// lint.Concept). This file re-implements the small, self-contained graph pass
// reading those additive fields, mirroring memorymeta.BuildTemporalView's
// approved-updates topology checks.
//
// Severity contract (design §9 / "default warning, strict failure"):
//   - non-strict: every temporal finding is a Warning (bundle stays usable).
//   - StrictMode: every temporal finding is an Error (HasErrors becomes true).

import "fmt"

// Stable temporal lint codes (mirror memorymeta / pkg/tool wire codes).
const (
	// Per-concept malformed metadata.
	tlInvalidState    = "invalid_memory_state"
	tlInvalidRelation = "invalid_memory_relation"
	// Graph topology findings.
	tlDangling     = "memory_relation_dangling_target"
	tlCrossProject = "memory_relation_cross_project"
	tlAmbiguous    = "ambiguous_update_head"
	tlCycle        = "memory_relation_cycle"
	tlTooDeep      = "temporal_history_too_deep"
)

// maxTemporalHistory bounds the linear approved-updates chain depth (design §4.3),
// matching memorymeta.maxHistoryItems.
const maxTemporalHistory = 128

// durableTemporalTypes are the concept types that may carry temporal memory
// metadata. Kept in sync with memorymeta.IsDurable.
var durableTemporalTypes = map[string]bool{
	"note":     true,
	"event":    true,
	"feedback": true,
}

// isDurable reports whether typ is a durable memory concept type.
func isDurable(typ string) bool { return durableTemporalTypes[typ] }

// hasAnyTemporal reports whether at least one concept in the bundle carries
// temporal metadata. The whole-bundle pass is skipped entirely otherwise, so
// legacy (no-temporal) bundles are byte/semantic identical to before.
func hasAnyTemporal(concepts []*Concept) bool {
	for _, c := range concepts {
		if c != nil && c.Temporal != nil {
			return true
		}
	}
	return false
}

// temporalIssues runs the per-concept malformed checks plus the approved-updates
// graph checks over the bundle. All returned issues carry the configured
// temporal severity (Warning in non-strict, Error in StrictMode).
func temporalIssues(concepts []*Concept, cfg *Config) []Issue {
	sev := Warning
	if cfg != nil && cfg.StrictMode {
		sev = Error
	}

	var issues []Issue

	// --- Per-concept malformed checks -------------------------------------
	for _, c := range concepts {
		if c == nil || c.Temporal == nil {
			continue
		}
		t := c.Temporal
		if t.StateWarn != "" {
			issues = append(issues, Issue{
				FilePath: c.FilePath,
				Severity: sev,
				Code:     tlInvalidState,
				Message:  fmt.Sprintf("malformed memory_state: %s", t.StateWarn),
			})
		}
		if t.ConfidenceWarn != "" {
			issues = append(issues, Issue{
				FilePath: c.FilePath,
				Severity: sev,
				Code:     tlInvalidState,
				Message:  fmt.Sprintf("malformed memory_confidence: %s", t.ConfidenceWarn),
			})
		}
		if t.RelationWarn != "" {
			issues = append(issues, Issue{
				FilePath: c.FilePath,
				Severity: sev,
				Code:     tlInvalidRelation,
				Message:  fmt.Sprintf("malformed memory_relation: %s", t.RelationWarn),
			})
		}
	}

	// --- Graph checks over approved-updates edges --------------------------
	// Index concepts by stable id for target resolution.
	byID := make(map[string]*Concept)
	for _, c := range concepts {
		if c == nil || c.OKFID == "" {
			continue
		}
		byID[c.OKFID] = c
	}

	// Build valid approved-updates edges. A source is active only when it is
	// approved, kind == updates, and exactly one target. Malformed edges that do
	// not meet this shape (dangling / cross-project / self-edge) are reported
	// on the source and never added to the active adjacency.
	outgoing := map[string]string{}   // source -> target
	incoming := map[string][]string{} // target -> [sources]

	addEdgeIssue := func(c *Concept, code, message string) {
		issues = append(issues, Issue{
			FilePath: c.FilePath,
			Severity: sev,
			Code:     code,
			Message:  message,
		})
	}

	for _, c := range concepts {
		if c == nil || c.Temporal == nil {
			continue
		}
		t := c.Temporal
		if t.State != "approved" || t.RelationKind != "updates" || len(t.RelationTargets) != 1 {
			continue
		}
		srcID := c.OKFID
		tgtID := t.RelationTargets[0]

		// Self edge is a trivial cycle; do not add to the active graph.
		if tgtID == srcID {
			addEdgeIssue(c, tlCycle, fmt.Sprintf("updates edge is a self-cycle to %q", tgtID))
			continue
		}

		tgt, ok := byID[tgtID]
		switch {
		case !ok:
			addEdgeIssue(c, tlDangling, fmt.Sprintf("updates target %q is unresolvable in this bundle", tgtID))
			continue
		case !isDurable(tgt.Type):
			addEdgeIssue(c, tlDangling, fmt.Sprintf("updates target %q is not a durable memory type", tgtID))
			continue
		case tgt.Temporal != nil && tgt.Temporal.State != "approved":
			addEdgeIssue(c, tlDangling, fmt.Sprintf("updates target %q is %s, not approved", tgtID, tgt.Temporal.State))
			continue
		case c.Project != tgt.Project:
			addEdgeIssue(c, tlCrossProject, fmt.Sprintf("updates edge crosses projects (%q -> %q)", c.Project, tgt.Project))
			continue
		}

		// Valid active edge.
		outgoing[srcID] = tgtID
		incoming[tgtID] = append(incoming[tgtID], srcID)
	}

	// Ambiguous fork: two or more approved updaters target the same concept.
	// Report on each implicated source (the updaters).
	ambiguousFlagged := map[string]bool{}
	for tgtID, srcs := range incoming {
		if len(srcs) < 2 {
			continue
		}
		for _, srcID := range srcs {
			if ambiguousFlagged[srcID] {
				continue
			}
			ambiguousFlagged[srcID] = true
			if src := byID[srcID]; src != nil {
				addEdgeIssue(src, tlAmbiguous,
					fmt.Sprintf("target %q has %d approved updaters; current head is ambiguous", tgtID, len(srcs)))
			}
		}
	}

	// Cycle: 3-color DFS over the directed active edges. Collect every node on
	// a back-edge cycle and report on each such source.
	cyclic := cyclicNodes(outgoing)
	cycleFlagged := map[string]bool{}
	for _, nodeID := range cyclic {
		if cycleFlagged[nodeID] {
			continue
		}
		cycleFlagged[nodeID] = true
		if node := byID[nodeID]; node != nil {
			addEdgeIssue(node, tlCycle, fmt.Sprintf("updates edge participates in a cycle at %q", nodeID))
		}
	}

	// Depth: walk each chain from its head (newest; no approved updater). If a
	// chain exceeds the safety bound, report temporal_history_too_deep on the
	// head. Visited guards keep walks finite even when a cycle is present.
	deepFlagged := map[string]bool{}
	for _, c := range concepts {
		if c == nil || c.OKFID == "" {
			continue
		}
		// A head has an outgoing edge and no incoming approved updater.
		if _, hasOut := outgoing[c.OKFID]; !hasOut {
			continue
		}
		if _, hasIn := incoming[c.OKFID]; hasIn {
			continue
		}
		if deepFlagged[c.OKFID] {
			continue
		}
		chainLen := 0
		seen := map[string]bool{}
		cur := c.OKFID
		for cur != "" && !seen[cur] {
			seen[cur] = true
			chainLen++
			if chainLen > maxTemporalHistory {
				deepFlagged[c.OKFID] = true
				addEdgeIssue(c, tlTooDeep,
					fmt.Sprintf("update chain exceeds %d nodes", maxTemporalHistory))
				break
			}
			cur = outgoing[cur]
		}
	}

	return issues
}

// cyclicNodes returns the set of node ids that participate in at least one
// directed cycle, given a source->target edge map.
func cyclicNodes(outgoing map[string]string) []string {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	color := map[string]int{}
	var path []string
	cyclic := map[string]bool{}

	var visit func(u string)
	visit = func(u string) {
		color[u] = visiting
		path = append(path, u)

		if tgt, ok := outgoing[u]; ok {
			switch color[tgt] {
			case visiting:
				// Back edge: tgt is already on the current path. Every node
				// from tgt to the end of the path is in a cycle.
				start := 0
				for ; start < len(path); start++ {
					if path[start] == tgt {
						break
					}
				}
				for i := start; i < len(path); i++ {
					cyclic[path[i]] = true
				}
			case unvisited:
				visit(tgt)
			}
		}

		path = path[:len(path)-1]
		color[u] = done
	}

	for id := range outgoing {
		if color[id] == unvisited {
			visit(id)
		}
	}

	out := make([]string, 0, len(cyclic))
	for id := range cyclic {
		out = append(out, id)
	}
	return out
}
