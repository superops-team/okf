package query

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/superops-team/okf/pkg/identity"
)

// GroupBy selects the hierarchical dimension over which the already-fused,
// scored candidate pool is projected (design §5). Projection is a pure,
// post-retrieval shaping step: it never changes channel candidates, scores or
// relevance formulas, and the ungrouped path keeps its existing output.
type GroupBy string

const (
	GroupChunk   GroupBy = "chunk"
	GroupConcept GroupBy = "concept"
	GroupSource  GroupBy = "source"
	GroupFolder  GroupBy = "folder"
)

// ErrInvalidGroupBy is returned (via errors.Is) when a public request names a
// group_by outside chunk|concept|source|folder. It never silently falls back.
var ErrInvalidGroupBy = errors.New("invalid_group_by")

// GroupError carries the stable invalid_group_by code for callers that map it
// into the tool envelope.
type GroupError struct {
	GroupBy string
}

func (e *GroupError) Error() string { return "invalid_group_by: " + e.GroupBy }
func (e *GroupError) Is(target error) bool {
	t, ok := target.(*GroupError)
	return ok && t != nil && e != nil
}

// ResultHit is the flat, entry-point-agnostic adapter for one already-scored
// candidate. It carries everything needed to compute group keys without
// re-reading the concept or re-running retrieval.
type ResultHit struct {
	// OKFID is the concept's stable id (empty for legacy concepts).
	OKFID string
	// ParentOKFID is present on derived chunk concepts; concept grouping uses it.
	ParentOKFID string
	// LegacyFingerprint is the deterministic fallback key for legacy concepts.
	LegacyFingerprint string
	Ref               string
	// ConceptPath is the normalized bundle-relative concept file path.
	ConceptPath string
	// SourcePath is the normalized source_path custom field (may be empty).
	SourcePath string
	StartLine  int
	EndLine    int
	// Rank is the 1-based position in the input (fused, scored) order.
	Rank int
	// Score is the unchanged fused score; the group score is always the
	// representative's score (never re-aggregated).
	Score      float64
	Provenance string
}

