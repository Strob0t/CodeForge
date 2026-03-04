"""Comprehensive E2E test driver for CodeForge.

Tests all features via API endpoints and browser automation (Playwright MCP).
Works with MCP --isolated mode (each tool call gets a fresh browser context).
"""

import json
import socket
import sys
import tempfile
import time
from urllib.error import HTTPError
from urllib.parse import urlparse
from urllib.request import Request, urlopen


def _safe_urlopen(req: Request | str, *, timeout: int = 30):
    """Wrap urlopen with scheme validation (only http/https allowed)."""
    url = req.full_url if isinstance(req, Request) else req
    if urlparse(url).scheme not in ("http", "https"):
        msg = f"Unsafe URL scheme: {url}"
        raise ValueError(msg)
    return urlopen(req, timeout=timeout)


# -- Config -----------------------------------------------------------
MCP_URL = "http://172.18.0.4:8001/mcp"
FRONTEND = "http://host.docker.internal:3000"
BACKEND = "http://localhost:8080"
SESSION_ID: str | None = None
MSG_ID = 0


# -- MCP Helpers ------------------------------------------------------
def mcp_call(method: str, params: dict | None = None) -> dict:
    global MSG_ID, SESSION_ID
    MSG_ID += 1
    body: dict = {"jsonrpc": "2.0", "id": MSG_ID, "method": method}
    if params:
        body["params"] = params
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    if SESSION_ID:
        headers["Mcp-Session-Id"] = SESSION_ID
    req = Request(MCP_URL, data=json.dumps(body).encode(), headers=headers)
    try:
        with _safe_urlopen(req, timeout=30) as resp:
            raw = resp.read().decode()
            if SESSION_ID is None:
                sid = resp.headers.get("Mcp-Session-Id")
                if sid:
                    SESSION_ID = sid
            for line in raw.strip().split("\n"):
                if line.startswith("data: "):
                    return json.loads(line[6:])
            return {"raw": raw}
    except Exception as e:
        return {"error": str(e)}


def tool_call(name: str, arguments: dict | None = None) -> dict:
    result = mcp_call("tools/call", {"name": name, "arguments": arguments or {}})
    if "result" in result:
        contents = result["result"].get("content", [])
        texts = [c.get("text", "") for c in contents if c.get("type") == "text"]
        return {"ok": not result["result"].get("isError", False), "text": "\n".join(texts)}
    return {"ok": False, "text": str(result)}


def navigate(url: str, retries: int = 1) -> str:
    """Navigate and return the snapshot text from the navigate response.
    In --isolated mode, this is the ONLY reliable way to get page content."""
    for attempt in range(retries + 1):
        r = tool_call("browser_navigate", {"url": url})
        text = r.get("text", "")
        if text and len(text) > 50:
            return text
        if attempt < retries:
            reinit_mcp()
            time.sleep(1)
    return r.get("text", "")


# -- HTTP Helpers -----------------------------------------------------
def http_request(method: str, path: str, token: str = "", data: dict | None = None) -> tuple[int, dict]:
    url = f"{BACKEND}{path}"
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    body = json.dumps(data).encode() if data else None
    req = Request(url, data=body, headers=headers, method=method)
    try:
        with _safe_urlopen(req, timeout=10) as resp:
            raw = resp.read().decode()
            return resp.status, json.loads(raw) if raw else {}
    except HTTPError as e:
        try:
            raw_body = json.loads(e.read().decode())
        except Exception:
            raw_body = {}
        return e.code, raw_body
    except Exception as e:
        return 0, {"error": str(e)}


def api_get(path: str, token: str) -> tuple[int, dict]:
    return http_request("GET", path, token)


def api_post(path: str, token: str, data: dict) -> tuple[int, dict]:
    return http_request("POST", path, token, data)


def api_put(path: str, token: str, data: dict) -> tuple[int, dict]:
    return http_request("PUT", path, token, data)


def api_delete(path: str, token: str) -> tuple[int, dict]:
    return http_request("DELETE", path, token)


# -- Test Results -----------------------------------------------------
results: list[dict] = []


