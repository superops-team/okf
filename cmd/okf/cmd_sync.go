package main

import (
	"flag"
	"fmt"
	"os"

	okf "github.com/superops-team/okf/pkg/okf"
)

// =============================================================================
// okf sync
// =============================================================================

func cmdSync(args []string) int {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Println("Usage: okf sync [options]")
		fmt.Println("")
		fmt.Println("Synchronize all indexed files: re-detect changes and apply strategies.")
		fmt.Println("Useful when source files may have been updated externally.")
		fmt.Println("")
		fmt.Println("Options:")
		flags.PrintDefaults()
	}

	var (
		dirFlag    = flags.String("dir", "", "Knowledge base directory")
		pruneFlag  = flags.Bool("prune", false, "Remove entries for missing source files")
		silent     = flags.Bool("silent", false, "Suppress informational output")
		detectOnly = flags.Bool("detect-only", false, "Only detect, do not apply changes")
	)

	// Allow flags and positional args to be mixed
	boolFlags := map[string]bool{"prune": true, "silent": true, "detect-only": true}
	reordered := reorderFlags(args, flags, boolFlags)

	if err := flags.Parse(reordered); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	kbDir, err := okf.ResolveKnowledgeDir(*dirFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	metaPath := okf.KnowledgeMetadataPath(kbDir)
	idx := okf.NewMetadataIndex()
	if err := idx.Load(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: metadata file is corrupted: %v\n", err)
		return 1
	}

	if idx.Len() == 0 {
		if !*silent {
			fmt.Println("No indexed files. Use 'okf add' first.")
		}
		return 0
	}

	if *detectOnly {
		return syncDetectOnly(idx, kbDir, *silent)
	}

	importer := okf.NewSmartImporter(idx, kbDir)
	synced, skipped, errors, err := importer.Sync(*pruneFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: sync failed: %v\n", err)
		return 1
	}

	// save metadata index back
	if err := idx.Save(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save metadata: %v\n", err)
		return 1
	}

	if !*silent {
		fmt.Println("Sync complete:")
		fmt.Printf("  Updated: %d\n", synced)
		fmt.Printf("  Skipped: %d\n", skipped)
		fmt.Printf("  Errors:  %d\n", errors)
	}

	return 0
}

// syncDetectOnly reports changes for each indexed source file.
func syncDetectOnly(idx *okf.MetadataIndex, kbDir string, silent bool) int {
	var sources []string
	idx.RangeFiles(func(target string, meta *okf.FileMetadata) bool {
		if meta != nil && meta.SourcePath != "" {
			sources = append(sources, meta.SourcePath)
		}
		return true
	})

	importer := okf.NewSmartImporter(idx, kbDir)
	reports, err := importer.DetectChanges(sources, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if !silent {
		changed := 0
		for _, r := range reports {
			if r.Result != okf.DetectNoChange {
				changed++
			}
			fmt.Printf("  [%s] %s\n", r.Result, r.SourcePath)
		}
		fmt.Printf("\nTotal: %d files, %d need attention\n", len(reports), changed)
	}
	return 0
}
