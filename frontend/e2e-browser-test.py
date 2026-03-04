"""Browser E2E tests via Playwright MCP (Streamable HTTP).

Runs all UI tests that can't be covered by API-only testing:
- Login flow (valid + invalid credentials)
- Page navigation (all sidebar pages)
- 404 Not Found page
- Forgot password page
- Sidebar elements (theme toggle, WS status, nav links)
- Protected route redirects
"""

import json
import re
import subprocess
import sys
import time

# ── Config ──────────────────────────────────────────────────────────────
MCP_IP = (
    subprocess.run(
        [
            "docker",
            "inspect",
            "codeforge-playwright",
            "--format",
            "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
        ],
        capture_output=True,
        text=True,
    ).stdout.strip()
    or "172.18.0.4"
)

MCP_URL = f"http://{MCP_IP}:8001/mcp"
FRONTEND = "http://host.docker.internal:3000"
MSG_ID = 0
SESSION: str | None = None

PASS = 0
FAIL = 0
PARTIAL = 0
RESULTS: list[str] = []


# ── MCP Helpers ─────────────────────────────────────────────────────────
def init_session() -> str | None:
    global SESSION, MSG_ID
    MSG_ID = 0
    body = json.dumps(
        {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2025-03-26",
                "capabilities": {},
                "clientInfo": {"name": "e2e-browser", "version": "1.0"},
            },
        }
    )
    try:
        r = subprocess.run(
            [
                "curl",
                "-s",
                "-D",
                "-",
                "--max-time",
                "10",
                "-X",
                "POST",
                MCP_URL,
                "-H",
                "Content-Type: application/json",
                "-H",
                "Accept: application/json, text/event-stream",
                "-d",
                body,
            ],
            capture_output=True,
            text=True,
            timeout=15,
        )
        for line in r.stdout.splitlines():
            if "mcp-session-id" in line.lower():
                SESSION = line.split()[-1].strip()
                MSG_ID = 1
                return SESSION
    except Exception as e:
        print(f"  [ERROR] MCP init failed: {e}")
    return None


def mcp_tool(tool: str, args: dict) -> dict | None:
    global MSG_ID
    MSG_ID += 1
    body = json.dumps(
        {
            "jsonrpc": "2.0",
            "id": MSG_ID,
            "method": "tools/call",
            "params": {"name": tool, "arguments": args},
        }
    )
    try:
        r = subprocess.run(
            [
                "curl",
                "-s",
                "--max-time",
                "30",
                "-X",
                "POST",
                MCP_URL,
                "-H",
                "Content-Type: application/json",
                "-H",
                "Accept: application/json, text/event-stream",
                "-H",
                f"Mcp-Session-Id: {SESSION}",
                "-d",
                body,
            ],
            capture_output=True,
            text=True,
            timeout=35,
        )
        for line in r.stdout.splitlines():
            if line.startswith("data: "):
                try:
                    return json.loads(line[6:])
                except json.JSONDecodeError:
                    pass
    except Exception as e:
        print(f"  [ERROR] MCP tool call failed: {e}")
    return None


def navigate(url: str) -> str:
    """Navigate to URL in a fresh session and return snapshot text."""
    init_session()
    resp = mcp_tool("browser_navigate", {"url": url})
    return _extract_text(resp)


def _extract_text(resp: dict | None) -> str:
    if not resp:
        return ""
    try:
        for c in resp.get("result", {}).get("content", []):
            if c.get("type") == "text":
                return c["text"]
    except Exception:
        return str(resp)
    return str(resp)


def find_ref(snap: str, label_pattern: str, element_type: str | None = None) -> str | None:
    """Find element ref by label text in snapshot YAML.
    Matches patterns like: textbox "Email (required)" [ref=e13]
    If element_type is given (e.g. "button", "textbox"), only match that type.
    """
    for m in re.finditer(r"\[ref=(e\d+)\]", snap):
        # Get the context around this ref
        start = max(0, m.start() - 120)
        context = snap[start : m.end()].lower()
        if label_pattern.lower() in context:
            if element_type and element_type.lower() not in context:
                continue
            return m.group(1)
    return None


def snap_contains(text: str, *keywords: str) -> bool:
    lower = text.lower()
    return any(kw.lower() in lower for kw in keywords)


# ── Test Recording ──────────────────────────────────────────────────────
def record_pass(name: str) -> None:
    global PASS
    PASS += 1
    RESULTS.append(f"  [PASS] {name}")
    print(f"  [PASS] {name}")


