"""Tool processes really run as their tenant's tool user (KI-71, KI-96), with real setpriv processes.

Needs root (to start processes as other users and to set up a secret owned by
the worker user) and POSIX ACLs on /tmp; skipped otherwise.
scripts/check-tool-isolation.sh runs the same checks in a container with the
production settings (worker uid 10001 with ambient capabilities, a tmpfs
secrets directory).
"""

from __future__ import annotations

import asyncio
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from codeforge.tool_identity import tool_tenant
from codeforge.tool_process import configure_tool_isolation, start_tool_process, start_tool_shell
from tests.tool_isolation_check import TENANTS, isolation_config, make_tenant_dir, run_checks

if TYPE_CHECKING:
    from collections.abc import Iterator

WORKER_UID = 10001
WORKSPACE_GID = 10010
TENANT_A, UID_A = TENANTS["A"]

pytestmark = pytest.mark.skipif(
    not hasattr(os, "geteuid") or os.geteuid() != 0 or shutil.which("setpriv") is None,
    reason="needs root and setpriv (util-linux) to start processes as the tool user",
)


@pytest.fixture
def shared_tmp() -> Iterator[Path]:
    # pytest's tmp_path is private to root; the tool users must reach these.
    base = Path(tempfile.mkdtemp(prefix="cf-ki96-", dir="/tmp"))
    base.chmod(0o755)
    yield base
    shutil.rmtree(base, ignore_errors=True)


@pytest.fixture
def volumes(shared_tmp: Path) -> tuple[str, str]:
    """The workspace root and the tool HOME base, as the worker (here: root) owns them."""
    root = shared_tmp / "workspaces"
    root.mkdir()
    os.chown(root, os.getuid(), WORKSPACE_GID)
    root.chmod(0o2771)
    home_base = shared_tmp / "tool-homes"
    home_base.mkdir(mode=0o711)
    return str(root), str(home_base)


@pytest.fixture
def workspace_a(volumes: tuple[str, str]) -> str:
    """Isolation configured for the volumes; tenant A's project workspace."""
    root, home_base = volumes
    status = configure_tool_isolation(isolation_config(root, home_base))
    assert status.ready, status.reason
    return make_tenant_dir(root, TENANT_A, UID_A)


async def test_tool_processes_of_two_tenants_are_isolated(shared_tmp: Path, volumes: tuple[str, str]) -> None:
    root, home_base = volumes
    secret = shared_tmp / "secret"
    secret.write_text("internal-admin-key")
    os.chown(secret, WORKER_UID, WORKER_UID)
    secret.chmod(0o400)

    report, problems = await run_checks(root, home_base, [str(secret)])

    assert problems == [], report
    for name, (tenant_id, uid) in TENANTS.items():
        tenant = report[f"tenant {name} ({tenant_id}, uid {uid})"]
        assert tenant["credentials"]["Uid"].split() == [str(uid)] * 4  # type: ignore[index]
        assert tenant["credentials"]["Groups"] == ""  # type: ignore[index]
        assert tenant["credentials"]["CapEff"] == "0000000000000000"  # type: ignore[index]
        assert "Permission denied" in str(tenant["worker secrets"][str(secret)])  # type: ignore[index]
        assert "Operation not permitted" in str(tenant["denied"]["signal the other tenant's process"])  # type: ignore[index]


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


async def test_no_secret_is_ever_on_a_tool_command_line(shared_tmp: Path, workspace_a: str) -> None:
    """KI-96 (E8): the KI-71 launcher passed the environment as env(1) arguments, which every
    process could read in /proc/<pid>/cmdline; the launch spec on a memfd is not readable."""
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
        async with tool_tenant(TENANT_A, UID_A, workspace_a):
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


async def test_a_tool_process_can_reopen_its_stdio_pipes(workspace_a: str) -> None:
    """Tools write to /dev/stdout and /dev/stderr (echo x > /dev/stderr, tee /dev/stderr, logging
    configs): reopening a pipe needs the pipe inode's permission, and the worker's pipes were 0600
    (KI-96, E11)."""
    async with tool_tenant(TENANT_A, UID_A, workspace_a):
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


