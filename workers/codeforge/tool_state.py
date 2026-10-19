"""The worker's view of the shared volumes for per-tenant tool identities (KI-96, ADR-018).

Every check and change here works on directory descriptors, never by a path
inside a tree a tool can write (rule W1): a tool of tenant T may create,
rename and replace entries in its workspaces and its HOME, and the worker,
which can write every tenant's tree, must never follow a symlink it planted.

- The workspace root (``/data/workspaces``) belongs to the worker, mode 2771:
  tools can traverse it, but neither list nor change it.
- ``<root>/.codeforge`` is the worker's own state (mode 0700, no ACL): UID
  bindings (``uids/<uid>`` names the tenant; they survive a database
  restore), migration stamps, cross-worker locks.
- A tenant directory ``<root>/<tenant>`` belongs to the worker (the Go Core,
  same UID), mode 2770, with exactly the tenant's access and default ACLs
  (codeforge.posix_acl): checked at every accept and launch.
- The HOME base (``/home/codeforge-tools``, a volume) belongs to the worker,
  mode 0711; each tool UID's HOME ``<base>/<uid>`` belongs to the worker,
  with the access ACL ``u::rwx,u:<uid>:rwx,g::---,m::rwx,o::---`` (mode
  0770: the group bits are the mask). The worker creates it once
  and never creates, writes, lists or removes anything below it.
"""

from __future__ import annotations

import contextlib
import errno
import os
import secrets
import stat
from dataclasses import dataclass
from enum import Enum

from codeforge import posix_acl
from codeforge.tool_identity import ToolIsolationError

ROOT_MODE = 0o2771
TENANT_DIR_MODE = 0o2770
HOME_BASE_MODE = 0o711
# rwx for the owner, nothing for others; the group bits are the mask of the
# HOME's ACL (u:<uid>:rwx), its owning group (the worker's own) gets nothing.
HOME_MODE = 0o770
# What an operator does about a volume without POSIX ACLs (EOPNOTSUPP).
ACL_REMEDY = "use a file system with POSIX ACLs (ext4, xfs, btrfs; ZFS with acltype=posixacl)"
STATE_DIR = ".codeforge"
STATE_SUBDIRS = ("uids", "tenants", "locks", "adopted")
_BINDING_MAX_BYTES = 128

_DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC


def worker_uid() -> int:
    return os.getuid()


def open_root(root: str) -> int:
    """A descriptor of the workspace root (the root itself may be reached through symlinks)."""
    try:
        return os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    except OSError as exc:
        raise ToolIsolationError(f"cannot open the workspace root {root}: {exc.strerror}") from exc


def open_dir_at(parent_fd: int, name: str) -> int:
    """A descriptor of the directory *name* in *parent_fd*, never through a symlink."""
    if not name or "/" in name or name in (".", ".."):
        raise ToolIsolationError(f"{name!r} is not a directory name")
    return os.open(name, _DIR_FLAGS, dir_fd=parent_fd)


def named_entries(acl: list[posix_acl.Entry] | None) -> list[posix_acl.Entry]:
    return [e for e in acl or [] if e.tag in (posix_acl.USER, posix_acl.GROUP)]


# ---------------------------------------------------------------------------
# The workspace root and the worker's state directory
# ---------------------------------------------------------------------------


def root_problems(root: str, *, fix: bool) -> list[str]:
    """What is wrong with the workspace root; with *fix* the worker first sets its mode (2771)."""
    fd = open_root(root)
    try:
        info = os.fstat(fd)
        if info.st_uid != worker_uid():
            return [f"the workspace root {root} belongs to uid {info.st_uid}, not the worker (uid {worker_uid()})"]
        if fix and stat.S_IMODE(info.st_mode) != ROOT_MODE:
            os.fchmod(fd, ROOT_MODE)
            info = os.fstat(fd)
        problems = []
        if stat.S_IMODE(info.st_mode) != ROOT_MODE:
            problems.append(f"the workspace root {root} has mode {stat.S_IMODE(info.st_mode):o}, expected 2771")
        try:
            named = named_entries(posix_acl.get_acl(fd, posix_acl.ACCESS))
            default = posix_acl.get_acl(fd, posix_acl.DEFAULT)
        except OSError as exc:
            return [*problems, no_acls_problem(root, exc)]
        if named or default is not None:
            problems.append(f"the workspace root {root} carries ACL entries (remove them: setfacl -b {root})")
        return problems
    finally:
        os.close(fd)


def no_acls_problem(path: str, exc: OSError) -> str:
    """The reason a volume cannot isolate tenants when its ACLs cannot be read or set."""
    return f"no POSIX ACLs on {path} ({exc.strerror or exc}): {ACL_REMEDY}"


