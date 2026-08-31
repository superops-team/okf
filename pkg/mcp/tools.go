package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/superops-team/okf/pkg/convert"
	"github.com/superops-team/okf/pkg/embeddings"
	"github.com/superops-team/okf/pkg/lint"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/parser"
	"github.com/superops-team/okf/pkg/query"
	toolsvc "github.com/superops-team/okf/pkg/tool"
	"github.com/superops-team/okf/pkg/vectorindex"
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
	service    *toolsvc.Service
}

// NewToolRegistry creates a registry with the legacy bundle-facing tools.
func NewToolRegistry() *ToolRegistry {
	return newToolRegistry(nil)
}

// NewToolRegistryWithService creates a registry that also exposes the
// repository-scoped, service-backed agent tools.
func NewToolRegistryWithService(service *toolsvc.Service) *ToolRegistry {
	return newToolRegistry(service)
}

func newToolRegistry(service *toolsvc.Service) *ToolRegistry {
	r := &ToolRegistry{
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
		service:  service,
	}
	r.registerCoreTools()
	if service != nil {
		r.registerAgentTools()
	}
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
		Name:        "okf_semantic_search",
		Description: "Semantic (natural-language) search over concepts; requires okf vector index built for the bundle",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Natural-language query (semantic, not literal)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum results (default: 10)",
				},
			},
			"required": []string{"query"},
		},
	}, r.handleSemanticSearch)

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
	r.Register(Tool{
		Name:        "okf_import_document",
		Description: "Import a document (PDF/DOCX/XLSX/PPTX/HTML/CSV/TXT) into the loaded bundle by converting it to Markdown",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Absolute path to the document to import",
				},
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Optional title override (defaults to the document extracted title or filename)",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Optional concept type (default: source)",
				},
			},
			"required": []string{"path"},
		},
	}, r.handleImportDocument)
}

