package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
//   okf add -dir=./kb file.md     OR   okf add file.md -dir=./kb
func cmdAdd(args []string) int {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Println("Usage: okf add [options] <path>")
		fmt.Println("")
		fmt.Println("Import files, directories, or archives into the knowledge base.")
		fmt.Println("Supports smart change detection and multiple merge strategies.")
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

	// Build SmartImporter
	importer := okf.NewSmartImporter(idx, kbDir)

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

	// Collect source files
	info, err := os.Stat(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: source path not accessible: %v\n", err)
		return 1
	}

	var sourceFiles []string
	if info.IsDir() {
		sourceFiles, err = collectMarkdownFiles(srcPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to collect files: %v\n", err)
			return 1
		}
	} else {
		sourceFiles = []string{srcPath}
	}

	if len(sourceFiles) == 0 {
		if !*silent {
			fmt.Println("No markdown files found.")
		}
		return 0
	}

	// Iterate and import
	var totalFound, imported, skipped, failed int

	for _, src := range sourceFiles {
		relTarget := computeTargetPath(src, srcPath, info.IsDir())

		if smartOpts.DetectOnly {
			reports, rerr := importer.DetectChanges([]string{src}, nil)
			if rerr != nil {
				failed++
				continue
			}
			totalFound++
			if len(reports) > 0 {
				r := reports[0]
				if r.Result == okf.DetectNoChange {
					skipped++
				} else {
					imported++
				}
				if !*silent {
					fmt.Printf("  [%s] %s\n", r.Result, src)
				}
			}
			continue
		}

		// Normal smart import flow
		result, ierr := importer.ImportFile(src, relTarget, smartOpts)
		totalFound++
		if ierr != nil {
			failed++
			continue
		}
		if result != nil && result.Changed {
			imported++
			if !*silent {
				fmt.Printf("  [%-9s] %s\n", result.Strategy, src)
			}
		} else {
			skipped++
		}
	}

	// Persist metadata index (only if modifications were made and not dry-run/detect-only)
	if !smartOpts.DetectOnly && !smartOpts.HashOnly && totalFound > 0 {
		if serr := idx.Save(metaPath); serr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save metadata: %v\n", serr)
			return 1
		}
	}

	// Report results
	if !*silent {
		fmt.Println("")
		fmt.Println("Import summary:")
		fmt.Printf("  Total files found: %d\n", totalFound)
		fmt.Printf("  Imported: %d\n", imported)
		fmt.Printf("  Skipped: %d\n", skipped)
		fmt.Printf("  Failed: %d\n", failed)
	}

	if failed > 0 {
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

// collectMarkdownFiles recursively collects all markdown files under root.
func collectMarkdownFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// computeTargetPath computes target path in knowledge base relative to srcRoot.
func computeTargetPath(src, srcRoot string, rootIsDir bool) string {
	if !rootIsDir {
		return filepath.Base(src)
	}
	rel, err := filepath.Rel(srcRoot, src)
	if err != nil {
		return filepath.Base(src)
	}
	return rel
}

// =============================================================================
// flag reordering helper - allows positional args and flags to be mixed
// =============================================================================

// reorderFlags reorganizes args so that all recognized flags come first,
// and positional args come last. boolFlags specifies which flag names are
// booleans (they don't consume the following token as a value).
// This enables users to type either:
//   okf add -dir=./kb file.md    OR   okf add file.md -dir=./kb
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
