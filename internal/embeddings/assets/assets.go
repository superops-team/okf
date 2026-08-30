// Package assets 内嵌并解包向量化所需静态资源（ONNX Runtime 动态库 + MiniLM 模型 + tokenizer）。
//
// 内嵌资源按平台通过 build tag 裁剪（见 ort_<goos>_<goarch>.go / model_<goarch>.go），
// 产物为单文件自包含；运行时解包到用户缓存目录并做 SHA256 校验后复用，全程不联网。
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrChecksumMismatch 表示缓存文件与内嵌清单 SHA256 不一致。
var ErrChecksumMismatch = errors.New("embedded asset checksum mismatch")

// Paths 返回解包后各资源的本地路径。
type Paths struct {
	ORTLib    string // ONNX Runtime 动态库
	Model     string // MiniLM 模型（.onnx）
	Tokenizer string // tokenizer.json
}

// DefaultDir 返回资源缓存根目录；环境变量 OKF_ORT_DIR 可覆盖默认的用户缓存目录。
func DefaultDir() (string, error) {
	if v := os.Getenv("OKF_ORT_DIR"); v != "" {
		return v, nil
	}
	return os.UserCacheDir()
}

// Ensure 确保内嵌资源解包到 <cache>/okf/{ort,models}/ 并通过 SHA256 校验，
// 返回各资源路径。命中缓存（校验一致）时跳过写盘；全程不发起网络请求。
// 资源损坏时返回含处置建议的错误（删除缓存后重试 / okf vector rebuild）。
func Ensure() (Paths, error) {
	base, err := DefaultDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve cache dir: %w", err)
	}
	ortDir := filepath.Join(base, "okf", "ort")
	modelDir := filepath.Join(base, "okf", "models")

	libPath, err := ensureFile(ortDir, ortLibName, ortLibData, ortLibSHA256, 0o755)
	if err != nil {
		return Paths{}, fmt.Errorf("ensure ORT library (删除 %s 后重试): %w", ortDir, err)
	}
	modelPath, err := ensureFile(modelDir, modelName, modelData, modelSHA256, 0o644)
	if err != nil {
		return Paths{}, fmt.Errorf("ensure model (删除 %s 后重试): %w", modelDir, err)
	}
	tokPath, err := ensureFile(modelDir, "tokenizer.json", tokenizerData, tokenizerSHA256, 0o644)
	if err != nil {
		return Paths{}, fmt.Errorf("ensure tokenizer: %w", err)
	}
	return Paths{ORTLib: libPath, Model: modelPath, Tokenizer: tokPath}, nil
}

// ensureFile 原子写盘（临时文件 + rename）+ SHA256 校验 + 缓存复用。
func ensureFile(dir, name string, data []byte, wantSHA string, mode os.FileMode) (string, error) {
	path := filepath.Join(dir, name)

	if ok, err := verifyFile(path, wantSHA); err != nil {
		return "", err
	} else if ok {
		return path, nil // 缓存命中
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}

	ok, err := verifyFile(path, wantSHA)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrChecksumMismatch
	}
	return path, nil
}

func verifyFile(path, wantSHA string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != wantSHA {
		return false, nil // 损坏：调用方将重写
	}
	return true, nil
}
