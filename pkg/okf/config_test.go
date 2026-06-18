package okf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// =============================================================================
// Phase 1: Configuration Management - Platform Default Path Tests
// =============================================================================

// TestGetPlatformDefault_Linux tests platform default on Linux
func TestGetPlatformDefault_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping Linux-specific test")
	}

	got := GetPlatformDefault()
	expected := filepath.Join(os.Getenv("HOME"), ".okf", "knowledge")

	if got != expected {
		t.Errorf("GetPlatformDefault() on Linux = %q, want %q", got, expected)
	}
}

// TestGetPlatformDefault_macOS tests platform default on macOS
func TestGetPlatformDefault_macOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("skipping macOS-specific test")
	}

	got := GetPlatformDefault()
	expected := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "okf", "knowledge")

	if got != expected {
		t.Errorf("GetPlatformDefault() on macOS = %q, want %q", got, expected)
	}
}

// TestGetPlatformDefault_Windows tests platform default on Windows
func TestGetPlatformDefault_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test")
	}

	got := GetPlatformDefault()
	expected := filepath.Join(os.Getenv("APPDATA"), "okf", "knowledge")

	if got != expected {
		t.Errorf("GetPlatformDefault() on Windows = %q, want %q", got, expected)
	}
}

// =============================================================================
// Phase 1: Configuration Management - Config File I/O Tests
// =============================================================================

// TestLoadConfig_FileNotFound tests loading config when file doesn't exist
func TestLoadConfig_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.yaml")

	cfg, err := LoadConfig(nonExistentPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v, want nil", err)
	}

	// Should return default config
	if cfg.KnowledgeDir != "" {
		t.Errorf("LoadConfig() KnowledgeDir = %q, want empty string for non-existent file", cfg.KnowledgeDir)
	}
}

// TestSaveConfig_Basic tests saving config to file
func TestSaveConfig_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{
		KnowledgeDir: "/custom/path",
	}

	err := SaveConfig(cfg, configPath)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v, want nil", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("SaveConfig() did not create config file at %q", configPath)
	}
}

// TestSaveConfig_CreatesParentDirs tests that SaveConfig creates parent directories
func TestSaveConfig_CreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir1", "subdir2", "config.yaml")

	cfg := &Config{
		KnowledgeDir: "/custom/path",
	}

	err := SaveConfig(cfg, configPath)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v, want nil", err)
	}

	// Verify parent directories were created
	if _, err := os.Stat(filepath.Dir(configPath)); os.IsNotExist(err) {
		t.Errorf("SaveConfig() did not create parent directories")
	}
}

// TestLoadConfig_RoundTrip tests loading config after saving
func TestLoadConfig_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	original := &Config{
		KnowledgeDir: "/test/knowledge",
	}

	if err := SaveConfig(original, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v, want nil", err)
	}

	if loaded.KnowledgeDir != original.KnowledgeDir {
		t.Errorf("LoadConfig() KnowledgeDir = %q, want %q", loaded.KnowledgeDir, original.KnowledgeDir)
	}
}

// TestLoadConfig_EmptyFile tests loading an empty config file
func TestLoadConfig_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "empty.yaml")

	// Create empty file
	if err := os.WriteFile(configPath, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create empty config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v, want nil", err)
	}

	if cfg.KnowledgeDir != "" {
		t.Errorf("LoadConfig() on empty file KnowledgeDir = %q, want empty", cfg.KnowledgeDir)
	}
}

// TestSaveConfig_Permission tests permission handling
func TestSaveConfig_Permission(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{KnowledgeDir: "/test"}

	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v, want nil", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	// Should be readable and writable
	if info.Mode()&0644 == 0 {
		t.Errorf("Config file permissions = %v, want readable/writable", info.Mode())
	}
}

// =============================================================================
// Phase 1: Configuration Management - Path Resolution Tests
// =============================================================================

// TestResolveKnowledgeDir_CLIFlag tests CLI flag takes highest priority
func TestResolveKnowledgeDir_CLIFlag(t *testing.T) {
	// Clear env
	os.Unsetenv("OKF_KNOWLEDGE_DIR")

	cliDir := filepath.Join(t.TempDir(), "cli", "knowledge")
	resolved, err := ResolveKnowledgeDir(cliDir)
	if err != nil {
		t.Fatalf("ResolveKnowledgeDir() error = %v", err)
	}

	if resolved != cliDir {
		t.Errorf("ResolveKnowledgeDir() with CLI flag = %q, want %q", resolved, cliDir)
	}
}

