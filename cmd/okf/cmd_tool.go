package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	toolsvc "github.com/superops-team/okf/pkg/tool"
)

func cmdTool(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: okf tool <status|init|refresh|query|context> [options]")
		return 1
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
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown tool subcommand: %s\n", subcommand)
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
		Query:          *query,
		Limit:          *limit,
		Type:           *typeFilter,
		Tag:            *tag,
		FilePath:       *filePath,
		Language:       *language,
		SymbolKind:     *symbolKind,
		QualifiedName:  *qualifiedName,
		RelationKind:   *relationKind,
		RelationSource: *relationSource,
		RelationTarget: *relationTarget,
		IncludeTrace:   *includeTrace,
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
		fmt.Printf("%s ok\n", envelope.Operation)
	} else if envelope.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", envelope.Error.Message)
	} else {
		fmt.Fprintln(os.Stderr, "Error: operation failed")
	}
	if !envelope.OK {
		return 1
	}
	return 0
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
