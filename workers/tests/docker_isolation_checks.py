"""In-container checks of per-tenant tool isolation (KI-96), run in the built worker image.

``test_tenant_isolation_docker.py`` runs each command with the production
service definition (``docker compose run`` of the worker service of
docker-compose.prod.yml: started as root, the entrypoint drops to the worker
user with SETUID, SETGID and KILL; read-only root; the workspaces and
tool_homes volumes; the 1771 /tmp) and the isolation settings of the image
and the compose file (``CODEFORGE_TOOL_ISOLATION=required``, Landlock
following it, ``APP_ENV=production``):

    python -m tests.docker_isolation_checks <command> [args]

Commands (each prints one ``CF-REPORT <json>`` line, exits 1 on a problem):

- ``image``: the generated tool users and the image layout.
- ``battery``: what agents run, as tenant A's tool user under Landlock:
  the system python3 and pip (never the worker's venv), a venv, git with
  the default identity, ptys, /dev/shm locks, /dev/stderr, scripts and
  binaries run from TMPDIR and HOME, pytest, ``python -m pytest`` and ruff
  (also through the quality gate's executor: the default gate commands of
  Python projects and the auto-agent's workspace test); node/npm/npx, go
  test and java when the image has them (the battery image).
- ``backends``: the agent backend CLIs of the image (KI-118): the executors
  find them on the tool PATH, Claude Code's has every option its executor
  uses, and each runs as tenant A's tool user under Landlock.
- ``refused-call``: a tool call when isolation is not ready starts nothing.
- ``migration``: a tree the KI-71 worker left (files of 10002, hard links
  into another tenant's tree, planted ACL entries) is migrated (D9); a
  tenant directory 10002 replaced is refused.
- ``scale-seed N`` / ``scale-migrate``: a legacy tree of N entries, migrated
  while the event loop keeps ticking (heartbeats).
- ``deletion``: a project tree a crashed tool locked the worker out of is
  removed through the worker as the tenant (D11).
- ``ki71-walk`` / ``after-rollback``: an older worker's start on the volume
  is detected and the state moved aside (D9).
- ``hold SECONDS`` / ``contend WAIT``: two workers on one volume; a
  migration waits for the tenant's work in the other worker.
- ``pin SECONDS``: a tool process keeps reading its own /proc entry while
  the host drops its caches (E6).
"""

from __future__ import annotations

import asyncio
import contextlib
import grp
import json
import os
import pwd
import shutil
import stat
import subprocess
import sys
import time
from dataclasses import replace
from pathlib import Path

from codeforge import posix_acl, tool_migration, tool_state
from codeforge.backends import build_default_router
from codeforge.claude_code_executor import ClaudeCodeCLIError, resolve_cli
from codeforge.config import get_settings
from codeforge.subprocess_env import tool_env
from codeforge.tool_identity import ToolIdentity, ToolIsolationError, accept_identity, tool_tenant, use_identity
from codeforge.tool_process import IsolationConfig, configure_tool_isolation, start_tool_process
from codeforge.workspace_deletion import delete_workspace
from tests.tool_isolation_check import TENANTS, make_tenant_dir

LEGACY_UID = 10002
WORKER_UID = 10001
WORKSPACE_GID = 10010
TENANT_A, UID_A = TENANTS["A"]
TENANT_B, UID_B = TENANTS["B"]
TENANT_C, UID_C = "tenant-c", 20002
_SETPRIV = shutil.which("setpriv") or "/usr/bin/setpriv"

Report = dict[str, object]


def _config() -> IsolationConfig:
    return IsolationConfig.from_settings(get_settings())


def _ready() -> IsolationConfig:
    config = _config()
    status = configure_tool_isolation(config)
    if not status.ready:
        raise SystemExit(_emit({"ready": False, "reason": status.reason}, [f"not ready: {status.reason}"]))
    return config


def _emit(report: Report, problems: list[str]) -> int:
    report["problems"] = problems
    sys.stdout.write("CF-REPORT " + json.dumps(report, default=str) + "\n")
    sys.stdout.flush()
    return 1 if problems else 0


async def _sh(command: str, cwd: str | None, extra: dict[str, str] | None = None) -> tuple[int, str]:
    proc = await start_tool_process(
        "/bin/bash",
        "-c",
        command,
        env=tool_env(extra=extra),
        cwd=cwd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
    )
    out, _ = await proc.communicate()
    return proc.returncode or 0, (out or b"").decode(errors="replace").strip()


def _as(uid: int, groups: str, command: str, cwd: str = "/") -> subprocess.CompletedProcess[str]:
    """Run *command* as *uid* (the worker may: CAP_SETUID), the way an older tool or the worker would."""
    setpriv = [_SETPRIV, f"--reuid={uid}", f"--regid={uid}", f"--groups={groups}" if groups else "--clear-groups"]
    return subprocess.run(  # noqa: S603 - the check's own fixed commands
        [*setpriv, "--", "/bin/sh", "-c", command], cwd=cwd, capture_output=True, text=True, check=False
    )