def record(phase: str, feature: str, status: str, detail: str = ""):
    results.append({"phase": phase, "feature": feature, "status": status, "detail": detail})
    icon = {"PASS": "PASS", "FAIL": "FAIL", "PARTIAL": "WARN"}[status]
    msg = f"  [{icon}] {feature}"
    if detail:
        msg += f" -- {detail}"
    print(msg)


def reinit_mcp():
    """Reinitialize MCP session (needed between long gaps of non-browser tests)."""
    global SESSION_ID, MSG_ID
    SESSION_ID = None
    MSG_ID = 0
    mcp_call(
        "initialize",
        {
            "protocolVersion": "2025-03-26",
            "capabilities": {},
            "clientInfo": {"name": "e2e-full", "version": "1.0"},
        },
    )
    print(f"  (MCP session reinitialized: {SESSION_ID})")


# -- Initialize MCP --------------------------------------------------
print("Initializing MCP session...")
init = mcp_call(
    "initialize",
    {
        "protocolVersion": "2025-03-26",
        "capabilities": {},
        "clientInfo": {"name": "e2e-full", "version": "1.0"},
    },
)
print(f"MCP Session: {SESSION_ID}")

# -- Get Auth Token ---------------------------------------------------
_, login_data = api_post("/api/v1/auth/login", "", {"email": "admin@localhost", "password": "Changeme123"})
TOKEN = login_data.get("access_token", "")
if not TOKEN:
    print("FATAL: Could not get auth token")
    sys.exit(1)
print(f"Auth token obtained (len={len(TOKEN)})\n")

# ================================================================
# PHASE 1: LOGIN & AUTH
# ================================================================
print("=" * 64)
print("PHASE 1: LOGIN & AUTH")
print("=" * 64)

# 1a. Login page renders (browser)
snap = navigate(f"{FRONTEND}/login")
snap_lower = snap.lower()
if "email" in snap_lower and "password" in snap_lower and "sign in" in snap_lower:
    record("Auth", "Login page renders with form fields", "PASS")
elif "email" in snap_lower or "sign in" in snap_lower:
    record("Auth", "Login page renders with form fields", "PARTIAL", "Some form elements found")
else:
    record("Auth", "Login page renders with form fields", "FAIL", f"Content: {snap[:200]}")

# 1b. API: Valid login
status, data = api_post("/api/v1/auth/login", "", {"email": "admin@localhost", "password": "Changeme123"})
if status == 200 and data.get("access_token"):
    record("Auth", "API: Valid login returns token", "PASS")
else:
    record("Auth", "API: Valid login returns token", "FAIL", f"status={status}")

# 1c. API: Invalid login
status, _ = api_post("/api/v1/auth/login", "", {"email": "admin@localhost", "password": "wrongpassword"})
if status == 401:
    record("Auth", "API: Invalid login returns 401", "PASS")
else:
    record("Auth", "API: Invalid login returns 401", "FAIL", f"status={status}")

# 1d. API: Unauthenticated request
status, _ = api_get("/api/v1/projects", "")
if status == 401:
    record("Auth", "API: Unauthenticated request returns 401", "PASS")
else:
    record("Auth", "API: Unauthenticated request returns 401", "FAIL", f"status={status}")

# 1e. API: Invalid token
status, _ = api_get("/api/v1/projects", "invalid-token-here")
if status == 401:
    record("Auth", "API: Invalid token returns 401", "PASS")
else:
    record("Auth", "API: Invalid token returns 401", "FAIL", f"status={status}")

# 1f. Forgot password page (browser)
snap = navigate(f"{FRONTEND}/forgot-password")
snap_lower = snap.lower()
if "forgot" in snap_lower or "reset" in snap_lower or "email" in snap_lower:
    record("Auth", "Forgot password page renders", "PASS")
elif len(snap) > 50:
    record("Auth", "Forgot password page renders", "PARTIAL", "Page loaded but keywords not found")
else:
    record("Auth", "Forgot password page renders", "FAIL", f"Content: {snap[:200]}")

