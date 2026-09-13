package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/superops-team/okf/pkg/agentconfig"
)

// agentEnvelope is the stable output of the okf agent command.
type agentEnvelope struct {
	Operation string                     `json:"operation"`
	OK        bool                       `json:"ok"`
	Reports   []agentconfig.ClientReport `json:"reports,omitempty"`
	Error     *agentErrBody              `json:"error,omitempty"`
}

type agentErrBody struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
	Path        string `json:"path,omitempty"`
}

// CmdAgent is the entry point for `okf agent ...`. It returns a process exit
// code. It does not touch global CLI wiring; main.go dispatches to it.
func CmdAgent(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: okf agent <plan|apply|status|remove> [--client X] [--format json] [--yes]")
		fmt.Println()
		fmt.Println("Project-scoped agent client integration (Cursor, Claude Code, Codex).")
		fmt.Println("Generates MCP server config and canonical workflow rules with self-describing")
		fmt.Println("ownership markers. Never overwrites unowned or malformed client config.")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  plan     Preview what will change (no writes)")
		fmt.Println("  apply    Write client config (requires --yes in non-interactive mode)")
		fmt.Println("  status   Show installed state per client")
		fmt.Println("  remove   Remove OKF-managed config (requires --yes)")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  --client   cursor|claude-code|codex|all (default: all)")
		fmt.Println("  --repo     Repository root (default: current directory)")
		fmt.Println("  --format   text|json")
		fmt.Println("  --yes      Confirm mutating operation")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  okf agent plan --client cursor")
		fmt.Println("  okf agent apply --client all --yes")
		fmt.Println("  okf agent status --client cursor")
		return 0
	}
	sub := args[0]
	switch sub {
	case "plan", "apply", "status", "remove":
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown agent subcommand: %s\n", sub)
		fmt.Fprintln(os.Stderr, "Valid subcommands: plan, apply, status, remove")
		fmt.Fprintln(os.Stderr, "Run 'okf agent --help' for usage.")
		return 1
	}

	fs := flag.NewFlagSet("agent "+sub, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	client := fs.String("client", "all", "Agent client: cursor|claude-code|codex|all")
	repo := fs.String("repo", "", "Repository root (default: current directory)")
	format := fs.String("format", "text", "Output format: text|json")
	yes := fs.Bool("yes", false, "Confirm mutating operation in non-interactive mode")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if extras := fs.Args(); len(extras) > 0 {
		fmt.Fprintf(os.Stderr, "Error: unexpected arguments: %s\n", strings.Join(extras, " "))
		return 1
	}

	root := *repo
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			wd = "."
		}
		root = wd
	}
	svc := agentconfig.NewService(root, nil)
	wantJSON := *format == "json" || hasAgentJSONFlag(args[1:])

	env := agentEnvelope{Operation: "agent." + sub, OK: true}
	var runErr error
	switch sub {
	case "plan":
		env.Reports, runErr = svc.Plan(*client)
	case "status":
		env.Reports, runErr = svc.Status(*client)
	case "apply":
		runErr = svc.Apply(*client, *yes)
	case "remove":
		runErr = svc.Remove(*client, *yes)
	}
	if runErr != nil {
		env.OK = false
		if ace := asAgentErr(runErr); ace != nil {
			env.Error = &agentErrBody{Code: ace.Code, Message: ace.Message, Remediation: ace.Remediation, Path: ace.Path}
		} else {
			env.Error = &agentErrBody{Code: "internal_error", Message: runErr.Error()}
		}
	}

	if wantJSON {
		data, mErr := json.MarshalIndent(env, "", "  ")
		if mErr != nil {
			fmt.Fprintln(os.Stderr, "Error: cannot encode output")
			return 1
		}
		fmt.Println(string(data))
	} else if env.OK {
		printAgentText(sub, env.Reports)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", env.Error.Message)
		if env.Error.Code != "" {
			fmt.Fprintf(os.Stderr, "Code: %s\n", env.Error.Code)
		}
		if env.Error.Remediation != "" {
			fmt.Fprintf(os.Stderr, "%s\n", env.Error.Remediation)
		}
	}
	if !env.OK {
		return 1
	}
	return 0
}

func asAgentErr(err error) *agentconfig.AgentConfigError {
	var ace *agentconfig.AgentConfigError
	if errors.As(err, &ace) {
		return ace
	}
	return nil
}

func printAgentText(sub string, reports []agentconfig.ClientReport) {
	for _, r := range reports {
		fmt.Printf("[%s] adapter=%s overall=%s\n", r.Client, r.AdapterVersion, r.Status)
		for _, f := range r.Files {
			fmt.Printf("  - %-32s action=%-10s status=%s", f.Path, f.Action, f.Status)
			if f.BeforeHash != "" {
				fmt.Printf(" before=%s", f.BeforeHash)
			}
			if f.AfterHash != "" {
				fmt.Printf(" after=%s", f.AfterHash)
			}
			fmt.Println()
		}
		for _, w := range r.Warnings {
			fmt.Printf("  ! %s\n", w)
		}
	}
	if sub == "apply" || sub == "remove" {
		fmt.Println("done.")
	}
}

func hasAgentJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" {
			return true
		}
	}
	return false
}