def private_dir_problem(fd: int, path: str) -> str:
    info = os.fstat(fd)
    if info.st_uid != worker_uid():
        return f"{path} belongs to uid {info.st_uid}, not the worker"
    if stat.S_IMODE(info.st_mode) & 0o077:
        return f"{path} has mode {stat.S_IMODE(info.st_mode):o}: group or others have access"
    if posix_acl.get_acl(fd, posix_acl.ACCESS) is not None or posix_acl.get_acl(fd, posix_acl.DEFAULT) is not None:
        return f"{path} carries an ACL"
    return ""


def _ensure_private_dir(parent_fd: int, name: str, path: str) -> int:
    with contextlib.suppress(FileExistsError):
        os.mkdir(name, 0o700, dir_fd=parent_fd)
    try:
        fd = open_dir_at(parent_fd, name)
    except OSError as exc:
        raise ToolIsolationError(f"cannot open {path} (a symlink?): {exc.strerror}") from exc
    if problem := private_dir_problem(fd, path):
        os.close(fd)
        raise ToolIsolationError(problem)
    return fd


@contextlib.contextmanager
def state_dir(root: str, subdir: str | None = None) -> object:
    """A descriptor of ``<root>/.codeforge`` (or one of its subdirectories), created and verified."""
    root_fd = open_root(root)
    try:
        fd = _ensure_private_dir(root_fd, STATE_DIR, f"{root}/{STATE_DIR}")
        if subdir is not None:
            try:
                sub = _ensure_private_dir(fd, subdir, f"{root}/{STATE_DIR}/{subdir}")
            finally:
                os.close(fd)
            fd = sub
        try:
            yield fd
        finally:
            os.close(fd)
    finally:
        os.close(root_fd)


def ensure_state_dirs(root: str) -> None:
    for name in STATE_SUBDIRS:
        with state_dir(root, name):
            pass


def read_small(dir_fd: int, name: str, limit: int) -> bytes | None:
    """A small regular file of the worker's in *dir_fd*; None when absent; never through a symlink or FIFO."""
    try:
        fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=dir_fd)
    except FileNotFoundError:
        return None
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != worker_uid() or info.st_size > limit:
            raise ToolIsolationError(f"{name} in the worker's state directory is not one of its files")
        return os.read(fd, limit + 1)
    finally:
        os.close(fd)


def write_all(fd: int, data: bytes) -> None:
    """Write all of *data* to *fd* (a write to a full volume can be short)."""
    view = memoryview(data)
    while view:
        view = view[os.write(fd, view) :]


def write_new(dir_fd: int, name: str, data: bytes) -> bool:
    """Create *name* with *data* (O_EXCL); False when it already exists.

    A file it could not write completely (a full or failing volume) is
    removed again, and the error raised.
    """
    try:
        fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600, dir_fd=dir_fd)
    except FileExistsError:
        return False
    try:
        try:
            write_all(fd, data)
        finally:
            os.close(fd)
    except BaseException:
        with contextlib.suppress(OSError):
            os.unlink(name, dir_fd=dir_fd)
        raise
    return True


def bind_tool_uid(root: str, uid: int, tenant_id: str) -> None:
    """Record that *uid* belongs to *tenant_id* on the volume; refuse a UID bound to another tenant.

    The binding survives a database restore: a restored database could hand
    the UID of an existing tenant's files to a new tenant (KI-96 S8).
    """
    with state_dir(root, "uids") as fd:
        for _ in range(2):
            current = read_small(fd, str(uid), _BINDING_MAX_BYTES)
            if current is None:
                if write_new(fd, str(uid), tenant_id.encode()):
                    return
                continue  # another worker bound it meanwhile: read its binding
            bound = current.decode(errors="replace").strip()
            if bound != tenant_id:
                raise ToolIsolationError(
                    f"tool uid {uid} of tenant {tenant_id} is bound to tenant {bound} on the workspaces volume: "
                    "the database and the volume disagree (restore both to the same point; the Go Core then "
                    "advances the UID sequence past the volume's bindings)"
                )
            return
    raise ToolIsolationError(f"cannot bind tool uid {uid} to tenant {tenant_id}")


def bound_tenant(root: str, uid: int) -> str | None:
    with state_dir(root, "uids") as fd:
        data = read_small(fd, str(uid), _BINDING_MAX_BYTES)
    return None if data is None else data.decode(errors="replace").strip()


# ---------------------------------------------------------------------------
# Tenant directories
# ---------------------------------------------------------------------------


