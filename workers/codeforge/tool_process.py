"""Start every process that runs for an agent, as the tool user (KI-71).

Agent tools run commands an LLM chose or code it wrote: Bash, grep, git in the
workspace, quality gates and workspace tests, benchmark test commands, the
agent CLIs of ``tasks.agent.*``, the Claude Code CLI and its policy hook. They
get a scrubbed environment (``codeforge.subprocess_env.tool_env``), and with
tool isolation they also run as a separate, unprivileged user
(``codeforge-tool``, uid 10002): they cannot read the worker's secret files
or ``/proc/<worker pid>/environ``, cannot signal or trace the worker, and have
no capabilities.

Mechanism: the worker container starts as root with only CAP_SETUID,
CAP_SETGID and CAP_KILL; its entrypoint (``scripts/worker-entrypoint.sh``) runs
the worker as the worker user (uid 10001) and keeps those three as ambient
capabilities. Every tool process starts through setpriv (util-linux), which
sets the tool user's UID and GID, the workspace group (gid 10010) as its only
supplementary group, clears the inheritable and ambient capability sets and
sets no_new_privs before it executes the command; the command therefore runs
without capabilities and cannot gain any. setpriv itself still holds the
worker's capabilities and the dynamic loader honours LD_PRELOAD and friends
for it (ambient capabilities do not set AT_SECURE), so it runs with a fixed
environment of its own; env(1), already as the tool user, gives the command
its environment. Popen closes every other file descriptor and sets the umask
(002: files stay writable for the workspace group, which the Go Core is in
too). CAP_KILL lets the worker stop tool processes of another user
(timeouts, cancels). MCP stdio servers start the same way
(tool_stdio_client).

``CODEFORGE_TOOL_ISOLATION`` selects the mode: ``required`` (the worker image,
docker-compose.prod.yml) starts tool processes only as the tool user and
fails the tool call with ToolIsolationError, starting nothing, when that is
not possible; ``off`` (the default elsewhere: development, tests, the
devcontainer) starts them as the worker user, as before. No other module
starts processes (tests/test_tool_process.py checks the sources).
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
import shutil
import stat
import subprocess
import threading
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Mapping, Sequence
    from contextlib import AbstractAsyncContextManager
    from typing import TextIO

    from anyio.streams.memory import MemoryObjectReceiveStream, MemoryObjectSendStream
    from mcp.shared.message import SessionMessage

    from codeforge.config import WorkerSettings

logger = logging.getLogger(__name__)

ISOLATION_REQUIRED = "required"
ISOLATION_OFF = "off"
# Files tool processes create stay writable for the workspace group.
TOOL_UMASK = 0o002
TOOL_USER = "codeforge-tool"

_LAUNCHER = "setpriv"
_ENV_PROGRAM = "env"
_SHELL = "/bin/sh"
_PROBE_TIMEOUT_SECONDS = 10.0
_PROBE_PATH = "/usr/local/bin:/usr/bin:/bin"
_READABLE_MARKER = "cf-readable "
# Run as the tool user by the isolation check: the process status of the tool
# process (cat inherits its credentials), then every path it can read.
_PROBE_SCRIPT = f"""cat /proc/self/status
for path in "$@"; do
  if [ -d "$path" ]; then
    ls -- "$path" >/dev/null 2>&1 && echo "{_READABLE_MARKER}$path"
  elif cat -- "$path" >/dev/null 2>&1; then
    echo "{_READABLE_MARKER}$path"
  fi