# ---------------------------------------------------------------------------
# image
# ---------------------------------------------------------------------------


def _user_problems(report: Report) -> list[str]:
    """10,001 generated users and groups, one each, no subordinate IDs; 10002 retired."""
    expected = {f"codeforge-t{uid}": uid for uid in range(20000, 30000)} | {"codeforge-system": 19999}
    generated = [e for e in pwd.getpwall() if e.pw_name in expected]
    report["generated passwd entries"] = len(generated)
    problems = [] if len(generated) == 10_001 else [f"{len(generated)} generated passwd entries, expected 10001"]
    home, shell = "/home/codeforge-tools", "/usr/sbin/nologin"
    problems.extend(
        f"passwd entry {e}"
        for e in generated
        if (e.pw_uid, e.pw_gid, e.pw_dir, e.pw_shell)
        != (expected[e.pw_name], expected[e.pw_name], f"{home}/{expected[e.pw_name]}", shell)
    )
    groups = [g for g in grp.getgrall() if g.gr_name in expected]
    report["generated group entries"] = len(groups)
    if len(groups) != 10_001 or any(g.gr_gid != expected[g.gr_name] or g.gr_mem for g in groups):
        problems.append("the generated groups are not one per tool user, without members")
    for name in ("/etc/subuid", "/etc/subgid"):
        text = Path(name).read_text() if Path(name).exists() else ""
        report[name] = len(text.splitlines())
        if "codeforge" in text:
            problems.append(f"{name} has entries for CodeForge users")
    legacy, workspace = pwd.getpwuid(LEGACY_UID), grp.getgrgid(WORKSPACE_GID)
    report["legacy"] = f"{legacy.pw_name} home {legacy.pw_dir}"
    report["workspace group members"] = workspace.gr_mem
    if legacy.pw_name != "codeforge-tool-legacy":
        problems.append(f"10002 is {legacy.pw_name}, expected codeforge-tool-legacy")
    if workspace.gr_mem != ["codeforge"]:
        problems.append(f"the workspace group has the members {workspace.gr_mem}, expected only the worker")
    return problems


def _layout_problems(report: Report) -> list[str]:
    problems = []
    for path, mode, uid, gid in (
        ("/data/workspaces", 0o2771, WORKER_UID, WORKSPACE_GID),
        ("/home/codeforge-tools", 0o711, WORKER_UID, WORKER_UID),
        ("/var/lib/codeforge/landlock-canary", 0o644, 0, 0),
    ):
        info = os.stat(path)
        report[path] = f"{stat.S_IMODE(info.st_mode):o} {info.st_uid}:{info.st_gid}"
        if (stat.S_IMODE(info.st_mode), info.st_uid, info.st_gid) != (mode, uid, gid):
            problems.append(f"{path} is {report[path]}, expected {mode:o} {uid}:{gid}")
    compiled = list(Path("/usr/local/lib/python3.12/json/__pycache__").glob("*.cpython-312.pyc"))
    report["compiled stdlib"] = bool(compiled)
    if not compiled:
        problems.append("the stdlib is not compiled (every tool launch imports it from source)")
    problems.extend(
        f"{binary} is missing"
        for binary in ("setfacl", "getfacl", "setpriv", "git", "script")
        if not shutil.which(binary)
    )
    vcs = shutil.which("git") or "/usr/bin/git"
    config = subprocess.run(  # noqa: S603 - fixed command
        [vcs, "config", "--system", "--list"], capture_output=True, text=True, check=False
    ).stdout
    report["system gitconfig"] = config.split("\n")
    problems.extend(
        f"/etc/gitconfig lacks {line}"
        for line in ("safe.directory=*", "user.name=CodeForge agent", "user.email=agent@codeforge.invalid")
        if line not in config
    )
    problems.extend(
        f"the image still sets {name}"
        for name in ("CODEFORGE_TOOL_UID", "CODEFORGE_TOOL_GID", "CODEFORGE_TOOL_HOME")
        if name in os.environ
    )
    return problems


def image() -> int:
    report: Report = {}
    problems = _user_problems(report) + _layout_problems(report)
    return _emit(report, problems)


# ---------------------------------------------------------------------------
# battery
# ---------------------------------------------------------------------------

