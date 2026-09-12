// Package manifest implements the metadata-only knowledge Manifest: it walks
// knowledge-directory .md files, decodes only their YAML frontmatter under a
// strict byte budget, and never reads Markdown bodies, loads embeddings or
// touches the vector index.
//
// Package boundary (design §4.3, §8): manifest may depend on pkg/identity
// (registry / duplicate check) and pkg/okf (reserved filename helpers). It
// MUST NOT depend on pkg/query. It calls no parser.ParseConcept, because that
// reads the whole file.
package manifest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/okf"
	"gopkg.in/yaml.v3"
)

// readBufferSize is the fixed 4 KiB read window used while scanning for the
// closing frontmatter delimiter. Per S22, bytes read past the delimiter are
// bounded by at most one such buffer prefetch.
const readBufferSize = 4 * 1024

// maxFrontmatterBytes caps the frontmatter section (between the opening and
// closing delimiter). A header exceeding this is a stable, bounded error
// rather than an unbounded read of a multi-megabyte body.
const maxFrontmatterBytes = 256 * 1024

// Stable per-file warning codes (spec S23). These omit only that one file;
// scanning continues for every other file.
const (
	CodeFrontmatterMissing  = "manifest_frontmatter_missing"
	CodeFrontmatterTooLarge = "manifest_frontmatter_too_large"
	CodeFrontmatterInvalid  = "manifest_frontmatter_invalid"
)

// FileReader abstracts file open/stat so the bounded reader can be tested with
// an instrumented fake that counts bytes read. Production uses OSFileReader.
type FileReader interface {
	Open(path string) (io.ReadCloser, error)
	Stat(path string) (fs.FileInfo, error)
}

// OSFileReader is the production FileReader backed by the local filesystem.
func OSFileReader() FileReader { return osFileReader{} }

type osFileReader struct{}

func (osFileReader) Open(path string) (io.ReadCloser, error) { return os.Open(path) }
func (osFileReader) Stat(path string) (fs.FileInfo, error)   { return os.Stat(path) }

// WarningError is a non-fatal, per-file scanning problem carrying a stable
// code. It omits only the offending file; the overall scan keeps going.
type WarningError struct {
	Code string
	Path string
	Err  error
}

func (e *WarningError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Path, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Path)
}

func (e *WarningError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// SourceRef is the metadata subset of an OKF source entry surfaced in a
// ManifestItem. It never carries source body content.
type SourceRef struct {
	Resource string `yaml:"resource"`
	Title    string `yaml:"title,omitempty"`
}

// GeneratedMeta mirrors OKF generated.{by,at}.
type GeneratedMeta struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"`
}

// VerifiedMeta mirrors OKF verified[].{by,at} after flexible normalization.
type VerifiedMeta struct {
	By string `yaml:"by"`
	At string `yaml:"at,omitempty"`
}

// Frontmatter is the decoded, metadata-only view of one concept file. No body
// content is ever populated here.
type Frontmatter struct {
	Type        string
	Title       string
	Description string
	Tags        []string
	Status      string
	StaleAfter  string
	Sources     []SourceRef
	Generated   *GeneratedMeta
	Verified    []VerifiedMeta
	OKFID       string
	ParentOKFID string
	SourcePath  string
}