func readOnlyAgentTool(name, description string, inputSchema map[string]interface{}) Tool {
	return Tool{
		Name: name, Description: description, InputSchema: inputSchema,
		Annotations: &ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func mutatingAgentTool(name, description string, inputSchema map[string]interface{}) Tool {
	return Tool{
		Name: name, Description: description, InputSchema: inputSchema,
		Annotations: &ToolAnnotations{IdempotentHint: true},
	}
}

func (r *ToolRegistry) registerAgentTools() {
	objectSchema := func(properties map[string]interface{}, required ...string) map[string]interface{} {
		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	stringProperty := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": description}
	}
	integerProperty := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "integer", "description": description}
	}
	booleanProperty := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "boolean", "description": description}
	}
	arrayProperty := func(description string) map[string]interface{} {
		return map[string]interface{}{
			"type":        "array",
			"description": description,
			"items":       map[string]interface{}{"type": "string"},
		}
	}
	objectProperty := func(description string) map[string]interface{} {
		return map[string]interface{}{"type": "object", "description": description}
	}

	r.Register(readOnlyAgentTool(
		"okf_status",
		"Report repository knowledge readiness without modifying files",
		objectSchema(map[string]interface{}{}),
	), func(_ map[string]interface{}) (*ToolCallResult, error) {
		return serviceEnvelopeResult(r.service.Status(context.Background(), toolsvc.StatusRequest{}))
	})
	r.Register(mutatingAgentTool(
		"okf_init",
		"Initialize repository knowledge for the configured repository",
		objectSchema(map[string]interface{}{}),
	), func(_ map[string]interface{}) (*ToolCallResult, error) {
		return serviceEnvelopeResult(r.service.Init(context.Background(), toolsvc.InitRequest{}))
	})
	r.Register(mutatingAgentTool(
		"okf_refresh",
		"Refresh repository knowledge using incremental, full, or cache-only mode",
		objectSchema(map[string]interface{}{
			"mode": stringProperty("Refresh mode: incremental, full, or cache-only"),
		}),
	), func(args map[string]interface{}) (*ToolCallResult, error) {
		mode, _ := args["mode"].(string)
		return serviceEnvelopeResult(r.service.Refresh(context.Background(), toolsvc.RefreshRequest{Mode: mode}))
	})
	queryProperties := map[string]interface{}{
		"query":           stringProperty("Non-empty repository knowledge query"),
		"limit":           integerProperty("Maximum results"),
		"type":            stringProperty("Concept type filter"),
		"tag":             stringProperty("Tag filter"),
		"file_path":       stringProperty("Source file path filter"),
		"language":        stringProperty("Language filter"),
		"symbol_kind":     stringProperty("Symbol kind filter"),
		"qualified_name":  stringProperty("Qualified symbol name filter"),
		"relation_kind":   stringProperty("Relation kind filter"),
		"relation_source": stringProperty("Relation source filter"),
		"relation_target": stringProperty("Relation target filter"),
		"include_trace":   booleanProperty("Include deterministic query trace"),
	}
	r.Register(readOnlyAgentTool(
		"okf_query",
		"Query repository knowledge through the shared service",
		objectSchema(queryProperties, "query"),
	), func(args map[string]interface{}) (*ToolCallResult, error) {
		return serviceEnvelopeResult(r.service.Query(context.Background(), queryRequestFromArgs(args)))
	})
	r.Register(readOnlyAgentTool(
		"okf_context",
		"Build a token-bounded repository context through the shared service",
		objectSchema(map[string]interface{}{
			"query":             stringProperty("Non-empty repository knowledge query"),
			"budget_tokens":     integerProperty("Maximum context token budget"),
			"include_relations": booleanProperty("Include related concepts"),
			"include_trace":     booleanProperty("Include deterministic context trace"),
		}, "query"),
	), func(args map[string]interface{}) (*ToolCallResult, error) {
		query, _ := args["query"].(string)
		return serviceEnvelopeResult(r.service.Context(context.Background(), toolsvc.ContextRequest{
			Query:            query,
			BudgetTokens:     intArg(args, "budget_tokens"),
			IncludeRelations: boolArg(args, "include_relations"),
			IncludeTrace:     boolArg(args, "include_trace"),
		}))
	})
	writeProperties := map[string]interface{}{
		"content":         stringProperty("Knowledge content to persist"),
		"project":         stringProperty("Optional project isolation key"),
		"tags":            arrayProperty("Optional normalized tags"),
		"metadata":        objectProperty("Optional small JSON metadata"),
		"idempotency_key": stringProperty("Required stable idempotency key"),
	}
	for _, definition := range []struct {
		name        string
		description string
		kind        string
	}{
		{name: "okf_note", description: "Persist an explicit durable note", kind: "note"},
		{name: "okf_log", description: "Persist an explicit durable event", kind: "event"},
	} {
		kind := definition.kind
		r.Register(mutatingAgentTool(
			definition.name,
			definition.description,
			objectSchema(writeProperties, "content", "idempotency_key"),
		), func(args map[string]interface{}) (*ToolCallResult, error) {
			if invalid := validateWriteArgs(
				args,
				[]string{"content", "idempotency_key"},
				[]string{"project"},
				[]string{"tags"},
				[]string{"metadata"},
				"content", "project", "tags", "metadata", "idempotency_key",
			); invalid != nil {
				return serviceEnvelopeResult(*invalid)
			}
			return serviceEnvelopeResult(r.service.WriteKnowledge(context.Background(), writeRequestFromArgs(kind, args)))
		})
	}
	r.Register(mutatingAgentTool(
		"okf_feedback",
		"Persist an explicit reusable feedback principle and evidence",
		objectSchema(map[string]interface{}{
			"principle":       stringProperty("Reusable principle to persist"),
			"category":        stringProperty("Feedback category"),
			"project":         stringProperty("Optional project isolation key"),
			"tags":            arrayProperty("Optional normalized tags"),
			"metadata":        objectProperty("Optional small JSON metadata"),
			"evidence_refs":   arrayProperty("Evidence references supporting the principle"),
			"idempotency_key": stringProperty("Required stable idempotency key"),
		}, "principle", "category", "idempotency_key"),
	), func(args map[string]interface{}) (*ToolCallResult, error) {
		if invalid := validateWriteArgs(
			args,
			[]string{"principle", "category", "idempotency_key"},
			[]string{"project"},
			[]string{"tags", "evidence_refs"},
			[]string{"metadata"},
			"principle", "category", "project", "tags", "metadata", "evidence_refs", "idempotency_key",
		); invalid != nil {
			return serviceEnvelopeResult(*invalid)
		}
		principle := args["principle"].(string)
		metadata := mapArg(args, "metadata")
		metadata["principle"] = principle
		category := args["category"].(string)
		metadata["category"] = category
		request := writeRequestFromArgs("feedback", args)
		request.Content = principle
		request.Metadata = metadata
		request.EvidenceRefs = stringSliceArg(args, "evidence_refs")
		return serviceEnvelopeResult(r.service.WriteKnowledge(context.Background(), request))
	})
	r.Register(readOnlyAgentTool(
		"okf_ask",
		"Query note, event, and feedback concepts through the shared service",
		objectSchema(map[string]interface{}{
			"query":         stringProperty("Non-empty note, event, or feedback query"),
			"project":       stringProperty("Optional project isolation key"),
			"limit":         integerProperty("Maximum results"),
			"include_trace": booleanProperty("Include deterministic query trace"),
		}, "query"),
	), func(args map[string]interface{}) (*ToolCallResult, error) {
		request := queryRequestFromArgs(args)
		request.Types = []string{"note", "event", "feedback"}
		return serviceEnvelopeResult(r.service.Query(context.Background(), request))
	})
}

