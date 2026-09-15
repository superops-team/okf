package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/okf/meta"
	"github.com/superops-team/okf/pkg/parser"
	toolsvc "github.com/superops-team/okf/pkg/tool"
)

type stdioFraming uint8

const (
	framingContentLength stdioFraming = iota
	framingNewline

	maxStdioMessageBytes = 16 << 20
)

type processEra uint8

const (
	eraUnselected processEra = iota
	eraLegacy
	eraModern
)

// Server is an MCP server that communicates over stdio.
type Server struct {
	tools        *ToolRegistry
	reader       *bufio.Reader
	writer       io.Writer
	logger       *log.Logger
	framing      stdioFraming
	era          processEra
	config       ServerConfig
	bundleLoaded bool
	skills       *SkillRegistry
}

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	BundlePath   string
	RepoPath     string
	KnowledgeDir string
	Logger       *log.Logger
}

// NewServer creates a new MCP server. Returns an error if the immutable Skill
// registry cannot be constructed (T2.3: no panic/partial catalog).
// BundlePath is NOT auto-loaded at construction; it is loaded only after a
// legacy initialize selects the legacy era (S16).
func NewServer(config ServerConfig) (*Server, error) {
	logger := config.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "[okf-mcp] ", log.LstdFlags)
	}

	service := toolsvc.NewService(toolsvc.Config{
		RepoPath:     config.RepoPath,
		KnowledgeDir: config.KnowledgeDir,
	})

	// Build immutable Skill registry (fail closed).
	skills, err := NewSkillRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to build skill registry: %w", err)
	}

	s := &Server{
		tools:  NewToolRegistryWithService(service),
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
		logger: logger,
		config: config,
		skills: skills,
	}

	return s, nil
}

// serverInfo returns the shared server implementation info from the single version source.
func (s *Server) serverInfo() ImplementationInfo {
	return ImplementationInfo{
		Name:    "okf-mcp-server",
		Version: meta.Version,
	}
}

// loadBundleLegacy loads the configured bundle for the legacy era. Called only
// after a successful legacy initialize (S16).
func (s *Server) loadBundleLegacy() {
	if s.config.BundlePath == "" || s.bundleLoaded {
		return
	}
	bundle, err := loadBundleSilent(s.config.BundlePath)
	if err != nil {
		s.logger.Printf("Warning: failed to auto-load bundle from %s: %v", s.config.BundlePath, err)
	} else {
		s.tools.SetBundle(bundle, s.config.BundlePath)
		s.logger.Printf("Auto-loaded bundle from %s (%d concepts)", s.config.BundlePath, len(bundle.Concepts))
	}
	s.bundleLoaded = true
}

func loadBundleSilent(path string) (*okf.KnowledgeBundle, error) {
	return okf.LoadBundle(path, &okf.LoadOptions{Recursive: true})
}

// Run starts the MCP server main loop.
// Uses the MCP stdio transport framing: Content-Length headers (LSP-style).
// Also accepts newline-delimited JSON for backward compatibility.
func (s *Server) Run() error {
	s.logger.Println("OKF MCP Server starting...")
	s.logger.Printf("Registered tools: %d", len(s.tools.List()))

	for {
		data, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				s.logger.Println("Client disconnected (EOF)")
				return nil
			}
			s.logger.Printf("Read error: %v", err)
			return err
		}
		if len(data) == 0 {
			continue
		}
		s.logger.Printf("Received: %s", truncate(string(data), 200))
		s.handleMessage(data)
	}
}

// readMessage reads one JSON-RPC message. Supports both Content-Length header
// framing (per MCP spec) and newline-delimited JSON (for simple testing).
func (s *Server) readMessage() ([]byte, error) {
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")

	// Content-Length header framing (MCP standard)
	if strings.HasPrefix(strings.ToLower(line), "content-length:") {
		clStr := strings.TrimSpace(line[len("Content-Length:"):])
		contentLen, perr := strconv.Atoi(clStr)
		if perr != nil || contentLen < 0 || contentLen > maxStdioMessageBytes {
			return nil, fmt.Errorf("invalid Content-Length: %s (must be between 0 and %d)", clStr, maxStdioMessageBytes)
		}
		// Read remaining headers until empty line
		for {
			hdrLine, herr := s.reader.ReadString('\n')
			if herr != nil {
				return nil, herr
			}
			if strings.TrimSpace(hdrLine) == "" {
				break
			}
		}
		// Read exactly contentLen bytes
		data := make([]byte, contentLen)
		if _, ferr := io.ReadFull(s.reader, data); ferr != nil {
			return nil, ferr
		}
		s.framing = framingContentLength
		return data, nil
	}

	// Newline-delimited JSON is the stdio framing used by modern MCP clients.
	s.framing = framingNewline
	return []byte(line), nil
}

