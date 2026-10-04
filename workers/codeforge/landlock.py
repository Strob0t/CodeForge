"""Landlock per tool call, the worker's side (KI-96 D6, ADR-018).

With isolation required every tool process is also confined by Landlock: the
launch helper (codeforge.tool_exec) builds a ruleset from the launch spec,
as the tool user, and restricts itself before it executes the command. The
worker decides the rules here:

- the system read-only (``/usr``, ``/etc``, a few devices and ``/proc`` and
  ``/sys`` files runtimes need); the command's own ``/proc/<pid>``;
- the work item's workspace and the tenant's HOME with every right except
  device nodes and device ioctls;
- per-call extras read-only (the Claude Code run directory, the hook file),
  operator toolchains from ``CODEFORGE_TOOL_READ_PATHS``;
- with ABI 6 or later, signals and abstract unix sockets scoped to the
  command's own domain.

Everything else is denied: ``/tmp``, ``/app``, ``/run`` (secrets), other
tenants' workspaces and HOMEs, the tenant's other projects, other processes'
``/proc`` entries (command lines and environments), the ``/proc`` listing.

``CODEFORGE_TOOL_LANDLOCK`` is ``required`` or ``off``; unset it follows
``CODEFORGE_TOOL_ISOLATION``, any other value counts as ``required``. ``off``
is refused in production (the worker is then not ready);
``CODEFORGE_TOOL_LANDLOCK_MIN_ABI`` (default 2: ABI 1 has no ``refer``, so
``mv`` and git break) is the oldest kernel ABI accepted.
"""

from __future__ import annotations

import os
import stat
from typing import TYPE_CHECKING

from codeforge.tool_exec import FS_RIGHTS, landlock_abi

if TYPE_CHECKING:
    from codeforge.tool_identity import ToolIdentity

LANDLOCK_REQUIRED = "required"
LANDLOCK_OFF = "off"
DEFAULT_MIN_ABI = 2
# A world-readable file of the image no rule ever grants: the probe must not read it.
CANARY = "/var/lib/codeforge/landlock-canary"
ALL_RIGHTS = tuple(FS_RIGHTS)

_READ = ["read-file", "read-dir"]
_READ_EXEC = ["execute", "read-file", "read-dir"]
_WORK = [right for right in ALL_RIGHTS if right not in ("make-char", "make-block", "ioctl-dev")]
_TTY = ["read-file", "write-file", "ioctl-dev"]

# (path, rights): optional, a container may lack some of them.
SYSTEM_RULES: tuple[tuple[str, list[str]], ...] = (
    ("/usr", _READ_EXEC),
    ("/etc", _READ),
    ("/dev/null", ["read-file", "write-file", "truncate"]),
    ("/dev/zero", ["read-file"]),
    ("/dev/random", ["read-file"]),
    ("/dev/urandom", ["read-file"]),
    # ENXIO without a controlling terminal instead of EACCES (E19).
    ("/dev/tty", ["read-file", "write-file"]),
    # Pseudo-terminals (os.openpty, script, pexpect, tmux, node-pty); no listing.
    ("/dev/ptmx", _TTY),
    ("/dev/pts", _TTY),
    # POSIX semaphores (multiprocessing.Lock); no listing.
    ("/dev/shm", ["read-file", "write-file", "make-reg", "remove-file", "truncate"]),  # noqa: S108 - a rule
    # node's os.cpus() and friends (E11), Bun's vm settings (E4), CPU and cgroup limits.
    ("/proc/cpuinfo", ["read-file"]),
    ("/proc/stat", ["read-file"]),
    ("/proc/meminfo", ["read-file"]),
    ("/proc/loadavg", ["read-file"]),
    ("/proc/uptime", ["read-file"]),
    ("/proc/sys", _READ),
    ("/sys/devices/system", _READ),
    ("/sys/fs/cgroup", _READ),
    ("/sys/kernel/mm/transparent_hugepage", _READ),
)

# Operator read paths that could reach secrets, tenants' trees or the canary.
_REFUSED_EXACT = ("/", "/proc", "/sys", "/run", "/tmp", "/data", "/home", "/var/lib/codeforge")  # noqa: S108
_REFUSED_BELOW = ("/run", "/proc", "/data", "/home/codeforge-tools", "/var/lib/codeforge")


