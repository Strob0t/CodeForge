"""Agent tool processes run as the tool user (KI-71).

Every process the worker starts for an agent goes through codeforge.tool_process.
With tool isolation required it runs as the unprivileged tool user (setpriv:
tool UID/GID, the workspace group, no capabilities, no_new_privs, umask 002);
when that is not possible the tool call fails and no process starts. With
isolation off (development, tests) tool processes start as before.
"""

from __future__ import annotations

import ast
import asyncio
import contextlib
import os
import stat
import subprocess
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from codeforge import tool_process
from codeforge.subprocess_env import tool_env
from codeforge.tool_process import (
    TOOL_UMASK,
    IsolationConfig,
    IsolationStatus,
    ToolIsolationError,
    check_tool_isolation,
    parse_isolation_mode,
    probe_problems,
    run_tool_process,
    share_with_tools,
    share_workspace_root,
    start_tool_process,
    start_tool_shell,
    worker_capability_problems,
)
from tests.test_subprocess_env import SPAWN_SITES, _FakeProc

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

WORKERS_DIR = Path(__file__).resolve().parents[1]
SOURCE_DIR = WORKERS_DIR / "codeforge"

LAUNCHER = "/usr/bin/setpriv"
CONFIG = IsolationConfig(mode="required", uid=10002, gid=10002, workspace_gid=10010, home="/home/codeforge-tool")
OFF = IsolationConfig(mode="off", uid=10002, gid=10002, workspace_gid=10010, home="/home/codeforge-tool")
READY = IsolationStatus(config=CONFIG, ready=True, launcher=LAUNCHER)
BROKEN = IsolationStatus(config=CONFIG, ready=False, reason="the worker lacks CAP_SETUID")


def _prefix(config: IsolationConfig) -> list[str]:
    return [
        LAUNCHER,
        f"--reuid={config.uid}",
        f"--regid={config.gid}",
        f"--groups={config.workspace_gid}",
        "--inh-caps=-all",
        "--ambient-caps=-all",
        "--no-new-privs",
        "--",
    ]


LAUNCH_PREFIX = _prefix(CONFIG)


@pytest.fixture
def isolation(monkeypatch: pytest.MonkeyPatch) -> Callable[[IsolationStatus], None]:
    """Install an isolation status without running the real check."""

    def install(status: IsolationStatus) -> None:
        monkeypatch.setattr(tool_process, "_status", status)

    return install


Spawn = tuple[tuple[object, ...], dict[str, object]]


@pytest.fixture
def spawns(monkeypatch: pytest.MonkeyPatch) -> list[Spawn]:
    """Record every asyncio and subprocess.run spawn (argv and keyword arguments)."""
    calls: list[Spawn] = []

    async def fake_exec(*args: object, **kwargs: object) -> _FakeProc:
        calls.append((args, kwargs))
        return _FakeProc()

    def fake_run(args: object, **kwargs: object) -> subprocess.CompletedProcess[str]:
        calls.append((tuple(args) if isinstance(args, list) else (args,), kwargs))
        return subprocess.CompletedProcess(args=args, returncode=0, stdout="", stderr="")

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake_exec)
    monkeypatch.setattr(asyncio, "create_subprocess_shell", fake_exec)
    monkeypatch.setattr(subprocess, "run", fake_run)
    return calls


# ---------------------------------------------------------------------------
# Settings
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("raw", "mode"),
    [
        ("required", "required"),
        ("REQUIRED", "required"),
        (" required\n", "required"),
        ("off", "off"),
        ("Off", "off"),
        ("", "off"),
        # Anything else fails closed.
        ("on", "required"),
        ("disabled", "required"),
        ("false", "required"),
        ("0", "required"),
    ],
)
def test_parse_isolation_mode(raw: str, mode: str) -> None:
    assert parse_isolation_mode(raw) == mode


