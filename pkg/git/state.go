package git

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/superops-team/okf/pkg/okf"
)

// State records the repository indexing checkpoint.
type State struct {
	SchemaVersion     int    `json:"schema_version"`
	LastIndexedCommit string `json:"last_indexed_commit"`
	UpdatedAt         string `json:"updated_at"`
}

// ReadState loads the OKF indexing state from disk.
func ReadState(cfg *Config) (*State, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	path := statePath(cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// WriteState saves the OKF indexing state to disk.
func WriteState(cfg *Config, state *State) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if state == nil {
		state = &State{}
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 1
	}
	if state.UpdatedAt == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	path := statePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func statePath(cfg *Config) string {
	return filepath.Join(filepath.Dir(resolveKnowledgeDir(cfg.RepoPath, cfg.KnowledgeDir)), "state.json")
}

func resolveKnowledgeDir(repoRoot, knowledgeDir string) string {
	if knowledgeDir == "" {
		knowledgeDir = DefaultConfig().KnowledgeDir
	}
	if filepath.IsAbs(knowledgeDir) {
		return knowledgeDir
	}
	return filepath.Join(repoRoot, knowledgeDir)
}

// UpdateSinceLastIndexedCommit updates based on the state checkpoint range.
func UpdateSinceLastIndexedCommit(cfg *Config) (*okf.KnowledgeBundle, []string, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	repoRoot, err := GetRepoRoot(cfg.RepoPath)
	if err != nil {
		return nil, nil, err
	}
	cfg.RepoPath = repoRoot

	head, err := GetCurrentCommit(repoRoot)
	if err != nil {
		return nil, nil, err
	}

	state, err := ReadState(cfg)
	if err != nil || state.LastIndexedCommit == "" {
		bundle, err := GenerateBundle(cfg, true)
		if err != nil {
			return nil, nil, err
		}
		var updated []string
		for _, concept := range bundle.Concepts {
			if concept.Resource != "" {
				updated = append(updated, concept.Resource)
			}
		}
		sort.Strings(updated)
		return bundle, updated, nil
	}

	if state.LastIndexedCommit == head {
		return nil, nil, nil
	}

	changedFiles, err := changedFilesBetween(repoRoot, state.LastIndexedCommit, head)
	if err != nil {
		return nil, nil, err
	}
	if len(changedFiles) == 0 {
		return nil, nil, WriteState(cfg, &State{LastIndexedCommit: head})
	}

	bundle, updated, err := UpdateBundle(cfg, changedFiles)
	if err != nil {
		return nil, nil, err
	}
	if bundle == nil || len(bundle.Concepts) == 0 {
		return nil, nil, WriteState(cfg, &State{LastIndexedCommit: head})
	}
	return bundle, updated, nil
}

func changedFilesBetween(repoRoot, from, to string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-status", "-z", from+".."+to)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w (output: %s)", err, string(out))
	}

	seen := make(map[string]bool)
	var files []string
	add := func(file string) {
		if file == "" || seen[file] {
			return
		}
		seen[file] = true
		files = append(files, file)
	}

	entries := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	for i := 0; i < len(entries); {
		status := entries[i]
		i++
		if status == "" || i >= len(entries) {
			continue
		}
		switch status[0] {
		case 'R':
			oldPath := entries[i]
			i++
			if i >= len(entries) {
				add(oldPath)
				continue
			}
			newPath := entries[i]
			i++
			add(oldPath)
			add(newPath)
		case 'C':
			i++ // source path
			if i >= len(entries) {
				continue
			}
			newPath := entries[i]
			i++
			add(newPath)
		default:
			file := entries[i]
			i++
			add(file)
		}
	}
	sort.Strings(files)
	return files, nil
}
