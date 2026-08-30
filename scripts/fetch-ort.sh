#!/usr/bin/env bash
# fetch-ort.sh — 构建期从 ONNX Runtime 官方 release 下载指定平台的 CPU 动态库，
# 校验 SHA256 后提取到 internal/embeddings/assets/libs/<goos>/<goarch>/。
# 用法: ./scripts/fetch-ort.sh <goos> <goarch> [ort_version]
#   例: ./scripts/fetch-ort.sh linux amd64 1.24.1
#       ./scripts/fetch-ort.sh darwin arm64 1.24.1
#       ./scripts/fetch-ort.sh windows amd64 1.24.1
# 产物为运行时零联网；本脚本仅在构建期/CI 执行。
set -euo pipefail

GOOS="${1:?usage: fetch-ort.sh <goos> <goarch> [ort_version]}"
GOARCH="${2:?usage: fetch-ort.sh <goos> <goarch> [ort_version]}"
ORT_VERSION="${3:-1.24.1}"
# darwin/amd64：官方自 1.24.1 起不再发布 x86_64 macOS 包，固定使用 1.23.1
if [[ "$GOOS/$GOARCH" == "darwin/amd64" && "$ORT_VERSION" == "1.24.1" ]]; then
  echo "==> darwin/amd64 使用 ORT 1.23.1（官方 1.24.1 无 x86_64 包）"
  ORT_VERSION="1.23.1"
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST_DIR="$REPO_ROOT/internal/embeddings/assets/libs/$GOOS/$GOARCH"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 官方 release 包名映射（CPU 版）
case "$GOOS/$GOARCH" in
  linux/amd64)   ASSET="onnxruntime-linux-x64-${ORT_VERSION}.tgz"; LIB="libonnxruntime.so" ;;
  linux/arm64)   ASSET="onnxruntime-linux-aarch64-${ORT_VERSION}.tgz"; LIB="libonnxruntime.so" ;;
  darwin/amd64)  ASSET="onnxruntime-osx-x86_64-${ORT_VERSION}.tgz"; LIB="libonnxruntime.dylib" ;;
  darwin/arm64)  ASSET="onnxruntime-osx-arm64-${ORT_VERSION}.tgz"; LIB="libonnxruntime.dylib" ;;
  windows/amd64) ASSET="onnxruntime-win-x64-${ORT_VERSION}.zip"; LIB="onnxruntime.dll" ;;
  *) echo "error: 不支持的平台 $GOOS/$GOARCH" >&2; exit 1 ;;
esac

URL="https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/${ASSET}"
echo "==> 下载 $ASSET (v$ORT_VERSION)"
curl -fsSL -o "$TMP/pkg" "$URL"

mkdir -p "$TMP/x"
if [[ "$ASSET" == *.zip ]]; then
  unzip -q "$TMP/pkg" -d "$TMP/x"
else
  tar -xzf "$TMP/pkg" -C "$TMP/x"
fi

# 官方包内 libonnxruntime.so/.dylib 通常是符号链接，需同时匹配链接并跟随复制内容
SRC="$(find "$TMP/x" \( -type f -o -type l \) -name "$LIB" | head -1)"
if [[ -z "$SRC" ]]; then
  echo "error: 包内未找到 $LIB" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"
cp -L "$SRC" "$DEST_DIR/$LIB"
chmod 0755 "$DEST_DIR/$LIB"

echo "==> 已写入 $DEST_DIR/$LIB"
echo "==> SHA256: $(shasum -a 256 "$DEST_DIR/$LIB" | awk '{print $1}')"
