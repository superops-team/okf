package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/superops-team/okf/pkg/identity"
	"github.com/superops-team/okf/pkg/okf"

	toolsvc "github.com/superops-team/okf/pkg/tool"
)

func cmdIdentity(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: okf identity <ensure|resolve> [options]")
		fmt.Println()
		fmt.Println("Stable concept identity (okf_id) migration and resolution.")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  ensure   Migrate concepts to stable okf_id (dry-run by default; --apply writes)")
		fmt.Println("  resolve  Resolve a stable ref (okf://concept/<id>) to its current file path")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  okf identity ensure --repo . --dir knowledge           # dry-run")
		fmt.Println("  okf identity ensure --repo . --dir knowledge --apply   # write ids")
		fmt.Println("  okf identity resolve --repo . --dir knowledge --ref okf://concept/<id>")
		return 0
	}
	switch args[0] {
	case "ensure":
		return cmdIdentityEnsure(args[1:])
	case "resolve":
		return cmdIdentityResolve(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown identity subcommand: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Valid subcommands: ensure, resolve")
		fmt.Fprintln(os.Stderr, "Run 'okf identity --help' for usage.")
		return 1
	}
}

// cmdIdentityEnsure runs the identity migration engine. Default is dry-run;
// --apply writes ids. The knowledge root is resolved the same way as other
// knowledge-dir commands (explicit --dir, else <repo>/.okf/knowledge, else repo).
func cmdIdentityEnsure(args []string) int {
	flags := newToolFlagSet("identity ensure")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	apply := flags.Bool("apply", false, "Apply id assignments (default is dry-run)")
	if err := parseToolFlags(flags, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", sanitizeFlagParseError(err))
		return 1
	}
	root := resolveIdentityRoot(*repoPath, *knowledgeDir)
	report, err := identity.Ensure(root, *apply)
	if *jsonOut {
		out := struct {
			*identity.EnsureReport
			Error *identityEnsureJSONError `json:"error,omitempty"`
		}{EnsureReport: report}
		if err != nil {
			out.Error = &identityEnsureJSONError{Message: err.Error()}
		}
		data, mErr := json.MarshalIndent(out, "", "  ")
		if mErr != nil {
			fmt.Fprintf(os.Stderr, "Error: marshal report: %v\n", mErr)
			return 1
		}
		fmt.Println(string(data))
		if err != nil {
			return 1
		}
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	mode := "dry-run"
	if *apply {
		mode = "apply"
	}
	fmt.Printf("identity ensure (%s): missing=%d existing=%d\n", mode, report.Missing, report.Existing)
	for _, c := range report.Changes {
		fmt.Printf("  [%s] %s\n", c.Action, c.Path)
	}
	if len(report.Invalid) > 0 {
		fmt.Printf("  invalid: %v\n", report.Invalid)
	}
	if len(report.Duplicates) > 0 {
		for _, d := range report.Duplicates {
			fmt.Printf("  duplicate %s: %v\n", d.OKFID, d.Paths)
		}
	}
	if report.VectorRebuildRequired {
		fmt.Println("  vector_rebuild_required=true (run `okf vector rebuild`)")
	}
	return 0
}

// identityEnsureJSONError is the JSON error envelope when Ensure itself fails
// (e.g. duplicate or invalid preflight). The report may still carry partial
// diagnostics, so it is always marshalled alongside the error.
type identityEnsureJSONError struct {
	Message string `json:"message"`
}

// cmdIdentityResolve resolves a stable okf://concept/<id> ref via the shared
// tool service and emits the standard ToolEnvelope.
func cmdIdentityResolve(args []string) int {
	flags := newToolFlagSet("identity resolve")
	repoPath, knowledgeDir, jsonOut := addToolCommonFlags(flags)
	ref := flags.String("ref", "", "Stable ref: okf://concept/<okf_id> or bare okf_ id")
	if err := parseToolFlags(flags, args); err != nil {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationResolve, toolsvc.ErrInvalidRequest, sanitizeFlagParseError(err), "Fix the invalid flag values and try again."), *jsonOut || hasJSONFlag(args))
	}
	if *ref == "" {
		return emitToolEnvelope(toolInvalidEnvelopeWithContext(*repoPath, *knowledgeDir, toolsvc.OperationResolve, toolsvc.ErrInvalidRequest, "ref must not be empty", "Pass --ref <okf://concept/<okf_id>>."), *jsonOut)
	}
	return emitToolEnvelope(toolService(*repoPath, *knowledgeDir).Resolve(context.Background(), toolsvc.ResolveRequest{Ref: *ref}), *jsonOut)
}

// resolveIdentityRoot picks the directory that identity.Ensure should scan.
// It mirrors the resolution used by loadKnowledgeBundle: an explicit --dir wins,
// otherwise <repo>/.okf/knowledge is used when present, else the repo root.
func resolveIdentityRoot(repoPath, knowledgeDir string) string {
	if repoPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "."
		}
		repoPath = wd
	}
	if knowledgeDir != "" {
		if filepath.IsAbs(knowledgeDir) {
			return knowledgeDir
		}
		return filepath.Join(repoPath, knowledgeDir)
	}
	okfDir := filepath.Join(repoPath, ".okf", "knowledge")
	if okf.Exists(okfDir) {
		return okfDir
	}
	return repoPath
}
