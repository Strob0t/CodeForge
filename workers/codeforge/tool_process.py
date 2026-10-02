"""Start every process that runs for an agent, as the tool user (KI-71, KI-96).

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
sets the tool user's UID, GID and supplementary groups, clears the
inheritable and ambient capability sets and sets no_new_privs; it then
executes the launch helper (``codeforge/tool_exec.py``, with the base Python
interpreter and ``-I -S``), already as the tool user, and the helper executes
the command, which therefore runs without capabilities and cannot gain any.
setpriv itself still holds the worker's capabilities and the dynamic loader
honours LD_PRELOAD and friends for it (ambient capabilities do not set
AT_SECURE), so it runs with a fixed environment of its own.

The command's environment, its working directory and the directories to
create for it travel in a launch spec on a memfd the helper reads (KI-96):
never as arguments, which every process in the container can read in
``/proc/<pid>/cmdline``. The helper checks its own credentials against the
spec, enters the working directory without following a symlink, and exits
with 125, running nothing, when anything is wrong. Popen closes every other
file descriptor and sets the umask. CAP_KILL lets the worker stop tool
processes of another user (timeouts, cancels). MCP stdio servers start the
same way (tool_stdio_client).

``CODEFORGE_TOOL_ISOLATION`` selects the mode: ``required`` (the worker image,
docker-compose.prod.yml) starts tool processes only as the tool user and
fails the tool call with ToolIsolationError, starting nothing, when that is
not possible; ``off`` (the default elsewhere: development, tests, the
devcontainer) starts them as the worker user, as before. No other module
starts processes (tests/test_tool_process.py checks the sources; the helper
is the second half of the launcher).
"""

from __future__ import annotations

import asyncio
import contextlib
import errno
import json
import logging
import os
import secrets
import shutil
import stat
import subprocess
import sys
import threading
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import AsyncIterator, Callable, Coroutine, Mapping, Sequence
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
# The launcher's second half; runs as the tool user with the base interpreter.
TOOL_EXEC = str(Path(__file__).resolve().with_name("tool_exec.py"))
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


def base_interpreter() -> str:
    """The interpreter the worker's venv is built on: the helper runs without the venv (-I -S)."""
    return getattr(sys, "_base_executable", "") or sys.executable


@dataclass(frozen=True)
class Launch:
    """How to start one tool process: the arguments, environment and working directory for
    Popen, the descriptors it passes on (the launch spec) and closes afterwards, and the umask.
    """

    argv: list[str]
    env: dict[str, str]
    cwd: str | None
    umask: int | None = None
    pass_fds: tuple[int, ...] = ()
    close_after_spawn: tuple[int, ...] = field(default=())

    def close(self) -> None:
        """Close the worker's copy of the launch spec (after the spawn, or when it failed)."""
        for fd in self.close_after_spawn:
            with contextlib.suppress(OSError):
                os.close(fd)


def write_spec(spec: Mapping[str, object]) -> int:
    """Write *spec* to a new memfd (close-on-exec) and return it, positioned at its start."""
    data = json.dumps(spec, separators=(",", ":")).encode()
    fd = os.memfd_create("cf-tool-launch", os.MFD_CLOEXEC)
    try:
        view = memoryview(data)
        while view:
            written = os.write(fd, view)
            view = view[written:]
        os.lseek(fd, 0, os.SEEK_SET)
    except BaseException:
        os.close(fd)
        raise
    return fd


