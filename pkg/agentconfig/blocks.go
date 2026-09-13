package agentconfig

import (
	"bytes"
	"strings"
)

// inspectWholeFile computes proposed bytes for a whole-file owned artifact
// (Cursor rule, Claude skill). Absent file -> create; present with OKF header
// -> replace; present without OKF header -> conflict.
func inspectWholeFile(current, desired []byte) (proposed []byte, st FileStatus, act FileAction) {
	if len(current) == 0 {
		return desired, StatusMissing, ActionCreate
	}
	if !bytes.Contains(current, []byte(PendingFileHeader)) {
		return nil, StatusConflict, ActionNoChange
	}
	if bytes.Equal(current, desired) {
		return current, StatusInstalled, ActionNoChange
	}
	return desired, StatusDrifted, ActionReplace
}

// removeWholeFile reports whether a whole-file path may be deleted (it must
// carry the OKF ownership header).
func removeWholeFile(current []byte) bool {
	return len(current) > 0 && bytes.Contains(current, []byte(PendingFileHeader))
}

// managedBlock joins delimited markers with a body on one line each.
func managedBlock(begin, body, end string) string {
	return begin + "\n" + strings.TrimSuffix(body, "\n") + "\n" + end
}

// inspectBlock computes proposed bytes for a delimited-text adapter (Codex
// TOML, Codex AGENTS.md). begin/end are the exact marker lines. extraConflict
// may perform additional ownership checks on the current bytes (e.g. scanning
// for an unowned TOML table).
//
// It preserves every byte outside the managed region. Missing markers means
// the block is appended (no conflict). Unbalanced/duplicate markers are a
// conflict.
func inspectBlock(current []byte, begin, end, body string, extraConflict func(current []byte, blockStart, blockEnd int) error) (proposed []byte, st FileStatus, act FileAction, err error) {
	block := managedBlock(begin, body, end)
	if len(current) == 0 {
		return []byte(block + "\n"), StatusMissing, ActionCreate, nil
	}

	cur := string(current)
	begins := strings.Count(cur, begin)
	ends := strings.Count(cur, end)

	// No markers at all: this is a first installation. Append the block; the
	// extraConflict hook still validates against unowned host tables.
	if begins == 0 && ends == 0 {
		if extraConflict != nil {
			if cerr := extraConflict(current, 0, 0); cerr != nil {
				return nil, StatusConflict, ActionNoChange, cerr
			}
		}
		sep := "\n"
		if len(current) == 0 {
			sep = ""
		} else if bytes.HasSuffix(current, []byte("\n\n")) {
			sep = ""
		} else if bytes.HasSuffix(current, []byte("\n")) {
			sep = "\n"
		} else {
			sep = "\n\n"
		}
		proposed = append(append(append([]byte{}, current...), []byte(sep)...), []byte(block+"\n")...)
		return proposed, StatusMissing, ActionCreate, nil
	}

	if begins != 1 || ends != 1 {
		return nil, StatusConflict, ActionNoChange,
			errConflict("managed markers are unbalanced (need exactly one begin and one end)")
	}
	i := strings.Index(cur, begin)
	j := strings.Index(cur, end)
	if i < 0 || j < 0 || i >= j {
		return nil, StatusConflict, ActionNoChange,
			errConflict("managed markers are unbalanced")
	}
	if extraConflict != nil {
		if cerr := extraConflict(current, i, j+len(end)); cerr != nil {
			return nil, StatusConflict, ActionNoChange, cerr
		}
	}

	prefix := current[:i]
	suffix := current[j+len(end):]
	currentBlock := current[i : j+len(end)]
	proposed = append(append(append([]byte{}, prefix...), []byte(block)...), suffix...)

	if bytes.Equal(currentBlock, []byte(block)) {
		return current, StatusInstalled, ActionNoChange, nil
	}
	return proposed, StatusDrifted, ActionReplace, nil
}

// removeBlock computes bytes that strip a balanced managed block. It returns
// (nil, false) when markers are unbalanced or absent in a way that makes
// removal ambiguous.
func removeBlock(current []byte, begin, end string) (proposed []byte, ok bool) {
	cur := string(current)
	if strings.Count(cur, begin) != 1 || strings.Count(cur, end) != 1 {
		return nil, false
	}
	i := strings.Index(cur, begin)
	j := strings.Index(cur, end)
	if i < 0 || j < 0 || i >= j {
		return nil, false
	}
	before := current[:i]
	after := current[j+len(end):]
	// Collapse the single blank-line seam the adapter inserts between a block
	// and surrounding content.
	switch {
	case len(before) > 0 && bytes.HasSuffix(before, []byte("\n")) && bytes.HasPrefix(after, []byte("\n")):
		after = after[1:]
	case len(before) == 0:
		after = bytes.TrimPrefix(after, []byte("\n"))
	}
	return append(before, after...), true
}

// tomlHasUnownedTable reports whether a [mcp_servers.okf...] TOML table header
// exists outside the managed byte range [blockStart, blockEnd).
func tomlHasUnownedTable(current []byte, blockStart, blockEnd int) error {
	lines := strings.Split(string(current), "\n")
	off := 0
	for _, line := range lines {
		start := off
		end := off + len(line)
		trimmed := strings.TrimSpace(line)
		isTable := strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "mcp_servers.okf")
		if isTable {
			inside := start >= blockStart && end <= blockEnd
			if !inside {
				return errConflict("unowned TOML table " + trimmed + " outside the OKF-managed block")
			}
		}
		off = end + 1 // +1 for the newline consumed by Split
	}
	return nil
}