# name -> (command, text the output must contain); run in tenant A's workspace.
BATTERY: dict[str, tuple[str, str]] = {
    "python3 is the system interpreter": ("python3 -c 'import sys; print(sys.prefix)'", "/usr/local"),
    "pip runs": ("pip --version", "pip "),
    "a venv with pip": ('python3 -m venv "$TMPDIR/venv" && "$TMPDIR/venv/bin/python" -m pip --version', "pip "),
    "the worker's venv is not on PATH": (
        'case "$PATH" in */app/.venv*) echo venv-on-path ;; *) echo clean ;; esac',
        "clean",
    ),
    "id -un names the tenant's tool user": ("id -un", f"codeforge-t{UID_A}"),
    "git commits with the default identity": (
        "git init -q repo && cd repo && echo a > f && git add f && git commit -qm init && "
        "echo b >> f && git stash -q && git stash pop -q && git gc -q && git log -1 --format=%an/%ae",
        "CodeForge agent/agent@codeforge.invalid",
    ),
    "shell features, mv, cp, ln": (
        "cat <<EOF > h\nhd\nEOF\ncat <(echo psub) && mkdir d && mv h d/h && cp d/h h2 && ln h2 h3 && "
        "ln -s h2 h4 && cat h4 && grep -r hd . >/dev/null && echo shell-ok",
        "shell-ok",
    ),
    "a pseudo-terminal": ("python3 -c 'import os; os.openpty(); print(\"pty-ok\")'", "pty-ok"),
    "script(1)": ("script -qc 'echo in-script' /dev/null", "in-script"),
    "multiprocessing locks in /dev/shm": (
        "python3 -c 'import multiprocessing as m; m.Lock(); print(\"lock-ok\")'",
        "lock-ok",
    ),
    "/dev/shm cannot be listed": ("ls /dev/shm 2>&1; true", "Permission denied"),
    "writes to /dev/stderr": ("echo err-ok > /dev/stderr", "err-ok"),
    "/dev/tty without a terminal is ENXIO, not EACCES": ("cat /dev/tty 2>&1; true", "No such device or address"),
    "TMPDIR is the work item's, below HOME": ('echo "$TMPDIR"', f"/home/codeforge-tools/{UID_A}/tmp/"),
    "a script in TMPDIR runs": (
        'printf "#!/bin/sh\\necho tmp-exec\\n" > "$TMPDIR/s" && chmod +x "$TMPDIR/s" && "$TMPDIR/s"',
        "tmp-exec",
    ),
    "a binary in HOME runs (the volume is not noexec)": (
        'mkdir -p "$HOME/.local/bin" && cp /usr/bin/true "$HOME/.local/bin/cf-true" && cf-true && echo home-exec',
        "home-exec",
    ),
    "HOME is writable, /tmp is not": (
        'touch "$HOME/x" && ! touch /tmp/cf-x 2>/dev/null && echo tmp-closed',
        "tmp-closed",
    ),
    "the JVM and tmux temp settings": (
        'echo "$JAVA_TOOL_OPTIONS $TMUX_TMPDIR"',
        "-Djava.io.tmpdir=/home/codeforge-tools/",
    ),
    # The default gate commands of Python projects (pytest, ruff check .) and the auto-agent's
    # workspace test (python -m pytest <file>): the system interpreter must have them.
    "pytest from the tool PATH": (
        "printf 'def test_x():\\n    assert True\\n' > test_cf_a.py && pytest -q test_cf_a.py",
        "1 passed",
    ),
    "python -m pytest": (
        "printf 'def test_y():\\n    assert True\\n' > test_cf_b.py && python -m pytest -q test_cf_b.py",
        "1 passed",
    ),
    "ruff from the tool PATH": ("printf 'x = 1\\n' > cf_ruff.py && ruff check cf_ruff.py", "All checks passed!"),
}

# The quality gate's own path (QualityGateExecutor.run_command, as the gates and the auto-agent's
# workspace test call it): command -> (whether it must pass, what its output must contain).
GATE_COMMANDS: dict[str, tuple[bool, str]] = {
    "pytest": (True, "1 passed"),
    "python -m pytest test_gate.py -v --tb=short": (True, "1 passed"),
    "ruff check .": (True, "All checks passed!"),
    # A failing test is a failed check because the test failed, not because pytest could not run.
    "pytest -q gate_fails.py": (False, "1 failed"),
}

# Toolchains of the battery image: (needed binary, command, expected output).
TOOLCHAINS: dict[str, tuple[str, str, str]] = {
    "node sees the CPUs and its user": (
        "node",
        "node -e 'const os=require(\"os\"); console.log(os.cpus().length>0, os.userInfo().username)'",
        f"true codeforge-t{UID_A}",
    ),
    "an npm package's shim runs from HOME": (
        "npm",
        "mkdir -p pkg/bin && printf '#!/usr/bin/env node\\nconsole.log(\"shim-ok\")\\n' > pkg/bin/cli.js && "
        'printf \'{"name":"cf-shim","version":"1.0.0","bin":{"cf-shim":"bin/cli.js"}}\' > pkg/package.json && '
        "npm pack ./pkg >/dev/null 2>&1 && npm install -g --offline --no-audit --no-fund ./cf-shim-1.0.0.tgz "
        '>/dev/null 2>&1 && ls "$HOME/.npm-global/lib/node_modules/cf-shim/bin" >/dev/null && cf-shim',
        "shim-ok",
    ),
    "go test builds and runs a test binary": (
        "go",
        "mkdir gt && cd gt && GOTOOLCHAIN=local GOPROXY=off go mod init example.com/gt >/dev/null 2>&1 && "
        "printf 'package gt\\nimport \"testing\"\\nfunc TestX(t *testing.T) {}\\n' > x_test.go && "
        "GOTOOLCHAIN=local GOPROXY=off go test ./... 2>&1",
        "ok",
    ),
    "java uses TMPDIR": (
        "java",
        "java -XshowSettings:properties -version 2>&1 | grep 'java.io.tmpdir'",
        f"/home/codeforge-tools/{UID_A}/tmp/",
    ),
}


