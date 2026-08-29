package main

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/superops-team/okf/pkg/convert"
	okf "github.com/superops-team/okf/pkg/okf"
)

// =============================================================================
// okf add
// =============================================================================

// cmdAdd handles the "okf add" command.
//
// Note: Go's flag package stops parsing at the first non-flag argument.
// To allow flexibility, we pre-scan args and reorder them so all recognized
// flags come first, preserving the original flag values (so both of these
// work):
//
//	okf add -dir=./kb file.md     OR   okf add file.md -dir=./kb
func cmdAdd(args []string) int {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Println("Usage: okf add [options] <path>")
		fmt.Println("")
		fmt.Println("Import files, directories, or archives into the knowledge base.")
		fmt.Println("Supports smart change detection and multiple merge strategies.")
		fmt.Println("Documents (PDF/DOCX/XLSX/PPTX/HTML/CSV/TXT/DOC) are auto-converted to Markdown.")
		fmt.Println("")
		fmt.Println("Options:")
		flags.PrintDefaults()
	}

	var (
		dirFlag     = flags.String("dir", "", "Knowledge base directory")
		forceFlag   = flags.Bool("force", false, "Overwrite existing files (equivalent to -strategy=overwrite)")
		dryRun      = flags.Bool("dry-run", false, "Show what would be imported without making changes")
		silent      = flags.Bool("silent", false, "Suppress informational output")
		strategy    = flags.String("strategy", "", "Merge strategy when file content changed: skip|overwrite|merge|patch")
		patchFields = flags.String("patch-fields", "", "Comma-separated frontmatter fields for 'patch' strategy (default: title,description,tags)")
		detectOnly  = flags.Bool("detect-only", false, "Only detect changes, do not perform import")
	)

	// Build the set of boolean flag names (these take no value)
	boolFlags := map[string]bool{
		"force":       true,
		"dry-run":     true,
		"silent":      true,
		"detect-only": true,
	}

	// Reorder: pull positional args to the end so flag.Parse works
	// regardless of whether user places flags before or after the path.
	reordered := reorderFlags(args, flags, boolFlags)

	if err := flags.Parse(reordered); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	// Now get path from flag.Args()
	pathArgs := flags.Args()
	if len(pathArgs) < 1 {
		fmt.Fprintln(os.Stderr, "Error: no path specified")
		flags.Usage()
		return 1
	}
	srcPath := pathArgs[0]

	// Resolve knowledge base directory
	kbDir, err := okf.ResolveKnowledgeDir(*dirFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to resolve knowledge directory: %v\n", err)
		return 1
	}

	// Ensure directory exists
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create knowledge directory: %v\n", err)
		return 1
	}

	// Validate and build smart import options
	smartOpts, err := buildSmartImportOptions(*strategy, *patchFields, *forceFlag, *detectOnly, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Load metadata index
	metaPath := okf.KnowledgeMetadataPath(kbDir)
	idx := okf.NewMetadataIndex()
	if err := idx.Load(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: metadata file corrupted, starting fresh: %v\n", err)
		idx = okf.NewMetadataIndex()
	}

	// Show what we're doing
	if !*silent {
		fmt.Printf("Importing from: %s\n", srcPath)
		fmt.Printf("To knowledge base: %s\n", kbDir)
		if smartOpts.ForceStrategy != "" {
			fmt.Printf("Strategy: %s\n", smartOpts.ForceStrategy)
		}
		if smartOpts.DetectOnly {
			fmt.Println("Mode: detect-only (no changes will be made)")
		}
	}

	// Pre-stage document conversion (PDF/DOCX/XLSX/PPTX/HTML/CSV/TXT/DOC).
	// Only when the source actually contains documents do we aggregate into a
	// deterministic staging dir; pure-markdown imports keep the existing path
	// untouched (zero behavior change).
	importSource := srcPath
	convertedCount := 0
	failedCount := 0
	stagingDir, convertedCount, failedCount, cleanup, err := convertAndStageDocuments(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if cleanup != nil {
		defer cleanup()
	}
	if stagingDir != "" {
		importSource = stagingDir
	}
	// If the source contained documents but none could be converted, that is
	// a hard failure (all-or-nothing for a single document / all-failed batch),
	// not a silent "no markdown found". Dry-run keeps the preview exit code 0.
	if failedCount > 0 && convertedCount == 0 && !smartOpts.DetectOnly {
		fmt.Fprintf(os.Stderr, "Error: %d document(s) could not be converted\n", failedCount)
		return 1
	}
	result, err := okf.SmartImportSource(importSource, kbDir, idx, smartOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if result.TotalFiles == 0 {
		if !*silent {
			fmt.Println("No markdown files found.")
		}
		return 0
	}

	// Persist metadata index (only if modifications were made and not dry-run/detect-only)
	if !smartOpts.DetectOnly && !smartOpts.HashOnly && result.TotalFiles > 0 {
		if serr := idx.Save(metaPath); serr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save metadata: %v\n", serr)
			return 1
		}
	}

	// Report results
	if !*silent {
		fmt.Println("")
		fmt.Println("Import summary:")
		fmt.Printf("  Total files found: %d\n", result.TotalFiles)
		fmt.Printf("  Imported: %d\n", result.ImportedFiles)
		fmt.Printf("  Skipped: %d\n", result.SkippedFiles)
		fmt.Printf("  Failed: %d\n", result.FailedFiles)
		if convertedCount > 0 {
			fmt.Printf("  Converted (documents): %d\n", convertedCount)
		}
		if failedCount > 0 {
			fmt.Printf("  Failed (documents): %d\n", failedCount)
		}
	}

	if result.FailedFiles > 0 {
		return 1
	}

	return 0
}

// buildSmartImportOptions validates strategy/patch-fields and builds options
func buildSmartImportOptions(strategy, patchFields string, force, detectOnly, dryRun bool) (*okf.SmartImportOptions, error) {
	opts := &okf.SmartImportOptions{
		DetectOnly: detectOnly,
		HashOnly:   false,
	}

	// strategy mapping
	var mappedStrategy okf.MergeStrategy
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "":
		mappedStrategy = "" // let DecideStrategy use meta default or skip
	case string(okf.StrategySkip):
		mappedStrategy = okf.StrategySkip
	case string(okf.StrategyOverwrite):
		mappedStrategy = okf.StrategyOverwrite
	case string(okf.StrategyMerge):
		mappedStrategy = okf.StrategyMerge
	case string(okf.StrategyPatch):
		mappedStrategy = okf.StrategyPatch
	default:
		return nil, fmt.Errorf("invalid strategy %q: must be one of skip|overwrite|merge|patch", strategy)
	}

	// -force is a shorthand for -strategy=overwrite
	if force && mappedStrategy == "" {
		mappedStrategy = okf.StrategyOverwrite
	}

	if dryRun {
		opts.DetectOnly = true
	}

	opts.ForceStrategy = mappedStrategy

	// Parse patch-fields (comma-separated)
	patchFields = strings.TrimSpace(patchFields)
	if patchFields != "" {
		fields := strings.Split(patchFields, ",")
		cleaned := make([]string, 0, len(fields))
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if f != "" {
				cleaned = append(cleaned, f)
			}
		}
		if len(cleaned) > 0 {
			opts.PatchFields = cleaned
		}
	}

	return opts, nil
}