// TestResolveKnowledgeDir_EnvVar tests environment variable takes priority over default
func TestResolveKnowledgeDir_EnvVar(t *testing.T) {
	// Clear CLI flag
	cliDir := ""

	// Set environment variable
	original := os.Getenv("OKF_KNOWLEDGE_DIR")
	defer func() {
		if original != "" {
			os.Setenv("OKF_KNOWLEDGE_DIR", original)
		} else {
			os.Unsetenv("OKF_KNOWLEDGE_DIR")
		}
	}()
	envDir := filepath.Join(t.TempDir(), "env", "knowledge")
	os.Setenv("OKF_KNOWLEDGE_DIR", envDir)

	resolved, err := ResolveKnowledgeDir(cliDir)
	if err != nil {
		t.Fatalf("ResolveKnowledgeDir() error = %v", err)
	}

	if resolved != envDir {
		t.Errorf("ResolveKnowledgeDir() with env var = %q, want %q", resolved, envDir)
	}
}

// TestResolveKnowledgeDir_PlatformDefault tests platform default is used when nothing else set
func TestResolveKnowledgeDir_PlatformDefault(t *testing.T) {
	// Clear CLI and env
	cliDir := ""
	os.Unsetenv("OKF_KNOWLEDGE_DIR")

	resolved, err := ResolveKnowledgeDir(cliDir)
	if err != nil {
		t.Fatalf("ResolveKnowledgeDir() error = %v", err)
	}

	expected := GetPlatformDefault()
	if resolved != expected {
		t.Errorf("ResolveKnowledgeDir() with defaults = %q, want %q", resolved, expected)
	}
}

// TestResolveKnowledgeDir_CreatesDirectory tests that directory is created if it doesn't exist
func TestResolveKnowledgeDir_CreatesDirectory(t *testing.T) {
	// Clear env
	os.Unsetenv("OKF_KNOWLEDGE_DIR")
	os.Unsetenv("OKF_CONFIG_PATH")

	cliDir := t.TempDir()

	resolved, err := ResolveKnowledgeDir(cliDir)
	if err != nil {
		t.Fatalf("ResolveKnowledgeDir() error = %v", err)
	}

	if resolved != cliDir {
		t.Errorf("ResolveKnowledgeDir() = %q, want %q", resolved, cliDir)
	}

	// Directory should be created
	if _, err := os.Stat(cliDir); os.IsNotExist(err) {
		t.Errorf("ResolveKnowledgeDir() did not create directory at %q", cliDir)
	}
}

// TestResolveKnowledgeDir_LocalKB tests existing local KB is detected
func TestResolveKnowledgeDir_LocalKB(t *testing.T) {
	// Clear env
	os.Unsetenv("OKF_KNOWLEDGE_DIR")

	// Create local .okf/knowledge directory
	tmpDir := t.TempDir()
	localKB := filepath.Join(tmpDir, ".okf", "knowledge")
	if err := os.MkdirAll(localKB, 0755); err != nil {
		t.Fatalf("Failed to create local KB: %v", err)
	}

	// Change to temp directory so local KB detection works
	originalCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalCwd)

	resolved, err := ResolveKnowledgeDir("")
	if err != nil {
		t.Fatalf("ResolveKnowledgeDir() error = %v", err)
	}

	if canonicalKnowledgePathKey(resolved) != canonicalKnowledgePathKey(localKB) {
		t.Errorf("ResolveKnowledgeDir() with local KB = %q, want canonical %q", resolved, localKB)
	}
}

// TestResolveKnowledgeDir_EmptyCLIFlag tests empty CLI flag doesn't override
func TestResolveKnowledgeDir_EmptyCLIFlag(t *testing.T) {
	// Clear env
	os.Unsetenv("OKF_KNOWLEDGE_DIR")

	// Empty CLI flag should fall through to env or default
	resolved, err := ResolveKnowledgeDir("")
	if err != nil {
		t.Fatalf("ResolveKnowledgeDir() error = %v", err)
	}

	// Should get platform default since no local KB exists
	expected := GetPlatformDefault()
	if resolved != expected {
		t.Errorf("ResolveKnowledgeDir() with empty CLI = %q, want %q", resolved, expected)
	}
}

