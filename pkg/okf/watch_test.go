package okf

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// WatchConfig 测试
// ============================================================================

// TestLoadWatchConfig_Basic 测试基本配置加载
func TestLoadWatchConfig_Basic(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, DefaultWatchConfigFilename)
	cfgContent := `version: 1
knowledgeDir: ./kb
debounce: 300ms
rules:
  - name: docs
    sources:
      - ./docs
    strategy: merge
    patterns:
      - "**/*.md"
  - name: notes
    sources:
      - ./notes
    strategy: patch
    patchFields:
      - title
      - tags
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadWatchConfig(dir)
	if err != nil {
		t.Fatalf("LoadWatchConfig: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("Version: got %d, want 1", cfg.Version)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("Rules: got %d, want 2", len(cfg.Rules))
	}
	if cfg.Rules[0].Strategy != StrategyMerge {
		t.Errorf("rule[0].Strategy: got %s, want %s", cfg.Rules[0].Strategy, StrategyMerge)
	}
	if cfg.Rules[1].Strategy != StrategyPatch {
		t.Errorf("rule[1].Strategy: got %s, want %s", cfg.Rules[1].Strategy, StrategyPatch)
	}
	if len(cfg.Rules[1].PatchFields) != 2 {
		t.Errorf("rule[1].PatchFields: got %v, want 2 fields", cfg.Rules[1].PatchFields)
	}
	if cfg.Rules[0].Debounce != 300*time.Millisecond {
		t.Errorf("Debounce: got %v, want 300ms", cfg.Rules[0].Debounce)
	}
	// sources should be resolved to absolute paths
	for _, r := range cfg.Rules {
		for _, s := range r.Sources {
			if !filepath.IsAbs(s) {
				t.Errorf("source %s should be absolute", s)
			}
		}
	}
}

// TestLoadWatchConfig_InvalidStrategy 测试未知策略应报错
func TestLoadWatchConfig_InvalidStrategy(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, DefaultWatchConfigFilename)
	cfgContent := `version: 1
rules:
  - name: bad
    sources:
      - ./src
    strategy: unknown-strategy
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := LoadWatchConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid strategy, got nil")
	}
}

