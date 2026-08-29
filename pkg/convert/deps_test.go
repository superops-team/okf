package convert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S2: the downmark dependency must be pinned to an exact version so that
// conversion output is deterministic (see docs/knowledge/releases.md).
func TestDependencyPinned(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "github.com/giraffesyo/downmark") {
			// require an exact version (vX.Y.Z), not a floating range
			fields := strings.Fields(line)
			if len(fields) != 2 {
				t.Fatalf("downmark line not pinned: %q", line)
			}
			ver := fields[1]
			if !strings.HasPrefix(ver, "v") || strings.Contains(ver, "x") {
				t.Fatalf("downmark version must be exact, got %q", ver)
			}
			t.Logf("downmark pinned to %s", ver)
			return
		}
	}
	t.Fatal("downmark not found in go.mod")
}

// S21: the re-import semantics of a downmark upgrade must be declared in the
// Release Notes.
func TestReleaseNotesDeclaresReimport(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "knowledge", "releases.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "re-import") && !strings.Contains(text, "Re-import") {
		t.Errorf("Release Notes must declare re-import semantics on downmark upgrade")
	}
	if !strings.Contains(text, "downmark v0.10.0") && !strings.Contains(text, "pinned") {
		t.Errorf("Release Notes must mention the pinned downmark version")
	}
}