async def test_tmpdir_and_home_are_the_tenants(workspace_a: str, volumes: tuple[str, str]) -> None:
    """TMPDIR exists (the helper made it as the tool user) and lies below the tenant's HOME."""
    from codeforge.subprocess_env import tool_env

    _, home_base = volumes
    async with tool_tenant(TENANT_A, UID_A, workspace_a) as identity:
        assert identity is not None
        proc = await start_tool_shell(
            'echo "$HOME $TMPDIR" && mktemp && stat -c "%u %a" "$TMPDIR"',
            env=tool_env(),
            cwd=workspace_a,
            stdout=asyncio.subprocess.PIPE,
        )
        out, _ = await proc.communicate()
    home_line, temp_file, tmp_stat = out.decode().split("\n")[:3]
    assert home_line == f"{home_base}/{UID_A} {home_base}/{UID_A}/tmp/{identity.work_id}"
    assert temp_file.startswith(f"{home_base}/{UID_A}/tmp/{identity.work_id}/")
    assert tmp_stat == f"{UID_A} 700"


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
async def test_mcp_stdio_server_runs_as_the_tenants_tool_user(shared_tmp: Path, workspace_a: str) -> None:
    from codeforge.mcp_models import MCPServerDef
    from codeforge.mcp_workbench import McpServerConnection

    script = shared_tmp / "whoami_server.py"
    script.write_text(_MCP_SERVER)
    script.chmod(0o644)
    async with tool_tenant(TENANT_A, UID_A, workspace_a):
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
    assert fields["Uid"].split() == [str(UID_A)] * 4
    assert fields["Groups"].split() == []
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
echo outside > "$OUTSIDE" && chmod 0600 "$OUTSIDE" && ln -s "$OUTSIDE" link-to-outside
"""


def _as_worker(args: list[str], cwd: str) -> subprocess.CompletedProcess[str]:
    """Run *args* as the worker user (uid 10001, workspace group 10010), like the worker and the Go Core."""
    setpriv = ["setpriv", f"--reuid={WORKER_UID}", f"--regid={WORKER_UID}", f"--groups={WORKSPACE_GID}", "--"]
    return subprocess.run([*setpriv, *args], cwd=cwd, capture_output=True, text=True, check=False)  # noqa: S603


async def test_files_the_tool_user_keeps_private_are_shared_with_the_workspace_group(
    workspace_a: str, volumes: tuple[str, str]
) -> None:
    _, home_base = volumes
    outside = f"{home_base}/{UID_A}/outside"  # the tool user's own file, outside the workspace
    async with tool_tenant(TENANT_A, UID_A, workspace_a):
        proc = await start_tool_shell(
            _PRIVATE_FILES, env={"PATH": "/usr/bin:/bin", "OUTSIDE": outside}, cwd=workspace_a
        )
        assert await proc.wait() == 0

    read = _as_worker(["sh", "-c", "cat private/file made-by-mktemp/file"], workspace_a)
    assert read.returncode == 0, read.stderr
    removed = _as_worker(["rm", "-rf", "private", "made-by-mktemp"], workspace_a)
    assert removed.returncode == 0, removed.stderr
    # A symlink is never followed: the file it points to keeps its mode.
    assert os.stat(outside).st_mode & 0o777 == 0o600


# A tool locks the workspace group out under the default ACLs (KI-96 E1, E17):
# mode 0000, a stripped access ACL, a directory without its default ACL.
_LOCKOUT = """
import os
os.mkdir("locked"); open("locked/f", "w").write("x"); os.chmod("locked/f", 0); os.chmod("locked", 0)
open("stripped", "w").write("x"); os.removexattr("stripped", "system.posix_acl_access"); os.chmod("stripped", 0o600)
os.mkdir("nodefault"); os.removexattr("nodefault", "system.posix_acl_default"); os.chmod("nodefault", 0o700)
open("nodefault/g", "w").write("x"); os.chmod("nodefault/g", 0o600)
"""


@pytest.mark.skipif(not Path("/usr/bin/python3").exists(), reason="needs /usr/bin/python3 for the tool")
async def test_the_sharing_pass_undoes_an_acl_lockout(workspace_a: str) -> None:
    async with tool_tenant(TENANT_A, UID_A, workspace_a):
        proc = await start_tool_process(
            "/usr/bin/python3", "-c", _LOCKOUT, env={"PATH": "/usr/bin:/bin"}, cwd=workspace_a
        )
        assert await proc.wait() == 0
        # The pass after the call already opened them again.
        read = _as_worker(["cat", "locked/f", "stripped", "nodefault/g"], workspace_a)
        assert read.returncode == 0, read.stderr

    removed = _as_worker(["rm", "-rf", "locked", "stripped", "nodefault"], workspace_a)
    assert removed.returncode == 0, removed.stderr
