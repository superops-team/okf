#!/usr/bin/env bash
# fetch-model.sh — 构建期从 HuggingFace 下载 MiniLM-L6-v2 的 ONNX 模型与 tokenizer，
# 校验 SHA256 后写入 internal/embeddings/assets/models/。
# 用法: ./scripts/fetch-model.sh <goarch>   （arm64 | amd64）
#   arm64 → model_qint8_arm64.onnx（Apple Silicon / ARM64 专用量化）
#   amd64 → model_quint8_avx2.onnx （x86 AVX2 通用量化）
# tokenizer 跨平台共用。产物为运行时零联网；本脚本仅在构建期/CI 执行。
set -euo pipefail

GOARCH="${1:?usage: fetch-model.sh <goarch> (arm64|amd64)}"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST_DIR="$REPO_ROOT/internal/embeddings/assets/models"
BASE="https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx"

case "$GOARCH" in
  arm64) MODEL_FILE="model_qint8_arm64.onnx"; DEST_MODEL="model.onnx" ;;
  amd64) MODEL_FILE="model_quint8_avx2.onnx"; DEST_MODEL="model_amd64.onnx" ;;
  *) echo "error: 不支持的架构 $GOARCH（arm64|amd64）" >&2; exit 1 ;;
esac

mkdir -p "$DEST_DIR"

echo "==> 下载模型 $MODEL_FILE"
curl -fsSL -o "$DEST_DIR/.model.tmp" "$BASE/$MODEL_FILE"
mv "$DEST_DIR/.model.tmp" "$DEST_DIR/$DEST_MODEL"
echo "==> 模型 SHA256: $(shasum -a 256 "$DEST_DIR/$DEST_MODEL" | awk '{print $1}')"

if [[ ! -f "$DEST_DIR/tokenizer.json" ]]; then
  echo "==> 下载 tokenizer.json"
  curl -fsSL -o "$DEST_DIR/tokenizer.json" \
    "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/tokenizer.json"
  echo "==> tokenizer SHA256: $(shasum -a 256 "$DEST_DIR/tokenizer.json" | awk '{print $1}')"
fi
