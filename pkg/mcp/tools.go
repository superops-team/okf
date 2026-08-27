package mcp

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/superops-team/okf/pkg/lint"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
)

// ToolHandler is a function that handles a tool call.
type ToolHandler func(args map[string]interface{}) (*ToolCallResult, error)

// ToolRegistry manages MCP tools and their handlers.
type ToolRegistry struct {
	mu         sync.RWMutex
	tools      map[string]Tool
	handlers   map[string]ToolHandler
	bundle     *okf.KnowledgeBundle
	bundlePath string
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
	}
	r.registerCoreTools()
	return r
}

// Register registers a tool and its handler.
func (r *ToolRegistry) Register(tool Tool, handler ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
	r.handlers[tool.Name] = handler
}

// List returns all registered tools.
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// Call invokes a tool by name.
func (r *ToolRegistry) Call(name string, args map[string]interface{}) (*ToolCallResult, error) {
	r.mu.RLock()
	handler, ok := r.handlers[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return handler(args)
}

// SetBundle sets the current knowledge bundle.
func (r *ToolRegistry) SetBundle(b *okf.KnowledgeBundle, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bundle = b
	r.bundlePath = path
}

// GetBundle returns the current bundle and its path.
func (r *ToolRegistry) GetBundle() (*okf.KnowledgeBundle, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bundle, r.bundlePath
}

func (r *ToolRegistry) registerCoreTools() {
	r.Register(Tool{
		Name:        "okf_load_bundle",
		Description: "Load an OKF knowledge bundle from a directory path",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the knowledge bundle root directory",
				},
			},
			"required": []string{"path"},
		},
	}, r.handleLoadBundle)

	r.Register(Tool{
		Name:        "okf_bundle_stats",
		Description: "Get statistics about the loaded knowledge bundle",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, r.handleBundleStats)

	r.Register(Tool{
		Name:        "okf_list_concepts",
		Description: "List concepts in the knowledge bundle, optionally filtered by type",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Filter by concept type",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of concepts to return (default: 50)",
				},
			},
		},
	}, r.handleListConcepts)

	r.Register(Tool{
		Name:        "okf_get_concept",
		Description: "Get full details of a concept by its file path",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "File path of the concept (relative to bundle root)",
				},
			},
			"required": []string{"path"},
		},
	}, r.handleGetConcept)

	r.Register(Tool{
		Name:        "okf_search",
		Description: "Search concepts by text query, type, or tag",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Text search query (matches title, description, content, tags)",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Filter by concept type",
				},
				"tag": map[string]interface{}{
					"type":        "string",
					"description": "Filter by tag",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum results (default: 20)",
				},
			},
		},
	}, r.handleSearch)

	r.Register(Tool{
		Name:        "okf_lint_bundle",
		Description: "Run lint checks on the entire knowledge bundle",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"strict": map[string]interface{}{
					"type":        "boolean",
					"description": "Strict mode (warnings fail)",
				},
			},
		},
	}, r.handleLintBundle)

	r.Register(Tool{
		Name:        "okf_lint_concept",
		Description: "Run lint checks on a single concept",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "File path of the concept to lint",
				},
			},
			"required": []string{"path"},
		},
	}, r.handleLintConcept)
}

// --- Handlers ---

func (r *ToolRegistry) handleLoadBundle(args map[string]interface{}) (*ToolCallResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return errorResult("path is required"), nil
	}

	bundle, err := okf.LoadBundle(path, &okf.LoadOptions{Recursive: true})
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to load bundle: %v", err)), nil
	}

	r.SetBundle(bundle, path)
	stats := bundle.Stats()
	msg := fmt.Sprintf("Loaded bundle from %s\nTotal concepts: %d\nUnique types: %d\nUnique tags: %d",
		path, stats.TotalConcepts, stats.UniqueTypes, stats.UniqueTags)

	return &ToolCallResult{Content: []ContentItem{TextContent(msg)}}, nil
}