@dataclass(frozen=True)
class IsolationStatus:
    """Whether tool processes can start, and how.

    With isolation off they always can (as the worker user). With isolation
    required they can once the check passed (``ready``); ``reason`` says why
    it did not, ``launcher`` is the setpriv that starts them, ``interpreter``
    the Python that runs the launch helper.
    """

    config: IsolationConfig
    ready: bool
    reason: str = ""
    launcher: str = ""
    interpreter: str = ""

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

    def spec(self, env: Mapping[str, str], cwd: str | None) -> dict[str, object]:
        """The launch spec the helper reads: who it must run as, the environment, where."""
        return {
            "uid": self.config.uid,
            "gid": self.config.gid,
            "groups": list(self.config.groups),
            "umask": TOOL_UMASK,
            "env": {name: value for name, value in env.items() if name and "=" not in name},
            "landlock": "off",
            "prepare": [],
            "home": None,
            "cwd": os.path.abspath(cwd) if cwd is not None else None,
        }

    def launch(self, argv: Sequence[str], env: Mapping[str, str], cwd: str | None = None) -> Launch:
        """Run *argv* as the tool user with exactly the environment *env* in *cwd*.

        Nothing of *env* is an argument: it is in the spec on the memfd.
        """
        fd = write_spec(self.spec(env, cwd))
        argv = [*self.launch_prefix(), self.interpreter, "-I", "-S", TOOL_EXEC, str(fd), *argv]
        return Launch(
            argv=argv,
            env=dict(_LAUNCHER_ENV),
            cwd="/",
            umask=TOOL_UMASK,
            pass_fds=(fd,),
            close_after_spawn=(fd,),
        )


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
    if launcher is None:
        return _not_ready(config, "setpriv (util-linux) is not installed")
    interpreter = base_interpreter()
    if not os.path.isabs(interpreter) or not os.access(interpreter, os.X_OK) or not os.path.isfile(TOOL_EXEC):
        return _not_ready(config, f"the launch helper cannot run ({interpreter} {TOOL_EXEC})")
    if config.uid <= 0 or config.gid <= 0 or config.uid == os.getuid() or config.gid == os.getgid():
        return _not_ready(
            config,
            f"the tool user (uid {config.uid}, gid {config.gid}) must be an unprivileged user "
            f"that differs from the worker user (uid {os.getuid()}, gid {os.getgid()})",
        )
    problems = worker_capability_problems(_own_status(), root=_is_root())
    if problems:
        return _not_ready(config, "; ".join(problems))
    status = IsolationStatus(config=config, ready=True, launcher=launcher, interpreter=interpreter)
    try:
        launch = status.launch(
            [_SHELL, "-c", _PROBE_SCRIPT, "cf-isolation-check", *_probe_paths()], _LAUNCHER_ENV, cwd="/"
        )
        output = _run_probe(launch, _PROBE_TIMEOUT_SECONDS)
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


def _run_probe(launch: Launch, timeout: float) -> str:
    try:
        completed = subprocess.run(  # noqa: S603 - fixed launcher, shell and script
            launch.argv,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            text=True,
            cwd=launch.cwd,
            env=launch.env,
            timeout=timeout,
            umask=TOOL_UMASK,
            pass_fds=launch.pass_fds,
            check=False,
        )
    finally:
        launch.close()
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


def _launch(argv: Sequence[str], env: Mapping[str, str], cwd: str | None) -> Launch:
    """How to start a tool process. Raises ToolIsolationError when it must not start."""
    status = tool_isolation()
    if not status.config.required:
        return Launch(argv=list(argv), env=dict(env), cwd=cwd)
    if not status.ready:
        raise ToolIsolationError(
            "tool isolation is required (CODEFORGE_TOOL_ISOLATION=required) but tool processes "
            f"cannot run as the tool user: {status.reason}"
        )
    return status.launch(argv, env, cwd)


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
    launch = _launch([program, *args], env, cwd)
    isolated = launch.umask is not None
    pipes = _OpenPipes(stdin, stdout, stderr) if isolated else None
    optional = {"stdin": stdin, "stdout": stdout, "stderr": stderr, "limit": limit, "umask": launch.umask}
    if pipes is not None:
        optional.update(pipes.child_streams())
    options: dict[str, object] = {
        "env": launch.env,
        "cwd": launch.cwd,
        "start_new_session": start_new_session,
        **{name: value for name, value in optional.items() if value is not None},
    }
    if launch.pass_fds:
        options["pass_fds"] = launch.pass_fds
    try:
        proc = await asyncio.create_subprocess_exec(*launch.argv, **options)  # type: ignore[arg-type]
    except BaseException:
        if pipes is not None:
            pipes.close_parent_ends()
        raise
    finally:
        launch.close()
        if pipes is not None:
            pipes.close_child_ends()
    if pipes is not None:
        await pipes.attach(proc, limit)
    if isolated and cwd is not None:
        _share_after_exit(proc, cwd)
    return proc


# asyncio's default StreamReader limit.
_STREAM_LIMIT = 2**16