// TestResolveKnowledgeDir_Precedence tests the full precedence order
func TestResolveKnowledgeDir_Precedence(t *testing.T) {
	// Clear env
	os.Unsetenv("OKF_KNOWLEDGE_DIR")

	// Create temp directory and change to it
	tmpDir := t.TempDir()
	originalCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalCwd)

	tests := []struct {
		name   string
		cliDir string
		setEnv bool
		envDir string
		want   string
	}{
		{"CLI overrides all", filepath.Join(tmpDir, "cli", "knowledge"), false, "", filepath.Join(tmpDir, "cli", "knowledge")},
		{"Env overrides platform default", "", true, filepath.Join(tmpDir, "env", "knowledge"), filepath.Join(tmpDir, "env", "knowledge")},
		{"Platform default fallback", "", false, "", GetPlatformDefault()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv("OKF_KNOWLEDGE_DIR", tt.envDir)
				defer os.Unsetenv("OKF_KNOWLEDGE_DIR")
			} else {
				os.Unsetenv("OKF_KNOWLEDGE_DIR")
			}

			resolved, err := ResolveKnowledgeDir(tt.cliDir)
			if err != nil {
				t.Fatalf("ResolveKnowledgeDir() error = %v", err)
			}

			if resolved != tt.want {
				t.Errorf("ResolveKnowledgeDir() = %q, want %q", resolved, tt.want)
			}
		})
	}
}

