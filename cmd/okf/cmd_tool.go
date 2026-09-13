package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/superops-team/okf/pkg/manifest"
	toolsvc "github.com/superops-team/okf/pkg/tool"
)

func cmdTool(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: okf tool <status|init|refresh|query|context|manifest> [options]")
		fmt.Println()
		fmt.Println("Agent-facing JSON tool operations. All commands accept --json for")
		fmt.Println("machine-parseable output and --repo/--dir for knowledge base location.")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  status    Show knowledge bundle status and freshness")
		fmt.Println("  init      Initialize a knowledge bundle")
		fmt.Println("  refresh   Refresh the knowledge bundle index")
		fmt.Println("  query     Semantic/hybrid search with optional grouping")
		fmt.Println("  context   Build context for a query")
		fmt.Println("  manifest  Metadata-only listing (no body, no index side effects)")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  okf tool manifest --repo . --dir knowledge")
		fmt.Println("  okf tool query --repo . --dir knowledge -q \"search terms\" --group-by source")
		return 0
	}

	subcommand := args[0]
	switch subcommand {
	case "status":
		return cmdToolStatus(args[1:])
	case "init":
		return cmdToolInit(args[1:])
	case "refresh":
		return cmdToolRefresh(args[1:])
	case "query":
		return cmdToolQuery(args[1:])
	case "context":
		return cmdToolContext(args[1:])
	case "manifest":
		return cmdToolManifest(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown tool subcommand: %s\n", subcommand)
		fmt.Fprintln(os.Stderr, "Valid subcommands: status, init, refresh, query, context, manifest")
		fmt.Fprintln(os.Stderr, "Run 'okf tool --help' for usage.")
		return 1
	}
}

func cmdToolStatus(args []string) int {
	flags := newToolFlagSet("tool status")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	if err := parseToolFlags(flags, args); err != nil {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationStatus, toolsvc.ErrInvalidRequest, sanitizeFlagParseError(err), "Fix the invalid flag values and try again."), *jsonOut || hasJSONFlag(args))
	}
	return emitToolEnvelope(toolService(*repoPath, *knowledgeDir).Status(context.Background(), toolsvc.StatusRequest{}), *jsonOut)
}

func cmdToolInit(args []string) int {
	flags := newToolFlagSet("tool init")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	if err := parseToolFlags(flags, args); err != nil {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationInit, toolsvc.ErrInvalidRequest, sanitizeFlagParseError(err), "Fix the invalid flag values and try again."), *jsonOut || hasJSONFlag(args))
	}
	return emitToolEnvelope(toolService(*repoPath, *knowledgeDir).Init(context.Background(), toolsvc.InitRequest{}), *jsonOut)
}

func cmdToolRefresh(args []string) int {
	flags := newToolFlagSet("tool refresh")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	mode := flags.String("mode", toolsvc.RefreshModeIncremental, "Refresh mode: incremental|full|cache-only")
	if err := parseToolFlags(flags, args); err != nil {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationRefresh, toolsvc.ErrInvalidRequest, sanitizeFlagParseError(err), "Fix the invalid flag values and try again."), *jsonOut || hasJSONFlag(args))
	}
	return emitToolEnvelope(toolService(*repoPath, *knowledgeDir).Refresh(context.Background(), toolsvc.RefreshRequest{Mode: *mode}), *jsonOut)
}

