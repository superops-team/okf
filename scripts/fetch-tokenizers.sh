#!/usr/bin/env bash
# fetch-tokenizers.sh — 构建期下载 pure-tokenizers 原生动态库，
# 校验官方发布归档 SHA256 后提取到 internal/embeddings/assets/libs/<goos>/<goarch>/。
# 用法: ./scripts/fetch-tokenizers.sh <goos> <goarch> [native_version]
#   例: ./scripts/fetch-tokenizers.sh linux amd64 rust-v0.1.5
# 产物通过 go:embed 内嵌，运行时不下载动态库。
set -euo pipefail

GOOS="${1:?usage: fetch-tokenizers.sh <goos> <goarch> [native_version]}"
GOARCH="${2:?usage: fetch-tokenizers.sh <goos> <goarch> [native_version]}"
NATIVE_VERSION="${3:-rust-v0.1.5}"

if [[ "$NATIVE_VERSION" != "rust-v0.1.5" ]]; then
  echo "error: unsupported native version $NATIVE_VERSION; update pinned checksums before upgrading" >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST_DIR="$REPO_ROOT/internal/embeddings/assets/libs/$GOOS/$GOARCH"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

case "$GOOS/$GOARCH" in
  linux/amd64)
    ASSET="libtokenizers-x86_64-unknown-linux-gnu.tar.gz"
    LIB="libtokenizers.so"
    ARCHIVE_SHA256="8c154b6411f96dc299fd6a18520c074ac1469940f90ded0009ffba28066eeb62"
    ;;
  linux/arm64)
    ASSET="libtokenizers-aarch64-unknown-linux-gnu.tar.gz"
    LIB="libtokenizers.so"
    ARCHIVE_SHA256="771508ec032bfa6d7d9a7a5579342370f90dc78c4013a0f25b0d5c4d332ceb1a"
    ;;
  darwin/amd64)
    ASSET="libtokenizers-x86_64-apple-darwin.tar.gz"
    LIB="libtokenizers.dylib"
    ARCHIVE_SHA256="904f5619ca6aa8e729c9fc3b5f5eeb901367dfbfa2c3f446f2087e266aef6e59"
    ;;
  darwin/arm64)
    ASSET="libtokenizers-aarch64-apple-darwin.tar.gz"
    LIB="libtokenizers.dylib"
    ARCHIVE_SHA256="b51a33e951121a2930d322d8419321e19ecf1d4a8cc3205d6436233e2fde8ac6"
    ;;
  windows/amd64)
    ASSET="libtokenizers-x86_64-pc-windows-msvc.tar.gz"
    LIB="tokenizers.dll"
    ARCHIVE_SHA256="4df138fec675462045b724e4406a4a35c265add8025ab0f1f0af3575c66e1c09"
    ;;
  *)
    echo "error: unsupported platform $GOOS/$GOARCH" >&2
    exit 1
    ;;
esac

URL="https://releases.amikos.tech/pure-tokenizers/${NATIVE_VERSION}/${ASSET}"
echo "==> downloading $ASSET ($NATIVE_VERSION)"
curl -fsSL -o "$TMP/$ASSET" "$URL"
ACTUAL_SHA256="$(sha256_file "$TMP/$ASSET")"
if [[ "$ACTUAL_SHA256" != "$ARCHIVE_SHA256" ]]; then
  echo "error: checksum mismatch for $ASSET: expected $ARCHIVE_SHA256, got $ACTUAL_SHA256" >&2
  exit 1
fi
echo "$ASSET: OK"

tar -xzf "$TMP/$ASSET" -C "$TMP"
SRC="$TMP/$LIB"
if [[ ! -f "$SRC" ]]; then
  echo "error: archive does not contain $LIB" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"
cp "$SRC" "$DEST_DIR/$LIB"
chmod 0755 "$DEST_DIR/$LIB"

echo "==> wrote $DEST_DIR/$LIB"
echo "==> SHA256: $(sha256_file "$DEST_DIR/$LIB")"