done
exit 0
"""

# The whole environment of setpriv, which runs with the worker's capabilities.
_LAUNCHER_ENV = {"PATH": _PROBE_PATH}

# Capability numbers (linux/capability.h).
_CAP_KILL = 5
_CAP_SETGID = 6
_CAP_SETUID = 7
_CAP_NAMES = {_CAP_KILL: "CAP_KILL", _CAP_SETGID: "CAP_SETGID", _CAP_SETUID: "CAP_SETUID"}
_TOOL_CAP_SETS = ("CapInh", "CapPrm", "CapEff", "CapAmb")


class ToolIsolationError(PermissionError):
    """Tool isolation is required, but tool processes cannot run as the tool user.

    An OSError, so callers that report a process that could not start as a
    failed tool call report this one too.
    """


def parse_isolation_mode(raw: str) -> str:
    """Return the isolation mode a CODEFORGE_TOOL_ISOLATION value selects; unknown values fail closed."""
    value = raw.strip().lower()
    if value in ("", ISOLATION_OFF):
        return ISOLATION_OFF
    if value != ISOLATION_REQUIRED:
        logger.error("unknown CODEFORGE_TOOL_ISOLATION value %r, using %r", raw, ISOLATION_REQUIRED)
    return ISOLATION_REQUIRED


@dataclass(frozen=True)
class IsolationConfig:
    """Who tool processes run as.

    ``workspace_gid`` is their only supplementary group (-1: none); the worker
    and the Go Core are in it too, so all three can write the workspaces.
    """

    mode: str
    uid: int
    gid: int
    workspace_gid: int
    home: str

    @property
    def required(self) -> bool:
        return self.mode == ISOLATION_REQUIRED

    @property
    def groups(self) -> tuple[int, ...]:
        return (self.workspace_gid,) if self.workspace_gid >= 0 else ()

    @classmethod
    def from_settings(cls, settings: WorkerSettings) -> IsolationConfig:
        return cls(
            mode=parse_isolation_mode(settings.tool_isolation),
            uid=settings.tool_uid,
            gid=settings.tool_gid,
            workspace_gid=settings.workspace_gid,
            home=settings.tool_home,
        )


@dataclass(frozen=True)
class IsolationStatus:
    """Whether tool processes can start, and how.

    With isolation off they always can (as the worker user). With isolation
    required they can once the check passed (``ready``); ``reason`` says why
    it did not, ``launcher`` is the setpriv that starts them.
    """

    config: IsolationConfig
    ready: bool
    reason: str = ""
    launcher: str = ""
    env_program: str = ""

    def command(self, argv: Sequence[str], env: Mapping[str, str]) -> list[str]:
        """The command line that runs *argv* as the tool user with exactly the environment *env*."""
        assignments = [f"{name}={value}" for name, value in env.items() if name and "=" not in name]
        return [*self.launch_prefix(), self.env_program, "-i", "--", *assignments, *argv]

    def launch_prefix(self) -> list[str]:
        """The setpriv command line that runs the command after it as the tool user."""
        groups = ",".join(str(gid) for gid in self.config.groups)
        return [
            self.launcher,
            f"--reuid={self.config.uid}",
            f"--regid={self.config.gid}",
            f"--groups={groups}" if groups else "--clear-groups",
            "--inh-caps=-all",
            "--ambient-caps=-all",
            "--no-new-privs",
            "--",
        ]


_status: IsolationStatus | None = None
_status_lock = threading.Lock()


def configure_tool_isolation(config: IsolationConfig) -> IsolationStatus:
    """Check that tool processes can start as *config* says; remember and return the result.

    The worker calls it once at startup, after reading its secrets.
    """
    global _status
    status = check_tool_isolation(config)
    with _status_lock:
        _status = status
    return status


def tool_isolation() -> IsolationStatus:
    """The isolation status; checked from the worker settings on first use."""
    global _status
    with _status_lock:
        if _status is None:
            from codeforge.config import get_settings

            _status = check_tool_isolation(IsolationConfig.from_settings(get_settings()))
        return _status


def tool_identity_env() -> dict[str, str]:
    """HOME, USER and LOGNAME of the tool user when tool processes run as it; else nothing."""
    config = tool_isolation().config
    if not config.required:
        return {}
    return {"HOME": config.home, "USER": TOOL_USER, "LOGNAME": TOOL_USER}


# ---------------------------------------------------------------------------
# The isolation check
# ---------------------------------------------------------------------------


def check_tool_isolation(config: IsolationConfig) -> IsolationStatus:
    """Return whether tool processes can start as *config* says.

    Required isolation is checked by starting a tool process: it must run as
    the tool user and group with the workspace group only, without any
    capability, with no_new_privs and umask 002, and must not be able to read
    the worker's environment or secret files.
    """
    if not config.required:
        return IsolationStatus(config=config, ready=True)
    launcher = shutil.which(_LAUNCHER, path=_PROBE_PATH)
    env_program = shutil.which(_ENV_PROGRAM, path=_PROBE_PATH)
    if launcher is None or env_program is None:
        return _not_ready(config, "setpriv (util-linux) or env (coreutils) is not installed")
    if config.uid <= 0 or config.gid <= 0 or config.uid == os.getuid() or config.gid == os.getgid():
        return _not_ready(
            config,
            f"the tool user (uid {config.uid}, gid {config.gid}) must be an unprivileged user "
            f"that differs from the worker user (uid {os.getuid()}, gid {os.getgid()})",
        )
    problems = worker_capability_problems(_own_status(), root=_is_root())
    if problems:
        return _not_ready(config, "; ".join(problems))
    status = IsolationStatus(config=config, ready=True, launcher=launcher, env_program=env_program)
    argv = status.command([_SHELL, "-c", _PROBE_SCRIPT, "cf-isolation-check", *_probe_paths()], _LAUNCHER_ENV)
    try:
        output = _run_probe(argv, _PROBE_TIMEOUT_SECONDS)
    except (OSError, subprocess.SubprocessError) as exc:
        return _not_ready(config, f"the isolation check could not start a tool process: {exc}")
    problems = probe_problems(output, config)
    if problems:
        return _not_ready(config, "a tool process " + "; ".join(problems))
    return status


def _not_ready(config: IsolationConfig, reason: str) -> IsolationStatus:
    return IsolationStatus(config=config, ready=False, reason=reason)


def _is_root() -> bool:
    return os.geteuid() == 0


def _own_status() -> str:
    try:
        return Path("/proc/self/status").read_text()
    except OSError:
        return ""


def _status_fields(text: str) -> dict[str, str]:
    fields: dict[str, str] = {}
    for line in text.splitlines():
        name, sep, value = line.partition(":")
        if sep:
            fields[name.strip()] = value.strip()
    return fields


def _capabilities(fields: Mapping[str, str], name: str) -> int | None:
    try:
        return int(fields[name], 16)
    except (KeyError, ValueError):
        return None


def worker_capability_problems(status_text: str, *, root: bool = False) -> list[str]:
    """What the worker lacks to start tool processes as the tool user and stop them.

    setpriv needs CAP_SETUID and CAP_SETGID after the worker executed it:
    ambient capabilities of a non-root worker (root gets its bounding set).
    Stopping tool processes of another user needs CAP_KILL.
    """
    fields = _status_fields(status_text)
    passed = _capabilities(fields, "CapBnd" if root else "CapAmb") or 0
    effective = _capabilities(fields, "CapEff") or 0
    problems = [
        f"the worker lacks {_CAP_NAMES[cap]} for the tool processes it starts"
        for cap in (_CAP_SETGID, _CAP_SETUID)
        if not passed & (1 << cap)
    ]
    if not effective & (1 << _CAP_KILL):
        problems.append("the worker lacks CAP_KILL to stop tool processes")
    if problems:
        problems.append(
            "start the worker container as root with cap_add SETUID, SETGID and KILL; "
            "the image entrypoint keeps them as ambient capabilities"
        )
    return problems


def _probe_paths() -> list[str]:
    """What a tool process must not read: the worker's environment and secret files."""
    from codeforge.secrets import SECRETS_DIR

    paths = [f"/proc/{os.getpid()}/environ"]
    if SECRETS_DIR.is_dir():
        paths.append(str(SECRETS_DIR))
        with contextlib.suppress(OSError):  # the worker cannot list it either (locked)
            paths.extend(str(entry) for entry in sorted(SECRETS_DIR.iterdir()))
    return paths


