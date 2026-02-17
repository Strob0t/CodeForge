#!/usr/bin/env bash
# CodeForge log helper — filters Docker Compose JSON logs.
# Usage:
#   ./scripts/logs.sh tail [N]           - Follow last N lines (default 50)
#   ./scripts/logs.sh errors             - Show only ERROR/error level entries
#   ./scripts/logs.sh service <name>     - Follow logs for a specific service
#   ./scripts/logs.sh request <id>       - Filter by request_id across all services
set -euo pipefail

CMD="${1:-tail}"
ARG="${2:-}"

case "$CMD" in
  tail)
    LINES="${ARG:-50}"
    docker compose logs --tail "$LINES" -f 2>/dev/null \
      || echo "Error: docker compose not available or no services running"
    ;;
  errors)
    docker compose logs --tail 500 2>/dev/null \
      | grep -iE '"level"\s*:\s*"(error|ERROR)"' \
      || echo "No errors found in last 500 lines"
    ;;
  service)
    if [ -z "$ARG" ]; then
      echo "Usage: $0 service <name>"
      echo "Available: postgres, nats, litellm, docs-mcp-server, playwright-mcp"
      exit 1
    fi
    docker compose logs --tail 100 -f "$ARG" 2>/dev/null \
      || echo "Error: service '$ARG' not found or not running"
    ;;
  request)
    if [ -z "$ARG" ]; then
      echo "Usage: $0 request <request-id>"
      exit 1
    fi
    docker compose logs --tail 1000 2>/dev/null \
      | grep "$ARG" \
      || echo "No logs found for request_id: $ARG"
    ;;
  *)
    echo "CodeForge Log Helper"
    echo ""
    echo "Usage:"
    echo "  $0 tail [N]           Follow last N lines (default 50)"
    echo "  $0 errors             Show ERROR entries"
    echo "  $0 service <name>     Follow logs for a service"
    echo "  $0 request <id>       Filter by request_id"
    exit 1
    ;;
esac