func (r *ToolRegistry) handleBundleStats(args map[string]interface{}) (*ToolCallResult, error) {
	bundle, path := r.GetBundle()
	if bundle == nil {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}

	stats := bundle.Stats()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Bundle: %s\n\n", path))
	sb.WriteString(fmt.Sprintf("Total concepts: %d\n", stats.TotalConcepts))
	sb.WriteString(fmt.Sprintf("Unique types: %d\n", stats.UniqueTypes))
	sb.WriteString(fmt.Sprintf("Unique tags: %d\n\n", stats.UniqueTags))

	sb.WriteString("By type:\n")
	for t, c := range stats.TypeCounts {
		sb.WriteString(fmt.Sprintf("  - %s: %d\n", t, c))
	}

	sb.WriteString("\nBy status:\n")
	for s, c := range stats.StatusCounts {
		sb.WriteString(fmt.Sprintf("  - %s: %d\n", s, c))
	}

	sb.WriteString("\nTrust tiers:\n")
	for t, c := range stats.TrustTierCounts {
		sb.WriteString(fmt.Sprintf("  - %s: %d\n", t, c))
	}

	sb.WriteString(fmt.Sprintf("\nStale concepts: %d\n", stats.StaleCount))
	sb.WriteString(fmt.Sprintf("Attested computations: %d\n", stats.AttestedComputationCount))
	sb.WriteString(fmt.Sprintf("Total sources: %d\n", countSources(bundle)))

	return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
}

func (r *ToolRegistry) handleListConcepts(args map[string]interface{}) (*ToolCallResult, error) {
	bundle, _ := r.GetBundle()
	if bundle == nil {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}

	typeFilter, _ := args["type"].(string)
	limit := 50
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	var sb strings.Builder
	count := 0
	for _, c := range bundle.Concepts {
		if typeFilter != "" && c.Type != typeFilter {
			continue
		}
		if count >= limit {
			break
		}
		stale := ""
		if c.IsStale(time.Now()) {
			stale = " [STALE]"
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s%s\n", count+1, c.Type, c.FilePath, stale))
		if c.Title != "" {
			sb.WriteString(fmt.Sprintf("   Title: %s\n", c.Title))
		}
		if c.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", c.Description))
		}
		count++
	}

	if count == 0 {
		sb.WriteString("No concepts found.")
	} else {
		sb.WriteString(fmt.Sprintf("\nShowing %d of %d concepts", count, len(bundle.Concepts)))
	}

	return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
}

func (r *ToolRegistry) handleGetConcept(args map[string]interface{}) (*ToolCallResult, error) {
	bundle, _ := r.GetBundle()
	if bundle == nil {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		return errorResult("path is required"), nil
	}

	for _, c := range bundle.Concepts {
		if c.FilePath == path {
			pc := conceptToParser(c)
			data, err := parser.SerializeConcept(pc, true)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to serialize concept: %v", err)), nil
			}
			return &ToolCallResult{Content: []ContentItem{TextContent(string(data))}}, nil
		}
	}

	return errorResult(fmt.Sprintf("Concept not found: %s", path)), nil
}

func (r *ToolRegistry) handleSearch(args map[string]interface{}) (*ToolCallResult, error) {
	bundle, _ := r.GetBundle()
	if bundle == nil {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}

	query, _ := args["query"].(string)
	typeFilter, _ := args["type"].(string)
	tagFilter, _ := args["tag"].(string)
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	queryLower := strings.ToLower(query)
	var sb strings.Builder
	count := 0

	for _, c := range bundle.Concepts {
		if typeFilter != "" && c.Type != typeFilter {
			continue
		}
		if tagFilter != "" && !containsTag(c.Tags, tagFilter) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(c.Title + " " + c.Description + " " + c.Content + " " + strings.Join(c.Tags, " "))
			if !strings.Contains(haystack, queryLower) {
				continue
			}
		}
		if count >= limit {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", count+1, c.Type, c.FilePath))
		if c.Title != "" {
			sb.WriteString(fmt.Sprintf("   Title: %s\n", c.Title))
		}
		if c.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", c.Description))
		}
		count++
	}

	if count == 0 {
		sb.WriteString("No results found.")
	} else {
		sb.WriteString(fmt.Sprintf("\nFound %d results", count))
	}

	return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
}