class _OpenPipes:
    """The stdio pipes of a tool process, made by the worker and opened to every user.

    A pipe's inode belongs to its creator with mode 0600. A tool process
    writes through the descriptors it inherits either way, but reopening
    them (/dev/stdout, /dev/stderr, /dev/fd/N, which logging configs, tee
    and shell redirections do) checks the inode's permission, and the tool
    user is not the worker (KI-96, E11). The pipes are made here and opened
    (0666) before the spawn; reopening still needs access to the holder's
    /proc/<pid>/fd, so no other user gains anything. asyncio gets the
    worker's ends as its streams, as with ``PIPE``.
    """

    def __init__(self, stdin: int | None, stdout: int | None, stderr: int | None) -> None:
        self._child: dict[str, int] = {}
        self._parent: dict[str, int] = {}
        try:
            if stdin == subprocess.PIPE:
                self._child["stdin"], self._parent["stdin"] = self._pipe()
            if stdout == subprocess.PIPE:
                self._parent["stdout"], self._child["stdout"] = self._pipe()
            if stderr == subprocess.PIPE:
                self._parent["stderr"], self._child["stderr"] = self._pipe()
            elif stderr == subprocess.STDOUT and "stdout" in self._child:
                self._child["stderr"] = self._child["stdout"]
        except BaseException:
            self.close_child_ends()
            self.close_parent_ends()
            raise

    @staticmethod
    def _pipe() -> tuple[int, int]:
        read_end, write_end = os.pipe2(os.O_CLOEXEC)
        try:
            for fd in (read_end, write_end):
                os.fchmod(fd, 0o666)
        except BaseException:
            os.close(read_end)
            os.close(write_end)
            raise
        return read_end, write_end

    def child_streams(self) -> dict[str, int]:
        return dict(self._child)

    def close_child_ends(self) -> None:
        for fd in set(self._child.values()):
            with contextlib.suppress(OSError):
                os.close(fd)
        self._child.clear()

    def close_parent_ends(self) -> None:
        for fd in self._parent.values():
            with contextlib.suppress(OSError):
                os.close(fd)
        self._parent.clear()

    async def attach(self, proc: asyncio.subprocess.Process, limit: int | None) -> None:
        """Give *proc* stream objects on the worker's ends, and a communicate() that uses them."""
        loop = asyncio.get_running_loop()
        try:
            if "stdin" in self._parent:
                fd = self._parent.pop("stdin")
                transport, protocol = await loop.connect_write_pipe(
                    lambda: asyncio.StreamReaderProtocol(asyncio.StreamReader()), os.fdopen(fd, "wb", buffering=0)
                )
                proc.stdin = asyncio.StreamWriter(transport, protocol, None, loop)
            for name in ("stdout", "stderr"):
                if name in self._parent:
                    fd = self._parent.pop(name)
                    reader = asyncio.StreamReader(limit=limit or _STREAM_LIMIT)
                    await loop.connect_read_pipe(
                        lambda reader=reader: asyncio.StreamReaderProtocol(reader), os.fdopen(fd, "rb", buffering=0)
                    )
                    setattr(proc, name, reader)
        finally:
            self.close_parent_ends()
        proc.communicate = _communicate_on(proc)  # type: ignore[method-assign]


def _communicate_on(
    proc: asyncio.subprocess.Process,
) -> Callable[[bytes | None], Coroutine[object, object, tuple[bytes | None, bytes | None]]]:
    """Process.communicate() for streams asyncio's transport does not own (_OpenPipes)."""

    async def feed(data: bytes | None) -> None:
        if proc.stdin is None:
            return
        try:
            if data:
                proc.stdin.write(data)
                await proc.stdin.drain()
        except (BrokenPipeError, ConnectionResetError):
            pass  # the process exited early; its exit code tells why
        finally:
            proc.stdin.close()

    async def read(stream: asyncio.StreamReader | None) -> bytes | None:
        return None if stream is None else await stream.read()

    async def communicate(input: bytes | None = None) -> tuple[bytes | None, bytes | None]:  # noqa: A002 - the asyncio signature
        _, out, err = await asyncio.gather(feed(input), read(proc.stdout), read(proc.stderr))
        await proc.wait()
        return out, err

    return communicate


def _share_after_exit(proc: asyncio.subprocess.Process, root: str) -> None:
    """Make waiting for *proc* (wait(), communicate()) also share what it created under *root*.

    The callers continue only once the files are shared, before the Go
    Core checkpoints or delivers the workspace.
    """
    wait = proc.wait
    shared = False

    async def wait_then_share() -> int:
        nonlocal shared
        code = await wait()
        if not shared:
            shared = True
            await share_tool_files(root)
        return code

    proc.wait = wait_then_share  # type: ignore[method-assign]


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
    launch = _launch(args, env, cwd)
    options: dict[str, object] = {"cwd": launch.cwd, "env": launch.env, "timeout": timeout}
    if launch.umask is not None:
        options["umask"] = launch.umask
    if launch.pass_fds:
        options["pass_fds"] = launch.pass_fds
    try:
        return subprocess.run(launch.argv, capture_output=True, text=True, check=False, **options)  # type: ignore[call-overload]  # noqa: S603 - the program is the caller's (no shell)
    finally:
        launch.close()
        if launch.umask is not None and cwd is not None:
            share_tool_files_sync(cwd)