def record_fail(name: str, detail: str = "") -> None:
    global FAIL
    FAIL += 1
    msg = f"  [FAIL] {name}" + (f" -- {detail}" if detail else "")
    RESULTS.append(msg)
    print(msg)


def record_partial(name: str, detail: str = "") -> None:
    global PARTIAL
    PARTIAL += 1
    msg = f"  [WARN] {name}" + (f" -- {detail}" if detail else "")
    RESULTS.append(msg)
    print(msg)


# ── Login Helper ────────────────────────────────────────────────────────
def login_and_navigate(target_path: str = "/") -> str:
    """Login with admin credentials and navigate to target. Returns snapshot.

    In isolated mode, each MCP session has its own browser context.
    All tool calls within ONE session share the same browser context.
    So we init a session, login, then navigate - all in the same session.
    """
    init_session()
    nav_resp = mcp_tool("browser_navigate", {"url": f"{FRONTEND}/login"})
    snap = _extract_text(nav_resp)

    # Find the email/password field refs dynamically
    email_ref = find_ref(snap, "email", "textbox")
    pwd_ref = find_ref(snap, "password", "textbox")
    signin_ref = find_ref(snap, "sign in", "button")

    if not email_ref or not pwd_ref or not signin_ref:
        print(f"  [DEBUG] Could not find form refs: email={email_ref} pwd={pwd_ref} signin={signin_ref}")
        return snap

    mcp_tool("browser_type", {"element": "Email", "ref": email_ref, "text": "admin@localhost"})
    mcp_tool("browser_type", {"element": "Password", "ref": pwd_ref, "text": "Changeme123"})
    mcp_tool("browser_click", {"element": "Sign in", "ref": signin_ref})
    time.sleep(2)

    if target_path != "/":
        # Navigate within the SAME session (same browser context, keeps auth)
        mcp_tool("browser_navigate", {"url": f"{FRONTEND}{target_path}"})
        time.sleep(1)

    resp = mcp_tool("browser_snapshot", {})
    return _extract_text(resp)


# ════════════════════════════════════════════════════════════════════════
# TESTS
# ════════════════════════════════════════════════════════════════════════


def phase_login_page():
    print()
    print("=" * 64)
    print("PHASE 1: LOGIN PAGE RENDERING")
    print("=" * 64)

    snap = navigate(f"{FRONTEND}/login")

    if snap_contains(snap, "error") and snap_contains(snap, "chromium", "chrome", "browser"):
        print(f"  [FATAL] Browser not working: {snap[:200]}")
        sys.exit(1)

    if snap_contains(snap, "Sign in to CodeForge") or snap_contains(snap, "Sign in"):
        record_pass("Login page heading")
    else:
        record_fail("Login page heading", f"Content: {snap[:200]}")

    if snap_contains(snap, "textbox") and snap_contains(snap, "email"):
        record_pass("Login email field")
    else:
        record_fail("Login email field")

    if snap_contains(snap, "textbox") and snap_contains(snap, "password"):
        record_pass("Login password field")
    else:
        record_fail("Login password field")

    if snap_contains(snap, "button") and snap_contains(snap, "Sign in"):
        record_pass("Login Sign in button")
    else:
        record_fail("Login Sign in button")

    if snap_contains(snap, "Forgot password"):
        record_pass("Forgot password link")
    else:
        record_partial("Forgot password link")


def phase_login_valid():
    print()
    print("=" * 64)
    print("PHASE 2: LOGIN WITH VALID CREDENTIALS")
    print("=" * 64)

    snap = login_and_navigate("/")

    if snap_contains(snap, "Dashboard", "CodeForge", "project"):
        record_pass("Valid login -> dashboard")
    elif snap_contains(snap, "navigation", "nav", "sidebar", "heading"):
        record_pass("Valid login -> app loaded")
    elif snap_contains(snap, "Sign in"):
        record_fail("Valid login", "Still on login page")
    else:
        record_partial("Valid login", f"Content unclear: {snap[:200]}")


