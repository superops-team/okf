package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDirEnvOverride(t *testing.T) {
	t.Setenv("OKF_ORT_DIR", "/tmp/okf-test-override")
	got, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/okf-test-override" {
		t.Fatalf("DefaultDir = %q, want override", got)
	}
}

func TestEnsureExtractsToCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OKF_ORT_DIR", dir)
	p, err := Ensure()
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"ORT lib": p.ORTLib, "model": p.Model, "tokenizer": p.Tokenizer,
	} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s 未落盘: %v", name, err)
		}
		if st.Size() == 0 {
			t.Fatalf("%s 为空文件", name)
		}
	}
	// 校验路径位于 OKF_ORT_DIR/okf/{ort,models}
	if filepath.Dir(p.ORTLib) != filepath.Join(dir, "okf", "ort") {
		t.Fatalf("ORT 路径异常: %s", p.ORTLib)
	}
	if filepath.Dir(p.Model) != filepath.Join(dir, "okf", "models") {
		t.Fatalf("模型路径异常: %s", p.Model)
	}
}

func TestEnsureReusesCacheWithoutRewrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OKF_ORT_DIR", dir)
	p1, err := Ensure()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(p1.ORTLib)
	if err != nil {
		t.Fatal(err)
	}
	// 二次调用应命中缓存（mtime 不变，未重写）
	if _, err := Ensure(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(p1.ORTLib)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("缓存未复用：文件被重写")
	}
}

func TestEnsureRewritesCorruptedFile(t *testing.T) {
	t.Setenv("OKF_ORT_DIR", t.TempDir())
	p1, err := Ensure()
	if err != nil {
		t.Fatal(err)
	}
	// 破坏 ORT 库文件
	if err := os.WriteFile(p1.ORTLib, []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}
	p2, err := Ensure()
	if err != nil {
		t.Fatalf("损坏文件应被重写而非报错: %v", err)
	}
	data, err := os.ReadFile(p2.ORTLib)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "corrupted" {
		t.Fatal("损坏文件未被重写")
	}
	if len(data) != len(ortLibData) {
		t.Fatalf("重写后大小 %d != 内嵌 %d", len(data), len(ortLibData))
	}
}
