#!/usr/bin/env python3
"""Comprehensive E2E test driver using Playwright MCP container + API testing."""

import base64
import json
import os
import socket
import sys
from urllib.parse import urlparse
from urllib.request import Request, urlopen


def _safe_urlopen(req: Request | str, *, timeout: int = 30):
    """Wrap urlopen with scheme validation (only http/https allowed)."""
    url = req.full_url if isinstance(req, Request) else req
    if urlparse(url).scheme not in ("http", "https"):
        msg = f"Unsafe URL scheme: {url}"
        raise ValueError(msg)
    return urlopen(req, timeout=timeout)


MCP_IP = "172.18.0.4"
MCP_URL = f"http://{MCP_IP}:8001/mcp"
FRONTEND = "http://host.docker.internal:3000"
BACKEND = "http://localhost:8080"

SESSION: str = ""
MSG_ID = 0

# ── MCP helpers ──────────────────────────────────────────────


def mcp_call(method: str, params: dict | None = None) -> dict:
    global MSG_ID, SESSION
    MSG_ID += 1
    body: dict = {"jsonrpc": "2.0", "id": MSG_ID, "method": method}
    if params:
        body["params"] = params
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    if SESSION:
        headers["Mcp-Session-Id"] = SESSION
    req = Request(MCP_URL, data=json.dumps(body).encode(), headers=headers)
    try:
        with _safe_urlopen(req, timeout=60) as resp:
            if not SESSION:
                sid = resp.headers.get("mcp-session-id", "")
                if sid:
                    SESSION = sid
            raw = resp.read().decode()
            for line in raw.strip().split("\n"):
                if line.startswith("data: "):
                    return json.loads(line[6:])
    except Exception as e:
        return {"error": str(e)}
    return {}


def tool(name: str, args: dict) -> str:
    result = mcp_call("tools/call", {"name": name, "arguments": args})
    if "error" in result:
        return f"[MCP_ERROR] {result['error']}"
    contents = result.get("result", {}).get("content", [])
    is_error = result.get("result", {}).get("isError", False)
    text = "\n".join(c.get("text", "") for c in contents if c.get("type") == "text")
    if is_error:
        return f"[MCP_ERROR] {text}"
    return text


def navigate(url: str) -> str:
    """Navigate to URL. Returns the snapshot from the navigate result."""
    return tool("browser_navigate", {"url": url})


def run_code(code: str) -> str:
    """Run Playwright code snippet for multi-step operations."""
    return tool("browser_run_code", {"code": code})


# ── API helpers ──────────────────────────────────────────────

TOKEN = ""


def api_login() -> str:
    req = Request(
        f"{BACKEND}/api/v1/auth/login",
        data=json.dumps({"email": "admin@localhost", "password": "Changeme123"}).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with _safe_urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read())
        return data.get("access_token", "")


def api(method: str, path: str, body: dict | None = None) -> tuple[int, dict]:
    headers = {"Content-Type": "application/json"}
    if TOKEN:
        headers["Authorization"] = f"Bearer {TOKEN}"
    data = json.dumps(body).encode() if body else None
    req = Request(f"{BACKEND}{path}", data=data, headers=headers, method=method)
    try:
        with _safe_urlopen(req, timeout=10) as resp:
            try:
                return resp.status, json.loads(resp.read())
            except Exception:
                return resp.status, {}
    except Exception as e:
        code = getattr(e, "code", 0)
        try:
            return code, json.loads(e.read())
        except Exception:
            return code, {"error": str(e)}


# ── Test tracking ────────────────────────────────────────────

results: list[dict] = []


def ok(feature: str, detail: str = ""):
    results.append({"f": feature, "s": "PASS", "d": detail})
    print(f"  [PASS] {feature}" + (f" -- {detail}" if detail else ""))


def fail(feature: str, detail: str = ""):
    results.append({"f": feature, "s": "FAIL", "d": detail})
    print(f"  [FAIL] {feature} -- {detail}")


def warn(feature: str, detail: str = ""):
    results.append({"f": feature, "s": "PARTIAL", "d": detail})
    print(f"  [WARN] {feature} -- {detail}")


def contains(text: str, keyword: str) -> bool:
    return keyword.lower() in text.lower()


# ══════════════════════════════════════════════════════════════
# TEST EXECUTION
# ══════════════════════════════════════════════════════════════

