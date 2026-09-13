// Package convert provides a unified, pure-Go document-to-Markdown
// conversion layer backed by downmark v0.10.0.
//
// It is a PRE-STAGE processor for okf's import pipeline: a converted
// document is standard Markdown (.md) and is fed to the existing
// SmartImportSource unchanged. The knowledge-base core model is never
// touched.
//
// Routing priority: archives > documents > markdown > other. .md files are
// NOT routed through this package (they use the existing pipeline).
package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/giraffesyo/downmark"
	"github.com/giraffesyo/downmark/all"
)

const (
	defaultMaxInputBytes = 64 << 20 // 64 MiB
	defaultTimeout       = 60 * time.Second
	resultLimitBytes     = 32 << 20 // downmark result budget (32 MiB)
)

// Sentinels. Callers match with errors.Is.
var (
	// ErrUnsupportedFormat is returned for a file whose extension is not a
	// supported document format (and is not an archive / markdown).
	ErrUnsupportedFormat = errors.New("convert: unsupported document format")
	// ErrInputTooLarge is returned when the input file exceeds
	// Options.MaxInputBytes before any conversion runs.
	ErrInputTooLarge = errors.New("convert: input exceeds size limit")
	// ErrNoText is returned when a PDF has no extractable text layer
	// (scanned images or unsupported fonts).
	ErrNoText = errors.New("convert: no extractable text; the PDF may be scanned images")
)

// Result is a single conversion outcome.
type Result struct {
	// Markdown is the converted body (no frontmatter).
	Markdown string
	// Title is the document title when the format provides one, else "".
	Title string
	// Warnings summarize what the conversion lost (from downmark's
	// structured []Warning, flattened to readable strings).
	Warnings []string
}

// Options control conversion behavior.
type Options struct {
	// MaxInputBytes is the hard input size limit checked before
	// conversion via os.Stat (default 64 MiB). <= 0 disables the check.
	MaxInputBytes int64
	// Timeout bounds a single conversion via context (default 60s).
	// < 0 means an immediately-cancelled context (used by tests).
	// == 0 means no timeout added (the caller's context is used as-is).
	Timeout time.Duration
}

// DefaultOptions returns the recommended defaults.
func DefaultOptions() *Options {
	return &Options{MaxInputBytes: defaultMaxInputBytes, Timeout: defaultTimeout}
}

// documentExts maps supported extensions (lowercased) to their type name.
var documentExts = map[string]string{
	".pdf": "pdf", ".docx": "docx", ".doc": "doc",
	".xlsx": "xlsx", ".pptx": "pptx",
	".html": "html", ".htm": "html",
	".csv": "csv", ".txt": "txt", ".text": "txt",
}

// IsSupportedDocument reports whether path is a "non-md document that must go
// through the conversion layer". NOTE: .md returns false — Markdown is NOT
// converted here and uses the existing pipeline.
func IsSupportedDocument(path string) bool {
	_, ok := documentExts[strings.ToLower(filepath.Ext(path))]
	return ok
}

// ChunkThreshold 是文档分块阈值：转换后 markdown 的 CJK-aware 词数超过它时，
// 导入方（cmd_add / MCP）应生成整文档概念 + 每块一个 <original>__cN.md
// 派生概念。小于等于该值的文档保持单概念（与既有行为完全一致）。
const ChunkThreshold = 2000

// Words returns the CJK-aware word count of text: whitespace-separated tokens
// count as words; each CJK character counts as one word. Used by callers to
// decide whether a converted document exceeds the chunking threshold.
func Words(text string) int { return countWords(text) }

// DocumentType returns the format type (pdf/docx/xlsx/pptx/html/csv/txt/doc),
// or "" when unsupported.
func DocumentType(path string) string {
	return documentExts[strings.ToLower(filepath.Ext(path))]
}