// =============================================================================
// flag reordering helper - allows positional args and flags to be mixed
// =============================================================================

// reorderFlags reorganizes args so that all recognized flags come first,
// and positional args come last. boolFlags specifies which flag names are
// booleans (they don't consume the following token as a value).
// This enables users to type either:
//
//	okf add -dir=./kb file.md    OR   okf add file.md -dir=./kb
func reorderFlags(args []string, fs *flag.FlagSet, boolFlags map[string]bool) []string {
	var flagsList, positional []string
	registered := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		registered[f.Name] = true
	})

	i := 0
	for i < len(args) {
		a := args[i]
		if len(a) >= 2 && a[0] == '-' {
			name := strings.TrimLeft(a, "-")
			hasValue := false
			if idx := strings.Index(name, "="); idx >= 0 {
				name = name[:idx]
				hasValue = true
			}
			if registered[name] {
				flagsList = append(flagsList, a)
				// Non-boolean flag consumes the next arg as its value (unless
				// the value was provided inline via '=').
				if !hasValue && !boolFlags[name] {
					if i+1 < len(args) {
						flagsList = append(flagsList, args[i+1])
						i++
					}
				}
				i++
				continue
			}
			// Unrecognized flag - treat as positional (e.g. filename starting with "-")
			positional = append(positional, a)
			i++
			continue
		}
		positional = append(positional, a)
		i++
	}
	return append(flagsList, positional...)
}

