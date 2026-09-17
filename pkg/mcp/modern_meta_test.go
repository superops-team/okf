package mcp

import (
	"encoding/json"
	"testing"
)

func TestModernMetaValidation(t *testing.T) {
	tests := []struct {
		name    string
		meta    json.RawMessage
		wantErr bool
		code    int
	}{
		{
			name: "valid modern meta",
			meta: json.RawMessage(`{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`),
		},
		{
			name:    "missing protocolVersion",
			meta:    json.RawMessage(`{"io.modelcontextprotocol/clientCapabilities":{}}`),
			wantErr: true,
			code:    InvalidParamsCode,
		},
		{
			name:    "wrong protocolVersion type",
			meta:    json.RawMessage(`{"io.modelcontextprotocol/protocolVersion":2026,"io.modelcontextprotocol/clientCapabilities":{}}`),
			wantErr: true,
			code:    InvalidParamsCode,
		},
		{
			name:    "missing clientCapabilities",
			meta:    json.RawMessage(`{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}`),
			wantErr: true,
			code:    InvalidParamsCode,
		},
		{
			name:    "unsupported version",
			meta:    json.RawMessage(`{"io.modelcontextprotocol/protocolVersion":"2024-11-05","io.modelcontextprotocol/clientCapabilities":{}}`),
			wantErr: true,
			code:    UnsupportedProtocolVersionCode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := ParseModernMeta(tt.meta)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var rpErr *RPCError
				if !errorsAsType(err, &rpErr) {
					t.Fatalf("expected *RPCError, got %T", err)
				}
				if rpErr.Code != tt.code {
					t.Errorf("expected code %d, got %d", tt.code, rpErr.Code)
				}
				if tt.code == UnsupportedProtocolVersionCode {
					data, ok := rpErr.Data.(map[string]any)
					if !ok {
						t.Fatal("expected error data map")
					}
					if data["requested"] != "2024-11-05" {
						t.Errorf("expected requested=2024-11-05, got %v", data["requested"])
					}
					supported, ok := data["supported"].([]string)
					if !ok || len(supported) != 1 || supported[0] != ModernProtocolVersion {
						t.Errorf("expected supported=[%s], got %v", ModernProtocolVersion, data["supported"])
					}
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if meta.ProtocolVersion != ModernProtocolVersion {
					t.Errorf("expected version %s, got %s", ModernProtocolVersion, meta.ProtocolVersion)
				}
			}
		})
	}
}

func TestModernMetaHasSkillsCapability(t *testing.T) {
	meta := &ModernRequestMeta{
		ProtocolVersion: ModernProtocolVersion,
		ClientCapabilities: map[string]any{
			"extensions": map[string]any{
				"io.modelcontextprotocol/skills": map[string]any{},
			},
		},
	}
	if !meta.HasSkillsCapability() {
		t.Error("expected skills capability")
	}
	meta2 := &ModernRequestMeta{
		ProtocolVersion:    ModernProtocolVersion,
		ClientCapabilities: map[string]any{},
	}
	if meta2.HasSkillsCapability() {
		t.Error("expected no skills capability")
	}
}

func TestMissingCapabilityError(t *testing.T) {
	err := NewMissingCapabilityError("io.modelcontextprotocol/skills")
	if err.Code != MissingRequiredClientCapabilityCode {
		t.Errorf("expected code %d, got %d", MissingRequiredClientCapabilityCode, err.Code)
	}
	data, ok := err.Data.(map[string]any)
	if !ok {
		t.Fatal("expected data map")
	}
	if data["requiredCapability"] != "io.modelcontextprotocol/skills" {
		t.Errorf("expected requiredCapability, got %v", data["requiredCapability"])
	}
}

func errorsAsType(err error, target **RPCError) bool {
	if e, ok := err.(*RPCError); ok {
		*target = e
		return true
	}
	return false
}