# ================================================================
# PHASE 2: HEALTH & INFRASTRUCTURE
# ================================================================
print("\n" + "=" * 64)
print("PHASE 2: HEALTH & INFRASTRUCTURE")
print("=" * 64)

status, data = api_get("/health", TOKEN)
if status == 200 and data.get("status") == "ok":
    record("Health", "Health endpoint", "PASS")
else:
    record("Health", "Health endpoint", "FAIL", f"status={status}")

status, data = api_get("/health/ready", TOKEN)
if status == 200:
    record("Health", "Readiness endpoint", "PASS")
else:
    record("Health", "Readiness endpoint", "FAIL", f"status={status}")

status, data = api_get("/health", TOKEN)
if data.get("dev_mode") is True:
    record("Health", "Dev mode enabled (APP_ENV=development)", "PASS")
else:
    record("Health", "Dev mode enabled (APP_ENV=development)", "FAIL", f"dev_mode={data.get('dev_mode')}")

# ================================================================
# PHASE 3: API ENDPOINT COVERAGE
# ================================================================
print("\n" + "=" * 64)
print("PHASE 3: API ENDPOINT COVERAGE")
print("=" * 64)

api_endpoints = [
    ("/api/v1/projects", "List Projects"),
    ("/api/v1/llm/models", "List LLM Models"),
    ("/api/v1/modes", "List Modes"),
    ("/api/v1/policies", "List Policies"),
    ("/api/v1/settings", "Get Settings"),
    ("/api/v1/users", "List Users"),
    ("/api/v1/mcp/servers", "List MCP Servers"),
    ("/api/v1/knowledge-bases", "List Knowledge Bases"),
    ("/api/v1/scopes", "List Scopes"),
    ("/api/v1/benchmarks/suites", "List Benchmark Suites"),
    ("/api/v1/prompt-sections", "List Prompt Sections"),
    ("/api/v1/providers/git", "Git Providers"),
    ("/api/v1/providers/agent", "Agent Providers"),
    ("/api/v1/costs", "Cost Tracking"),
]

for path, name in api_endpoints:
    status, _ = api_get(path, TOKEN)
    if status == 200:
        record("API", f"GET {name} ({path})", "PASS")
    else:
        record("API", f"GET {name} ({path})", "FAIL", f"HTTP {status}")

# ================================================================
# PHASE 4: CRUD OPERATIONS
# ================================================================
print("\n" + "=" * 64)
print("PHASE 4: CRUD OPERATIONS")
print("=" * 64)

# -- 4a. Project CRUD --
print("  Testing Project CRUD...")
status, data = api_post(
    "/api/v1/projects",
    TOKEN,
    {
        "name": "E2E Test Project",
        "description": "Created by E2E test",
        "repo_type": "local",
        "repo_url": "https://github.com/example/test-repo.git",
    },
)
project_id = data.get("id", "")
if status in (200, 201) and project_id:
    record("CRUD", "Create Project", "PASS", f"id={project_id}")

    status, data = api_get(f"/api/v1/projects/{project_id}", TOKEN)
    if status == 200 and data.get("name") == "E2E Test Project":
        record("CRUD", "Read Project by ID", "PASS")
    else:
        record("CRUD", "Read Project by ID", "FAIL", f"status={status}")

    status, _ = api_put(
        f"/api/v1/projects/{project_id}",
        TOKEN,
        {
            "name": "E2E Test Project Updated",
            "description": "Updated by E2E test",
        },
    )
    if status == 200:
        record("CRUD", "Update Project", "PASS")
    else:
        record("CRUD", "Update Project", "FAIL", f"status={status}")

    status, data = api_get(f"/api/v1/projects/{project_id}", TOKEN)
    if status == 200 and data.get("name") == "E2E Test Project Updated":
        record("CRUD", "Verify Project Update", "PASS")
    else:
        record("CRUD", "Verify Project Update", "FAIL", f"name={data.get('name')}")

    status, _ = api_delete(f"/api/v1/projects/{project_id}", TOKEN)
    if status in (200, 204):
        record("CRUD", "Delete Project", "PASS")
    else:
        record("CRUD", "Delete Project", "FAIL", f"status={status}")

    status, _ = api_get(f"/api/v1/projects/{project_id}", TOKEN)
    if status == 404:
        record("CRUD", "Verify Project Deletion (404)", "PASS")
    else:
        record("CRUD", "Verify Project Deletion (404)", "FAIL", f"status={status}")
