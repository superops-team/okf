#!/usr/bin/env bash
#
# okf — One-click installer for Linux and macOS
# Downloads pre-built binary from GitHub Releases
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.sh | bash -s -- v1.2.0
#
# Environment variables:
#   OKF_VERSION   - specific version to install (default: latest)
#   OKF_INSTALL_DIR - install directory (default: /usr/local/bin or ~/.local/bin)

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
GITHUB_REPO="superops-team/okf"
BINARY_NAME="okf"
TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'okf-install')"

# ---------------------------------------------------------------------------
# ANSI colors (only if stdout is a terminal)
# ---------------------------------------------------------------------------
if [ -t 1 ]; then
  C_RED="\033[31m"
  C_GREEN="\033[32m"
  C_YELLOW="\033[33m"
  C_BLUE="\033[34m"
  C_CYAN="\033[36m"
  C_RESET="\033[0m"
else
  C_RED=""
  C_GREEN=""
  C_YELLOW=""
  C_BLUE=""
  C_CYAN=""
  C_RESET=""
fi

log_info()    { printf "${C_BLUE}  ▸${C_RESET} %s\n" "$*"; }
log_success() { printf "${C_GREEN}  ✓${C_RESET} %s\n" "$*"; }
log_warn()    { printf "${C_YELLOW}  ⚠${C_RESET} %s\n" "$*"; }
log_error()   { printf "${C_RED}  ✗${C_RESET} %s\n" "$*"; }

banner() {
  printf "\n"
  printf "${C_CYAN}╔══════════════════════════════════════╗${C_RESET}\n"
  printf "${C_CYAN}║  okf — Open Knowledge Format  installer  ║${C_RESET}\n"
  printf "${C_CYAN}╚══════════════════════════════════════╝${C_RESET}\n"
  printf "\n"
}

# ---------------------------------------------------------------------------
# Cleanup on exit
# ---------------------------------------------------------------------------
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Check for required tools
# ---------------------------------------------------------------------------
require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log_error "Required command not found: $1"
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Detect OS and architecture, normalize to GitHub asset naming convention
# ---------------------------------------------------------------------------
detect_os() {
  local os
  os="$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux|darwin) echo "$os" ;;
    *) log_error "Unsupported operating system: $os"; exit 1 ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m 2>/dev/null)"
  case "$arch" in
    x86_64|amd64)     echo "amd64" ;;
    aarch64|arm64)     echo "arm64" ;;
    *) log_error "Unsupported architecture: $arch"; exit 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# Resolve the latest release version via GitHub API
