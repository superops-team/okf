package query

import "github.com/superops-team/okf/pkg/okf"

// This file is the single canonical conversion between the OKF core model
// (pkg/okf) and the query-layer concept. Both the classic CLI (cmd/okf) and
// the MCP server (pkg/mcp) and the Service (pkg/tool) route through here so
// that CustomFields (including okf_id and source_path) are always preserved.
//
// The conversion is defensive: Tags and CustomFields are deep-copied so that
// mutating the query.Concept never aliases or mutates the source okf.Concept.

// ConceptFromOKF converts one okf.Concept into a query.Concept, deep-copying
// Tags and CustomFields.
func ConceptFromOKF(c *okf.Concept) *Concept {
	if c == nil {
		return nil
	}
	return &Concept{
		Type:         c.Type,
		Title:        c.Title,
		Description:  c.Description,
		Resource:     c.Resource,
		Tags:         cloneStringSlice(c.Tags),
		Content:      c.Content,
		FilePath:     c.FilePath,
		CustomFields: cloneCustomFields(c.CustomFields),
	}
}

// BundleFromOKF converts an entire OKF bundle into a query bundle.
func BundleFromOKF(b *okf.KnowledgeBundle) *KnowledgeBundle {
	if b == nil {
		return nil
	}
	concepts := make([]*Concept, 0, len(b.Concepts))
	for _, c := range b.Concepts {
		concepts = append(concepts, ConceptFromOKF(c))
	}
	return &KnowledgeBundle{Concepts: concepts}
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneCustomFields(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneCustomValue(v)
	}
	return dst
}

// cloneCustomValue recursively copies the composite JSON-like values that can
// appear in CustomFields (maps and slices). Scalar values (string, bool,
// float64, nil) are immutable and can be shared safely.
func cloneCustomValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for k, nested := range typed {
			copied[k] = cloneCustomValue(nested)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, nested := range typed {
			copied[i] = cloneCustomValue(nested)
		}
		return copied
	default:
		return v
	}
}
