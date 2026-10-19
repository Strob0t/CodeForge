#!/usr/bin/env bash
# Product files stay under the size ceiling (AGENTS.md, section 5). Data and
# contract files (i18n catalogs, the frontend API type file) are exempt.
# Usage: scripts/check-file-sizes.sh [files...]   (no files: the whole tree)
set -euo pipefail
cd "$(dirname "$0")/.."
limit=700

is_exempt() {
  case "$1" in
    frontend/src/i18n/*|frontend/src/api/types.ts) return 0 ;;
    *_test.go|*.test.ts|*.test.tsx|workers/tests/*|*/node_modules/*) return 0 ;;
  esac
  return 1
}

if [ "$#" -gt 0 ]; then
  files=("$@")
else
  mapfile -t files < <(git ls-files 'internal/*.go' 'cmd/*.go' 'workers/codeforge/*.py' 'frontend/src/*.ts' 'frontend/src/*.tsx')
fi

status=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  case "$f" in *.go|*.py|*.ts|*.tsx) ;; *) continue ;; esac
  is_exempt "$f" && continue
  lines=$(wc -l < "$f")
  if [ "$lines" -gt "$limit" ]; then
    echo "$f: $lines lines (limit $limit): split by responsibility, no helpers/utils dumping ground" >&2
    status=1
  fi
done
exit $status
