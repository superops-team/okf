package okf

import (
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config represents the OKF configuration.
type Config struct {
	KnowledgeDir   string   `yaml:"knowledge_dir"`
	KnowledgePaths []string `yaml:"knowledge_paths"`
}

type KnowledgePathSource string

const (
	KnowledgePathSourceCLI             KnowledgePathSource = "cli"
	KnowledgePathSourceEnv             KnowledgePathSource = "env"
	KnowledgePathSourceConfig          KnowledgePathSource = "config"
	KnowledgePathSourceRepoLocal       KnowledgePathSource = "repo_local"
	KnowledgePathSourceWorkingDirLocal KnowledgePathSource = "working_dir_local"
	KnowledgePathSourcePlatformDefault KnowledgePathSource = "platform_default"
	KnowledgePathSourceOverlay         KnowledgePathSource = "overlay"
)

type ResolveKnowledgePathsOptions struct {
	CLIDir      string
	ConfigPath  string
	RepoRoot    string
	WorkingDir  string
	ReadOnly    bool
	EnsureWrite bool
}

type ResolvedKnowledgePath struct {
	Path   string
	Source KnowledgePathSource
	Rank   int
}

type ResolvedKnowledgePaths struct {
	WritePath   string
	WriteSource KnowledgePathSource
	ReadPaths   []ResolvedKnowledgePath
	Warnings    []string
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

func GetDefaultConfigPath() string {
	if configPath := os.Getenv("OKF_CONFIG_PATH"); configPath != "" {
		return configPath
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".okf", "config.yaml")
	}

	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "okf", "config.yaml")
		}
		return filepath.Join(home, "okf", "config.yaml")
	default:
		return filepath.Join(home, ".okf", "config.yaml")
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

func ResolveKnowledgePaths(opts ResolveKnowledgePathsOptions) (ResolvedKnowledgePaths, error) {
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath()
	}
	workingDir := opts.WorkingDir
	if workingDir == "" {
		if wd, err := os.Getwd(); err == nil {
			workingDir = wd
		}
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return ResolvedKnowledgePaths{}, err
	}

	writePath, writeSource := selectWriteKnowledgePath(opts, cfg, workingDir)
	if opts.EnsureWrite && !opts.ReadOnly {
		if err := os.MkdirAll(writePath, 0755); err != nil {
			return ResolvedKnowledgePaths{}, err
		}
	}

	readPaths := []ResolvedKnowledgePath{{Path: writePath, Source: writeSource, Rank: 0}}
	for _, overlay := range cfg.KnowledgePaths {
		overlay = filepath.Clean(overlay)
		if overlay == "." || overlay == "" {
			continue
		}
		readPaths = append(readPaths, ResolvedKnowledgePath{Path: overlay, Source: KnowledgePathSourceOverlay})
	}
	readPaths = dedupeKnowledgeReadPaths(readPaths)

	return ResolvedKnowledgePaths{
		WritePath:   writePath,
		WriteSource: writeSource,
		ReadPaths:   readPaths,
		Warnings:    []string{},
	}, nil
}

func selectWriteKnowledgePath(opts ResolveKnowledgePathsOptions, cfg *Config, workingDir string) (string, KnowledgePathSource) {
	if opts.CLIDir != "" {
		return opts.CLIDir, KnowledgePathSourceCLI
	}
	if envDir := os.Getenv("OKF_KNOWLEDGE_DIR"); envDir != "" {
		return envDir, KnowledgePathSourceEnv
	}
	if cfg != nil && cfg.KnowledgeDir != "" {
		return cfg.KnowledgeDir, KnowledgePathSourceConfig
	}
	if opts.RepoRoot != "" {
		repoLocal := filepath.Join(opts.RepoRoot, ".okf", "knowledge")
		if info, err := os.Stat(repoLocal); err == nil && info.IsDir() {
			return repoLocal, KnowledgePathSourceRepoLocal
		}
		return repoLocal, KnowledgePathSourceRepoLocal
	}
	if opts.RepoRoot == "" && workingDir != "" {
		cwdLocal := filepath.Join(workingDir, ".okf", "knowledge")
		if info, err := os.Stat(cwdLocal); err == nil && info.IsDir() {
			return cwdLocal, KnowledgePathSourceWorkingDirLocal
		}
	}
	return GetPlatformDefault(), KnowledgePathSourcePlatformDefault
}

func dedupeKnowledgeReadPaths(paths []ResolvedKnowledgePath) []ResolvedKnowledgePath {
	seen := map[string]bool{}
	result := make([]ResolvedKnowledgePath, 0, len(paths))
	for _, p := range paths {
		key := canonicalKnowledgePathKey(p.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		p.Rank = len(result)
		result = append(result, p)
	}
	return result
}

func canonicalKnowledgePathKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if evaluated, err := filepath.EvalSymlinks(path); err == nil {
		path = evaluated
	}
	return filepath.Clean(path)
}

// ResolveKnowledgeDir resolves the knowledge directory path using precedence:
// 1. CLI flag (highest priority)
// 2. Environment variable (OKF_KNOWLEDGE_DIR)
// 3. Local .okf/knowledge directory in current working directory
// 4. Platform default (lowest priority)
func ResolveKnowledgeDir(cliDir string) (string, error) {
	resolved, err := ResolveKnowledgePaths(ResolveKnowledgePathsOptions{
		CLIDir:      cliDir,
		EnsureWrite: true,
	})
	if err != nil {
		return "", err
	}
	return resolved.WritePath, nil
}