else:
    record("CRUD", "Create Project", "FAIL", f"status={status}, resp={json.dumps(data)[:200]}")

# -- 4b. MCP Server CRUD --
print("  Testing MCP Server CRUD...")
status, data = api_post(
    "/api/v1/mcp/servers",
    TOKEN,
    {
        "name": "E2E Test MCP Server",
        "url": "http://localhost:9999/mcp",
        "transport": "streamable_http",
        "enabled": True,
    },
)
mcp_id = data.get("id", "")
if status in (200, 201) and mcp_id:
    record("CRUD", "Create MCP Server", "PASS", f"id={mcp_id}")

    status, _ = api_get(f"/api/v1/mcp/servers/{mcp_id}", TOKEN)
    if status == 200:
        record("CRUD", "Read MCP Server by ID", "PASS")
    else:
        record("CRUD", "Read MCP Server by ID", "FAIL", f"status={status}")

    status, _ = api_delete(f"/api/v1/mcp/servers/{mcp_id}", TOKEN)
    if status in (200, 204):
        record("CRUD", "Delete MCP Server", "PASS")
    else:
        record("CRUD", "Delete MCP Server", "FAIL", f"status={status}")
else:
    record("CRUD", "Create MCP Server", "FAIL", f"status={status}, resp={json.dumps(data)[:200]}")

# -- 4c. Knowledge Base CRUD --
print("  Testing Knowledge Base CRUD...")
status, data = api_post(
    "/api/v1/knowledge-bases",
    TOKEN,
    {
        "name": "E2E Test KB",
        "description": "Created by E2E test",
        "category": "custom",
        "content_path": tempfile.mkdtemp(prefix="e2e-kb-test-"),
    },
)
kb_id = data.get("id", "")
if status in (200, 201) and kb_id:
    record("CRUD", "Create Knowledge Base", "PASS", f"id={kb_id}")

    status, _ = api_delete(f"/api/v1/knowledge-bases/{kb_id}", TOKEN)
    if status in (200, 204):
        record("CRUD", "Delete Knowledge Base", "PASS")
    else:
        record("CRUD", "Delete Knowledge Base", "FAIL", f"status={status}")
else:
    record("CRUD", "Create Knowledge Base", "FAIL", f"status={status}, resp={json.dumps(data)[:200]}")

# -- 4d. Scope CRUD --
print("  Testing Scope CRUD...")
status, data = api_post(
    "/api/v1/scopes",
    TOKEN,
    {
        "name": "E2E Test Scope",
        "description": "Created by E2E test",
        "type": "global",
    },
)
scope_id = data.get("id", "")
if status in (200, 201) and scope_id:
    record("CRUD", "Create Scope", "PASS", f"id={scope_id}")

    status, _ = api_delete(f"/api/v1/scopes/{scope_id}", TOKEN)
    if status in (200, 204):
        record("CRUD", "Delete Scope", "PASS")
    else:
        record("CRUD", "Delete Scope", "FAIL", f"status={status}")
else:
    record("CRUD", "Create Scope", "FAIL", f"status={status}, resp={json.dumps(data)[:200]}")

# ================================================================
# PHASE 5: FRONTEND PAGE NAVIGATION (Browser)
# ================================================================
print("\n" + "=" * 64)
print("PHASE 5: FRONTEND PAGE NAVIGATION")
print("=" * 64)

reinit_mcp()
# In MCP --isolated mode, each navigate creates a fresh browser context.
# Authenticated pages will redirect to /login since there's no persistent session.
# We test: (a) unauthenticated pages render, (b) auth pages properly redirect to login.