def _run_probe(argv: list[str], timeout: float) -> str:
    completed = subprocess.run(  # noqa: S603 - fixed launcher, shell and script
        argv,
        stdin=subprocess.DEVNULL,
        capture_output=True,
        text=True,
        cwd="/",
        env=_LAUNCHER_ENV,
        timeout=timeout,
        umask=TOOL_UMASK,
        check=False,
    )
    if completed.returncode != 0:
        raise OSError(f"exit code {completed.returncode}: {completed.stderr.strip()[:500]}")
    return completed.stdout


def probe_problems(output: str, config: IsolationConfig) -> list[str]:
    """What is wrong with a tool process, from the output of the isolation check's probe."""
    readable = [line[len(_READABLE_MARKER) :] for line in output.splitlines() if line.startswith(_READABLE_MARKER)]
    fields = _status_fields(output)
    if "Uid" not in fields or "Gid" not in fields:
        return ["reported no process status"]
    problems: list[str] = []
    if any(uid != str(config.uid) for uid in fields["Uid"].split()):
        problems.append(f"runs with uid {fields['Uid']}, expected {config.uid}")
    if any(gid != str(config.gid) for gid in fields["Gid"].split()):
        problems.append(f"runs with gid {fields['Gid']}, expected {config.gid}")
    groups = sorted(int(gid) for gid in fields.get("Groups", "").split())
    if groups != sorted(config.groups):
        problems.append(f"has the supplementary groups {groups}, expected {sorted(config.groups)}")
    held = [name for name in _TOOL_CAP_SETS if _capabilities(fields, name) != 0]
    if held:
        problems.append("holds capabilities (" + ", ".join(f"{n}={fields.get(n, '?')}" for n in held) + ")")
    if fields.get("NoNewPrivs") != "1":
        problems.append("runs without no_new_privs")
    if "Umask" in fields and int(fields["Umask"], 8) != TOOL_UMASK:
        problems.append(f"runs with umask {fields['Umask']}, expected {TOOL_UMASK:04o}")
    problems.extend(f"can read {path}" for path in readable)
    return problems