// ConvertToMarkdown converts the document at path to Markdown.
//
// Errors are recognizable via errors.Is: ErrUnsupportedFormat,
// ErrInputTooLarge, ErrNoText; conversion failures pass through downmark's
// errors (e.g. result-too-large).
func ConvertToMarkdown(ctx context.Context, path string, opts *Options) (*Result, error) {
	if opts == nil {
		opts = DefaultOptions()
	}
	if DocumentType(path) == "" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, path)
	}
	if opts.MaxInputBytes > 0 {
		st, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if st.Size() > opts.MaxInputBytes {
			return nil, fmt.Errorf("%w: %s (%d bytes)", ErrInputTooLarge, path, st.Size())
		}
	}
	switch {
	case opts.Timeout > 0:
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	case opts.Timeout < 0:
		// Immediately-cancelled context (deterministic timeout test).
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}
	ctx = downmark.WithResultLimit(ctx, resultLimitBytes)

	res, err := all.ConvertFile(ctx, path)
	if err != nil {
		if isNoTextError(err) {
			return nil, fmt.Errorf("%w: %v", ErrNoText, err)
		}
		return nil, err
	}
	return &Result{
		Markdown: res.Markdown,
		Title:    res.Title,
		Warnings: summarizeWarnings(res.Warnings),
	}, nil
}

// summarizeWarnings flattens downmark's structured []Warning into readable
// strings ("converter: location: err").
func summarizeWarnings(ws []downmark.Warning) []string {
	if len(ws) == 0 {
		return nil
	}
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Error())
	}
	return out
}

// WrapConcept builds a full OKF concept document (frontmatter + body) from a
// converted document. Shared by cmd_add and the MCP import tool so the
// frontmatter format stays single-sourced. Emits the okf.document generation
// marker so generated-file lifecycle (okf sync -prune, incremental updates)
// can identify and remove the product without touching author-owned files.
//
// okfID, when non-empty, is written as the stable okf_id extension. The
// conversion/staging path passes empty so temporary artifacts never receive a
// random id; only the final owned destination supplies it.
func WrapConcept(title, filename, format, ctype, body, okfID string) string {
	desc := fmt.Sprintf("Converted from %s (via %s)", filename, format)
	idLine := ""
	if okfID != "" {
		idLine = fmt.Sprintf("okf_id: %q\n", okfID)
	}
	return fmt.Sprintf("---\ntype: %s\ntitle: %q\ndescription: %q\n%sgenerated: true\ngenerator: %q\nsource_path: %q\n---\n%s\n",
		ctype, title, desc, idLine, generatorName, filename, body)
}

// generatorName identifies products of the document-conversion import path.
// hasTrustedGeneratedMetadata (pkg/okf/prune_files.go) accepts it alongside
// the git generator's "okf.git".
const generatorName = "okf.document"

// WrapChunkConcept builds a chunk-concept document (frontmatter + body) for a
// chunk of a converted document. The chunk title is written explicitly at
// import time (never filename-derived); custom fields carry chunk_index,
// chunk_count, source_path, derived:true, the okf.document generation marker
// and heading_path (when present) so dedupe and sync lifecycle can identify
// derived artifacts. parentOKFID, when non-empty, records the parent concept's
// stable okf_id under parent_okf_id.
func WrapChunkConcept(title, filename, format, sourcePath string, index, count int, headingPath, body, parentOKFID string) string {
	desc := fmt.Sprintf("Converted from %s (via %s)", filename, format)
	var sb strings.Builder
	fmt.Fprintf(&sb, "---\ntype: source\ntitle: %q\ndescription: %q\n", title, desc)
	fmt.Fprintf(&sb, "generated: true\ngenerator: %q\n", generatorName)
	fmt.Fprintf(&sb, "chunk_index: %d\nchunk_count: %d\n", index, count)
	fmt.Fprintf(&sb, "source_path: %q\nderived: \"true\"\n", sourcePath)
	if parentOKFID != "" {
		fmt.Fprintf(&sb, "parent_okf_id: %q\n", parentOKFID)
	}
	if headingPath != "" {
		fmt.Fprintf(&sb, "heading_path: %q\n", headingPath)
	}
	sb.WriteString("---\n")
	sb.WriteString(body)
	sb.WriteString("\n")
	return sb.String()
}

// isNoTextError reports whether err is downmark's scanned/blank-PDF error,
// which is wrapped in a ConversionError of the "pdf" attempt.
func isNoTextError(err error) bool {
	var ce *downmark.ConversionError
	if errors.As(err, &ce) {
		for _, a := range ce.Attempts {
			if strings.Contains(a.Error(), "no extractable text") {
				return true
			}
		}
	}
	return strings.Contains(err.Error(), "no extractable text")
}