def _share_command(root: str, config: IsolationConfig) -> list[str]:
    """One walk of *root* (never across file systems, never following a symlink): the files and
    directories the tool user owns move into the workspace group and become group-readable and
    -writable, directories also searchable and setgid. Only what needs a change is changed.
    """
    uid, gid = str(config.uid), str(config.workspace_gid)
    return [
        "find", "-P", root, "-xdev",
        "(", "-user", uid, "(", "-type", "d", "-o", "-type", "f", ")", "!", "-group", gid,
        "-exec", "chgrp", gid, "{}", "+", ")",
        ",",
        "(", "-user", uid, "-type", "d", "!", "-perm", "-2070", "-exec", "chmod", "g+rwxs", "{}", "+", ")",
        ",",
        "(", "-user", uid, "-type", "f", "!", "-perm", "-0060", "-exec", "chmod", "g+rw", "{}", "+", ")",
    ]  # fmt: skip


def _share_launch(root: str) -> Launch | None:
    status = tool_isolation()
    if not status.config.required or not status.ready:
        return None
    return _launch(_share_command(root, status.config), {"PATH": _PROBE_PATH}, None)


async def share_tool_files(root: str) -> None:
    """Share what the tool user created under *root* with the workspace group (KI-71 review).

    Agents create files with owner-only modes (mkdtemp, mkdir -m 0700,
    umask 077) that the worker and the Go Core (uid 10001, workspace group)
    could neither read nor delete: project deletion (GDPR erasure), git add
    of checkpoints and delivery, and benchmark cleanups failed. The pass
    runs as the tool user, which owns them; it changes nothing of anybody
    else's. A pass that could not share everything is logged.
    """
    launch = _share_launch(root)
    if launch is None:
        return
    try:
        proc = await asyncio.create_subprocess_exec(
            *launch.argv,
            env=launch.env,
            cwd=launch.cwd,
            umask=launch.umask,
            pass_fds=launch.pass_fds,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
        )
        _, err = await proc.communicate()
    except OSError as exc:
        logger.warning("could not share the tool files under %s: %s", root, exc)
        return
    finally:
        launch.close()
    if proc.returncode:
        logger.warning("could not share every tool file under %s: %s", root, err.decode(errors="replace")[-500:])


