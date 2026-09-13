package manifest

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeFile is an in-memory file served by countingReader.
type fakeFile struct {
	data []byte
}

func (f *fakeFile) Read(p []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

// countingReader is an instrumented FileReader that counts total bytes handed to
// callers via Read, so S22 can prove the multi-megabyte body is never streamed.
type countingReader struct {
	files      map[string]*fakeFile
	bytesRead  int64
	openErrors map[string]error
}

func newCountingReader() *countingReader {
	return &countingReader{files: map[string]*fakeFile{}, openErrors: map[string]error{}}
}

func (c *countingReader) add(path, content string) {
	c.files[path] = &fakeFile{data: []byte(content)}
}

func (c *countingReader) Open(path string) (io.ReadCloser, error) {
	if err, ok := c.openErrors[path]; ok {
		return nil, err
	}
	f, ok := c.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &countingReadCloser{src: f, counter: c}, nil
}

func (c *countingReader) Stat(path string) (fs.FileInfo, error) {
	f, ok := c.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &fakeFileInfo{size: int64(len(f.data))}, nil
}

type countingReadCloser struct {
	src     *fakeFile
	counter *countingReader
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	r.counter.bytesRead += int64(n)
	return n, err
}

func (r *countingReadCloser) Close() error { return nil }

type fakeFileInfo struct{ size int64 }

func (f *fakeFileInfo) Name() string       { return "" }
func (f *fakeFileInfo) Size() int64        { return f.size }
func (f *fakeFileInfo) Mode() fs.FileMode  { return 0o644 }
func (f *fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f *fakeFileInfo) IsDir() bool        { return false }
func (f *fakeFileInfo) Sys() any           { return nil }

func mustReport(t *testing.T, fr FileReader, path string) *Frontmatter {
	t.Helper()
	yamlContent, err := readBoundedFrontmatter(fr, path)
	if err != nil {
		var werr *WarningError
		if errors.As(err, &werr) {
			t.Fatalf("unexpected warning for %s: %s", path, werr.Code)
		}
		t.Fatalf("read bounded frontmatter %s: %v", path, err)
	}
	meta, err := decodeFrontmatter(yamlContent)
	if err != nil {
		t.Fatalf("decode frontmatter %s: %v", path, err)
	}
	return meta
}

// TestManifestDoesNotParseBody proves S22: a multi-megabyte body is never read.
// Bytes streamed by the instrumented reader are bounded by the frontmatter plus
// at most one 4 KiB buffer prefetch.
func TestManifestDoesNotParseBody(t *testing.T) {
	body := strings.Repeat("B", 3<<20) // 3 MiB body
	content := "---\n" +
		"type: source\n" +
		"title: Big\n" +
		"---\n" +
		body

	fr := newCountingReader()
	fr.add("/repo/concepts/big.md", content)

	meta := mustReport(t, fr, "/repo/concepts/big.md")
	if meta.Title != "Big" || meta.Type != "source" {
		t.Fatalf("meta = %+v", meta)
	}

	frontmatterBytes := len("---\ntype: source\ntitle: Big\n---\n")
	limit := int64(frontmatterBytes + readBufferSize)
	if fr.bytesRead > limit {
		t.Fatalf("bytes read = %d, want <= frontmatter(%d) + one 4KiB buffer(%d); body was read",
			fr.bytesRead, frontmatterBytes, readBufferSize)
	}
}

// TestManifestHeaderLimit proves S23: a frontmatter header over 256 KiB is a
// stable bounded error and never streams to the end of a huge file.
func TestManifestHeaderLimit(t *testing.T) {
	longLine := strings.Repeat("k", maxFrontmatterBytes+100)
	content := "---\ntitle: " + longLine + "\n" // no closing delimiter, header grows past cap

	fr := newCountingReader()
	fr.add("/repo/bigheader.md", content)

	_, err := readBoundedFrontmatter(fr, "/repo/bigheader.md")
	var werr *WarningError
	if !errors.As(err, &werr) || werr.Code != CodeFrontmatterTooLarge {
		t.Fatalf("err = %v, want code %s", err, CodeFrontmatterTooLarge)
	}
	// The whole pathological file is maxFrontmatterBytes+small; the reader must
	// not have read beyond the cap plus one buffer.
	if fr.bytesRead > maxFrontmatterBytes+readBufferSize+64 {
		t.Fatalf("bytes read = %d, want bounded near %d", fr.bytesRead, maxFrontmatterBytes)
	}
}

// TestManifestMissingDelimiter proves S23: a file whose frontmatter never closes
// (or never opens) emits manifest_frontmatter_missing and scanning continues.
func TestManifestMissingDelimiter(t *testing.T) {
	cases := map[string]string{
		"no_open":    "type: source\ntitle: never opened\n",
		"no_close":   "---\ntype: source\ntitle: opened but never closed\n",
		"empty_body": "---\ntype: source\n---\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			fr := newCountingReader()
			path := "/repo/" + name + ".md"
			fr.add(path, content)
			_, err := readBoundedFrontmatter(fr, path)
			if name == "empty_body" {
				if err != nil {
					t.Fatalf("empty body with valid frontmatter should decode, got %v", err)
				}
				return
			}
			var werr *WarningError
			if !errors.As(err, &werr) || werr.Code != CodeFrontmatterMissing {
				t.Fatalf("err = %v, want code %s", err, CodeFrontmatterMissing)
			}
		})
	}
}

