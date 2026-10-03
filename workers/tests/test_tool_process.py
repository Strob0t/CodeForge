"""Agent tool processes run as their tenant's tool user (KI-71, KI-96).

Every process the worker starts for an agent goes through codeforge.tool_process.
With tool isolation required it runs as the current tool identity (setpriv:
the tenant's tool UID and GID, no supplementary group, no capabilities,
no_new_privs, umask 007, then the launch helper with the environment from a
memfd, never on argv); without an identity, or when isolation is not
possible, the tool call fails and no process starts. With isolation off
(development, tests) tool processes start as before.
"""

from __future__ import annotations

import ast
import asyncio
import contextlib
import errno
import json
import os
import random
import signal
import stat
import string
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from codeforge import landlock, posix_acl, tool_identity, tool_process
from codeforge.subprocess_env import tool_env
from codeforge.tool_identity import ToolIdentity, current_identity
from codeforge.tool_process import (
    TOOL_EXEC,
    TOOL_UMASK,
    TOOL_WALK,
    IsolationConfig,
    IsolationStatus,
    Launch,
    ToolIsolationError,
    check_tool_isolation,
    grant_tool_access,
    parse_isolation_mode,
    probe_problems,
    run_tool_process,
    start_tool_process,
    start_tool_shell,
    worker_capability_problems,
)
from tests.test_subprocess_env import SPAWN_SITES, _FakeProc

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable, Iterator, Mapping, Sequence

WORKERS_DIR = Path(__file__).resolve().parents[1]
SOURCE_DIR = WORKERS_DIR / "codeforge"

LAUNCHER = "/usr/bin/setpriv"
INTERPRETER = "/usr/bin/python3"
LAUNCHER_ENV = {"PATH": "/usr/local/bin:/usr/bin:/bin"}
TOOL_PATH = "/usr/local/bin:/usr/bin:/bin"
CONFIG = IsolationConfig(
    mode="required", workspace_root="/data/workspaces", home_base="/home/codeforge-tools", tool_path=TOOL_PATH
)
OFF = IsolationConfig(mode="off")
READY = IsolationStatus(config=CONFIG, ready=True, launcher=LAUNCHER, interpreter=INTERPRETER)
BROKEN = IsolationStatus(config=CONFIG, ready=False, reason="the worker lacks CAP_SETUID")
# Workspaces outside the root (tests' temporary directories) need no tenant directory check.
IDENT = ToolIdentity(
    tenant_id="tenant-a", uid=20007, home="/home/codeforge-tools/20007", work_id="tok1", workspace="/ws"
)


def _prefix(uid: int = 20007) -> list[str]:
    return [
        LAUNCHER,
        f"--reuid={uid}",
        f"--regid={uid}",
        "--clear-groups",
        "--inh-caps=-all",
        "--ambient-caps=-all",
        "--no-new-privs",
        "--",
    ]


def _helper_prefix(uid: int = 20007) -> list[str]:
    """setpriv, then the launch helper with the base interpreter."""
    return [*_prefix(uid), INTERPRETER, "-I", "-S", TOOL_EXEC]


def _command(args: tuple[object, ...], uid: int = 20007) -> list[object]:
    """The command a launch runs: what follows the helper and its spec descriptor."""
    prefix = _helper_prefix(uid)
    assert list(args[: len(prefix)]) == prefix, args
    assert str(args[len(prefix)]).isdigit(), args
    return list(args[len(prefix) + 1 :])


def read_spec(kwargs: Mapping[str, object]) -> dict[str, object] | None:
    """The launch spec on the memfd a spawn passes (read before the launcher closes it)."""
    fds = kwargs.get("pass_fds") or ()
    if not fds:
        return None
    fd = fds[0]  # type: ignore[index]
    os.lseek(fd, 0, os.SEEK_SET)
    data = b""
    while chunk := os.read(fd, 65536):
        data += chunk
    os.lseek(fd, 0, os.SEEK_SET)
    return json.loads(data)


@pytest.fixture
def isolation(monkeypatch: pytest.MonkeyPatch) -> Callable[[IsolationStatus], None]:
    """Install an isolation status without running the real check."""

    def install(status: IsolationStatus) -> None:
        monkeypatch.setattr(tool_process, "_status", status)

    return install


@pytest.fixture
def identity() -> Iterator[Callable[[ToolIdentity | None], None]]:
    """Set the current tool identity for the test.

    An async test sets it in its own task's context, which ends with the
    test; a sync test sets it in the main context, which is cleared here.
    """

    def install(ident: ToolIdentity | None) -> None:
        current_identity.set(ident)

    yield install
    current_identity.set(None)


Spawn = tuple[tuple[object, ...], dict[str, object]]


class _FakePopen:
    """A helper process that ends at once with no output; it has no real PID (pid -1: never signalled)."""

    def __init__(self, calls: list[Spawn], args: object, **kwargs: object) -> None:
        calls.append((tuple(args) if isinstance(args, list) else (args,), {**kwargs, "spec": read_spec(kwargs)}))
        self.args, self.pid, self.returncode = args, -1, 0

    def communicate(self, input: str | None = None, timeout: float | None = None) -> tuple[str, str]:  # noqa: A002
        return "", ""

    def wait(self, timeout: float | None = None) -> int:
        return 0

    def __enter__(self) -> _FakePopen:
        return self

    def __exit__(self, *_exc: object) -> None:
        return None