print("=" * 64)
print("CodeForge E2E Test Suite")
print("=" * 64)

# Initialize MCP
mcp_call(
    "initialize",
    {
        "protocolVersion": "2025-03-26",
        "capabilities": {},
        "clientInfo": {"name": "e2e", "version": "1.0"},
    },
)
print(f"MCP Session: {SESSION}\n")

# ── 1. LOGIN PAGE ────────────────────────────────────────────
print("── 1. Authentication ──")

# Navigate returns snapshot directly (isolated mode)
snap = navigate(f"{FRONTEND}/login")
if contains(snap, "Sign in") and contains(snap, "Email") and contains(snap, "Password"):
    ok("Login page renders correctly")
else:
    fail("Login page render", snap[:300])

# Test invalid login (use run_code for multi-step in one context)
result = run_code(f"""
await page.goto('{FRONTEND}/login');
await page.locator('#email').fill('admin@localhost');
await page.locator('#password').fill('wrongpassword');
await page.locator('button[type="submit"]').click();
await page.waitForTimeout(2000);
""")
if contains(result, "Sign in") or contains(result, "invalid") or contains(result, "error") or contains(result, "login"):
    ok("Invalid login: stays on login page / shows error")
else:
    fail("Invalid login", result[:300])

# Test valid login
result = run_code(f"""
await page.goto('{FRONTEND}/login');
await page.locator('#email').fill('admin@localhost');
await page.locator('#password').fill('Changeme123');
await page.locator('button[type="submit"]').click();
await page.waitForTimeout(3000);
""")
if not contains(result, "Sign in") or contains(result, "Dashboard") or contains(result, "project"):
    ok("Valid login: redirects away from login")
else:
    fail("Valid login redirect", result[:300])

# Forgot password page
snap = navigate(f"{FRONTEND}/forgot-password")
if contains(snap, "forgot") or contains(snap, "reset") or contains(snap, "email"):
    ok("Forgot password page loads")
else:
    fail("Forgot password page", snap[:200])

# ── 2. DASHBOARD ─────────────────────────────────────────────
print("\n── 2. Dashboard ──")

# Use run_code to navigate authenticated (set token in localStorage first)
result = run_code(f"""
await page.goto('{FRONTEND}/login');
await page.locator('#email').fill('admin@localhost');
await page.locator('#password').fill('Changeme123');
await page.locator('button[type="submit"]').click();
await page.waitForTimeout(3000);
// Now we should be on dashboard
await page.waitForTimeout(1000);
""")

if contains(result, "project") or contains(result, "dashboard") or contains(result, "CodeForge"):
    ok("Dashboard page loads after login")
else:
    # Try direct navigation
    snap = navigate(f"{FRONTEND}/")
    if contains(snap, "project") or contains(snap, "dashboard"):
        ok("Dashboard page loads (direct nav)")
    else:
        warn("Dashboard page", f"Content: {result[:200]}")

# Check sidebar
sidebar_keywords = ["Dashboard", "Cost", "Model", "Mode", "Activity", "Knowledge", "Scope", "MCP", "Prompt", "Setting"]
found = sum(1 for kw in sidebar_keywords if contains(result, kw))
if found >= 7:
    ok(f"Sidebar navigation items ({found}/{len(sidebar_keywords)} found)")
elif found >= 4:
    warn("Sidebar navigation", f"{found}/{len(sidebar_keywords)} items visible")
else:
    warn("Sidebar navigation", f"Only {found}/{len(sidebar_keywords)} visible")

# Theme, WS, API status indicators
if contains(result, "theme") or contains(result, "dark") or contains(result, "light") or contains(result, "toggle"):
    ok("Theme toggle visible")
else:
    warn("Theme toggle", "Not detected in snapshot")

if contains(result, "connected") or contains(result, "WebSocket") or contains(result, "ws"):
    ok("WebSocket status indicator")
else:
    warn("WebSocket status", "Not detected")

# ── 3. ALL PAGES (via run_code with login) ───────────────────
print("\n── 3. Page Navigation ──")

