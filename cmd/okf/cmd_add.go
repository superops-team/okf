package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	okf "github.com/superops-team/okf/pkg/okf"
)

// cmdAdd handles the "okf add" command.
func cmdAdd(args []string) int {
	flags := flag.NewFlagSet("add", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Println("Usage: okf add <path> [options]")
		fmt.Println("")
		fmt.Println("Import files, directories, or archives into the knowledge base.")
		fmt.Println("")
		fmt.Println("Options:")
		flags.PrintDefaults()
	}

	var (
		dirFlag   = flags.String("dir", "", "Knowledge base directory")
		forceFlag = flags.Bool("force", false, "Overwrite existing files")
		dryRun    = flags.Bool("dry-run", false, "Show what would be imported without making changes")
		silent    = flags.Bool("silent", false, "Suppress informational output")
	)

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	// Get source path
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

	// Build import options
	opts := &okf.ImportOptions{
		DryRun: *dryRun,
		Force:  *forceFlag,
		Silent: *silent,
	}

	// Show what we're doing
	if !*silent {
		fmt.Printf("Importing from: %s\n", srcPath)
		fmt.Printf("To knowledge base: %s\n", kbDir)
	}

	// Perform import
	result, err := okf.Import(srcPath, kbDir, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: import failed: %v\n", err)
		return 1
	}

	// Report results
	if !*silent {
		fmt.Println("")
		fmt.Println("Import summary:")
		fmt.Printf("  Total files found: %d\n", result.TotalFiles)
		fmt.Printf("  Imported: %d\n", result.ImportedFiles)
		fmt.Printf("  Skipped: %d\n", result.SkippedFiles)
		fmt.Printf("  Failed: %d\n", result.FailedFiles)
	}

	// Report errors
	if len(result.Errors) > 0 && !*silent {
		fmt.Println("")
		fmt.Println("Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  %s: %s\n", filepath.Base(e.FilePath), e.Message)
		}
	}

	// Return non-zero if any files failed
	if result.FailedFiles > 0 {
		return 1
	}

	return 0
}
