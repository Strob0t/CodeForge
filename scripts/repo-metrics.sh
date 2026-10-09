#!/usr/bin/env bash
# Repository process metrics (docs/testing/repo-metrics.md): commit sizes,
# fix and feature hygiene, files over the size ceiling, lint warnings.
# Usage: scripts/repo-metrics.sh [base-ref]   (default: merge-base with origin/staging)
set -euo pipefail
cd "$(dirname "$0")/.."

base="${1:-$(git merge-base origin/staging HEAD 2>/dev/null || git merge-base staging HEAD)}"
range="$base..HEAD"

echo "## Commits ($range, non-merge)"
git log --no-merges --format='%h' --shortstat "$range" | awk '
  /files? changed/ {
    f=$1; ins=0; del=0
    for (i=1; i<=NF; i++) { if ($i ~ /insertion/) ins=$(i-1); if ($i ~ /deletion/) del=$(i-1) }
    n++; tf+=f; ti+=ins; td+=del
    if (ins+del > 1000) huge++
    if (ins+del > 400) big++
  }
  END {
    if (n == 0) { print "no commits"; exit }
    printf "commits %d | avg files %.1f | avg +%.0f/-%.0f | over 400 lines %d | over 1000 lines %d\n", n, tf/n, ti/n, td/n, big, huge
  }'

count_with() { # $1 grep for subjects, $2 regex over file paths
  git log --no-merges --format='%h' --grep="$1" "$range" --name-only | awk -v re="$2" '
    $0 ~ /^[0-9a-f]+$/ && length($0) >= 7 { h=$0; seen[h]=0; next }
    NF && $0 ~ re { seen[h]=1 }
    END { n=0; t=0; for (k in seen) { t++; n+=seen[k] }; printf "%d/%d", n, t }'
}
echo "fix commits with a test change: $(count_with '^fix' '(_test\.go|/tests/|\.test\.tsx?|test_[a-z_]+\.py)')"
echo "feat commits with a docs change: $(count_with '^feat' '^docs/')"

echo
echo "## Files over 700 lines (product code; data and contract files exempt)"
over() { # $1 label, then paths
  local label="$1"; shift
  local n
  n=$(find "$@" -type f \( -name '*.go' -o -name '*.py' -o -name '*.ts' -o -name '*.tsx' \) \
      -not -name '*_test.go' -not -name '*.test.ts' -not -name '*.test.tsx' -not -path '*/node_modules/*' \
      -not -path 'frontend/src/i18n/*' -not -path 'frontend/src/api/types.ts' -not -path 'workers/tests/*' \
      -print0 | xargs -0 wc -l | awk '$1>700 && $2!="total"' | wc -l)
  echo "$label: $n"
}
over "Go" internal cmd
over "Python" workers/codeforge
over "TypeScript" frontend/src

echo
echo "## Lint warnings"
if [ -x frontend/node_modules/.bin/eslint ]; then
  w=$(cd frontend && node_modules/.bin/eslint . 2>/dev/null | grep -oE '[0-9]+ warnings?' | tail -1 || true)
  echo "frontend eslint: ${w:-0 warnings}"
else
  echo "frontend eslint: node_modules missing"
fi
