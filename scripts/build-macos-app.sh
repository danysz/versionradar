#!/usr/bin/env bash
set -euo pipefail

# build-macos-app.sh — Build the Go sidecar binary and bundle it inside the macOS Flutter app.
# Usage: [TARGET_ARCH=arm64] ./scripts/build-macos-app.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "Error: build-macos-app.sh must be run on macOS (Xcode required for flutter build macos)." >&2
  exit 1
fi

if command -v flutter >/dev/null 2>&1; then
  FLUTTER_BIN="$(command -v flutter)"
elif [[ -x "/Users/Shared/flutter/bin/flutter" ]]; then
  FLUTTER_BIN="/Users/Shared/flutter/bin/flutter"
else
  echo "Error: 'flutter' was not found in PATH or at /Users/Shared/flutter/bin/flutter." >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Error: 'go' was not found in PATH." >&2
  exit 1
fi

TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
SIDECAR_STAGE="${REPO_DIR}/client/macos/Runner/Resources/versionradar-server"

echo "==> 1. Building versionradar-server (darwin/${TARGET_ARCH})..."
mkdir -p "$(dirname "${SIDECAR_STAGE}")"
(
  cd "${REPO_DIR}/backend"
  GOOS=darwin GOARCH="${TARGET_ARCH}" go build -ldflags="-s -w" -o "${SIDECAR_STAGE}" ./cmd/server
)
chmod +x "${SIDECAR_STAGE}"

echo "==> 2. Building Flutter macOS app (release)..."
(
  cd "${REPO_DIR}/client"
  "${FLUTTER_BIN}" build macos --release
)

APP_BUNDLE="${REPO_DIR}/client/build/macos/Build/Products/Release/VersionRadar.app"
BUNDLED_SIDECAR="${APP_BUNDLE}/Contents/Resources/versionradar-server"

if [[ ! -x "${BUNDLED_SIDECAR}" ]]; then
  echo "==> 3. Copying sidecar into ${APP_BUNDLE}/Contents/Resources/..."
  mkdir -p "${APP_BUNDLE}/Contents/Resources"
  cp -f "${SIDECAR_STAGE}" "${BUNDLED_SIDECAR}"
  chmod +x "${BUNDLED_SIDECAR}"
  codesign --force --deep --sign - "${APP_BUNDLE}"
fi

echo ""
echo "✓ Built VersionRadar.app with bundled sidecar!"
echo "  App:     ${APP_BUNDLE}"
echo "  Sidecar: ${BUNDLED_SIDECAR}"
echo "  Size:    $(du -sh "${APP_BUNDLE}" | awk '{print $1}')"
