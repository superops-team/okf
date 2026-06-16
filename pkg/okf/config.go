package okf

import (
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config represents the OKF configuration.
type Config struct {
	KnowledgeDir string `yaml:"knowledge_dir"`
}

// GetPlatformDefault returns the platform-specific default knowledge directory.
func GetPlatformDefault() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to temp directory if home cannot be determined
		home = os.TempDir()
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "okf", "knowledge")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "okf", "knowledge")
		}
		return filepath.Join(home, "okf", "knowledge")
	default: // linux and others
		return filepath.Join(home, ".okf", "knowledge")
	}
}

// LoadConfig loads configuration from a YAML file.
// Returns a default config if the file doesn't exist.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return cfg, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig saves configuration to a YAML file.
// Creates parent directories if they don't exist.
func SaveConfig(cfg *Config, path string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ResolveKnowledgeDir resolves the knowledge directory path using precedence:
// 1. CLI flag (highest priority)
// 2. Environment variable (OKF_KNOWLEDGE_DIR)
// 3. Local .okf/knowledge directory in current working directory
// 4. Platform default (lowest priority)
func ResolveKnowledgeDir(cliDir string) (string, error) {
	// 1. CLI flag takes highest priority
	if cliDir != "" {
		if err := os.MkdirAll(cliDir, 0755); err != nil {
			return "", err
		}
		return cliDir, nil
	}

	// 2. Environment variable
	if envDir := os.Getenv("OKF_KNOWLEDGE_DIR"); envDir != "" {
		if err := os.MkdirAll(envDir, 0755); err != nil {
			return "", err
		}
		return envDir, nil
	}

	// 3. Local .okf/knowledge directory in current working directory
	cwd, err := os.Getwd()
	if err == nil {
		localKB := filepath.Join(cwd, ".okf", "knowledge")
		if _, err := os.Stat(localKB); err == nil {
			return localKB, nil
		}
	}

	// 4. Platform default (lowest priority)
	defaultDir := GetPlatformDefault()
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		return "", err
	}
	return defaultDir, nil
}
