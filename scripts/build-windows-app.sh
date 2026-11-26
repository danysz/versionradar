#!/usr/bin/env bash
set -euo pipefail

# build-windows-app.sh — Build versionradar-server.exe and (on Windows) the Flutter Windows release bundle.
#
# Notes on cross-compiling:
# - `versionradar-server.exe` (Go) CAN be cross-compiled from macOS or Linux using GOOS=windows GOARCH=amd64.
# - `flutter build windows --release` CANNOT be cross-compiled from macOS/Linux; it requires a Windows host
#   with Visual Studio (Desktop development with C++) and `client/windows/` scaffolded via:
#     cd client && flutter create --platforms=windows .
#
# Usage:
#   On macOS/Linux (cross-compiles sidecar to build/windows/versionradar-server.exe):
#     ./scripts/build-windows-app.sh
#   On Windows (Git Bash / MSYS2):
#     ./scripts/build-windows-app.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "Error: 'go' was not found in PATH." >&2
  exit 1
fi

TARGET_ARCH="${TARGET_ARCH:-amd64}"
WIN_STAGE_DIR="${REPO_DIR}/build/windows"
SIDECAR_EXE="${WIN_STAGE_DIR}/versionradar-server.exe"

echo "==> 1. Building versionradar-server.exe (windows/${TARGET_ARCH})..."
mkdir -p "${WIN_STAGE_DIR}"
(
  cd "${REPO_DIR}/backend"
  GOOS=windows GOARCH="${TARGET_ARCH}" go build -ldflags="-s -w" -o "${SIDECAR_EXE}" ./cmd/server
)

echo "✓ Built Windows sidecar: ${SIDECAR_EXE} ($(du -h "${SIDECAR_EXE}" | awk '{print $1}'))"

IS_WINDOWS=false
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*|Windows_NT)
    IS_WINDOWS=true
    ;;
esac

if [[ "${IS_WINDOWS}" != "true" ]]; then
  echo ""
  echo "Skipping 'flutter build windows --release' (requires a Windows host with Visual Studio)."
  echo "To build the full Windows desktop bundle on Windows:"
  echo "  1. Ensure Windows runner files exist (run once if client/windows is missing):"
  echo "       cd client && flutter create --platforms=windows ."
  echo "  2. Run this script in Git Bash / MSYS2 on Windows, or run:"
  echo "       cd client && flutter build windows --release"
  echo "       mkdir -p build/windows/x64/runner/Release/data"
  echo "       cp ../build/windows/versionradar-server.exe build/windows/x64/runner/Release/data/versionradar-server.exe"
  exit 0
fi

if ! command -v flutter >/dev/null 2>&1; then
  echo "Error: 'flutter' was not found in PATH." >&2
  exit 1
fi

if [[ ! -d "${REPO_DIR}/client/windows" ]]; then
  echo "==> 2. Scaffolding missing client/windows runner..."
  (
    cd "${REPO_DIR}/client"
    flutter create --platforms=windows .
  )
fi

echo "==> 3. Building Flutter Windows app (release)..."
(
  cd "${REPO_DIR}/client"
  flutter build windows --release
)

RELEASE_DIR="${REPO_DIR}/client/build/windows/x64/runner/Release"
if [[ ! -d "${RELEASE_DIR}" ]]; then
  RELEASE_DIR="${REPO_DIR}/client/build/windows/runner/Release"
fi

mkdir -p "${RELEASE_DIR}/data"
cp -f "${SIDECAR_EXE}" "${RELEASE_DIR}/data/versionradar-server.exe"

echo ""
echo "✓ Built Windows app with bundled sidecar!"
echo "  Release dir: ${RELEASE_DIR}"
echo "  Sidecar:     ${RELEASE_DIR}/data/versionradar-server.exe"