func cmdToolQuery(args []string) int {
	flags := newToolFlagSet("tool query")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	query := flags.String("q", "", "Query text")
	limit := flags.Int("limit", 10, "Maximum number of results")
	typeFilter := flags.String("type", "", "Concept type filter")
	tag := flags.String("tag", "", "Tag filter")
	filePath := flags.String("file-path", "", "Source file path filter")
	language := flags.String("language", "", "Code language filter")
	symbolKind := flags.String("symbol-kind", "", "Symbol kind filter")
	qualifiedName := flags.String("qualified-name", "", "Qualified symbol name filter")
	relationKind := flags.String("relation-kind", "", "Relation kind filter")
	relationSource := flags.String("relation-source", "", "Relation source filter")
	relationTarget := flags.String("relation-target", "", "Relation target filter")
	includeTrace := flags.Bool("include-trace", false, "Include compact retrieval trace")
	groupBy := flags.String("group-by", "", "Project results into chunk|concept|source|folder groups (omit keeps ungrouped output)")
	includeGroupMembers := flags.Bool("include-group-members", false, "Include per-member hit lists inside each projected group")
	if err := parseToolFlags(flags, args); err != nil {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationQuery, toolsvc.ErrInvalidRequest, sanitizeFlagParseError(err), "Fix the invalid flag values and try again."), *jsonOut || hasJSONFlag(args))
	}
	if strings.TrimSpace(*query) == "" {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationQuery, toolsvc.ErrInvalidQuery, "query must not be empty", "Pass --q with a non-empty query string."), *jsonOut)
	}
	if *limit < 0 {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationQuery, toolsvc.ErrInvalidRequest, "limit must be non-negative", "Pass --limit 0 or a positive integer."), *jsonOut)
	}
	return emitToolEnvelope(toolService(*repoPath, *knowledgeDir).Query(context.Background(), toolsvc.QueryRequest{
		Query:               *query,
		Limit:               *limit,
		Type:                *typeFilter,
		Tag:                 *tag,
		FilePath:            *filePath,
		Language:            *language,
		SymbolKind:          *symbolKind,
		QualifiedName:       *qualifiedName,
		RelationKind:        *relationKind,
		RelationSource:      *relationSource,
		RelationTarget:      *relationTarget,
		IncludeTrace:        *includeTrace,
		GroupBy:             *groupBy,
		IncludeGroupMembers: *includeGroupMembers,
	}), *jsonOut)
}

func cmdToolContext(args []string) int {
	flags := newToolFlagSet("tool context")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	query := flags.String("q", "", "Query text")
	budgetTokens := flags.Int("budget-tokens", 4000, "Estimated token budget")
	includeRelations := flags.Bool("include-relations", false, "Include relation expansion")
	includeTrace := flags.Bool("include-trace", false, "Include compact retrieval trace")
	if err := parseToolFlags(flags, args); err != nil {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationContext, toolsvc.ErrInvalidRequest, sanitizeFlagParseError(err), "Fix the invalid flag values and try again."), *jsonOut || hasJSONFlag(args))
	}
	if strings.TrimSpace(*query) == "" {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationContext, toolsvc.ErrInvalidQuery, "query must not be empty", "Pass --q with a non-empty query string."), *jsonOut)
	}
	if *budgetTokens <= 0 {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationContext, toolsvc.ErrInvalidRequest, "budget-tokens must be positive", "Pass --budget-tokens with a positive integer."), *jsonOut)
	}
	return emitToolEnvelope(toolService(*repoPath, *knowledgeDir).Context(context.Background(), toolsvc.ContextRequest{
		Query:            *query,
		BudgetTokens:     *budgetTokens,
		IncludeRelations: *includeRelations,
		IncludeTrace:     *includeTrace,
	}), *jsonOut)
}

func cmdToolManifest(args []string) int {
	flags := newToolFlagSet("tool manifest")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	offset := flags.Int("offset", 0, "Pagination offset")
	limit := flags.Int("limit", 0, "Maximum items (1..500); omitted defaults to 100")
	types := flags.String("types", "", "Comma-separated type filter (OR within)")
	tags := flags.String("tags", "", "Comma-separated tag filter (OR within)")
	statuses := flags.String("statuses", "", "Comma-separated status filter (OR within)")
	stale := flags.Bool("stale", false, "Filter by staleness (omit to match all)")
	folderPrefix := flags.String("folder-prefix", "", "Bundle-relative folder prefix filter")
	includeTrace := flags.Bool("include-trace", false, "Include deterministic scan trace")
	if err := parseToolFlags(flags, args); err != nil {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationManifest, toolsvc.ErrInvalidRequest, sanitizeFlagParseError(err), "Fix the invalid flag values and try again."), *jsonOut || hasJSONFlag(args))
	}

	req := toolsvc.ManifestRequest{
		Offset:       *offset,
		Types:        splitListFlag(*types),
		Tags:         splitListFlag(*tags),
		Statuses:     splitListFlag(*statuses),
		FolderPrefix: *folderPrefix,
		IncludeTrace: *includeTrace,
	}
	// Preserve omitted-vs-explicit presence: a limit flag that was not provided
	// stays nil (→100); an explicit value is pointer-backed and range-validated.
	if flagSetOnCLI(flags, "limit") {
		l := *limit
		req.Limit = &l
	}
	if flagSetOnCLI(flags, "stale") {
		s := *stale
		req.Stale = &s
	}
	return emitToolEnvelope(toolService(*repoPath, *knowledgeDir).Manifest(context.Background(), req), *jsonOut)
}

