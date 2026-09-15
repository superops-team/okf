package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// modernToolNames is the sorted list of 11 service-backed Agent tools exposed in modern era.
var modernToolNames = []string{
	"okf_ask",
	"okf_context",
	"okf_feedback",
	"okf_init",
	"okf_log",
	"okf_manifest",
	"okf_note",
	"okf_query",
	"okf_refresh",
	"okf_resolve",
	"okf_status",
}

// modernToolSet is a set for fast lookup.
var modernToolSet = func() map[string]bool {
	m := make(map[string]bool, len(modernToolNames))
	for _, n := range modernToolNames {
		m[n] = true
	}
	return m
}()

// handleModernDiscover handles server/discover for the modern era.
func (s *Server) handleModernDiscover(id json.RawMessage, meta *ModernRequestMeta) {
	caps := map[string]any{
		"tools":     map[string]any{"listChanged": false},
		"resources": map[string]any{"listChanged": false},
		"extensions": map[string]any{
			SkillsExtensionID: map[string]any{},
		},
	}
	result := modernResultWithData{
		ModernResult: NewModernResult(s.serverInfo(), true),
		Data: map[string]any{
			"supportedVersions": []string{ModernProtocolVersion},
			"capabilities":      caps,
			"instructions":      "OKF MCP Server. Use skill://okf/SKILL.md for the canonical Agent workflow. Modern era exposes 11 service-backed tools and the static Skill resource.",
		},
	}
	s.sendResponse(id, result)
}

// handleModernToolsList returns only the 11 modern service-backed tools.
func (s *Server) handleModernToolsList(id json.RawMessage, meta *ModernRequestMeta) {
	all := s.tools.List()
	var modern []Tool
	for _, t := range all {
		if modernToolSet[t.Name] {
			modern = append(modern, t)
		}
	}
	sort.Slice(modern, func(i, j int) bool { return modern[i].Name < modern[j].Name })
	result := modernResultWithData{
		ModernResult: NewModernResult(s.serverInfo(), true),
		Data: map[string]any{
			"tools": modern,
		},
	}
	s.sendResponse(id, result)
}

// handleModernToolsCall routes only modern tools; legacy-only tools return method error.
func (s *Server) handleModernToolsCall(id json.RawMessage, meta *ModernRequestMeta, params json.RawMessage) {
	var callParams ToolCallParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Invalid params: %v", err))
		return
	}
	if !modernToolSet[callParams.Name] {
		s.sendError(id, MethodNotFoundCode, fmt.Sprintf("Tool %s is not available in modern era (legacy bundle-state tool)", callParams.Name))
		return
	}
	result, err := s.tools.Call(callParams.Name, callParams.Arguments)
	if err != nil {
		s.sendResponse(id, &ToolCallResult{
			Content: []ContentItem{TextContent(fmt.Sprintf("Error: %v", err))},
			IsError: true,
		})
		return
	}
	// tools/call is non-cacheable: resultType + _meta only, no TTL/cacheScope
	envelope := modernResultWithData{
		ModernResult: NewModernResult(s.serverInfo(), false),
		Data: map[string]any{
			"content": result.Content,
			"isError": result.IsError,
		},
	}
	s.sendResponse(id, envelope)
}

// skillResources returns the static Skill resources for modern resources/list.
func (s *Server) skillResources() []Resource {
	if s.skills == nil {
		return nil
	}
	return s.skills.Resources()
}

// readSkillResource reads a Skill resource by URI.
func (s *Server) readSkillResource(uri string) (ResourceContents, error) {
	if s.skills == nil {
		return ResourceContents{}, fmt.Errorf("skill registry not initialized")
	}
	return s.skills.Read(uri)
}

// skillList returns the Skill list for skills/list.
func (s *Server) skillList() []Skill {
	if s.skills == nil {
		return nil
	}
	return s.skills.List()
}

// skillGet returns a single Skill by URI.
func (s *Server) skillGet(uri string) (Skill, error) {
	if s.skills == nil {
		return Skill{}, fmt.Errorf("skill registry not initialized")
	}
	return s.skills.Get(uri)
}
func (s *Server) handleModernNotImplemented(id json.RawMessage, method string) {
	s.sendError(id, MethodNotFoundCode, fmt.Sprintf("Method %s is not implemented in modern era", method))
}

// handleModernResourcesList returns only the static Skill resource.
// Full implementation in P3 (T3.2); currently returns the Skill resource when registry is ready.
func (s *Server) handleModernResourcesList(id json.RawMessage, meta *ModernRequestMeta) {
	resources := s.skillResources()
	result := modernResultWithData{
		ModernResult: NewModernResult(s.serverInfo(), true),
		Data: map[string]any{
			"resources": resources,
		},
	}
	s.sendResponse(id, result)
}

// handleModernResourcesRead reads the Skill resource or rejects unknown skill: URIs.
func (s *Server) handleModernResourcesRead(id json.RawMessage, meta *ModernRequestMeta, params json.RawMessage) {
	var readParams ResourceReadParams
	if err := json.Unmarshal(params, &readParams); err != nil {
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Invalid params: %v", err))
		return
	}
	if !strings.HasPrefix(readParams.URI, "skill:") {
		s.sendError(id, InvalidParamsCode, "Unsupported resource URI in modern era")
		return
	}
	content, err := s.readSkillResource(readParams.URI)
	if err != nil {
		s.sendError(id, InvalidParamsCode, err.Error())
		return
	}
	result := modernResultWithData{
		ModernResult: NewModernResult(s.serverInfo(), true),
		Data: map[string]any{
			"contents": []ResourceContents{content},
		},
	}
	s.sendResponse(id, result)
}

// handleModernSkillsList returns the single canonical Skill.
func (s *Server) handleModernSkillsList(id json.RawMessage, meta *ModernRequestMeta, params json.RawMessage) {
	if len(params) > 0 {
		var p struct {
			Cursor string `json:"cursor,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err == nil && p.Cursor != "" {
			s.sendError(id, InvalidParamsCode, "unknown cursor: skills/list has no pagination")
			return
		}
	}
	if !meta.HasSkillsCapability() {
		rpcErr := NewMissingCapabilityError(SkillsExtensionID)
		s.sendError(id, rpcErr.Code, rpcErr.Message)
		return
	}
	skills := s.skillList()
	result := modernResultWithData{
		ModernResult: NewModernResult(s.serverInfo(), true),
		Data: map[string]any{
			"skills": skills,
		},
	}
	s.sendResponse(id, result)
}

// handleModernSkillsGet returns a single Skill by URI.
func (s *Server) handleModernSkillsGet(id json.RawMessage, meta *ModernRequestMeta, params json.RawMessage) {
	if !meta.HasSkillsCapability() {
		rpcErr := NewMissingCapabilityError(SkillsExtensionID)
		s.sendError(id, rpcErr.Code, rpcErr.Message)
		return
	}
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.URI == "" {
		s.sendError(id, InvalidParamsCode, "skills/get requires a string uri")
		return
	}
	skill, err := s.skillGet(p.URI)
	if err != nil {
		s.sendError(id, InvalidParamsCode, err.Error())
		return
	}
	result := modernResultWithData{
		ModernResult: NewModernResult(s.serverInfo(), true),
		Data: map[string]any{
			"skill": skill,
		},
	}
	s.sendResponse(id, result)
}
