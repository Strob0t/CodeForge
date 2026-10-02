"""End-to-end check of per-tenant tool isolation (KI-71, KI-96) with real processes.

Sets up two tenants as the Go Core does (tenant directories with their tool
UIDs' ACLs, one project each), starts tool processes of both through
codeforge.tool_process with isolation required and reports their
credentials and what they could reach: the worker's environment and secret
files, the other tenant's workspace, HOME and processes, the levels above
their own workspace. Used by test_tool_isolation_integration.py (as root)
and, in a container with the production settings, by
scripts/check-tool-isolation.sh:

    python -m tests.tool_isolation_check <workspace root> <tool HOME base> <secret file>...

Standard library only (the container has no test dependencies). Prints a JSON
report; exits 1 when a check fails.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import os
import stat
import sys
from pathlib import Path

from codeforge import posix_acl
from codeforge.subprocess_env import tool_env
from codeforge.subprocess_utils import terminate_process_group
from codeforge.tool_identity import tool_tenant
from codeforge.tool_process import IsolationConfig, configure_tool_isolation, start_tool_process

TOOL_PATH = "/usr/local/bin:/usr/bin:/bin"
# name -> (tenant ID, tool UID)
TENANTS = {"A": ("tenant-a", 20000), "B": ("tenant-b", 20001)}
PROJECT = "p1"
STATUS_FIELDS = ("Uid", "Gid", "Groups", "CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb", "NoNewPrivs", "Umask")
_DIR = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
_DENIED = ("Permission denied", "Operation not permitted")


def isolation_config(root: str, home_base: str) -> IsolationConfig:
    return IsolationConfig(mode="required", workspace_root=root, home_base=home_base, tool_path=TOOL_PATH)


def make_tenant_dir(root: str, tenant_id: str, uid: int) -> str:
    """What the Go Core's workspaceacl.EnsureTenantDir does, plus one project as git clone or MkdirAll makes it."""
    root_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        with contextlib.suppress(FileExistsError):
            os.mkdir(tenant_id, 0o770, dir_fd=root_fd)
        fd = os.open(tenant_id, _DIR, dir_fd=root_fd)
        try:
            os.fchmod(fd, 0o2770)
            posix_acl.set_acl(fd, posix_acl.ACCESS, posix_acl.tenant_access(uid))
            posix_acl.set_acl(fd, posix_acl.DEFAULT, posix_acl.tenant_default(uid))
            with contextlib.suppress(FileExistsError):
                os.mkdir(PROJECT, 0o770, dir_fd=fd)
        finally:
            os.close(fd)
    finally:
        os.close(root_fd)
    return os.path.join(root, tenant_id, PROJECT)


async def _sh(command: str, cwd: str | None = None, env: dict[str, str] | None = None) -> tuple[int, str]:
    proc = await start_tool_process(
        "/bin/sh",
        "-c",
        command,
        env={**tool_env(), **(env or {})},
        cwd=cwd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
    )
    out, _ = await proc.communicate()
    return proc.returncode or 0, (out or b"").decode(errors="replace").strip()


def _status_fields(text: str) -> dict[str, str]:
    fields = {}
    for line in text.splitlines():
        name, _, value = line.partition(":")
        if name in STATUS_FIELDS:
            fields[name] = value.strip()
    return fields


def credential_problems(tool: dict[str, str], uid: int) -> list[str]:
    ids = "\t".join([str(uid)] * 4)
    expected = {"Uid": ids, "Gid": ids, "Groups": "", "Umask": "0007", "NoNewPrivs": "1"}
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
    return problems


def _denials(root: str, home_base: str, own: str, other: str, other_pid: int) -> dict[str, str]:
    """What a tool process of the tenant *own* must not do (shell commands, by check name)."""
    other_uid = TENANTS[other][1]
    other_ws = f"{root}/{TENANTS[other][0]}/{PROJECT}"
    return {
        "list the workspace root": f"ls '{root}'",
        "list its own tenant directory": f"ls '{root}/{TENANTS[own][0]}'",
        "create in its own tenant directory": f"mkdir '{root}/{TENANTS[own][0]}/new-project'",
        "list the other tenant's directory": f"ls '{root}/{TENANTS[other][0]}'",
        "list the other tenant's workspace": f"ls '{other_ws}'",
        "read the other tenant's file": f"cat '{other_ws}/secret.txt'",
        "write the other tenant's workspace": f"echo x > '{other_ws}/planted'",
        "list the worker's state": f"ls '{root}/.codeforge'",
        "list the HOME base": f"ls '{home_base}'",
        "list the other tenant's HOME": f"ls '{home_base}/{other_uid}'",
        "signal the other tenant's process": f"kill -0 {other_pid}",
        "read the other tenant's environment": f"cat /proc/{other_pid}/environ",
    }


