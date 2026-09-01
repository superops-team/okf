package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"strings"
	"testing"
)

func TestServerRespondsUsingRequestFraming(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFirst string
	}{
		{
			name:      "newline",
			input:     `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n",
			wantFirst: `{"jsonrpc":"2.0"`,
		},
		{
			name:      "content-length",
			input:     contentLengthFrame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`),
			wantFirst: "Content-Length:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			server := NewServer(ServerConfig{RepoPath: t.TempDir(), Logger: log.New(io.Discard, "", 0)})
			server.reader = bufio.NewReader(strings.NewReader(tt.input))
			server.writer = &out

			if err := server.Run(); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !strings.HasPrefix(out.String(), tt.wantFirst) {
				t.Fatalf("response prefix = %q, want %q", out.String(), tt.wantFirst)
			}
			if tt.name == "newline" {
				var response Response
				if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil {
					t.Fatalf("decode newline response: %v", err)
				}
			}
		})
	}
}

func TestServerRejectsUnsafeContentLength(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header string
	}{
		{name: "negative", header: "Content-Length: -1\r\n\r\n"},
		{name: "oversized", header: "Content-Length: 16777217\r\n\r\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer(ServerConfig{RepoPath: t.TempDir(), Logger: log.New(io.Discard, "", 0)})
			server.reader = bufio.NewReader(strings.NewReader(tt.header))
			if _, err := server.readMessage(); err == nil || !strings.Contains(err.Error(), "Content-Length") {
				t.Fatalf("readMessage error = %v, want Content-Length rejection", err)
			}
		})
	}
}

func contentLengthFrame(body string) string {
	return "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}