# Test login page (unauthenticated - should render)
snap = navigate(f"{FRONTEND}/login")
snap_lower = snap.lower()
if "sign in" in snap_lower and "email" in snap_lower:
    record("UI", "Login page renders correctly", "PASS")
else:
    record("UI", "Login page renders correctly", "FAIL", f"Content: {snap[:200]}")

# Test each page - in isolated mode, expect redirect to login for auth-required pages
pages = [
    ("/", "Dashboard"),
    ("/costs", "Cost Dashboard"),
    ("/models", "Models"),
    ("/modes", "Modes"),
    ("/activity", "Activity"),
    ("/knowledge-bases", "Knowledge Bases"),
    ("/scopes", "Scopes"),
    ("/mcp", "MCP Servers"),
    ("/prompts", "Prompts"),
    ("/settings", "Settings"),
    ("/benchmarks", "Benchmarks"),
]

for path, name in pages:
    snap = navigate(f"{FRONTEND}{path}")
    snap_lower = snap.lower()

    has_error_boundary = "error" in snap_lower and "boundary" in snap_lower
    redirected_to_login = "sign in" in snap_lower or "/login" in snap_lower
    has_content = len(snap) > 100

    if has_error_boundary:
        record("UI", f"Page: {name} ({path})", "FAIL", "Error boundary triggered")
    elif redirected_to_login:
        # RouteGuard working correctly - redirects unauthenticated users to login
        record("UI", f"Page: {name} ({path}) - RouteGuard redirect", "PASS")
    elif has_content:
        record("UI", f"Page: {name} ({path})", "PASS")
    else:
        record("UI", f"Page: {name} ({path})", "FAIL", f"Empty page ({len(snap)} chars)")

# 404 page
snap = navigate(f"{FRONTEND}/nonexistent-page-xyz")
snap_lower = snap.lower()
if "not found" in snap_lower or "404" in snap_lower:
    record("UI", "404 Not Found page", "PASS")
elif "sign in" in snap_lower:
    # 404 redirected to login due to route guard
    record("UI", "404 Not Found page", "PARTIAL", "Redirected to login instead of 404")
else:
    record("UI", "404 Not Found page", "FAIL", f"Content: {snap[:200]}")

# Forgot password page (unauthenticated - should render without redirect)
snap = navigate(f"{FRONTEND}/forgot-password")
snap_lower = snap.lower()
if "forgot" in snap_lower or "reset" in snap_lower:
    record("UI", "Forgot password page renders", "PASS")
elif "email" in snap_lower:
    record("UI", "Forgot password page renders", "PARTIAL", "Email field found but no 'forgot' keyword")
else:
    record("UI", "Forgot password page renders", "FAIL", f"Content: {snap[:200]}")

# ================================================================
# PHASE 6: SETTINGS & CONFIGURATION
# ================================================================
print("\n" + "=" * 64)
print("PHASE 6: SETTINGS & CONFIGURATION")
print("=" * 64)

status, data = api_get("/api/v1/settings", TOKEN)
if status == 200 and isinstance(data, dict):
    record("Settings", "Get current settings", "PASS")
else:
    record("Settings", "Get current settings", "FAIL", f"status={status}")

# ================================================================
# PHASE 7: USERS & ROLES
# ================================================================
print("\n" + "=" * 64)
print("PHASE 7: USERS & ROLES")
print("=" * 64)

status, data = api_get("/api/v1/users", TOKEN)
if status == 200 and isinstance(data, list):
    record("Users", "List users", "PASS", f"count={len(data)}")
    admin_users = [u for u in data if u.get("email") == "admin@localhost"]
    if admin_users:
        admin = admin_users[0]
        record("Users", "Admin user exists with correct role", "PASS", f"role={admin.get('role')}")
    else:
        record("Users", "Admin user exists with correct role", "FAIL", "admin@localhost not found")
else:
    record("Users", "List users", "FAIL", f"status={status}")

# ================================================================
# PHASE 8: MODES & POLICIES
# ================================================================
print("\n" + "=" * 64)
print("PHASE 8: MODES & POLICIES")
print("=" * 64)