func queryRequestFromArgs(args map[string]interface{}) toolsvc.QueryRequest {
	query, _ := args["query"].(string)
	typeFilter, _ := args["type"].(string)
	project, _ := args["project"].(string)
	tag, _ := args["tag"].(string)
	filePath, _ := args["file_path"].(string)
	language, _ := args["language"].(string)
	symbolKind, _ := args["symbol_kind"].(string)
	qualifiedName, _ := args["qualified_name"].(string)
	relationKind, _ := args["relation_kind"].(string)
	relationSource, _ := args["relation_source"].(string)
	relationTarget, _ := args["relation_target"].(string)
	return toolsvc.QueryRequest{
		Query:          query,
		Limit:          intArg(args, "limit"),
		Type:           typeFilter,
		Project:        project,
		Tag:            tag,
		FilePath:       filePath,
		Language:       language,
		SymbolKind:     symbolKind,
		QualifiedName:  qualifiedName,
		RelationKind:   relationKind,
		RelationSource: relationSource,
		RelationTarget: relationTarget,
		IncludeTrace:   boolArg(args, "include_trace"),
	}
}

func validateWriteArgs(
	args map[string]interface{},
	requiredStrings []string,
	optionalStrings []string,
	stringSlices []string,
	objects []string,
	allowed ...string,
) *toolsvc.ToolEnvelope {
	if invalid := rejectUnknownArgs(args, allowed...); invalid != nil {
		return invalid
	}
	for _, key := range requiredStrings {
		value, ok := args[key]
		if !ok {
			return invalidWriteArgs("request is missing required field " + key)
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return invalidWriteArgs(key + " must be a non-empty string")
		}
	}
	for _, key := range optionalStrings {
		if value, ok := args[key]; ok {
			if _, ok := value.(string); !ok {
				return invalidWriteArgs(key + " must be a string")
			}
		}
	}
	for _, key := range stringSlices {
		value, ok := args[key]
		if !ok {
			continue
		}
		switch values := value.(type) {
		case []string:
		case []interface{}:
			for _, item := range values {
				if _, ok := item.(string); !ok {
					return invalidWriteArgs(key + " must contain only strings")
				}
			}
		default:
			return invalidWriteArgs(key + " must be an array of strings")
		}
	}
	for _, key := range objects {
		if value, ok := args[key]; ok {
			if _, ok := value.(map[string]interface{}); !ok {
				return invalidWriteArgs(key + " must be an object")
			}
		}
	}
	return nil
}