func (s *Server) handleMessage(data []byte) {
	method, id, params, isNotification, err := ParseMessage(data)
	if err != nil {
		s.logger.Printf("Parse error: %v", err)
		s.sendError(nil, ParseErrorCode, err.Error())
		return
	}

	if isNotification {
		s.handleNotification(method, params)
		return
	}

	// Dual-era dispatch (S01-S05).
	// Modern requests carry _meta in params; legacy uses initialize.
	if s.era == eraUnselected {
		// Check if params contain _meta (modern) or this is initialize (legacy).
		// Presence of _meta selects modern era even if version is unsupported;
		// version validation returns -32022 in the modern handler (S03).
		if hasModernMeta(params) {
			s.era = eraModern
			s.logger.Println("Era selected: modern 2026-07-28")
		} else if method == "initialize" {
			s.era = eraLegacy
			s.logger.Println("Era selected: legacy 2024-11-05")
		} else {
			// Neither _meta nor initialize: reject (S05).
			s.sendError(id, InvalidRequestCode, "request must begin with initialize (legacy) or carry modern _meta")
			return
		}
	}

	if s.era == eraModern {
		s.handleModernMessage(method, id, params)
		return
	}

	// Legacy era.
	switch method {
	case "initialize":
		s.handleInitialize(id, params)
	case "tools/list":
		s.handleToolsList(id)
	case "tools/call":
		s.handleToolsCall(id, params)
	case "resources/list":
		s.handleResourcesList(id)
	case "resources/read":
		s.handleResourcesRead(id, params)
	case "prompts/list":
		s.handlePromptsList(id)
	case "prompts/get":
		s.handlePromptsGet(id, params)
	case "ping":
		s.sendResponse(id, map[string]interface{}{})
	default:
		// Mixed-era rejection (S05): modern method in legacy process.
		if isModernMethod(method) {
			s.sendError(id, InvalidRequestCode, "method not available in legacy era; restart for modern 2026-07-28")
			return
		}
		s.logger.Printf("Unknown method: %s", method)
		s.sendError(id, MethodNotFoundCode, fmt.Sprintf("Method not found: %s", method))
	}
}

// hasModernMeta reports whether params contain a _meta object (modern request marker).
func hasModernMeta(params json.RawMessage) bool {
	if len(params) == 0 {
		return false
	}
	var wrapper struct {
		Meta json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(params, &wrapper); err != nil {
		return false
	}
	return len(wrapper.Meta) > 0
}

// extractModernMeta attempts to parse _meta from params. Returns error if absent/invalid.
func extractModernMeta(params json.RawMessage) (*ModernRequestMeta, error) {
	if len(params) == 0 {
		return nil, fmt.Errorf("no params")
	}
	var wrapper struct {
		Meta json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(params, &wrapper); err != nil {
		return nil, err
	}
	return ParseModernMeta(wrapper.Meta)
}

// isModernMethod reports whether a method is modern-only.
func isModernMethod(method string) bool {
	switch method {
	case "server/discover", "skills/list", "skills/get":
		return true
	}
	return false
}

// handleModernMessage dispatches modern-era methods with per-request _meta validation.
func (s *Server) handleModernMessage(method string, id json.RawMessage, params json.RawMessage) {
	meta, err := extractModernMeta(params)
	if err != nil {
		var rpErr *RPCError
		if e, ok := err.(*RPCError); ok {
			rpErr = e
		} else {
			rpErr = &RPCError{Code: InvalidParamsCode, Message: err.Error()}
		}
		s.sendError(id, rpErr.Code, rpErr.Message)
		return
	}

	switch method {
	case "server/discover":
		s.handleModernDiscover(id, meta)
	case "tools/list":
		s.handleModernToolsList(id, meta)
	case "tools/call":
		s.handleModernToolsCall(id, meta, params)
	case "resources/list":
		s.handleModernResourcesList(id, meta)
	case "resources/read":
		s.handleModernResourcesRead(id, meta, params)
	case "skills/list":
		s.handleModernSkillsList(id, meta, params)
	case "skills/get":
		s.handleModernSkillsGet(id, meta, params)
	case "prompts/list", "prompts/get", "ping":
		s.handleModernNotImplemented(id, method)
	default:
		s.sendError(id, MethodNotFoundCode, fmt.Sprintf("Method not found: %s", method))
	}
}

func (s *Server) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "notifications/initialized":
		s.logger.Println("Client initialized notification received")
	case "notifications/cancelled":
		s.logger.Println("Request cancelled notification")
	default:
		s.logger.Printf("Unknown notification: %s", method)
	}
}