async def _battery() -> int:
    config = _ready()
    workspace = make_tenant_dir(config.workspace_root, TENANT_A, UID_A)
    report: Report = {}
    problems: list[str] = []
    async with tool_tenant(TENANT_A, UID_A, workspace) as identity:
        assert identity is not None
        run_dir = f"{workspace}/battery-{identity.work_id}"  # each battery run starts in an empty directory
        await _sh(f"mkdir {run_dir}", workspace)
        _, out = await _sh("cat /proc/$$/status | grep -E '^(Uid|Gid|Groups|CapEff|NoNewPrivs|Umask)'", run_dir)
        report["credentials"] = out.splitlines()
        for name, (command, expected) in BATTERY.items():
            code, out = await _sh(command, run_dir)
            report[name] = f"exit {code}: {out[-300:]}"
            if expected not in out:
                problems.append(f"{name}: exit {code} {out[-300:]!r}")
        for name, (binary, command, expected) in TOOLCHAINS.items():
            if shutil.which(binary, path=config.tool_path) is None:
                report[name] = "absent"
                continue
            code, out = await _sh(command, run_dir)
            report[name] = f"exit {code}: {out[-300:]}"
            if code != 0 or expected not in out:
                problems.append(f"{name}: exit {code} {out[-300:]!r}")
        problems += await _gate_commands(f"{workspace}/gates-{identity.work_id}", report)
    report["toolchains"] = [name for name in TOOLCHAINS if report[name] != "absent"]
    return _emit(report, problems)


# The backend CLIs the image ships (workers/tests/test_backend_clis.py): the
# executors that find them, and what each prints for --version.
BACKEND_EXECUTORS = ("aider", "goose", "opencode")
BACKEND_VERSIONS: dict[str, tuple[str, str]] = {
    "aider": ("aider --version", "aider 0.86.2"),
    "claude": ("claude --version", "2.1.289 (Claude Code)"),
    "goose": ("goose --version", "1.29.0"),
    "opencode": ("opencode --version", "1.18.34"),
}


async def _backends() -> int:
    config = _ready()
    report: Report = {}
    problems: list[str] = []
    router = build_default_router()
    for name in BACKEND_EXECUTORS:
        executor = router.get(name)
        available = executor is not None and await executor.check_available()
        report[f"{name} executor finds its CLI"] = available
        if not available:
            problems.append(f"{name}: the executor does not find its CLI on the tool PATH")
    try:
        report["claude code cli"] = await resolve_cli(get_settings().claudecode_path)
    except ClaudeCodeCLIError as exc:
        problems.append(f"claude: {exc}")
    workspace = make_tenant_dir(config.workspace_root, TENANT_A, UID_A)
    async with tool_tenant(TENANT_A, UID_A, workspace):
        for name, (command, expected) in BACKEND_VERSIONS.items():
            code, out = await _sh(command, workspace)
            report[command] = f"exit {code}: {out[-300:]}"
            if code != 0 or expected not in out:
                problems.append(f"{name}: exit {code} {out[-300:]!r}")
    return _emit(report, problems)


async def _gate_commands(project: str, report: Report) -> list[str]:
    """GATE_COMMANDS through QualityGateExecutor in a Python project with a pytest configuration."""
    import structlog

    from codeforge.qualitygate import QualityGateExecutor

    await _sh(
        f"mkdir {project} && cd {project} && printf '[pytest]\\n' > pytest.ini && "
        "printf 'def test_ok():\\n    assert True\\n' > test_gate.py && "
        "printf 'def test_no():\\n    assert False\\n' > gate_fails.py",
        None,
    )
    executor, log, problems = QualityGateExecutor(timeout_seconds=120), structlog.get_logger(), []
    for command, (must_pass, expected) in GATE_COMMANDS.items():
        passed, output = await executor.run_command(command, project, log)
        report[f"gate: {command}"] = f"passed={passed}: {output.strip()[-300:]}"
        if passed is not must_pass or expected not in output:
            problems.append(f"gate command {command!r}: passed={passed}, expected {must_pass}: {output[-300:]!r}")
    return problems


# ---------------------------------------------------------------------------
# refused-call
# ---------------------------------------------------------------------------