async def _tenant_checks(
    name: str, root: str, home_base: str, secret_files: list[str], other_pid: int
) -> tuple[dict[str, object], list[str]]:
    tenant_id, uid = TENANTS[name]
    other = "B" if name == "A" else "A"
    workspace = f"{root}/{tenant_id}/{PROJECT}"
    report: dict[str, object] = {}
    problems: list[str] = []
    async with tool_tenant(tenant_id, uid, workspace):
        _, out = await _sh("cat /proc/self/status", cwd=workspace)
        tool = _status_fields(out)
        report["credentials"] = tool
        problems.extend(f"{name}: {p}" for p in credential_problems(tool, uid))

        reads = {}
        for path in [f"/proc/{os.getpid()}/environ", *secret_files]:
            code, out = await _sh(f"cat -- '{path}' >/dev/null", cwd=workspace)
            reads[path] = out or f"exit {code}"
            if code == 0:
                problems.append(f"{name}: a tool process can read {path}")
        report["worker secrets"] = reads

        denied = {}
        for check, command in _denials(root, home_base, name, other, other_pid).items():
            code, out = await _sh(command, cwd=workspace)
            denied[check] = out or f"exit {code}"
            if code == 0 or not any(word in out for word in _DENIED):
                problems.append(f"{name}: a tool process could {check} ({command}: exit {code} {out[:200]!r})")
        report["denied"] = denied

        code, out = await _sh(
            "echo x > tool-file && mkdir tool-dir && echo y > tool-dir/f && mkdir -m 0700 private && "
            'echo s > private/f && echo h > "$HOME/.cf-check" && test -d "$TMPDIR" && touch "$TMPDIR/t"',
            cwd=workspace,
        )
        if code != 0:
            problems.append(f"{name}: a tool process cannot write its workspace or HOME: {out}")
    # The end-of-work pass shared what the tools created; the worker (and the
    # Go Core, in the workspace group) can change and remove it.
    shared = {}
    for entry in ("tool-file", "tool-dir", "tool-dir/f", "private", "private/f"):
        path = os.path.join(workspace, entry)
        info = os.lstat(path)
        acl = posix_acl.get_acl(path, posix_acl.ACCESS) or []
        shared[entry] = f"{stat.filemode(info.st_mode)} {info.st_uid}:{info.st_gid} " + ",".join(map(str, acl))
        mask = next((e.perm for e in acl if e.tag == posix_acl.MASK), stat.S_IMODE(info.st_mode) >> 3 & 7)
        group = next((e.perm for e in acl if e.tag == posix_acl.GROUP and e.id == posix_acl.WORKSPACE_GID), 0)
        wanted = 7 if stat.S_ISDIR(info.st_mode) else 6
        if info.st_uid != uid or group & mask & wanted != wanted:
            problems.append(f"{name}: {entry} is not shared with the workspace group: {shared[entry]}")
    report["shared"] = shared
    with open(os.path.join(workspace, "tool-file"), "a") as f:
        f.write("worker\n")
    os.remove(os.path.join(workspace, "private", "f"))
    os.rmdir(os.path.join(workspace, "private"))
    return report, problems


async def run_checks(root: str, home_base: str, secret_files: list[str]) -> tuple[dict[str, object], list[str]]:
    """Return a report and the failed checks."""
    report: dict[str, object] = {"worker": _status_fields(Path("/proc/self/status").read_text())}
    status = configure_tool_isolation(isolation_config(root, home_base))
    report["ready"] = status.ready
    if not status.ready:
        return report, [f"isolation not ready: {status.reason}"]
    for tenant_id, uid in TENANTS.values():
        workspace = make_tenant_dir(root, tenant_id, uid)
        Path(workspace, "secret.txt").write_text(f"secret of {tenant_id}\n")

    problems: list[str] = []
    for name, other in (("A", "B"), ("B", "A")):
        tenant_id, uid = TENANTS[other]
        workspace = f"{root}/{tenant_id}/{PROJECT}"
        # A process of the other tenant, with a secret in its environment.
        async with tool_tenant(tenant_id, uid, workspace):
            env = {**tool_env(), "TENANT_SECRET": f"secret-{tenant_id}"}
            other_proc = await start_tool_process("sleep", "60", env=env, cwd=workspace, start_new_session=True)
            try:
                tenant_report, tenant_problems = await _tenant_checks(
                    name, root, home_base, secret_files, other_proc.pid
                )
                cmdline = Path(f"/proc/{other_proc.pid}/cmdline").read_bytes()
                if b"secret-" in cmdline:
                    tenant_problems.append(f"{other}: a tool command line carries its environment: {cmdline!r}")
            finally:
                await asyncio.wait_for(terminate_process_group(other_proc, grace_period=1.0), timeout=10)
            tenant_report["stop tool process"] = f"exit {other_proc.returncode}"
            if other_proc.returncode is None:
                tenant_problems.append("the worker cannot stop a tool process")
        report[f"tenant {name} ({TENANTS[name][0]}, uid {TENANTS[name][1]})"] = tenant_report
        problems.extend(tenant_problems)
    return report, problems


def main(argv: list[str]) -> int:
    report, problems = asyncio.run(run_checks(argv[0], argv[1], argv[2:]))
    report["problems"] = problems
    sys.stdout.write(json.dumps(report, indent=2, default=str) + "\n")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