def share_tool_files_sync(root: str) -> None:
    """share_tool_files for synchronous callers."""
    launch = _share_launch(root)
    if launch is None:
        return
    try:
        done = subprocess.run(  # noqa: S603 - fixed program
            launch.argv,
            env=launch.env,
            cwd=launch.cwd,
            umask=launch.umask,
            pass_fds=launch.pass_fds,
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError as exc:
        logger.warning("could not share the tool files under %s: %s", root, exc)
        return
    finally:
        launch.close()
    if done.returncode:
        logger.warning("could not share every tool file under %s: %s", root, done.stderr[-500:])


# How long an MCP stdio server may take to exit after its stdin closed (the SDK's value).
_STDIO_EXIT_SECONDS = 2.0


@contextlib.asynccontextmanager
async def tool_stdio_client(
    command: str,
    args: Sequence[str],
    *,
    declared_env: Mapping[str, str] | None,
    errlog: TextIO,
    cwd: str | None = None,
) -> AsyncIterator[
    tuple[MemoryObjectReceiveStream[SessionMessage | Exception], MemoryObjectSendStream[SessionMessage]]
]:
    """Start an MCP stdio server for an agent, as the tool user; yield the MCP SDK's streams.

    The MCP SDK's stdio_client cannot pass the launch spec's descriptor, so
    the server starts through start_tool_process like every tool process (in
    a session of its own) and this speaks the stdio transport (one JSON-RPC
    message per line) as the SDK does. Its environment is ``tool_env`` plus
    the server's declared variables (``declared_tool_env``). Raises
    ToolIsolationError, starting nothing, when isolation is required and not
    available.
    """
    import anyio
    from mcp.shared.message import SessionMessage

    from codeforge.subprocess_env import declared_tool_env, tool_env
    from codeforge.subprocess_utils import terminate_process_group

    proc = await start_tool_process(
        command,
        *args,
        env=tool_env(extra=declared_tool_env(declared_env)),
        cwd=cwd,
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=errlog.fileno(),
        start_new_session=True,
    )
    read_writer, read_stream = anyio.create_memory_object_stream[SessionMessage | Exception](0)
    write_stream, write_reader = anyio.create_memory_object_stream[SessionMessage](0)
    try:
        async with anyio.create_task_group() as tg:
            tg.start_soon(_stdio_read, proc.stdout, read_writer)
            tg.start_soon(_stdio_write, proc.stdin, write_reader)
            try:
                yield read_stream, write_stream
            finally:
                # The MCP stdio shutdown: close the server's input, give it time to exit, then stop it.
                if proc.stdin is not None:
                    proc.stdin.close()
                try:
                    await asyncio.wait_for(proc.wait(), timeout=_STDIO_EXIT_SECONDS)
                except TimeoutError:
                    await terminate_process_group(proc)
                tg.cancel_scope.cancel()
    finally:
        for stream in (read_stream, write_stream, read_writer, write_reader):
            await stream.aclose()
        if proc.returncode is None:
            await terminate_process_group(proc)


async def _stdio_read(
    stdout: asyncio.StreamReader | None, messages: MemoryObjectSendStream[SessionMessage | Exception]
) -> None:
    """Hand every line the MCP server writes to the session as a message (or the parse error)."""
    import anyio
    from mcp import types
    from mcp.shared.message import SessionMessage

    if stdout is None:
        return
    async with messages:
        try:
            while line := await stdout.readline():
                text = line.decode(errors="replace").strip()
                if not text:
                    continue
                try:
                    message = types.JSONRPCMessage.model_validate_json(text)
                except ValueError as exc:
                    await messages.send(exc)
                    continue
                await messages.send(SessionMessage(message))
        except anyio.ClosedResourceError:
            await anyio.lowlevel.checkpoint()


async def _stdio_write(stdin: asyncio.StreamWriter | None, messages: MemoryObjectReceiveStream[SessionMessage]) -> None:
    """Write every message of the session to the MCP server, one JSON line each."""
    import anyio

    if stdin is None:
        return
    async with messages:
        try:
            async for session_message in messages:
                data = session_message.message.model_dump_json(by_alias=True, exclude_none=True)
                stdin.write((data + "\n").encode())
                await stdin.drain()
        except (anyio.ClosedResourceError, BrokenPipeError, ConnectionResetError):
            await anyio.lowlevel.checkpoint()


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


def _share_entry(name: str, gid: int, uid: int, dir_fd: int | None = None) -> bool:
    """Move one entry of the worker's into group *gid* and open it to the group; True if it changed.

    The entry (*name*, relative to *dir_fd*) is changed only through a
    descriptor opened without following a symlink, after checking it is the
    regular file or directory the walk saw: a tool process may swap an entry
    for a symlink while the walk runs, and a change by path would follow it
    onto the worker's files outside the workspaces. Anything else (symlinks,
    FIFOs, devices, other users' entries) is left alone.
    """
    try:
        seen = os.stat(name, dir_fd=dir_fd, follow_symlinks=False)
    except OSError as exc:
        logger.warning("cannot share %s with the tool user: %s", name, exc)
        return False
    if not (stat.S_ISREG(seen.st_mode) or stat.S_ISDIR(seen.st_mode)) or seen.st_uid != uid:
        return False
    try:
        fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=dir_fd)
    except OSError as exc:
        if exc.errno != errno.ELOOP:  # ELOOP: swapped for a symlink meanwhile, left alone
            logger.warning("cannot share %s with the tool user: %s", name, exc)
        return False
    try:
        info = os.fstat(fd)
        if (info.st_dev, info.st_ino) != (seen.st_dev, seen.st_ino) or info.st_uid != uid:
            return False  # swapped for another entry meanwhile
        mode = _shared_mode(info.st_mode)
        if info.st_gid == gid and stat.S_IMODE(info.st_mode) == mode:
            return False
        os.fchown(fd, -1, gid)
        os.fchmod(fd, mode)
    except OSError as exc:
        logger.warning("cannot share %s with the tool user: %s", name, exc)
        return False
    finally:
        os.close(fd)
    return True


