#!/usr/bin/env bash
set -euo pipefail

# build-standalone.sh — Build Flutter web UI and embed into standalone Go binary
# Usage: [TARGET_OS=linux] [TARGET_ARCH=amd64] ./scripts/build-standalone.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if command -v flutter >/dev/null 2>&1; then
  FLUTTER_BIN="$(command -v flutter)"
elif [[ -x "/Users/Shared/flutter/bin/flutter" ]]; then
  FLUTTER_BIN="/Users/Shared/flutter/bin/flutter"
else
  echo "Error: 'flutter' was not found in PATH or at /Users/Shared/flutter/bin/flutter." >&2
  echo "Please install Flutter or add it to your PATH and try again." >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Error: 'go' was not found in PATH." >&2
  exit 1
fi

echo "==> 1. Building Flutter web app (release)..."
(
  cd "${REPO_DIR}/client"
  "${FLUTTER_BIN}" build web --release
)

echo "==> 2. Copying web build to backend/server/dist..."
DIST_DIR="${REPO_DIR}/backend/server/dist"
mkdir -p "${DIST_DIR}"
rm -rf "${DIST_DIR:?}"/*
cp -r "${REPO_DIR}/client/build/web"/* "${DIST_DIR}/"

TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
EXT=""
if [[ "${TARGET_OS}" == "windows" ]]; then
  EXT=".exe"
fi

OUTPUT_REL="bin/versionradar${EXT}"
OUTPUT_ABS="${REPO_DIR}/backend/${OUTPUT_REL}"

echo "==> 3. Building Go standalone binary (${TARGET_OS}/${TARGET_ARCH})..."
mkdir -p "${REPO_DIR}/backend/bin"
(
  cd "${REPO_DIR}/backend"
  GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" go build -ldflags="-s -w" -o "${OUTPUT_REL}" ./cmd/server
)

BINARY_SIZE="$(du -h "${OUTPUT_ABS}" | awk '{print $1}')"
echo ""
echo "✓ Built standalone binary successfully!"
echo "  Path: ${OUTPUT_ABS}"
echo "  Size: ${BINARY_SIZE}"
