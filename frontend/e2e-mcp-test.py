"""E2E test driver using Playwright MCP container.

Drives a real browser in the Playwright container to test all frontend features.
The frontend runs at localhost:3000, but from the container's perspective,
the host is accessible via host.docker.internal or the Docker bridge IP.
"""

import json
import sys
import time
from urllib.parse import urlparse
from urllib.request import Request, urlopen


def _safe_urlopen(req: Request | str, *, timeout: int = 30):
    """Wrap urlopen with scheme validation (only http/https allowed)."""
    url = req.full_url if isinstance(req, Request) else req
    if urlparse(url).scheme not in ("http", "https"):
        msg = f"Unsafe URL scheme: {url}"
        raise ValueError(msg)
    return urlopen(req, timeout=timeout)


MCP_URL = "http://172.18.0.4:8001/mcp"
SESSION_ID: str | None = None
MSG_ID = 0

# From inside Docker container, the host services are at:
FRONTEND_URL = "http://host.docker.internal:3000"
BACKEND_URL = "http://host.docker.internal:8080"


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
            # Parse SSE format
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


def navigate(url: str) -> dict:
    return tool_call("browser_navigate", {"url": url})


def snapshot() -> str:
    r = tool_call("browser_snapshot")
    return r.get("text", "")


def click(ref: str) -> dict:
    return tool_call("browser_click", {"element": ref, "ref": ref})


def fill(ref: str, value: str) -> dict:
    return tool_call("browser_fill_form", {"values": [{"ref": ref, "value": value}]})


def wait_for(text: str, timeout: int = 10000) -> dict:
    return tool_call("browser_wait_for", {"text": text, "timeout": timeout})


def console_messages() -> dict:
    return tool_call("browser_console_messages")


def take_screenshot() -> dict:
    return tool_call("browser_take_screenshot")


# ──────────────────────────────────────────────────────────────
# Test Results
# ──────────────────────────────────────────────────────────────
results: list[dict] = []


def record(feature: str, status: str, detail: str = ""):
    icon = {"PASS": "PASS", "FAIL": "FAIL", "PARTIAL": "PARTIAL"}[status]
    results.append({"feature": feature, "status": status, "detail": detail})
    print(f"  [{icon}] {feature}" + (f" -- {detail}" if detail else ""))


# ──────────────────────────────────────────────────────────────
# Initialize session
# ──────────────────────────────────────────────────────────────
print("=== Initializing MCP session ===")
init = mcp_call(
    "initialize",
    {
        "protocolVersion": "2025-03-26",
        "capabilities": {},
        "clientInfo": {"name": "e2e-test", "version": "1.0"},
    },
)
print(f"Session: {SESSION_ID}")

# ──────────────────────────────────────────────────────────────
# 1. LOGIN PAGE
# ──────────────────────────────────────────────────────────────
print("\n=== 1. Login Page ===")
nav = navigate(f"{FRONTEND_URL}/login")
time.sleep(2)
snap = snapshot()

if "email" in snap.lower() and "password" in snap.lower():
    record("Login page renders", "PASS")
else:
    record("Login page renders", "FAIL", f"Snapshot: {snap[:200]}")

# Try login
fill_result = tool_call(
    "browser_fill_form",
    {
        "values": [
            {"ref": "email", "value": "admin@localhost"},
            {"ref": "password", "value": "Changeme123"},
        ]
    },
)

# Find and click submit button
snap = snapshot()
print(f"Login form snapshot (first 300 chars): {snap[:300]}")

# Click login button
submit = tool_call("browser_click", {"element": "Sign in", "ref": "Sign in"})
time.sleep(3)

snap = snapshot()
if "dashboard" in snap.lower() or "projects" in snap.lower() or "codeforge" in snap.lower():
    record("Login with valid credentials", "PASS")
else:
    record("Login with valid credentials", "FAIL", f"After login: {snap[:300]}")

# ──────────────────────────────────────────────────────────────
# 2. DASHBOARD
# ──────────────────────────────────────────────────────────────
print("\n=== 2. Dashboard ===")
navigate(f"{FRONTEND_URL}/")
time.sleep(2)
snap = snapshot()

if "dashboard" in snap.lower() or "project" in snap.lower():
    record("Dashboard loads", "PASS")
else:
    record("Dashboard loads", "FAIL", f"Snapshot: {snap[:300]}")

# ──────────────────────────────────────────────────────────────
# 3. NAVIGATION - Test each sidebar link
# ──────────────────────────────────────────────────────────────
print("\n=== 3. Navigation ===")
pages = [
    ("/costs", "costs", "Cost Dashboard"),
    ("/models", "model", "Models Page"),
    ("/modes", "mode", "Modes Page"),
    ("/activity", "activity", "Activity Page"),
    ("/knowledge-bases", "knowledge", "Knowledge Bases"),
    ("/scopes", "scope", "Scopes Page"),
    ("/mcp", "mcp", "MCP Servers"),
    ("/prompts", "prompt", "Prompts"),
    ("/settings", "setting", "Settings Page"),
    ("/benchmarks", "benchmark", "Benchmarks"),
]

for path, keyword, name in pages:
    navigate(f"{FRONTEND_URL}{path}")
    time.sleep(1.5)
    snap = snapshot()
    # Check for errors
    has_error = "error" in snap.lower() and "boundary" in snap.lower()
    has_content = keyword in snap.lower() or len(snap) > 100
    if has_error:
        record(f"Navigation: {name}", "FAIL", f"Error on page: {snap[:200]}")
    elif has_content:
        record(f"Navigation: {name}", "PASS")
    else:
        record(f"Navigation: {name}", "PARTIAL", f"Page loaded but keyword '{keyword}' not found: {snap[:200]}")

