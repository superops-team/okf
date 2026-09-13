package vectorindex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/okf/pkg/identity"
)

// S15: a v2 (or otherwise incompatible) vector index is never silently used as
// v3. Loading it must return the typed index_rebuild_required error whose
// remediation names "okf vector rebuild", and no hit may be returned.
func TestVectorV2RequiresRebuild(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(3)
	idx.Add("v3:id:okf_17a2c56db85c4889b4f8fe02ca9ac67e#0", []float32{1, 0, 0})
	if err := idx.Save(dir, Meta{Model: "m", OkfVersion: "v"}); err != nil {
		t.Fatal(err)
	}

	// Downgrade the persisted meta to v2 to simulate a pre-v3 index.
	metaPath := filepath.Join(dir, MetaFileName)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.IndexFormatVersion = 2
	patched, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, patched, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := NewHNSW(3)
	_, err = loaded.Load(dir)
	if err == nil {
		t.Fatal("v2 index loaded without error (would silently return stale hits)")
	}
	// The error must carry the stable index_rebuild_required code.
	if !errors.Is(err, identity.ErrIndexRebuildRequired) {
		t.Fatalf("Load error = %v, want errors.Is identity.ErrIndexRebuildRequired", err)
	}
	var identErr *identity.Error
	if !errors.As(err, &identErr) {
		t.Fatalf("Load error %T does not wrap *identity.Error", err)
	}
	if identErr.Code != identity.CodeIndexRebuildRequired {
		t.Fatalf("error code = %q, want %q", identErr.Code, identity.CodeIndexRebuildRequired)
	}
	if !strings.Contains(err.Error(), "okf vector rebuild") {
		t.Fatalf("error message %q does not name remediation 'okf vector rebuild'", err.Error())
	}
	// No v2 hit may be surfaced as a live v3 hit: the loaded index must be empty.
	if loaded.Len() != 0 {
		t.Fatalf("loaded v2-mismatch index Len = %d, want 0", loaded.Len())
	}
}
