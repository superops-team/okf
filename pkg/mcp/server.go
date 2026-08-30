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
	"github.com/superops-team/okf/pkg/parser"
	toolsvc "github.com/superops-team/okf/pkg/tool"
)

type stdioFraming uint8

const (
	framingContentLength stdioFraming = iota
	framingNewline

	maxStdioMessageBytes = 16 << 20
)

// Server is an MCP server that communicates over stdio.
type Server struct {
	tools   *ToolRegistry
	reader  *bufio.Reader
	writer  io.Writer
	logger  *log.Logger
	framing stdioFraming
}

// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	BundlePath   string
	RepoPath     string
	KnowledgeDir string
	Logger       *log.Logger
}

// NewServer creates a new MCP server.
func NewServer(config ServerConfig) *Server {
	logger := config.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "[okf-mcp] ", log.LstdFlags)
	}

	service := toolsvc.NewService(toolsvc.Config{
		RepoPath:     config.RepoPath,
		KnowledgeDir: config.KnowledgeDir,
	})
	s := &Server{
		tools:  NewToolRegistryWithService(service),
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
		logger: logger,
	}

	// Auto-load bundle if path is provided
	if config.BundlePath != "" {
		bundle, err := loadBundleSilent(config.BundlePath)
		if err != nil {
			logger.Printf("Warning: failed to auto-load bundle from %s: %v", config.BundlePath, err)
		} else {
			s.tools.SetBundle(bundle, config.BundlePath)
			logger.Printf("Auto-loaded bundle from %s (%d concepts)", config.BundlePath, len(bundle.Concepts))
		}
	}

	return s
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
		// Try to send error response if we can extract id
		s.sendError(nil, ParseErrorCode, err.Error())
		return
	}

	if isNotification {
		s.handleNotification(method, params)
		return
	}

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
		s.logger.Printf("Unknown method: %s", method)
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

	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{ListChanged: false},
			Resources: &ResourcesCapability{ListChanged: false},
			Prompts:   &PromptsCapability{ListChanged: false},
		},
		ServerInfo: ImplementationInfo{
			Name:    "okf-mcp-server",
			Version: "0.3.0",
		},
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
	s.sendResponse(id, ResourcesListResult{Resources: resources})
}

func (s *Server) handleResourcesRead(id json.RawMessage, params json.RawMessage) {
	var readParams ResourceReadParams
	if err := json.Unmarshal(params, &readParams); err != nil {
		s.sendError(id, InvalidParamsCode, fmt.Sprintf("Invalid params: %v", err))
		return
	}

	bundle, bundlePath := s.tools.GetBundle()
	if bundle == nil {
		s.sendError(id, InvalidParamsCode, "No bundle loaded")
		return
	}

	// Parse URI: okf://concept/{bundlePath}/{conceptPath}
	uri := readParams.URI
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