// TestManifestInvalidYAML proves S23: bounded YAML that is syntactically invalid
// emits manifest_frontmatter_invalid.
func TestManifestInvalidYAML(t *testing.T) {
	content := "---\n" +
		"type: [unclosed\n" +
		"  bad: : :\n" +
		"---\n" +
		"body\n"
	fr := newCountingReader()
	fr.add("/repo/badyaml.md", content)

	yamlContent, err := readBoundedFrontmatter(fr, "/repo/badyaml.md")
	if err != nil {
		t.Fatalf("bounded read should succeed for invalid YAML: %v", err)
	}
	if _, derr := decodeFrontmatter(yamlContent); derr == nil {
		t.Fatal("decode should fail on invalid YAML")
	}
}

// TestScanContinuesAfterWarnings proves one bad file omits itself but the good
// file is still reported, and reserved index.md/log.md are skipped.
func TestScanContinuesAfterWarnings(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"good.md":  "---\ntype: concept\ntitle: Good\n---\nbody\n",
		"bad.md":   "---\ntitle: no close\n",
		"index.md": "---\ntype: index\n---\n",
		"log.md":   "---\ntype: log\n---\n",
	}
	for name, body := range files {
		writeTmpFile(t, root, name, body)
	}

	reports, err := ScanKnowledgeFiles(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	byPath := map[string]FileReport{}
	for _, r := range reports {
		byPath[r.Path] = r
	}
	if _, ok := byPath["index.md"]; ok {
		t.Error("index.md should be skipped as reserved")
	}
	if _, ok := byPath["log.md"]; ok {
		t.Error("log.md should be skipped as reserved")
	}
	good := byPath["good.md"]
	if good.Warning != "" || good.Meta == nil || good.Meta.Title != "Good" {
		t.Fatalf("good.md report = %+v", good)
	}
	if good.Size != int64(len(files["good.md"])) {
		t.Fatalf("good.md size = %d, want %d", good.Size, len(files["good.md"]))
	}
	bad := byPath["bad.md"]
	if bad.Warning != CodeFrontmatterMissing || bad.Meta != nil {
		t.Fatalf("bad.md report = %+v", bad)
	}
}

// L5 (second-round review lock-in): hidden directories (e.g. .git) are not
// recursed into, while an uppercase .md suffix is still recognized as a concept.
func TestScanSkipsHiddenDirsAndUpperSuffix(t *testing.T) {
	root := t.TempDir()
	writeTmpFile(t, root, ".git/ignored.md", "---\ntype: concept\ntitle: Ignored\n---\nbody\n")
	writeTmpFile(t, root, "README.MD", "---\ntype: concept\ntitle: Readme Upper\n---\nbody\n")
	writeTmpFile(t, root, "real.md", "---\ntype: concept\ntitle: Real\n---\nbody\n")

	reports, err := ScanKnowledgeFiles(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	byPath := map[string]FileReport{}
	for _, r := range reports {
		byPath[r.Path] = r
	}
	if _, ok := byPath[".git/ignored.md"]; ok {
		t.Error(".git/ignored.md must be skipped (hidden dir not recursed)")
	}
	readme, ok := byPath["README.MD"]
	if !ok {
		t.Fatalf("README.MD must be recognized (case-insensitive .md); reports=%+v", reports)
	}
	if readme.Meta == nil || readme.Meta.Title != "Readme Upper" {
		t.Fatalf("README.MD report = %+v", readme)
	}
	if _, ok := byPath["real.md"]; !ok {
		t.Error("real.md should be scanned")
	}
}

func writeTmpFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