@pytest.fixture
def spawns(monkeypatch: pytest.MonkeyPatch) -> list[Spawn]:
    """Record every asyncio, subprocess.run and subprocess.Popen spawn (argv and keyword arguments,
    plus the launch spec it passes as ``spec``)."""
    calls: list[Spawn] = []

    async def fake_exec(*args: object, **kwargs: object) -> _FakeProc:
        calls.append((args, {**kwargs, "spec": read_spec(kwargs)}))
        return _FakeProc()

    def fake_run(args: object, **kwargs: object) -> subprocess.CompletedProcess[str]:
        calls.append((tuple(args) if isinstance(args, list) else (args,), {**kwargs, "spec": read_spec(kwargs)}))
        return subprocess.CompletedProcess(args=args, returncode=0, stdout="", stderr="")

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake_exec)
    monkeypatch.setattr(asyncio, "create_subprocess_shell", fake_exec)
    monkeypatch.setattr(subprocess, "run", fake_run)
    monkeypatch.setattr(subprocess, "Popen", lambda args, **kwargs: _FakePopen(calls, args, **kwargs))
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
    monkeypatch.setenv("CODEFORGE_WORKSPACE_ROOT", "/data/workspaces/")
    monkeypatch.setenv("CODEFORGE_TOOL_HOME_BASE", "/home/tools")
    monkeypatch.setenv("CODEFORGE_TOOL_PATH", "/opt/bin:/usr/bin")
    monkeypatch.setenv("CODEFORGE_WORKSPACE_GID", "20010")
    for name in ("CODEFORGE_TOOL_LANDLOCK", "CODEFORGE_TOOL_LANDLOCK_MIN_ABI", "CODEFORGE_TOOL_READ_PATHS"):
        monkeypatch.delenv(name, raising=False)
    monkeypatch.setenv("APP_ENV", "development")
    config = IsolationConfig.from_settings(WorkerSettings())
    assert config == IsolationConfig(
        mode="required",
        workspace_root="/data/workspaces",
        home_base="/home/tools",
        tool_path="/opt/bin:/usr/bin",
        workspace_gid=20010,
        landlock="required",  # follows isolation
        landlock_min_abi=2,
    )

    monkeypatch.setenv("CODEFORGE_TOOL_LANDLOCK", "off")
    monkeypatch.setenv("CODEFORGE_TOOL_LANDLOCK_MIN_ABI", "6")
    monkeypatch.setenv("CODEFORGE_TOOL_READ_PATHS", "/opt:/app/.venv")
    monkeypatch.setenv("APP_ENV", "production")
    config = IsolationConfig.from_settings(WorkerSettings())
    assert (config.landlock, config.landlock_min_abi, config.read_paths, config.production) == (
        "off",
        6,
        ("/opt", "/app/.venv"),
        True,
    )
    assert not config.confined


def test_isolation_defaults_to_off(monkeypatch: pytest.MonkeyPatch) -> None:
    from codeforge.config import WorkerSettings

    for name in ("CODEFORGE_TOOL_ISOLATION", "CODEFORGE_TOOL_HOME_BASE", "CODEFORGE_TOOL_PATH"):
        monkeypatch.delenv(name, raising=False)
    config = IsolationConfig.from_settings(WorkerSettings())
    assert config.mode == "off"
    assert config.home_base == "/home/codeforge-tools"
    assert config.tool_path == "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"


# ---------------------------------------------------------------------------
# Spawning: off, required with an identity, without one, not ready
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