class TenantDir(Enum):
    """What the verification of a tenant directory found."""

    OK = "ok"
    # The worker's own directory, but its mode or ACLs differ (or it was never
    # migrated): the migration fixes it.
    MIGRATE = "migrate"
    # Not a directory, a symlink, or another user's: never touched.
    REFUSED = "refused"


@dataclass(frozen=True)
class TenantDirCheck:
    state: TenantDir
    reason: str = ""
    dev: int = 0
    ino: int = 0


def check_tenant_dir(root: str, tenant_id: str, uid: int) -> TenantDirCheck:
    """Verify ``<root>/<tenant_id>`` for tool UID *uid*: about five syscalls, at every accept and launch."""
    root_fd = open_root(root)
    try:
        try:
            fd = os.open(tenant_id, _DIR_FLAGS, dir_fd=root_fd) if is_name(tenant_id) else -1
        except OSError as exc:
            if exc.errno == errno.ENOENT:
                return TenantDirCheck(TenantDir.REFUSED, f"the tenant directory of tenant {tenant_id} does not exist")
            return TenantDirCheck(
                TenantDir.REFUSED,
                f"the tenant directory of tenant {tenant_id} is a symlink or not a directory ({exc.strerror})",
            )
        if fd < 0:
            return TenantDirCheck(TenantDir.REFUSED, f"{tenant_id!r} is not a tenant ID")
        try:
            return tenant_dir_state(fd, tenant_id, uid)
        finally:
            os.close(fd)
    finally:
        os.close(root_fd)


def tenant_dir_state(fd: int, tenant_id: str, uid: int) -> TenantDirCheck:
    info = os.fstat(fd)
    if info.st_uid != worker_uid():
        return TenantDirCheck(
            TenantDir.REFUSED,
            f"the tenant directory of tenant {tenant_id} (tool uid {uid}) belongs to uid {info.st_uid}, not the "
            "worker: refused and never migrated (an operator fixes the owner with a one-off root container)",
        )
    access = posix_acl.get_acl(fd, posix_acl.ACCESS)
    default = posix_acl.get_acl(fd, posix_acl.DEFAULT)
    if (
        stat.S_IMODE(info.st_mode) != TENANT_DIR_MODE
        or not posix_acl.equal(access, posix_acl.tenant_access(uid))
        or not posix_acl.equal(default, posix_acl.tenant_default(uid))
    ):
        return TenantDirCheck(
            TenantDir.MIGRATE,
            f"the tenant directory of tenant {tenant_id} lacks the ACLs of tool uid {uid}",
            info.st_dev,
            info.st_ino,
        )
    return TenantDirCheck(TenantDir.OK, "", info.st_dev, info.st_ino)


def is_name(name: str) -> bool:
    return bool(name) and "/" not in name and "\0" not in name and name not in (".", "..")


def adopted_workspace_ready(path: str, uid: int) -> str:
    """Why an adopted workspace (outside the root) is not ready for tool uid *uid*, "" when it is."""
    try:
        fd = os.open(path, _DIR_FLAGS)
    except OSError as exc:
        return f"the adopted workspace {path} cannot be opened ({exc.strerror})"
    try:
        access = posix_acl.get_acl(fd, posix_acl.ACCESS) or []
    finally:
        os.close(fd)
    if not any(e.tag == posix_acl.USER and e.id == uid and e.perm & 0o7 == 0o7 for e in access):
        return (
            f"the adopted workspace {path} has no ACL for tool uid {uid}: run "
            f"setfacl -R -m u:{uid}:rwX -m d:u:{uid}:rwX -m g:{posix_acl.WORKSPACE_GID}:rwX "
            f"-m d:g:{posix_acl.WORKSPACE_GID}:rwX {path}"
        )
    return ""


# ---------------------------------------------------------------------------
# Tool HOMEs
# ---------------------------------------------------------------------------


