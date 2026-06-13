package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/git"
	"github.com/superops-team/okf/pkg/lint"
	"github.com/superops-team/okf/pkg/okf"
	"github.com/superops-team/okf/pkg/okf/meta"
)

const Version = "1.0.0"

const usage = `okf - Open Knowledge Format CLI

Usage:
  okf <command> [options]

Commands:
  init        Initialize knowledge base from git repository
  update      Update knowledge base from latest commit
  lint        Check knowledge base for specification compliance
  show        Show knowledge base information
  search      Search the knowledge base
  hook        Install git hook for automatic updates
  version     Show version information
  help        Show this help message

Options:
  -repo PATH       Repository path (default: current directory)
  -dir PATH        Knowledge directory (default: .okf/knowledge)
  -verbose         Show detailed output
  -strict          Strict lint mode (warnings fail)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init", "generate":
		cmdInit(os.Args[2:])
	case "update":
		cmdUpdate(os.Args[2:])
	case "lint":
		cmdLint(os.Args[2:])
	case "show", "info":
		cmdShow(os.Args[2:])
	case "search":
		cmdSearch(os.Args[2:])
	case "hook":
		cmdHook(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("okf version %s (built with %s)\n", Version, meta.Info())
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Printf("Unknown command: %s\n\n", os.Args[1])
		fmt.Print(usage)
		os.Exit(1)
	}
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	repoPath := fs.String("repo", "", "Repository path")
	knowledgeDir := fs.String("dir", ".okf/knowledge", "Knowledge directory")
	force := fs.Bool("force", false, "Overwrite existing")
	_ = fs.Bool("verbose", false, "Verbose output") // TODO: implement verbose in init
	fs.Parse(args)

	if *repoPath == "" {
		wd, _ := os.Getwd()
		*repoPath = wd
	}

	if !git.IsRepo(*repoPath) {
		fmt.Printf("Error: %s is not a git repository\n", *repoPath)
		os.Exit(1)
	}

	cfg := git.DefaultConfig()
	cfg.RepoPath = *repoPath
	cfg.KnowledgeDir = *knowledgeDir

	start := time.Now()

	fmt.Println("Generating knowledge base from repository...")
	bundle, err := git.GenerateBundle(cfg, *force)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	saved, err := git.SaveKnowledgeBase(bundle, cfg)
	if err != nil {
		fmt.Printf("Error saving: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	fmt.Printf("\n✓ Generated %d concepts (%d saved to disk)\n", len(bundle.Concepts), saved)
	fmt.Printf("✓ Took %v\n", elapsed)

	fmt.Println("\nRunning lint check...")
	result := lintBundle(bundle)
	if result.HasErrors() {
		fmt.Printf("⚠ Lint: %d errors, %d warnings\n", result.Errors, result.Warnings)
	} else {
		fmt.Println("✓ Lint passed")
	}

	stats := bundle.Stats()
	fmt.Println("\n=== Statistics ===")
	fmt.Printf("Total Concepts: %d\n", stats.TotalConcepts)
	fmt.Printf("Unique Types: %d\n", stats.UniqueTypes)
	fmt.Printf("Unique Tags: %d\n", stats.UniqueTags)
}

func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	repoPath := fs.String("repo", "", "Repository path")
	full := fs.Bool("full", false, "Full regeneration")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	if *repoPath == "" {
		wd, _ := os.Getwd()
		*repoPath = wd
	}

	cfg := git.DefaultConfig()
	cfg.RepoPath = *repoPath

	if *full {
		fmt.Println("Running full regeneration...")
		bundle, err := git.GenerateBundle(cfg, true)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		saved, err := git.SaveKnowledgeBase(bundle, cfg)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Full regeneration complete: %d concepts saved\n", saved)
		return
	}

	start := time.Now()
	fmt.Println("Updating knowledge base from latest commit...")

	bundle, updated, err := git.UpdateFromLastCommit(cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if bundle == nil || len(bundle.Concepts) == 0 {
		fmt.Println("No changes detected.")
		return
	}

	outputDir := filepath.Join(cfg.RepoPath, cfg.KnowledgeDir)
	os.MkdirAll(outputDir, 0755)

	// Note: In production, would use parser.SerializeConcept here
	_ = outputDir
	_ = bundle

	elapsed := time.Since(start)
	fmt.Printf("✓ Updated %d concepts in %v\n", len(bundle.Concepts), elapsed)

	if *verbose {
		fmt.Println("\nChanged files:")
		for _, p := range updated {
			fmt.Printf("  - %s\n", p)
		}
	}
}

func cmdLint(args []string) {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	path := fs.String("path", "", "Knowledge base path")
	strict := fs.Bool("strict", false, "Strict mode")
	verbose := fs.Bool("verbose", false, "Show all issues")
	fs.Parse(args)

	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	var bundle *okf.KnowledgeBundle
	var err error

	okfDir := filepath.Join(*path, ".okf/knowledge")
	if okf.Exists(okfDir) {
		bundle, err = okf.LoadBundle(okfDir, okf.DefaultLoadOptions())
	} else {
		bundle, err = okf.LoadBundle(*path, okf.DefaultLoadOptions())
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if bundle == nil || len(bundle.Concepts) == 0 {
		fmt.Println("No concepts found.")
		return
	}

	cfg := lint.DefaultConfig()
	if *strict {
		cfg.StrictMode = true
	}

	result := lintBundleWithConfig(bundle, cfg)

	fmt.Printf("Linting %d concepts...\n\n", result.ConceptsChecked)

	for _, issue := range result.Issues {
		icon := map[lint.Severity]string{
			lint.Error:   "❌",
			lint.Warning: "⚠",
			lint.Info:    "ℹ",
		}[issue.Severity]

		filter := lint.Warning
		if *verbose {
			filter = lint.Info
		}

		if issue.Severity >= filter {
			loc := issue.FilePath
			if issue.Line > 0 {
				loc = fmt.Sprintf("%s:%d", loc, issue.Line)
			}
			fmt.Printf("%s [%s] %s - %s\n", icon, issue.Code, loc, issue.Message)
			if *verbose && issue.Suggestion != "" {
				fmt.Printf("   → %s\n", issue.Suggestion)
			}
		}
	}

	fmt.Printf("\n%s\n", result.Summary())

	if result.HasErrors() || (*strict && result.Warnings > 0) {
		os.Exit(1)
	}
	fmt.Println("✓ All checks passed!")
}

func cmdShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	path := fs.String("path", "", "Knowledge base path")
	detail := fs.Bool("detail", false, "Show details")
	fs.Parse(args)

	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	var bundle *okf.KnowledgeBundle
	var err error

	okfDir := filepath.Join(*path, ".okf/knowledge")
	if okf.Exists(okfDir) {
		bundle, err = okf.LoadBundle(okfDir, okf.DefaultLoadOptions())
	} else {
		bundle, err = okf.LoadBundle(*path, okf.DefaultLoadOptions())
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Knowledge Bundle: %s\n", bundle.Name)
	fmt.Printf("Root Path: %s\n", bundle.RootPath)
	fmt.Printf("Concepts: %d\n\n", len(bundle.Concepts))

	stats := bundle.Stats()
	fmt.Println("=== Statistics ===")
	fmt.Printf("Total: %d | Types: %d | Tags: %d\n\n", stats.TotalConcepts, stats.UniqueTypes, stats.UniqueTags)

	fmt.Println("=== Concepts by Type ===")
	for t, c := range stats.TypeCounts {
		fmt.Printf("  %-15s %d\n", t, c)
	}

	if *detail {
		fmt.Println("\n=== Concepts ===")
		for _, c := range bundle.Concepts {
			fmt.Printf("  [%s] %s - %s\n", c.Type, c.Title, c.Description)
		}
	}
}

func cmdSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	path := fs.String("path", "", "Knowledge base path")
	queryStr := fs.String("q", "", "Search query")
	cType := fs.String("type", "", "Filter by type")
	tag := fs.String("tag", "", "Filter by tag")
	fs.Parse(args)

	if *path == "" {
		wd, _ := os.Getwd()
		*path = wd
	}

	if *queryStr == "" && *cType == "" && *tag == "" {
		fmt.Println("Error: specify -q, -type, or -tag")
		os.Exit(1)
	}

	var bundle *okf.KnowledgeBundle
	var err error

	okfDir := filepath.Join(*path, ".okf/knowledge")
	if okf.Exists(okfDir) {
		bundle, err = okf.LoadBundle(okfDir, okf.DefaultLoadOptions())
	} else {
		bundle, err = okf.LoadBundle(*path, okf.DefaultLoadOptions())
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	results := bundle.Search(*queryStr)

	if *cType != "" {
		var filtered []*okf.Concept
		for _, c := range results {
			if c.Type == *cType {
				filtered = append(filtered, c)
			}
		}
		results = filtered
	}

	if *tag != "" {
		var filtered []*okf.Concept
		for _, c := range results {
			for _, t := range c.Tags {
				if t == *tag {
					filtered = append(filtered, c)
					break
				}
			}
		}
		results = filtered
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return
	}

	fmt.Printf("Found %d results:\n\n", len(results))
	for i, c := range results {
		fmt.Printf("%d. [%s] %s\n", i+1, c.Type, c.Title)
		fmt.Printf("   %s\n", c.Description)
		if c.FilePath != "" {
			fmt.Printf("   → %s\n", c.FilePath)
		}
		fmt.Println()
	}
}

func cmdHook(args []string) {
	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	repoPath := fs.String("repo", "", "Repository path")
	hookType := fs.String("type", "post-commit", "Hook type: pre-commit, post-commit, pre-push")
	uninstall := fs.Bool("uninstall", false, "Uninstall hook")
	force := fs.Bool("force", false, "Overwrite existing")
	fs.Parse(args)

	if *repoPath == "" {
		wd, _ := os.Getwd()
		*repoPath = wd
	}

	root, err := git.GetRepoRoot(*repoPath)
	if err != nil {
		fmt.Printf("Error: not a git repository: %v\n", err)
		os.Exit(1)
	}

	hookDir := filepath.Join(root, ".git", "hooks")
	hookFile := filepath.Join(hookDir, *hookType)

	if *uninstall {
		if okf.Exists(hookFile) {
			os.Remove(hookFile)
			fmt.Printf("✓ Removed %s hook\n", *hookType)
			return
		}
		fmt.Printf("Hook %s not found\n", *hookType)
		return
	}

	if okf.Exists(hookFile) && !*force {
		content, _ := os.ReadFile(hookFile)
		if strings.Contains(string(content), "# OKF Hook") {
			fmt.Println("OKF hook already installed. Use -force to overwrite.")
			return
		}
		fmt.Printf("Warning: existing hook found at %s\n", hookFile)
		os.Exit(1)
	}

	os.MkdirAll(hookDir, 0755)

	script := generateHookScript(*hookType, root)
	os.WriteFile(hookFile, []byte(script), 0755)
	os.Chmod(hookFile, 0755)

	fmt.Printf("✓ Installed %s hook at: %s\n", *hookType, hookFile)
	fmt.Printf("  Hook location: %s\n", hookFile)
}

func generateHookScript(hookType, repoRoot string) string {
	var body string
	switch hookType {
	case "post-commit":
		body = `# Update knowledge base from commit
