package memorydefense

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// configShape is the YAML shape of the memory_defense config section.
type configShape struct {
	Enabled   bool     `yaml:"enabled"`
	Action    string   `yaml:"action"`
	Detectors []string `yaml:"detectors"`
}

// LoadPolicy reads <repoRoot>/.okf/config.yaml and extracts the memory_defense
// section. A missing file or section yields a disabled policy (backward
// compatible). An invalid action value returns an error.
func LoadPolicy(repoRoot string) (Policy, error) {
	pol := Policy{Enabled: false, Action: "redact"}
	path := filepath.Join(repoRoot, ".okf", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pol, nil
		}
		return pol, fmt.Errorf("read memory_defense config: %w", err)
	}
	if len(data) == 0 {
		return pol, nil
	}
	var raw struct {
		MemoryDefense configShape `yaml:"memory_defense"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return pol, fmt.Errorf("parse memory_defense config: %w", err)
	}
	pol.Enabled = raw.MemoryDefense.Enabled
	pol.Action = strings.ToLower(strings.TrimSpace(raw.MemoryDefense.Action))
	if pol.Action == "" {
		pol.Action = "redact"
	}
	if pol.Action != "redact" && pol.Action != "block" {
		return pol, fmt.Errorf("invalid memory_defense.action %q: must be redact or block", raw.MemoryDefense.Action)
	}
	pol.Detectors = raw.MemoryDefense.Detectors
	return pol, nil
}