func TestResolveKnowledgePaths_ConfigKnowledgeDirParticipates(t *testing.T) {
	t.Setenv("OKF_KNOWLEDGE_DIR", "")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configuredDir := filepath.Join(tmpDir, "configured", "knowledge")
	if err := SaveConfig(&Config{KnowledgeDir: configuredDir}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	resolved, err := ResolveKnowledgePaths(ResolveKnowledgePathsOptions{
		ConfigPath: configPath,
		WorkingDir: tmpDir,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("ResolveKnowledgePaths() error = %v", err)
	}

	if resolved.WritePath != configuredDir {
		t.Fatalf("WritePath = %q, want %q", resolved.WritePath, configuredDir)
	}
	if resolved.WriteSource != KnowledgePathSourceConfig {
		t.Fatalf("WriteSource = %q, want %q", resolved.WriteSource, KnowledgePathSourceConfig)
	}
	if len(resolved.ReadPaths) != 1 {
		t.Fatalf("ReadPaths length = %d, want 1", len(resolved.ReadPaths))
	}
	if resolved.ReadPaths[0].Path != configuredDir {
		t.Fatalf("ReadPaths[0].Path = %q, want %q", resolved.ReadPaths[0].Path, configuredDir)
	}
}

func TestResolveKnowledgePaths_ReadOnlyDoesNotCreateMissingPaths(t *testing.T) {
	t.Setenv("OKF_KNOWLEDGE_DIR", "")

	tmpDir := t.TempDir()
	missingConfig := filepath.Join(tmpDir, "missing", "config.yaml")
	missingWriteDir := filepath.Join(tmpDir, "write", "knowledge")

	resolved, err := ResolveKnowledgePaths(ResolveKnowledgePathsOptions{
		CLIDir:     missingWriteDir,
		ConfigPath: missingConfig,
		WorkingDir: tmpDir,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("ResolveKnowledgePaths() error = %v", err)
	}
	if resolved.WritePath != missingWriteDir {
		t.Fatalf("WritePath = %q, want %q", resolved.WritePath, missingWriteDir)
	}
	if _, err := os.Stat(missingConfig); !os.IsNotExist(err) {
		t.Fatalf("read-only resolver created config file or unexpected stat error: %v", err)
	}
	if _, err := os.Stat(missingWriteDir); !os.IsNotExist(err) {
		t.Fatalf("read-only resolver created write directory or unexpected stat error: %v", err)
	}
}

func TestResolveKnowledgePaths_RepoLocalPrecedesWorkingDirLocal(t *testing.T) {
	t.Setenv("OKF_KNOWLEDGE_DIR", "")

	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "repo")
	workingDir := filepath.Join(tmpDir, "work")
	repoLocal := filepath.Join(repoRoot, ".okf", "knowledge")
	workingLocal := filepath.Join(workingDir, ".okf", "knowledge")
	for _, dir := range []string{repoLocal, workingLocal} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	resolved, err := ResolveKnowledgePaths(ResolveKnowledgePathsOptions{
		ConfigPath: filepath.Join(tmpDir, "config.yaml"),
		RepoRoot:   repoRoot,
		WorkingDir: workingDir,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("ResolveKnowledgePaths() error = %v", err)
	}

	if resolved.WritePath != repoLocal {
		t.Fatalf("WritePath = %q, want repo local %q", resolved.WritePath, repoLocal)
	}
	if resolved.WriteSource != KnowledgePathSourceRepoLocal {
		t.Fatalf("WriteSource = %q, want %q", resolved.WriteSource, KnowledgePathSourceRepoLocal)
	}
}

func TestResolveKnowledgePaths_OverlayOrderAndDeduplication(t *testing.T) {
	t.Setenv("OKF_KNOWLEDGE_DIR", "")

	tmpDir := t.TempDir()
	writeDir := filepath.Join(tmpDir, "write")
	overlayA := filepath.Join(tmpDir, "overlay-a")
	overlayB := filepath.Join(tmpDir, "overlay-b")
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := SaveConfig(&Config{
		KnowledgeDir:   writeDir,
		KnowledgePaths: []string{overlayA, writeDir, overlayB, overlayA},
	}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	resolved, err := ResolveKnowledgePaths(ResolveKnowledgePathsOptions{
		ConfigPath: configPath,
		WorkingDir: tmpDir,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("ResolveKnowledgePaths() error = %v", err)
	}

	want := []string{writeDir, overlayA, overlayB}
	if len(resolved.ReadPaths) != len(want) {
		t.Fatalf("ReadPaths length = %d, want %d: %#v", len(resolved.ReadPaths), len(want), resolved.ReadPaths)
	}
	for i, wantPath := range want {
		if resolved.ReadPaths[i].Path != wantPath || resolved.ReadPaths[i].Rank != i {
			t.Fatalf("ReadPaths[%d] = %#v, want path %q rank %d", i, resolved.ReadPaths[i], wantPath, i)
		}
	}
}

func TestResolveKnowledgePaths_LegacySingleDirReadPaths(t *testing.T) {
	t.Setenv("OKF_KNOWLEDGE_DIR", "")

	tmpDir := t.TempDir()
	configuredDir := filepath.Join(tmpDir, "legacy", "knowledge")
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := SaveConfig(&Config{KnowledgeDir: configuredDir}, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	resolved, err := ResolveKnowledgePaths(ResolveKnowledgePathsOptions{
		ConfigPath: configPath,
		WorkingDir: tmpDir,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("ResolveKnowledgePaths() error = %v", err)
	}
	if len(resolved.ReadPaths) != 1 {
		t.Fatalf("ReadPaths length = %d, want exactly one legacy path", len(resolved.ReadPaths))
	}
	if resolved.WritePath != configuredDir {
		t.Fatalf("WritePath = %q, want legacy configured dir %q", resolved.WritePath, configuredDir)
	}
	if resolved.WriteSource != KnowledgePathSourceConfig {
		t.Fatalf("WriteSource = %q, want %q", resolved.WriteSource, KnowledgePathSourceConfig)
	}
	if resolved.ReadPaths[0].Path != configuredDir {
		t.Fatalf("ReadPaths[0].Path = %q, want %q", resolved.ReadPaths[0].Path, configuredDir)
	}
	if resolved.ReadPaths[0].Source != KnowledgePathSourceConfig || resolved.ReadPaths[0].Rank != 0 {
		t.Fatalf("ReadPaths[0] = %#v, want legacy config source rank 0", resolved.ReadPaths[0])
	}
}

func TestResolveKnowledgePaths_DefaultsToRepoRelativeKnowledgeDir(t *testing.T) {
	t.Setenv("OKF_KNOWLEDGE_DIR", "")

	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}

	resolved, err := ResolveKnowledgePaths(ResolveKnowledgePathsOptions{
		ConfigPath: filepath.Join(tmpDir, "config.yaml"),
		RepoRoot:   repoRoot,
		WorkingDir: repoRoot,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("ResolveKnowledgePaths() error = %v", err)
	}

	want := filepath.Join(repoRoot, ".okf", "knowledge")
	if resolved.WritePath != want {
		t.Fatalf("WritePath = %q, want %q", resolved.WritePath, want)
	}
	if resolved.WriteSource != KnowledgePathSourceRepoLocal {
		t.Fatalf("WriteSource = %q, want %q", resolved.WriteSource, KnowledgePathSourceRepoLocal)
	}
}
