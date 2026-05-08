#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?Usage: build.sh <version>}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NPM_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$NPM_DIR")"
DIST_DIR="${DIST_DIR:-$(dirname "$NPM_DIR")/dist}"

echo "==> Building npm packages for v${VERSION}"

# Platform mapping: npm_dir -> goreleaser archive name
declare -A PLATFORM_MAP=(
  ["darwin-arm64"]="openscholar_darwin_arm64"
  ["darwin-x64"]="openscholar_darwin_amd64"
  ["linux-x64"]="openscholar_linux_amd64"
  ["linux-arm64"]="openscholar_linux_arm64"
  ["win32-x64"]="openscholar_windows_amd64"
  ["win32-arm64"]="openscholar_windows_arm64"
)

# Update version in all package.json files
echo "==> Updating versions to ${VERSION}"

copy_license() {
  local target_dir="$1"
  if [ -f "$REPO_ROOT/LICENSE" ]; then
    cp "$REPO_ROOT/LICENSE" "$target_dir/LICENSE"
  fi
}

# Main package
tmp=$(mktemp)
jq --arg v "$VERSION" '
  .version = $v |
  .optionalDependencies = (.optionalDependencies | to_entries | map(.value = $v) | from_entries)
' "$NPM_DIR/package.json" > "$tmp" && mv "$tmp" "$NPM_DIR/package.json"
copy_license "$NPM_DIR"

# Platform packages
for platform_dir in "$NPM_DIR"/platforms/*/; do
  pkg_json="$platform_dir/package.json"
  if [ -f "$pkg_json" ]; then
    tmp=$(mktemp)
    jq --arg v "$VERSION" '.version = $v' "$pkg_json" > "$tmp" && mv "$tmp" "$pkg_json"
    copy_license "$platform_dir"
  fi
done

# Extract binaries from GoReleaser dist
echo "==> Extracting binaries from ${DIST_DIR}"

for platform in "${!PLATFORM_MAP[@]}"; do
  goreleaser_name="${PLATFORM_MAP[$platform]}"
  platform_dir="$NPM_DIR/platforms/$platform"

  # Determine archive extension and binary name
  if [[ "$platform" == win32-* ]]; then
    archive="${DIST_DIR}/${goreleaser_name}.zip"
    binary_name="openscholar.exe"
  else
    archive="${DIST_DIR}/${goreleaser_name}.tar.gz"
    binary_name="openscholar"
  fi

  if [ ! -f "$archive" ]; then
    echo "  Warning: ${archive} not found, skipping ${platform}"
    continue
  fi

  echo "  Extracting ${platform}..."
  tmpdir=$(mktemp -d)

  if [[ "$archive" == *.tar.gz ]]; then
    tar -xzf "$archive" -C "$tmpdir"
  else
    unzip -q "$archive" -d "$tmpdir"
  fi

  # Find and copy binary
  found=$(find "$tmpdir" -name "$binary_name" -type f | head -1)
  if [ -n "$found" ]; then
    cp "$found" "$platform_dir/$binary_name"
    chmod +x "$platform_dir/$binary_name" 2>/dev/null || true
    echo "  ✓ ${platform}"
  else
    echo "  ✗ Binary not found in archive for ${platform}"
  fi

  rm -rf "$tmpdir"
done

echo "==> Done. Ready to publish."
