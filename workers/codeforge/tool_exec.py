"""Start one agent tool process: the second half of the launcher (KI-96, ADR-018).

The worker starts every tool process as

    setpriv --reuid=T --regid=T ... --no-new-privs -- <python> -I -S tool_exec.py <fd> <argv...>

and writes a launch spec (JSON) to a memfd it passes as descriptor <fd>. This
helper already runs as the tool user. It reads and closes the spec, checks its
own credentials against it, creates the per-work directories the spec names
below the tool HOME, changes into the working directory, sets the umask and
executes argv with exactly the spec's environment. Nothing of the environment
(provider keys, the Claude Code policy token, MCP server tokens) is ever an
argument: /proc/<pid>/cmdline is readable by every process in the container,
/proc/<pid>/environ only by the same user.

Every path is opened one component at a time from "/" without following a
symlink: the tool user may have planted one in its HOME or workspace, and the
helper must never act anywhere else.

Before that it confines itself, and so the command, with Landlock (D6): the
spec's ``landlock`` names the paths the command may use and how (the
workspace and the tenant's HOME fully, the system read-only, ...); the
helper opens every rule path itself, as the tool user and without following
a symlink, and with ABI 6 or later also scopes signals and abstract unix
sockets to the command's own domain. ``"off"`` (development only) skips it.

Any failure prints ``cf-tool-exec: <reason>`` and exits 125 without running
the command. Standard library only: it runs with ``-I -S`` (no site, no user
paths, no PYTHON* variables) and imports nothing of CodeForge.
"""

from __future__ import annotations

import ctypes
import json
import os
import stat
import struct
import sys

EXIT_REFUSED = 125
SPEC_FIELDS = ("uid", "gid", "groups", "umask", "env", "landlock", "prepare", "home", "cwd")
LANDLOCK_OFF = "off"
# A spec is a few kilobytes; anything far larger is not one.
_SPEC_MAX_BYTES = 4 << 20
_CAP_SETS = ("CapInh", "CapPrm", "CapEff", "CapAmb")


class LaunchRefusedError(Exception):
    """The tool process must not start."""


def _fail(reason: str) -> None:
    os.write(2, f"cf-tool-exec: {reason}\n".encode(errors="replace"))
    os._exit(EXIT_REFUSED)


def read_spec(fd: int) -> dict[str, object]:
    """Read the launch spec from *fd* and close it; every field must be present."""
    chunks: list[bytes] = []
    size = 0
    try:
        os.lseek(fd, 0, os.SEEK_SET)
        while chunk := os.read(fd, 65536):
            size += len(chunk)
            if size > _SPEC_MAX_BYTES:
                raise LaunchRefusedError("the launch spec is too large")
            chunks.append(chunk)
    except OSError as exc:
        raise LaunchRefusedError(f"cannot read the launch spec: {exc.strerror}") from exc
    finally:
        try:
            os.close(fd)
        except OSError as exc:
            raise LaunchRefusedError(f"cannot close the launch spec: {exc.strerror}") from exc
    try:
        spec = json.loads(b"".join(chunks))
    except ValueError as exc:
        raise LaunchRefusedError(f"the launch spec is not JSON: {exc}") from exc
    if not isinstance(spec, dict):
        raise LaunchRefusedError("the launch spec is not an object")
    missing = [name for name in SPEC_FIELDS if name not in spec]
    if missing:
        raise LaunchRefusedError("the launch spec lacks " + ", ".join(missing))
    _check_types(spec)
    return spec


def _check_types(spec: dict[str, object]) -> None:
    for name in ("uid", "gid", "umask"):
        if type(spec[name]) is not int or spec[name] < 0:  # type: ignore[operator]
            raise LaunchRefusedError(f"the launch spec's {name} is not a number")
    groups = spec["groups"]
    if not isinstance(groups, list) or any(type(gid) is not int or gid < 0 for gid in groups):
        raise LaunchRefusedError("the launch spec's groups are not numbers")
    env = spec["env"]
    if not isinstance(env, dict) or any(not isinstance(k, str) or not isinstance(v, str) for k, v in env.items()):
        raise LaunchRefusedError("the launch spec's env is not a map of strings")
    prepare = spec["prepare"]
    if not isinstance(prepare, list) or any(not isinstance(entry, str) for entry in prepare):
        raise LaunchRefusedError("the launch spec's prepare is not a list of strings")
    for name in ("home", "cwd"):
        if spec[name] is not None and not isinstance(spec[name], str):
            raise LaunchRefusedError(f"the launch spec's {name} is not a path")
    if prepare and spec["home"] is None:
        raise LaunchRefusedError("the launch spec prepares directories without a home")


