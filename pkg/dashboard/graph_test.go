package dashboard

import (
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

func makeTestBundle() *okf.KnowledgeBundle {
	return &okf.KnowledgeBundle{
		Name: "test",
		Concepts: []*okf.Concept{
			{
				Type:        "Concept",
				Title:       "Parent Concept",
				Description: "This is a parent concept",
				Tags:        []string{"tag1", "tag2"},
				FilePath:    "parent.md",
				Content:     "Parent content here",
				CustomFields: map[string]interface{}{
					"okf_id": "okf_parent123",
				},
			},
			{
				Type:        "Concept",
				Title:       "Child Concept",
				Description: "This is a child concept",
				Tags:        []string{"tag1", "tag3"},
				FilePath:    "sub/child.md",
				Content:     "Child content here",
				CustomFields: map[string]interface{}{
					"okf_id":        "okf_child456",
					"parent_okf_id": "okf_parent123",
					"source_path":   "src/child.go",
				},
			},
			{
				Type:        "Process",
				Title:       "Sibling Process",
				Description: "A process in the same folder as parent",
				Tags:        []string{"tag2"},
				FilePath:    "sibling.md",
				Content:     "Sibling content",
				CustomFields: map[string]interface{}{
					"okf_id": "okf_sibling789",
				},
			},
			{
				Type:        "Concept",
				Title:       "No ID Concept",
				Description: "Concept without okf_id, uses path as identity",
				Tags:        []string{"tag3"},
				FilePath:    "noid.md",
				Content:     "No ID content",
			},
		},
	}
}

func TestBuildGraph_NilBundle(t *testing.T) {
	g := BuildGraph(nil)
	if g == nil {
		t.Fatal("expected non-nil graph for nil bundle")
	}
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(g.Edges))
	}
	if g.Stats.TotalConcepts != 0 {
		t.Errorf("expected 0 total concepts, got %d", g.Stats.TotalConcepts)
	}
}

func TestBuildGraph_EmptyBundle(t *testing.T) {
	g := BuildGraph(&okf.KnowledgeBundle{})
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.Nodes))
	}
}

func TestBuildGraph_NodeCount(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)
	if len(g.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(g.Nodes))
	}
}

func TestBuildGraph_NodeIdentity(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)

	// Nodes with okf_id should use it as identity.
	found := map[string]bool{}
	for _, n := range g.Nodes {
		found[n.ID] = true
	}
	if !found["okf_parent123"] {
		t.Error("expected node with id okf_parent123")
	}
	if !found["okf_child456"] {
		t.Error("expected node with id okf_child456")
	}
	// Node without okf_id should use FilePath.
	if !found["noid.md"] {
		t.Error("expected node with id noid.md (path fallback)")
	}
}

func TestBuildGraph_NodeFields(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)

	var parent *GraphNode
	for i := range g.Nodes {
		if g.Nodes[i].ID == "okf_parent123" {
			parent = &g.Nodes[i]
			break
		}
	}
	if parent == nil {
		t.Fatal("parent node not found")
	}
	if parent.Title != "Parent Concept" {
		t.Errorf("expected title 'Parent Concept', got '%s'", parent.Title)
	}
	if parent.Type != "Concept" {
		t.Errorf("expected type 'Concept', got '%s'", parent.Type)
	}
	if len(parent.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(parent.Tags))
	}
	if parent.FilePath != "parent.md" {
		t.Errorf("expected filePath 'parent.md', got '%s'", parent.FilePath)
	}
	if parent.OKFID != "okf_parent123" {
		t.Errorf("expected okfId 'okf_parent123', got '%s'", parent.OKFID)
	}
	if parent.Size < 15 {
		t.Errorf("expected size >= 15, got %d", parent.Size)
	}
}

func TestBuildGraph_ParentEdges(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)

	parentEdgeCount := 0
	for _, e := range g.Edges {
		if e.Kind == "parent" {
			parentEdgeCount++
			if e.Source != "okf_parent123" || e.Target != "okf_child456" {
				t.Errorf("unexpected parent edge: %s -> %s", e.Source, e.Target)
			}
		}
	}
	if parentEdgeCount != 1 {
		t.Errorf("expected 1 parent edge, got %d", parentEdgeCount)
	}
}