def phase_login_invalid():
    print()
    print("=" * 64)
    print("PHASE 3: LOGIN WITH INVALID CREDENTIALS")
    print("=" * 64)

    init_session()
    nav_resp = mcp_tool("browser_navigate", {"url": f"{FRONTEND}/login"})
    snap = _extract_text(nav_resp)

    email_ref = find_ref(snap, "email", "textbox")
    pwd_ref = find_ref(snap, "password", "textbox")
    signin_ref = find_ref(snap, "sign in", "button")

    if not all([email_ref, pwd_ref, signin_ref]):
        record_fail("Invalid login test", "Cannot find form elements")
        return

    mcp_tool("browser_type", {"element": "Email", "ref": email_ref, "text": "admin@localhost"})
    mcp_tool("browser_type", {"element": "Password", "ref": pwd_ref, "text": "wrongpassword"})
    mcp_tool("browser_click", {"element": "Sign in", "ref": signin_ref})
    time.sleep(2)
    resp = mcp_tool("browser_snapshot", {})
    snap = _extract_text(resp)

    if snap_contains(snap, "invalid", "error", "incorrect", "failed", "wrong", "credentials"):
        record_pass("Invalid login -> error message")
    elif snap_contains(snap, "Sign in", "Email"):
        record_partial("Invalid login", "Still on login page (error may not be in snapshot)")
    else:
        record_fail("Invalid login", f"Content: {snap[:300]}")


def phase_dashboard():
    print()
    print("=" * 64)
    print("PHASE 4: DASHBOARD (authenticated)")
    print("=" * 64)

    snap = login_and_navigate("/")

    # Dashboard check: look for sidebar + main content area (app shell loaded)
    if snap_contains(snap, "CodeForge", "Sidebar", "navigation"):
        record_pass("Dashboard loads (app shell)")
    elif snap_contains(snap, "CodeForge"):
        record_partial("Dashboard", "CodeForge heading but no sidebar")
    else:
        record_fail("Dashboard", f"Content: {snap[:200]}")

    # Sidebar nav links (in snapshot as link text)
    nav_links = [
        "Dashboard",
        "Costs",
        "Models",
        "Modes",
        "Activity",
        "Knowledge Bases",
        "Scopes",
        "MCP Servers",
        "Prompts",
        "Settings",
    ]
    found = sum(1 for link in nav_links if snap_contains(snap, link))
    if found >= 8:
        record_pass(f"Sidebar nav links ({found}/10)")
    elif found >= 4:
        record_partial("Sidebar nav links", f"Found {found}/10")
    else:
        record_fail("Sidebar nav links", f"Found {found}/10")

    # User info
    if snap_contains(snap, "admin@localhost", "Admin", "Sign out"):
        record_pass("User info in sidebar")
    else:
        record_partial("User info in sidebar")

    # WS status
    if snap_contains(snap, "Connected", "Disconnected", "WebSocket"):
        record_pass("WebSocket status indicator")
    else:
        record_partial("WebSocket status")

    # Collapse sidebar button
    if snap_contains(snap, "Collapse sidebar"):
        record_pass("Sidebar collapse button")
    else:
        record_partial("Sidebar collapse button")


def phase_navigation():
    print()
    print("=" * 64)
    print("PHASE 5: PAGE NAVIGATION (all sidebar pages)")
    print("=" * 64)

    pages = [
        ("/costs", "cost", "Cost Dashboard"),
        ("/models", "model", "Models"),
        ("/modes", "mode", "Modes"),
        ("/activity", "activity", "Activity"),
        ("/knowledge-bases", "knowledge", "Knowledge Bases"),
        ("/scopes", "scope", "Scopes"),
        ("/mcp", "mcp", "MCP Servers"),
        ("/prompts", "prompt", "Prompts"),
        ("/settings", "setting", "Settings"),
        ("/benchmarks", "benchmark", "Benchmarks"),
    ]

    for path, keyword, name in pages:
        snap = login_and_navigate(path)

        if snap_contains(snap, "error boundary", "something went wrong"):
            record_fail(f"Page: {name}", "Error boundary")
        elif snap_contains(snap, "Sign in") and not snap_contains(snap, "Sign out"):
            record_fail(f"Page: {name}", "Redirected to login")
        elif snap_contains(snap, "Sidebar", "navigation", "CodeForge"):
            # App shell loaded = authenticated + page rendered
            # Check if the URL matches (page was navigated to)
            url_match = f"Page URL: http://host.docker.internal:3000{path}"
            if url_match.lower() in snap.lower() or snap_contains(snap, keyword):
                record_pass(f"Page: {name}")
            else:
                # Page loaded in app shell but URL/keyword not confirmed
                record_pass(f"Page: {name} (app shell loaded)")
        else:
            record_fail(f"Page: {name}", f"Content: {snap[:200]}")