def _status_fields() -> dict[str, str]:
    fields: dict[str, str] = {}
    with open("/proc/self/status") as status:
        for line in status:
            name, sep, value = line.partition(":")
            if sep:
                fields[name.strip()] = value.strip()
    return fields


def check_credentials(spec: dict[str, object], fields: dict[str, str] | None = None) -> None:
    """The helper must run exactly as the spec says: its UID and GID, its groups, no capability, no_new_privs."""
    if fields is None:
        try:
            fields = _status_fields()
        except OSError as exc:
            raise LaunchRefusedError(f"cannot read the process status: {exc.strerror}") from exc
    uid, gid = str(spec["uid"]), str(spec["gid"])
    if fields.get("Uid", "").split() != [uid] * 4:
        raise LaunchRefusedError(f"runs with uid {fields.get('Uid')!r}, expected {uid}")
    if fields.get("Gid", "").split() != [gid] * 4:
        raise LaunchRefusedError(f"runs with gid {fields.get('Gid')!r}, expected {gid}")
    groups = sorted(int(g) for g in fields.get("Groups", "").split())
    expected = sorted(spec["groups"])  # type: ignore[type-var]
    if groups != expected:
        raise LaunchRefusedError(f"has the supplementary groups {groups}, expected {expected}")
    for name in _CAP_SETS:
        try:
            held = int(fields.get(name, "x"), 16)
        except ValueError:
            held = -1
        if held != 0:
            raise LaunchRefusedError(f"holds capabilities ({name}={fields.get(name)})")
    if fields.get("NoNewPrivs") != "1":
        raise LaunchRefusedError("runs without no_new_privs")


def _components(path: str) -> list[str]:
    if not path.startswith("/"):
        raise LaunchRefusedError(f"{path!r} is not an absolute path")
    parts = [part for part in path.split("/") if part]
    if any(part in (".", "..") for part in parts):
        raise LaunchRefusedError(f"{path!r} is not a plain path")
    return parts