def home_base_problems(base: str) -> list[str]:
    """What is wrong with the HOME base (a volume): the worker's, 0711, no ACL, not noexec."""
    try:
        fd = os.open(base, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    except OSError as exc:
        return [f"the tool HOME base {base} cannot be opened ({exc.strerror}): mount the tool_homes volume there"]
    try:
        info = os.fstat(fd)
        problems = []
        if info.st_uid != worker_uid():
            problems.append(f"the tool HOME base {base} belongs to uid {info.st_uid}, not the worker")
        elif stat.S_IMODE(info.st_mode) != HOME_BASE_MODE:
            os.fchmod(fd, HOME_BASE_MODE)
        try:
            acls = (posix_acl.get_acl(fd, posix_acl.ACCESS), posix_acl.get_acl(fd, posix_acl.DEFAULT))
        except OSError as exc:
            problems.append(no_acls_problem(base, exc))
        else:
            if acls != (None, None):
                problems.append(f"the tool HOME base {base} carries an ACL")
        flags = os.fstatvfs(fd).f_flag
        if flags & os.ST_RDONLY:
            problems.append(
                f"the tool HOME base {base} is read-only (no volume mounted?): mount the tool_homes volume there"
            )
        if flags & os.ST_NOEXEC:
            problems.append(
                f"the tool HOME base {base} is mounted noexec (an old tmpfs?): tools run binaries from their HOME; "
                "mount the tool_homes volume there"
            )
        return problems
    finally:
        os.close(fd)


def ensure_home(base: str, uid: int) -> tuple[int, int]:
    """Create ``<base>/<uid>`` once (worker-owned, ``u:<uid>:rwx``, no group, no others) and verify it.

    Returns its (dev, inode).

    Relative to the base's descriptor: the base is the worker's and 0711, so
    no tool can rename or replace a HOME.
    """
    base_fd = os.open(base, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        with contextlib.suppress(FileExistsError):
            os.mkdir(str(uid), 0o700, dir_fd=base_fd)  # no group access before the ACL
        try:
            fd = open_dir_at(base_fd, str(uid))
        except OSError as exc:
            raise ToolIsolationError(f"the HOME of tool uid {uid} cannot be opened ({exc.strerror})") from exc
        try:
            info = os.fstat(fd)
            if info.st_uid != worker_uid():
                raise ToolIsolationError(f"the HOME of tool uid {uid} belongs to uid {info.st_uid}, not the worker")
            if stat.S_IMODE(info.st_mode) != HOME_MODE:
                os.fchmod(fd, HOME_MODE)
            if not posix_acl.equal(posix_acl.get_acl(fd, posix_acl.ACCESS), posix_acl.home_access(uid)):
                posix_acl.set_acl(fd, posix_acl.ACCESS, posix_acl.home_access(uid))
            if posix_acl.get_acl(fd, posix_acl.DEFAULT) is not None:
                posix_acl.remove_acl(fd, posix_acl.DEFAULT)
            return info.st_dev, info.st_ino
        finally:
            os.close(fd)
    finally:
        os.close(base_fd)


def verify_home(base: str, uid: int, expected: tuple[int, int]) -> None:
    """The HOME is still the one ensure_home made: owner, inode and ACL (refused otherwise)."""
    base_fd = os.open(base, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        try:
            fd = open_dir_at(base_fd, str(uid))
        except OSError as exc:
            raise ToolIsolationError(f"the HOME of tool uid {uid} is gone or replaced ({exc.strerror})") from exc
        try:
            info = os.fstat(fd)
            if (info.st_dev, info.st_ino) != expected or info.st_uid != worker_uid():
                raise ToolIsolationError(f"the HOME of tool uid {uid} was replaced")
            if not posix_acl.equal(posix_acl.get_acl(fd, posix_acl.ACCESS), posix_acl.home_access(uid)):
                raise ToolIsolationError(f"the HOME of tool uid {uid} has another ACL")
        finally:
            os.close(fd)
    finally:
        os.close(base_fd)


def temporary_name(prefix: str) -> str:
    """A name for a temporary entry on a volume worker replicas share.

    Not the PID: every replica is PID 1 of its container, and one would
    take over or remove another's entry.
    """
    return f"{prefix}{secrets.token_hex(8)}"


def acl_support_problem(directory: str) -> str:
    """Set and read back a default ACL on a new directory below *directory*; "" when that works."""
    try:
        parent = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    except OSError as exc:
        return f"{directory} cannot be opened ({exc.strerror})"
    name = temporary_name(".cf-acl-check-")
    try:
        try:
            os.mkdir(name, 0o700, dir_fd=parent)
        except OSError as exc:
            return (
                f"the worker cannot create a directory in {directory} ({exc.strerror}): it must be a writable "
                f"volume of the worker (uid {worker_uid()})"
            )
        fd = open_dir_at(parent, name)
        try:
            wanted = posix_acl.tenant_default(20000)
            posix_acl.set_acl(fd, posix_acl.DEFAULT, wanted)
            if not posix_acl.equal(posix_acl.get_acl(fd, posix_acl.DEFAULT), wanted):
                return f"POSIX ACLs on {directory} do not read back"
        finally:
            os.close(fd)
    except OSError as exc:
        return no_acls_problem(directory, exc)
    finally:
        with contextlib.suppress(OSError):
            os.rmdir(name, dir_fd=parent)
        os.close(parent)
    return ""
