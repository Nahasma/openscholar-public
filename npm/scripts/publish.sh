#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NPM_DIR="$(dirname "$SCRIPT_DIR")"
DRY_RUN=""

if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN="--dry-run"
  echo "==> DRY RUN MODE"
fi

# Publish platform packages first
echo "==> Publishing platform packages..."

PLATFORMS=(
  "darwin-arm64"
  "darwin-x64"
  "linux-x64"
  "linux-arm64"
  "win32-x64"
  "win32-arm64"
)

for platform in "${PLATFORMS[@]}"; do
  pkg_dir="$NPM_DIR/platforms/$platform"
  if [ -f "$pkg_dir/package.json" ]; then
    echo "  Publishing @openscholar/${platform}..."
    (cd "$pkg_dir" && npm publish --access public $DRY_RUN)
  else
    echo "  Skipping ${platform} (no package.json)"
  fi
done

# Publish main package
echo "==> Publishing main package..."
(cd "$NPM_DIR" && npm publish --access public $DRY_RUN)

echo "==> Done."