# ---------------------------------------------------------------------------
resolve_latest_version() {
  local url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
  local resp
  if ! resp="$(curl -fsSL "$url" 2>/dev/null)"; then
    log_error "Failed to query GitHub API for latest release"
    log_warn "Check your network or specify a version explicitly:"
    log_warn "  curl -fsSL https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.sh | bash -s -- v1.2.0"
    exit 1
  fi
  local tag
  tag="$(printf '%s' "$resp" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed -E 's/.*"([^"]*)"$/\1/')"
  if [ -z "$tag" ]; then
    log_error "Could not parse tag_name from GitHub API response"
    exit 1
  fi
  echo "$tag"
}

# ---------------------------------------------------------------------------
# Determine install dir (prefer /usr/local/bin if writable, else ~/.local/bin)
# ---------------------------------------------------------------------------
resolve_install_dir() {
  if [ -n "${OKF_INSTALL_DIR:-}" ]; then
    echo "$OKF_INSTALL_DIR"
    return
  fi
  if [ -w /usr/local/bin ] || sudo -n true 2>/dev/null; then
    echo "/usr/local/bin"
  else
    echo "$HOME/.local/bin"
  fi
}

# ---------------------------------------------------------------------------
# Download a file with curl; follow redirects and retry on transient failures
# ---------------------------------------------------------------------------
download() {
  local url="$1"
  local dest="$2"
  log_info "Downloading: $url"
  curl -fsSL --retry 3 --retry-connrefused --connect-timeout 30 \
       -o "$dest" "$url"
}

# ---------------------------------------------------------------------------
# Verify checksum against SHA256SUMS
# ---------------------------------------------------------------------------
verify_checksum() {
  local asset_name="$1"
  local checksums_file="$2"
  if [ ! -f "$checksums_file" ]; then
    log_warn "SHA256SUMS not found, skipping verification"
    return 0
  fi
  local expected actual
  expected="$(grep -E "( |\*|^)${asset_name}\$" "$checksums_file" | awk '{print $1}' || true)"
  if [ -z "$expected" ]; then
    log_warn "No checksum entry for $asset_name, skipping verification"
    return 0
  fi
  actual="$(sha256sum "$asset_name" | awk '{print $1}')"
  if [ "$expected" != "$actual" ]; then
    log_error "Checksum mismatch for $asset_name"
    log_error "  expected: $expected"
    log_error "  actual  : $actual"
    exit 1
  fi
  log_success "Checksum verified (SHA256)"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  banner

  require "curl"
  require "tar"

  local OS ARCH VERSION INSTALL_DIR
  OS="$(detect_os)"
  ARCH="$(detect_arch)"

  log_info "Detected system: ${OS}/${ARCH}"

  # Resolve version: arg -> env -> latest
  if [ -n "${1:-}" ]; then
    VERSION="$1"
  elif [ -n "${OKF_VERSION:-}" ]; then
    VERSION="${OKF_VERSION}"
  else
    log_info "Resolving latest release version..."
    VERSION="$(resolve_latest_version)"
  fi
  VERSION="${VERSION#v}"
  log_info "Version: v${VERSION}"

  local asset_ext="tar.gz"
  local asset_name="${BINARY_NAME}_${VERSION}_${OS}_${ARCH}.${asset_ext}"
  local download_url="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/${asset_name}"
  local checksums_url="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/SHA256SUMS"

  # Download
  (
    cd "$TMP_DIR"
    download "$download_url" "$asset_name"
    download "$checksums_url" "SHA256SUMS" 2>/dev/null || true
    verify_checksum "$asset_name" "SHA256SUMS"

    log_info "Extracting archive..."
    tar -xzf "$asset_name"
  )

  # Resolve install dir
  INSTALL_DIR="$(resolve_install_dir)"
  mkdir -p "$INSTALL_DIR"

  # Locate binary
  local binary_path
  binary_path="$(find "$TMP_DIR" -type f -name "${BINARY_NAME}" | head -1)"
  if [ -z "$binary_path" ]; then
    log_error "Could not find binary ${BINARY_NAME} in archive"
    exit 1
  fi

  # Install
  if [ -w "$INSTALL_DIR" ]; then
    install -m 0755 "$binary_path" "$INSTALL_DIR/${BINARY_NAME}"
  else
    log_info "Requesting elevated permissions to write to $INSTALL_DIR..."
    sudo install -m 0755 "$binary_path" "$INSTALL_DIR/${BINARY_NAME}"
  fi

  # Verify PATH
  local installed="$INSTALL_DIR/${BINARY_NAME}"
  chmod +x "$installed"

  # PATH check and hint
  if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
    log_warn "$INSTALL_DIR is not in your PATH."
    log_info "  Add it by appending to your shell config (~/.bashrc, ~/.zshrc, ~/.profile):"
    printf "       export PATH=\"%s:\\$PATH\"\n" "$INSTALL_DIR"
  fi

  # Verify installation
  log_success "Installed okf to ${installed}"
  local version_output
  if version_output="$("$installed" --version 2>&1)" || \
     version_output="$("$installed" version 2>&1)" || \
     version_output="$( "$installed" 2>&1 | head -5 )"; then
    :
  fi
  log_info "Binary info: $(file "$installed" | head -1 | cut -d: -f2)"

  printf "\n"
  printf "${C_GREEN}═══════════════════════════════════${C_RESET}\n"
  printf "${C_GREEN}  Installation complete!         ${C_RESET}\n"
  printf "${C_GREEN}═══════════════════════════════════${C_RESET}\n"
  printf "  Run:  okf --help\n"
  printf "\n"
}

main "$@"
