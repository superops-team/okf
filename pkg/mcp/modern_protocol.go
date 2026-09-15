package mcp

import (
	"encoding/json"
	"fmt"
)

// Modern protocol constants.
const (
	ModernProtocolVersion               = "2026-07-28"
	LegacyProtocolVersion               = "2024-11-05"
	SkillsExtensionID                   = "io.modelcontextprotocol/skills"
	UnsupportedProtocolVersionCode      = -32022
	MissingRequiredClientCapabilityCode = -32021
)

// ModernRequestMeta is the per-request _meta object required by modern 2026-07-28.
type ModernRequestMeta struct {
	ProtocolVersion    string              `json:"io.modelcontextprotocol/protocolVersion"`
	ClientCapabilities map[string]any      `json:"io.modelcontextprotocol/clientCapabilities"`
	ClientInfo         *ImplementationInfo `json:"io.modelcontextprotocol/clientInfo,omitempty"`
}

// ParseModernMeta validates and parses a modern _meta object.
// Returns -32602 for missing/wrong-type required fields, -32022 for unsupported version.
func ParseModernMeta(raw json.RawMessage) (*ModernRequestMeta, error) {
	if len(raw) == 0 {
		return nil, &RPCError{Code: InvalidParamsCode, Message: "missing modern request _meta"}
	}
	var meta ModernRequestMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, &RPCError{Code: InvalidParamsCode, Message: fmt.Sprintf("invalid _meta JSON: %v", err)}
	}
	if meta.ProtocolVersion == "" {
		return nil, &RPCError{Code: InvalidParamsCode, Message: "missing io.modelcontextprotocol/protocolVersion"}
	}
	if meta.ClientCapabilities == nil {
		return nil, &RPCError{Code: InvalidParamsCode, Message: "missing io.modelcontextprotocol/clientCapabilities"}
	}
	if meta.ProtocolVersion != ModernProtocolVersion {
		return nil, &RPCError{
			Code:    UnsupportedProtocolVersionCode,
			Message: "unsupported protocol version",
			Data: map[string]any{
				"supported": []string{ModernProtocolVersion},
				"requested": meta.ProtocolVersion,
			},
		}
	}
	return &meta, nil
}

// HasSkillsCapability reports whether the client declared the Skills extension.
func (m *ModernRequestMeta) HasSkillsCapability() bool {
	if m == nil || m.ClientCapabilities == nil {
		return false
	}
	ext, ok := m.ClientCapabilities["extensions"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = ext[SkillsExtensionID]
	return ok
}

// NewMissingCapabilityError creates a -32021 error for a missing client capability.
func NewMissingCapabilityError(capability string) *RPCError {
	return &RPCError{
		Code:    MissingRequiredClientCapabilityCode,
		Message: fmt.Sprintf("missing required client capability: %s", capability),
		Data: map[string]any{
			"requiredCapability": capability,
		},
	}
}

// ModernResultMeta is the _meta object attached to every modern success result.
type ModernResultMeta struct {
	ServerInfo ImplementationInfo `json:"io.modelcontextprotocol/serverInfo"`
}

// ModernResult is the envelope for all modern success results.
type ModernResult struct {
	ResultType string           `json:"resultType"`
	Meta       ModernResultMeta `json:"_meta"`
	TTLMs      int              `json:"ttlMs,omitempty"`
	CacheScope string           `json:"cacheScope,omitempty"`
}

// NewModernResult builds a modern result envelope. cacheable adds ttlMs/cacheScope.
func NewModernResult(serverInfo ImplementationInfo, cacheable bool) ModernResult {
	r := ModernResult{
		ResultType: "complete",
		Meta:       ModernResultMeta{ServerInfo: serverInfo},
	}
	if cacheable {
		r.TTLMs = 300000
		r.CacheScope = "private"
	}
	return r
}

// modernResultWithData embeds ModernResult plus arbitrary data fields.
type modernResultWithData struct {
	ModernResult
	Data map[string]any `json:"-"`
}

// MarshalJSON for modernResultWithData flattens data fields into the result.
func (r modernResultWithData) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"resultType": r.ResultType,
		"_meta":      r.Meta,
	}
	if r.TTLMs > 0 {
		m["ttlMs"] = r.TTLMs
	}
	if r.CacheScope != "" {
		m["cacheScope"] = r.CacheScope
	}
	for k, v := range r.Data {
		m[k] = v
	}
	return json.Marshal(m)
}
