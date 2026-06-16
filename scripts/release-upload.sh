#!/usr/bin/env bash
#
# okf — Release upload script
# Creates or updates a GitHub release and uploads build artifacts
#
# Usage: GITHUB_TOKEN=<pat> ./scripts/release-upload.sh 1.2.0

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

GITHUB_REPO="superops-team/okf"
VERSION="${1:-}"
DIST_DIR="$REPO_ROOT/dist"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi

VERSION="${VERSION#v}"
TAG="v${VERSION}"

# Resolve token
TOKEN="${GITHUB_TOKEN:-}"
if [ -z "$TOKEN" ]; then
  remote_url="$(git remote get-url origin 2>/dev/null || true)"
  if [[ "$remote_url" =~ ^https://([^@]+)@github\.com/ ]]; then
    TOKEN="${BASH_REMATCH[1]}"
  fi
fi

if [ -z "$TOKEN" ]; then
  echo "Error: GITHUB_TOKEN is not set"
  exit 1
fi

API="https://api.github.com"
UPLOADS="https://uploads.github.com"
AUTH=(-H "Authorization: token ${TOKEN}" -H "Accept: application/vnd.github+json")

log() { echo "  [+] $*"; }

# Build release body
BODY="## okf v${VERSION}

Project-level knowledge base system for AI Agents.

### Install

**Linux / macOS:**
\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/superops-team/okf/${TAG}/scripts/install.sh | bash
\`\`\`

**Windows (PowerShell):**
\`\`\`powershell
iwr -useb https://raw.githubusercontent.com/superops-team/okf/${TAG}/scripts/install.ps1 | iex
\`\`\`

### Build from source
\`\`\`bash
go install github.com/superops-team/okf/cmd/okf@${TAG}
\`\`\`
"

# Create release (draft first to ensure it exists)
log "Creating release ${TAG}..."

RELEASE_ID=$(curl -fsSL "${AUTH[@]}" \
  "${API}/repos/${GITHUB_REPO}/releases/tags/${TAG}" 2>/dev/null \
  | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d.get("id",""))' 2>/dev/null || true)

if [ -z "$RELEASE_ID" ]; then
  PAYLOAD=$(python3 -c "
import json,sys
body=sys.stdin.read()
print(json.dumps({
    'tag_name': sys.argv[1],
    'name': 'okf v' + sys.argv[1].lstrip('v'),
    'body': body,
    'draft': False,
    'prerelease': False
}))
" "$TAG" <<<"$BODY")

  RELEASE_ID=$(curl -fsSL "${AUTH[@]}" -d "$PAYLOAD" \
    "${API}/repos/${GITHUB_REPO}/releases" \
    | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
  log "Release created: ID=${RELEASE_ID}"
else
  log "Release already exists: ID=${RELEASE_ID}"
fi

# List existing assets to avoid duplicates
EXISTING=$(curl -fsSL "${AUTH[@]}" \
  "${API}/repos/${GITHUB_REPO}/releases/${RELEASE_ID}/assets" \
  | python3 -c 'import json,sys;[print(a["name"]) for a in json.load(sys.stdin)]')

log "Existing assets: $(echo "$EXISTING" | tr '\n' ', ')"

# Upload artifacts
ARTIFACTS=(
  "okf_${VERSION}_linux_amd64.tar.gz"
  "okf_${VERSION}_linux_arm64.tar.gz"
  "okf_${VERSION}_darwin_amd64.tar.gz"
  "okf_${VERSION}_darwin_arm64.tar.gz"
  "okf_${VERSION}_windows_amd64.zip"
  "okf_${VERSION}_windows_arm64.zip"
  "SHA256SUMS"
)

for artifact in "${ARTIFACTS[@]}"; do
  path="${DIST_DIR}/${artifact}"
  if [ ! -f "$path" ]; then
    echo "  [!] Missing: $artifact"
    continue
  fi
  if echo "$EXISTING" | grep -qx "$artifact"; then
    log "  skip: $artifact (exists)"
    continue
  fi

  mime=$(file --mime-type -b "$path" 2>/dev/null || echo "application/octet-stream")
  log "  upload: $artifact ($mime)"
  curl -fsSL "${AUTH[@]}" -H "Content-Type: $mime" \
    --data-binary @"$path" \
    "${UPLOADS}/repos/${GITHUB_REPO}/releases/${RELEASE_ID}/assets?name=${artifact}" \
    >/dev/null
done

log ""
log "Done! https://github.com/${GITHUB_REPO}/releases/tag/${TAG}"