func invalidWriteArgs(message string) *toolsvc.ToolEnvelope {
	return &toolsvc.ToolEnvelope{
		SchemaVersion: toolsvc.SchemaVersion,
		Operation:     toolsvc.OperationWrite,
		OK:            false,
		Mutating:      true,
		Warnings:      []string{},
		Error: &toolsvc.ToolError{
			Code:        toolsvc.ErrInvalidRequest,
			Message:     message,
			Remediation: "Correct the request to match the declared tool schema.",
		},
	}
}

func writeRequestFromArgs(kind string, args map[string]interface{}) toolsvc.WriteKnowledgeRequest {
	content, _ := args["content"].(string)
	project, _ := args["project"].(string)
	idempotencyKey, _ := args["idempotency_key"].(string)
	return toolsvc.WriteKnowledgeRequest{
		Kind:           kind,
		Content:        content,
		Project:        project,
		Tags:           stringSliceArg(args, "tags"),
		Metadata:       mapArg(args, "metadata"),
		IdempotencyKey: idempotencyKey,
	}
}

func stringSliceArg(args map[string]interface{}, key string) []string {
	switch values := args[key].(type) {
	case []string:
		return append([]string(nil), values...)
	case []interface{}:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil
			}
			result = append(result, text)
		}
		return result
	default:
		return nil
	}
}

func mapArg(args map[string]interface{}, key string) map[string]interface{} {
	value, ok := args[key].(map[string]interface{})
	if !ok || value == nil {
		return map[string]interface{}{}
	}
	copy := make(map[string]interface{}, len(value))
	for field, nested := range value {
		copy[field] = nested
	}
	return copy
}

func rejectUnknownArgs(args map[string]interface{}, allowed ...string) *toolsvc.ToolEnvelope {
	allow := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allow[key] = struct{}{}
	}
	for key := range args {
		if _, ok := allow[key]; !ok {
			return &toolsvc.ToolEnvelope{
				SchemaVersion: toolsvc.SchemaVersion,
				Operation:     toolsvc.OperationWrite,
				OK:            false,
				Mutating:      true,
				Warnings:      []string{},
				Error: &toolsvc.ToolError{
					Code:        toolsvc.ErrInvalidRequest,
					Message:     "request contains an unknown field",
					Remediation: "Remove fields that are not declared by the tool schema.",
				},
			}
		}
	}
	return nil
}