func (s *Server) handleInitialize(id json.RawMessage, params json.RawMessage) {
	var initParams InitializeRequestParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &initParams); err != nil {
			s.logger.Printf("Failed to parse initialize params: %v", err)
		}
	}

	s.logger.Printf("Client: %s v%s (protocol %s)",
		initParams.ClientInfo.Name,
		initParams.ClientInfo.Version,
		initParams.ProtocolVersion)

	// Legacy era: load bundle after successful initialize (S16).
	s.loadBundleLegacy()

	result := InitializeResult{
		ProtocolVersion: LegacyProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{ListChanged: false},
			Resources: &ResourcesCapability{ListChanged: false},
			Prompts:   &PromptsCapability{ListChanged: false},
		},
		ServerInfo:   s.serverInfo(),
		Instructions: "OKF (Open Knowledge Format) MCP Server. Load and query knowledge bundles, inspect concepts, run lint checks.",
	}

	s.sendResponse(id, result)
}

func (s *Server) handleToolsList(id json.RawMessage) {
	tools := s.tools.List()
	result := ToolsListResult{
		Tools: tools,
	}
	s.sendResponse(id, result)
}

func (s *Server) handleToolsCall(id json.RawMessage, params json.RawMessage) {
	var callParams ToolCallParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Invalid params: %v", err))
		return
	}

	s.logger.Printf("Tool call: %s", callParams.Name)

	result, err := s.tools.Call(callParams.Name, callParams.Arguments)
	if err != nil {
		s.logger.Printf("Tool error: %v", err)
		s.sendResponse(id, &ToolCallResult{
			Content: []ContentItem{TextContent(fmt.Sprintf("Error: %v", err))},
			IsError: true,
		})
		return
	}

	s.sendResponse(id, result)
}

func (s *Server) handleResourcesList(id json.RawMessage) {
	bundle, path := s.tools.GetBundle()
	var resources []Resource
	if bundle != nil {
		resources = append(resources, Resource{
			URI:         fmt.Sprintf("okf://bundle/%s", path),
			Name:        "Knowledge Bundle",
			Description: fmt.Sprintf("OKF bundle at %s with %d concepts", path, len(bundle.Concepts)),
			MIMEType:    "application/json",
		})
		for _, c := range bundle.Concepts {
			resources = append(resources, Resource{
				URI:         fmt.Sprintf("okf://concept/%s/%s", path, c.FilePath),
				Name:        c.Title,
				Description: fmt.Sprintf("[%s] %s", c.Type, c.FilePath),
				MIMEType:    "text/markdown",
			})
		}
	}
	// Append Skill Resource additively (S22): legacy prior Resources retain order
	// before the appended Skill.
	if s.skills != nil {
		resources = append(resources, s.skills.Resources()...)
	}
	s.sendResponse(id, ResourcesListResult{Resources: resources})
}