func TestBuildGraph_TagEdges(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)

	tagEdgeCount := 0
	for _, e := range g.Edges {
		if e.Kind == "tag" {
			tagEdgeCount++
		}
	}
	// tag1 shared by parent+child, tag2 shared by parent+sibling, tag3 shared by child+noid
	// Star topology: 3 tag groups × 1 edge each = 3 tag edges
	if tagEdgeCount < 2 {
		t.Errorf("expected at least 2 tag edges, got %d", tagEdgeCount)
	}
}

func TestBuildGraph_Stats(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)

	if g.Stats.TotalConcepts != 4 {
		t.Errorf("expected 4 total concepts, got %d", g.Stats.TotalConcepts)
	}
	if g.Stats.Types["Concept"] != 3 {
		t.Errorf("expected 3 Concept type, got %d", g.Stats.Types["Concept"])
	}
	if g.Stats.Types["Process"] != 1 {
		t.Errorf("expected 1 Process type, got %d", g.Stats.Types["Process"])
	}
	if g.Stats.Tags["tag1"] != 2 {
		t.Errorf("expected tag1 count 2, got %d", g.Stats.Tags["tag1"])
	}
	if g.Stats.Tags["tag2"] != 2 {
		t.Errorf("expected tag2 count 2, got %d", g.Stats.Tags["tag2"])
	}
	if g.Stats.Folders["sub"] != 1 {
		t.Errorf("expected folder 'sub' count 1, got %d", g.Stats.Folders["sub"])
	}
}

func TestBuildGraph_NoDuplicateEdges(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)

	seen := map[string]bool{}
	for _, e := range g.Edges {
		key := e.Source + "|" + e.Target + "|" + e.Kind
		if seen[key] {
			t.Errorf("duplicate edge found: %s -> %s (%s)", e.Source, e.Target, e.Kind)
		}
		seen[key] = true
	}
}

func TestBuildGraph_EdgesSorted(t *testing.T) {
	bundle := makeTestBundle()
	g := BuildGraph(bundle)

	for i := 1; i < len(g.Edges); i++ {
		if g.Edges[i-1].Source > g.Edges[i].Source {
			t.Errorf("edges not sorted by source at index %d", i)
		}
	}
}

func TestConceptToNode_TitleFallback(t *testing.T) {
	c := &okf.Concept{
		Type:     "Concept",
		FilePath: "my-document.md",
		Content:  "content",
	}
	node := conceptToNode(c)
	if node.Title != "my-document" {
		t.Errorf("expected title fallback 'my-document', got '%s'", node.Title)
	}
}

func TestConceptToNode_FolderRoot(t *testing.T) {
	c := &okf.Concept{
		Type:     "Concept",
		Title:    "Root Doc",
		FilePath: "root.md",
	}
	node := conceptToNode(c)
	if node.Folder != "" {
		t.Errorf("expected empty folder for root file, got '%s'", node.Folder)
	}
}

func TestBuildConceptDetail_Found(t *testing.T) {
	bundle := makeTestBundle()
	detail := BuildConceptDetail(bundle, "okf_parent123")
	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	if detail.Title != "Parent Concept" {
		t.Errorf("expected title 'Parent Concept', got '%s'", detail.Title)
	}
	if detail.Content != "Parent content here" {
		t.Errorf("expected content 'Parent content here', got '%s'", detail.Content)
	}
	if detail.CustomFields["okf_id"] != "okf_parent123" {
		t.Errorf("expected custom field okf_id, got '%v'", detail.CustomFields["okf_id"])
	}
}

func TestBuildConceptDetail_NotFound(t *testing.T) {
	bundle := makeTestBundle()
	detail := BuildConceptDetail(bundle, "nonexistent_id")
	if detail != nil {
		t.Error("expected nil detail for nonexistent id")
	}
}

func TestBuildConceptDetail_NilBundle(t *testing.T) {
	detail := BuildConceptDetail(nil, "anything")
	if detail != nil {
		t.Error("expected nil detail for nil bundle")
	}
}

