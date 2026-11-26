#!/usr/bin/env bash
set -euo pipefail

# build-standalone.sh — Build Flutter web UI and embed into standalone Go binary
# Usage: [VERSION=v1.0.0] [TARGET_OS=linux] [TARGET_ARCH=amd64] ./scripts/build-standalone.sh

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

VERSION="${VERSION:-$(git -C "${REPO_DIR}" describe --tags --always 2>/dev/null || echo "v0.1.0")}"
VERSION="${VERSION#backend/}"
[[ "${VERSION}" =~ ^v ]] || VERSION="v${VERSION}"

TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
EXT=""
if [[ "${TARGET_OS}" == "windows" ]]; then
  EXT=".exe"
fi

BUILD_DIR="${REPO_DIR}/build"
mkdir -p "${BUILD_DIR}"

VERSIONED_NAME="versionradar-${VERSION}-${TARGET_OS}-${TARGET_ARCH}${EXT}"
OUTPUT_ABS="${BUILD_DIR}/${VERSIONED_NAME}"

echo "==> 3. Building Go standalone binary (${VERSION}, ${TARGET_OS}/${TARGET_ARCH})..."
(
  cd "${REPO_DIR}/backend"
  GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" go build -ldflags="-s -w" -o "${OUTPUT_ABS}" ./cmd/server
)

cp -f "${OUTPUT_ABS}" "${BUILD_DIR}/versionradar${EXT}"

BINARY_SIZE="$(du -h "${OUTPUT_ABS}" | awk '{print $1}')"
echo ""
echo "✓ Built standalone binary successfully!"
echo "  Version: ${VERSION}"
echo "  Path:    ${OUTPUT_ABS}"
echo "  Alias:   ${BUILD_DIR}/versionradar${EXT}"
echo "  Size:    ${BINARY_SIZE}"
