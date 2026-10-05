#!/usr/bin/env bash
# sync-version.sh - Propagate VERSION file to all package manifests and the
# image tags docker-compose.prod.yml pins.
# Usage: ./scripts/sync-version.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
VERSION_FILE="$ROOT_DIR/VERSION"

if [[ ! -f "$VERSION_FILE" ]]; then
  echo "ERROR: VERSION file not found at $VERSION_FILE" >&2
  exit 1
fi

VERSION=$(tr -d '[:space:]' < "$VERSION_FILE")

if [[ -z "$VERSION" ]]; then
  echo "ERROR: VERSION file is empty" >&2
  exit 1
fi

echo "Syncing version: $VERSION"

# pyproject.toml
sed -i "s/^version = \".*\"/version = \"$VERSION\"/" "$ROOT_DIR/pyproject.toml"
echo "  updated pyproject.toml"

# frontend/package.json (only the top-level "version" field)
sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" "$ROOT_DIR/frontend/package.json"
echo "  updated frontend/package.json"

# frontend/package-lock.json (root-level entries only: lines 3 and 9)
sed -i '3s/"version": "[^"]*"/"version": "'"$VERSION"'"/' "$ROOT_DIR/frontend/package-lock.json"
sed -i '9s/"version": "[^"]*"/"version": "'"$VERSION"'"/' "$ROOT_DIR/frontend/package-lock.json"
echo "  updated frontend/package-lock.json"

# docker-compose.prod.yml: the default image tags (KI-116), e.g.
# ${CORE_IMAGE:-ghcr.io/strob0t/codeforge-core:0.8.0}
sed -i -E 's#(\$\{(CORE|WORKER|FRONTEND)_IMAGE:-ghcr\.io/strob0t/codeforge-(core|worker|frontend)):[^}]*\}#\1:'"$VERSION"'}#' \
  "$ROOT_DIR/docker-compose.prod.yml"
echo "  updated docker-compose.prod.yml"

echo "Done. All manifests now at v$VERSION."