// =============================================================================
// Document pre-stage conversion (P2)
// =============================================================================

// convertAndStageDocuments aggregates every file to import — existing .md files
// plus document-to-markdown conversion outputs — into a single deterministic
// staging directory that the existing SmartImportSource can consume.
//
//   - stagingDir == "" means the source contains no documents: callers keep the
//     original path (pure-markdown behavior is untouched).
//   - The staging path is derived from srcPath (sha1), so the SourcePath used
//     by smart-import change detection stays stable across runs (OKF S20).
//   - cleanup() removes the staging dir; always defer it (OKF S19).
func convertAndStageDocuments(srcPath string) (stagingDir string, convertedCount, failedCount int, cleanup func(), err error) {
	cleanup = func() {}
	root := srcPath
	rootIsDir := false

	// Fast path: no archive and no document at all -> leave the existing
	// pipeline untouched.
	if !okf.IsArchive(srcPath) {
		info, serr := os.Stat(srcPath)
		if serr != nil {
			return "", 0, 0, cleanup, serr
		}
		rootIsDir = info.IsDir()
		if !rootIsDir && !convert.IsSupportedDocument(srcPath) {
			return "", 0, 0, cleanup, nil // single markdown file
		}
		if rootIsDir {
			docs, werr := walkDocuments(srcPath)
			if werr != nil {
				return "", 0, 0, cleanup, werr
			}
			if len(docs) == 0 {
				return "", 0, 0, cleanup, nil // pure-markdown directory
			}
		}
	}

	// Deterministic staging dir keyed by the source path (stable identity).
	sum := sha1.Sum([]byte(srcPath))
	stagingDir = filepath.Join(os.TempDir(), "okf-convert", hex.EncodeToString(sum[:]))
	cleanup = func() { _ = os.RemoveAll(stagingDir) }
	if rerr := os.RemoveAll(stagingDir); rerr != nil {
		return "", 0, 0, cleanup, rerr
	}
	if merr := os.MkdirAll(stagingDir, 0o755); merr != nil {
		return "", 0, 0, cleanup, merr
	}

	// Archives are extracted into the staging root; their .md members already
	// live under staging, so no copying is needed below.
	if okf.IsArchive(srcPath) {
		// Full extraction (unlike okf.ExtractArchive, which keeps only .md
		// members) so document members can be converted below.
		if aerr := extractArchiveFull(srcPath, stagingDir); aerr != nil {
			return "", 0, 0, cleanup, aerr
		}
		root = stagingDir
		rootIsDir = true
	}

	// Copy existing markdown files into staging (preserving relative layout)
	// so SmartImportSource sees one aggregated tree.
	mdFiles, cerr := okf.CollectFiles(root)
	if cerr != nil {
		return "", 0, 0, cleanup, cerr
	}
	for _, md := range mdFiles {
		if root == stagingDir {
			continue // already inside staging (archive case)
		}
		rel := relativePath(root, md, rootIsDir)
		if cerr := copyFile(md, filepath.Join(stagingDir, rel)); cerr != nil {
			return "", 0, 0, cleanup, cerr
		}
	}

	// Convert supported documents to Markdown and stage the <original>.md.
	docs, derr := walkDocuments(root)
	if derr != nil {
		return "", 0, 0, cleanup, derr
	}
	for _, doc := range docs {
		res, xerr := convert.ConvertToMarkdown(context.Background(), doc, nil)
		if xerr != nil {
			fmt.Fprintf(os.Stderr, "  skip document %s: %v\n", doc, xerr)
			failedCount++ // a single failure must not abort the batch (OKF S15)
			continue
		}
		rel := relativePath(root, doc, rootIsDir)
		title := res.Title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(doc), filepath.Ext(doc))
		}
		out := filepath.Join(stagingDir, rel+".md")
		if merr := os.MkdirAll(filepath.Dir(out), 0o755); merr != nil {
			return "", 0, 0, cleanup, merr
		}
		body := wrapFrontmatter(title, filepath.Base(doc), convert.DocumentType(doc), res.Markdown)
		if werr := os.WriteFile(out, []byte(body), 0o644); werr != nil {
			return "", 0, 0, cleanup, werr
		}
		convertedCount++
	}
	return stagingDir, convertedCount, failedCount, cleanup, nil
}

