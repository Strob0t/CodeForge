#!/usr/bin/env bash
# CodeForge test runner — runs unit, integration, and lint/build tests.
# Usage:
#   ./scripts/test.sh              Run unit tests (Go + Python + Frontend)
#   ./scripts/test.sh go           Go unit tests only
#   ./scripts/test.sh python       Python unit tests only
#   ./scripts/test.sh frontend     Frontend lint + build
#   ./scripts/test.sh integration  Integration tests (requires docker compose services)
#   ./scripts/test.sh all          Everything including integration tests
set -euo pipefail

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

ROOTDIR="$(cd "$(dirname "$0")/.." && pwd)"
SUITE="${1:-unit}"

# Track results
declare -A RESULTS

run_go() {
  echo -e "${CYAN}=== Go Unit Tests ===${NC}"
  if go test -race -count=1 ./...; then
    RESULTS[go]="pass"
    echo -e "${GREEN}Go: PASS${NC}"
  else
    RESULTS[go]="fail"
    echo -e "${RED}Go: FAIL${NC}"
  fi
  echo ""
}

run_python() {
  echo -e "${CYAN}=== Python Unit Tests ===${NC}"
  if (cd "$ROOTDIR/workers" && poetry run pytest -v); then
    RESULTS[python]="pass"
    echo -e "${GREEN}Python: PASS${NC}"
  else
    RESULTS[python]="fail"
    echo -e "${RED}Python: FAIL${NC}"
  fi
  echo ""
}

run_frontend() {
  echo -e "${CYAN}=== Frontend Lint + Build ===${NC}"
  if npm run lint --prefix "$ROOTDIR/frontend" && npm run build --prefix "$ROOTDIR/frontend"; then
    RESULTS[frontend]="pass"
    echo -e "${GREEN}Frontend: PASS${NC}"
  else
    RESULTS[frontend]="fail"
    echo -e "${RED}Frontend: FAIL${NC}"
  fi
  echo ""
}

run_integration() {
  echo -e "${CYAN}=== Integration Tests ===${NC}"

  # Check if PostgreSQL is reachable (TCP check on port 5432)
  if ! (echo > /dev/tcp/localhost/5432) 2>/dev/null; then
    echo -e "${YELLOW}PostgreSQL not reachable. Start services first:${NC}"
    echo "  docker compose up -d postgres nats"
    RESULTS[integration]="skip"
    return
  fi

  if go test -race -count=1 -tags=integration "$ROOTDIR/tests/integration/..."; then
    RESULTS[integration]="pass"
    echo -e "${GREEN}Integration: PASS${NC}"
  else
    RESULTS[integration]="fail"
    echo -e "${RED}Integration: FAIL${NC}"
  fi
  echo ""
}

print_summary() {
  echo -e "${CYAN}=== Summary ===${NC}"
  local failed=0
  for suite in "${!RESULTS[@]}"; do
    case "${RESULTS[$suite]}" in
      pass) echo -e "  ${GREEN}$suite: PASS${NC}" ;;
      fail) echo -e "  ${RED}$suite: FAIL${NC}"; failed=1 ;;
      skip) echo -e "  ${YELLOW}$suite: SKIP${NC}" ;;
    esac
  done
  echo ""
  return $failed
}

case "$SUITE" in
  go)
    run_go
    ;;
  python)
    run_python
    ;;
  frontend)
    run_frontend
    ;;
  integration)
    run_integration
    ;;
  unit)
    run_go
    run_python
    run_frontend
    ;;
  all)
    run_go
    run_python
    run_frontend
    run_integration
    ;;
  *)
    echo "CodeForge Test Runner"
    echo ""
    echo "Usage:"
    echo "  $0              Run unit tests (Go + Python + Frontend)"
    echo "  $0 go           Go unit tests only"
    echo "  $0 python       Python unit tests only"
    echo "  $0 frontend     Frontend lint + build"
    echo "  $0 integration  Integration tests (requires docker compose)"
    echo "  $0 all          Everything including integration"
    exit 1
    ;;
esac

print_summary