func intArg(args map[string]interface{}, key string) int {
	switch value := args[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func boolArg(args map[string]interface{}, key string) bool {
	value, _ := args[key].(bool)
	return value
}

func serviceEnvelopeResult(envelope toolsvc.ToolEnvelope) (*ToolCallResult, error) {
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal tool service envelope: %w", err)
	}
	return &ToolCallResult{Content: []ContentItem{TextContent(string(data))}}, nil
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

func (r *ToolRegistry) handleSemanticSearch(args map[string]interface{}) (*ToolCallResult, error) {
	bundle, bundlePath := r.GetBundle()
	if bundle == nil {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}
	q, _ := args["query"].(string)
	if strings.TrimSpace(q) == "" {
		return errorResult("query is required"), nil
	}
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	emb, err := embeddings.NewMiniLM()
	if err != nil {
		return errorResult(fmt.Sprintf("向量模型初始化失败: %v", err)), nil
	}
	defer emb.Close()

	idx := vectorindex.NewHNSW(emb.Dimension())
	idxDir := filepath.Join(bundlePath, ".okf", "vector")
	if _, err := idx.Load(idxDir); err != nil {
		return errorResult(fmt.Sprintf("向量索引不可用，请先执行 okf vector index: %v", err)), nil
	}

	backend := &mcpSemanticBackend{emb: emb, idx: idx}
	results, err := query.SemanticSearch(mcpToQueryBundle(bundle), q, backend, query.SearchOptions{TopK: limit})
	if err != nil {
		return errorResult(fmt.Sprintf("语义检索失败: %v", err)), nil
	}

	var sb strings.Builder
	if len(results) == 0 {
		sb.WriteString("No results found.")
		return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
	}
	for i, res := range results {
		c := res.Concept
		sb.WriteString(fmt.Sprintf("%d. [%s] %s (source=%s, score=%.4f)\n", i+1, c.Type, c.FilePath, res.Source, res.SemanticScore))
		if c.Title != "" {
			sb.WriteString(fmt.Sprintf("   Title: %s\n", c.Title))
		}
		if c.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", c.Description))
		}
	}
	sb.WriteString(fmt.Sprintf("\nFound %d results", len(results)))
	return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
}

// mcpSemanticBackend 适配 query.SemanticBackend：MiniLM 编码 + HNSW 近邻检索。
type mcpSemanticBackend struct {
	emb *embeddings.MiniLM
	idx *vectorindex.HNSW
}

func (b *mcpSemanticBackend) EmbedQuery(text string) ([]float32, error) {
	return b.emb.EmbedQuery(text)
}

func (b *mcpSemanticBackend) Search(vec []float32, k int) []query.SemanticHit {
	matches := b.idx.Search(vec, k)
	out := make([]query.SemanticHit, len(matches))
	for i, m := range matches {
		out[i] = query.SemanticHit{Key: m.Key, Score: m.Score}
	}
	return out
}

// mcpToQueryBundle 将 okf bundle 转换为 query bundle（与 CLI toQueryBundle 逻辑一致）。
func mcpToQueryBundle(bundle *okf.KnowledgeBundle) *query.KnowledgeBundle {
	concepts := make([]*query.Concept, 0, len(bundle.Concepts))
	for _, c := range bundle.Concepts {
		concepts = append(concepts, &query.Concept{
			Type:        c.Type,
			Title:       c.Title,
			Description: c.Description,
			Resource:    c.Resource,
			Tags:        c.Tags,
			Content:     c.Content,
			FilePath:    c.FilePath,
		})
	}
	return &query.KnowledgeBundle{Concepts: concepts}
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

// handleImportDocument converts a document and writes it into the loaded
// bundle root as <original>.md, then refreshes the bundle.
func (r *ToolRegistry) handleImportDocument(args map[string]interface{}) (*ToolCallResult, error) {
	_, bundlePath := r.GetBundle()
	if bundlePath == "" {
		return errorResult("No bundle loaded. Call okf_load_bundle first."), nil
	}
	path, _ := args["path"].(string)
	if path == "" {
		return errorResult("path is required"), nil
	}
	titleOverride, _ := args["title"].(string)
	typeOverride, _ := args["type"].(string)

	res, err := convert.ConvertToMarkdown(context.Background(), path, nil)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to convert document: %v", err)), nil
	}
	title := res.Title
	if titleOverride != "" {
		title = titleOverride
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	ctype := "source"
	if typeOverride != "" {
		ctype = typeOverride
	}
	body := convert.WrapConcept(title, filepath.Base(path), convert.DocumentType(path), ctype, res.Markdown)
	out := filepath.Join(bundlePath, filepath.Base(path)+".md")
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		return errorResult(fmt.Sprintf("Failed to write concept: %v", err)), nil
	}
	// Refresh the bundle so subsequent tools see the new concept.
	if bundle, lerr := okf.LoadBundle(bundlePath, &okf.LoadOptions{Recursive: true}); lerr == nil {
		r.SetBundle(bundle, bundlePath)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Imported document -> %s\n", out))
	sb.WriteString(fmt.Sprintf("Title: %s\n", title))
	sb.WriteString(fmt.Sprintf("Type: %s\n", ctype))
	if len(res.Warnings) > 0 {
		sb.WriteString(fmt.Sprintf("Warnings: %d\n", len(res.Warnings)))
		for _, w := range res.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	} else {
		sb.WriteString("Warnings: 0\n")
	}
	return &ToolCallResult{Content: []ContentItem{TextContent(sb.String())}}, nil
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