async def _refused_call() -> int:
    config = _config()
    status = configure_tool_isolation(config)
    report: Report = {"ready": status.ready, "reason": status.reason}
    problems: list[str] = []
    if status.ready:
        problems.append("isolation is ready, expected it not to be")
    marker = f"{config.workspace_root}/cf-refused-marker"
    try:
        async with tool_tenant(TENANT_A, UID_A, f"{config.workspace_root}/{TENANT_A}/p1"):
            await _sh(f"touch {marker}", None)
        problems.append("a tool call ran")
    except ToolIsolationError as exc:
        report["tool call"] = f"ToolIsolationError: {exc}"
    try:
        with use_identity(None):
            await _sh(f"touch {marker}", None)
        problems.append("a tool call without an identity ran")
    except ToolIsolationError as exc:
        report["tool call without identity"] = f"ToolIsolationError: {exc}"
    report["marker exists"] = os.path.exists(marker)
    if os.path.exists(marker):
        problems.append("the marker file exists: a tool process ran")
    return _emit(report, problems)


# ---------------------------------------------------------------------------
# migration (E9)
# ---------------------------------------------------------------------------


def _legacy_layout(root: str, tenants: list[str]) -> None:
    """Tenant trees as the KI-71 worker left them: root and tenant directories 2775 in the workspace group."""
    for tenant in tenants:
        for path in (f"{root}/{tenant}", f"{root}/{tenant}/p1"):
            os.makedirs(path, exist_ok=True)
            os.chown(path, -1, WORKSPACE_GID)
            os.chmod(path, 0o2775)  # noqa: S103 - the KI-71 layout being reproduced
    os.chmod(root, 0o2775)  # noqa: S103 - the KI-71 layout being reproduced


def _seed_legacy(root: str) -> list[str]:
    _legacy_layout(root, [TENANT_A, TENANT_B])
    a, b = f"{root}/{TENANT_A}/p1", f"{root}/{TENANT_B}/p1"
    problems = []
    seeds = {
        b: "mkdir .git && echo '[core]' > .git/config && echo B-SECRET > secret.txt && echo hook > planted "
        "&& echo b > btool.txt && chmod 664 .git/config secret.txt planted btool.txt",
        a: "umask 002; mkdir src && echo tool > src/f && mkdir -m 0700 priv && echo p > priv/f && "
        f"ln {b}/secret.txt stolen.txt && ln {b}/.git/config bcfg && echo mine > mine && rm {b}/planted && "
        f"ln mine {b}/planted && setfacl -m u:{UID_A}:rw {b}/btool.txt && ln -s {b}/secret.txt sym && "
        "echo legacy > legacy.txt && chmod 664 legacy.txt && echo LEGACY-DONE",
    }
    for cwd, command in seeds.items():
        done = _as(LEGACY_UID, str(WORKSPACE_GID), command, cwd)
        if done.returncode:
            problems.append(f"seeding {cwd}: {done.stderr}")
    # A tenant directory the shared tool user replaced: never migrated, refused.
    done = _as(LEGACY_UID, str(WORKSPACE_GID), f"mkdir -m 2775 {root}/{TENANT_C} && mkdir {root}/{TENANT_C}/p1")
    if done.returncode:
        problems.append(f"seeding {TENANT_C}: {done.stderr}")
    return problems


