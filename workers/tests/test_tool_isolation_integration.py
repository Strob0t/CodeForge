"""Tool processes really run as the tool user (KI-71), with real setpriv processes.

Needs root (to start processes as other users and to set up a secret owned by
the worker user); skipped otherwise. scripts/check-tool-isolation.sh runs the
same checks in a container with the production settings (worker uid 10001
with ambient capabilities, a tmpfs secrets directory).
"""

from __future__ import annotations

import asyncio
import os
import shutil
import subprocess
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


# Run as another user: polls every process's command line and environment for
# the launch tokens while the go file exists; prints how many it saw, and how
# many launch helpers it saw at all (the poll works).
_POLLER = """
import os, sys
go, needle, helpers, seen = sys.argv[1], b"LEAKME", 0, set()
me = str(os.getpid())
while os.path.exists(go):
    for pid in os.listdir("/proc"):
        if not pid.isdigit() or pid == me:
            continue
        for what in ("cmdline", "environ"):
            try:
                with open(f"/proc/{pid}/{what}", "rb") as f:
                    data = f.read()
            except OSError:
                continue
            if what == "cmdline" and b"tool_exec.py" in data:
                helpers += 1
            index = data.find(needle)
            if index >= 0:
                seen.add(data[index:index + 10])
print(len(seen), helpers)
"""


async def test_no_secret_is_ever_on_a_tool_command_line(shared_tmp: Path) -> None:
    """KI-96 (E8): the KI-71 launcher passed the environment as env(1) arguments, which every
    process could read in /proc/<pid>/cmdline; the launch spec on a memfd is not readable."""
    import sys

    from codeforge.tool_process import configure_tool_isolation, start_tool_process
    from tests.tool_isolation_check import CONFIG

    assert configure_tool_isolation(CONFIG).ready
    go = shared_tmp / "polling"
    go.write_text("")
    go.chmod(0o644)
    setpriv = shutil.which("setpriv") or "setpriv"
    poller = subprocess.Popen(  # noqa: S603 - the test's own poller, as another user
        [setpriv, "--reuid=10003", "--regid=10003", "--clear-groups", "--",
         getattr(sys, "_base_executable", sys.executable), "-I", "-S", "-c", _POLLER, str(go)],
        stdout=subprocess.PIPE,
        text=True,
    )  # fmt: skip
    try:
        for index in range(200):
            proc = await start_tool_process(
                "sh", "-c", "sleep 0.01", env={"PATH": "/usr/bin:/bin", "TOKEN": f"LEAKME{index:04d}"}
            )
            assert await proc.wait() == 0
    finally:
        go.unlink()
        out, _ = poller.communicate(timeout=60)
    seen, helpers = (int(n) for n in out.split())
    assert helpers > 0, "the poller saw no launch at all"
    assert seen == 0, f"{seen} secrets read from other processes' command lines or environments"


async def test_a_tool_process_can_reopen_its_stdio_pipes() -> None:
    """Tools write to /dev/stdout and /dev/stderr (echo x > /dev/stderr, tee /dev/stderr, logging
    configs): reopening a pipe needs the pipe inode's permission, and the worker's pipes were 0600
    (KI-96, E11)."""
    from codeforge.tool_process import configure_tool_isolation, start_tool_process
    from tests.tool_isolation_check import CONFIG

    assert configure_tool_isolation(CONFIG).ready
    proc = await start_tool_process(
        "sh",
        "-c",
        "echo out > /dev/stdout && echo err > /dev/stderr && cat /dev/stdin > /dev/fd/1",
        env={"PATH": "/usr/bin:/bin"},
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    out, err = await proc.communicate(b"in\n")
    assert (proc.returncode, out, err) == (0, b"out\nin\n", b"err\n")

    proc = await start_tool_process(
        "sh",
        "-c",
        "echo merged > /dev/stderr",
        env={"PATH": "/usr/bin:/bin"},
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
    )
    assert proc.stdout is not None
    assert await proc.stdout.readline() == b"merged\n"
    assert await proc.wait() == 0


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


# What the tool user creates with owner-only modes (mkdtemp, mkdir -m 0700,
# umask 077) must stay readable and deletable for the worker and the Go Core
# (both in the workspace group): project deletion (GDPR erasure), git add -A
# of checkpoints and delivery, and the benchmark workspace cleanup (KI-71
# review, finding 7).
_PRIVATE_FILES = """
umask 077
mkdir -m 0700 private && echo secret > private/file
tmp=$(mktemp -d -p .) && echo tmp > "$tmp/file" && mv "$tmp" made-by-mktemp
mkdir other-group && chgrp 10002 other-group && chmod 0700 other-group && echo x > other-group/file
echo outside > "$OUTSIDE" && chmod 0600 "$OUTSIDE" && ln -s "$OUTSIDE" link-to-outside
"""


def _as_worker(args: list[str], cwd: Path) -> subprocess.CompletedProcess[str]:
    """Run *args* as the worker user (uid 10001, workspace group 10010), like the worker and the Go Core."""
    setpriv = ["setpriv", f"--reuid={WORKER_UID}", f"--regid={WORKER_UID}", f"--groups={WORKSPACE_GID}", "--"]
    return subprocess.run([*setpriv, *args], cwd=cwd, capture_output=True, text=True, check=False)  # noqa: S603


async def test_files_the_tool_user_keeps_private_are_shared_with_the_workspace_group(shared_tmp: Path) -> None:
    from codeforge.tool_process import configure_tool_isolation, start_tool_shell
    from tests.tool_isolation_check import CONFIG

    assert configure_tool_isolation(CONFIG).ready
    workspace = shared_tmp / "workspace"
    workspace.mkdir()
    os.chown(workspace, WORKER_UID, WORKSPACE_GID)
    workspace.chmod(0o2775)
    home = shared_tmp / "tool-home"  # the tool user's own directory, outside the workspace
    home.mkdir()
    os.chown(home, CONFIG.uid, CONFIG.gid)
    outside = home / "outside"

    proc = await start_tool_shell(
        _PRIVATE_FILES, env={"PATH": "/usr/bin:/bin", "OUTSIDE": str(outside)}, cwd=str(workspace)
    )
    assert await proc.wait() == 0

    read = _as_worker(["sh", "-c", "cat private/file made-by-mktemp/file other-group/file"], workspace)
    assert read.returncode == 0, read.stderr
    removed = _as_worker(["rm", "-rf", "private", "made-by-mktemp", "other-group"], workspace)
    assert removed.returncode == 0, removed.stderr
    # A symlink is never followed: the file it points to keeps its mode.
    assert outside.stat().st_mode & 0o777 == 0o600
