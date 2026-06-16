package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	okf "github.com/superops-team/okf/pkg/okf"
)

// =============================================================================
// okf metadata
//
// Subcommands:
//   inspect  - Print metadata index contents
//   rebuild  - Rebuild metadata index from existing knowledge base files
//   clean    - Remove orphaned metadata entries (source missing, target missing)
// =============================================================================

func cmdMetadata(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: missing subcommand (inspect|rebuild|clean)")
		metadataUsage()
		return 1
	}

	// Peel off subcommand (first arg)
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "inspect":
		return metadataInspect(rest)
	case "rebuild":
		return metadataRebuild(rest)
	case "clean":
		return metadataClean(rest)
	case "help", "-h", "--help":
		metadataUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown subcommand %q\n", sub)
		metadataUsage()
		return 1
	}
}

func metadataUsage() {
	fmt.Println("Usage: okf metadata <subcommand> [options]")
	fmt.Println("")
	fmt.Println("Manage the knowledge base metadata index.")
	fmt.Println("")
	fmt.Println("Subcommands:")
	fmt.Println("  inspect   Show the contents of the metadata index")
	fmt.Println("  rebuild   Rebuild index by scanning the knowledge base directory")
	fmt.Println("  clean     Remove orphaned entries (missing sources or targets)")
	fmt.Println("")
	fmt.Println("Common options:")
	fmt.Println("  -dir PATH   Knowledge base directory")
}

// --------- inspect ---------

func metadataInspect(args []string) int {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	dirFlag := flags.String("dir", "", "Knowledge base directory")
	quiet := flags.Bool("quiet", false, "Only print summary")

	boolFlags := map[string]bool{"quiet": true}
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
	metaPath := okf.KnowledgeMetadataPath(kbDir)
	idx := okf.NewMetadataIndex()
	if err := idx.Load(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load metadata: %v\n", err)
		return 1
	}

	fmt.Printf("Metadata file: %s\n", metaPath)
	fmt.Printf("Version:       %s\n", idx.Version)
	fmt.Printf("Files indexed: %d\n", idx.Len())
	fmt.Println("")

	if *quiet || idx.Len() == 0 {
		return 0
	}

	idx.RangeFiles(func(target string, meta *okf.FileMetadata) bool {
		if meta == nil {
			return true
		}
		fmt.Printf("- target=%s\n", target)
		fmt.Printf("    source:  %s\n", meta.SourcePath)
		fmt.Printf("    hash:    %s\n", meta.ContentHash)
		fmt.Printf("    size:    %d bytes\n", meta.FileSize)
		fmt.Printf("    modified: %s\n", meta.LastModified.Format(time.RFC3339))
		if meta.Strategy != "" {
			fmt.Printf("    strategy: %s\n", meta.Strategy)
		}
		if len(meta.PatchFields) > 0 {
			fmt.Printf("    patch-fields: %v\n", meta.PatchFields)
		}
		fmt.Printf("    source-exists: %v\n", meta.SourceExists)
		return true
	})
	return 0
}

// --------- rebuild ---------

func metadataRebuild(args []string) int {
	flags := flag.NewFlagSet("rebuild", flag.ContinueOnError)
	dirFlag := flags.String("dir", "", "Knowledge base directory")
	silent := flags.Bool("silent", false, "Suppress informational output")
	force := flags.Bool("force", false, "Force rebuild even if metadata already exists")

	boolFlags := map[string]bool{"silent": true, "force": true}
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

	// If metadata already exists and -force not given, abort
	if !*force {
		if _, err := os.Stat(metaPath); err == nil {
			fmt.Fprintf(os.Stderr, "Error: metadata file already exists at %s. Use -force to overwrite.\n", metaPath)
			return 1
		}
	}

	// Collect all .md files under kbDir
	var targets []string
	walkErr := filepath.Walk(kbDir, func(p string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.IsDir() {
			// Skip hidden dirs
			name := filepath.Base(p)
			if len(name) > 1 && name[0] == '.' && p != kbDir {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(p) == okf.DefaultMetadataFilename {
			return nil
		}
		if filepath.Ext(p) == ".md" {
			targets = append(targets, p)
		}
		return nil
	})
	if walkErr != nil {
		fmt.Fprintf(os.Stderr, "Error: walk failed: %v\n", walkErr)
		return 1
	}

	idx := okf.NewMetadataIndex()
	rebuilt := 0
	for _, absPath := range targets {
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(kbDir, absPath)
		if err != nil {
			rel = filepath.Base(absPath)
		}
		hash, err := okf.ComputeFileHash(absPath)
		if err != nil {
			continue
		}
		meta := &okf.FileMetadata{
			SourcePath:   absPath,
			TargetPath:   rel,
			ContentHash:  hash,
			LastModified: info.ModTime(),
			LastImported: time.Now().UTC(),
			FileSize:     info.Size(),
			Strategy:     okf.StrategySkip,
			SourceExists: true,
		}
		if err := idx.Add(meta); err != nil {
			continue
		}
		rebuilt++
	}

	if err := idx.Save(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save metadata: %v\n", err)
		return 1
	}

	if !*silent {
		fmt.Printf("Rebuilt metadata index: %d entries -> %s\n", rebuilt, metaPath)
	}
	return 0
}

// --------- clean ---------

func metadataClean(args []string) int {
	flags := flag.NewFlagSet("clean", flag.ContinueOnError)
	dirFlag := flags.String("dir", "", "Knowledge base directory")
	silent := flags.Bool("silent", false, "Suppress informational output")
	dryRun := flags.Bool("dry-run", false, "Show what would be removed without making changes")

	boolFlags := map[string]bool{"silent": true, "dry-run": true}
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
	metaPath := okf.KnowledgeMetadataPath(kbDir)
	idx := okf.NewMetadataIndex()
	if err := idx.Load(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load metadata: %v\n", err)
		return 1
	}

	toRemove := make([]string, 0)
	idx.RangeFiles(func(target string, meta *okf.FileMetadata) bool {
		if meta == nil {
			return true
		}
		sourceGone := false
		if meta.SourcePath != "" {
			if _, err := os.Stat(meta.SourcePath); err != nil {
				sourceGone = true
			}
		}
		targetGone := false
		absTarget := filepath.Join(kbDir, target)
		if _, err := os.Stat(absTarget); err != nil {
			targetGone = true
		}
		if sourceGone || targetGone {
			toRemove = append(toRemove, target)
		}
		return true
	})

	if len(toRemove) == 0 {
		if !*silent {
			fmt.Println("Nothing to clean.")
		}
		return 0
	}

	if *dryRun {
		if !*silent {
			fmt.Printf("Would remove %d orphaned entries:\n", len(toRemove))
			for _, t := range toRemove {
				fmt.Printf("  - %s\n", t)
			}
		}
		return 0
	}

	for _, t := range toRemove {
		_ = idx.DeleteByTarget(t)
	}
	if err := idx.Save(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save metadata: %v\n", err)
		return 1
	}

	if !*silent {
		fmt.Printf("Cleaned %d orphaned entries.\n", len(toRemove))
	}
	return 0
}