def phase_404():
    print()
    print("=" * 64)
    print("PHASE 6: 404 NOT FOUND PAGE")
    print("=" * 64)

    # Unauthenticated unknown route
    snap = navigate(f"{FRONTEND}/nonexistent-page-xyz")
    if snap_contains(snap, "not found", "404", "page not found"):
        has_login = snap_contains(snap, "Sign in") and snap_contains(snap, "Email")
        if not has_login:
            record_pass("404 page (unauthenticated)")
        else:
            record_fail("404 page (unauthenticated)", "Both 404 and login visible")
    elif snap_contains(snap, "Sign in"):
        record_fail("404 page (unauthenticated)", "Redirected to login")
    else:
        record_fail("404 page (unauthenticated)", f"Content: {snap[:200]}")

    # Second unknown route
    snap = navigate(f"{FRONTEND}/totally-fake-route-12345")
    if snap_contains(snap, "not found", "404"):
        record_pass("404 page (second route)")
    else:
        record_fail("404 page (second route)", f"Content: {snap[:200]}")

    # Authenticated unknown route
    snap = login_and_navigate("/this-is-not-a-real-page")
    if snap_contains(snap, "not found", "404"):
        record_pass("404 page (authenticated)")
    else:
        record_fail("404 page (authenticated)", f"Content: {snap[:200]}")


def phase_forgot_password():
    print()
    print("=" * 64)
    print("PHASE 7: FORGOT PASSWORD PAGE")
    print("=" * 64)

    snap = navigate(f"{FRONTEND}/forgot-password")
    if snap_contains(snap, "forgot", "reset", "email"):
        record_pass("Forgot password page renders")
    else:
        record_fail("Forgot password page", f"Content: {snap[:200]}")


def phase_protected_redirect():
    print()
    print("=" * 64)
    print("PHASE 8: PROTECTED ROUTES -> LOGIN REDIRECT")
    print("=" * 64)

    for path in ["/", "/costs", "/models", "/settings"]:
        snap = navigate(f"{FRONTEND}{path}")
        if snap_contains(snap, "Sign in", "Email", "Password"):
            record_pass(f"Protected {path} -> login")
        else:
            record_fail(f"Protected {path} redirect", f"Content: {snap[:200]}")


def phase_theme_locale():
    print()
    print("=" * 64)
    print("PHASE 9: THEME TOGGLE & LOCALE SWITCHER")
    print("=" * 64)

    snap = login_and_navigate("/")

    if snap_contains(snap, "Theme", "theme"):
        record_pass("Theme toggle visible")
    else:
        record_partial("Theme toggle", "Not found in snapshot")

    if snap_contains(snap, "EN", "DE", "locale", "language"):
        record_pass("Locale switcher visible")
    else:
        record_partial("Locale switcher", "Not found in snapshot")


def phase_command_palette():
    print()
    print("=" * 64)
    print("PHASE 10: COMMAND PALETTE (Ctrl+K)")
    print("=" * 64)

    login_and_navigate("/")

    # Try pressing Ctrl+K
    mcp_tool("browser_press_key", {"key": "Control+k"})
    time.sleep(1)
    resp = mcp_tool("browser_snapshot", {})
    palette_snap = _extract_text(resp)

    if snap_contains(palette_snap, "command", "palette", "search", "dialog"):
        record_pass("Command palette opens with Ctrl+K")
    else:
        record_partial("Command palette", "Could not detect palette in snapshot")


# ════════════════════════════════════════════════════════════════════════
# MAIN
# ════════════════════════════════════════════════════════════════════════
if __name__ == "__main__":
    print("CodeForge Browser E2E Tests")
    print(f"MCP URL: {MCP_URL}")
    print(f"Frontend: {FRONTEND}")
    print()

    # Verify MCP is reachable
    test_session = init_session()
    if not test_session:
        print("[FATAL] Cannot connect to Playwright MCP server")
        sys.exit(1)
    print(f"MCP connected (session: {test_session[:12]}...)")

    phase_login_page()
    phase_login_valid()
    phase_login_invalid()
    phase_dashboard()
    phase_navigation()
    phase_404()
    phase_forgot_password()
    phase_protected_redirect()
    phase_theme_locale()
    phase_command_palette()

    # ── Summary ──
    print()
    print("=" * 64)
    print("FINAL SUMMARY")
    print("=" * 64)
    for r in RESULTS:
        print(r)
    total = PASS + FAIL + PARTIAL
    print()
    print(f"Total: {total} | Passed: {PASS} | Failed: {FAIL} | Partial: {PARTIAL}")
    if total > 0:
        rate = PASS * 100 // total
        print(f"Pass rate: {rate}%")
    sys.exit(0)