async def _migration() -> int:
    config = _config()
    root = config.workspace_root
    problems = _seed_legacy(root)
    status = configure_tool_isolation(config)
    if not status.ready:
        return _emit({"reason": status.reason}, [f"not ready: {status.reason}"])
    a, b = f"{root}/{TENANT_A}/p1", f"{root}/{TENANT_B}/p1"
    b_config_before = os.stat(f"{b}/.git/config")
    checks_a = {
        "A reads its legacy files (incl. the 0700 directory)": ("cat src/f priv/f legacy.txt", "tool", True),
        "A appends to its old link to B's .git/config": ("echo A >> bcfg && echo wrote", "wrote", True),
        "A writes its file that was linked into B": ("echo A >> mine && echo wrote", "wrote", True),
        "A replaces a legacy file, then chmods it": (
            "cp legacy.txt l2 && mv l2 legacy.txt && chmod 750 legacy.txt && stat -c '%u %a' legacy.txt",
            f"{UID_A} 750",
            True,
        ),
        "A uses the ACL it planted on B's file": (f"cat {b}/btool.txt", "Permission denied", False),
        "A lists the workspace root": (f"ls {root}", "Permission denied", False),
        "A lists tenant B": (f"ls {root}/{TENANT_B}", "Permission denied", False),
        "A renames its project directory": (f"mv {a} {root}/{TENANT_A}/p9", "Permission denied", False),
    }
    report: Report = {}
    async with tool_tenant(TENANT_A, UID_A, a):
        _, out = await _sh("cp legacy.txt l3 && chmod 600 legacy.txt 2>&1; true", a)
        report["A chmods a 10002 file it did not replace"] = out
        if "Operation not permitted" not in out:
            problems.append(f"A could chmod a file of 10002: {out!r}")
        for name, (command, expected, succeeds) in checks_a.items():
            code, out = await _sh(command, a)
            report[name] = f"exit {code}: {out}"
            if expected not in out or (code == 0) != succeeds:
                problems.append(f"{name}: exit {code} {out!r}")
    b_config_after = os.stat(f"{b}/.git/config")
    report["B's .git/config"] = Path(f"{b}/.git/config").read_text()
    if Path(f"{b}/.git/config").read_text() != "[core]\n" or b_config_after.st_ino != b_config_before.st_ino:
        problems.append("B's .git/config changed through A's old hard link")
    report["B's planted"] = Path(f"{b}/planted").read_text()
    if Path(f"{b}/planted").read_text() != "mine\n":
        problems.append("B's planted file changed through A's old hard link")
    async with tool_tenant(TENANT_B, UID_B, b):
        code, out = await _sh("cat secret.txt planted btool.txt .git/config", b)
        report["B reads its own files"] = f"exit {code}: {out}"
        if code != 0:
            problems.append(f"B cannot read its own files: {out!r}")
    acl = posix_acl.get_acl(f"{b}/btool.txt", posix_acl.ACCESS) or []
    report["B's btool.txt ACL"] = [str(e) for e in acl]
    if any(e.tag == posix_acl.USER and e.id == UID_A for e in acl):
        problems.append("the ACL entry A planted on B's file survived B's migration")
    try:
        async with tool_tenant(TENANT_C, UID_C, f"{root}/{TENANT_C}/p1"):
            problems.append("tool work in a tenant directory of 10002 started")
    except ToolIsolationError as exc:
        report["tenant directory of 10002"] = str(exc)
        if "10002" not in str(exc):
            problems.append(f"the refusal does not name the owner: {exc}")
    return _emit(report, problems)


# ---------------------------------------------------------------------------
# scale-seed / scale-migrate
# ---------------------------------------------------------------------------

_SCALE_TENANT, _SCALE_UID = "tenant-scale", 20005
_SEED_SCRIPT = """
import os, sys
n = int(sys.argv[1]); base = sys.argv[2]
os.umask(0o002)
for d in range(0, n, 1000):
    path = f"{base}/d{d // 1000}"
    os.mkdir(path)
    for i in range(min(1000, n - d) - 1):
        os.close(os.open(f"{path}/f{i}", os.O_CREAT | os.O_WRONLY, 0o664))
"""


def scale_seed(entries: int) -> int:
    root = _config().workspace_root
    _legacy_layout(root, [_SCALE_TENANT])
    project = f"{root}/{_SCALE_TENANT}/p1"
    started = time.monotonic()
    done = subprocess.run(  # noqa: S603 - the check's own fixed command
        [_SETPRIV, f"--reuid={LEGACY_UID}", f"--regid={LEGACY_UID}", f"--groups={WORKSPACE_GID}", "--",
         sys.executable, "-I", "-c", _SEED_SCRIPT, str(entries), project],
        capture_output=True, text=True, check=False,
    )  # fmt: skip
    problems = [f"seeding: {done.stderr[-500:]}"] if done.returncode else []
    return _emit({"entries": entries, "seconds": round(time.monotonic() - started, 1)}, problems)


async def _scale_migrate() -> int:
    config = _ready()
    project = f"{config.workspace_root}/{_SCALE_TENANT}/p1"
    gaps: list[float] = []
    stop = asyncio.Event()

    async def tick() -> None:
        last = time.monotonic()
        while not stop.is_set():
            await asyncio.sleep(0.1)
            now = time.monotonic()
            gaps.append(now - last)
            last = now

    ticker = asyncio.create_task(tick())
    started = time.monotonic()
    async with tool_tenant(_SCALE_TENANT, _SCALE_UID, project):
        migrated = time.monotonic() - started
        code, out = await _sh("ls d0 | wc -l && touch d0/f0 && echo writable", project)
    stop.set()
    await ticker
    report: Report = {
        "migration seconds": round(migrated, 1),
        "event loop ticks": len(gaps),
        "longest tick gap seconds": round(max(gaps, default=0.0), 2),
        "tenant after migration": out,
    }
    problems = []
    if max(gaps, default=0.0) > 2.0:
        problems.append(f"the event loop stalled for {max(gaps):.1f} s during the migration (no heartbeats)")
    if code != 0 or "writable" not in out:
        problems.append(f"the tenant cannot use its migrated tree: {out!r}")
    return _emit(report, problems)


# ---------------------------------------------------------------------------
# deletion (D11)
# ---------------------------------------------------------------------------

