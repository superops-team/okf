package vectorindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 索引格式版本：分块级索引的 key 形如 "<指纹>#<序号>"，与旧的概念级 key 不兼容。
// 加载旧格式必须明确报错并提示 rebuild，不得静默降级产生错误结果。

func TestSaveWritesIndexFormatVersion(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(3)
	idx.Add("a#0", []float32{1, 0, 0})
	if err := idx.Save(dir, Meta{Model: "m", OkfVersion: "v"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, MetaFileName))
	if err != nil {
		t.Fatal(err)
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.IndexFormatVersion != CurrentIndexFormatVersion {
		t.Fatalf("IndexFormatVersion = %d，期望 %d", m.IndexFormatVersion, CurrentIndexFormatVersion)
	}
}

func TestLoadRejectsMissingFormatVersion(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(3)
	idx.Add("a", []float32{1, 0, 0})
	if err := idx.Save(dir, Meta{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	// 模拟旧版本产物：抹掉 index_format_version 字段
	path := filepath.Join(dir, MetaFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	delete(generic, "index_format_version")
	patched, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, patched, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := NewHNSW(3)
	_, err = loaded.Load(dir)
	if err == nil {
		t.Fatal("缺少 index_format_version 未报错（会静默使用旧索引）")
	}
	if !strings.Contains(err.Error(), "rebuild") {
		t.Errorf("错误信息未提示 rebuild: %v", err)
	}
}

func TestLoadRejectsOlderFormatVersion(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(3)
	idx.Add("a", []float32{1, 0, 0})
	if err := idx.Save(dir, Meta{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, MetaFileName)
	raw, _ := os.ReadFile(path)
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.IndexFormatVersion = CurrentIndexFormatVersion - 1
	patched, _ := json.Marshal(m)
	if err := os.WriteFile(path, patched, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := NewHNSW(3)
	if _, err := loaded.Load(dir); err == nil {
		t.Fatal("旧格式版本未报错")
	}
}

// 当前版本必须能正常往返，且不触发自动重建（避免隐式长耗时操作）。
func TestLoadAcceptsCurrentFormatVersion(t *testing.T) {
	dir := t.TempDir()
	idx := NewHNSW(3)
	idx.Add("a#0", []float32{1, 0, 0})
	idx.Add("a#1", []float32{0, 1, 0})
	if err := idx.Save(dir, Meta{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	loaded := NewHNSW(3)
	meta, err := loaded.Load(dir)
	if err != nil {
		t.Fatalf("当前格式应可加载: %v", err)
	}
	if meta.IndexFormatVersion != CurrentIndexFormatVersion {
		t.Errorf("加载的版本 = %d，期望 %d", meta.IndexFormatVersion, CurrentIndexFormatVersion)
	}
	if loaded.Len() != 2 {
		t.Errorf("Len = %d，期望 2", loaded.Len())
	}
}
