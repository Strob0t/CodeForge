#!/bin/bash
# Starts the Playwright MCP server command given by .mcp.json and points it at
# the newest Chromium under $PLAYWRIGHT_BROWSERS_PATH (Claude Code cloud images:
# /opt/pw-browsers/chromium-<build>/chrome-linux/chrome). The image's build
# number changes with image updates and need not match the build the pinned
# @playwright/mcp expects, so it is looked up at every start instead of pinned.
# Without a local Chromium, Playwright's own default browser lookup applies.
#
# Usage: scripts/mcp-playwright.sh <command> [args...]
set -euo pipefail
shopt -s nullglob

browsers="${PLAYWRIGHT_BROWSERS_PATH:-/opt/pw-browsers}"
# chrome-linux64 is the layout of the newer Chrome for Testing based builds.
candidates=("$browsers"/chromium-*/chrome-linux/chrome "$browsers"/chromium-*/chrome-linux64/chrome)

chrome=""
if [ "${#candidates[@]}" -gt 0 ]; then
  chrome="$(printf '%s\n' "${candidates[@]}" | sort -V | tail -n 1)"
fi

if [ -n "$chrome" ] && [ -x "$chrome" ]; then
  echo "mcp-playwright: using $chrome" >&2
  exec "$@" --executable-path "$chrome"
fi

echo "mcp-playwright: no executable Chromium under $browsers, using the Playwright default" >&2
exec "$@"
