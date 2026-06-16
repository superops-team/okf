package okf

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// Frontmatter 解析器测试
// =============================================================================

func TestParseFrontmatter_Standard(t *testing.T) {
	content := []byte(`---
type: api
title: Test API
description: A test API
tags:
  - test
  - api
---

# Body
This is the body.
`)
	fm, body, hasFM := ParseFrontmatter(content)
	if !hasFM {
		t.Fatal("expected frontmatter to be detected")
	}
	if fm["type"] != "api" {
		t.Errorf("type mismatch: %v", fm["type"])
	}
	if fm["title"] != "Test API" {
		t.Errorf("title mismatch: %v", fm["title"])
	}
	if body == "" {
		t.Error("body should not be empty")
	}
	if !containsString([]string{body}, "# Body") == false {
		t.Error("body should contain '# Body'")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte("# Just a heading\n\nNo frontmatter here.\n")
	fm, body, hasFM := ParseFrontmatter(content)
	if hasFM {
		t.Error("expected no frontmatter")
	}
	if fm != nil {
		t.Error("fm should be nil")
	}
	if body == "" {
		t.Error("body should not be empty")
	}
}

func TestSerializeFrontmatter_Basic(t *testing.T) {
	fm := map[string]interface{}{
		"type":        "api",
		"title":       "Test",
		"description": "A test",
		"tags":        []string{"test", "api"},
	}
	body := "# Body\n\nContent."

	result := string(SerializeFrontmatter(fm, body))

	if !contains(result, "type: api") {
		t.Error("expected serialized output to contain 'type: api'")
	}
	if !contains(result, "title: Test") {
		t.Error("expected serialized output to contain 'title: Test'")
	}
	if !contains(result, "tags:") {
		t.Error("expected serialized output to contain 'tags:'")
	}
	if !contains(result, "# Body") {
		t.Error("expected serialized output to contain body")
	}
}

func TestRoundTrip_Frontmatter(t *testing.T) {
	original := []byte(`---
type: api
title: Test
tags:
  - a
  - b
---

# Body
`)
	fm, body, _ := ParseFrontmatter(original)
	roundTripped := SerializeFrontmatter(fm, body)

	fm2, body2, _ := ParseFrontmatter(roundTripped)
	if fm2["type"] != fm["type"] {
		t.Error("type mismatch after roundtrip")
	}
	if fm2["title"] != fm["title"] {
		t.Error("title mismatch after roundtrip")
	}
	if body2 != body {
		t.Errorf("body mismatch after roundtrip:\n%s\nvs\n%s", body, body2)
	}
}

// =============================================================================
// MergeStrategy 决策测试
// =============================================================================

func TestDecideStrategy_CLIOverrides(t *testing.T) {
	meta := &FileMetadata{Strategy: StrategySkip}
	opts := &SmartImportOptions{ForceStrategy: StrategyOverwrite}

	got := DecideStrategy(meta, opts)
	if got != StrategyOverwrite {
		t.Errorf("expected overwrite from CLI, got %s", got)
	}
}

func TestDecideStrategy_RecordedStrategy(t *testing.T) {
	meta := &FileMetadata{Strategy: StrategyMerge}
	opts := &SmartImportOptions{}

	got := DecideStrategy(meta, opts)
	if got != StrategyMerge {
		t.Errorf("expected merge from record, got %s", got)
	}
}

func TestDecideStrategy_DefaultIsSkip(t *testing.T) {
	meta := &FileMetadata{}
	opts := &SmartImportOptions{}

	got := DecideStrategy(meta, opts)
	if got != StrategySkip {
		t.Errorf("expected default skip, got %s", got)
	}
}

// =============================================================================
// DoSkip 测试
// =============================================================================

func TestDoSkip_TargetExists(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	if err := os.WriteFile(source, []byte("source content"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("target content"), 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	meta := &FileMetadata{SourcePath: source, TargetPath: target}
	result, err := DoSkip(source, target, meta)
	if err != nil {
		t.Fatalf("DoSkip failed: %v", err)
	}
	if result.Changed {
		t.Error("expected Changed=false when target exists")
	}

	// Verify target content unchanged
	data, _ := os.ReadFile(target)
	if string(data) != "target content" {
		t.Errorf("target content was modified: %s", string(data))
	}
}

func TestDoSkip_TargetNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	if err := os.WriteFile(source, []byte("new content"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	meta := &FileMetadata{SourcePath: source, TargetPath: target}
	result, err := DoSkip(source, target, meta)
	if err != nil {
		t.Fatalf("DoSkip failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true when target does not exist")
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		t.Error("target should be created")
	}
}

// =============================================================================
// DoOverwrite 测试
// =============================================================================

func TestDoOverwrite_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	if err := os.WriteFile(source, []byte("new content"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	meta := &FileMetadata{SourcePath: source, TargetPath: target}
	result, err := DoOverwrite(source, target, meta)
	if err != nil {
		t.Fatalf("DoOverwrite failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true")
	}

	data, _ := os.ReadFile(target)
	if string(data) != "new content" {
		t.Errorf("target content mismatch: %s", string(data))
	}
}

// =============================================================================
// DoMerge 测试
// =============================================================================

func TestDoMerge_ConflictingFields(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	sourceContent := `---
type: api
title: New Title
description: New Description
tags:
  - source-tag
---

# New Body
Source body content
`
	targetContent := `---
type: api
title: Old Title
description: Old Description
tags:
  - target-tag
  - common
---

# Old Body
Target body content
`
	os.WriteFile(source, []byte(sourceContent), 0644)
	os.WriteFile(target, []byte(targetContent), 0644)

	meta := &FileMetadata{SourcePath: source, TargetPath: target}
	result, err := DoMerge(source, target, meta)
	if err != nil {
		t.Fatalf("DoMerge failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true")
	}

	merged := string(result.Content)

	// 保留目标字段
	if !contains(merged, "title: Old Title") {
		t.Error("target's title should be preserved")
	}
	if !contains(merged, "description: Old Description") {
		t.Error("target's description should be preserved")
	}

	// 正文保留目标
	if !contains(merged, "# Old Body") {
		t.Error("target's body should be preserved")
	}
	if contains(merged, "Source body content") {
		t.Error("source's body should NOT be in merged result")
	}

	// tags 取并集
	if !contains(merged, "source-tag") {
		t.Error("source-tag should be in merged tags")
	}
	if !contains(merged, "target-tag") {
		t.Error("target-tag should be in merged tags")
	}
	if !contains(merged, "common") {
		t.Error("common tag should be in merged tags")
	}
}

func TestDoMerge_NoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	os.WriteFile(source, []byte("source content"), 0644)
	os.WriteFile(target, []byte("target content"), 0644)

	meta := &FileMetadata{SourcePath: source, TargetPath: target}
	result, err := DoMerge(source, target, meta)
	if err != nil {
		t.Fatalf("DoMerge failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true when no frontmatter")
	}
	if string(result.Content) != "target content" {
		t.Errorf("expected target content preserved, got: %s", string(result.Content))
	}
}

func TestDoMerge_BodyPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	os.WriteFile(source, []byte("---\ntype: api\ntitle: New\n---\n# New Body\n"), 0644)
	os.WriteFile(target, []byte("---\ntype: api\ntitle: Old\n---\n# Old Body\nMore content\n"), 0644)

	meta := &FileMetadata{SourcePath: source, TargetPath: target}
	result, err := DoMerge(source, target, meta)
	if err != nil {
		t.Fatalf("DoMerge failed: %v", err)
	}

	if !contains(string(result.Content), "# Old Body") {
		t.Error("body should be preserved from target")
	}
	if contains(string(result.Content), "New Body") {
		t.Error("source's new body should NOT replace target's body")
	}
}

// =============================================================================
// DoPatch 测试
// =============================================================================

func TestDoPatch_SpecificFields(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	sourceContent := `---
type: api
title: New Title
description: New Description
extra_field: extra
---
# Body
`
	targetContent := `---
type: api
title: Old Title
description: Old Description
custom_field: custom
---
# Old Body
`
	os.WriteFile(source, []byte(sourceContent), 0644)
	os.WriteFile(target, []byte(targetContent), 0644)

	meta := &FileMetadata{
		SourcePath:  source,
		TargetPath:  target,
		PatchFields: []string{"title", "description"},
	}

	result, err := DoPatch(source, target, meta)
	if err != nil {
		t.Fatalf("DoPatch failed: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true")
	}

	patched := string(result.Content)

	// 更新的字段
	if !contains(patched, "title: New Title") {
		t.Error("title should be updated to source value")
	}
	if !contains(patched, "description: New Description") {
		t.Error("description should be updated to source value")
	}

	// 保留的字段
	if !contains(patched, "custom_field: custom") {
		t.Error("custom_field should be preserved")
	}
	if !contains(patched, "type: api") {
		t.Error("type should be preserved (not in patch fields)")
	}

	// 正文保留目标
	if !contains(patched, "# Old Body") {
		t.Error("body should be preserved from target")
	}
}

func TestDoPatch_EmptyFieldsUsesDefault(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "src.md")
	target := filepath.Join(tmpDir, "dst.md")

	os.WriteFile(source, []byte("---\ntype: api\ntitle: New\n---\n# Body\n"), 0644)
	os.WriteFile(target, []byte("---\ntype: api\ntitle: Old\n---\n# Old Body\n"), 0644)

	meta := &FileMetadata{
		SourcePath:  source,
		TargetPath:  target,
		PatchFields: nil, // 应使用默认值
	}

	result, err := DoPatch(source, target, meta)
	if err != nil {
		t.Fatalf("DoPatch failed: %v", err)
	}

	if !contains(string(result.Content), "title: New") {
		t.Error("title should be updated by default patch fields")
	}
	if !contains(string(result.Content), "# Old Body") {
		t.Error("body should be preserved")
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