def open_nofollow(path: str, *, directory: bool, flags: int = os.O_PATH) -> int:
    """Open *path* one component at a time from "/" without following a symlink anywhere."""
    parts = _components(path)
    fd = os.open("/", os.O_PATH | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for index, part in enumerate(parts):
            last = index == len(parts) - 1
            want_dir = directory or not last
            mode = (flags if last else os.O_PATH) | os.O_NOFOLLOW | os.O_CLOEXEC
            if want_dir:
                mode |= os.O_DIRECTORY
            try:
                child = os.open(part, mode, dir_fd=fd)
            except OSError as exc:
                if exc.errno in (20, 40):  # ENOTDIR, ELOOP: a symlink (or no directory) where one is needed
                    raise LaunchRefusedError(f"{path!r}: {part!r} is a symlink or not a directory") from exc
                raise LaunchRefusedError(f"{path!r}: {exc.strerror}") from exc
            os.close(fd)
            fd = child
            if stat.S_ISLNK(os.fstat(fd).st_mode):
                raise LaunchRefusedError(f"{path!r} is a symlink")
        return fd
    except BaseException:
        os.close(fd)
        raise


def prepare(home: str, entries: list[str]) -> None:
    """Create each entry (``tmp/<token>``) below *home*, one component at a time, as this user.

    A component that exists must be a directory this user owns, never a symlink.
    """
    if not entries:
        return
    home_fd = open_nofollow(home, directory=True)
    try:
        for entry in entries:
            parts = [part for part in entry.split("/") if part]
            if not parts or any(part in (".", "..") for part in parts) or entry.startswith("/"):
                raise LaunchRefusedError(f"prepare entry {entry!r} is not a relative path")
            fd = os.dup(home_fd)
            try:
                for part in parts:
                    try:
                        os.mkdir(part, 0o700, dir_fd=fd)
                    except FileExistsError:
                        pass
                    except OSError as exc:
                        raise LaunchRefusedError(f"cannot create {entry!r} in {home}: {exc.strerror}") from exc
                    try:
                        child = os.open(part, os.O_PATH | os.O_NOFOLLOW | os.O_DIRECTORY | os.O_CLOEXEC, dir_fd=fd)
                    except OSError as exc:
                        raise LaunchRefusedError(
                            f"{entry!r} in {home}: {part!r} is a symlink or not a directory"
                        ) from exc
                    os.close(fd)
                    fd = child
                    info = os.fstat(fd)
                    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid():
                        raise LaunchRefusedError(f"{entry!r} in {home}: {part!r} is not this user's directory")
            finally:
                os.close(fd)
    finally:
        os.close(home_fd)


def change_directory(cwd: str | None) -> None:
    if cwd is None:
        return
    fd = open_nofollow(cwd, directory=True)
    try:
        os.fchdir(fd)
    except OSError as exc:
        raise LaunchRefusedError(f"cannot enter {cwd!r}: {exc.strerror}") from exc
    finally:
        os.close(fd)


# Landlock (linux/landlock.h); the system call numbers are the same on every architecture.
SYS_LANDLOCK_CREATE_RULESET = 444
SYS_LANDLOCK_ADD_RULE = 445
SYS_LANDLOCK_RESTRICT_SELF = 446
_CREATE_RULESET_VERSION = 1
_RULE_PATH_BENEATH = 1
FS_RIGHTS = {
    "execute": 1 << 0,
    "write-file": 1 << 1,
    "read-file": 1 << 2,
    "read-dir": 1 << 3,
    "remove-dir": 1 << 4,
    "remove-file": 1 << 5,
    "make-char": 1 << 6,
    "make-dir": 1 << 7,
    "make-reg": 1 << 8,
    "make-sock": 1 << 9,
    "make-fifo": 1 << 10,
    "make-block": 1 << 11,
    "make-sym": 1 << 12,
    "refer": 1 << 13,  # ABI 2
    "truncate": 1 << 14,  # ABI 3
    "ioctl-dev": 1 << 15,  # ABI 5
}
# What a rule on a file (not a directory) may allow.
_FILE_RIGHTS = (
    FS_RIGHTS["execute"] | FS_RIGHTS["write-file"] | FS_RIGHTS["read-file"] | FS_RIGHTS["truncate"]
) | FS_RIGHTS["ioctl-dev"]
_SCOPE_ABSTRACT_UNIX_SOCKET = 1 << 0
_SCOPE_SIGNAL = 1 << 1
_LANDLOCK_FIELDS = ("rules", "scope", "min_abi", "proc_self")


def _syscall(number: int, *args: object) -> int:
    libc = ctypes.CDLL(None, use_errno=True)
    result = libc.syscall(number, *args)
    if result < 0:
        errno = ctypes.get_errno()
        raise OSError(errno, os.strerror(errno))
    return int(result)


def landlock_abi() -> int:
    """The kernel's Landlock ABI version; 0 without Landlock (not built, not enabled, or filtered)."""
    try:
        return _syscall(SYS_LANDLOCK_CREATE_RULESET, None, ctypes.c_size_t(0), ctypes.c_uint32(_CREATE_RULESET_VERSION))
    except OSError:
        return 0


def handled_access(abi: int) -> int:
    """Every filesystem right ABI *abi* knows: all of them are handled, so all are denied unless a rule allows."""
    if abi >= 5:
        return (1 << 16) - 1
    if abi >= 3:
        return (1 << 15) - 1
    if abi == 2:
        return (1 << 14) - 1
    return (1 << 13) - 1


def ruleset_attr(abi: int, handled: int, scoped: int) -> bytes:
    """struct landlock_ruleset_attr as ABI *abi* knows it: 8, 16 (network) or 24 (scopes) bytes."""
    attr = struct.pack("<Q", handled)
    if abi >= 4:
        attr += struct.pack("<Q", 0)  # no network rights handled (KI-110)
    if abi >= 6:
        attr += struct.pack("<Q", scoped)
    return attr


def _check_landlock(landlock: object) -> dict[str, object]:
    if not isinstance(landlock, dict) or sorted(landlock) != sorted(_LANDLOCK_FIELDS):
        raise LaunchRefusedError(f"unknown landlock setting {landlock!r}")
    rules = landlock["rules"]
    if not isinstance(rules, list):
        raise LaunchRefusedError("the landlock rules are not a list")
    for rule in rules:
        if (
            not isinstance(rule, list)
            or len(rule) != 3
            or not isinstance(rule[0], str)
            or not rule[0].startswith("/")
            or not isinstance(rule[1], list)
            or any(right not in FS_RIGHTS for right in rule[1])
            or not isinstance(rule[2], bool)
        ):
            raise LaunchRefusedError(f"malformed landlock rule {rule!r}")
    if type(landlock["min_abi"]) is not int or not isinstance(landlock["scope"], bool):
        raise LaunchRefusedError("malformed landlock setting")
    if not isinstance(landlock["proc_self"], bool):
        raise LaunchRefusedError("malformed landlock setting")
    return landlock


def _add_rule(ruleset: int, allowed: int, fd: int) -> None:
    attr = ctypes.create_string_buffer(struct.pack("<Qi", allowed, fd), 12)
    _syscall(SYS_LANDLOCK_ADD_RULE, ctypes.c_int(ruleset), ctypes.c_int(_RULE_PATH_BENEATH), attr, ctypes.c_uint32(0))


def _add_path_rules(ruleset: int, rules: list[list[object]], handled: int) -> None:
    for path, rights, required in rules:
        try:
            fd = open_nofollow(path, directory=False)  # type: ignore[arg-type]
        except LaunchRefusedError:
            if required:
                raise
            continue  # an optional system path this container lacks (/dev/tty, ...) or links elsewhere
        try:
            allowed = 0
            for right in rights:  # type: ignore[attr-defined]
                allowed |= FS_RIGHTS[right]
            if not stat.S_ISDIR(os.fstat(fd).st_mode):
                allowed &= _FILE_RIGHTS
            if allowed & handled:
                _add_rule(ruleset, allowed & handled, fd)
        finally:
            os.close(fd)


def apply_landlock(spec: dict[str, object]) -> None:
    """Confine this process (and so the command) to the spec's Landlock rules; "off" does nothing."""
    if spec["landlock"] == LANDLOCK_OFF:
        return
    landlock = _check_landlock(spec["landlock"])
    abi = landlock_abi()
    if abi == 0:
        raise LaunchRefusedError("no Landlock in this kernel (or it is filtered): tool processes cannot be confined")
    min_abi: int = landlock["min_abi"]  # type: ignore[assignment]
    if abi < min_abi:
        raise LaunchRefusedError(f"Landlock ABI {abi} is below the required {min_abi}")
    handled = handled_access(abi)
    scoped = _SCOPE_SIGNAL | _SCOPE_ABSTRACT_UNIX_SOCKET if landlock["scope"] and abi >= 6 else 0
    attr = ruleset_attr(abi, handled, scoped)
    try:
        ruleset = _syscall(
            SYS_LANDLOCK_CREATE_RULESET, ctypes.create_string_buffer(attr, len(attr)), ctypes.c_size_t(len(attr)), 0
        )
    except OSError as exc:
        raise LaunchRefusedError(f"cannot create a Landlock ruleset: {exc.strerror}") from exc
    try:
        try:
            _add_path_rules(ruleset, landlock["rules"], handled)  # type: ignore[arg-type]
            if landlock["proc_self"]:
                # The command's own /proc entry. The descriptor stays open across
                # the exec: it pins the dentry, a rule on an evicted one would
                # stop matching (E6).
                fd = os.open(f"/proc/{os.getpid()}", os.O_PATH | os.O_DIRECTORY | os.O_NOFOLLOW)
                os.set_inheritable(fd, True)
                _add_rule(ruleset, FS_RIGHTS["read-file"] | FS_RIGHTS["read-dir"], fd)
            _syscall(SYS_LANDLOCK_RESTRICT_SELF, ctypes.c_int(ruleset), ctypes.c_uint32(0))
        except OSError as exc:
            raise LaunchRefusedError(f"cannot apply the Landlock rules: {exc.strerror or exc}") from exc
    finally:
        os.close(ruleset)


def run(argv: list[str]) -> None:
    """Everything up to the exec; raises LaunchRefusedError."""
    if len(argv) < 2:
        raise LaunchRefusedError("usage: tool_exec.py <spec fd> <program> [args...]")
    try:
        fd = int(argv[0])
    except ValueError as exc:
        raise LaunchRefusedError(f"{argv[0]!r} is not a descriptor") from exc
    spec = read_spec(fd)
    check_credentials(spec)
    apply_landlock(spec)
    prepare(spec["home"], spec["prepare"])  # type: ignore[arg-type]
    change_directory(spec["cwd"])  # type: ignore[arg-type]
    os.umask(spec["umask"])  # type: ignore[arg-type]
    env: dict[str, str] = spec["env"]  # type: ignore[assignment]
    try:
        os.execvpe(argv[1], argv[1:], env)  # noqa: S606 - the spec's command, already as the tool user
    except OSError as exc:
        raise LaunchRefusedError(f"cannot execute {argv[1]!r}: {exc.strerror}") from exc


def main() -> None:
    try:
        run(sys.argv[1:])
    except LaunchRefusedError as exc:
        _fail(str(exc))
    except Exception as exc:  # anything unexpected must not run the command either
        _fail(f"{type(exc).__name__}: {exc}")


if __name__ == "__main__":
    main()
