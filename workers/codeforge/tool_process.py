"""Start every process that runs for an agent, as the tenant's tool user (KI-71, KI-96, ADR-018).

Agent tools run commands an LLM chose or code it wrote: Bash, grep, git in the
workspace, quality gates and workspace tests, benchmark test commands, the
agent CLIs of ``tasks.agent.*``, the Claude Code CLI and its policy hook, MCP
stdio servers. They get a scrubbed environment
(``codeforge.subprocess_env.tool_env``), and with tool isolation they run as
the tool UID of their tenant (codeforge.tool_identity: 20000-29999, no
supplementary group): they cannot read the worker's secret files or
``/proc/<worker pid>/environ``, cannot signal or trace the worker or another
tenant's tools, reach no other tenant's files (POSIX ACLs,
codeforge.tool_state) and have no capabilities.

Mechanism: the worker container starts as root with only CAP_SETUID,
CAP_SETGID and CAP_KILL; its entrypoint (``scripts/worker-entrypoint.sh``) runs
the worker as the worker user (uid 10001) and keeps those three as ambient
capabilities. Every tool process starts through setpriv (util-linux), which
sets the tool UID and GID, clears the supplementary groups and the
inheritable and ambient capability sets and sets no_new_privs; it then
executes the launch helper (``codeforge/tool_exec.py``, with the base Python
interpreter and ``-I -S``), already as the tool user, and the helper executes
the command, which therefore runs without capabilities and cannot gain any.
setpriv itself still holds the worker's capabilities and the dynamic loader
honours LD_PRELOAD and friends for it (ambient capabilities do not set
AT_SECURE), so it runs with a fixed environment of its own.

The command's environment, its working directory and the per-work
directories to create below its HOME travel in a launch spec on a memfd the
helper reads: never as arguments, which every process in the container can
read in ``/proc/<pid>/cmdline``. The helper checks its own credentials
against the spec, creates the directories and enters the working directory
without following a symlink, and exits with 125, running nothing, when
anything is wrong. The worker never changes into a directory a tool can
write. Popen closes every other file descriptor and sets the umask (007).
CAP_KILL lets the worker stop tool processes of another user (timeouts,
cancels).

``CODEFORGE_TOOL_ISOLATION`` selects the mode: ``required`` (the worker image,
docker-compose.prod.yml) starts tool processes only as a tool identity and
fails the tool call with ToolIsolationError, starting nothing, when that is
not possible; ``off`` (the default elsewhere: development, tests, the
devcontainer) starts them as the worker user, as before. No other module
starts processes (tests/test_tool_process.py checks the sources; the helper
is the second half of the launcher).
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import os
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING

from codeforge import landlock as landlock_rules
from codeforge import posix_acl
from codeforge.tool_identity import (
    DEFAULT_HOME_BASE,
    DEFAULT_TOOL_PATH,
    SYSTEM_TOOL_UID,
    TENANT_TOOL_UMASK,
    WORKSPACE_GID,
    ToolIdentity,
    ToolIsolationError,
    current_identity,
    in_tenant_area,
    new_work_id,
    verify_tenant_dir,
)

if TYPE_CHECKING:
    from collections.abc import AsyncIterator, Callable, Coroutine, Mapping, Sequence
    from typing import TextIO

    from anyio.streams.memory import MemoryObjectReceiveStream, MemoryObjectSendStream
    from mcp.shared.message import SessionMessage

    from codeforge.config import WorkerSettings

logger = logging.getLogger(__name__)

__all__ = ["ToolIsolationError"]

ISOLATION_REQUIRED = "required"
ISOLATION_OFF = "off"
# Files tool processes create outside default ACLs stay their tenant's own.
TOOL_UMASK = TENANT_TOOL_UMASK

_LAUNCHER = "setpriv"
# The launcher's second half; runs as the tool user with the base interpreter.
TOOL_EXEC = str(Path(__file__).resolve().with_name("tool_exec.py"))
# The sharing pass, run as the tenant's tool UID (KI-96 D8).
TOOL_WALK = str(Path(__file__).resolve().with_name("tool_walk.py"))
# What the walker reads as a program (Landlock read rules of its launches).
WALKER_FILES = (TOOL_WALK, str(Path(TOOL_WALK).with_name("posix_acl.py")))
_SHELL = "/bin/sh"
_PROBE_TIMEOUT_SECONDS = 10.0
_PROBE_PATH = "/usr/local/bin:/usr/bin:/bin"
_READABLE_MARKER = "cf-readable "
_WRITABLE_MARKER = "cf-writable "
_OK_MARKER = "cf-ok "
# The programs the probe runs with the real tool environment when the tool
# PATH has them: a PATH whose python3 cannot run under Landlock (the worker's
# venv, E15) fails the check instead of every tool call.
_PROBE_PROGRAMS = {"python3": "python3 -c pass", "git": "git --version"}
# Run as the system tool user by the isolation check: the process status of
# the tool process (the shell's own: Landlock lets only the exec target read
# its /proc entry), what it must be able to do, then every path it can read
# (and must not).
_PROBE_SCRIPT = f"""cat /proc/$$/status
touch "$HOME/.cf-probe" && rm -f "$HOME/.cf-probe" && echo "{_OK_MARKER}home"
touch "$TMPDIR/.cf-probe" && rm -f "$TMPDIR/.cf-probe" && echo "{_OK_MARKER}tmp"
for program in $CF_PROBE_PROGRAMS; do
  case "$program" in
    python3) python3 -c pass >/dev/null 2>&1 && echo "{_OK_MARKER}python3" ;;
    git) git --version >/dev/null 2>&1 && echo "{_OK_MARKER}git" ;;
  esac