# ---------------------------------------------------------------------------
# Starting tool processes
# ---------------------------------------------------------------------------


def _launch(argv: Sequence[str], env: Mapping[str, str]) -> tuple[list[str], dict[str, str], int | None]:
    """The command line, process environment and umask of a tool process.

    Raises ToolIsolationError when the process must not start.
    """
    status = tool_isolation()
    if not status.config.required:
        return list(argv), dict(env), None
    if not status.ready:
        raise ToolIsolationError(
            "tool isolation is required (CODEFORGE_TOOL_ISOLATION=required) but tool processes "
            f"cannot run as the tool user: {status.reason}"
        )
    return status.command(argv, env), dict(_LAUNCHER_ENV), TOOL_UMASK


async def start_tool_process(
    program: str,
    *args: str,
    env: Mapping[str, str],
    cwd: str | None = None,
    stdin: int | None = None,
    stdout: int | None = None,
    stderr: int | None = None,
    start_new_session: bool = False,
    limit: int | None = None,
) -> asyncio.subprocess.Process:
    """Start *program* with *args* for an agent (asyncio.create_subprocess_exec).

    *env* is the whole environment of the process (``tool_env``). Raises
    ToolIsolationError, starting nothing, when isolation is required and not
    available.
    """
    argv, process_env, umask = _launch([program, *args], env)
    optional = {"stdin": stdin, "stdout": stdout, "stderr": stderr, "limit": limit, "umask": umask}
    options: dict[str, object] = {
        "env": process_env,
        "cwd": cwd,
        "start_new_session": start_new_session,
        **{name: value for name, value in optional.items() if value is not None},
    }
    return await asyncio.create_subprocess_exec(*argv, **options)  # type: ignore[arg-type]


async def start_tool_shell(
    command: str,
    *,
    env: Mapping[str, str],
    cwd: str | None = None,
    stdin: int | None = None,
    stdout: int | None = None,
    stderr: int | None = None,
    start_new_session: bool = False,
) -> asyncio.subprocess.Process:
    """Start a shell *command* for an agent, like asyncio.create_subprocess_shell (/bin/sh -c)."""
    return await start_tool_process(
        _SHELL,
        "-c",
        command,
        env=env,
        cwd=cwd,
        stdin=stdin,
        stdout=stdout,
        stderr=stderr,
        start_new_session=start_new_session,
    )


