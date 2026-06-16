#!/usr/bin/env bash
#
# okf — Cross-platform release build script
# Builds static binaries for Linux, macOS, Windows (amd64 + arm64)
#
# Usage: ./scripts/build-release.sh [version]
#   version: defaults to git tag (vX.Y.Z) or pkg/okf/meta/version.go

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

BINARY_NAME="okf"
OUTPUT_DIR="${OUTPUT_DIR:-$REPO_ROOT/dist}"
MODULE_PATH="github.com/superops-team/okf"

detect_version() {
  local v
  v="$(git describe --tags --abbrev=0 2>/dev/null || true)"
  if [[ -n "$v" ]]; then
    # strip leading 'v' if present for ldflags
    echo "${v#v}"
    return
  fi
  # fallback: read from source
  v="$(grep -E 'Version\s*=' "$REPO_ROOT/pkg/okf/meta/version.go" | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"
  echo "$v"
}

VERSION="${1:-$(detect_version)}"
BUILD_DATE="$(date -u +%Y-%m-%d)"
LD_FLAGS="-s -w -X ${MODULE_PATH}/pkg/okf/meta.Version=${VERSION} -X ${MODULE_PATH}/pkg/okf/meta.BuildDate=${BUILD_DATE}"

echo "=== okf release builder ==="
echo "  Module : ${MODULE_PATH}"
echo "  Version: ${VERSION}"
echo "  Date   : ${BUILD_DATE}"
echo "  Output : ${OUTPUT_DIR}"
echo ""

mkdir -p "$OUTPUT_DIR"
rm -rf "${OUTPUT_DIR:?}"/*

PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

build_platform() {
  local os="$1"
  local arch="$2"
  local ext=""
  if [[ "$os" == "windows" ]]; then
    ext=".exe"
  fi

  local arch_dir="${OUTPUT_DIR}/${BINARY_NAME}_${VERSION}_${os}_${arch}"
  mkdir -p "$arch_dir"

  echo "  ▸ ${os}/${arch}..."

  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath \
      -ldflags "$LD_FLAGS" \
      -o "${arch_dir}/${BINARY_NAME}${ext}" \
      ./cmd/okf/

  # LICENSE + README
  cp "$REPO_ROOT/README.md" "$arch_dir/" 2>/dev/null || true

  # Create archive
  local archive_name="${BINARY_NAME}_${VERSION}_${os}_${arch}"
  if [[ "$os" == "windows" ]]; then
    (cd "$OUTPUT_DIR" && zip -r -q "${archive_name}.zip" "$(basename "$arch_dir")")
  else
    tar -C "$OUTPUT_DIR" -czf "${OUTPUT_DIR}/${archive_name}.tar.gz" "$(basename "$arch_dir")"
  fi
}

# Build each platform
for p in "${PLATFORMS[@]}"; do
  os="${p%/*}"
  arch="${p#*/}"
  build_platform "$os" "$arch"
done

# Generate checksums
echo ""
echo "  ▸ Generating SHA256SUMS..."
(
  cd "$OUTPUT_DIR"
  sha256sum *.tar.gz *.zip > "SHA256SUMS"
  cat "SHA256SUMS"
)

echo ""
echo "=== Done ==="
echo "Artifacts in: ${OUTPUT_DIR}"
ls -lh "$OUTPUT_DIR"
