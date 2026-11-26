#!/usr/bin/env bash
set -euo pipefail

# publish.sh — Publish VersionRadar Community to GitHub and tag Go backend submodule
# Usage: ./scripts/publish.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_DIR}"

echo "==> 1. Verifying git state..."
CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
echo "Current branch: ${CURRENT_BRANCH}"

echo "==> 2. Running backend tests..."
cd "${REPO_DIR}/backend"
go test ./...

echo "==> 3. Running Flutter analyze & test..."
cd "${REPO_DIR}/client"
flutter analyze --no-fatal-infos
flutter test

cd "${REPO_DIR}"

echo "==> 4. Checking git status..."
if [[ -n "$(git status -s)" ]]; then
  echo "Working tree has uncommitted changes. Please commit or stash first."
  git status -s
  exit 1
fi

echo "==> 5. Ready to push to GitHub (origin)."
echo "Ensure https://github.com/danysz/versionradar is created and empty."
read -p "Push 'main' and tag 'backend/v0.1.0' now? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
  # Push main branch
  git push -u origin "${CURRENT_BRANCH}:main"
  
  # Create release tag for Go submodule if not already present
  if ! git rev-parse "backend/v0.1.0" >/dev/null 2>&1; then
    echo "Creating git tag backend/v0.1.0..."
    git tag -a backend/v0.1.0 -m "VersionRadar Community backend v0.1.0 release"
  fi
  
  echo "Pushing tag backend/v0.1.0 to origin..."
  git push origin backend/v0.1.0

  echo ""
  echo "✓ Successfully published VersionRadar Community to GitHub!"
  echo "  Repo: https://github.com/danysz/versionradar"
  echo "  Tag:  backend/v0.1.0"
  echo ""
  echo "Next step: Run 'scripts/switch-to-remote-backend.sh' in versionradar-pro."
fi