func (s *Server) handleResourcesRead(id json.RawMessage, params json.RawMessage) {
	var readParams ResourceReadParams
	if err := json.Unmarshal(params, &readParams); err != nil {
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Invalid params: %v", err))
		return
	}

	uri := readParams.URI

	// Skill Resource read works in both eras (S23) and does not require a bundle.
	if strings.HasPrefix(uri, "skill:") {
		if s.skills != nil {
			content, err := s.skills.Read(uri)
			if err != nil {
				s.sendError(id, InvalidParamsCode, err.Error())
				return
			}
			s.sendResponse(id, ResourceReadResult{
				Contents: []ResourceContents{content},
			})
			return
		}
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Unsupported resource URI: %s", uri))
		return
	}

	bundle, bundlePath := s.tools.GetBundle()
	if bundle == nil {
		s.sendError(id, InvalidParamsCode, "No bundle loaded")
		return
	}

	// Parse URI: okf://concept/{bundlePath}/{conceptPath}
	if strings.HasPrefix(uri, "okf://concept/") {
		rest := strings.TrimPrefix(uri, "okf://concept/")
		// Remove bundle path prefix
		conceptPath := strings.TrimPrefix(rest, bundlePath+"/")
		for _, c := range bundle.Concepts {
			if c.FilePath == conceptPath {
				data, err := parser.SerializeConcept(conceptToParser(c), true)
				if err != nil {
					s.sendError(id, InternalErrorCode, err.Error())
					return
				}
				s.sendResponse(id, ResourceReadResult{
					Contents: []ResourceContents{{
						URI:      uri,
						MIMEType: "text/markdown",
						Text:     string(data),
					}},
				})
				return
			}
		}
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Concept not found: %s", conceptPath))
		return
	}

	s.sendError(id, InvalidParamsCode, fmt.Sprintf("Unsupported resource URI: %s", uri))
}

func (s *Server) handlePromptsList(id json.RawMessage) {
	prompts := []Prompt{
		{
			Name:        "okf_explain_concept",
			Description: "Explain a concept from the knowledge bundle",
			Arguments: []PromptArgument{
				{Name: "path", Description: "Path to the concept", Required: true},
				{Name: "depth", Description: "Explanation depth (brief/detailed/comprehensive)"},
			},
		},
		{
			Name:        "okf_summarize_bundle",
			Description: "Summarize the entire knowledge bundle",
		},
	}
	s.sendResponse(id, PromptsListResult{Prompts: prompts})
}

func (s *Server) handlePromptsGet(id json.RawMessage, params json.RawMessage) {
	var getParams PromptGetParams
	if err := json.Unmarshal(params, &getParams); err != nil {
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Invalid params: %v", err))
		return
	}

	var result PromptGetResult
	switch getParams.Name {
	case "okf_explain_concept":
		path := getParams.Arguments["path"]
		result = PromptGetResult{
			Description: "Explain a concept",
			Messages: []PromptMessage{
				{
					Role:    "user",
					Content: TextContent(fmt.Sprintf("Please explain the concept at path '%s' from the OKF knowledge bundle. Use the okf_get_concept tool to retrieve it first, then provide a clear explanation.", path)),
				},
			},
		}
	case "okf_summarize_bundle":
		result = PromptGetResult{
			Description: "Summarize the knowledge bundle",
			Messages: []PromptMessage{
				{
					Role:    "user",
					Content: TextContent("Please summarize the OKF knowledge bundle. First use okf_bundle_stats to get statistics, then okf_list_concepts to list concepts, and provide a comprehensive summary."),
				},
			},
		}
	default:
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Unknown prompt: %s", getParams.Name))
		return
	}

	s.sendResponse(id, result)
}

func (s *Server) sendResponse(id json.RawMessage, result interface{}) {
	resp, err := NewResponse(id, result)
	if err != nil {
		s.logger.Printf("Failed to create response: %v", err)
		return
	}
	s.writeMessage(resp)
}

func (s *Server) sendError(id json.RawMessage, code int, message string) {
	resp := NewErrorResponse(id, code, message)
	s.writeMessage(resp)
}

func (s *Server) writeMessage(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Printf("Failed to marshal message: %v", err)
		return
	}
	var payload []byte
	if s.framing == framingNewline {
		payload = append(data, '\n')
	} else {
		var buf strings.Builder
		buf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(data)))
		buf.WriteString("\r\n")
		buf.Write(data)
		payload = []byte(buf.String())
	}
	if _, err := s.writer.Write(payload); err != nil {
		s.logger.Printf("Failed to write message: %v", err)
		return
	}
	// Flush stdout to ensure message is sent promptly
	if f, ok := s.writer.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
	s.logger.Printf("Sent: %s", truncate(string(data), 200))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