_LOCKOUT = (
    "git init -q {ws} && "
    "mkdir -m 0700 -p {ws}/priv/inner && echo x > {ws}/priv/inner/f && "
    "python3 -c 'import tempfile; tempfile.mkdtemp(dir=\"{ws}\")' && "
    "echo s > {ws}/stripped && setfacl -x g:10010 {ws}/stripped && setfacl -m g::--- {ws}/stripped && "
    "mkdir {ws}/locked && echo l > {ws}/locked/f && chmod 0 {ws}/locked/f {ws}/locked && "
    "ln -s {other} {ws}/link-to-b && echo locked-out"
)


def _core_patch(workspace: str) -> str:
    """As the worker (uid 10001, the Go Core's): what writePatch makes in the workspace's .git."""
    patches = f"{workspace}/.git/codeforge/patches"
    os.mkdir(os.path.dirname(patches), 0o700)
    os.mkdir(patches, 0o700)
    patch = f"{patches}/run-1.patch"
    fd = os.open(patch, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        os.write(fd, b"diff --git a/f b/f\n")
    finally:
        os.close(fd)
    return patch


async def _tool_ls(identity: ToolIdentity, directory: str) -> tuple[int, str]:
    with use_identity(identity):
        return await _sh(f"ls {directory} 2>&1", None)


async def _deletion() -> int:
    config = _ready()
    root = config.workspace_root
    other = make_tenant_dir(root, TENANT_B, UID_B)
    Path(other, "keep.txt").write_text("B keeps this\n")
    make_tenant_dir(root, TENANT_A, UID_A)
    workspace = f"{root}/{TENANT_A}/p-del"
    os.mkdir(workspace, 0o770)
    report: Report = {}
    problems: list[str] = []
    async with tool_tenant(TENANT_A, UID_A, workspace):
        pass  # the tenant's tree is migrated and stamped, as before any tool work
    # The workspace as a write path of an identity without one skips the
    # per-call sharing pass; entering the identity without tool_tenant()
    # skips the end of work: the tree stays as a tool left it when its
    # worker crashed (Landlock still confines the tool to the workspace).
    identity = replace(accept_identity(TENANT_A, UID_A, workspace), workspace=None).with_paths(write=(workspace,))
    with use_identity(identity):
        code, out = await _sh(_LOCKOUT.format(ws=workspace, other=other), None)
    report["tool"] = f"exit {code}: {out}"
    if "locked-out" not in out:
        problems.append(f"the tool could not lock the worker out: {out!r}")
    try:
        shutil.rmtree(f"{workspace}/priv")
        problems.append("the worker could remove the locked-out tree itself (expected EACCES, E16)")
    except PermissionError as exc:
        report["the worker removes it itself"] = str(exc)
    # What the Go Core's writePatch leaves (same UID as the worker): .git/codeforge/patches 0700 and
    # the patch 0600. Under the default ACL the mask is ---: the tenant's removal cannot reach them.
    patch = _core_patch(workspace)
    code, out = await _tool_ls(identity, os.path.dirname(patch))
    report["the tenant lists the Go Core's patches"] = f"exit {code}: {out}"
    if code == 0:
        problems.append(f"the tenant can list the Go Core's private patch directory: {out!r}")
    await delete_workspace(TENANT_A, UID_A, workspace)
    report["workspace exists after the deletion"] = os.path.exists(workspace)
    if os.path.exists(workspace):
        problems.append("the workspace was not removed")
    report["B's file"] = Path(other, "keep.txt").read_text()
    if not Path(other, "keep.txt").exists():
        problems.append("the removal followed the symlink into tenant B's workspace")
    await delete_workspace(TENANT_A, UID_A, workspace)  # at least once: gone counts as removed
    return _emit(report, problems)


# ---------------------------------------------------------------------------
# ki71-walk / after-rollback
# ---------------------------------------------------------------------------


def _ki71_share(name: str, dir_fd: int | None) -> None:
    """What the KI-71 worker's walk did to each of its entries: the workspace group, g+rw(x), setgid dirs."""
    info = os.stat(name, dir_fd=dir_fd, follow_symlinks=False)
    if info.st_uid != WORKER_UID or not (stat.S_ISREG(info.st_mode) or stat.S_ISDIR(info.st_mode)):
        return
    fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=dir_fd)
    try:
        mode = stat.S_IMODE(info.st_mode) | stat.S_IRGRP | stat.S_IWGRP
        if stat.S_ISDIR(info.st_mode):
            mode |= stat.S_IXGRP | stat.S_ISGID
        os.fchown(fd, -1, WORKSPACE_GID)
        os.fchmod(fd, mode)
    finally:
        os.close(fd)


def ki71_walk() -> int:
    """The KI-71 worker's start (WORKSPACE_SHARING_VERSION "2") on the volume: an older image after a rollback."""
    root = _config().workspace_root
    for dirpath, dirnames, filenames, dir_fd in os.fwalk(root, follow_symlinks=False):
        for name in (*dirnames, *filenames):
            if dirpath == root and name.startswith(tool_migration.ROOT_STAMP):
                continue
            _ki71_share(name, dir_fd)
    tmp = f"{root}/{tool_migration.ROOT_STAMP}.old"
    Path(tmp).write_text("2\n")
    os.replace(tmp, f"{root}/{tool_migration.ROOT_STAMP}")
    info = os.stat(f"{root}/{tool_state.STATE_DIR}")
    return _emit({".codeforge": f"{stat.S_IMODE(info.st_mode):o} gid {info.st_gid}"}, [])