# ──────────────────────────────────────────────────────────────
# 4. NOT FOUND PAGE
# ──────────────────────────────────────────────────────────────
print("\n=== 4. 404 Page ===")
navigate(f"{FRONTEND_URL}/nonexistent-page")
time.sleep(1.5)
snap = snapshot()
if "not found" in snap.lower() or "404" in snap.lower():
    record("404 page", "PASS")
else:
    record("404 page", "FAIL", f"No 404 indicator: {snap[:200]}")

# ──────────────────────────────────────────────────────────────
# 5. API ENDPOINT TESTING
# ──────────────────────────────────────────────────────────────
print("\n=== 5. API Endpoints ===")


def api_get(path: str, token: str) -> tuple[int, dict]:
    req = Request(
        f"http://localhost:8080{path}",
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
    )
    try:
        with _safe_urlopen(req, timeout=10) as resp:
            return resp.status, json.loads(resp.read())
    except Exception as e:
        if hasattr(e, "code"):
            return e.code, {}
        return 0, {"error": str(e)}


def api_post(path: str, token: str, data: dict) -> tuple[int, dict]:
    req = Request(
        f"http://localhost:8080{path}",
        data=json.dumps(data).encode(),
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        method="POST",
    )
    try:
        with _safe_urlopen(req, timeout=10) as resp:
            return resp.status, json.loads(resp.read())
    except Exception as e:
        if hasattr(e, "code"):
            try:
                body = json.loads(e.read())
            except Exception:
                body = {}
            return e.code, body
        return 0, {"error": str(e)}


# Login and get token
_, login_data = api_post("/api/v1/auth/login", "", {"email": "admin@localhost", "password": "Changeme123"})
token = login_data.get("access_token", "")

if token:
    record("API: Login", "PASS")
else:
    record("API: Login", "FAIL", "No access token returned")

# Health
status, data = api_get("/health", token)
record("API: Health", "PASS" if status == 200 else "FAIL", f"status={status}")

status, data = api_get("/health/ready", token)
record("API: Readiness", "PASS" if status == 200 else "FAIL", f"status={status}")

# Projects
status, data = api_get("/api/v1/projects", token)
record("API: List Projects", "PASS" if status == 200 else "FAIL", f"status={status}")

# Models/LLM
status, data = api_get("/api/v1/llm/models", token)
record("API: List Models", "PASS" if status == 200 else "FAIL", f"status={status}")

# Modes
status, data = api_get("/api/v1/modes", token)
record("API: List Modes", "PASS" if status == 200 else "FAIL", f"status={status}")

# Costs
status, data = api_get("/api/v1/costs/summary", token)
record("API: Cost Summary", "PASS" if status == 200 else "FAIL", f"status={status}")

# Policies
status, data = api_get("/api/v1/policies", token)
record("API: List Policies", "PASS" if status == 200 else "FAIL", f"status={status}")

# Settings
status, data = api_get("/api/v1/settings", token)
record("API: Get Settings", "PASS" if status == 200 else "FAIL", f"status={status}")

# Users
status, data = api_get("/api/v1/users", token)
record("API: List Users", "PASS" if status == 200 else "FAIL", f"status={status}")

# MCP Servers
status, data = api_get("/api/v1/mcp/servers", token)
record("API: List MCP Servers", "PASS" if status == 200 else "FAIL", f"status={status}")

# Knowledge Bases
status, data = api_get("/api/v1/knowledge-bases", token)
record("API: List Knowledge Bases", "PASS" if status == 200 else "FAIL", f"status={status}")

# Scopes
status, data = api_get("/api/v1/scopes", token)
record("API: List Scopes", "PASS" if status == 200 else "FAIL", f"status={status}")

# Benchmarks (dev mode)
status, data = api_get("/api/v1/benchmarks/suites", token)
record("API: List Benchmark Suites", "PASS" if status == 200 else "FAIL", f"status={status}")

# Prompt Sections
status, data = api_get("/api/v1/prompt-sections", token)
record("API: List Prompt Sections", "PASS" if status == 200 else "FAIL", f"status={status}")

# Providers
status, data = api_get("/api/v1/providers/git", token)
record("API: Git Providers", "PASS" if status == 200 else "FAIL", f"status={status}")

status, data = api_get("/api/v1/providers/agent", token)
record("API: Agent Providers", "PASS" if status == 200 else "FAIL", f"status={status}")

# ──────────────────────────────────────────────────────────────
# SUMMARY
# ──────────────────────────────────────────────────────────────
print("\n" + "=" * 60)
print("E2E TEST SUMMARY")
print("=" * 60)

passed = sum(1 for r in results if r["status"] == "PASS")
failed = sum(1 for r in results if r["status"] == "FAIL")
partial = sum(1 for r in results if r["status"] == "PARTIAL")
total = len(results)

for r in results:
    icon = {"PASS": "  OK ", "FAIL": "FAIL", "PARTIAL": "WARN"}[r["status"]]
    detail = f" -- {r['detail']}" if r["detail"] else ""
    print(f"  [{icon}] {r['feature']}{detail}")

print(f"\nTotal: {total} | Passed: {passed} | Failed: {failed} | Partial: {partial}")
print(f"Pass rate: {passed / total * 100:.1f}%")

# Exit with failure if any test failed
sys.exit(1 if failed > 0 else 0)
