"""Tool processes really run as the tool user (KI-71), with real setpriv processes.

Needs root (to start processes as other users and to set up a secret owned by
the worker user); skipped otherwise. scripts/check-tool-isolation.sh runs the
same checks in a container with the production settings (worker uid 10001
with ambient capabilities, a tmpfs secrets directory).
"""

from __future__ import annotations

import os
import shutil
import tempfile
from pathlib import Path

import pytest

from tests.tool_isolation_check import run_checks

WORKER_UID = 10001
WORKSPACE_GID = 10010

pytestmark = pytest.mark.skipif(
    not hasattr(os, "geteuid") or os.geteuid() != 0 or shutil.which("setpriv") is None,
    reason="needs root and setpriv (util-linux) to start processes as the tool user",
)


@pytest.fixture
def shared_tmp() -> Path:
    # pytest's tmp_path is private to root; the tool user must reach these.
    base = Path(tempfile.mkdtemp(prefix="cf-ki71-", dir="/tmp"))
    base.chmod(0o755)
    yield base
    shutil.rmtree(base, ignore_errors=True)


async def test_tool_process_runs_isolated(shared_tmp: Path) -> None:
    workspace = shared_tmp / "workspace"
    workspace.mkdir()
    os.chown(workspace, WORKER_UID, WORKSPACE_GID)
    workspace.chmod(0o2775)

    secret = shared_tmp / "secret"
    secret.write_text("internal-admin-key")
    os.chown(secret, WORKER_UID, WORKER_UID)
    secret.chmod(0o400)

    report, problems = await run_checks(str(workspace), [str(secret)])

    assert problems == [], report
    assert report["tool"]["CapEff"] == "0000000000000000"  # type: ignore[index]
    assert report["tool"]["NoNewPrivs"] == "1"  # type: ignore[index]
    assert "Permission denied" in str(report[f"read {secret}"])
    assert "Permission denied" in str(report[f"read /proc/{os.getpid()}/environ"])


# A minimal MCP stdio server (newline-delimited JSON-RPC, standard library
# only, so the tool user can run it with the system Python): its one tool
# returns the server process's status.
_MCP_SERVER = """
import json, sys
for line in sys.stdin:
    msg = json.loads(line)
    method, mid = msg.get("method"), msg.get("id")
    if mid is None:
        continue
    if method == "initialize":
        result = {"protocolVersion": msg["params"]["protocolVersion"], "capabilities": {"tools": {}},
                  "serverInfo": {"name": "whoami", "version": "1"}}
    elif method == "tools/list":
        result = {"tools": [{"name": "whoami", "description": "status", "inputSchema": {"type": "object"}}]}
    elif method == "tools/call":
        with open("/proc/self/status") as f:
            result = {"content": [{"type": "text", "text": f.read()}], "isError": False}
    else:
        result = {}
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": mid, "result": result}) + "\\n")
    sys.stdout.flush()
"""


@pytest.mark.skipif(not Path("/usr/bin/python3").exists(), reason="needs /usr/bin/python3 for the server")
async def test_mcp_stdio_server_runs_as_the_tool_user(shared_tmp: Path) -> None:
    from codeforge.mcp_models import MCPServerDef
    from codeforge.mcp_workbench import McpServerConnection
    from codeforge.tool_process import configure_tool_isolation
    from tests.tool_isolation_check import CONFIG

    assert configure_tool_isolation(CONFIG).ready
    script = shared_tmp / "whoami_server.py"
    script.write_text(_MCP_SERVER)
    script.chmod(0o644)
    connection = McpServerConnection(
        MCPServerDef(id="whoami", name="whoami", transport="stdio", command="/usr/bin/python3", args=[str(script)])
    )
    await connection.connect()
    try:
        assert [tool.name for tool in await connection.list_tools()] == ["whoami"]
        result = await connection.call_tool("whoami", {})
    finally:
        await connection.disconnect()
    fields = dict(line.split(":", 1) for line in result.output.splitlines() if ":" in line)
    assert fields["Uid"].split() == ["10002"] * 4
    assert fields["Groups"].split() == ["10010"]
    assert fields["CapEff"].strip() == "0000000000000000"
    assert fields["CapAmb"].strip() == "0000000000000000"
    assert fields["NoNewPrivs"].strip() == "1"
