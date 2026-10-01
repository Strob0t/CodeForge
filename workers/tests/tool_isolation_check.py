"""End-to-end check of tool isolation (KI-71) with real processes.

Starts tool processes through codeforge.tool_process with isolation required
and reports their credentials and what they can reach. Used by
test_tool_isolation_integration.py (as root) and, in a container with the
production settings, by scripts/check-tool-isolation.sh:

    python -m tests.tool_isolation_check <workspace dir> <secret file>...

Standard library only (the container has no test dependencies). Prints a JSON
report; exits 1 when a check fails.
"""

from __future__ import annotations

import asyncio
import json
import os
import stat
import sys
from pathlib import Path

from codeforge.subprocess_utils import terminate_process_group
from codeforge.tool_process import IsolationConfig, configure_tool_isolation, start_tool_process

CONFIG = IsolationConfig(mode="required", uid=10002, gid=10002, workspace_gid=10010, home="/tmp")
STATUS_FIELDS = ("Uid", "Gid", "Groups", "CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb", "NoNewPrivs", "Umask")


async def _tool(command: str, cwd: str = "/") -> tuple[int, str]:
    proc = await start_tool_process(
        "/bin/sh",
        "-c",
        command,
        env={"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": CONFIG.home},
        cwd=cwd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
    )
    out, _ = await proc.communicate()
    return proc.returncode or 0, out.decode(errors="replace")


def _status_fields(text: str) -> dict[str, str]:
    fields = {}
    for line in text.splitlines():
        name, _, value = line.partition(":")
        if name in STATUS_FIELDS:
            fields[name] = value.strip()
    return fields


def _credential_problems(tool: dict[str, str]) -> list[str]:
    expected = {"Uid": "10002\t10002\t10002\t10002", "Gid": "10002\t10002\t10002\t10002", "Groups": "10010"}
    problems = [
        f"tool {name} is {tool.get(name)!r}, expected {value!r}"
        for name, value in expected.items()
        if tool.get(name) != value
    ]
    problems.extend(
        f"tool holds capabilities: {name}={tool.get(name)}"
        for name in ("CapInh", "CapPrm", "CapEff", "CapAmb")
        if int(tool.get(name, "1"), 16) != 0
    )
    if tool.get("NoNewPrivs") != "1":
        problems.append("tool runs without no_new_privs")
    return problems


async def run_checks(workspace: str, secret_files: list[str]) -> tuple[dict[str, object], list[str]]:
    """Return a report and the failed checks."""
    problems: list[str] = []
    status = configure_tool_isolation(CONFIG)
    report: dict[str, object] = {"worker": _status_fields(Path("/proc/self/status").read_text()), "ready": status.ready}
    if not status.ready:
        return report, [f"isolation not ready: {status.reason}"]

    _, out = await _tool("cat /proc/self/status")
    tool = _status_fields(out)
    report["tool"] = tool
    problems.extend(_credential_problems(tool))

    for path in [f"/proc/{os.getpid()}/environ", *secret_files]:
        code, out = await _tool(f"cat -- '{path}' >/dev/null")
        report[f"read {path}"] = out.strip() or f"exit {code}"
        if code == 0:
            problems.append(f"tool can read {path}")

    code, out = await _tool("echo x > tool-file && mkdir tool-dir && echo y > tool-dir/f", cwd=workspace)
    if code != 0:
        problems.append(f"tool cannot write the workspace: {out.strip()}")
    for name in ("tool-file", "tool-dir", "tool-dir/f"):
        info = os.stat(os.path.join(workspace, name))
        report[f"workspace {name}"] = f"{stat.filemode(info.st_mode)} {info.st_uid}:{info.st_gid}"
        if info.st_gid != CONFIG.workspace_gid or not info.st_mode & stat.S_IWGRP:
            problems.append(f"{name} is not writable for the workspace group: {report[f'workspace {name}']}")
    # The worker (in the workspace group) changes and removes what the tool created.
    with open(os.path.join(workspace, "tool-file"), "a") as f:
        f.write("worker\n")
    os.remove(os.path.join(workspace, "tool-dir", "f"))

    proc = await start_tool_process("sleep", "60", env={"PATH": "/usr/bin:/bin"}, start_new_session=True)
    await asyncio.wait_for(terminate_process_group(proc, grace_period=1.0), timeout=10)
    report["stop tool process"] = f"exit {proc.returncode}"
    if proc.returncode is None:
        problems.append("the worker cannot stop a tool process")
    return report, problems


def main(argv: list[str]) -> int:
    report, problems = asyncio.run(run_checks(argv[0], argv[1:]))
    report["problems"] = problems
    sys.stdout.write(json.dumps(report, indent=2) + "\n")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
