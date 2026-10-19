#!/usr/bin/env bash
# AGENTS.md stays under its word limit (AGENTS.md, "Keeping this file small").
set -euo pipefail
limit=3000
file="$(dirname "$0")/../AGENTS.md"
words=$(wc -w < "$file")
if [ "$words" -gt "$limit" ]; then
  echo "AGENTS.md has $words words; the limit is $limit (move mechanics to docs/ and link them)" >&2
  exit 1
fi
echo "AGENTS.md: $words words (limit $limit)"
