#!/usr/bin/env bash
set -euo pipefail

VERSION="${OPENSCHOLAR_VERSION:-v0.1.0-alpha.4}"
MODULE="github.com/Nahasma/openscholar-public"
BIN_DIR="${GOBIN:-$(go env GOPATH)/bin}"

mkdir -p "$BIN_DIR"

GOBIN="$BIN_DIR" go install "${MODULE}@${VERSION}"

SOURCE_BIN="$BIN_DIR/openscholar-public"
TARGET_BIN="$BIN_DIR/openscholar"

if [ -f "$SOURCE_BIN" ]; then
  mv "$SOURCE_BIN" "$TARGET_BIN"
  chmod +x "$TARGET_BIN" 2>/dev/null || true
fi

echo "OpenScholar installed to $TARGET_BIN"
