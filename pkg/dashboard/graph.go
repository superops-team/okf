// Package dashboard provides an embedded web dashboard for exploring an OKF
// knowledge bundle as an interactive knowledge graph.
//
// Architecture:
//
//	graph.go   - bundle → GraphData conversion (pure, testable)
//	api.go     - HTTP handlers (JSON API, testable via httptest)
//	server.go  - server lifecycle, static asset embedding, command wiring
//	web/       - frontend (single HTML, embedded via go:embed)
//
// The frontend talks exclusively to /api/v1/*. Handlers never render HTML;
// they return structured JSON. This keeps the contract stable as the UI evolves.
package dashboard

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/superops-team/okf/pkg/okf"
)

// GraphNode is a single concept in the visual graph.
type GraphNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	FilePath    string   `json:"filePath,omitempty"`
	Folder      string   `json:"folder,omitempty"`
	OKFID       string   `json:"okfId,omitempty"`
	ParentOKFID string   `json:"parentOkfId,omitempty"`
	// Size is a derived visual weight (tag count + content length bucketed).
	Size int `json:"size"`
}

// GraphEdge is a directed relationship between two concepts.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	// Kind describes the relationship: "parent", "source", "tag", "folder".
	Kind string `json:"kind"`
}

// GraphData is the full payload served to the frontend.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	// Stats provides aggregate numbers for the dashboard header.
	Stats GraphStats `json:"stats"`
}

// GraphStats holds aggregate counts for quick display.
type GraphStats struct {
	TotalConcepts int            `json:"totalConcepts"`
	Types         map[string]int `json:"types"`
	Tags          map[string]int `json:"tags"`
	Folders       map[string]int `json:"folders"`
	TotalEdges    int            `json:"totalEdges"`
}

// BuildGraph converts a KnowledgeBundle into visual graph data.
//
// Node identity: okf_id when present, otherwise a stable hash of FilePath.
// Edges are derived from:
//   - parent_okf_id  → parent-child hierarchy
//   - shared tags    → tag-based similarity (one edge per shared tag pair,
//     capped to avoid O(n²) blowup on large tag sets)
//   - shared folder  → folder co-location (only when no other edge exists)
func BuildGraph(bundle *okf.KnowledgeBundle) *GraphData {
	if bundle == nil || len(bundle.Concepts) == 0 {
		return &GraphData{
			Nodes: []GraphNode{},
			Edges: []GraphEdge{},
			Stats: GraphStats{Types: map[string]int{}, Tags: map[string]int{}, Folders: map[string]int{}},
		}
	}

	nodes := make([]GraphNode, 0, len(bundle.Concepts))
	idByPath := make(map[string]string, len(bundle.Concepts))
	idByOKF := make(map[string]string, len(bundle.Concepts))
	stats := GraphStats{
		TotalConcepts: len(bundle.Concepts),
		Types:         map[string]int{},
		Tags:          map[string]int{},
		Folders:       map[string]int{},
	}

	for _, c := range bundle.Concepts {
		if c == nil {
			continue
		}
		node := conceptToNode(c)
		nodes = append(nodes, node)
		idByPath[c.FilePath] = node.ID
		if node.OKFID != "" {
			idByOKF[node.OKFID] = node.ID
		}
		stats.Types[node.Type]++
		if node.Folder != "" {
			stats.Folders[node.Folder]++
		}
		for _, t := range node.Tags {
			stats.Tags[t]++
		}
	}

	edges := buildEdges(nodes, idByOKF)
	stats.TotalEdges = len(edges)

	return &GraphData{Nodes: nodes, Edges: edges, Stats: stats}
}

// conceptToNode converts a single OKF concept to a graph node.
func conceptToNode(c *okf.Concept) GraphNode {
	okfID, _ := c.CustomFields["okf_id"].(string)
	parentOKFID, _ := c.CustomFields["parent_okf_id"].(string)

	title := c.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(c.FilePath), filepath.Ext(c.FilePath))
	}

	folder := filepath.Dir(c.FilePath)
	if folder == "." {
		folder = ""
	}

	// Visual weight: tags contribute 5 each, content length bucketed.
	size := 15 + len(c.Tags)*5
	if len(c.Content) > 2000 {
		size += 10
	}
	if len(c.Content) > 8000 {
		size += 10
	}

	return GraphNode{
		ID:          nodeID(c, okfID),
		Title:       title,
		Type:        c.Type,
		Description: c.Description,
		Tags:        c.Tags,
		FilePath:    c.FilePath,
		Folder:      folder,
		OKFID:       okfID,
		ParentOKFID: parentOKFID,
		Size:        size,
	}
}

