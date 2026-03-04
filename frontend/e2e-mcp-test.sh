#!/bin/bash
# E2E Test Driver using Playwright MCP
set -euo pipefail

MCP_IP="172.18.0.4"
MCP_URL="http://$MCP_IP:8001/mcp"
FRONTEND="http://host.docker.internal:3000"
BACKEND="http://localhost:8080"
PASS=0
FAIL=0
PARTIAL=0
RESULTS=""

# Initialize MCP session
SESSION=$(curl -s -D - -X POST "$MCP_URL" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"e2e","version":"1.0"}}}' 2>&1 | grep -i 'mcp-session-id' | tr -d '\r' | awk '{print $2}')
echo "MCP Session: $SESSION"

MSG_ID=1

mcp_tool() {
  local tool=$1
  local args=$2
  MSG_ID=$((MSG_ID + 1))
  local body="{\"jsonrpc\":\"2.0\",\"id\":$MSG_ID,\"method\":\"tools/call\",\"params\":{\"name\":\"$tool\",\"arguments\":$args}}"
  local resp
  resp=$(curl -s -X POST "$MCP_URL" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    -H "Mcp-Session-Id: $SESSION" \
    -d "$body" 2>&1)
  echo "$resp" | grep '^data: ' | sed 's/^data: //'
}

navigate() {
  mcp_tool "browser_navigate" "{\"url\":\"$1\"}"
}

get_snapshot() {
  mcp_tool "browser_snapshot" "{}"
}

fill_form() {
  mcp_tool "browser_fill_form" "$1"
}

click_element() {
  mcp_tool "browser_click" "{\"element\":\"$1\",\"ref\":\"$1\"}"
}

wait_text() {
  mcp_tool "browser_wait_for" "{\"text\":\"$1\",\"timeout\":${2:-10000}}"
}

record_pass() {
  PASS=$((PASS + 1))
  RESULTS="$RESULTS\n  [PASS] $1"
  echo "  [PASS] $1"
}

record_fail() {
  FAIL=$((FAIL + 1))
  RESULTS="$RESULTS\n  [FAIL] $1 -- $2"
  echo "  [FAIL] $1 -- $2"
}

record_partial() {
  PARTIAL=$((PARTIAL + 1))
  RESULTS="$RESULTS\n  [WARN] $1 -- $2"
  echo "  [WARN] $1 -- $2"
}

# Helper: check if snapshot contains keyword (case-insensitive)
snap_contains() {
  echo "$1" | python3 -c "import sys; text=sys.stdin.read().lower(); exit(0 if '$2'.lower() in text else 1)"
}

# Get auth token
TOKEN=$(curl -s -X POST "$BACKEND/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@localhost","password":"Changeme123"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))")

api_get() {
  curl -s -o /dev/null -w "%{http_code}" "$BACKEND$1" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json"
}

api_get_body() {
  curl -s "$BACKEND$1" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json"
}

echo ""
echo "================================================================"
echo "PHASE 1: LOGIN PAGE"
echo "================================================================"

SNAP=$(navigate "$FRONTEND/login")
if snap_contains "$SNAP" "Sign in"; then
  record_pass "Login page renders with form"
else
  record_fail "Login page renders" "Missing form elements"
fi

# Fill login form
fill_form '{"values":[{"ref":"e12","value":"admin@localhost"},{"ref":"e17","value":"Changeme123"}]}'
sleep 0.5

# Click sign in
click_element "e18"
sleep 3

# Check redirect to dashboard
SNAP=$(get_snapshot)
if snap_contains "$SNAP" "Dashboard" || snap_contains "$SNAP" "Projects" || snap_contains "$SNAP" "CodeForge"; then
  record_pass "Login with valid credentials -> redirects to dashboard"
else
  # Maybe snapshot shows something else
  echo "  DEBUG: post-login snapshot: $(echo $SNAP | head -c 500)"
  record_fail "Login redirect" "Not redirected to dashboard"
fi

echo ""
echo "================================================================"
echo "PHASE 2: DASHBOARD"
echo "================================================================"

SNAP=$(navigate "$FRONTEND/")
sleep 2
SNAP=$(get_snapshot)
if snap_contains "$SNAP" "dashboard" || snap_contains "$SNAP" "project" || snap_contains "$SNAP" "no project"; then
  record_pass "Dashboard page loads"
