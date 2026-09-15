package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"sync/atomic"
	"testing"

	"github.com/superops-team/okf/pkg/okf"
)

// spyLoader counts calls and records whether it was invoked.
type spyLoader struct {
	calls atomic.Int32
}

func (s *spyLoader) load(path string) (*okf.KnowledgeBundle, error) {
	s.calls.Add(1)
	// Return an empty bundle (not nil) so SetBundle succeeds.
	return &okf.KnowledgeBundle{}, nil
}

func (s *spyLoader) count() int {
	return int(s.calls.Load())
}

// newSpyServer creates a Server with an injected bundle loader spy.
func newSpyServer(t *testing.T, bundlePath string) (*Server, *spyLoader) {
	t.Helper()
	spy := &spyLoader{}
	s, err := NewServer(ServerConfig{
		RepoPath:   t.TempDir(),
		BundlePath: bundlePath,
		Logger:     log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.writer = &bytes.Buffer{}
	s.framing = framingNewline
	s.bundleLoader = spy.load
	return s, spy
}

// S16: NewServer must NOT load BundlePath at construction.
func TestNewServerDoesNotLoadBundle(t *testing.T) {
	_, spy := newSpyServer(t, "/some/bundle/path")
	if spy.count() != 0 {
		t.Errorf("NewServer loaded bundle %d times, expected 0", spy.count())
	}
}

// S16: modern discover/list/get/read must NOT load BundlePath.
func TestModernOperationsDoNotLoadBundle(t *testing.T) {
	s, spy := newSpyServer(t, "/some/bundle/path")
	meta := &ModernRequestMeta{
		ProtocolVersion:    ModernProtocolVersion,
		ClientCapabilities: map[string]any{},
	}

	// modern discover
	s.handleModernDiscover(jsonRaw(`"1"`), meta)
	if spy.count() != 0 {
		t.Errorf("after discover: bundle loaded %d times, expected 0", spy.count())
	}

	// modern tools/list
	s.handleModernToolsList(jsonRaw(`"2"`), meta)
	if spy.count() != 0 {
		t.Errorf("after tools/list: bundle loaded %d times, expected 0", spy.count())
	}

	// modern resources/list
	s.handleModernResourcesList(jsonRaw(`"3"`), meta)
	if spy.count() != 0 {
		t.Errorf("after resources/list: bundle loaded %d times, expected 0", spy.count())
	}

	// modern resources/read (skill)
	s.handleModernResourcesRead(jsonRaw(`"4"`), meta, jsonRaw(`{"uri":"skill://okf/SKILL.md"}`))
	if spy.count() != 0 {
		t.Errorf("after resources/read: bundle loaded %d times, expected 0", spy.count())
	}

	// modern skills/list (with capability)
	metaSkills := &ModernRequestMeta{
		ProtocolVersion: ModernProtocolVersion,
		ClientCapabilities: map[string]any{
			"extensions": map[string]any{SkillsExtensionID: map[string]any{}},
		},
	}
	s.handleModernSkillsList(jsonRaw(`"5"`), metaSkills, nil)
	if spy.count() != 0 {
		t.Errorf("after skills/list: bundle loaded %d times, expected 0", spy.count())
	}

	// modern skills/get
	s.handleModernSkillsGet(jsonRaw(`"6"`), metaSkills, jsonRaw(`{"uri":"skill://okf/SKILL.md"}`))
	if spy.count() != 0 {
		t.Errorf("after skills/get: bundle loaded %d times, expected 0", spy.count())
	}
}

// S16: legacy initialize must load BundlePath exactly once.
func TestLegacyInitializeLoadsBundleOnce(t *testing.T) {
	s, spy := newSpyServer(t, "/some/bundle/path")

	// First legacy initialize
	s.handleInitialize(jsonRaw(`"1"`), jsonRaw(`{"protocolVersion":"2024-11-05","capabilities":{}}`))
	if spy.count() != 1 {
		t.Errorf("after first initialize: bundle loaded %d times, expected 1", spy.count())
	}

	// Second legacy initialize should NOT reload (bundleLoaded guard)
	s.handleInitialize(jsonRaw(`"2"`), jsonRaw(`{"protocolVersion":"2024-11-05","capabilities":{}}`))
	if spy.count() != 1 {
		t.Errorf("after second initialize: bundle loaded %d times, expected 1 (idempotent)", spy.count())
	}
}

// S16: legacy initialize with empty BundlePath must NOT load.
func TestLegacyInitializeEmptyBundlePathNoLoad(t *testing.T) {
	s, spy := newSpyServer(t, "")
	s.handleInitialize(jsonRaw(`"1"`), jsonRaw(`{"protocolVersion":"2024-11-05","capabilities":{}}`))
	if spy.count() != 0 {
		t.Errorf("empty BundlePath: bundle loaded %d times, expected 0", spy.count())
	}
}

// S24: unknown skill: URI must NOT trigger bundle or filesystem fallback.
func TestUnknownSkillURINoBundleFallback(t *testing.T) {
	s, spy := newSpyServer(t, "/some/bundle/path")
	meta := &ModernRequestMeta{
		ProtocolVersion:    ModernProtocolVersion,
		ClientCapabilities: map[string]any{},
	}

	// Read an unknown skill: URI — must return error without touching bundle.
	s.handleModernResourcesRead(jsonRaw(`"1"`), meta, jsonRaw(`{"uri":"skill://okf/unknown.md"}`))
	if spy.count() != 0 {
		t.Errorf("unknown skill URI triggered bundle load %d times, expected 0", spy.count())
	}

	// Verify the response is an error (not a fallback to bundle content).
	output := s.writer.(*bytes.Buffer).String()
	var resp map[string]any
	json.Unmarshal([]byte(output), &resp)
	if resp["error"] == nil {
		t.Error("unknown skill URI should return error, got success")
	}
}

// S24: traversal-shaped skill: URI must NOT trigger bundle fallback.
func TestTraversalSkillURINoBundleFallback(t *testing.T) {
	s, spy := newSpyServer(t, "/some/bundle/path")
	meta := &ModernRequestMeta{
		ProtocolVersion:    ModernProtocolVersion,
		ClientCapabilities: map[string]any{},
	}

	s.handleModernResourcesRead(jsonRaw(`"1"`), meta, jsonRaw(`{"uri":"skill://okf/../../../etc/passwd"}`))
	if spy.count() != 0 {
		t.Errorf("traversal skill URI triggered bundle load %d times, expected 0", spy.count())
	}
}

func jsonRaw(s string) []byte {
	return []byte(s)
}