async def _after_rollback() -> int:
    config = _ready()
    root = config.workspace_root
    report: Report = {"rollback copies": sorted(p.name for p in Path(root).glob(".codeforge.rollback-*"))}
    problems = []
    if not report["rollback copies"]:
        problems.append("the older worker's run was not detected: no .codeforge.rollback-* directory")
    if stat.S_IMODE(os.stat(f"{root}/{tool_state.STATE_DIR}").st_mode) & 0o077:
        problems.append("the new state directory is open to the group")
    stamp = Path(root, tool_state.STATE_DIR, "tenants", TENANT_A)
    report["stamp of A before its next work item"] = stamp.exists()
    async with tool_tenant(TENANT_A, UID_A, f"{root}/{TENANT_A}/p1"):
        pass
    report["stamp of A after its next work item"] = stamp.exists()
    if not stamp.exists():
        problems.append("tenant A was not migrated again after the rollback")
    report["root stamp"] = Path(root, tool_migration.ROOT_STAMP).read_text().strip()
    return _emit(report, problems)


async def _prepare_a() -> int:
    config = _ready()
    workspace = make_tenant_dir(config.workspace_root, TENANT_A, UID_A)
    async with tool_tenant(TENANT_A, UID_A, workspace):
        code, out = await _sh("echo ok > made-by-a && cat made-by-a", workspace)
    return _emit({"A": out}, [] if code == 0 else [out])


# ---------------------------------------------------------------------------
# hold / contend (two workers on one volume)
# ---------------------------------------------------------------------------


async def _hold(seconds: float) -> int:
    config = _ready()
    workspace = make_tenant_dir(config.workspace_root, TENANT_A, UID_A)
    async with tool_tenant(TENANT_A, UID_A, workspace):
        sys.stdout.write("CF-HOLDING\n")
        sys.stdout.flush()
        await asyncio.sleep(seconds)
    return _emit({"held seconds": seconds}, [])


async def _contend(wait: float) -> int:
    config = _ready()
    root = config.workspace_root
    # The tenant's tree needs a migration (its stamp is gone): that needs the
    # exclusive lock, which the other worker's work item keeps shared.
    with contextlib.suppress(FileNotFoundError):
        os.unlink(f"{root}/{tool_state.STATE_DIR}/tenants/{TENANT_A}")
    tool_migration.LOCK_WAIT_SECONDS = wait
    started = time.monotonic()
    report: Report = {}
    try:
        async with tool_tenant(TENANT_A, UID_A, f"{root}/{TENANT_A}/p1"):
            report["entered"] = True
    except ToolIsolationError as exc:
        report["refused"] = str(exc)
    report["seconds"] = round(time.monotonic() - started, 1)
    return _emit(report, [])


# ---------------------------------------------------------------------------
# pin (E6)
# ---------------------------------------------------------------------------


async def _pin(seconds: float) -> int:
    config = _ready()
    workspace = make_tenant_dir(config.workspace_root, TENANT_A, UID_A)
    reads = max(1, int(seconds / 0.25))
    command = (
        f"ok=0; for i in $(seq {reads}); do cat /proc/$$/status >/dev/null 2>&1 && ok=$((ok+1)); "
        'sleep 0.25; done; echo "pinned-reads $ok"'
    )
    async with tool_tenant(TENANT_A, UID_A, workspace):
        sys.stdout.write("CF-PINNING\n")
        sys.stdout.flush()
        _, out = await _sh(command, workspace)
    ok = int(out.rsplit(" ", 1)[-1]) if out.startswith("pinned-reads") else -1
    problems = [] if ok == reads else [f"{ok} of {reads} reads of the tool's own /proc entry succeeded"]
    return _emit({"reads": reads, "ok": ok}, problems)


def main(argv: list[str]) -> int:
    command, args = argv[0], argv[1:]
    sync = {"image": image, "ki71-walk": ki71_walk}
    if command in sync:
        return sync[command]()
    if command == "scale-seed":
        return scale_seed(int(args[0]))
    runners = {
        "battery": _battery,
        "backends": _backends,
        "refused-call": _refused_call,
        "migration": _migration,
        "scale-migrate": _scale_migrate,
        "deletion": _deletion,
        "after-rollback": _after_rollback,
        "prepare-a": _prepare_a,
    }
    if command in runners:
        return asyncio.run(runners[command]())
    timed = {"hold": _hold, "contend": _contend, "pin": _pin}
    return asyncio.run(timed[command](float(args[0])))


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