pages = [
    ("/costs", ["cost", "spend", "budget", "usage", "total", "tracking"], "Cost Dashboard"),
    ("/models", ["model", "provider", "llm", "discover"], "Models"),
    ("/modes", ["mode", "architect", "coder", "agent"], "Modes"),
    ("/activity", ["activity", "event", "log", "recent"], "Activity"),
    ("/knowledge-bases", ["knowledge", "base", "document", "kb"], "Knowledge Bases"),
    ("/scopes", ["scope", "retrieval", "search"], "Scopes"),
    ("/mcp", ["mcp", "server", "tool", "protocol"], "MCP Servers"),
    ("/prompts", ["prompt", "template", "section", "system"], "Prompts"),
    ("/settings", ["setting", "config", "preference", "general"], "Settings"),
    ("/benchmarks", ["benchmark", "suite", "eval", "run"], "Benchmarks (dev)"),
]

for path, keywords, name in pages:
    result = run_code(f"""
await page.goto('{FRONTEND}/login');
await page.locator('#email').fill('admin@localhost');
await page.locator('#password').fill('Changeme123');
await page.locator('button[type="submit"]').click();
await page.waitForTimeout(3000);
await page.goto('{FRONTEND}{path}');
await page.waitForTimeout(2000);
""")
    if "[MCP_ERROR]" in result:
        fail(f"Page: {name} ({path})", f"MCP error: {result[:200]}")
        continue
    if contains(result, "something went wrong") or (contains(result, "error") and contains(result, "boundary")):
        fail(f"Page: {name} ({path})", "Error boundary triggered")
        continue
    found_kw = any(contains(result, kw) for kw in keywords)
    has_structure = (
        contains(result, "heading")
        or contains(result, "button")
        or contains(result, "table")
        or contains(result, "link")
    )
    if found_kw:
        ok(f"Page: {name} ({path})")
    elif has_structure:
        warn(f"Page: {name} ({path})", f"Page loaded but keywords {keywords} not detected")
    else:
        fail(f"Page: {name} ({path})", f"Page empty: {result[:200]}")

# 404
snap = navigate(f"{FRONTEND}/nonexistent-xyz-456")
if contains(snap, "not found") or contains(snap, "404") or contains(snap, "page"):
    ok("404 Not Found page")
else:
    fail("404 page", snap[:200])

# ── 4. API ENDPOINTS ─────────────────────────────────────────
print("\n── 4. API Endpoints ──")

TOKEN = api_login()
if TOKEN:
    ok("API: Login")
else:
    fail("API: Login", "No token")
    sys.exit(1)

api_tests = [
    ("GET", "/health", "Health"),
    ("GET", "/health/ready", "Readiness"),
    ("GET", "/api/v1/projects", "List Projects"),
    ("GET", "/api/v1/llm/models", "List Models"),
    ("GET", "/api/v1/modes", "List Modes"),
    ("GET", "/api/v1/policies", "List Policies"),
    ("GET", "/api/v1/settings", "Settings"),
    ("GET", "/api/v1/users", "Users"),
    ("GET", "/api/v1/mcp/servers", "MCP Servers"),
    ("GET", "/api/v1/knowledge-bases", "Knowledge Bases"),
    ("GET", "/api/v1/scopes", "Scopes"),
    ("GET", "/api/v1/benchmarks/suites", "Benchmark Suites"),
    ("GET", "/api/v1/prompt-sections", "Prompt Sections"),
    ("GET", "/api/v1/providers/git", "Git Providers"),
    ("GET", "/api/v1/providers/agent", "Agent Providers"),
    ("GET", "/api/v1/costs", "Cost Tracking"),
]

for method, path, name in api_tests:
    code, data = api(method, path)
    if code == 200:
        ok(f"API: {name} ({method} {path})")
    else:
        fail(f"API: {name} ({method} {path})", f"HTTP {code}")

# ── 5. CRUD OPERATIONS ───────────────────────────────────────
print("\n── 5. CRUD Operations ──")

# Project CRUD
code, data = api(
    "POST",
    "/api/v1/projects",
    {
        "name": "E2E Test Project",
        "description": "Created by E2E test",
        "repo_type": "git",
        "repo_url": "https://github.com/example/test-repo.git",
    },
)
pid = data.get("id", "")
if code in (200, 201) and pid:
    ok("Create Project")

    code, _ = api("GET", f"/api/v1/projects/{pid}")
    ok("Read Project") if code == 200 else fail("Read Project", f"HTTP {code}")

    code, _ = api("PUT", f"/api/v1/projects/{pid}", {"name": "E2E Updated", "description": "Updated"})
    ok("Update Project") if code == 200 else fail("Update Project", f"HTTP {code}")

    code, _ = api("DELETE", f"/api/v1/projects/{pid}")
    ok("Delete Project") if code in (200, 204) else fail("Delete Project", f"HTTP {code}")