// nodeID returns a stable identifier for a concept.
// Precedence: okf_id > sha1-like path hash > FilePath.
func nodeID(c *okf.Concept, okfID string) string {
	if okfID != "" {
		return okfID
	}
	// Fallback: use FilePath as-is (it is unique within a bundle).
	return c.FilePath
}

// buildEdges derives relationships from the node set.
func buildEdges(nodes []GraphNode, idByOKF map[string]string) []GraphEdge {
	edges := make([]GraphEdge, 0, len(nodes)*2)
	seen := make(map[string]bool, len(nodes)*2)

	addEdge := func(src, dst, kind string) {
		if src == "" || dst == "" || src == dst {
			return
		}
		key := src + "|" + dst + "|" + kind
		if seen[key] {
			return
		}
		seen[key] = true
		edges = append(edges, GraphEdge{Source: src, Target: dst, Kind: kind})
	}

	// 1. Parent-child edges from parent_okf_id.
	for _, n := range nodes {
		if n.ParentOKFID != "" {
			if parentID, ok := idByOKF[n.ParentOKFID]; ok {
				addEdge(parentID, n.ID, "parent")
			}
		}
	}

	// 2. Tag-based edges: concepts sharing at least one tag.
	//    Build tag → node IDs, then connect within each tag group.
	//    Cap group size to avoid O(n²) on very common tags.
	tagGroups := make(map[string][]string, 64)
	for _, n := range nodes {
		for _, t := range n.Tags {
			tagGroups[t] = append(tagGroups[t], n.ID)
		}
	}
	for _, group := range tagGroups {
		if len(group) < 2 || len(group) > 50 {
			continue // skip singleton and over-common tags
		}
		// Connect each node to the first in the group (star topology)
		// instead of full mesh — keeps edge count linear.
		center := group[0]
		for i := 1; i < len(group); i++ {
			addEdge(center, group[i], "tag")
		}
	}

	// 3. Folder co-location: concepts in the same folder (only when
	//    no other edge connects them, to avoid redundant edges).
	folderGroups := make(map[string][]string, 32)
	for _, n := range nodes {
		if n.Folder != "" {
			folderGroups[n.Folder] = append(folderGroups[n.Folder], n.ID)
		}
	}
	for _, group := range folderGroups {
		if len(group) < 2 || len(group) > 30 {
			continue
		}
		center := group[0]
		for i := 1; i < len(group); i++ {
			// Only add folder edge if no other relationship already exists.
			keyA := center + "|" + group[i]
			keyB := group[i] + "|" + center
			if !hasAnyEdge(seen, keyA) && !hasAnyEdge(seen, keyB) {
				addEdge(center, group[i], "folder")
			}
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		return edges[i].Target < edges[j].Target
	})

	return edges
}

// hasAnyEdge checks whether any edge (regardless of kind) exists for the
// given source|target prefix key.
func hasAnyEdge(seen map[string]bool, prefix string) bool {
	// We stored keys as "src|dst|kind"; check all three kinds.
	for _, kind := range []string{"parent", "tag", "folder"} {
		if seen[prefix+"|"+kind] {
			return true
		}
	}
	return false
}

// ConceptDetail is the full detail payload for a single concept, served by
// /api/v1/concepts/{id}. It includes the markdown content (truncated).
type ConceptDetail struct {
	GraphNode
	Content      string            `json:"content,omitempty"`
	CustomFields map[string]string `json:"customFields,omitempty"`
}

// BuildConceptDetail returns detail for a single concept by node ID.
// Returns nil if not found. Content is truncated to 8000 chars to keep
// API responses bounded.
func BuildConceptDetail(bundle *okf.KnowledgeBundle, nodeID string) *ConceptDetail {
	if bundle == nil {
		return nil
	}
	for _, c := range bundle.Concepts {
		if c == nil {
			continue
		}
		okfID, _ := c.CustomFields["okf_id"].(string)
		conceptID := okfID
		if conceptID == "" {
			conceptID = c.FilePath
		}
		if conceptID != nodeID {
			continue
		}
		node := conceptToNode(c)
		detail := &ConceptDetail{GraphNode: node, CustomFields: flattenCustomFields(c.CustomFields)}
		if len(c.Content) > 8000 {
			detail.Content = c.Content[:8000] + "\n\n... (truncated, full content in source file)"
		} else {
			detail.Content = c.Content
		}
		return detail
	}
	return nil
}

// flattenCustomFields converts the raw CustomFields map to a string map for
// JSON display. Non-string values are rendered via %v.
func flattenCustomFields(fields map[string]interface{}) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		switch val := v.(type) {
		case string:
			out[k] = val
		default:
			out[k] = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(
				strings.ReplaceAll(fmt.Sprintf("%v", val), "map[", ""), "]", ""), " ", ", "))
		}
	}
	return out
}
