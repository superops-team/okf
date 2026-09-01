package mcp

import (
	"encoding/json"
	"fmt"
)

// JSONRPCVersion is the JSON-RPC version used by MCP.
const JSONRPCVersion = "2.0"

// Request represents a JSON-RPC request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Notification represents a JSON-RPC notification (no id).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// Standard JSON-RPC error codes.
const (
	ParseErrorCode     = -32700
	InvalidRequestCode = -32600
	MethodNotFoundCode = -32601
	InvalidParamsCode  = -32602
	InternalErrorCode  = -32603
)

// InitializeRequestParams is the params for the initialize request.
type InitializeRequestParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ImplementationInfo     `json:"clientInfo"`
}

// ImplementationInfo describes a client or server implementation.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the result of the initialize request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ImplementationInfo `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ServerCapabilities describes the server's capabilities.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
}

// ToolsCapability describes the tools capability.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability describes the resources capability.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability describes the prompts capability.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Annotations *ToolAnnotations       `json:"annotations,omitempty"`
}

// ToolAnnotations declares behavioral properties without relying on tool names.
type ToolAnnotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
	IdempotentHint  bool `json:"idempotentHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
}

// ToolsListResult is the result of tools/list.
type ToolsListResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// ToolCallParams is the params for tools/call.
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolCallResult is the result of tools/call.
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem represents a content item in a tool result.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TextContent creates a text content item.
func TextContent(text string) ContentItem {
	return ContentItem{Type: "text", Text: text}
}

// Resource represents an MCP resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// ResourcesListResult is the result of resources/list.
type ResourcesListResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ResourceReadParams is the params for resources/read.
type ResourceReadParams struct {
	URI string `json:"uri"`
}

// ResourceReadResult is the result of resources/read.
type ResourceReadResult struct {
	Contents []ResourceContents `json:"contents"`
}

// ResourceContents represents the contents of a resource.
type ResourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// Prompt represents an MCP prompt.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument represents an argument for a prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptsListResult is the result of prompts/list.
type PromptsListResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// PromptGetParams is the params for prompts/get.
type PromptGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// PromptGetResult is the result of prompts/get.
type PromptGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage represents a message in a prompt result.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content ContentItem `json:"content"`
}

// ParseMessage parses a JSON-RPC message from raw bytes.
func ParseMessage(data []byte) (method string, id json.RawMessage, params json.RawMessage, isNotification bool, err error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return "", nil, nil, false, fmt.Errorf("parse error: %w", err)
	}
	if req.JSONRPC != JSONRPCVersion {
		return "", nil, nil, false, fmt.Errorf("invalid jsonrpc version: %s", req.JSONRPC)
	}
	isNotification = len(req.ID) == 0
	return req.Method, req.ID, req.Params, isNotification, nil
}

// NewResponse creates a success response.
func NewResponse(id json.RawMessage, result interface{}) (*Response, error) {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  resultBytes,
	}, nil
}

// NewErrorResponse creates an error response.
func NewErrorResponse(id json.RawMessage, code int, message string) *Response {
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
}

// NewNotification creates a notification.
func NewNotification(method string, params interface{}) (*Notification, error) {
	var paramsBytes json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsBytes = b
	}
	return &Notification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsBytes,
	}, nil
}