def test_isolation_config_from_settings(monkeypatch: pytest.MonkeyPatch) -> None:
    from codeforge.config import WorkerSettings

    monkeypatch.setenv("CODEFORGE_TOOL_ISOLATION", "required")
    monkeypatch.setenv("CODEFORGE_TOOL_UID", "20002")
    monkeypatch.setenv("CODEFORGE_TOOL_GID", "20003")
    monkeypatch.setenv("CODEFORGE_WORKSPACE_GID", "20010")
    monkeypatch.setenv("CODEFORGE_TOOL_HOME", "/home/tool")
    config = IsolationConfig.from_settings(WorkerSettings())
    assert config == IsolationConfig(mode="required", uid=20002, gid=20003, workspace_gid=20010, home="/home/tool")


def test_isolation_defaults_to_off(monkeypatch: pytest.MonkeyPatch) -> None:
    from codeforge.config import WorkerSettings

    for name in ("CODEFORGE_TOOL_ISOLATION", "CODEFORGE_TOOL_UID", "CODEFORGE_TOOL_GID", "CODEFORGE_WORKSPACE_GID"):
        monkeypatch.delenv(name, raising=False)
    config = IsolationConfig.from_settings(WorkerSettings())
    assert config.mode == "off"
    assert (config.uid, config.gid, config.workspace_gid) == (10002, 10002, 10010)


# ---------------------------------------------------------------------------
# Spawning: off, required and ready, required and not ready
# ---------------------------------------------------------------------------


async def test_off_starts_the_command_as_before(
    isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]
) -> None:
    isolation(IsolationStatus(config=OFF, ready=True))
    await start_tool_process(
        "git", "status", env={"PATH": "/bin"}, cwd="/ws", stdout=asyncio.subprocess.PIPE, start_new_session=True
    )
    args, kwargs = spawns[0]
    assert args == ("git", "status")
    assert "umask" not in kwargs
    assert kwargs["env"] == {"PATH": "/bin"}
    assert kwargs["cwd"] == "/ws"
    assert kwargs["start_new_session"] is True


async def test_required_starts_the_command_as_the_tool_user(
    isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]
) -> None:
    isolation(READY)
    await start_tool_process("git", "status", env={"PATH": "/bin"}, cwd="/ws", stdout=asyncio.subprocess.PIPE)
    args, kwargs = spawns[0]
    assert list(args) == [*LAUNCH_PREFIX, "git", "status"]
    assert kwargs["umask"] == TOOL_UMASK == 0o002
    assert kwargs["env"] == {"PATH": "/bin"}
    assert kwargs["cwd"] == "/ws"


async def test_shell_commands_run_through_sh(isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]) -> None:
    isolation(READY)
    await start_tool_shell("pytest -q && echo ok", env={}, cwd="/ws")
    args, _ = spawns[0]
    assert list(args) == [*LAUNCH_PREFIX, "/bin/sh", "-c", "pytest -q && echo ok"]


def test_sync_run_as_the_tool_user(isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]) -> None:
    isolation(READY)
    run_tool_process(["git", "log"], env={"PATH": "/bin"}, cwd="/ws", timeout=5)
    args, kwargs = spawns[0]
    assert list(args) == [*LAUNCH_PREFIX, "git", "log"]
    assert kwargs["umask"] == 0o002
    assert kwargs["timeout"] == 5


def test_groups_cleared_without_a_workspace_group(
    isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]
) -> None:
    config = IsolationConfig(mode="required", uid=10002, gid=10002, workspace_gid=-1, home="/home/tool")
    isolation(IsolationStatus(config=config, ready=True, launcher=LAUNCHER))
    run_tool_process(["true"], env={})
    args, _ = spawns[0]
    assert "--clear-groups" in args
    assert not [a for a in args if str(a).startswith("--groups")]


async def test_required_without_isolation_fails_and_starts_nothing(
    isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]
) -> None:
    isolation(BROKEN)
    with pytest.raises(ToolIsolationError, match="CAP_SETUID") as exc_info:
        await start_tool_process("bash", "-c", "env", env={})
    with pytest.raises(ToolIsolationError):
        await start_tool_shell("env", env={})
    with pytest.raises(ToolIsolationError):
        run_tool_process(["env"], env={})
    assert spawns == []
    # Callers that report OSError as a failed tool call report this one too.
    assert isinstance(exc_info.value, OSError)
    assert "CODEFORGE_TOOL_ISOLATION=required" in str(exc_info.value)


