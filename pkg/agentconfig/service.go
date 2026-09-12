package agentconfig

import (
	"errors"
	"fmt"
)

// DefaultCommand is the generated MCP server command. It assumes the
// project-scoped client launches the server with the repository root as cwd,
// so no absolute machine path is persisted.
var DefaultCommand = []string{"okf", "mcp", "--repo", "."}

// Service owns project-file planning and mutation for one repository. It
// never starts MCP, embedding or index processes.
type Service struct {
	root string
	cmd  []string
}

// NewService builds a Service for repo root. cmd is the resolved OKF MCP
// command; when nil DefaultCommand is used.
func NewService(root string, cmd []string) *Service {
	if len(cmd) == 0 {
		cmd = DefaultCommand
	}
	return &Service{root: root, cmd: cmd}
}

// FileReport is the public, content-free description of one managed file.
type FileReport struct {
	Path       string `json:"path"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	BeforeHash string `json:"before_hash,omitempty"`
	AfterHash  string `json:"after_hash,omitempty"`
}

// ClientReport summarizes the plan/status for one client.
type ClientReport struct {
	Client         string       `json:"client"`
	AdapterVersion string       `json:"adapter_version"`
	Status         string       `json:"status"`
	Files          []FileReport `json:"files"`
	Warnings       []string     `json:"warnings"`
}

func (s *Service) resolveAdapters(client string) ([]adapter, error) {
	if client == "all" {
		names := []string{"cursor", "claude-code", "codex"}
		out := make([]adapter, 0, len(names))
		for _, n := range names {
			out = append(out, adapters()[n])
		}
		return out, nil
	}
	if a, ok := adapters()[client]; ok {
		return []adapter{a}, nil
	}
	return nil, errUnsupported(fmt.Sprintf("unknown or unsupported agent client %q", client))
}

func reportFromOps(a adapter, ops []fileOp) ClientReport {
	r := ClientReport{
		Client:         a.name(),
		AdapterVersion: a.version(),
		Files:          []FileReport{},
		Warnings:       []string{},
	}
	overall := "no_change"
	for _, op := range ops {
		rep := FileReport{
			Path:   op.rel,
			Action: string(op.action),
			Status: string(op.status),
		}
		if len(op.current) > 0 {
			rep.BeforeHash = hashShort(op.current)
		}
		if op.delete {
			rep.Action = string(ActionRemove)
		} else if len(op.proposed) > 0 {
			rep.AfterHash = hashShort(op.proposed)
		}
		switch op.status {
		case StatusConflict:
			overall = "conflict"
		}
		if overall != "conflict" && op.action != ActionNoChange {
			overall = "change"
		}
		r.Files = append(r.Files, rep)
	}
	r.Status = overall
	return r
}

// Plan is read-only: it reports every proposed path, action and redacted hash
// and writes no file, starts no server, embedding or index.
func (s *Service) Plan(client string) ([]ClientReport, error) {
	ads, err := s.resolveAdapters(client)
	if err != nil {
		return nil, err
	}
	return s.runPlans(ads, modeInstall)
}

// Status reports the owned-item state for each client file without writing.
func (s *Service) Status(client string) ([]ClientReport, error) {
	ads, err := s.resolveAdapters(client)
	if err != nil {
		return nil, err
	}
	return s.runPlans(ads, modeInstall)
}

func (s *Service) runPlans(ads []adapter, mode opMode) ([]ClientReport, error) {
	reports := make([]ClientReport, 0, len(ads))
	for _, a := range ads {
		ops, err := buildOps(a, s.root, s.cmd, mode)
		if err != nil {
			var ace *AgentConfigError
			if errors.As(err, &ace) {
				rep := ClientReport{
					Client:         a.name(),
					AdapterVersion: a.version(),
					Status:         "conflict",
					Files:          []FileReport{},
					Warnings:       []string{ace.Error()},
				}
				if ace.Path != "" {
					rep.Files = append(rep.Files, FileReport{
						Path:   ace.Path,
						Action: string(ActionNoChange),
						Status: string(StatusConflict),
					})
				}
				reports = append(reports, rep)
				continue
			}
			return nil, err
		}
		reports = append(reports, reportFromOps(a, ops))
	}
	return reports, nil
}

type restorePoint struct {
	abs     string
	orig    []byte
	existed bool
}

// Apply is mutating. In non-interactive mode it requires yes=true; otherwise
// it fails before writing anything. Each file is replaced atomically; a
// cross-file failure restores changed files from in-process original bytes.
func (s *Service) Apply(client string, yes bool) error {
	if !yes {
		return &AgentConfigError{
			Code:        ErrRequiresConfirmation,
			Message:     "apply mutates project files; pass --yes in non-interactive mode",
			Remediation: "Re-run with --yes to confirm.",
		}
	}
	ads, err := s.resolveAdapters(client)
	if err != nil {
		return err
	}
	for _, a := range ads {
		ops, berr := buildOps(a, s.root, s.cmd, modeInstall)
		if berr != nil {
			return berr
		}
		if err := s.commit(a, ops); err != nil {
			return err
		}
	}
	return nil
}

// Remove is mutating and ownership-safe: it removes only OKF-owned content.
// Ambiguous ownership always fails with no force mode.
func (s *Service) Remove(client string, yes bool) error {
	if !yes {
		return &AgentConfigError{
			Code:        ErrRequiresConfirmation,
			Message:     "remove deletes OKF-owned content; pass --yes in non-interactive mode",
			Remediation: "Re-run with --yes to confirm.",
		}
	}
	ads, err := s.resolveAdapters(client)
	if err != nil {
		return err
	}
	for _, a := range ads {
		ops, berr := buildOps(a, s.root, s.cmd, modeRemove)
		if berr != nil {
			return berr
		}
		if err := s.commit(a, ops); err != nil {
			return err
		}
	}
	return nil
}

// commit applies ops in order with rollback. No-op ops are skipped.
func (s *Service) commit(a adapter, ops []fileOp) error {
	var restored []restorePoint
	for _, op := range ops {
		changed := op.action == ActionRemove || !byteEqual(op.current, op.proposed)
		if op.delete {
			changed = len(op.current) > 0
		} else if len(op.current) == 0 && len(op.proposed) == 0 {
			changed = false
		}
		if !changed {
			continue
		}
		rp := restorePoint{abs: op.abs, orig: op.current, existed: len(op.current) > 0}
		var werr error
		if op.delete {
			werr = removeOwnedFile(op.abs)
		} else {
			werr = writeFileAtomic(op.abs, op.proposed)
		}
		if werr != nil {
			// Rollback everything already applied.
			var rollbackErrs []string
			for _, r := range restored {
				if r.existed {
					if rerr := writeFileAtomic(r.abs, r.orig); rerr != nil {
						rollbackErrs = append(rollbackErrs, rerr.Error())
					}
				} else {
					if rerr := removeOwnedFile(r.abs); rerr != nil {
						rollbackErrs = append(rollbackErrs, rerr.Error())
					}
				}
			}
			if len(rollbackErrs) > 0 {
				return &AgentConfigError{
					Code:    "agent_config_rollback_failed",
					Message: fmt.Sprintf("apply failed at %q and rollback also failed: %v", op.rel, werr),
					Remediation: "Changed files were restored where possible; inspect these paths: " +
						joinStrings(rollbackErrs, "; "),
				}
			}
			return &AgentConfigError{
				Code:        "agent_config_apply_failed",
				Message:     fmt.Sprintf("apply failed at %q: %v", op.rel, werr),
				Remediation: "Changed files were rolled back to their original bytes.",
			}
		}
		restored = append(restored, rp)
	}
	return nil
}

func joinStrings(in []string, sep string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}