# Bumped when the walk over the workspaces must run again after an upgrade:
# 1 opened the worker's files to the workspace group (KI-71), 2 also shares
# the files the tool user kept private (share_tool_files).
WORKSPACE_SHARING_VERSION = "2"
_SHARING_STAMP = ".codeforge-workspace-sharing"


def share_workspace_root(root: str, gid: int) -> int:
    """Open the workspaces under *root* to the workspace group *gid*; return how many entries changed.

    Workspaces created before tool isolation belong to the worker user and
    its own group with mode 0644/0755, which the tool user cannot write. The
    worker owns them, so it moves them into the workspace group, makes them
    group-writable and directories setgid; files the tool user kept private
    are shared as the tool user. Symlinks and entries of other users are
    left alone. The walk runs once per WORKSPACE_SHARING_VERSION: a stamp in
    the root records it. A root of another user is not walked (the worker
    could not finish it and would walk it on every start): that is logged
    once, with the fix.
    """
    try:
        info = os.stat(root)
    except FileNotFoundError:
        return 0
    uid = os.getuid()
    if info.st_uid != uid:
        logger.error(
            "workspace root %s belongs to uid %d, not the worker (uid %d): workspaces created before "
            "tool isolation stay closed to agent tools. Fix: chown %d:%d %s && chmod 2775 %s; "
            "the worker shares the workspaces on its next start",
            root, info.st_uid, uid, uid, gid, root, root,
        )  # fmt: skip
        return 0
    if _read_stamp(root) == WORKSPACE_SHARING_VERSION:
        return 0
    changed = 0
    # Relative to directory descriptors, never following a symlink: a tool
    # process may change the tree while it is walked (_share_entry).
    for dirpath, dirnames, filenames, dir_fd in os.fwalk(root, follow_symlinks=False):
        for name in (*dirnames, *filenames):
            if dirpath == root and name.startswith(_SHARING_STAMP):
                continue
            changed += _share_entry(name, gid, uid, dir_fd=dir_fd)
    changed += _share_entry(root, gid, uid)
    share_tool_files_sync(root)
    _write_stamp(root)
    return changed


# A stamp holds a short version; anything longer is not one.
_STAMP_MAX_BYTES = 64


def _read_stamp(root: str) -> str:
    """The version the stamp records; "" when there is no valid stamp, and the walk runs.

    The tool user may write the root and plant anything at the stamp's
    place: it is opened without following a symlink and without blocking (a
    FIFO), must be a regular file of at most _STAMP_MAX_BYTES bytes, and is
    read as bytes. Anything else is logged and ignored, never raised: a
    worker that failed here would crash-loop at its start.
    """
    path = os.path.join(root, _SHARING_STAMP)
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    except FileNotFoundError:
        return ""
    except OSError as exc:
        logger.warning("ignoring the workspace sharing stamp %s: %s", path, exc)
        return ""
    try:
        if not stat.S_ISREG(os.fstat(fd).st_mode):
            logger.warning("ignoring the workspace sharing stamp %s: not a regular file", path)
            return ""
        data = os.read(fd, _STAMP_MAX_BYTES + 1)
    except OSError as exc:
        logger.warning("ignoring the workspace sharing stamp %s: %s", path, exc)
        return ""
    finally:
        os.close(fd)
    if len(data) > _STAMP_MAX_BYTES:
        logger.warning("ignoring the workspace sharing stamp %s: longer than %d bytes", path, _STAMP_MAX_BYTES)
        return ""
    try:
        return data.decode("ascii").strip()
    except UnicodeDecodeError:
        logger.warning("ignoring the workspace sharing stamp %s: not a version", path)
        return ""


def _write_stamp(root: str) -> None:
    """Record the walk: a new file renamed over the stamp's place, so whatever the tool user
    planted there (a symlink, a FIFO) is replaced, never written through or waited on.
    """
    path = os.path.join(root, _SHARING_STAMP)
    temporary = f"{path}.{secrets.token_hex(8)}"
    try:
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o664)
        try:
            os.write(fd, (WORKSPACE_SHARING_VERSION + "\n").encode())
        finally:
            os.close(fd)
        os.replace(temporary, path)
    except OSError as exc:
        logger.warning("cannot record the workspace sharing in %s (walked again on the next start): %s", root, exc)
        with contextlib.suppress(OSError):
            os.unlink(temporary)