// rawFrontmatter is the YAML decode target. Verified stays interface{} so a
// bare mapping {by,at} normalizes the same as a list (mirrors parser leniency).
type rawFrontmatter struct {
	Type        string         `yaml:"type"`
	Title       string         `yaml:"title,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Status      string         `yaml:"status,omitempty"`
	StaleAfter  string         `yaml:"stale_after,omitempty"`
	Sources     []SourceRef    `yaml:"sources,omitempty"`
	Generated   *GeneratedMeta `yaml:"generated,omitempty"`
	Verified    interface{}    `yaml:"verified,omitempty"`
	Custom      map[string]any `yaml:",inline"`
}

// FileReport is one scanned concept file. Warning is empty on success; a
// non-empty Warning is one of the manifest_frontmatter_* codes and means Meta
// is nil and the file was omitted.
type FileReport struct {
	Path    string // slash-normalized, relative to the knowledge root
	Size    int64  // file size in bytes, from Stat
	Meta    *Frontmatter
	Warning string
}

// ScanKnowledgeFiles walks root recursively, decodes only the bounded
// frontmatter of each .md concept file, and returns one FileReport per file.
// Per-file frontmatter problems are reported as Warning codes and never abort
// the scan. Reserved index.md/log.md files are skipped (they are not concepts).
// A nil fr uses OSFileReader.
func ScanKnowledgeFiles(ctx context.Context, root string, fr FileReader) ([]FileReport, error) {
	if fr == nil {
		fr = OSFileReader()
	}
	var reports []FileReport
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if d.IsDir() {
			// Skip hidden directories (e.g. .git, .okf internals) without
			// recursing into them, mirroring identity.collectConceptFiles.
			// Never skip the root itself.
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		// Case-insensitive .md match, consistent with collectConceptFiles.
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		if okf.IsIndexFile(path) || okf.IsLogFile(path) {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		report := FileReport{Path: filepath.ToSlash(rel)}
		if info, serr := fr.Stat(path); serr == nil {
			report.Size = info.Size()
		}
		yamlContent, rerr := readBoundedFrontmatter(fr, path)
		if rerr != nil {
			var werr *WarningError
			if errors.As(rerr, &werr) {
				report.Warning = werr.Code
				reports = append(reports, report)
				return nil
			}
			// A non-frontmatter IO failure (e.g. permission): omit the file
			// without inventing a warning code.
			return nil
		}
		meta, derr := decodeFrontmatter(yamlContent)
		if derr != nil {
			report.Warning = CodeFrontmatterInvalid
			reports = append(reports, report)
			return nil
		}
		report.Meta = meta
		reports = append(reports, report)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reports, nil
}

// minOpeningCheck is the smallest byte count after which a missing opening
// delimiter is conclusive: "---\n" is 4 bytes, "---\r\n" is 5.
const minOpeningCheck = 4

var (
	errNoOpeningDelimiter = errors.New("no opening frontmatter delimiter")
	errNoClosingDelimiter = errors.New("no closing frontmatter delimiter")
	errExceedsLimit       = errors.New("frontmatter exceeds 256 KiB")
)

// readBoundedFrontmatter opens path via fr, reads at most one 4 KiB buffer at a
// time, stops at the second frontmatter delimiter, and returns the raw YAML
// content between the delimiters. It never reads the Markdown body.
//
// Error classification:
//   - no opening/closing delimiter   → *WarningError{CodeFrontmatterMissing}
//   - frontmatter content > 256 KiB → *WarningError{CodeFrontmatterTooLarge}
//   - bounded YAML decode failure    → returned by the caller via decodeFrontmatter
func readBoundedFrontmatter(fr FileReader, path string) ([]byte, error) {
	rc, err := fr.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer rc.Close()

	var head []byte
	buf := make([]byte, readBufferSize)
	openingLen := 0
	confirmed := false

	for {
		n, rerr := rc.Read(buf)
		if n > 0 {
			head = append(head, buf[:n]...)

			if !confirmed {
				if l := openingDelimLen(head); l > 0 {
					confirmed = true
					openingLen = l
				} else if len(head) >= minOpeningCheck {
					return nil, &WarningError{Code: CodeFrontmatterMissing, Path: path, Err: errNoOpeningDelimiter}
				}
			}
			if confirmed {
				if idx := closingDelimIndex(head[openingLen:]); idx >= 0 {
					return append([]byte(nil), head[openingLen:openingLen+idx]...), nil
				}
				if contentLen := len(head) - openingLen; contentLen > maxFrontmatterBytes {
					return nil, &WarningError{Code: CodeFrontmatterTooLarge, Path: path, Err: errExceedsLimit}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, fmt.Errorf("read: %w", rerr)
		}
	}

	if !confirmed {
		return nil, &WarningError{Code: CodeFrontmatterMissing, Path: path, Err: errNoOpeningDelimiter}
	}
	return nil, &WarningError{Code: CodeFrontmatterMissing, Path: path, Err: errNoClosingDelimiter}
}

// openingDelimLen returns the opening delimiter length (4 for "---\n", 5 for
// "---\r\n"), or 0 if head cannot yet be classified / does not open frontmatter.
func openingDelimLen(head []byte) int {
	switch {
	case bytes.HasPrefix(head, []byte("---\r\n")):
		return 5
	case bytes.HasPrefix(head, []byte("---\n")):
		return 4
	default:
		return 0
	}
}

// closingDelimIndex mirrors parser.findFrontmatterEnd but relative to a region
// that starts immediately after the opening delimiter newline. It returns the
// offset of a "---" run followed by EOF or a line break, or -1.
func closingDelimIndex(region []byte) int {
	for i := 0; i <= len(region)-3; i++ {
		if region[i] == '-' && region[i+1] == '-' && region[i+2] == '-' {
			next := i + 3
			if next >= len(region) || region[next] == '\n' || region[next] == '\r' {
				return i
			}
		}
	}
	return -1
}

// decodeFrontmatter unmarshals the already-bounded YAML content into metadata.
func decodeFrontmatter(content []byte) (*Frontmatter, error) {
	var raw rawFrontmatter
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	fm := &Frontmatter{
		Type:        raw.Type,
		Title:       raw.Title,
		Description: raw.Description,
		Tags:        cloneStrings(raw.Tags),
		Status:      raw.Status,
		StaleAfter:  raw.StaleAfter,
		Sources:     raw.Sources,
		Generated:   raw.Generated,
		Verified:    normalizeVerified(raw.Verified),
	}
	if raw.Custom != nil {
		fm.OKFID, _ = raw.Custom["okf_id"].(string)
		fm.ParentOKFID, _ = raw.Custom["parent_okf_id"].(string)
		fm.SourcePath, _ = raw.Custom["source_path"].(string)
	}
	return fm, nil
}

// normalizeVerified accepts nil, a single {by,at} mapping, or a list of
// mappings and returns a clean slice. Anything unrecognized yields nil.
func normalizeVerified(v interface{}) []VerifiedMeta {
	switch typed := v.(type) {
	case nil:
		return nil
	case []interface{}:
		out := make([]VerifiedMeta, 0, len(typed))
		for _, item := range typed {
			if meta, ok := verifiedMetaFromMap(item); ok {
				out = append(out, meta)
			}
		}
		return out
	case map[string]interface{}:
		if meta, ok := verifiedMetaFromMap(typed); ok {
			return []VerifiedMeta{meta}
		}
	}
	return nil
}

func verifiedMetaFromMap(v interface{}) (VerifiedMeta, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return VerifiedMeta{}, false
	}
	return VerifiedMeta{By: scalarString(m["by"]), At: scalarString(m["at"])}, true
}

// scalarString renders a YAML scalar as a string. yaml.v3 decodes timestamp
// values into time.Time, so those are normalized to RFC3339 rather than being
// silently dropped by a strict string type assertion.
func scalarString(v interface{}) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	case time.Time:
		return typed.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