done
for path in "$@"; do
  if [ -d "$path" ]; then
    ls -- "$path" >/dev/null 2>&1 && echo "{_READABLE_MARKER}$path"
  elif cat -- "$path" >/dev/null 2>&1; then
    echo "{_READABLE_MARKER}$path"
  fi
done
if [ -n "$CF_PROBE_TMP" ] && touch "$CF_PROBE_TMP" 2>/dev/null; then
  rm -f "$CF_PROBE_TMP"
  echo "{_WRITABLE_MARKER}$CF_PROBE_TMP"
fi
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
    """How tool processes are isolated.

    ``workspace_root`` holds the tenant directories (``<root>/<tenant>``) and
    the worker's state; ``home_base`` the tool HOMEs (a volume);
    ``tool_path`` is the PATH tool processes get (never the worker's).
    """

    mode: str
    workspace_root: str = ""
    home_base: str = DEFAULT_HOME_BASE
    tool_path: str = DEFAULT_TOOL_PATH
    workspace_gid: int = WORKSPACE_GID
    # Landlock per tool call (codeforge.landlock): "required" or "off".
    landlock: str = landlock_rules.LANDLOCK_REQUIRED
    landlock_min_abi: int = landlock_rules.DEFAULT_MIN_ABI
    read_paths: tuple[str, ...] = ()
    production: bool = False
    # A tenant's HOME cache larger than this is removed when the tenant goes idle (D7, D10).
    cache_max_mb: int = 4096

    @property
    def required(self) -> bool:
        return self.mode == ISOLATION_REQUIRED

    @property
    def confined(self) -> bool:
        return self.required and self.landlock != landlock_rules.LANDLOCK_OFF

    @classmethod
    def from_settings(cls, settings: WorkerSettings) -> IsolationConfig:
        mode = parse_isolation_mode(settings.tool_isolation)
        return cls(
            mode=mode,
            workspace_root=os.path.normpath(settings.workspace_root) if settings.workspace_root else "",
            home_base=settings.tool_home_base,
            tool_path=settings.tool_path,
            workspace_gid=settings.workspace_gid,
            landlock=landlock_rules.parse_mode(settings.tool_landlock, mode),
            landlock_min_abi=settings.tool_landlock_min_abi,
            read_paths=landlock_rules.parse_read_paths(settings.tool_read_paths),
            production=settings.app_env.strip().lower() == "production",
            cache_max_mb=settings.tool_cache_max_mb,
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
    # The kernel's Landlock ABI tool processes are confined with (0: not confined).
    landlock_abi: int = 0

    def launch_prefix(self, identity: ToolIdentity) -> list[str]:
        """The setpriv command line that runs the command after it as *identity* (tenant UIDs: in no group)."""
        groups = f"--groups={','.join(map(str, identity.groups))}" if identity.groups else "--clear-groups"
        return [
            self.launcher,
            f"--reuid={identity.uid}",
            f"--regid={identity.gid}",
            groups,
            "--inh-caps=-all",
            "--ambient-caps=-all",
            "--no-new-privs",
            "--",
        ]

    def spec(
        self, env: Mapping[str, str], cwd: str | None, identity: ToolIdentity, files: tuple[str, ...] = ()
    ) -> dict[str, object]:
        """The launch spec the helper reads: who it must run as, the environment, where, what to prepare,
        what Landlock lets it use (*files*: programs it reads, as the walker)."""
        return {
            "uid": identity.uid,
            "gid": identity.gid,
            "groups": list(identity.groups),
            "umask": TOOL_UMASK,
            "env": {name: value for name, value in env.items() if name and "=" not in name},
            "landlock": landlock_rules.spec_value(
                self.config.landlock,
                identity,
                min_abi=self.config.landlock_min_abi,
                interpreter_prefix=sys.base_prefix,
                read_paths=self.config.read_paths,
                files=files,
            ),
            "prepare": identity.prepare(),
            "home": identity.home or None,
            "cwd": os.path.normpath(cwd) if cwd is not None else None,
        }

    def launch(
        self,
        argv: Sequence[str],
        env: Mapping[str, str],
        cwd: str | None,
        identity: ToolIdentity,
        files: tuple[str, ...] = (),
    ) -> Launch:
        """Run *argv* as *identity* with exactly the environment *env* in *cwd*.

        Nothing of *env* is an argument: it is in the spec on the memfd.
        """
        fd = write_spec(self.spec(env, cwd, identity, files))
        argv = [*self.launch_prefix(identity), self.interpreter, "-I", "-S", TOOL_EXEC, str(fd), *argv]
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


# ---------------------------------------------------------------------------
# The isolation check
# ---------------------------------------------------------------------------


def check_tool_isolation(config: IsolationConfig) -> IsolationStatus:
    """Return whether tool processes can start as *config* says.

    Required isolation checks the volumes (the workspace root is the
    worker's with mode 2771, its state directory, the HOME base volume, POSIX
    ACL support on both), the Landlock settings and the kernel's ABI, and
    starts a tool process as the system tool user, confined as every tool
    process is: it must run as uid and gid 19999 without any group or
    capability, with no_new_privs and umask 007, write its HOME and TMPDIR,
    run the tool PATH's python3 and git, and must not be able to read the
    worker's environment, command line or secret files, list /dev/shm, read
    the canary or create a file in /tmp. With ABI 6 or later a second tool
    process must not be able to signal the first one.
    """
    if not config.required:
        return IsolationStatus(config=config, ready=True)
    problem = _landlock_problem(config)
    if problem:
        return _not_ready(config, problem)
    launcher = shutil.which(_LAUNCHER, path=_PROBE_PATH)
    if launcher is None:
        return _not_ready(config, "setpriv (util-linux) is not installed")
    interpreter = base_interpreter()
    if not os.path.isabs(interpreter) or not os.access(interpreter, os.X_OK) or not os.path.isfile(TOOL_EXEC):
        return _not_ready(config, f"the launch helper cannot run ({interpreter} {TOOL_EXEC})")
    problems = worker_capability_problems(_own_status(), root=_is_root())
    if problems:
        return _not_ready(config, "; ".join(problems))
    problems = volume_problems(config)
    if problems:
        return _not_ready(config, "; ".join(problems))
    abi = landlock_rules.kernel_abi() if config.confined else 0
    status = IsolationStatus(config=config, ready=True, launcher=launcher, interpreter=interpreter, landlock_abi=abi)
    programs = tuple(name for name in _PROBE_PROGRAMS if shutil.which(name, path=config.tool_path))
    try:
        from codeforge.tool_identity import identity_env, tenant_home

        identity = ToolIdentity(
            tenant_id="",
            uid=SYSTEM_TOOL_UID,
            home=tenant_home(config.home_base, SYSTEM_TOOL_UID),
            work_id=new_work_id(),
        )
        env = {**identity_env(identity, config.tool_path), "CF_PROBE_PROGRAMS": " ".join(programs)}
        if config.confined:
            env["CF_PROBE_TMP"] = f"/tmp/cf-probe-{new_work_id()}"  # noqa: S108 - must not be creatable
        launch = status.launch(
            [_SHELL, "-c", _PROBE_SCRIPT, "cf-isolation-check", *_probe_paths(config)], env, "/", identity
        )
        try:
            output = _run_probe(launch, _PROBE_TIMEOUT_SECONDS)
        finally:
            _remove_probe_tmp(identity, status)
    except (OSError, subprocess.SubprocessError) as exc:
        return _not_ready(config, f"the isolation check could not start a tool process: {exc}")
    problems = probe_problems(output, SYSTEM_TOOL_UID, ("home", "tmp", *programs))
    if problems:
        return _not_ready(config, "a tool process " + "; ".join(problems))
    if abi >= _ABI_SCOPES:
        problem = _scope_problem(status, identity)
        if problem:
            return _not_ready(config, problem)
    return status


# Landlock scopes signals and abstract unix sockets from ABI 6 on.
_ABI_SCOPES = 6


def _remove_probe_tmp(identity: ToolIdentity, status: IsolationStatus) -> None:
    remove_as_tool_sync([identity.tmpdir], identity, confine=identity.home, status=status)


def _landlock_problem(config: IsolationConfig) -> str:
    """Why the Landlock settings or the kernel keep tool processes from being confined; "" if they do not."""
    if not config.confined:
        if config.production:
            return (
                "CODEFORGE_TOOL_LANDLOCK=off is not allowed with APP_ENV=production: tool command lines and "
                "/proc would be readable across tenants"
            )
        return ""
    abi = landlock_rules.kernel_abi()
    if abi < config.landlock_min_abi:
        found = f"Landlock ABI {abi}" if abi else "no Landlock (kernel, lsm= list or a seccomp profile)"
        return (
            f"{found} found, CODEFORGE_TOOL_LANDLOCK_MIN_ABI is {config.landlock_min_abi}: use a kernel with "
            "Landlock enabled (5.19 or later) and Docker 23 or later"
        )
    problems = landlock_rules.read_path_problems(config.read_paths)
    return "; ".join(problems)


def _scope_problem(status: IsolationStatus, identity: ToolIdentity) -> str:
    """With scopes, a tool process must not signal another one of its UID (a sibling Landlock domain)."""
    sleeper_launch = status.launch(["sleep", "30"], {"PATH": _PROBE_PATH}, "/", identity)
    try:
        sleeper = subprocess.Popen(  # noqa: S603 - fixed launcher and program
            sleeper_launch.argv,
            env=sleeper_launch.env,
            cwd=sleeper_launch.cwd,
            umask=TOOL_UMASK,
            pass_fds=sleeper_launch.pass_fds,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    finally:
        sleeper_launch.close()
    try:
        script = f"if kill -0 {sleeper.pid} 2>/dev/null; then echo allowed; else echo denied; fi"
        output = _run_probe(status.launch([_SHELL, "-c", script], {"PATH": _PROBE_PATH}, "/", identity), 10.0)
    except (OSError, subprocess.SubprocessError) as exc:
        return f"the isolation check could not start a tool process: {exc}"
    finally:
        sleeper.kill()  # the worker's own child, started above
        sleeper.wait()
    if output.strip() != "denied":
        return "a tool process can signal a tool process of another Landlock domain: signals are not scoped"
    return ""


def volume_problems(config: IsolationConfig) -> list[str]:
    """What keeps the volumes from isolating tenants: the root, the state directory, the HOMEs, ACLs."""
    from codeforge import tool_migration, tool_state

    if not config.workspace_root or not os.path.isabs(config.workspace_root):
        return ["tool isolation needs CODEFORGE_WORKSPACE_ROOT (an absolute path, the Go Core's workspace root)"]
    if not os.path.isabs(config.home_base):
        return [f"the tool HOME base {config.home_base!r} must be an absolute path"]
    try:
        problems = tool_state.root_problems(config.workspace_root, fix=True)
        if problems:
            return problems
        tool_migration.detect_rollback(config.workspace_root)
        tool_state.ensure_state_dirs(config.workspace_root)
        for problem in tool_migration.refused_tenant_dirs(config.workspace_root):
            logger.error(problem)
        problems = tool_state.home_base_problems(config.home_base)
        for directory in (config.workspace_root, config.home_base):
            if problem := tool_state.acl_support_problem(directory):
                problems.append(problem)
        return problems
    except (OSError, ToolIsolationError) as exc:
        return [str(exc)]


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


def _probe_paths(config: IsolationConfig) -> list[str]:
    """What a tool process must not read: the worker's environment and secret files; when
    confined also the worker's command line (readable for every UID by DAC), the /dev/shm
    listing and the canary."""
    from codeforge.secrets import SECRETS_DIR

    paths = [f"/proc/{os.getpid()}/environ"]
    if config.confined:
        paths += [f"/proc/{os.getpid()}/cmdline", "/dev/shm"]  # noqa: S108 - must not be listable
        if os.path.exists(landlock_rules.CANARY):
            paths.append(landlock_rules.CANARY)
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


def probe_problems(output: str, uid: int, expected: Sequence[str] = ()) -> list[str]:
    """What is wrong with a tool process of *uid*, from the output of the isolation check's probe.

    *expected* names what it must have managed (``cf-ok <name>``: its HOME,
    its TMPDIR, the tool PATH's programs).
    """
    lines = output.splitlines()
    readable = [line[len(_READABLE_MARKER) :] for line in lines if line.startswith(_READABLE_MARKER)]
    writable = [line[len(_WRITABLE_MARKER) :] for line in lines if line.startswith(_WRITABLE_MARKER)]
    done = {line[len(_OK_MARKER) :].strip() for line in lines if line.startswith(_OK_MARKER)}
    fields = _status_fields(output)
    if "Uid" not in fields or "Gid" not in fields:
        return ["reported no process status"]
    problems: list[str] = []
    if any(value != str(uid) for value in fields["Uid"].split()):
        problems.append(f"runs with uid {fields['Uid']}, expected {uid}")
    if any(value != str(uid) for value in fields["Gid"].split()):
        problems.append(f"runs with gid {fields['Gid']}, expected {uid}")
    groups = sorted(int(gid) for gid in fields.get("Groups", "").split())
    if groups:
        problems.append(f"has the supplementary groups {groups}, expected none")
    held = [name for name in _TOOL_CAP_SETS if _capabilities(fields, name) != 0]
    if held:
        problems.append("holds capabilities (" + ", ".join(f"{n}={fields.get(n, '?')}" for n in held) + ")")
    if fields.get("NoNewPrivs") != "1":
        problems.append("runs without no_new_privs")
    if "Umask" in fields and int(fields["Umask"], 8) != TOOL_UMASK:
        problems.append(f"runs with umask {fields['Umask']}, expected {TOOL_UMASK:04o}")
    problems.extend(f"can read {path}" for path in readable)
    problems.extend(f"can create {path}" for path in writable)
    problems.extend(_PROBE_FAILURES.get(name, f"cannot {name}") for name in expected if name not in done)
    return problems


_PROBE_FAILURES = {
    "home": "cannot write its home directory (HOME)",
    "tmp": "cannot write its tmp directory (TMPDIR)",
    "python3": "cannot run python3 from the tool PATH (CODEFORGE_TOOL_PATH; a venv interpreter cannot run confined)",
    "git": "cannot run git from the tool PATH (CODEFORGE_TOOL_PATH)",
}


# ---------------------------------------------------------------------------
# Starting tool processes
# ---------------------------------------------------------------------------


def _inside(path: str, directory: str) -> bool:
    return path == directory or path.startswith(directory.rstrip("/") + "/")


def _launch(
    argv: Sequence[str], env: Mapping[str, str], cwd: str | None, identity: ToolIdentity | None = None
) -> Launch:
    """How to start a tool process. Raises ToolIsolationError when it must not start.

    With isolation required the process runs as *identity*, else as the
    current tool identity (codeforge.tool_identity.tool_tenant); a process
    without one does not start. Its tenant directory is verified again, and
    its working directory must lie in its workspace or HOME.
    """
    status = tool_isolation()
    if not status.config.required:
        return Launch(argv=list(argv), env=dict(env), cwd=cwd)
    if not status.ready:
        raise ToolIsolationError(
            "tool isolation is required (CODEFORGE_TOOL_ISOLATION=required) but tool processes "
            f"cannot run as a tool user: {status.reason}"
        )
    identity = identity or current_identity.get()
    if identity is None:
        raise ToolIsolationError("a tool process without a tenant's tool identity: refused (isolation is required)")
    if not identity.is_system and in_tenant_area(status.config.workspace_root, identity.workspace):
        try:
            verify_tenant_dir(status.config.workspace_root, identity.tenant_id, identity.uid)
        except OSError as exc:
            raise ToolIsolationError(f"tool process of tenant {identity.tenant_id} refused: {exc}") from exc
    if cwd is not None:
        cwd = os.path.normpath(os.path.join("/", cwd))
        allowed = [d for d in (identity.workspace, identity.home, *identity.write_paths) if d]
        if not identity.is_system and not any(_inside(cwd, d) for d in allowed):
            raise ToolIsolationError(
                f"tool process of tenant {identity.tenant_id} (tool uid {identity.uid}): working directory "
                f"{cwd} is outside its workspace"
            )
    return status.launch(argv, env, cwd, identity)


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
    identity: ToolIdentity | None = None,
) -> asyncio.subprocess.Process:
    """Start *program* with *args* for an agent (asyncio.create_subprocess_exec).

    *env* is the whole environment of the process (``tool_env``). It runs as
    the current tool identity; *identity* overrides it (only the isolation
    probe and the system's own calls). Raises ToolIsolationError, starting
    nothing, when isolation is required and not available.
    """
    identity = identity or current_identity.get()
    launch = _launch([program, *args], env, cwd, identity)
    isolated = launch.umask is not None
    started = time.time()
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
    if isolated and identity is not None and identity.workspace:
        _share_after_exit(proc, identity, started)
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


def _share_after_exit(proc: asyncio.subprocess.Process, identity: ToolIdentity, started: float) -> None:
    """Make waiting for *proc* (wait(), communicate()) also share what it changed in its workspace.

    The callers continue only once the files are shared, before the Go
    Core checkpoints or delivers the workspace. Only entries changed since
    the process started are checked.
    """
    wait = proc.wait
    shared = False

    async def wait_then_share() -> int:
        nonlocal shared
        code = await wait()
        if not shared:
            shared = True
            await share_tool_files(identity.workspace or "", identity, since=started)
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
    identity: ToolIdentity | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run *args* for an agent and wait for it (subprocess.run, text output captured)."""
    identity = identity or current_identity.get()
    launch = _launch(args, env, cwd, identity)
    started = time.time()
    options: dict[str, object] = {"cwd": launch.cwd, "env": launch.env, "timeout": timeout}
    if launch.umask is not None:
        options["umask"] = launch.umask
    if launch.pass_fds:
        options["pass_fds"] = launch.pass_fds
    try:
        return subprocess.run(launch.argv, capture_output=True, text=True, check=False, **options)  # type: ignore[call-overload]  # noqa: S603 - the program is the caller's (no shell)
    finally:
        launch.close()
        if launch.umask is not None and identity is not None and identity.workspace:
            share_tool_files_sync(identity.workspace, identity, since=started)


# ---------------------------------------------------------------------------
# Sharing what tool processes create with the Go Core and the worker
# ---------------------------------------------------------------------------


def _share_launch(root: str, identity: ToolIdentity | None, since: float | None) -> Launch | None:
    status = tool_isolation()
    if not status.config.required or not status.ready or not root:
        return None
    identity = identity or current_identity.get()
    if identity is None or identity.is_system:
        return None
    argv = [status.interpreter, "-I", "-S", TOOL_WALK, "share", root]
    if since is not None:
        argv += ["--since", repr(since)]
    return status.launch(argv, {"PATH": _PROBE_PATH}, None, identity.with_workspace(root), files=WALKER_FILES)


def _log_share(root: str, returncode: int | None, out: str, err: str) -> None:
    if returncode == 0:
        logger.debug("shared tool files under %s: %s", root, out.strip())
    elif returncode == 1:
        logger.warning("could not share every tool file under %s: %s", root, out.strip()[-1000:])
    else:
        logger.warning("could not share the tool files under %s (exit %s): %s", root, returncode, err.strip()[-500:])


async def share_tool_files(root: str, identity: ToolIdentity | None = None, *, since: float | None = None) -> None:
    """Share what the tool identity created under *root* with the workspace group (KI-71 review, KI-96 D8).

    Agents create files with owner-only modes (mkdtemp, mkdir -m 0700,
    umask 077), strip ACL entries or default ACLs; the worker and the Go
    Core could then neither read nor delete them: project deletion (GDPR
    erasure), checkpoints and delivery, and benchmark cleanups failed. The
    pass (codeforge.tool_walk) runs as the tenant's tool UID, which owns
    them, and changes nothing of anybody else's. With *since* only entries
    changed after that time are checked (after one tool call); without it
    every entry (the end of a work item). A pass that could not share
    everything, or did not end in time, is logged. It runs in a thread: the
    event loop goes on.
    """
    await asyncio.to_thread(share_tool_files_sync, root, identity, since=since)


# How long the worker waits for one of its own commands that runs as a tool
# UID: the sharing pass, a removal, measuring a cache (KI-96 D8, D10, D11).
# The worker awaits them while the next message of the subject waits, and a
# process of the same UID can stop them (Landlock scopes signals to a domain
# only from ABI 6 on). One that does not end in time is killed with its
# process group, and the step counts as failed.
HELPER_TIMEOUT_SECONDS = 600.0
# The migration's owner-run walks cover a whole tenant tree (D9).
MIGRATION_WALK_TIMEOUT_SECONDS = 3600.0
# How long the output of a killed helper is still read.
_KILLED_OUTPUT_SECONDS = 5.0


def _kill_group(proc: subprocess.Popen[str]) -> None:
    """Kill the helper's process group (its own session): the worker's child, not waited for yet,
    so the group's ID is still its PID."""
    if proc.returncode is None and proc.pid > 1:
        with contextlib.suppress(ProcessLookupError):
            os.killpg(proc.pid, signal.SIGKILL)


def _run_helper(launch: Launch, timeout: float) -> subprocess.CompletedProcess[str]:
    """Run *launch*, one of the worker's own commands as a tool UID, for at most *timeout* seconds.

    It runs in a session of its own. When it has not ended in time (another
    process of its UID stopped it, or it is slow) its whole process group is
    killed: the result has return code -SIGKILL and says why on stderr.
    """
    try:
        proc = subprocess.Popen(  # noqa: S603 - fixed programs of the worker's
            launch.argv,
            env=launch.env,
            cwd=launch.cwd,
            umask=TOOL_UMASK if launch.umask is None else launch.umask,
            pass_fds=launch.pass_fds,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            start_new_session=True,
        )
    finally:
        launch.close()
    with proc:
        try:
            out, err = proc.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            _kill_group(proc)
            try:
                out, err = proc.communicate(timeout=_KILLED_OUTPUT_SECONDS)
            except subprocess.TimeoutExpired:
                out, err = "", ""
            proc.wait()
            reason = f"timed out after {timeout:g} s (stopped by another process of its user?): killed"
            logger.warning("the worker's command %s %s", _helper_name(launch.argv), reason)
            return subprocess.CompletedProcess(launch.argv, -signal.SIGKILL, out or "", f"{err or ''}\n{reason}")
        except BaseException:
            _kill_group(proc)
            raise
    return subprocess.CompletedProcess(launch.argv, proc.returncode, out, err)


def _helper_name(argv: Sequence[str]) -> str:
    """What a helper is, for the log: the walker's mode or the removal, not its paths."""
    for index, arg in enumerate(argv):
        if arg == TOOL_WALK:
            return " ".join(["tool_walk.py", *argv[index + 1 : index + 2]])
        if arg in ("cf-remove", "cf-du"):
            return arg
    return os.path.basename(str(argv[-1])) if argv else "?"


def run_as_tool(
    identity: ToolIdentity,
    argv: Sequence[str],
    *,
    files: tuple[str, ...] = (),
    timeout: float | None = None,
    status: IsolationStatus | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run one of the worker's own commands (walker, removal, measurement) as *identity* and wait for it.

    At most *timeout* seconds (HELPER_TIMEOUT_SECONDS by default; then it
    is killed, see _run_helper). Unlike start_tool_process no sharing pass
    follows: these act on what the identity's Landlock rules confine them to.
    """
    status = status or tool_isolation()
    if not status.config.required or not status.ready:
        raise ToolIsolationError(f"tool isolation is not ready: {status.reason}")
    launch = status.launch(argv, {"PATH": _PROBE_PATH}, None, identity, files=files)
    return _run_helper(launch, HELPER_TIMEOUT_SECONDS if timeout is None else timeout)


def run_walker(
    identity: ToolIdentity, args: Sequence[str], *, timeout: float | None = None
) -> subprocess.CompletedProcess[str]:
    """Run codeforge.tool_walk with *args* as *identity* and wait for it (the migration's owner steps)."""
    status = tool_isolation()
    return run_as_tool(
        identity, [status.interpreter, "-I", "-S", TOOL_WALK, *args], files=WALKER_FILES, timeout=timeout
    )


# Removes what a tool wrote, as the tool's UID (W1): own entries first get
# owner rights back (a tool may have made them 0000), then everything goes,
# never across file systems; a symlink is removed, never followed.
_REMOVE_SCRIPT = """mode="$1"; shift
status=0
for path in "$@"; do
  if [ -L "$path" ]; then rm -f -- "$path" || status=1; continue; fi
  [ -e "$path" ] || continue
  chmod -R u+rwX -- "$path" 2>/dev/null
  if [ "$mode" = contents ]; then
    find "$path" -xdev -mindepth 1 -delete || status=1
  else
    rm -rf --one-file-system -- "$path" || status=1
  fi
done
exit $status
"""


def _confined(identity: ToolIdentity, confine: str) -> ToolIdentity:
    """*identity*'s UID with Landlock rules for *confine* alone (no HOME, no workspace, no extras)."""
    return ToolIdentity(
        tenant_id=identity.tenant_id, uid=identity.uid, home="", work_id=new_work_id(), workspace=confine
    )


def remove_as_tool_sync(
    paths: Sequence[str],
    identity: ToolIdentity,
    *,
    confine: str,
    contents_only: bool = False,
    status: IsolationStatus | None = None,
) -> bool:
    """Remove *paths* (or only what is inside them) as *identity*'s UID, confined to *confine* (D10, D11).

    True when everything is gone; a failure is logged.
    """
    mode = "contents" if contents_only else "all"
    try:
        done = run_as_tool(
            _confined(identity, confine), [_SHELL, "-c", _REMOVE_SCRIPT, "cf-remove", mode, *paths], status=status
        )
    except (OSError, subprocess.SubprocessError) as exc:
        logger.warning("could not remove %s as tool uid %d: %s", list(paths), identity.uid, exc)
        return False
    if done.returncode:
        logger.warning(
            "could not remove all of %s as tool uid %d: %s", list(paths), identity.uid, done.stderr.strip()[-500:]
        )
    return done.returncode == 0


async def remove_as_tool(
    paths: Sequence[str], identity: ToolIdentity, *, confine: str, contents_only: bool = False
) -> bool:
    """remove_as_tool_sync in a thread: the event loop goes on."""
    return await asyncio.to_thread(
        remove_as_tool_sync, list(paths), identity, confine=confine, contents_only=contents_only
    )


def share_tool_files_sync(root: str, identity: ToolIdentity | None = None, *, since: float | None = None) -> None:
    """share_tool_files for synchronous callers."""
    launch = _share_launch(root, identity, since)
    if launch is None:
        return
    try:
        done = _run_helper(launch, HELPER_TIMEOUT_SECONDS)
    except OSError as exc:
        logger.warning("could not share the tool files under %s: %s", root, exc)
        return
    _log_share(root, done.returncode, done.stdout or "", done.stderr or "")


def workspace_acl(uid: int) -> list[posix_acl.Entry]:
    """The ACL of a worker-made tool workspace: the tool UID and the workspace group, nobody else."""
    return [
        posix_acl.Entry(posix_acl.USER_OBJ, 7),
        posix_acl.Entry(posix_acl.USER, 7, uid),
        posix_acl.Entry(posix_acl.GROUP_OBJ, 0),
        posix_acl.Entry(posix_acl.GROUP, 7, WORKSPACE_GID),
        posix_acl.Entry(posix_acl.MASK, 7),
        posix_acl.Entry(posix_acl.OTHER, 0),
    ]


@contextlib.asynccontextmanager
async def tool_workspace(prefix: str, base: str | None = None) -> AsyncIterator[str]:
    """A temporary workspace the current tool identity works in (a benchmark task's), removed afterwards.

    The worker makes it (``mkdtemp`` in /tmp, which only the worker can
    write) and gives it, through its descriptor, an access and default ACL
    for the identity's UID and the workspace group; while the block runs it
    is the identity's workspace. Leaving removes what the tools created as
    the identity's UID (confined to the workspace, D11), then the worker
    removes the directory. Without isolation it is a plain temporary
    directory.
    """
    path = tempfile.mkdtemp(prefix=prefix, dir=base)
    status = tool_isolation()
    identity = current_identity.get()
    isolated = status.config.required and status.ready and identity is not None
    reset = None
    try:
        if isolated:
            fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
            try:
                acl = workspace_acl(identity.uid)  # type: ignore[union-attr]
                posix_acl.set_acl(fd, posix_acl.ACCESS, acl)
                posix_acl.set_acl(fd, posix_acl.DEFAULT, acl)
            finally:
                os.close(fd)
            reset = current_identity.set(identity.with_workspace(path))  # type: ignore[union-attr]
        yield path
    finally:
        try:
            if reset is not None:
                workspace_identity = current_identity.get() or identity
                removed = await remove_as_tool(
                    [path],
                    workspace_identity,
                    confine=path,
                    contents_only=True,  # type: ignore[arg-type]
                )
                if not removed:  # what the tools left: at least the worker can remove it
                    await share_tool_files(path)
        finally:
            if reset is not None:
                current_identity.reset(reset)
            shutil.rmtree(path, ignore_errors=True)


def grant_tool_access(target: str | int, *, writable: bool, identity: ToolIdentity | None = None) -> None:
    """Let the tool identity use a file, directory or socket the worker made for it (W1).

    *target* is a descriptor, or a path inside a directory only the worker
    can write (``/tmp`` is 1771; the Claude Code run directory is 0700); it
    is opened without following a symlink and must be the worker's own. It
    gets an exact access ACL: the worker keeps everything, the tool UID gets
    read (and write with *writable*; search for directories), nobody else
    anything. Without isolation (or while no tool process can start) it does
    nothing.
    """
    status = tool_isolation()
    if not status.config.required or not status.ready:
        return
    identity = identity or current_identity.get()
    if identity is None:
        raise ToolIsolationError("no tool identity to grant access to")
    fd = target if isinstance(target, int) else os.open(target, os.O_PATH | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        info = os.fstat(fd)
        if info.st_uid != os.getuid() or not (
            stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode) or stat.S_ISSOCK(info.st_mode)
        ):
            raise ToolIsolationError(f"{target!r} is not one of the worker's files: access not granted")
        directory = stat.S_ISDIR(info.st_mode)
        owner = 7 if directory else 6
        tool = (7 if writable else 5) if directory else (6 if writable else 4)
        entries = [
            posix_acl.Entry(posix_acl.USER_OBJ, owner),
            posix_acl.Entry(posix_acl.USER, tool, identity.uid),
            posix_acl.Entry(posix_acl.GROUP_OBJ, 0),
            posix_acl.Entry(posix_acl.MASK, tool),
            posix_acl.Entry(posix_acl.OTHER, 0),
        ]
        # An O_PATH descriptor takes no xattr calls: its /proc/self/fd link names exactly this inode.
        posix_acl.set_acl(f"/proc/self/fd/{fd}", posix_acl.ACCESS, entries)
    finally:
        if not isinstance(target, int):
            os.close(fd)


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

    if cwd is None:
        # The server runs in the run's workspace (KI-96), where its tool identity may work.
        identity = current_identity.get()
        cwd = identity.workspace if identity is not None else None
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
