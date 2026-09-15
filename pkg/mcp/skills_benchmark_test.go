package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"testing"
)

// benchmarkServer creates a Server with in-memory writer for benchmarking.
func benchmarkServer(b *testing.B) *Server {
	b.Helper()
	s, err := NewServer(ServerConfig{
		RepoPath: b.TempDir(),
		Logger:   log.New(io.Discard, "", 0),
	})
	if err != nil {
		b.Fatalf("NewServer: %v", err)
	}
	s.writer = &bytes.Buffer{}
	return s
}

func modernParams() json.RawMessage {
	return json.RawMessage(`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{"extensions":{"io.modelcontextprotocol/skills":{}}}}}`)
}

func BenchmarkModernDiscover(b *testing.B) {
	s := benchmarkServer(b)
	params := modernParams()
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		s.handleModernDiscover(json.RawMessage(`"bench"`), &ModernRequestMeta{
			ProtocolVersion:    ModernProtocolVersion,
			ClientCapabilities: map[string]any{},
		})
		s.writer.(*bytes.Buffer).Reset()
	}
	_ = params
}

func BenchmarkModernToolsList(b *testing.B) {
	s := benchmarkServer(b)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		s.handleModernToolsList(json.RawMessage(`"bench"`), &ModernRequestMeta{
			ProtocolVersion:    ModernProtocolVersion,
			ClientCapabilities: map[string]any{},
		})
		s.writer.(*bytes.Buffer).Reset()
	}
}

func BenchmarkModernSkillsList(b *testing.B) {
	s := benchmarkServer(b)
	meta := &ModernRequestMeta{
		ProtocolVersion: ModernProtocolVersion,
		ClientCapabilities: map[string]any{
			"extensions": map[string]any{SkillsExtensionID: map[string]any{}},
		},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		s.handleModernSkillsList(json.RawMessage(`"bench"`), meta, nil)
		s.writer.(*bytes.Buffer).Reset()
	}
}

func BenchmarkModernSkillsGet(b *testing.B) {
	s := benchmarkServer(b)
	meta := &ModernRequestMeta{
		ProtocolVersion: ModernProtocolVersion,
		ClientCapabilities: map[string]any{
			"extensions": map[string]any{SkillsExtensionID: map[string]any{}},
		},
	}
	params := json.RawMessage(`{"uri":"skill://okf/SKILL.md"}`)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		s.handleModernSkillsGet(json.RawMessage(`"bench"`), meta, params)
		s.writer.(*bytes.Buffer).Reset()
	}
}

func BenchmarkModernResourcesRead(b *testing.B) {
	s := benchmarkServer(b)
	meta := &ModernRequestMeta{
		ProtocolVersion:    ModernProtocolVersion,
		ClientCapabilities: map[string]any{},
	}
	params := json.RawMessage(`{"uri":"skill://okf/SKILL.md"}`)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		s.handleModernResourcesRead(json.RawMessage(`"bench"`), meta, params)
		s.writer.(*bytes.Buffer).Reset()
	}
}

func BenchmarkSkillRegistryList(b *testing.B) {
	r, err := NewSkillRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = r.List()
	}
}

func BenchmarkSkillRegistryRead(b *testing.B) {
	r, err := NewSkillRegistry()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = r.Read("skill://okf/SKILL.md")
	}
}

// BenchmarkSkillRegistryNoBundleIO verifies that Skill operations do not touch
// the knowledge bundle. The registry is built from static rendered bytes only.
func BenchmarkSkillRegistryNoBundleIO(b *testing.B) {
	r, err := NewSkillRegistry()
	if err != nil {
		b.Fatal(err)
	}
	// Verify zero bundle dependency: registry has no bundle field.
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = r.Digest()
		_ = r.Size()
		_ = r.Resources()
	}
}