def parse_mode(raw: str, isolation_mode: str) -> str:
    """The Landlock mode: unset follows isolation; anything but "required" or "off" fails closed."""
    value = raw.strip().lower()
    if not value:
        return LANDLOCK_REQUIRED if isolation_mode == "required" else LANDLOCK_OFF
    return LANDLOCK_OFF if value == LANDLOCK_OFF else LANDLOCK_REQUIRED


def parse_read_paths(raw: str) -> tuple[str, ...]:
    """CODEFORGE_TOOL_READ_PATHS: colon-separated absolute paths."""
    return tuple(os.path.normpath(part) for part in raw.split(":") if part.strip())


def _inside(path: str, directory: str) -> bool:
    return path == directory or path.startswith(directory.rstrip("/") + "/")


def read_path_problems(paths: tuple[str, ...]) -> list[str]:
    """Why operator read paths are refused: relative, missing, through a symlink, or reaching what tools must not."""
    problems = []
    for path in paths:
        if not path.startswith("/"):
            problems.append(f"CODEFORGE_TOOL_READ_PATHS entry {path!r} is not an absolute path")
            continue
        normal = os.path.normpath(path)
        if normal in _REFUSED_EXACT or any(_inside(guarded, normal) for guarded in _REFUSED_EXACT[1:]):
            problems.append(f"CODEFORGE_TOOL_READ_PATHS entry {path} would let tools read {normal}: refused")
            continue
        if any(_inside(normal, guarded) for guarded in _REFUSED_BELOW):
            problems.append(f"CODEFORGE_TOOL_READ_PATHS entry {path} lies below a guarded directory: refused")
            continue
        problem = _symlink_problem(normal)
        if problem:
            problems.append(f"CODEFORGE_TOOL_READ_PATHS entry {path}: {problem}")
    return problems


def _symlink_problem(path: str) -> str:
    current = ""
    for part in [p for p in path.split("/") if p]:
        current += "/" + part
        try:
            info = os.lstat(current)
        except FileNotFoundError:
            return "does not exist"
        except OSError as exc:
            return exc.strerror or str(exc)
        if stat.S_ISLNK(info.st_mode):
            return f"{current} is a symlink"
    return ""


def rules_for(
    identity: ToolIdentity,
    *,
    interpreter_prefix: str,
    read_paths: tuple[str, ...] = (),
    files: tuple[str, ...] = (),
) -> list[list[object]]:
    """The Landlock rules of a launch as *identity*: ``[path, rights, required]``.

    *files* are files the command reads as a program (the walker and its
    codec); *interpreter_prefix* is the base interpreter's ``sys.base_prefix``
    (a rule when it lies outside ``/usr``, as on CI runners).
    """
    rules: list[list[object]] = [[path, rights, False] for path, rights in SYSTEM_RULES]
    if not _inside(os.path.normpath(interpreter_prefix), "/usr"):
        rules.append([os.path.normpath(interpreter_prefix), _READ_EXEC, True])
    work = [path for path in (identity.workspace, identity.home, *identity.write_paths) if path]
    rules += [[os.path.normpath(path), _WORK, True] for path in work]
    rules += [[os.path.normpath(path), _READ_EXEC, True] for path in (*identity.read_paths, *read_paths)]
    rules += [[os.path.normpath(path), ["read-file"], True] for path in files]
    return rules


def spec_value(
    mode: str,
    identity: ToolIdentity,
    *,
    min_abi: int,
    interpreter_prefix: str,
    read_paths: tuple[str, ...] = (),
    files: tuple[str, ...] = (),
) -> str | dict[str, object]:
    """The launch spec's ``landlock`` field: "off", or the rules the helper applies."""
    if mode == LANDLOCK_OFF:
        return LANDLOCK_OFF
    return {
        "rules": rules_for(identity, interpreter_prefix=interpreter_prefix, read_paths=read_paths, files=files),
        "scope": True,
        "min_abi": min_abi,
        "proc_self": True,
    }


def kernel_abi() -> int:
    """The kernel's Landlock ABI as the worker sees it (0: none)."""
    return landlock_abi()