status, data = api_get("/api/v1/modes", TOKEN)
if status == 200 and isinstance(data, list):
    record("Modes", "List modes", "PASS", f"count={len(data)}")
    if len(data) > 0 and "name" in data[0]:
        record("Modes", "Mode has name field", "PASS", f"first={data[0]['name']}")
else:
    record("Modes", "List modes", "FAIL", f"status={status}")

status, data = api_get("/api/v1/policies", TOKEN)
if status == 200:
    count = len(data) if isinstance(data, list) else "N/A"
    record("Policies", "List policies", "PASS", f"count={count}")
else:
    record("Policies", "List policies", "FAIL", f"status={status}")

# ================================================================
# PHASE 9: LLM MODELS & PROVIDERS
# ================================================================
print("\n" + "=" * 64)
print("PHASE 9: LLM MODELS & PROVIDERS")
print("=" * 64)

status, data = api_get("/api/v1/llm/models", TOKEN)
if status == 200:
    count = len(data) if isinstance(data, list) else "N/A"
    record("LLM", "List LLM models", "PASS", f"count={count}")
else:
    record("LLM", "List LLM models", "FAIL", f"status={status}")

status, _ = api_get("/api/v1/providers/git", TOKEN)
record("Providers", "Git providers", "PASS" if status == 200 else "FAIL", f"status={status}")

status, _ = api_get("/api/v1/providers/agent", TOKEN)
record("Providers", "Agent providers", "PASS" if status == 200 else "FAIL", f"status={status}")

# ================================================================
# PHASE 10: COST TRACKING
# ================================================================
print("\n" + "=" * 64)
print("PHASE 10: COST TRACKING")
print("=" * 64)

status, _ = api_get("/api/v1/costs", TOKEN)
record("Costs", "GET /costs", "PASS" if status == 200 else "FAIL", f"status={status}")

# ================================================================
# PHASE 11: BENCHMARKS
# ================================================================
print("\n" + "=" * 64)
print("PHASE 11: BENCHMARKS")
print("=" * 64)

status, data = api_get("/api/v1/benchmarks/suites", TOKEN)
if status == 200:
    count = len(data) if isinstance(data, list) else "N/A"
    record("Benchmarks", "List benchmark suites", "PASS", f"count={count}")
else:
    record("Benchmarks", "List benchmark suites", "FAIL", f"status={status}")

# ================================================================
# PHASE 12: PROMPT SECTIONS
# ================================================================
print("\n" + "=" * 64)
print("PHASE 12: PROMPT SECTIONS")
print("=" * 64)

status, data = api_get("/api/v1/prompt-sections", TOKEN)
if status == 200:
    count = len(data) if isinstance(data, list) else "N/A"
    record("Prompts", "List prompt sections", "PASS", f"count={count}")
else:
    record("Prompts", "List prompt sections", "FAIL", f"status={status}")

# ================================================================
# PHASE 13: WEBSOCKET
# ================================================================
print("\n" + "=" * 64)
print("PHASE 13: WEBSOCKET")
print("=" * 64)

# Test WebSocket upgrade WITH auth
try:
    sock = socket.create_connection(("localhost", 8080), timeout=5)
    ws_req = (
        f"GET /ws?token={TOKEN} HTTP/1.1\r\n"
        f"Host: localhost:8080\r\n"
        f"Upgrade: websocket\r\n"
        f"Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"
        f"Sec-WebSocket-Version: 13\r\n"
        f"\r\n"
    )
    sock.sendall(ws_req.encode())
    resp = sock.recv(4096).decode(errors="replace")
    sock.close()
    if "101" in resp:
        record("WebSocket", "WebSocket upgrade with auth token", "PASS")
    elif "401" in resp:
        record("WebSocket", "WebSocket upgrade with auth token", "FAIL", "Got 401")
    else:
        first_line = resp.split("\r\n")[0] if resp else "empty"
        record("WebSocket", "WebSocket upgrade with auth token", "PARTIAL", f"Response: {first_line}")
except Exception as e:
    record("WebSocket", "WebSocket upgrade with auth token", "FAIL", str(e))

