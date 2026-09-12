package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestManifestBenchmark1000FileBytesRead is a machine-readable evidence emitter
// for S48 (resource bounds). It builds 1,000 concept files, each with a small
// frontmatter and a multi-hundred-KiB body, scans them through the production
// ScanKnowledgeFiles with an instrumented counting reader, and prints a single
// BENCH line: total body bytes on disk vs the bytes the Manifest actually read.
// The verify script greps this line into evidence.md.
//
// The invariant: bytesRead is bounded by the frontmatter bytes plus one 4 KiB
// buffer prefetch per file, and is NOT proportional to the multi-MiB bodies.
func TestManifestBenchmark1000FileBytesRead(t *testing.T) {
	const n = 1000
	const bodySize = 256 * 1024 // 256 KiB body per file
	body := strings.Repeat("B", bodySize)

	root := t.TempDir()
	fr := newCountingReader()
	totalBody := int64(0)
	totalFrontmatter := int64(0)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("concept_%04d.md", i)
		front := fmt.Sprintf("---\ntype: concept\ntitle: Bench %04d\ntags: [go]\n---\n", i)
		content := front + body
		totalBody += int64(bodySize)
		totalFrontmatter += int64(len(front))
		abs := filepath.Join(root, name)
		fr.add(abs, content)
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}

	reports, err := ScanKnowledgeFiles(context.Background(), root, fr)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(reports) != n {
		t.Fatalf("reports = %d, want %d", len(reports), n)
	}

	// Bounded read: at most frontmatter bytes plus one 4 KiB prefetch per file.
	perFileBudget := totalFrontmatter + int64(n)*readBufferSize
	if fr.bytesRead > perFileBudget {
		t.Fatalf("bytesRead=%d exceeds frontmatter+prefetch budget=%d (bodies were streamed)",
			fr.bytesRead, perFileBudget)
	}
	fmt.Printf("BENCH files=%d body_bytes_on_disk=%d frontmatter_bytes=%d bytes_read=%d per_file_prefetch=%d\n",
		n, totalBody, totalFrontmatter, fr.bytesRead, readBufferSize)
}