# ---------------------------------------------------------------------------
# The tool's environment
# ---------------------------------------------------------------------------


def test_tool_env_uses_the_tool_users_home_when_isolated(
    isolation: Callable[[IsolationStatus], None], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("HOME", "/home/codeforge")
    monkeypatch.setenv("USER", "codeforge")
    isolation(READY)
    env = tool_env()
    assert env["HOME"] == "/home/codeforge-tool"
    assert env["USER"] == env["LOGNAME"] == "codeforge-tool"
    # An explicit value still wins (the Claude Code capability check's empty HOME).
    assert tool_env(extra={"HOME": "/tmp/x"})["HOME"] == "/tmp/x"


def test_tool_env_unchanged_when_off(
    isolation: Callable[[IsolationStatus], None], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("HOME", "/home/codeforge")
    monkeypatch.delenv("LOGNAME", raising=False)
    isolation(IsolationStatus(config=OFF, ready=True))
    env = tool_env()
    assert env["HOME"] == "/home/codeforge"
    assert "LOGNAME" not in env


@pytest.mark.parametrize(
    "name", ["NATS_URL_FILE", "DATABASE_URL_FILE", "LITELLM_MASTER_KEY_FILE", "CODEFORGE_INTERNAL_KEY_FILE"]
)
def test_tool_env_never_names_the_secret_files(name: str, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    secret = tmp_path / "secret"
    secret.write_text("nats://worker:pass@nats:4222\n")
    monkeypatch.delenv(name.removesuffix("_FILE"), raising=False)
    monkeypatch.setenv(name, str(secret))
    assert name not in tool_env()
    assert name not in tool_env(passthrough=(name,), passthrough_prefixes=("NATS_", "DATABASE_", "CODEFORGE_"))


# ---------------------------------------------------------------------------
# The isolation check
# ---------------------------------------------------------------------------

_STATUS_OK = """Name:\tcat
Umask:\t0002
Uid:\t10002\t10002\t10002\t10002
Gid:\t10002\t10002\t10002\t10002
Groups:\t10010
CapInh:\t0000000000000000
CapPrm:\t0000000000000000
CapEff:\t0000000000000000
CapBnd:\t00000000000000e0
CapAmb:\t0000000000000000
NoNewPrivs:\t1
"""


def test_probe_of_an_isolated_tool_process_passes() -> None:
    assert probe_problems(_STATUS_OK, CONFIG) == []


@pytest.mark.parametrize(
    ("line", "replacement", "problem"),
    [
        ("Uid:\t10002\t10002\t10002\t10002", "Uid:\t10001\t10001\t10001\t10001", "uid"),
        ("Uid:\t10002\t10002\t10002\t10002", "Uid:\t10002\t10001\t10002\t10002", "uid"),
        ("Gid:\t10002\t10002\t10002\t10002", "Gid:\t10001\t10001\t10001\t10001", "gid"),
        ("Groups:\t10010", "Groups:\t10001 10010", "groups"),
        ("Groups:\t10010", "Groups:\t", "groups"),
        ("CapEff:\t0000000000000000", "CapEff:\t00000000000000c0", "capabilities"),
        ("CapPrm:\t0000000000000000", "CapPrm:\t0000000000000020", "capabilities"),
        ("CapAmb:\t0000000000000000", "CapAmb:\t00000000000000c0", "capabilities"),
        ("CapInh:\t0000000000000000", "CapInh:\t0000000000000080", "capabilities"),
        ("NoNewPrivs:\t1", "NoNewPrivs:\t0", "no_new_privs"),
        ("Umask:\t0002", "Umask:\t0022", "umask"),
    ],
)
def test_probe_finds_what_is_wrong(line: str, replacement: str, problem: str) -> None:
    problems = probe_problems(_STATUS_OK.replace(line, replacement), CONFIG)
    assert problems, f"{problem} not detected"
    assert any(problem in p for p in problems), problems


def test_probe_reports_readable_secrets_and_environ() -> None:
    output = _STATUS_OK + "cf-readable /run/secrets/database-url\ncf-readable /proc/1/environ\n"
    problems = probe_problems(output, CONFIG)
    assert any("/run/secrets/database-url" in p for p in problems)
    assert any("/proc/1/environ" in p for p in problems)


def test_probe_without_status_output_fails() -> None:
    assert probe_problems("", CONFIG)


_WORKER_STATUS = "CapInh:\t00000000000000e0\nCapPrm:\t00000000000000e0\nCapEff:\t00000000000000e0\nCapAmb:\t{amb}\n"


@pytest.mark.parametrize(
    ("amb", "eff", "missing"),
    [
        ("00000000000000e0", "00000000000000e0", []),
        ("0000000000000000", "00000000000000e0", ["CAP_SETGID", "CAP_SETUID"]),
        ("0000000000000080", "00000000000000e0", ["CAP_SETGID"]),
        ("00000000000000c0", "00000000000000c0", ["CAP_KILL"]),
        ("000001ffffffffff", "000001ffffffffff", []),
    ],
)
def test_worker_capabilities(amb: str, eff: str, missing: list[str]) -> None:
    status = _WORKER_STATUS.format(amb=amb).replace("CapEff:\t00000000000000e0", f"CapEff:\t{eff}")
    problems = worker_capability_problems(status)
    for cap in missing:
        assert any(cap in p for p in problems), (cap, problems)
    if not missing:
        assert problems == []


def test_check_off_is_ready_without_a_probe(monkeypatch: pytest.MonkeyPatch) -> None:
    def no_probe(*_args: object, **_kwargs: object) -> None:
        raise AssertionError("off must not probe")

    monkeypatch.setattr(tool_process, "_run_probe", no_probe)
    status = check_tool_isolation(OFF)
    assert status.ready
    assert status.launcher == ""


def test_check_required_without_launcher_is_not_ready(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process.shutil, "which", lambda *_a, **_k: None)
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "setpriv" in status.reason


def test_check_required_without_capabilities_is_not_ready(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process.shutil, "which", lambda *_a, **_k: LAUNCHER)
    monkeypatch.setattr(tool_process, "_is_root", lambda: False)
    monkeypatch.setattr(tool_process, "_own_status", lambda: _WORKER_STATUS.format(amb="0000000000000000"))
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "CAP_SETUID" in status.reason


def test_check_required_with_the_worker_uid_as_tool_uid_is_not_ready(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process.shutil, "which", lambda *_a, **_k: LAUNCHER)
    monkeypatch.setattr(tool_process, "_is_root", lambda: False)
    monkeypatch.setattr(tool_process, "_own_status", lambda: _WORKER_STATUS.format(amb="00000000000000e0"))
    for config in (
        IsolationConfig(mode="required", uid=os.getuid(), gid=10002, workspace_gid=10010, home="/h"),
        IsolationConfig(mode="required", uid=10002, gid=os.getgid(), workspace_gid=10010, home="/h"),
        IsolationConfig(mode="required", uid=0, gid=10002, workspace_gid=10010, home="/h"),
    ):
        status = check_tool_isolation(config)
        assert not status.ready, config
        assert "differ" in status.reason


def test_check_required_runs_the_probe(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process.shutil, "which", lambda *_a, **_k: LAUNCHER)
    monkeypatch.setattr(tool_process, "_is_root", lambda: False)
    monkeypatch.setattr(tool_process, "_own_status", lambda: _WORKER_STATUS.format(amb="00000000000000e0"))
    probes: list[list[str]] = []

    def fake_probe(argv: list[str], _timeout: float) -> str:
        probes.append(argv)
        return _STATUS_OK

    monkeypatch.setattr(tool_process, "_run_probe", fake_probe)
    status = check_tool_isolation(CONFIG)
    assert status.ready, status.reason
    assert probes[0][: len(LAUNCH_PREFIX)] == LAUNCH_PREFIX
    assert f"/proc/{os.getpid()}/environ" in probes[0]

    monkeypatch.setattr(tool_process, "_run_probe", lambda *_a: _STATUS_OK.replace("NoNewPrivs:\t1", "NoNewPrivs:\t0"))
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "no_new_privs" in status.reason


def test_check_required_probe_failure_is_not_ready(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process.shutil, "which", lambda *_a, **_k: LAUNCHER)
    monkeypatch.setattr(tool_process, "_is_root", lambda: False)
    monkeypatch.setattr(tool_process, "_own_status", lambda: _WORKER_STATUS.format(amb="00000000000000e0"))

    def failing_probe(_argv: list[str], _timeout: float) -> str:
        raise OSError("setpriv: setresuid failed: Operation not permitted")

    monkeypatch.setattr(tool_process, "_run_probe", failing_probe)
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "setresuid" in status.reason


# ---------------------------------------------------------------------------
# Sharing files and workspaces with the tool user
# ---------------------------------------------------------------------------


def _own_group_config() -> IsolationConfig:
    # The test process may change a file's group only to a group it is in.
    return IsolationConfig(mode="required", uid=10002, gid=10002, workspace_gid=os.getgid(), home="/h")


def test_share_with_tools_is_a_no_op_when_off(isolation: Callable[[IsolationStatus], None], tmp_path: Path) -> None:
    isolation(IsolationStatus(config=OFF, ready=True))
    path = tmp_path / "f"
    path.write_text("x")
    path.chmod(0o600)
    share_with_tools(str(path), writable=True)
    assert stat.S_IMODE(path.stat().st_mode) == 0o600


@pytest.mark.parametrize(
    ("is_dir", "writable", "mode"),
    [(True, True, 0o2770), (True, False, 0o2750), (False, True, 0o660), (False, False, 0o640)],
)
def test_share_with_tools(
    is_dir: bool, writable: bool, mode: int, isolation: Callable[[IsolationStatus], None], tmp_path: Path
) -> None:
    isolation(IsolationStatus(config=_own_group_config(), ready=True, launcher=LAUNCHER))
    path = tmp_path / "x"
    if is_dir:
        path.mkdir(mode=0o700)
    else:
        path.write_text("x")
        path.chmod(0o600)
    share_with_tools(str(path), writable=writable)
    info = path.stat()
    assert stat.S_IMODE(info.st_mode) == mode
    assert info.st_gid == os.getgid()


def test_share_workspace_root(tmp_path: Path) -> None:
    root = tmp_path / "workspaces"
    (root / "tenant" / "project" / "src").mkdir(parents=True)
    for d in (root, root / "tenant", root / "tenant" / "project", root / "tenant" / "project" / "src"):
        d.chmod(0o755)
    source = root / "tenant" / "project" / "src" / "main.py"
    source.write_text("print(1)\n")
    source.chmod(0o644)
    script = root / "tenant" / "project" / "run.sh"
    script.write_text("#!/bin/sh\n")
    script.chmod(0o755)
    private = root / "tenant" / "project" / "key"
    private.write_text("x")
    private.chmod(0o600)
    outside = tmp_path / "outside"
    outside.write_text("x")
    outside.chmod(0o600)
    (root / "tenant" / "project" / "link").symlink_to(outside)

    changed = share_workspace_root(str(root), os.getgid())

    assert changed == 7
    for d in (root, root / "tenant", root / "tenant" / "project", root / "tenant" / "project" / "src"):
        assert stat.S_IMODE(d.stat().st_mode) == 0o2775, d
    assert stat.S_IMODE(source.stat().st_mode) == 0o664
    assert stat.S_IMODE(script.stat().st_mode) == 0o775
    assert stat.S_IMODE(private.stat().st_mode) == 0o660
    # Symlinks and their targets are left alone.
    assert stat.S_IMODE(outside.stat().st_mode) == 0o600
    # Once the root is shared, nothing is walked again.
    assert share_workspace_root(str(root), os.getgid()) == 0


def test_share_workspace_root_missing_root_is_no_error(tmp_path: Path) -> None:
    assert share_workspace_root(str(tmp_path / "missing"), os.getgid()) == 0


# ---------------------------------------------------------------------------
# Every spawn goes through codeforge.tool_process
# ---------------------------------------------------------------------------

_SPAWN_ATTRIBUTES = {
    "asyncio": {"create_subprocess_exec", "create_subprocess_shell"},
    "subprocess": {"run", "Popen", "call", "check_call", "check_output", "getoutput", "getstatusoutput"},
    "os": {
        "system",
        "popen",
        "fork",
        "forkpty",
        "posix_spawn",
        "posix_spawnp",
        "execl",
        "execle",
        "execlp",
        "execlpe",
        "execv",
        "execve",
        "execvp",
        "execvpe",
        "spawnl",
        "spawnle",
        "spawnlp",
        "spawnlpe",
        "spawnv",
        "spawnve",
        "spawnvp",
        "spawnvpe",
    },
    "pty": {"spawn", "fork"},
}
# Event loop methods that start processes, whatever the loop object is called.
_LOOP_METHODS = {"subprocess_exec", "subprocess_shell"}


def _spawn_calls(path: Path, base: Path = WORKERS_DIR) -> list[str]:
    tree = ast.parse(path.read_text(), filename=str(path))
    found: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Attribute):
            owner = node.value.id if isinstance(node.value, ast.Name) else ""
            if node.attr in _SPAWN_ATTRIBUTES.get(owner, set()) or node.attr in _LOOP_METHODS:
                found.append(f"{path.relative_to(base)}:{node.lineno} {owner}.{node.attr}")
        elif isinstance(node, ast.ImportFrom) and node.module in _SPAWN_ATTRIBUTES:
            found.extend(
                f"{path.relative_to(base)}:{node.lineno} from {node.module} import {alias.name}"
                for alias in node.names
                if alias.name in _SPAWN_ATTRIBUTES[node.module]
            )
    return found


def test_only_tool_process_starts_processes() -> None:
    """A new subprocess call outside codeforge.tool_process would run as the worker user."""
    offenders: list[str] = []
    for path in sorted(SOURCE_DIR.rglob("*.py")):
        if path == SOURCE_DIR / "tool_process.py":
            continue
        offenders.extend(_spawn_calls(path))
    assert offenders == [], "start processes through codeforge.tool_process:\n" + "\n".join(offenders)


def test_the_spawn_scan_finds_spawns(tmp_path: Path) -> None:
    sample = tmp_path / "sample.py"
    sample.write_text(
        "import asyncio, subprocess, os\n"
        "from subprocess import Popen\n"
        "async def f(loop):\n"
        "    await asyncio.create_subprocess_exec('x')\n"
        "    subprocess.run(['x'])\n"
        "    os.system('x')\n"
        "    await loop.subprocess_exec(object, 'x')\n"
    )
    found = _spawn_calls(sample, tmp_path)
    assert len(found) == 5, found
    assert _spawn_calls(SOURCE_DIR / "tool_process.py"), "the helper itself must be found by the scan"


# Every agent spawn site, isolated: it starts the command through the launcher.
async def _bash(ws: Path) -> None:
    from codeforge.tools.bash import BashTool

    await BashTool().execute({"command": "env"}, str(ws))


def _site(name: str) -> Callable[[Path], Awaitable[None]]:
    return _bash if name == "bash" else dict(SPAWN_SITES)[name]


_SITES = [
    "search_files",
    "quality_gate",
    "git_in_workspace",
    "cli_backend",
    "claude_code",
    "claude_code_cli_check",
    "benchmark_test_command",
    "functional_test",
    "synthetic_benchmark_git",
    "run_subprocess_helper",
]


@pytest.mark.parametrize("site", ["bash", *_SITES])
async def test_every_spawn_site_runs_as_the_tool_user(
    site: str,
    isolation: Callable[[IsolationStatus], None],
    spawns: list[Spawn],
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PATH", "/usr/bin:/bin")
    # The workspace group is the test's own: sharing a file with the tool user
    # changes its group, which a non-root test may do only to its own groups.
    config = _own_group_config()
    isolation(IsolationStatus(config=config, ready=True, launcher=LAUNCHER))
    await _site(site)(tmp_path)
    assert spawns, f"{site} started no process"
    for args, kwargs in spawns:
        assert list(args[: len(_prefix(config))]) == _prefix(config), f"{site}: {args}"
        assert kwargs.get("umask") == 0o002, site


@pytest.mark.parametrize("site", ["bash", *_SITES])
async def test_every_spawn_site_fails_closed(
    site: str,
    isolation: Callable[[IsolationStatus], None],
    spawns: list[Spawn],
    tmp_path: Path,
) -> None:
    isolation(BROKEN)
    with contextlib.suppress(ToolIsolationError):
        await _site(site)(tmp_path)
    assert spawns == [], f"{site} started a process without isolation"


# ---------------------------------------------------------------------------
# Worker startup
# ---------------------------------------------------------------------------


def test_lock_secrets_dir(tmp_path: Path) -> None:
    from codeforge.secrets import lock_secrets_dir

    secrets_dir = tmp_path / "secrets"
    secrets_dir.mkdir(mode=0o700)
    (secrets_dir / "database-url").write_text("x")
    assert lock_secrets_dir(secrets_dir)
    assert stat.S_IMODE(secrets_dir.stat().st_mode) == 0
    assert not lock_secrets_dir(tmp_path / "missing")
    secrets_dir.chmod(0o700)


def test_lock_secrets_dir_leaves_other_users_directories(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    from codeforge import secrets

    secrets_dir = tmp_path / "secrets"
    secrets_dir.mkdir(mode=0o755)
    monkeypatch.setattr(secrets.os, "getuid", lambda: secrets_dir.stat().st_uid + 1)
    assert not secrets.lock_secrets_dir(secrets_dir)
    assert stat.S_IMODE(secrets_dir.stat().st_mode) == 0o755


@pytest.mark.parametrize(
    ("mode", "ready", "locks", "shares", "umask"),
    [
        ("off", True, False, False, None),
        ("required", True, True, True, 0o002),
        ("required", False, True, False, 0o002),
    ],
)
def test_setup_tool_isolation(
    mode: str,
    ready: bool,
    locks: bool,
    shares: bool,
    umask: int | None,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    import codeforge.consumer as consumer_module
    from codeforge.config import WorkerSettings

    monkeypatch.setenv("CODEFORGE_TOOL_ISOLATION", mode)
    monkeypatch.setenv("CODEFORGE_WORKSPACE_ROOT", "/data/workspaces")
    calls: list[str] = []
    umasks: list[int] = []

    def fake_configure(config: IsolationConfig) -> IsolationStatus:
        calls.append("check")
        return IsolationStatus(config=config, ready=ready, reason="" if ready else "no CAP_SETUID", launcher=LAUNCHER)

    monkeypatch.setattr(consumer_module, "configure_tool_isolation", fake_configure)
    monkeypatch.setattr(consumer_module, "lock_secrets_dir", lambda: calls.append("lock") or True)
    monkeypatch.setattr(
        consumer_module, "share_workspace_root", lambda root, gid: calls.append(f"share {root} {gid}") or 3
    )
    monkeypatch.setattr(consumer_module.os, "umask", lambda value: umasks.append(value) or 0o022)

    status = consumer_module.setup_tool_isolation(WorkerSettings())

    assert status.ready is ready
    assert calls[0] == "check"
    assert ("lock" in calls) is locks
    assert ("share /data/workspaces 10010" in calls) is shares
    assert umasks == ([] if umask is None else [umask])