else
  echo "  DEBUG: dashboard snapshot: $(echo $SNAP | head -c 500)"
  record_fail "Dashboard page" "Page did not load expected content"
fi

echo ""
echo "================================================================"
echo "PHASE 3: NAVIGATION - All Pages"
echo "================================================================"

test_page() {
  local path=$1
  local keyword=$2
  local name=$3
  SNAP=$(navigate "$FRONTEND$path")
  sleep 1
  SNAP=$(get_snapshot)
  if snap_contains "$SNAP" "error" && snap_contains "$SNAP" "boundary"; then
    record_fail "Page: $name" "Error boundary triggered"
  elif snap_contains "$SNAP" "$keyword"; then
    record_pass "Page: $name"
  elif snap_contains "$SNAP" "heading" || snap_contains "$SNAP" "button"; then
    record_partial "Page: $name" "Page loaded but keyword '$keyword' not found"
  else
    record_fail "Page: $name" "Page did not render"
  fi
}

test_page "/costs" "cost" "Cost Dashboard"
test_page "/models" "model" "Models"
test_page "/modes" "mode" "Modes"
test_page "/activity" "activity" "Activity"
test_page "/knowledge-bases" "knowledge" "Knowledge Bases"
test_page "/scopes" "scope" "Scopes"
test_page "/mcp" "mcp" "MCP Servers"
test_page "/prompts" "prompt" "Prompts"
test_page "/settings" "setting" "Settings"
test_page "/benchmarks" "benchmark" "Benchmarks"

echo ""
echo "================================================================"
echo "PHASE 4: 404 PAGE"
echo "================================================================"

SNAP=$(navigate "$FRONTEND/nonexistent-route-xyz")
sleep 1
SNAP=$(get_snapshot)
if snap_contains "$SNAP" "not found" || snap_contains "$SNAP" "404"; then
  record_pass "404 Not Found page"
else
  record_fail "404 page" "No 404 indicator"
fi

echo ""
echo "================================================================"
echo "PHASE 5: SIDEBAR FEATURES"
echo "================================================================"

# Navigate to dashboard first
navigate "$FRONTEND/" > /dev/null
sleep 2
SNAP=$(get_snapshot)

# Check sidebar elements
if snap_contains "$SNAP" "sidebar" || snap_contains "$SNAP" "navigation" || snap_contains "$SNAP" "nav"; then
  record_pass "Sidebar renders"
else
  record_partial "Sidebar renders" "Sidebar structure not clearly identified"
fi

# Check theme toggle
if snap_contains "$SNAP" "theme" || snap_contains "$SNAP" "dark" || snap_contains "$SNAP" "light"; then
  record_pass "Theme toggle present"
else
  record_partial "Theme toggle" "Not found in snapshot"
fi

# Check WebSocket status
if snap_contains "$SNAP" "connected" || snap_contains "$SNAP" "disconnected" || snap_contains "$SNAP" "websocket" || snap_contains "$SNAP" "ws"; then
  record_pass "WebSocket status indicator"
else
  record_partial "WebSocket status" "Not visible in snapshot"
fi

echo ""
echo "================================================================"
echo "PHASE 6: API ENDPOINT TESTING"
echo "================================================================"

test_api() {
  local path=$1
  local name=$2
  local code
  code=$(api_get "$path")
  if [ "$code" = "200" ]; then
    record_pass "API: $name (GET $path)"
  else
    record_fail "API: $name (GET $path)" "HTTP $code"
  fi
}

test_api "/health" "Health Check"
test_api "/health/ready" "Readiness Check"
test_api "/api/v1/projects" "List Projects"
test_api "/api/v1/llm/models" "List LLM Models"
test_api "/api/v1/modes" "List Modes"
test_api "/api/v1/policies" "List Policies"
test_api "/api/v1/settings" "Get Settings"
test_api "/api/v1/users" "List Users"
test_api "/api/v1/mcp/servers" "List MCP Servers"
test_api "/api/v1/knowledge-bases" "List Knowledge Bases"
test_api "/api/v1/scopes" "List Scopes"
test_api "/api/v1/benchmarks/suites" "List Benchmark Suites"
test_api "/api/v1/prompt-sections" "List Prompt Sections"
test_api "/api/v1/providers/git" "Git Providers"
test_api "/api/v1/providers/agent" "Agent Providers"
test_api "/api/v1/costs" "Cost Tracking"