def run_tool_process(
    args: Sequence[str],
    *,
    env: Mapping[str, str],
    cwd: str | None = None,
    timeout: float | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run *args* for an agent and wait for it (subprocess.run, text output captured)."""
    argv, process_env, umask = _launch(args, env)
    options: dict[str, object] = {"cwd": cwd, "env": process_env, "timeout": timeout}
    if umask is not None:
        options["umask"] = umask
    return subprocess.run(argv, capture_output=True, text=True, check=False, **options)  # type: ignore[call-overload]  # noqa: S603 - the program is the caller's (no shell)


def tool_stdio_client(
    command: str,
    args: Sequence[str],
    *,
    declared_env: Mapping[str, str] | None,
    errlog: TextIO,
) -> AbstractAsyncContextManager[
    tuple[MemoryObjectReceiveStream[SessionMessage | Exception], MemoryObjectSendStream[SessionMessage]]
]:
    """Start an MCP stdio server for an agent (the MCP SDK's stdio_client), as the tool user.

    Its environment is ``tool_env`` plus the server's declared variables
    (``declared_tool_env``). The SDK starts the process in a session of its
    own; it inherits the worker's umask (002 with isolation). Raises
    ToolIsolationError, starting nothing, when isolation is required and not
    available.
    """
    from mcp import StdioServerParameters, stdio_client

    from codeforge.subprocess_env import declared_tool_env, tool_env

    argv, env, _ = _launch([command, *args], tool_env(extra=declared_tool_env(declared_env)))
    return stdio_client(StdioServerParameters(command=argv[0], args=argv[1:], env=env), errlog=errlog)


# ---------------------------------------------------------------------------
# Files and directories shared with tool processes
# ---------------------------------------------------------------------------


def share_with_tools(path: str, *, writable: bool) -> None:
    """Let tool processes use a file or directory the worker created for them.

    With isolation it moves *path* into the workspace group and opens it to
    the group (directories setgid, so what is created inside stays in the
    group); without isolation (or while no tool process can start) it does
    nothing.
    """
    status = tool_isolation()
    if not status.config.required or not status.ready or not status.config.groups:
        return
    os.chown(path, -1, status.config.workspace_gid)
    if stat.S_ISDIR(os.stat(path).st_mode):
        os.chmod(path, 0o2770 if writable else 0o2750)
    else:
        os.chmod(path, 0o660 if writable else 0o640)


def _shared_mode(mode: int) -> int:
    """*mode* opened to the group: read and write, search and setgid for directories, execute if the owner may."""
    shared = stat.S_IMODE(mode) | stat.S_IRGRP | stat.S_IWGRP
    if stat.S_ISDIR(mode):
        return shared | stat.S_IXGRP | stat.S_ISGID
    if mode & stat.S_IXUSR:
        shared |= stat.S_IXGRP
    return shared


def _share_entry(path: str, gid: int, uid: int) -> bool:
    """Move one entry of the worker's into group *gid* and open it to the group; True if it changed."""
    try:
        info = os.lstat(path)
        if stat.S_ISLNK(info.st_mode) or info.st_uid != uid:
            return False
        mode = _shared_mode(info.st_mode)
        if info.st_gid == gid and stat.S_IMODE(info.st_mode) == mode:
            return False
        os.chown(path, -1, gid, follow_symlinks=False)
        os.chmod(path, mode)
    except OSError as exc:
        logger.warning("cannot share %s with the tool user: %s", path, exc)
        return False
    return True


def share_workspace_root(root: str, gid: int) -> int:
    """Open the workspaces under *root* to the workspace group *gid*, once; return how many entries changed.

    Workspaces created before tool isolation belong to the worker user and
    its own group with mode 0644/0755, which the tool user cannot write. The
    worker owns them, so it moves them into the workspace group, makes them
    group-writable and directories setgid. The root is changed last: once it
    is shared, nothing is walked again. Symlinks and entries of other users
    are left alone.
    """
    try:
        info = os.stat(root)
    except FileNotFoundError:
        return 0
    if info.st_gid == gid and stat.S_IMODE(info.st_mode) & 0o2070 == 0o2070:
        return 0
    uid = os.getuid()
    changed = 0
    for dirpath, dirnames, filenames in os.walk(root):
        for name in (*dirnames, *filenames):
            changed += _share_entry(os.path.join(dirpath, name), gid, uid)
    changed += _share_entry(root, gid, uid)
    return changed