else:
    fail("Create Project", f"HTTP {code}: {json.dumps(data)[:200]}")

# Mode CRUD
code, modes_data = api("GET", "/api/v1/modes")
if code == 200:
    modes = modes_data if isinstance(modes_data, list) else modes_data.get("modes", [])
    if len(modes) > 0:
        ok(f"List Modes: {len(modes)} modes found")
    else:
        warn("List Modes", "Empty list")

# MCP Server CRUD
code, data = api(
    "POST",
    "/api/v1/mcp/servers",
    {
        "name": "E2E Test MCP",
        "url": "http://localhost:9999/mcp",
        "transport": "streamable-http",
    },
)
mcp_id = data.get("id", "")
if code in (200, 201) and mcp_id:
    ok("Create MCP Server")
    code, _ = api("DELETE", f"/api/v1/mcp/servers/{mcp_id}")
    ok("Delete MCP Server") if code in (200, 204) else fail("Delete MCP Server", f"HTTP {code}")
else:
    fail("Create MCP Server", f"HTTP {code}: {json.dumps(data)[:200]}")

# ── 6. AUTH EDGE CASES ───────────────────────────────────────
print("\n── 6. Auth Edge Cases ──")

code, _ = api("POST", "/api/v1/auth/login", {"email": "admin@localhost", "password": "wrong"})
ok("Invalid login returns 401") if code == 401 else fail("Invalid login", f"HTTP {code}")

saved = TOKEN
TOKEN = ""
code, _ = api("GET", "/api/v1/projects")
ok("Unauth request returns 401") if code == 401 else fail("Unauth request", f"HTTP {code}")
TOKEN = saved

# Expired/invalid token
saved = TOKEN
TOKEN = "invalid.token.here"  # noqa: S105
code, _ = api("GET", "/api/v1/projects")
ok("Invalid token returns 401") if code == 401 else fail("Invalid token", f"HTTP {code}")
TOKEN = saved

# ── 7. WEBSOCKET ─────────────────────────────────────────────
print("\n── 7. WebSocket ──")

try:
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.settimeout(5)
    sock.connect(("localhost", 8080))
    key = base64.b64encode(os.urandom(16)).decode()
    handshake = (
        f"GET /ws HTTP/1.1\r\n"
        f"Host: localhost:8080\r\n"
        f"Upgrade: websocket\r\n"
        f"Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: {key}\r\n"
        f"Sec-WebSocket-Version: 13\r\n"
        f"\r\n"
    )
    sock.sendall(handshake.encode())
    response = sock.recv(4096).decode()
    sock.close()
    if "101" in response and "Upgrade" in response:
        ok("WebSocket handshake succeeds")
    else:
        fail("WebSocket handshake", f"Response: {response[:200]}")
except Exception as e:
    fail("WebSocket connection", str(e))

# ══════════════════════════════════════════════════════════════
# SUMMARY
# ══════════════════════════════════════════════════════════════

print("\n" + "=" * 64)
print("FINAL TEST REPORT")
print("=" * 64)

passed = sum(1 for r in results if r["s"] == "PASS")
failed = sum(1 for r in results if r["s"] == "FAIL")
partial = sum(1 for r in results if r["s"] == "PARTIAL")
total = len(results)

print()
for r in results:
    icon = {"PASS": "  OK ", "FAIL": "FAIL", "PARTIAL": "WARN"}[r["s"]]
    d = f" -- {r['d']}" if r["d"] else ""
    print(f"  [{icon}] {r['f']}{d}")

print(f"\nTotal: {total} | Passed: {passed} | Failed: {failed} | Partial: {partial}")
if total > 0:
    print(f"Pass rate: {passed * 100 / total:.1f}%")

# List failures for fix prioritization
if failed > 0:
    print("\nFailed tests requiring fixes:")
    for i, r in enumerate(results, 1):
        if r["s"] == "FAIL":
            print(f"  {i}. {r['f']}: {r['d']}")

sys.exit(1 if failed > 0 else 0)