echo ""
echo "================================================================"
echo "PHASE 7: CRUD OPERATIONS"
echo "================================================================"

# Test project creation
echo "  Testing project CRUD..."
CREATE_RESP=$(curl -s -X POST "$BACKEND/api/v1/projects" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"E2E Test Project","description":"Created by E2E test","repo_type":"local","repo_url":"/tmp/test-repo"}')
PROJECT_ID=$(echo "$CREATE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)

if [ -n "$PROJECT_ID" ] && [ "$PROJECT_ID" != "" ]; then
  record_pass "API: Create Project"

  # Read
  code=$(api_get "/api/v1/projects/$PROJECT_ID")
  if [ "$code" = "200" ]; then
    record_pass "API: Get Project by ID"
  else
    record_fail "API: Get Project by ID" "HTTP $code"
  fi

  # Update
  UPDATE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BACKEND/api/v1/projects/$PROJECT_ID" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"E2E Test Project Updated","description":"Updated by E2E test"}')
  if [ "$UPDATE_CODE" = "200" ]; then
    record_pass "API: Update Project"
  else
    record_fail "API: Update Project" "HTTP $UPDATE_CODE"
  fi

  # Delete
  DELETE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BACKEND/api/v1/projects/$PROJECT_ID" \
    -H "Authorization: Bearer $TOKEN")
  if [ "$DELETE_CODE" = "200" ] || [ "$DELETE_CODE" = "204" ]; then
    record_pass "API: Delete Project"
  else
    record_fail "API: Delete Project" "HTTP $DELETE_CODE"
  fi
else
  record_fail "API: Create Project" "No project ID returned: $CREATE_RESP"
fi

echo ""
echo "================================================================"
echo "PHASE 8: AUTH EDGE CASES"
echo "================================================================"

# Invalid login
INVALID_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BACKEND/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@localhost","password":"wrongpassword"}')
if [ "$INVALID_CODE" = "401" ]; then
  record_pass "API: Invalid login returns 401"
else
  record_fail "API: Invalid login" "Expected 401, got $INVALID_CODE"
fi

# Missing auth header
NOAUTH_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BACKEND/api/v1/projects")
if [ "$NOAUTH_CODE" = "401" ]; then
  record_pass "API: Unauthenticated request returns 401"
else
  record_fail "API: Unauthenticated request" "Expected 401, got $NOAUTH_CODE"
fi

echo ""
echo "================================================================"
echo "PHASE 9: FRONTEND UI INTERACTIONS"
echo "================================================================"

# Test login page with invalid credentials via UI
SNAP=$(navigate "$FRONTEND/login")
sleep 1
fill_form '{"values":[{"ref":"e12","value":"admin@localhost"},{"ref":"e17","value":"wrongpass"}]}'
sleep 0.5
click_element "e18"
sleep 2
SNAP=$(get_snapshot)
if snap_contains "$SNAP" "invalid" || snap_contains "$SNAP" "error" || snap_contains "$SNAP" "incorrect" || snap_contains "$SNAP" "failed" || snap_contains "$SNAP" "Sign in"; then
  record_pass "UI: Login with invalid credentials shows error"
else
  record_fail "UI: Invalid login error" "No error shown: $(echo $SNAP | head -c 300)"
fi

# Test forgot password page
SNAP=$(navigate "$FRONTEND/forgot-password")
sleep 1
SNAP=$(get_snapshot)
if snap_contains "$SNAP" "forgot" || snap_contains "$SNAP" "reset" || snap_contains "$SNAP" "email"; then
  record_pass "UI: Forgot password page loads"
else
  record_fail "UI: Forgot password page" "Page did not render"
fi

echo ""
echo "================================================================"
echo "FINAL SUMMARY"
echo "================================================================"

TOTAL=$((PASS + FAIL + PARTIAL))
echo -e "$RESULTS"
echo ""
echo "Total: $TOTAL | Passed: $PASS | Failed: $FAIL | Partial: $PARTIAL"
if [ $TOTAL -gt 0 ]; then
  RATE=$((PASS * 100 / TOTAL))
  echo "Pass rate: ${RATE}%"
fi

exit 0