func (r *ToolRegistry) handleLintBundle(args map[string]interface{}) (*ToolCallResult, error) {
	bundle, _ := r.GetBundle()
	if bundle == nil {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}

	strict, _ := args["strict"].(bool)
	cfg := lint.DefaultConfig()
	if strict {
		cfg.StrictMode = true
	}

	lintConcepts := toLintConcepts(bundle.Concepts)
	result := lint.LintBundle(lintConcepts, cfg)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Linted %d concepts\n", result.ConceptsChecked))
	sb.WriteString(fmt.Sprintf("Errors: %d\n", result.Errors))
	sb.WriteString(fmt.Sprintf("Warnings: %d\n", result.Warnings))
	sb.WriteString(fmt.Sprintf("Infos: %d\n\n", result.Infos))

	for _, issue := range result.Issues {
		icon := "ℹ"
		switch issue.Severity {
		case lint.Error:
			icon = "❌"
		case lint.Warning:
			icon = "⚠"
		}
		loc := issue.FilePath
		if issue.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, issue.Line)
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s - %s\n", icon, issue.Code, loc, issue.Message))
	}

	if result.HasErrors() {
		sb.WriteString("\n❌ Lint failed with errors.")
	} else if result.Warnings > 0 && strict {
		sb.WriteString("\n❌ Lint failed (strict mode: warnings are errors).")
	} else {
		sb.WriteString("\n✅ Lint passed.")
	}

	return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
}

func (r *ToolRegistry) handleLintConcept(args map[string]interface{}) (*ToolCallResult, error) {
	bundle, _ := r.GetBundle()
	if bundle == nil {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}

	path, _ := args["path"].(string)
	if path == "" {
		return errorResult("path is required"), nil
	}

	for _, c := range bundle.Concepts {
		if c.FilePath == path {
			lintConcepts := toLintConcepts([]*okf.Concept{c})
			result := lint.LintBundle(lintConcepts, lint.DefaultConfig())

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Linting: %s\n\n", path))
			sb.WriteString(fmt.Sprintf("Errors: %d\n", result.Errors))
			sb.WriteString(fmt.Sprintf("Warnings: %d\n", result.Warnings))
			sb.WriteString(fmt.Sprintf("Infos: %d\n\n", result.Infos))

			for _, issue := range result.Issues {
				icon := "ℹ"
				switch issue.Severity {
				case lint.Error:
					icon = "❌"
				case lint.Warning:
					icon = "⚠"
				}
				sb.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, issue.Code, issue.Message))
			}

			if result.HasErrors() {
				sb.WriteString("\n❌ Has errors.")
			} else {
				sb.WriteString("\n✅ Passed.")
			}

			return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
		}
	}

	return errorResult(fmt.Sprintf("Concept not found: %s", path)), nil
}

// --- Helpers ---

func errorResult(msg string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentItem{TextContent(msg)},
		IsError: true,
	}
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func toLintConcepts(concepts []*okf.Concept) []*lint.Concept {
	result := make([]*lint.Concept, len(concepts))
	for i, c := range concepts {
		result[i] = &lint.Concept{
			Type:        c.Type,
			Title:       c.Title,
			Description: c.Description,
			Resource:    c.Resource,
			Tags:        c.Tags,
			Timestamp:   c.Timestamp,
			Content:     c.Content,
			FilePath:    c.FilePath,
			HasSources:  len(c.Sources) > 0,
			HasVerified: len(c.Verified) > 0,
			Status:      string(c.Status),
			StaleAfter:  c.StaleAfter,
			Runtime:     c.Runtime,
		}
		if c.Generated != nil {
			result[i].GeneratedBy = c.Generated.By
			result[i].GeneratedAt = c.Generated.At
		}
	}
	return result
}
