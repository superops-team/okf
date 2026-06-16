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

	cliDir := "/cli/path"
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
	os.Setenv("OKF_KNOWLEDGE_DIR", "/env/path")

	resolved, err := ResolveKnowledgeDir(cliDir)
	if err != nil {
		t.Fatalf("ResolveKnowledgeDir() error = %v", err)
	}

	if resolved != "/env/path" {
		t.Errorf("ResolveKnowledgeDir() with env var = %q, want %q", resolved, "/env/path")
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

	if resolved != localKB {
		t.Errorf("ResolveKnowledgeDir() with local KB = %q, want %q", resolved, localKB)
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
		{"CLI overrides all", "/cli/path", false, "", "/cli/path"},
		{"Env overrides platform default", "", true, "/env/path", "/env/path"},
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