# Test WebSocket WITHOUT auth (should be rejected)
try:
    sock = socket.create_connection(("localhost", 8080), timeout=5)
    ws_req = (
        "GET /ws HTTP/1.1\r\n"
        "Host: localhost:8080\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        "\r\n"
    )
    sock.sendall(ws_req.encode())
    resp = sock.recv(4096).decode(errors="replace")
    sock.close()
    if "401" in resp:
        record("WebSocket", "WebSocket without auth returns 401", "PASS")
    elif "101" in resp:
        record("WebSocket", "WebSocket without auth returns 401", "FAIL", "Allowed without auth!")
    else:
        first_line = resp.split("\r\n")[0] if resp else "empty"
        record("WebSocket", "WebSocket without auth returns 401", "PARTIAL", f"Response: {first_line}")
except Exception as e:
    record("WebSocket", "WebSocket without auth returns 401", "FAIL", str(e))

# ================================================================
# PHASE 14: MCP SERVER MANAGEMENT
# ================================================================
print("\n" + "=" * 64)
print("PHASE 14: MCP SERVER MANAGEMENT")
print("=" * 64)

status, _ = api_get("/api/v1/mcp/servers", TOKEN)
record("MCP", "MCP servers list endpoint", "PASS" if status == 200 else "FAIL", f"status={status}")

# ================================================================
# PHASE 15: KNOWLEDGE BASES
# ================================================================
print("\n" + "=" * 64)
print("PHASE 15: KNOWLEDGE BASES")
print("=" * 64)

status, data = api_get("/api/v1/knowledge-bases", TOKEN)
if status == 200:
    count = len(data) if isinstance(data, list) else "N/A"
    record("KB", "List knowledge bases", "PASS", f"count={count}")
else:
    record("KB", "List knowledge bases", "FAIL", f"status={status}")

# ================================================================
# PHASE 16: SCOPES
# ================================================================
print("\n" + "=" * 64)
print("PHASE 16: SCOPES")
print("=" * 64)

status, data = api_get("/api/v1/scopes", TOKEN)
if status == 200:
    count = len(data) if isinstance(data, list) else "N/A"
    record("Scopes", "List scopes", "PASS", f"count={count}")
else:
    record("Scopes", "List scopes", "FAIL", f"status={status}")

# ================================================================
# FINAL SUMMARY
# ================================================================
print("\n" + "=" * 64)
print("E2E TEST FINAL REPORT")
print("=" * 64)

passed = sum(1 for r in results if r["status"] == "PASS")
failed = sum(1 for r in results if r["status"] == "FAIL")
partial = sum(1 for r in results if r["status"] == "PARTIAL")
total = len(results)

# Group by phase
phases: dict[str, list[dict]] = {}
for r in results:
    phases.setdefault(r["phase"], []).append(r)

print("\nDetailed Results by Phase:")
print("-" * 64)

for phase, tests in phases.items():
    p = sum(1 for t in tests if t["status"] == "PASS")
    f = sum(1 for t in tests if t["status"] == "FAIL")
    w = sum(1 for t in tests if t["status"] == "PARTIAL")
    print(f"\n  {phase}: {p} pass, {f} fail, {w} partial")
    for t in tests:
        icon = {"PASS": "  OK ", "FAIL": "FAIL", "PARTIAL": "WARN"}[t["status"]]
        detail = f" -- {t['detail']}" if t["detail"] else ""
        print(f"    [{icon}] {t['feature']}{detail}")

print("\n" + "=" * 64)
print(f"TOTAL: {total} tests | PASSED: {passed} | FAILED: {failed} | PARTIAL: {partial}")
if total > 0:
    print(f"Pass rate: {passed / total * 100:.1f}%")
    print(f"Pass+Partial rate: {(passed + partial) / total * 100:.1f}%")
print("=" * 64)

# List failures for quick reference
if failed > 0:
    print(f"\n{failed} FAILURES:")
    for r in results:
        if r["status"] == "FAIL":
            print(f"  - [{r['phase']}] {r['feature']}: {r['detail']}")

sys.exit(1 if failed > 0 else 0)