func TestBuildConceptDetail_ContentTruncation(t *testing.T) {
	longContent := make([]byte, 10000)
	for i := range longContent {
		longContent[i] = 'x'
	}
	bundle := &okf.KnowledgeBundle{
		Concepts: []*okf.Concept{
			{
				Type:     "Concept",
				Title:    "Long Doc",
				FilePath: "long.md",
				Content:  string(longContent),
				CustomFields: map[string]interface{}{
					"okf_id": "okf_long",
				},
			},
		},
	}
	detail := BuildConceptDetail(bundle, "okf_long")
	if detail == nil {
		t.Fatal("expected non-nil detail")
	}
	if len(detail.Content) > 8100 {
		t.Errorf("content should be truncated to ~8000 chars, got %d", len(detail.Content))
	}
	if !strings.HasSuffix(detail.Content, "full content in source file)") {
		t.Errorf("content should end with truncation notice, got: ...%s", detail.Content[len(detail.Content)-40:])
	}
}

func TestBuildConceptDetail_PathIdentity(t *testing.T) {
	bundle := makeTestBundle()
	// noid.md has no okf_id, should be found by FilePath.
	detail := BuildConceptDetail(bundle, "noid.md")
	if detail == nil {
		t.Fatal("expected detail for noid.md (path identity)")
	}
	if detail.Title != "No ID Concept" {
		t.Errorf("expected title 'No ID Concept', got '%s'", detail.Title)
	}
}

func TestFlattenCustomFields(t *testing.T) {
	fields := map[string]interface{}{
		"string_key": "value",
		"int_key":    42,
		"bool_key":   true,
	}
	result := flattenCustomFields(fields)
	if result["string_key"] != "value" {
		t.Errorf("expected string value, got '%s'", result["string_key"])
	}
	if result["int_key"] != "42" {
		t.Errorf("expected int flattened to '42', got '%s'", result["int_key"])
	}
	if result["bool_key"] != "true" {
		t.Errorf("expected bool flattened to 'true', got '%s'", result["bool_key"])
	}
}

func TestFlattenCustomFields_Nil(t *testing.T) {
	result := flattenCustomFields(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestMatchScore(t *testing.T) {
	c := &okf.Concept{
		Title:       "Authentication Service",
		Description: "Handles user login and tokens",
		Tags:        []string{"security", "auth"},
		FilePath:    "services/auth.md",
	}
	// Title match = 50
	if score := matchScore(c, "auth"); score < 50 {
		t.Errorf("expected score >= 50 for title match, got %d", score)
	}
	// Tag match = 30
	if score := matchScore(c, "security"); score < 30 {
		t.Errorf("expected score >= 30 for tag match, got %d", score)
	}
	// No match
	if score := matchScore(c, "nonexistent"); score != 0 {
		t.Errorf("expected score 0 for no match, got %d", score)
	}
}

func TestBuildGraph_LargeBundlePerformance(t *testing.T) {
	// Build a bundle with 200 concepts to ensure no O(n²) blowup.
	concepts := make([]*okf.Concept, 200)
	for i := range concepts {
		concepts[i] = &okf.Concept{
			Type:     "Concept",
			Title:    "Concept " + string(rune('A'+i%26)),
			Tags:     []string{"shared-tag"},
			FilePath: "concept_" + string(rune('a'+i%26)) + ".md",
			Content:  "content",
			CustomFields: map[string]interface{}{
				"okf_id": "okf_concept_" + string(rune('a'+i%26)),
			},
		}
	}
	bundle := &okf.KnowledgeBundle{Concepts: concepts}
	g := BuildGraph(bundle)
	if len(g.Nodes) != 200 {
		t.Errorf("expected 200 nodes, got %d", len(g.Nodes))
	}
	// shared-tag group has 200 nodes (>50 cap), so no tag edges should be added.
	tagEdges := 0
	for _, e := range g.Edges {
		if e.Kind == "tag" {
			tagEdges++
		}
	}
	if tagEdges != 0 {
		t.Errorf("expected 0 tag edges for over-common tag (>50 cap), got %d", tagEdges)
	}
}