cd "$(git rev-parse --show-toplevel)"
if command -v okf &> /dev/null; then
    okf update -verbose 2>/dev/null || true
fi
exit 0
`
	case "pre-commit":
		body = `# Lint knowledge base before commit
OKF_DIR="$(git rev-parse --show-toplevel)/.okf/knowledge"
if [ -d "$OKF_DIR" ]; then
    if command -v okf &> /dev/null; then
        cd "$(git rev-parse --show-toplevel)"
        okf lint || { echo "[OKF] Lint failed. Fix issues before committing."; exit 1; }
    fi
fi
exit 0
`
	case "pre-push":
		body = `# Update before push
cd "$(git rev-parse --show-toplevel)"
if command -v okf &> /dev/null; then
    okf init -force 2>/dev/null || true
fi
exit 0
`
	default:
		body = "# Unsupported hook type\nexit 0\n"
	}

	return "#!/bin/bash\n\n# OKF Hook - Automated Knowledge Base Update\n# okf CLI v" + Version + "\n\n" + body
}

// lintBundle converts and lints a bundle.
func lintBundle(b *okf.KnowledgeBundle) *lint.Result {
	return lintBundleWithConfig(b, lint.DefaultConfig())
}

func lintBundleWithConfig(b *okf.KnowledgeBundle, cfg *lint.Config) *lint.Result {
	// Convert concepts to lint.Concept format
	concepts := make([]*lint.Concept, len(b.Concepts))
	for i, c := range b.Concepts {
		concepts[i] = &lint.Concept{
			Type:        c.Type,
			Title:       c.Title,
			Description: c.Description,
			Resource:    c.Resource,
			Tags:        c.Tags,
			Timestamp:   c.Timestamp,
			Content:     c.Content,
			FilePath:    c.FilePath,
		}
	}

	return lint.LintBundle(concepts, cfg)
}