// flagSetOnCLI reports whether a flag was explicitly provided on the command
// line (as opposed to left at its default value).
func flagSetOnCLI(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func splitListFlag(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newToolFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(ioDiscard{})
	return flags
}

func addToolCommonFlags(flags *flag.FlagSet) (*string, *string, *bool) {
	repoPath := flags.String("repo", "", "Repository path")
	knowledgeDir := flags.String("dir", "", "Knowledge directory")
	jsonOut := flags.Bool("json", false, "Output stable JSON envelope")
	return repoPath, knowledgeDir, jsonOut
}

func parseToolFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if extras := flags.Args(); len(extras) > 0 {
		return errors.New("unexpected positional arguments: " + strings.Join(extras, " "))
	}
	return nil
}

func toolService(repoPath, knowledgeDir string) *toolsvc.Service {
	return toolsvc.NewService(toolsvc.Config{RepoPath: repoPath, KnowledgeDir: knowledgeDir})
}

func emitToolEnvelope(envelope toolsvc.ToolEnvelope, jsonOut bool) int {
	if jsonOut {
		data, err := json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to marshal tool envelope: %v\n", err)
			return 1
		}
		fmt.Println(string(data))
	} else if envelope.OK {
		if envelope.Operation == toolsvc.OperationManifest {
			printManifestText(envelope)
		} else {
			fmt.Printf("%s ok\n", envelope.Operation)
		}
	} else if envelope.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", envelope.Error.Message)
		if envelope.Error.Remediation != "" {
			fmt.Fprintf(os.Stderr, "  → %s\n", envelope.Error.Remediation)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Error: operation failed")
	}
	if !envelope.OK {
		return 1
	}
	return 0
}

// printManifestText renders a manifest envelope as a human-readable listing.
// It shows total/offset/limit, then each item with path, title, type, tags,
// identity state, and estimated tokens. Warnings are listed separately.
func printManifestText(envelope toolsvc.ToolEnvelope) {
	var result manifest.ManifestResult
	switch r := envelope.Result.(type) {
	case manifest.ManifestResult:
		result = r
	case *manifest.ManifestResult:
		if r != nil {
			result = *r
		}
	default:
		fmt.Printf("%s ok\n", envelope.Operation)
		return
	}
	fmt.Printf("Manifest: %d concept(s) (offset=%d, limit=%d)\n", result.Total, result.Offset, result.Limit)
	if len(result.Items) == 0 {
		fmt.Println("  (no items)")
	}
	for i, item := range result.Items {
		idTag := ""
		if item.OKFID != "" {
			idTag = " id=" + item.OKFID[:12] + "…"
		}
		tags := ""
		if len(item.Tags) > 0 {
			tags = " [" + strings.Join(item.Tags, ",") + "]"
		}
		stale := ""
		if item.Stale {
			stale = " [stale]"
		}
		fmt.Printf("  %d. %s (%s)%s%s%s — ~%d tokens\n",
			i+1, item.Path, item.Type, idTag, tags, stale, item.EstimatedTokens)
		if item.Title != "" {
			fmt.Printf("     %s\n", item.Title)
		}
	}
	if len(result.Warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("  - %s: %s\n", w.Code, w.Path)
		}
	}
}

func toolInvalidEnvelope(operation, code, message, remediation string) toolsvc.ToolEnvelope {
	return toolsvc.ToolEnvelope{
		SchemaVersion: toolsvc.SchemaVersion,
		Operation:     operation,
		OK:            false,
		Mutating:      operation == toolsvc.OperationInit || operation == toolsvc.OperationRefresh,
		Warnings:      []string{},
		Error: &toolsvc.ToolError{
			Code:        code,
			Message:     message,
			Remediation: remediation,
		},
	}
}

func toolInvalidEnvelopeWithContext(repoPath, knowledgeDir, operation, code, message, remediation string) toolsvc.ToolEnvelope {
	env := toolInvalidEnvelope(operation, code, message, remediation)
	status := toolService(repoPath, knowledgeDir).Status(context.Background(), toolsvc.StatusRequest{})
	env.RepoRoot = status.RepoRoot
	env.KnowledgeDir = status.KnowledgeDir
	env.Freshness = status.Freshness
	env.Warnings = append(env.Warnings, status.Warnings...)
	return env
}

func sanitizeFlagParseError(err error) string {
	if err == nil {
		return "invalid flags"
	}
	return strings.TrimSpace(strings.ReplaceAll(err.Error(), "\n", " "))
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "-json" {
			return true
		}
	}
	return false
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