// walkDocuments recursively collects every convertible document under root.
func walkDocuments(root string) ([]string, error) {
	var docs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.IsDir() {
			return nil
		}
		if convert.IsSupportedDocument(path) {
			docs = append(docs, path)
		}
		return nil
	})
	return docs, err
}

// copyFile copies src to dst, creating parent directories.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// extractArchiveFull extracts every member of an archive into destDir,
// preserving relative layout, with zip-slip / symlink / size guards.
// Unlike okf.ExtractArchive (markdown-only), it keeps all members so that
// document conversion can pick them up (OKF S26).
func extractArchiveFull(archivePath, destDir string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return err
	}
	if info.Size() > okf.MaxArchiveSize {
		return fmt.Errorf("archive exceeds maximum size limit of %d bytes", okf.MaxArchiveSize)
	}
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractZipFull(archivePath, destDir)
	case strings.HasSuffix(lower, ".tar.bz2"):
		return extractTarFull(archivePath, destDir, true)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tar"):
		return extractTarFull(archivePath, destDir, false)
	case strings.HasSuffix(lower, ".tar.xz"):
		return fmt.Errorf("unsupported archive format: %s (.tar.xz is not supported; use .tar.gz or .zip)", archivePath)
	default:
		return fmt.Errorf("unsupported archive format: %s", archivePath)
	}
}

func extractZipFull(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink entry rejected: %s", f.Name)
		}
		if f.UncompressedSize64 > okf.MaxFileSize {
			return fmt.Errorf("member exceeds size limit: %s", f.Name)
		}
		if err := safeArchiveTarget(destDir, f.Name); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		err = writeExtracted(src, filepath.Join(destDir, filepath.Clean(f.Name)))
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarFull(archivePath, destDir string, bz2 bool) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(strings.ToLower(archivePath), ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	} else if bz2 {
		r = bzip2.NewReader(f)
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			return fmt.Errorf("link entry rejected: %s", hdr.Name)
		}
		if hdr.Size > okf.MaxFileSize {
			return fmt.Errorf("member exceeds size limit: %s", hdr.Name)
		}
		if err := safeArchiveTarget(destDir, hdr.Name); err != nil {
			return err
		}
		if err := writeExtracted(tr, filepath.Join(destDir, filepath.Clean(hdr.Name))); err != nil {
			return err
		}
	}
	return nil
}

// writeExtracted writes an archive member to dst, creating parents.
// It uses an io.LimitedReader as a second guard: even if an archive header
// lies about (or omits) the member size, decompression can never exceed
// okf.MaxFileSize bytes.
func writeExtracted(r io.Reader, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	n, err := io.Copy(out, &io.LimitedReader{R: r, N: okf.MaxFileSize + 1})
	if err != nil {
		return err
	}
	if n > okf.MaxFileSize {
		return fmt.Errorf("member exceeds size limit after decompression: %s", filepath.Base(dst))
	}
	return nil
}

// safeArchiveTarget rejects entries that escape destDir (zip-slip) or use
// absolute paths.
func safeArchiveTarget(destDir, name string) error {
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("archive entry escapes destination: %s", name)
	}
	if filepath.IsAbs(clean) {
		return fmt.Errorf("archive entry has absolute path: %s", name)
	}
	return nil
}

// relativePath computes the destination-relative path of a collected file.
// A single-file root has no directory structure: its relative path is just
// the base name (filepath.Rel(file, file) would wrongly return ".").
func relativePath(root, path string, rootIsDir bool) string {
	if !rootIsDir {
		return filepath.Base(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return rel
}

// wrapFrontmatter builds a full OKF concept body from converted Markdown.
// Delegates to convert.WrapConcept so cmd_add and the MCP tool share one
// frontmatter format.
func wrapFrontmatter(title, filename, format, body string) string {
	return convert.WrapConcept(title, filename, format, "source", body)
}