// HitRef identifies one member of a group without embedding the whole hit.
type HitRef struct {
	Ref         string `json:"ref,omitempty"`
	ConceptPath string `json:"concept_path,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	Rank        int    `json:"rank"`
}

// GroupedHit is one projected group. The representative is the first raw member
// by input rank; HitCount/ConceptCount/SourceCount use unique normalized keys.
// CoveredSources is the deduplicated, sorted set of all source/concept
// identifiers this group contains (from every member, not just the
// representative). It is always populated (even when includeMembers=false) so
// that grouped-utility metrics (S47 relevant-source recall) can compute
// coverage over the full group membership.
type GroupedHit struct {
	GroupBy        GroupBy   `json:"group_by"`
	GroupKey       string    `json:"group_key"`
	Ref            string    `json:"ref,omitempty"`
	Representative ResultHit `json:"representative"`
	HitCount       int       `json:"hit_count"`
	ConceptCount   int       `json:"concept_count"`
	SourceCount    int       `json:"source_count"`
	CoveredSources []string  `json:"covered_sources,omitempty"`
	Members        []HitRef  `json:"members,omitempty"`
}

// Project groups the already-fused, scored hits by groupBy. Input order is
// authoritative: the first raw member of each group is its representative, its
// score is unchanged, and groups are ordered by representative rank then
// group_key. At most limit groups are returned (fewer when the pool has fewer
// unique keys). includeMembers=false omits the member list but still computes
// counts. It never increases the candidate pool or performs a second retrieval.
func Project(hits []ResultHit, groupBy GroupBy, includeMembers bool, limit int) ([]GroupedHit, []string, error) {
	if !isKnownGroupBy(groupBy) {
		return nil, nil, &GroupError{GroupBy: string(groupBy)}
	}

	accs := make(map[string]*groupAcc)
	order := make([]string, 0, len(hits))
	var warnings []string

	for i := range hits {
		h := hits[i]
		key, warns := groupKey(h, groupBy)
		warnings = append(warnings, warns...)
		acc, ok := accs[key]
		if !ok {
			acc = &groupAcc{
				key:          key,
				rep:          h,
				concepts:     map[string]struct{}{},
				sources:      map[string]struct{}{},
				coveredPaths: map[string]struct{}{},
			}
			accs[key] = acc
			order = append(order, key)
		}
		acc.hits = append(acc.hits, h)
		acc.concepts[conceptIdentityKey(h)] = struct{}{}
		if src := normalizedSourceForKey(h); src != "" {
			acc.sources[src] = struct{}{}
		}
		// CoveredPaths: plain normalized source or concept path (no prefix),
		// used by S47 relevant-source recall to match against golden expected
		// docs. Source path takes precedence; concept path is the fallback.
		if src, ok := normalizedSource(h); ok {
			acc.coveredPaths[src] = struct{}{}
		} else if cp, ok := normalizeRelPath(h.ConceptPath); ok {
			acc.coveredPaths[cp] = struct{}{}
		}
	}

	groups := make([]GroupedHit, 0, len(order))
	for _, key := range order {
		acc := accs[key]
		rep := acc.rep
		// CoveredSources: deduplicated, sorted set of all plain source/concept
		// paths this group contains (from every member, not just the
		// representative). Always populated (even when includeMembers=false) so
		// S47 relevant-source recall can compute coverage over the full group
		// membership. These are plain normalized paths (no src:/concept: prefix)
		// so they match golden expected_docs identifiers.
		covered := make([]string, 0, len(acc.coveredPaths))
		for s := range acc.coveredPaths {
			covered = append(covered, s)
		}
		sort.Strings(covered)
		g := GroupedHit{
			GroupBy:        groupBy,
			GroupKey:       key,
			Ref:            rep.Ref,
			Representative: rep,
			HitCount:       len(acc.hits),
			ConceptCount:   len(acc.concepts),
			SourceCount:    len(acc.sources),
			CoveredSources: covered,
		}
		if includeMembers {
			g.Members = make([]HitRef, len(acc.hits))
			for i, m := range acc.hits {
				g.Members[i] = HitRef{Ref: m.Ref, ConceptPath: m.ConceptPath, SourcePath: m.SourcePath, Rank: m.Rank}
			}
		}
		groups = append(groups, g)
	}

	// Deterministic order: representative input rank first, then group_key.
	sort.SliceStable(groups, func(a, b int) bool {
		if groups[a].Representative.Rank != groups[b].Representative.Rank {
			return groups[a].Representative.Rank < groups[b].Representative.Rank
		}
		return strings.Compare(groups[a].GroupKey, groups[b].GroupKey) < 0
	})

	if limit > 0 && len(groups) > limit {
		groups = groups[:limit]
	}
	return groups, dedupWarnings(warnings), nil
}

type groupAcc struct {
	key          string
	rep          ResultHit
	hits         []ResultHit
	concepts     map[string]struct{}
	sources      map[string]struct{} // prefixed keys (src:/concept:) for SourceCount
	coveredPaths map[string]struct{} // plain normalized paths for CoveredSources
}

func isKnownGroupBy(g GroupBy) bool {
	switch g {
	case GroupChunk, GroupConcept, GroupSource, GroupFolder:
		return true
	}
	return false
}

// groupKey computes the group key for one hit and any warnings (e.g. a folder
// path that fell back to concept grouping).
func groupKey(h ResultHit, groupBy GroupBy) (string, []string) {
	switch groupBy {
	case GroupChunk:
		if h.StartLine > 0 || h.EndLine > 0 {
			return fmt.Sprintf("%s@%d:%d", conceptKeyForHit(h), h.StartLine, h.EndLine), nil
		}
		return fmt.Sprintf("%s@rank:%d", conceptKeyForHit(h), h.Rank), nil
	case GroupConcept:
		return conceptGroupKey(h), nil
	case GroupSource:
		if src, ok := normalizedSource(h); ok {
			return "src:" + src, nil
		}
		return "concept:" + conceptKeyForHit(h), nil
	case GroupFolder:
		return folderKey(h)
	}
	return "", nil
}

// conceptKeyForHit is the identity-aware concept key (v3:id / v3:legacy).
func conceptKeyForHit(h ResultHit) string {
	return identity.Key(h.OKFID, h.LegacyFingerprint)
}

// conceptGroupKey uses the parent identity when a derived chunk carries a valid
// parent_okf_id; otherwise the concept's own id; otherwise a legacy fallback.
func conceptGroupKey(h ResultHit) string {
	if _, err := identity.Parse(h.ParentOKFID); err == nil {
		return "id:" + h.ParentOKFID
	}
	if _, err := identity.Parse(h.OKFID); err == nil {
		return "id:" + h.OKFID
	}
	return "legacy:" + h.LegacyFingerprint
}

func conceptIdentityKey(h ResultHit) string { return conceptGroupKey(h) }

// normalizedSourceForKey returns the normalized source path for unique counting,
// falling back to the concept key when there is no usable source.
func normalizedSourceForKey(h ResultHit) string {
	if src, ok := normalizedSource(h); ok {
		return "src:" + src
	}
	return "concept:" + conceptKeyForHit(h)
}

// normalizedSource validates and normalizes a source path. It rejects absolute
// paths and ".." escapes.
func normalizedSource(h ResultHit) (string, bool) {
	return normalizeRelPath(h.SourcePath)
}

// folderKey derives a portable, bundle-relative folder key. A valid source path
// takes precedence; otherwise the concept path. The knowledge root maps to ".".
// An invalid path (absolute / .. escape) cannot become a folder key and falls
// back to concept grouping with a warning.
func folderKey(h ResultHit) (string, []string) {
	if src, ok := normalizeRelPath(h.SourcePath); ok {
		return "folder:" + dirOf(src), nil
	}
	if concept, ok := normalizeRelPath(h.ConceptPath); ok {
		return "folder:" + dirOf(concept), nil
	}
	warn := "invalid path fell back to concept grouping for hit rank " + fmt.Sprint(h.Rank)
	return "concept:" + conceptKeyForHit(h), []string{warn}
}

// normalizeRelPath enforces the portable path contract: absolute paths and
// cleaned paths whose first segment is ".." are rejected. Output uses "/".
func normalizeRelPath(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	if filepath.IsAbs(p) {
		return "", false
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == "." || clean == "" {
		return "", false
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// dirOf returns the "/" dirname of a normalized relative path; the root maps to
// "." (design §5.2).
func dirOf(normalized string) string {
	if i := strings.LastIndex(normalized, "/"); i >= 0 {
		return normalized[:i]
	}
	return "."
}

func dedupWarnings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, w := range in {
		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		out = append(out, w)
	}
	return out
}