async def test_required_starts_the_command_as_the_tenants_tool_user(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    await start_tool_process("git", "status", env={"PATH": "/bin"}, cwd="/ws", stdout=asyncio.subprocess.PIPE)
    args, kwargs = spawns[0]
    assert _command(args) == ["git", "status"]
    assert kwargs["umask"] == TOOL_UMASK == 0o007
    # setpriv holds the worker's capabilities: it gets a fixed environment; the
    # helper gives the command its own after the switch, from the spec.
    assert kwargs["env"] == LAUNCHER_ENV
    # The worker never changes into a directory a tool can write: the helper does.
    assert kwargs["cwd"] == "/"
    spec = dict(kwargs["spec"])  # type: ignore[call-overload]
    confinement = spec.pop("landlock")
    assert spec == {
        "uid": 20007,
        "gid": 20007,
        "groups": [],
        "umask": 0o007,
        "env": {"PATH": "/bin"},
        "prepare": ["tmp/tok1"],
        "home": "/home/codeforge-tools/20007",
        "cwd": "/ws",
    }
    # Confined by Landlock to its workspace, its HOME and the system (KI-96 D6).
    assert confinement == landlock.spec_value("required", IDENT, min_abi=2, interpreter_prefix=sys.base_prefix)


async def test_landlock_off_is_written_as_off(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    config = IsolationConfig(mode="required", workspace_root="/data/workspaces", landlock="off")
    isolation(IsolationStatus(config=config, ready=True, launcher=LAUNCHER, interpreter=INTERPRETER))
    identity(IDENT)
    await start_tool_process("true", env={})
    assert spawns[0][1]["spec"]["landlock"] == "off"  # type: ignore[index]


async def test_operator_read_paths_and_the_walker_files_are_readable(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    config = IsolationConfig(mode="required", workspace_root="/data/workspaces", read_paths=("/opt",))
    isolation(IsolationStatus(config=config, ready=True, launcher=LAUNCHER, interpreter=INTERPRETER))
    identity(IDENT)
    proc = await start_tool_process("true", env={}, cwd="/ws")
    await proc.wait()
    command, walker = (kwargs["spec"]["landlock"]["rules"] for _a, kwargs in spawns)  # type: ignore[index]
    assert ["/opt", ["execute", "read-file", "read-dir"], True] in command
    walk_files = {rule[0] for rule in walker if rule[1] == ["read-file"] and rule[2]}
    assert walk_files == {TOOL_WALK, str(Path(TOOL_WALK).with_name("posix_acl.py"))}


async def test_an_explicit_identity_overrides_the_current_one(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    system = ToolIdentity(tenant_id="", uid=19999, home="/home/codeforge-tools/19999", work_id="sys")
    await start_tool_process("true", env={}, identity=system)
    run_tool_process(["true"], env={}, identity=system)
    for args, kwargs in spawns:
        assert _command(args, 19999) == ["true"]
        assert kwargs["spec"]["uid"] == 19999  # type: ignore[index]


async def test_required_without_an_identity_starts_nothing(
    isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]
) -> None:
    isolation(READY)
    with pytest.raises(ToolIsolationError, match="without a tenant"):
        await start_tool_process("true", env={})
    with pytest.raises(ToolIsolationError):
        run_tool_process(["true"], env={})
    assert spawns == []


@pytest.mark.parametrize("cwd", ["/", "/data/workspaces/other/p", "/ws-other", "/home/codeforge-tools/20008"])
async def test_a_working_directory_outside_the_identitys_areas_is_refused(
    cwd: str,
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    with pytest.raises(ToolIsolationError, match="outside its workspace"):
        await start_tool_process("true", env={}, cwd=cwd)
    assert spawns == []


async def test_a_working_directory_inside_the_workspace_or_home_is_accepted(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    for cwd in ("/ws", "/ws/sub/dir", "/ws/./x/..", "/home/codeforge-tools/20007/tmp/tok1"):
        await start_tool_process("true", env={}, cwd=cwd)
    assert [kwargs["spec"]["cwd"] for _a, kwargs in spawns if _command(_a) == ["true"]] == [  # type: ignore[index]
        "/ws",
        "/ws/sub/dir",
        "/ws",
        "/home/codeforge-tools/20007/tmp/tok1",
    ]


async def test_the_tenant_directory_is_verified_at_every_launch(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    isolation(READY)
    identity(IDENT.with_workspace("/data/workspaces/tenant-a/p1"))
    checked: list[tuple[str, str, int]] = []

    def refuse(root: str, tenant_id: str, uid: int) -> None:
        checked.append((root, tenant_id, uid))
        raise ToolIsolationError("the tenant directory of tenant tenant-a belongs to uid 10002")

    monkeypatch.setattr(tool_process, "verify_tenant_dir", refuse)
    with pytest.raises(ToolIsolationError, match="uid 10002"):
        await start_tool_process("true", env={}, cwd="/data/workspaces/tenant-a/p1")
    assert checked == [("/data/workspaces", "tenant-a", 20007)]
    assert spawns == []


async def test_the_launcher_never_gets_the_tools_environment(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    env = {"LD_PRELOAD": "/data/workspaces/t/p/evil.so", "GCONV_PATH": "/x", "PATH": "/bin", "": "x", "A=B": "y"}
    await start_tool_process("true", env=env)
    run_tool_process(["true"], env=env)
    commands = [(args, kwargs) for args, kwargs in spawns if _command(args)[3:4] != [TOOL_WALK]]
    assert len(commands) == 2
    for args, kwargs in spawns:
        assert kwargs["env"] == LAUNCHER_ENV
        assert not [a for a in args if "evil.so" in str(a) or "PATH=" in str(a)]
    for args, kwargs in commands:
        assert _command(args) == ["true"]
        # The command still gets its environment; invalid names are dropped.
        assert kwargs["spec"]["env"] == {  # type: ignore[index]
            "LD_PRELOAD": "/data/workspaces/t/p/evil.so",
            "GCONV_PATH": "/x",
            "PATH": "/bin",
        }


def _random_text(rng: random.Random) -> str:
    alphabet = string.ascii_letters + string.digits + string.punctuation + " \té中"
    return "".join(rng.choice(alphabet) for _ in range(rng.randint(8, 40)))


async def test_no_environment_value_is_ever_an_argument(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    """Property: whatever the environment, none of its values (secrets) is in argv (KI-96, E8)."""
    isolation(READY)
    identity(IDENT)
    rng = random.Random(96)  # noqa: S311 - test data, not cryptography
    for _ in range(50):
        env = {f"V{index}_{rng.randint(0, 99)}": _random_text(rng) for index in range(rng.randint(1, 12))}
        spawns.clear()
        await start_tool_process("cmd", env=env, cwd="/ws")
        run_tool_process(["cmd"], env=env, cwd="/ws")
        commands = 0
        for args, kwargs in spawns:
            joined = "\0".join(str(a) for a in args)
            assert not [value for value in env.values() if value in joined], args
            if _command(args) == ["cmd"]:  # not the sharing pass after it
                commands += 1
                assert kwargs["spec"]["env"] == env  # type: ignore[index]
        assert commands == 2


async def test_the_launch_spec_is_closed_after_the_spawn(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    await start_tool_process("true", env={})
    run_tool_process(["true"], env={})
    for _args, kwargs in spawns:
        (fd,) = kwargs["pass_fds"]  # type: ignore[misc]
        with pytest.raises(OSError):
            os.fstat(fd)


async def test_isolated_stdio_pipes_are_open_to_the_tool_user(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """The tool user can reopen its pipes (/dev/stdout, /dev/stderr): they are 0666 (KI-96, E11)."""
    isolation(READY)
    identity(IDENT)
    seen: dict[str, object] = {}

    async def fake_exec(*args: object, **kwargs: object) -> _FakeProc:
        if TOOL_WALK in args:  # the sharing pass after the exit
            return _FakeProc()
        for name in ("stdin", "stdout", "stderr"):
            fd = kwargs[name]
            assert isinstance(fd, int)
            seen[name] = (fd, stat.S_IMODE(os.fstat(fd).st_mode), stat.S_ISFIFO(os.fstat(fd).st_mode))
        return _FakeProc()

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake_exec)
    proc = await start_tool_process(
        "cat", env={}, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, limit=1024
    )
    assert {mode for _fd, mode, _fifo in seen.values()} == {0o666}  # type: ignore[misc]
    assert all(fifo for _fd, _mode, fifo in seen.values())  # type: ignore[misc]
    assert seen["stdout"][0] == seen["stderr"][0]  # type: ignore[index]  # stderr=STDOUT: the same pipe
    for _fd, _mode, _fifo in seen.values():  # type: ignore[misc]
        with pytest.raises(OSError):
            os.fstat(_fd)  # the worker closed the child's ends after the spawn
    # The worker's ends are the process's streams; the child's ends are closed, so they read EOF.
    assert isinstance(proc.stdout, asyncio.StreamReader)
    assert isinstance(proc.stdin, asyncio.StreamWriter)
    out, _ = await proc.communicate(b"ignored")
    assert out == b""


async def test_off_keeps_asyncio_pipes(isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]) -> None:
    isolation(IsolationStatus(config=OFF, ready=True))
    await start_tool_process("cat", env={}, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    _args, kwargs = spawns[0]
    assert (kwargs["stdout"], kwargs["stderr"]) == (subprocess.PIPE, subprocess.STDOUT)


async def test_shell_commands_run_through_sh(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    await start_tool_shell("pytest -q && echo ok", env={}, cwd="/ws")
    args, _ = spawns[0]
    assert _command(args) == ["/bin/sh", "-c", "pytest -q && echo ok"]


def test_sync_run_as_the_tool_user(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    identity(IDENT)
    run_tool_process(["git", "log"], env={"PATH": "/bin"}, cwd="/ws", timeout=5)
    args, kwargs = spawns[0]
    assert _command(args) == ["git", "log"]
    assert kwargs["env"] == LAUNCHER_ENV
    assert kwargs["umask"] == 0o007
    assert kwargs["timeout"] == 5
    assert kwargs["cwd"] == "/"
    assert kwargs["spec"]["cwd"] == "/ws"  # type: ignore[index]
    # Then the sharing pass of what changed since, as the same tool user.
    share_args, share_kwargs = spawns[1]
    walk = _command(share_args)
    assert walk[:6] == [INTERPRETER, "-I", "-S", TOOL_WALK, "share", "/ws"]
    assert walk[6] == "--since"
    assert abs(float(walk[7]) - time.time()) < 60  # type: ignore[arg-type]
    assert share_kwargs["spec"]["uid"] == 20007  # type: ignore[index]


async def test_every_tenant_tool_process_shares_its_workspace_after_it_exits(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    """Whatever its working directory: a process can write anywhere in its workspace (KI-96 D8)."""
    isolation(READY)
    identity(IDENT)
    before = time.time()
    proc = await start_tool_process("true", env={}, cwd="/ws/sub")
    assert len(spawns) == 1, "the pass runs once the caller waited for the process"
    await proc.wait()
    await proc.wait()  # once
    walks = [_command(a) for a, _k in spawns[1:]]
    assert len(walks) == 1
    assert walks[0][:6] == [INTERPRETER, "-I", "-S", TOOL_WALK, "share", "/ws"]
    assert float(walks[0][7]) >= before - 1  # type: ignore[arg-type]

    spawns.clear()
    proc = await start_tool_process("true", env={})  # no working directory
    await proc.wait()
    assert [_command(a)[3] for a, _k in spawns[1:]] == [TOOL_WALK]


def test_removal_runs_as_the_tool_user_confined_to_the_directory(
    isolation: Callable[[IsolationStatus], None], spawns: list[Spawn]
) -> None:
    """D10/D11: a tree a tool wrote is removed as its tool UID, never by the worker through a path."""
    isolation(READY)
    home = "/home/codeforge-tools/20007"
    assert tool_process.remove_as_tool_sync([f"{home}/tmp/w1", f"{home}/claude/w1"], IDENT, confine=home)
    ((args, kwargs),) = spawns
    command = _command(args)
    assert command[:2] == ["/bin/sh", "-c"]
    assert command[3:] == ["cf-remove", "all", f"{home}/tmp/w1", f"{home}/claude/w1"]
    spec = kwargs["spec"]
    assert spec["uid"] == 20007  # type: ignore[index]
    assert spec["prepare"] == []  # type: ignore[index]
    writable = [rule[0] for rule in spec["landlock"]["rules"] if "remove-file" in rule[1]]  # type: ignore[index]
    assert sorted(writable) == sorted([home, "/dev/shm"])
    assert "/ws" not in [rule[0] for rule in spec["landlock"]["rules"]]  # type: ignore[index]


# ---------------------------------------------------------------------------
# The worker's own commands as a tool UID end in bounded time (KI-96 review)
# ---------------------------------------------------------------------------

# A helper a process of its UID stopped (below Landlock ABI 6 a tenant's
# leftover process may signal it), with a child in its process group.
_STOPPED_HELPER = 'sleep 30 & echo "$!" > "$0"; kill -STOP $$; echo resumed'


@pytest.fixture
def stopped_helpers(
    isolation: Callable[[IsolationStatus], None], monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> Path:
    """Every command the worker runs as a tool UID gets stopped (real processes, without the launcher).

    Returns the file the helper writes its child's PID to.
    """
    isolation(READY)
    monkeypatch.setattr(tool_process, "HELPER_TIMEOUT_SECONDS", 1.0)
    pidfile = tmp_path / "child.pid"

    def launch(
        _self: IsolationStatus,
        _argv: Sequence[str],
        _env: Mapping[str, str],
        _cwd: str | None,
        _identity: ToolIdentity,
        files: tuple[str, ...] = (),
    ) -> Launch:
        return Launch(
            argv=["/bin/sh", "-c", _STOPPED_HELPER, str(pidfile)],
            env={"PATH": "/usr/bin:/bin"},
            cwd="/",
            umask=TOOL_UMASK,
        )

    monkeypatch.setattr(IsolationStatus, "launch", launch)
    return pidfile


def _within[T](seconds: float, call: Callable[[], T]) -> T:
    """*call*'s result; the test fails when it has not returned after *seconds* (it waits forever)."""
    results: list[T] = []
    errors: list[BaseException] = []

    def run() -> None:
        try:
            results.append(call())
        except BaseException as exc:  # handed to the test below
            errors.append(exc)

    thread = threading.Thread(target=run, daemon=True)
    thread.start()
    thread.join(seconds)
    if thread.is_alive():
        pytest.fail(f"still waiting for a stopped helper after {seconds} s")
    if errors:
        raise errors[0]
    return results[0]


def _child_gone(pidfile: Path, seconds: float = 5.0) -> bool:
    """Whether the stopped helper's child (in its process group) was killed with it: gone or a zombie."""
    deadline = time.monotonic() + seconds
    while not pidfile.exists() or not pidfile.read_text().strip():
        if time.monotonic() >= deadline:
            return False
        time.sleep(0.05)
    pid = pidfile.read_text().strip()
    while True:
        try:
            if Path(f"/proc/{pid}/cmdline").read_bytes() != b"sleep\x0030\x00":
                return True  # the PID is another process's now
            state = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()[0]
        except OSError:
            return True
        if state in ("Z", "X"):
            return True
        if time.monotonic() >= deadline:
            return False
        time.sleep(0.05)


def test_a_removal_a_process_of_its_uid_stopped_ends_after_the_timeout(stopped_helpers: Path) -> None:
    """The worker awaits it while its subject's next message waits: it must not wait forever."""
    started = time.monotonic()
    assert not _within(15, lambda: tool_process.remove_as_tool_sync(["/h/tmp/x"], IDENT, confine="/h"))
    assert time.monotonic() - started < 10
    assert _child_gone(stopped_helpers), "the helper's whole process group is killed"


async def test_a_sharing_pass_a_process_of_its_uid_stopped_ends_after_the_timeout(stopped_helpers: Path) -> None:
    started = time.monotonic()
    await asyncio.wait_for(tool_process.share_tool_files("/ws", IDENT), 15)
    assert time.monotonic() - started < 10
    assert _child_gone(stopped_helpers)


def test_a_synchronous_sharing_pass_a_process_of_its_uid_stopped_ends_after_the_timeout(
    stopped_helpers: Path,
) -> None:
    _within(15, lambda: tool_process.share_tool_files_sync("/ws", IDENT))
    assert _child_gone(stopped_helpers)


def test_a_stopped_walker_is_reported_as_killed(stopped_helpers: Path) -> None:
    done = _within(15, lambda: tool_process.run_walker(IDENT, ["share", "/ws"]))
    assert done.returncode == -signal.SIGKILL
    assert "timed out" in done.stderr
    assert _child_gone(stopped_helpers)


async def test_a_benchmark_workspace_is_emptied_as_the_tool_user(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
    tmp_path: Path,
) -> None:
    isolation(READY)
    identity(IDENT)
    async with tool_process.tool_workspace("cf-bench-", str(tmp_path)) as path:
        assert current_identity.get().workspace == path  # type: ignore[union-attr]
        acl = posix_acl.get_acl(path, posix_acl.DEFAULT)
        assert posix_acl.Entry(posix_acl.USER, 7, 20007) in (acl or [])
    ((args, kwargs),) = spawns
    assert _command(args)[3:] == ["cf-remove", "contents", path]
    assert [rule[0] for rule in kwargs["spec"]["landlock"]["rules"] if rule[2]] == [path]  # type: ignore[index]
    assert not Path(path).exists(), "the worker removes the emptied directory"
    assert current_identity.get() is IDENT


async def test_tenantless_and_off_spawns_share_nothing(
    isolation: Callable[[IsolationStatus], None],
    spawns: list[Spawn],
) -> None:
    isolation(READY)
    system = ToolIdentity(tenant_id="", uid=19999, home="/home/codeforge-tools/19999", work_id="sys")
    proc = await start_tool_process("true", env={}, identity=system)
    await proc.wait()
    no_workspace = ToolIdentity(tenant_id="t", uid=20007, home="/h", work_id="w")
    proc = await start_tool_process("true", env={}, identity=no_workspace)
    await proc.wait()
    assert len(spawns) == 2
    isolation(IsolationStatus(config=OFF, ready=True))
    spawns.clear()
    proc = await start_tool_process("true", env={}, cwd="/ws")
    await proc.wait()
    assert len(spawns) == 1


async def test_required_without_isolation_fails_and_starts_nothing(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    isolation(BROKEN)
    identity(IDENT)
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


def test_tool_env_is_the_identitys_environment(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Every cache, config, data and temp location below the tenant's HOME; the tool PATH (S6, O1, O7)."""
    monkeypatch.setenv("HOME", "/home/codeforge")
    monkeypatch.setenv("PATH", "/app/.venv/bin:/usr/bin")
    monkeypatch.setenv("GOCACHE", "/shared/go")
    monkeypatch.setenv("XDG_CACHE_HOME", "/shared/cache")
    monkeypatch.setenv("npm_config_cache", "/shared/npm")
    monkeypatch.setenv("TMPDIR", "/tmp")
    isolation(READY)
    identity(IDENT)
    env = tool_env()
    home, tmp = "/home/codeforge-tools/20007", "/home/codeforge-tools/20007/tmp/tok1"
    assert env["HOME"] == home
    assert env["USER"] == env["LOGNAME"] == "codeforge-t20007"
    assert env["PATH"].startswith(TOOL_PATH + ":")
    assert "/app/.venv" not in env["PATH"]
    assert env["PATH"].endswith(f"{home}/.npm-global/bin")
    for name in ("TMPDIR", "TMP", "TEMP", "GOTMPDIR", "TMUX_TMPDIR"):
        assert env[name] == tmp, name
    assert env["JAVA_TOOL_OPTIONS"] == f"-Djava.io.tmpdir={tmp}"
    for name in ("XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "GOPATH", "GOMODCACHE",
                 "GOCACHE", "CARGO_HOME", "RUSTUP_HOME", "npm_config_cache", "npm_config_prefix", "PIP_CACHE_DIR",
                 "UV_CACHE_DIR"):  # fmt: skip
        assert env[name].startswith(home + "/"), name
    # An explicit value still wins (the Claude Code config directory).
    assert tool_env(extra={"CLAUDE_CONFIG_DIR": "/x"})["CLAUDE_CONFIG_DIR"] == "/x"


def test_operator_values_of_identity_variables_are_dropped_with_a_warning(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    monkeypatch.setenv("GOCACHE", "/shared/go")
    monkeypatch.setattr(tool_identity, "_warned_dropped", set())
    isolation(READY)
    identity(IDENT)
    with caplog.at_level("WARNING"):
        tool_env()
        tool_env()
    warnings = [r for r in caplog.records if "GOCACHE" in r.getMessage()]
    assert len(warnings) == 1


def test_tool_env_unchanged_when_off(
    isolation: Callable[[IsolationStatus], None], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("HOME", "/home/codeforge")
    monkeypatch.setenv("GOCACHE", "/shared/go")
    monkeypatch.delenv("LOGNAME", raising=False)
    isolation(IsolationStatus(config=OFF, ready=True))
    env = tool_env()
    assert env["HOME"] == "/home/codeforge"
    assert env["GOCACHE"] == "/shared/go"
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
Umask:\t0007
Uid:\t19999\t19999\t19999\t19999
Gid:\t19999\t19999\t19999\t19999
Groups:\t
CapInh:\t0000000000000000
CapPrm:\t0000000000000000
CapEff:\t0000000000000000
CapBnd:\t00000000000000e0
CapAmb:\t0000000000000000
NoNewPrivs:\t1
"""


_PROBE_OK = _STATUS_OK + "cf-ok home\ncf-ok tmp\ncf-ok python3\ncf-ok git\n"
_EXPECTED = ("home", "tmp", "python3", "git")


def test_probe_of_an_isolated_tool_process_passes() -> None:
    assert probe_problems(_STATUS_OK, 19999) == []
    assert probe_problems(_PROBE_OK, 19999, _EXPECTED) == []


@pytest.mark.parametrize("missing", _EXPECTED)
def test_probe_finds_what_a_tool_cannot_do(missing: str) -> None:
    """E15: a tool PATH whose python3 cannot run under Landlock (the worker's venv) fails the check."""
    problems = probe_problems(_PROBE_OK.replace(f"cf-ok {missing}\n", ""), 19999, _EXPECTED)
    assert any(missing in p for p in problems), problems


@pytest.mark.parametrize(
    "line",
    ["cf-readable /proc/1/cmdline", "cf-readable /dev/shm", "cf-readable /var/lib/codeforge/landlock-canary",
     "cf-writable /tmp/cf-probe-x"],
)  # fmt: skip
def test_probe_finds_what_landlock_let_through(line: str) -> None:
    assert probe_problems(_PROBE_OK + line + "\n", 19999, _EXPECTED)


@pytest.mark.parametrize(
    ("line", "replacement", "problem"),
    [
        ("Uid:\t19999\t19999\t19999\t19999", "Uid:\t10001\t10001\t10001\t10001", "uid"),
        ("Uid:\t19999\t19999\t19999\t19999", "Uid:\t19999\t10001\t19999\t19999", "uid"),
        ("Gid:\t19999\t19999\t19999\t19999", "Gid:\t10001\t10001\t10001\t10001", "gid"),
        ("Groups:\t", "Groups:\t10010", "groups"),
        ("Groups:\t", "Groups:\t10001 10010", "groups"),
        ("CapEff:\t0000000000000000", "CapEff:\t00000000000000c0", "capabilities"),
        ("CapPrm:\t0000000000000000", "CapPrm:\t0000000000000020", "capabilities"),
        ("CapAmb:\t0000000000000000", "CapAmb:\t00000000000000c0", "capabilities"),
        ("CapInh:\t0000000000000000", "CapInh:\t0000000000000080", "capabilities"),
        ("NoNewPrivs:\t1", "NoNewPrivs:\t0", "no_new_privs"),
        ("Umask:\t0007", "Umask:\t0002", "umask"),
    ],
)
def test_probe_finds_what_is_wrong(line: str, replacement: str, problem: str) -> None:
    problems = probe_problems(_STATUS_OK.replace(line, replacement), 19999)
    assert problems, f"{problem} not detected"
    assert any(problem in p for p in problems), problems


def test_probe_reports_readable_secrets_and_environ() -> None:
    output = _STATUS_OK + "cf-readable /run/secrets/database-url\ncf-readable /proc/1/environ\n"
    problems = probe_problems(output, 19999)
    assert any("/run/secrets/database-url" in p for p in problems)
    assert any("/proc/1/environ" in p for p in problems)


def test_probe_without_status_output_fails() -> None:
    assert probe_problems("", 19999)


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


@pytest.fixture
def checkable(monkeypatch: pytest.MonkeyPatch) -> None:
    """A worker that could isolate: launcher, capabilities, volumes, the system HOME, Landlock ABI 7."""
    monkeypatch.setattr(tool_process.shutil, "which", lambda *_a, **_k: LAUNCHER)
    monkeypatch.setattr(tool_process, "_is_root", lambda: False)
    monkeypatch.setattr(tool_process, "_own_status", lambda: _WORKER_STATUS.format(amb="00000000000000e0"))
    monkeypatch.setattr(tool_process, "volume_problems", lambda _config: [])
    monkeypatch.setattr(tool_identity, "tenant_home", lambda base, uid: f"{base}/{uid}")
    monkeypatch.setattr(landlock, "kernel_abi", lambda: 7)
    monkeypatch.setattr(tool_process, "_scope_problem", lambda _status, _identity: "")
    monkeypatch.setattr(tool_process, "_run_probe", lambda launch, _timeout: launch.close() or _PROBE_OK)


@pytest.mark.parametrize(
    ("config", "reason"),
    [
        (IsolationConfig(mode="required", workspace_root="/w", landlock="off", production=True), "APP_ENV=production"),
        (IsolationConfig(mode="required", workspace_root="/w", landlock_min_abi=8), "ABI 7"),
        (IsolationConfig(mode="required", workspace_root="/w", read_paths=("/run/secrets",)), "READ_PATHS"),
    ],
)
def test_landlock_settings_that_make_isolation_not_ready(config: IsolationConfig, reason: str, checkable: None) -> None:
    status = check_tool_isolation(config)
    assert not status.ready
    assert reason in status.reason


def test_landlock_off_outside_production_is_ready(checkable: None) -> None:
    status = check_tool_isolation(IsolationConfig(mode="required", workspace_root="/w", landlock="off"))
    assert status.ready, status.reason
    assert status.landlock_abi == 0


def test_a_kernel_without_scopes_is_ready_without_the_scope_check(
    checkable: None, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(landlock, "kernel_abi", lambda: 5)

    def no_scope_check(_status: object, _identity: object) -> str:
        raise AssertionError("ABI 5 has no scopes")

    monkeypatch.setattr(tool_process, "_scope_problem", no_scope_check)
    status = check_tool_isolation(CONFIG)
    assert status.ready, status.reason
    assert status.landlock_abi == 5


def test_an_unscoped_signal_makes_isolation_not_ready(checkable: None, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process, "_scope_problem", lambda _s, _i: "a tool process can signal another domain")
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "signal" in status.reason


def test_check_required_without_capabilities_is_not_ready(checkable: None, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process, "_own_status", lambda: _WORKER_STATUS.format(amb="0000000000000000"))
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "CAP_SETUID" in status.reason


def test_check_required_with_volume_problems_is_not_ready(checkable: None, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process, "volume_problems", lambda _config: ["the tool HOME base is mounted noexec"])
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "noexec" in status.reason


def test_check_required_runs_the_probe_as_the_system_tool_user(
    checkable: None, monkeypatch: pytest.MonkeyPatch
) -> None:
    probes: list[tuple[list[str], dict[str, object] | None]] = []

    def fake_probe(launch: tool_process.Launch, _timeout: float) -> str:
        probes.append((launch.argv, read_spec({"pass_fds": launch.pass_fds})))
        launch.close()
        return _PROBE_OK

    monkeypatch.setattr(tool_process, "_run_probe", fake_probe)
    status = check_tool_isolation(CONFIG)
    assert status.ready, status.reason
    assert status.landlock_abi == 7
    argv, spec = probes[0]
    assert argv[: len(_prefix(19999))] == _prefix(19999)
    # It must not reach the worker's environment and command line, /dev/shm's listing.
    for path in (f"/proc/{os.getpid()}/environ", f"/proc/{os.getpid()}/cmdline", "/dev/shm"):
        assert path in argv
    assert spec is not None
    assert spec["uid"] == 19999
    assert spec["groups"] == []
    assert spec["home"] == "/home/codeforge-tools/19999"
    assert isinstance(spec["landlock"], dict)
    # The real tool environment: the tool PATH and the identity's HOME.
    assert spec["env"]["PATH"].startswith(TOOL_PATH)  # type: ignore[index]
    assert spec["env"]["CF_PROBE_TMP"].startswith("/tmp/")  # type: ignore[index]

    monkeypatch.setattr(tool_process, "_run_probe", lambda *_a: _PROBE_OK.replace("NoNewPrivs:\t1", "NoNewPrivs:\t0"))
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "no_new_privs" in status.reason


def test_check_required_probe_failure_is_not_ready(checkable: None, monkeypatch: pytest.MonkeyPatch) -> None:
    def failing_probe(_launch: tool_process.Launch, _timeout: float) -> str:
        raise OSError("setpriv: setresuid failed: Operation not permitted")

    monkeypatch.setattr(tool_process, "_run_probe", failing_probe)
    status = check_tool_isolation(CONFIG)
    assert not status.ready
    assert "setresuid" in status.reason


@pytest.mark.parametrize(
    ("config", "problem"),
    [
        (IsolationConfig(mode="required"), "CODEFORGE_WORKSPACE_ROOT"),
        (IsolationConfig(mode="required", workspace_root="data/workspaces"), "CODEFORGE_WORKSPACE_ROOT"),
        (IsolationConfig(mode="required", workspace_root="/data/w", home_base="home"), "absolute"),
    ],
)
def test_volume_problems_need_absolute_roots(config: IsolationConfig, problem: str) -> None:
    problems = tool_process.volume_problems(config)
    assert problems
    assert problem in problems[0]


# ---------------------------------------------------------------------------
# Granting a tool identity access to what the worker made for it
# ---------------------------------------------------------------------------


def test_grant_tool_access_is_a_no_op_when_off(isolation: Callable[[IsolationStatus], None], tmp_path: Path) -> None:
    isolation(IsolationStatus(config=OFF, ready=True))
    path = tmp_path / "f"
    path.write_text("x")
    path.chmod(0o600)
    grant_tool_access(str(path), writable=True)
    assert stat.S_IMODE(path.stat().st_mode) == 0o600
    assert posix_acl.get_acl(str(path), posix_acl.ACCESS) is None


@pytest.mark.parametrize(
    ("is_dir", "writable", "tool_perm"),
    [(True, True, 7), (True, False, 5), (False, True, 6), (False, False, 4)],
)
def test_grant_tool_access(
    is_dir: bool,
    writable: bool,
    tool_perm: int,
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    tmp_path: Path,
) -> None:
    isolation(READY)
    identity(IDENT)
    path = tmp_path / "x"
    if is_dir:
        path.mkdir(mode=0o700)
    else:
        path.write_text("x")
        path.chmod(0o600)
    try:
        grant_tool_access(str(path), writable=writable)
    except OSError as exc:
        if exc.errno == errno.EOPNOTSUPP:
            pytest.skip("no POSIX ACLs on the test file system")
        raise
    acl = posix_acl.get_acl(str(path), posix_acl.ACCESS)
    assert acl is not None
    assert posix_acl.Entry(posix_acl.USER, tool_perm, 20007) in acl
    assert posix_acl.Entry(posix_acl.GROUP_OBJ, 0) in acl
    assert posix_acl.Entry(posix_acl.OTHER, 0) in acl


def test_grant_tool_access_never_follows_a_symlink(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    tmp_path: Path,
) -> None:
    isolation(READY)
    identity(IDENT)
    target = tmp_path / "target"
    target.write_text("x")
    link = tmp_path / "link"
    link.symlink_to(target)
    with pytest.raises(ToolIsolationError):
        grant_tool_access(str(link), writable=True)
    assert posix_acl.get_acl(str(target), posix_acl.ACCESS) is None


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
    "anyio": {"open_process", "run_process"},
}
# Library functions that start processes, however they are imported.
_SPAWN_NAMES = {"stdio_client", "StdioServerParameters"}
# Event loop methods that start processes, whatever the loop object is called.
_LOOP_METHODS = {"subprocess_exec", "subprocess_shell"}


def _spawn_calls(path: Path, base: Path = WORKERS_DIR) -> list[str]:
    tree = ast.parse(path.read_text(), filename=str(path))
    found: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Attribute):
            owner = node.value.id if isinstance(node.value, ast.Name) else ""
            if node.attr in _SPAWN_ATTRIBUTES.get(owner, set()) or node.attr in _LOOP_METHODS | _SPAWN_NAMES:
                found.append(f"{path.relative_to(base)}:{node.lineno} {owner}.{node.attr}")
        elif isinstance(node, ast.Name) and node.id in _SPAWN_NAMES:
            found.append(f"{path.relative_to(base)}:{node.lineno} {node.id}")
        elif isinstance(node, ast.ImportFrom):
            found.extend(
                f"{path.relative_to(base)}:{node.lineno} from {node.module} import {alias.name}"
                for alias in node.names
                if alias.name in _SPAWN_ATTRIBUTES.get(node.module or "", set()) | _SPAWN_NAMES
            )
    return found


# The launcher (tool_process) and its second half, the launch helper that
# executes the command as the tool user (KI-96).
_LAUNCHER_MODULES = {SOURCE_DIR / "tool_process.py", SOURCE_DIR / "tool_exec.py"}


def test_only_tool_process_starts_processes() -> None:
    """A new subprocess call outside codeforge.tool_process would run as the worker user."""
    offenders: list[str] = []
    for path in sorted(SOURCE_DIR.rglob("*.py")):
        if path in _LAUNCHER_MODULES:
            continue
        offenders.extend(_spawn_calls(path))
    assert offenders == [], "start processes through codeforge.tool_process:\n" + "\n".join(offenders)


def test_the_helper_only_executes_the_spec_command() -> None:
    """tool_exec.py executes (never forks or spawns), once."""
    calls = _spawn_calls(SOURCE_DIR / "tool_exec.py")
    assert len(calls) == 1, calls
    assert calls[0].endswith("os.execvpe"), calls


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
        "from mcp import stdio_client\n"
        "import anyio, mcp\n"
        "async def g():\n"
        "    await anyio.open_process(['x'])\n"
        "    await anyio.run_process(['x'])\n"
        "    mcp.StdioServerParameters(command='x')\n"
    )
    found = _spawn_calls(sample, tmp_path)
    assert len(found) == 9, found
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
async def test_every_spawn_site_runs_as_the_tenants_tool_user(
    site: str,
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("PATH", "/usr/bin:/bin")
    monkeypatch.setattr(tool_identity, "tenant_home", lambda base, uid: f"{base}/{uid}")
    isolation(READY)
    identity(IDENT.with_workspace(str(tmp_path)))
    await _site(site)(tmp_path)
    assert spawns, f"{site} started no process"
    for args, kwargs in spawns:
        spec = kwargs["spec"]
        assert spec is not None, site
        uid = spec["uid"]  # type: ignore[index]
        # The CLI check belongs to no tenant: the system tool user.
        assert uid == (19999 if site == "claude_code_cli_check" else 20007), site
        assert _command(args, uid), f"{site}: {args}"  # type: ignore[arg-type]
        assert kwargs.get("umask") == 0o007, site
        assert kwargs["env"] == LAUNCHER_ENV, site
        assert spec["groups"] == [], site  # type: ignore[index]


@pytest.mark.parametrize("site", ["bash", *_SITES])
async def test_every_spawn_site_fails_closed(
    site: str,
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
    tmp_path: Path,
) -> None:
    isolation(BROKEN)
    identity(IDENT.with_workspace(str(tmp_path)))
    with contextlib.suppress(ToolIsolationError):
        await _site(site)(tmp_path)
    assert spawns == [], f"{site} started a process without isolation"


@pytest.mark.parametrize("site", ["bash", *[s for s in _SITES if s != "claude_code_cli_check"]])
async def test_every_spawn_site_without_a_tenant_fails_closed(
    site: str,
    isolation: Callable[[IsolationStatus], None],
    spawns: list[Spawn],
    tmp_path: Path,
) -> None:
    """A tenant's spawn site with no tool identity starts nothing (only the marked system calls may)."""
    isolation(READY)
    with contextlib.suppress(ToolIsolationError):
        await _site(site)(tmp_path)
    assert spawns == [], f"{site} started a process without a tool identity"


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
    ("mode", "ready", "locks", "umask"),
    [
        ("off", True, False, None),
        ("required", True, True, 0o002),
        ("required", False, True, 0o002),
    ],
)
def test_setup_tool_isolation(
    mode: str,
    ready: bool,
    locks: bool,
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
        calls.append(f"check {config.workspace_root}")
        return IsolationStatus(
            config=config,
            ready=ready,
            reason="" if ready else "no CAP_SETUID",
            launcher=LAUNCHER,
            interpreter=INTERPRETER,
        )

    monkeypatch.setattr(consumer_module, "configure_tool_isolation", fake_configure)
    monkeypatch.setattr(consumer_module, "lock_secrets_dir", lambda: calls.append("lock") or True)
    monkeypatch.setattr(consumer_module.os, "umask", lambda value: umasks.append(value) or 0o022)

    status = consumer_module.setup_tool_isolation(WorkerSettings())

    assert status.ready is ready
    assert calls[0] == "check /data/workspaces"
    assert ("lock" in calls) is locks
    assert umasks == ([] if umask is None else [umask])


# ---------------------------------------------------------------------------
# MCP stdio servers
# ---------------------------------------------------------------------------


async def test_mcp_stdio_server_runs_as_the_tenants_tool_user(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from codeforge.tool_process import tool_stdio_client

    monkeypatch.setenv("PATH", "/usr/bin:/bin")
    monkeypatch.setenv("CODEFORGE_INTERNAL_KEY", "internal-admin-key")
    isolation(READY)
    identity(IDENT)
    with open(os.devnull, "w") as errlog:
        async with tool_stdio_client(
            "npx",
            ["-y", "@modelcontextprotocol/server-github"],
            declared_env={"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_x", "LD_PRELOAD": "/w/evil.so", "PYTHONPATH": "/w"},
            errlog=errlog,
        ):
            pass
    (args, kwargs), *sharing = spawns
    assert all(_command(a)[3] == TOOL_WALK for a, _k in sharing), sharing
    assert _command(args) == ["npx", "-y", "@modelcontextprotocol/server-github"]
    # The server's token is in the spec on the memfd, never an argument (KI-96).
    assert not [a for a in args if "ghp_x" in str(a)]
    spec = kwargs["spec"]
    env = spec["env"]  # type: ignore[index]
    assert env["GITHUB_PERSONAL_ACCESS_TOKEN"] == "ghp_x"  # noqa: S105 - a test value
    assert env["HOME"] == "/home/codeforge-tools/20007"
    assert not [name for name in env if name.startswith(("LD_PRELOAD", "PYTHONPATH", "CODEFORGE_"))]
    # It starts in the run's workspace.
    assert spec["cwd"] == "/ws"  # type: ignore[index]
    assert kwargs["env"] == LAUNCHER_ENV
    assert kwargs["start_new_session"] is True


async def test_mcp_stdio_server_off_keeps_the_command(
    isolation: Callable[[IsolationStatus], None], spawns: list[Spawn], monkeypatch: pytest.MonkeyPatch
) -> None:
    from codeforge.tool_process import tool_stdio_client

    monkeypatch.setenv("CODEFORGE_INTERNAL_KEY", "internal-admin-key")
    isolation(IsolationStatus(config=OFF, ready=True))
    with open(os.devnull, "w") as errlog:
        async with tool_stdio_client("node", ["server.js"], declared_env={"API_TOKEN": "t"}, errlog=errlog):
            pass
    ((args, kwargs),) = spawns
    assert args == ("node", "server.js")
    assert kwargs["env"]["API_TOKEN"] == "t"  # type: ignore[index]  # noqa: S105 - a test value
    assert "CODEFORGE_INTERNAL_KEY" not in kwargs["env"]  # type: ignore[operator]


async def test_mcp_stdio_server_fails_closed(
    isolation: Callable[[IsolationStatus], None],
    identity: Callable[[ToolIdentity | None], None],
    spawns: list[Spawn],
) -> None:
    from codeforge.mcp_models import MCPServerDef
    from codeforge.mcp_workbench import McpServerConnection

    isolation(BROKEN)
    identity(IDENT)
    connection = McpServerConnection(MCPServerDef(id="s1", name="s1", transport="stdio", command="node"))
    with pytest.raises(ToolIsolationError):
        await connection.connect()
    assert spawns == []


# A minimal MCP stdio server: newline-delimited JSON-RPC, standard library only.
ECHO_MCP_SERVER = """
import json, sys
for line in sys.stdin:
    msg = json.loads(line)
    method, mid = msg.get("method"), msg.get("id")
    if mid is None:
        continue
    if method == "initialize":
        result = {"protocolVersion": msg["params"]["protocolVersion"], "capabilities": {"tools": {}},
                  "serverInfo": {"name": "echo", "version": "1"}}
    elif method == "tools/list":
        result = {"tools": [{"name": "echo", "description": "echo", "inputSchema": {"type": "object"}}]}
    elif method == "tools/call":
        result = {"content": [{"type": "text", "text": json.dumps(msg["params"]["arguments"])}], "isError": False}
    else:
        result = {}
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": mid, "result": result}) + "\\n")
    sys.stdout.flush()
"""


async def test_mcp_stdio_transport_talks_to_a_real_server(
    isolation: Callable[[IsolationStatus], None], tmp_path: Path
) -> None:
    import sys

    from codeforge.mcp_models import MCPServerDef
    from codeforge.mcp_workbench import McpServerConnection

    isolation(IsolationStatus(config=OFF, ready=True))
    script = tmp_path / "echo_server.py"
    script.write_text(ECHO_MCP_SERVER)
    connection = McpServerConnection(
        MCPServerDef(id="echo", name="echo", transport="stdio", command=sys.executable, args=[str(script)])
    )
    await connection.connect()
    try:
        assert [tool.name for tool in await connection.list_tools()] == ["echo"]
        result = await connection.call_tool("echo", {"text": "hi"})
    finally:
        await connection.disconnect()
    assert json.loads(result.output) == {"text": "hi"}


@pytest.mark.parametrize(
    ("name", "kept"),
    [
        ("GITHUB_PERSONAL_ACCESS_TOKEN", True),
        ("API_KEY", True),
        ("PATH", True),
        ("LD_PRELOAD", False),
        ("ld_library_path", False),
        ("LD_AUDIT", False),
        ("PYTHONPATH", False),
        ("PYTHONSTARTUP", False),
        ("NODE_OPTIONS", False),
        ("PERL5OPT", False),
        ("RUBYOPT", False),
        ("BASH_ENV", False),
        ("GCONV_PATH", False),
        ("GLIBC_TUNABLES", False),
        ("CODEFORGE_INTERNAL_KEY", False),
        ("LITELLM_MASTER_KEY", False),
        ("DATABASE_URL", False),
        ("NATS_URL_FILE", False),
    ],
)
def test_declared_tool_env(name: str, kept: bool) -> None:
    from codeforge.subprocess_env import declared_tool_env

    assert (name in declared_tool_env({name: "v"})) is kept
    assert declared_tool_env(None) == {}
