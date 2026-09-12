package agentconfig

import (
	"os"
)

// adapterVersion is the explicit adapter contract version, reported in status
// output. Bump it together with a reviewed spec change.
const adapterVersion = "agentconfig-v1"

// opMode selects install/render vs removal planning.
type opMode int

const (
	modeInstall opMode = iota
	modeRemove
)

// fileOp is the resolved, repository-absolute desired state for one managed
// file. proposed==nil && delete==true means delete the file entirely.
type fileOp struct {
	rel      string
	abs      string
	current  []byte
	proposed []byte
	delete   bool
	status   FileStatus
	action   FileAction
}

// fileSpec is one managed file for an adapter.
type fileSpec struct {
	rel string
	// inspectInstall computes the desired end state for apply.
	inspectInstall func(current []byte, cmd []string) (proposed []byte, delete bool, st FileStatus, act FileAction, err error)
	// inspectRemove computes the end state for remove. (nil, false, nil) means
	// there is nothing OKF owns to remove.
	inspectRemove func(current []byte) (proposed []byte, delete bool, err error)
}

// adapter builds plan ops for one client.
type adapter interface {
	name() string
	version() string
	specs() []fileSpec
}

// resolve reads current bytes at abs (nil when the file is absent) and
// enforces repository path boundaries.
func (s fileSpec) resolve(root string, cmd []string, mode opMode) (fileOp, error) {
	abs, err := safeJoin(root, s.rel)
	if err != nil {
		return fileOp{rel: s.rel, status: StatusConflict, action: ActionNoChange}, err
	}
	current, rerr := os.ReadFile(abs)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			current = nil
		} else {
			return fileOp{rel: s.rel, abs: abs, status: StatusConflict, action: ActionNoChange},
				errConflict("cannot read " + s.rel + ": " + rerr.Error())
		}
	}

	op := fileOp{rel: s.rel, abs: abs, current: current}
	if mode == modeInstall {
		prop, del, st, act, ierr := s.inspectInstall(current, cmd)
		if ierr != nil {
			if ace, ok := ierr.(*AgentConfigError); ok {
				ace.Path = s.rel
			}
			op.status = StatusConflict
			op.action = ActionNoChange
			return op, ierr
		}
		op.proposed, op.delete, op.status, op.action = prop, del, st, act
	} else {
		prop, del, rerr := s.inspectRemove(current)
		if rerr != nil {
			if ace, ok := rerr.(*AgentConfigError); ok {
				ace.Path = s.rel
			}
			op.status = StatusConflict
			op.action = ActionNoChange
			return op, rerr
		}
		if prop == nil && !del {
			op.status = StatusMissing
			op.action = ActionNoChange
			op.proposed = current
			return op, nil
		}
		op.proposed, op.delete = prop, del
		op.status = StatusInstalled
		op.action = ActionRemove
	}
	return op, nil
}

// buildOps runs all specs for an adapter under the given mode.
func buildOps(a adapter, root string, cmd []string, mode opMode) ([]fileOp, error) {
	ops := make([]fileOp, 0, len(a.specs()))
	for _, sp := range a.specs() {
		op, err := sp.resolve(root, cmd, mode)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// ---- spec constructors ----

// jsonMCPSpec manages an mcpServers.okf JSON entry.
func jsonMCPSpec(rel string) fileSpec {
	return fileSpec{
		rel: rel,
		inspectInstall: func(current []byte, cmd []string) ([]byte, bool, FileStatus, FileAction, error) {
			prop, st, act, err := inspectJSONMCP(current, cmd)
			return prop, false, st, act, err
		},
		inspectRemove: func(current []byte) ([]byte, bool, error) {
			prop, err := removeJSONMCP(current)
			if err != nil {
				return nil, false, err
			}
			if prop == nil {
				return nil, false, nil
			}
			return prop, false, nil
		},
	}
}

// wholeFileSpec manages a file owned in its entirety by the OKF header.
func wholeFileSpec(rel string, desired []byte) fileSpec {
	return fileSpec{
		rel: rel,
		inspectInstall: func(current []byte, cmd []string) ([]byte, bool, FileStatus, FileAction, error) {
			prop, st, act := inspectWholeFile(current, desired)
			return prop, false, st, act, nil
		},
		inspectRemove: func(current []byte) ([]byte, bool, error) {
			if len(current) == 0 {
				return nil, false, nil
			}
			if !removeWholeFile(current) {
				return nil, false, errConflict("file lacks the OKF ownership header; refusing to remove unowned file")
			}
			return nil, true, nil
		},
	}
}

// blockSpec manages a delimited Markdown/TOML region.
func blockSpec(rel, begin, end string, body []byte, extra func(current []byte, s, e int) error) fileSpec {
	return fileSpec{
		rel: rel,
		inspectInstall: func(current []byte, cmd []string) ([]byte, bool, FileStatus, FileAction, error) {
			prop, st, act, err := inspectBlock(current, begin, end, string(body), extra)
			return prop, false, st, act, err
		},
		inspectRemove: func(current []byte) ([]byte, bool, error) {
			if len(current) == 0 {
				return nil, false, nil
			}
			prop, ok := removeBlock(current, begin, end)
			if !ok {
				return nil, false, errConflict("managed markers are unbalanced; refusing to remove ambiguous region")
			}
			return prop, false, nil
		},
	}
}

// adapters registry.
func adapters() map[string]adapter {
	return map[string]adapter{
		"cursor":      cursorAdapter{},
		"claude-code": claudeAdapter{},
		"codex":       codexAdapter{},
	}
}
