package mcp

import (
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

// conceptToParser converts an okf.Concept to a parser.Concept for serialization.
func conceptToParser(c *okf.Concept) *parser.Concept {
	if c == nil {
		return nil
	}
	pc := &parser.Concept{
		Type:         c.Type,
		Title:        c.Title,
		Description:  c.Description,
		Resource:     c.Resource,
		Tags:         c.Tags,
		Timestamp:    c.Timestamp,
		Content:      c.Content,
		FilePath:     c.FilePath,
		CustomFields: c.CustomFields,
		Status:       string(c.Status),
		StaleAfter:   c.StaleAfter,
		Runtime:      c.Runtime,
		Computation:  c.Computation,
	}

	if len(c.Sources) > 0 {
		pc.Sources = make([]parser.Source, len(c.Sources))
		for i, s := range c.Sources {
			pc.Sources[i] = parser.Source{
				ID:           s.ID,
				Resource:     s.Resource,
				Title:        s.Title,
				Author:       s.Author,
				UsageCount:   s.UsageCount,
				LastModified: s.LastModified,
			}
		}
	}
	if c.UsageWindow != nil {
		pc.UsageWindow = &parser.UsageWindow{From: c.UsageWindow.From, To: c.UsageWindow.To}
	}
	if c.Generated != nil {
		pc.Generated = &parser.GeneratedInfo{By: c.Generated.By, At: c.Generated.At}
	}
	if len(c.Verified) > 0 {
		pc.Verified = make([]parser.VerificationEvent, len(c.Verified))
		for i, v := range c.Verified {
			pc.Verified[i] = parser.VerificationEvent{By: v.By, At: v.At}
		}
	}
	if len(c.Parameters) > 0 {
		pc.Parameters = make([]parser.Parameter, len(c.Parameters))
		for i, p := range c.Parameters {
			pc.Parameters[i] = parser.Parameter{Name: p.Name, Type: p.Type, Required: p.Required}
		}
	}
	if c.Executor != nil {
		pc.Executor = &parser.ExecutorRef{Resource: c.Executor.Resource, Receipt: c.Executor.Receipt}
	}
	if c.Attester != nil {
		pc.Attester = &parser.AttesterRef{Resource: c.Attester.Resource}
	}

	return pc
}

// countSources counts total sources across all concepts in a bundle.
func countSources(bundle *okf.KnowledgeBundle) int {
	count := 0
	if bundle != nil {
		for _, c := range bundle.Concepts {
			count += len(c.Sources)
		}
	}
	return count
}