// TestLoadWatchConfig_DirectFile 测试直接指定配置文件路径
func TestLoadWatchConfig_DirectFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, DefaultWatchConfigFilename)
	cfgContent := `version: 1
rules:
  - name: r
    sources:
      - ./src
    strategy: overwrite
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// 直接传文件路径（不是目录）
	cfg, err := LoadWatchConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadWatchConfig(file): %v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("Rules: got %d, want 1", len(cfg.Rules))
	}
}

// TestLoadWatchConfig_Missing 测试缺失报错
func TestLoadWatchConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadWatchConfig(dir)
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
}

// ============================================================================
// glob 匹配测试
// ============================================================================

func TestWatchRule_MatchFilename(t *testing.T) {
	rule := &WatchRule{
		Sources:  []string{"/tmp/docs"},
		Patterns: []string{"**/*.md"},
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"simple md", "/tmp/docs/article.md", true},
		{"nested md", "/tmp/docs/sub/dir/file.md", true},
		{"not md", "/tmp/docs/data.txt", false},
		{"root level md", "article.md", true}, // basename fallback
		{"hidden file", "/tmp/docs/.secret", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rule.MatchFilename(tt.path)
			if got != tt.want {
				t.Errorf("MatchFilename(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchGlobStar_DirectMatch(t *testing.T) {
	// 没有 **，直接 filepath.Match
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.md", "article.md", true},
		{"*.md", "article.txt", false},
		{"a?.md", "a1.md", true},
		{"a?.md", "ab.md", true},  // ? 匹配 'b'
		{"a?.md", "a.md", false},  // ? 必须匹配一个字符
		{"[abc].md", "a.md", true},
		{"[abc].md", "d.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchGlobStar(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchGlobStar(%q,%q)=%v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchGlobStar_DoubleStar(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"**/*.md", "a.md", true},
		{"**/*.md", "docs/a.md", true},
		{"**/*.md", "docs/sub/a.md", true},
		{"**/*.md", "docs/a.txt", false},
		{"docs/**", "docs/a.md", true},
		{"docs/**", "docs/sub/x.md", true},
		{"docs/**/*.md", "docs/a.md", true},
		{"docs/**/*.md", "docs/sub/x.md", true},
		{"docs/**/*.md", "other/a.md", false},
		{"**", "anything/else.md", true},
		{"**", "a", true},
		{"**", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchGlobStar(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchGlobStar(%q,%q)=%v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// ============================================================================
// WatchDaemon 集成测试（模拟文件变更）
// ============================================================================

// TestWatchDaemon_NewAndRun 测试 daemon 创建与基础运行
func TestWatchDaemon_NewAndRun(t *testing.T) {
	dir := t.TempDir()
	kbDir := filepath.Join(dir, "kb")
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cfg := &WatchConfig{
		Version:      1,
		KnowledgeDir: kbDir,
		Debounce:     100 * time.Millisecond,
		Rules: []WatchRule{
			{
				Name:     "test-rule",
				Sources:  []string{srcDir},
				Strategy: StrategyOverwrite,
				Patterns: []string{"**/*.md"},
				Debounce: 100 * time.Millisecond,
			},
		},
	}

	daemon, err := NewWatchDaemon(cfg)
	if err != nil {
		t.Fatalf("NewWatchDaemon: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(200 * time.Millisecond)
		// write a file
		testFile := filepath.Join(srcDir, "article.md")
		content := "---\ntitle: T\n---\nHello\n"
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Errorf("WriteFile: %v", err)
			return
		}
		// wait for debounce + processing
		time.Sleep(800 * time.Millisecond)
		cancel()
	}()

	// Run daemon (blocks until ctx done)
	if err := daemon.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	// Check target was created
	expectedTarget := filepath.Join(kbDir, "article.md")
	if _, err := os.Stat(expectedTarget); err != nil {
		t.Fatalf("expected target file %s not created: %v", expectedTarget, err)
	}

	// Check metadata index exists
	metaPath := filepath.Join(kbDir, DefaultMetadataFilename)
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("expected metadata file %s not created: %v", metaPath, err)
	}

	// Load and verify
	idx := NewMetadataIndex()
	if err := idx.Load(metaPath); err != nil {
		t.Fatalf("Load metadata: %v", err)
	}
	if idx.Len() != 1 {
		t.Fatalf("idx.Len: got %d, want 1", idx.Len())
	}
}

// TestWatchDaemon_MergeStrategy 测试多次写入时 merge 策略
func TestWatchDaemon_MergeStrategy(t *testing.T) {
	dir := t.TempDir()
	kbDir := filepath.Join(dir, "kb")
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cfg := &WatchConfig{
		Version:      1,
		KnowledgeDir: kbDir,
		Debounce:     100 * time.Millisecond,
		Rules: []WatchRule{
			{
				Name:     "test-merge",
				Sources:  []string{srcDir},
				Strategy: StrategyMerge,
				Patterns: []string{"**/*.md"},
				Debounce: 100 * time.Millisecond,
			},
		},
	}

	daemon, err := NewWatchDaemon(cfg)
	if err != nil {
		t.Fatalf("NewWatchDaemon: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(200 * time.Millisecond)
		testFile := filepath.Join(srcDir, "doc.md")
		// First write
		os.WriteFile(testFile, []byte("---\ntitle: T1\ntags: [a]\n---\nHi\n"), 0644)
		time.Sleep(500 * time.Millisecond)
		// Second write - should merge tags
		os.WriteFile(testFile, []byte("---\ntitle: T2\ntags: [b]\n---\nUpdated\n"), 0644)
		time.Sleep(800 * time.Millisecond)
		cancel()
	}()

	if err := daemon.Run(ctx); err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}

	// Target should exist and have merged content
	expectedTarget := filepath.Join(kbDir, "doc.md")
	data, err := os.ReadFile(expectedTarget)
	if err != nil {
		t.Fatalf("expected target file not created: %v", err)
	}

	// merge 策略：保留 target body, 合并 tags, 保留 target 自定义字段
	// 第一次写入 body=Hi, title=T1, tags=[a]
	// 第二次写入 source: body=Updated, title=T2, tags=[b]
	// merge 后 body=Hi(来自 target), title=T1(来自 target), tags=[[a],[b]](合并)
	if !strings.Contains(string(data), "Hi") {
		t.Errorf("expected target body to be 'Hi' (merge keeps target body), got: %s", string(data))
	}
	if !strings.Contains(string(data), "title: T1") {
		t.Errorf("expected target custom field 'title: T1' preserved, got: %s", string(data))
	}
	if !strings.Contains(string(data), "[a]") || !strings.Contains(string(data), "[b]") {
		t.Errorf("expected tags merged, got: %s", string(data))
	}
}
