"""Symlink-safe access to workspace files (KI-95).

Agents write the workspaces: as the tool user they can create files, symlinks,
hard links, FIFOs and sockets anywhere in a workspace and swap them at any
time. The worker reads and writes workspace files in its own process (the file
tools, the repo map, retrieval and GraphRAG indexers, the benchmark runner)
with rights the tool user does not have, so a workspace path must never
resolve outside the workspace, and a special file must never block the worker
or be read.

Every path is resolved below an open descriptor of the workspace directory,
one component at a time, the way Go's os.Root does in the Go Core:

- Intermediate components are opened with O_DIRECTORY | O_NOFOLLOW relative to
  the descriptor of their parent, so a directory swapped for a symlink after it
  was opened cannot redirect the rest of the walk.
- Absolute paths, and ".." that would climb above the workspace, are refused
  ("path leaves the workspace").
- A symlink is followed only by resolving its target through the same walk,
  at most MAX_SYMLINKS times per path: a relative target that stays inside the
  workspace works; an absolute target (even one that names a place inside) or
  one that climbs above the workspace leaves it. This is os.Root's behaviour,
  so the worker and the Go Core treat a workspace the same way.
- The last component is opened with O_NOFOLLOW | O_NONBLOCK and checked with
  fstat: reads and writes take regular files only, so a FIFO never blocks and
  a socket or device is never used ("not a regular file").
- Walks and listings (os.scandir) work on directory descriptors. A walk is
  iterative, holds at most _MAX_HELD_DIRS descriptors and stops at
  MAX_WALK_DEPTH; it enters a symlinked directory only when asked to (the
  glob and listing tools), through the workspace, never into a directory on
  its own path and at most MAX_SYMLINKS times per path.

Hard links cannot be told apart from regular files; with
fs.protected_hardlinks=1 (the default of systemd-based hosts, and what the
deployment expects) the tool user can only link files it owns or may already
read and write, so a hard link gives the worker no file the tool user could
not read itself.

Only this module touches workspace files directly; a test scans the worker's
code for other file access (tests/test_workspace_fs_scan.py).
"""

from __future__ import annotations

import contextlib
import errno
import os
import posixpath
import secrets
import stat
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterator
    from types import TracebackType
    from typing import BinaryIO

# Symlinks followed per path, as in Go's os.Root.
MAX_SYMLINKS = 8
# Upper bound for the steps of one resolution and for re-resolving a path an
# agent keeps swapping, whatever happens in the workspace meanwhile.
_MAX_STEPS = 255
_MAX_ATTEMPTS = 8

_DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC
_FILE_FLAGS = os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC
_READ_CHUNK = 1 << 20
# Directories below a walk's start it enters at most (deeper ones are counted in WalkStats.too_deep).
MAX_WALK_DEPTH = 128
# Descriptors a walk holds at once; below that depth directories are reopened from the root.
_MAX_HELD_DIRS = 32


class WorkspacePathError(OSError):
    """A workspace path the helper refuses (an OSError, so tool error handling reports its message)."""


class PathLeavesWorkspaceError(WorkspacePathError):
    def __init__(self, rel: str) -> None:
        super().__init__(f"path leaves the workspace: {rel}")


class NotRegularFileError(WorkspacePathError):
    def __init__(self, rel: str) -> None:
        super().__init__(f"not a regular file: {rel}")


class FileTooLargeError(WorkspacePathError):
    def __init__(self, rel: str, size: int, limit: int) -> None:
        super().__init__(f"file too large: {rel} ({size} bytes, max {limit})")


class BlockedPathError(WorkspacePathError):
    def __init__(self, name: str) -> None:
        super().__init__(f"access to {name} is blocked")


def _not_found(rel: str) -> FileNotFoundError:
    return FileNotFoundError(errno.ENOENT, os.strerror(errno.ENOENT), rel)


def _not_a_directory(rel: str) -> NotADirectoryError:
    return NotADirectoryError(errno.ENOTDIR, os.strerror(errno.ENOTDIR), rel)


def _split(rel: str, original: str) -> list[str]:
    """The components of a workspace-relative path ("" and "." dropped)."""
    if "\x00" in rel:
        raise WorkspacePathError(f"invalid path: {original!r}")
    if rel.startswith("/"):
        raise PathLeavesWorkspaceError(original)
    return [part for part in rel.split("/") if part not in ("", ".")]


def workspace_relative(workspace: str, path: str) -> str:
    """*path* relative to *workspace* when it is an absolute path that (lexically) names a place inside it.

    Models often pass absolute paths, as the workspace path reads or in its
    resolved form (pwd -P in a shell); both forms are accepted. Anything else
    is returned unchanged, and the helper refuses absolute paths.
    """
    if not path.startswith("/"):
        return path
    target = posixpath.normpath(path)
    absolute = posixpath.normpath(os.path.abspath(workspace))
    for base in dict.fromkeys((absolute, os.path.realpath(absolute))):
        if target == base:
            return "."
        prefix = base.rstrip("/") + "/"
        if target.startswith(prefix):
            return target[len(prefix) :]
    return path


@dataclass(frozen=True)
class ListedEntry:
    """One entry of a directory listing.

    is_dir is true for a directory, and for a symlink that resolves to a
    directory inside the workspace; target is then the workspace-relative
    directory it resolves to.
    """

    name: str
    is_dir: bool
    is_symlink: bool
    target: str = ""


@dataclass
class WalkStats:
    """What a walk left out; the caller logs it once."""

    too_deep: int = 0  # directories deeper than max_depth, not entered
    loops: int = 0  # symlinked directories not entered: back to the walk's own path, or too many on it
    errors: int = 0  # directories that could not be opened or listed (removed or swapped meanwhile)


class WorkspaceRoot:
    """An open workspace directory; every path is resolved below it (see the module docstring).

    blocked_names are directory or file names (compared case-insensitively)
    that a resolved path may not contain, wherever they come from (the path or
    a symlink target); the file tools block ".git" (KI-77).
    """

    def __init__(self, path: str, *, blocked_names: frozenset[str] = frozenset()) -> None:
        self.path = os.path.normpath(path)
        self._blocked = frozenset(name.lower() for name in blocked_names)
        try:
            # O_NOFOLLOW: the workspace directory itself must not be a symlink.
            # The tool user can replace its workspace (the tenant directory is
            # group-writable); the components above it are the operator's.
            self._fd = os.open(self.path, _DIR_FLAGS)
        except OSError as exc:
            if exc.errno in (errno.ELOOP, errno.ENOTDIR) and os.path.islink(self.path):
                raise WorkspacePathError(f"workspace directory {self.path} is a symlink") from None
            raise

    @classmethod
    def operator_dir(cls, path: str) -> WorkspaceRoot:
        """A root for a directory the operator configured (the knowledge content root, the datasets directory).

        Unlike a workspace, its own path may contain symlinks; names below it
        are resolved like workspace names.
        """
        root = cls.__new__(cls)
        root.path = os.path.normpath(path)
        root._blocked = frozenset()
        root._fd = os.open(root.path, _DIR_FLAGS & ~os.O_NOFOLLOW)
        return root

    @property
    def fd(self) -> int:
        return self._fd

    def close(self) -> None:
        if self._fd >= 0:
            os.close(self._fd)
            self._fd = -1

    def __enter__(self) -> WorkspaceRoot:
        return self

    def __exit__(
        self, exc_type: type[BaseException] | None, exc: BaseException | None, tb: TracebackType | None
    ) -> None:
        self.close()

    # ------------------------------------------------------------------
    # Resolution
    # ------------------------------------------------------------------

    def _release(self, fd: int) -> None:
        if fd != self._fd:
            os.close(fd)

    def _check_blocked(self, name: str) -> None:
        if name.lower() in self._blocked:
            raise BlockedPathError(name.lower())

    def _locate(self, rel: str, *, follow_final: bool = True, make_parents: bool = False) -> tuple[int, str, list[str]]:
        """Open the directory that holds the last component of *rel*.

        Returns (directory descriptor, last name, resolved components). The
        name is "" when *rel* names the workspace itself. The descriptor is the
        root's own or a new one; release it with _release. Symlinks are
        resolved through the walk, the last component's too unless
        follow_final is false. A missing last component is not an error (the
        caller creates it or reports it); missing directories are created with
        make_parents.
        """
        parts = _split(rel, rel)
        for part in parts:  # names a symlink brings in are checked on the walk
            self._check_blocked(part)
        fd = self._fd
        i = steps = links = 0
        try:
            while parts:
                steps += 1
                if steps > _MAX_STEPS:
                    raise WorkspacePathError(f"path too complex: {rel}")
                part = parts[i]
                if part == "..":
                    # parts[i - 1] is a real directory the walk opened: climb
                    # lexically and walk again from the root (a held
                    # descriptor's ".." could lead anywhere once it is moved).
                    if i == 0:
                        raise PathLeavesWorkspaceError(rel)
                    parts = parts[: i - 1] + parts[i + 1 :]
                else:
                    self._check_blocked(part)
                    if i == len(parts) - 1:
                        target = _symlink_target(part, fd) if follow_final else None
                        if target is None:
                            return fd, part, parts
                    else:
                        step = _enter(part, fd, rel, make_parents=make_parents)
                        if step is None:
                            continue  # created a missing directory: open it
                        if isinstance(step, int):
                            self._release(fd)
                            fd, i = step, i + 1
                            continue
                        target = step
                    links += 1
                    if links > MAX_SYMLINKS:
                        raise WorkspacePathError(f"too many levels of symbolic links: {rel}")
                    parts = parts[:i] + _split(target, rel) + parts[i + 1 :]
                # Walk again from the root with the rewritten components.
                self._release(fd)
                fd, i = self._fd, 0
        except BaseException:
            self._release(fd)
            raise
        return fd, "", []

    def _open_final(self, rel: str, flags: int) -> int:
        """Open the existing last component of *rel* (symlinks inside the workspace followed), never blocking."""
        for _ in range(_MAX_ATTEMPTS):
            dir_fd, name, _parts = self._locate(rel)
            try:
                if not name:
                    return os.open(".", flags | _FILE_FLAGS, dir_fd=self._fd)
                try:
                    return os.open(name, flags | _FILE_FLAGS, dir_fd=dir_fd)
                except FileNotFoundError:
                    raise _not_found(rel) from None
                except OSError as exc:
                    if exc.errno == errno.ELOOP:
                        continue  # swapped for a symlink after _locate looked: resolve again
                    if exc.errno in (errno.ENXIO, errno.EISDIR):
                        raise NotRegularFileError(rel) from None  # a socket, a FIFO without reader, a directory
                    raise
            finally:
                self._release(dir_fd)
        raise WorkspacePathError(f"path keeps changing: {rel}")

    def resolve(self, rel: str) -> str:
        """The workspace-relative path *rel* resolves to ("." for the workspace); it must exist."""
        dir_fd, name, parts = self._locate(rel)
        try:
            if name:
                try:
                    os.stat(name, dir_fd=dir_fd, follow_symlinks=False)
                except FileNotFoundError:
                    raise _not_found(rel) from None
        finally:
            self._release(dir_fd)
        return "/".join(parts) or "."

    def stat(self, rel: str) -> os.stat_result:
        """stat of what *rel* resolves to (symlinks inside the workspace followed)."""
        dir_fd, name, _parts = self._locate(rel)
        try:
            if not name:
                return os.fstat(self._fd)
            try:
                return os.stat(name, dir_fd=dir_fd, follow_symlinks=False)
            except FileNotFoundError:
                raise _not_found(rel) from None
        finally:
            self._release(dir_fd)

    def is_file(self, rel: str) -> bool:
        """True when *rel* resolves to a regular file inside the workspace."""
        try:
            return stat.S_ISREG(self.stat(rel).st_mode)
        except OSError:
            return False

    def is_dir(self, rel: str) -> bool:
        """True when *rel* resolves to a directory inside the workspace."""
        try:
            return stat.S_ISDIR(self.stat(rel).st_mode)
        except OSError:
            return False

    # ------------------------------------------------------------------
    # Files
    # ------------------------------------------------------------------

    def read_bytes(self, rel: str, *, max_bytes: int | None = None) -> bytes:
        """The content of the regular file *rel*; FileTooLargeError above max_bytes."""
        return _read_regular(self._open_final(rel, os.O_RDONLY), rel, max_bytes)

    def read_entry(self, dir_fd: int, name: str, rel: str, *, max_bytes: int | None = None) -> bytes:
        """The content of the regular file *name* of a walked directory (*rel* is its workspace path).

        Opened relative to the walk's descriptor without following a symlink;
        a symlink is resolved through the root like any other path.
        """
        self._check_blocked(name)
        try:
            fd = os.open(name, os.O_RDONLY | _FILE_FLAGS, dir_fd=dir_fd)
        except FileNotFoundError:
            raise _not_found(rel) from None
        except OSError as exc:
            if exc.errno == errno.ELOOP:
                return self.read_bytes(rel, max_bytes=max_bytes)
            if exc.errno in (errno.ENXIO, errno.EISDIR):
                raise NotRegularFileError(rel) from None
            raise
        return _read_regular(fd, rel, max_bytes)

    def open_binary(self, rel: str) -> BinaryIO:
        """The regular file *rel*, opened for buffered binary reading; close it."""
        fd = self._open_final(rel, os.O_RDONLY)
        try:
            if not stat.S_ISREG(os.fstat(fd).st_mode):
                raise NotRegularFileError(rel)
            return os.fdopen(fd, "rb")
        except BaseException:
            os.close(fd)
            raise

    def read_text(
        self, rel: str, *, max_bytes: int | None = None, encoding: str = "utf-8", errors: str = "strict"
    ) -> str:
        return self.read_bytes(rel, max_bytes=max_bytes).decode(encoding, errors)

    def write_bytes(self, rel: str, data: bytes, *, make_parents: bool = False, mode: int = 0o666) -> None:
        """Write *data* to *rel*: an existing regular file is overwritten in place (owner and mode kept,
        symlinks inside the workspace followed), a missing one is created (mode masked by the umask).
        """
        for _ in range(_MAX_ATTEMPTS):
            dir_fd, name, _parts = self._locate(rel, make_parents=make_parents)
            try:
                if not name:
                    raise NotRegularFileError(rel)
                try:
                    fd = os.open(name, os.O_WRONLY | _FILE_FLAGS, dir_fd=dir_fd)
                except FileNotFoundError:
                    try:
                        fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | _FILE_FLAGS, mode, dir_fd=dir_fd)
                    except FileExistsError:
                        continue  # created meanwhile (perhaps as a symlink): resolve again
                except OSError as exc:
                    if exc.errno == errno.ELOOP:
                        continue
                    if exc.errno in (errno.ENXIO, errno.EISDIR):
                        raise NotRegularFileError(rel) from None
                    raise
            finally:
                self._release(dir_fd)
            try:
                if not stat.S_ISREG(os.fstat(fd).st_mode):
                    raise NotRegularFileError(rel)
                os.ftruncate(fd, 0)
                _write_all(fd, data)
            finally:
                os.close(fd)
            return
        raise WorkspacePathError(f"path keeps changing: {rel}")

    def write_text(self, rel: str, text: str, *, make_parents: bool = False, encoding: str = "utf-8") -> None:
        self.write_bytes(rel, text.encode(encoding), make_parents=make_parents)

    def replace_bytes(self, rel: str, data: bytes, *, make_parents: bool = False, mode: int = 0o666) -> None:
        """Put a new regular file with *data* at *rel*, whatever is there now (a symlink, a FIFO, a file).

        The file is written under a temporary name in the same directory
        descriptor and renamed over *rel*; the last component is never followed.
        """
        dir_fd, name, _parts = self._locate(rel, follow_final=False, make_parents=make_parents)
        try:
            if not name:
                raise NotRegularFileError(rel)
            temporary = f".codeforge-{secrets.token_hex(8)}.tmp"
            fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | _FILE_FLAGS, mode, dir_fd=dir_fd)
            try:
                try:
                    _write_all(fd, data)
                finally:
                    os.close(fd)
                os.rename(temporary, name, src_dir_fd=dir_fd, dst_dir_fd=dir_fd)
            except BaseException:
                with contextlib.suppress(OSError):
                    os.unlink(temporary, dir_fd=dir_fd)
                raise
        finally:
            self._release(dir_fd)

    # ------------------------------------------------------------------
    # Directories
    # ------------------------------------------------------------------

    def _open_dir(self, rel: str) -> tuple[int, str]:
        """A new descriptor of the directory *rel* resolves to, and its resolved path."""
        for _ in range(_MAX_ATTEMPTS):
            dir_fd, name, parts = self._locate(rel)
            try:
                if not name:
                    return os.open(".", _DIR_FLAGS, dir_fd=self._fd), "."
                try:
                    return os.open(name, _DIR_FLAGS, dir_fd=dir_fd), "/".join(parts)
                except FileNotFoundError:
                    raise _not_found(rel) from None
                except OSError as exc:
                    # O_DIRECTORY | O_NOFOLLOW answers ENOTDIR for a symlink too.
                    if exc.errno not in (errno.ELOOP, errno.ENOTDIR):
                        raise
                    if not _is_symlink(name, dir_fd):
                        raise _not_a_directory(rel) from None
                    continue  # swapped for a symlink after _locate looked: resolve again
            finally:
                self._release(dir_fd)
        raise WorkspacePathError(f"path keeps changing: {rel}")

    def subroot(self, rel: str) -> WorkspaceRoot:
        """A root for the directory *rel* resolves to (symlinks inside followed); its paths stay below it.

        Raises PathLeavesWorkspaceError when *rel* leaves this root,
        NotADirectoryError when it is not a directory.
        """
        fd, resolved = self._open_dir(rel)
        sub = WorkspaceRoot.__new__(WorkspaceRoot)
        sub.path = os.path.normpath(os.path.join(self.path, resolved))
        sub._blocked = self._blocked
        sub._fd = fd
        return sub

    def list_dir(self, rel: str = ".") -> list[ListedEntry]:
        """The entries of the directory *rel* (unsorted); symlinks are classified, never descended."""
        fd, base = self._open_dir(rel)
        try:
            with os.scandir(fd) as it:
                found = [(entry.name, entry.is_symlink(), entry.is_dir(follow_symlinks=False)) for entry in it]
        finally:
            os.close(fd)
        entries: list[ListedEntry] = []
        for name, is_symlink, is_dir in found:
            target = ""
            if is_symlink:
                target = self._symlinked_dir(name if base == "." else f"{base}/{name}")
                is_dir = bool(target)
            entries.append(ListedEntry(name=name, is_dir=is_dir, is_symlink=is_symlink, target=target))
        return entries

    def _symlinked_dir(self, rel: str) -> str:
        """The workspace-relative directory the symlink *rel* resolves to, "" when it is no directory inside."""
        try:
            resolved = self.resolve(rel)
            return resolved if self.is_dir(resolved) else ""
        except OSError:
            return ""

    def walk(
        self,
        rel: str = ".",
        *,
        follow_dir_symlinks: bool = False,
        max_depth: int = MAX_WALK_DEPTH,
        stats: WalkStats | None = None,
    ) -> Iterator[tuple[str, list[str], list[str], int]]:
        """Walk the directory *rel* top-down, iteratively: (directory, dirnames, filenames, descriptor).

        dirnames are the subdirectories and the symlinks that resolve to a
        directory inside the workspace; filenames everything else. Prune
        dirnames in place to skip directories. A symlinked directory is
        entered only with follow_dir_symlinks: resolved through the workspace,
        not when it leads to a directory on the walk's own path, and at most
        MAX_SYMLINKS on one path; the walk reports it under the symlink's path.
        Directories deeper than max_depth below *rel* are not entered. What the
        walk left out is counted in *stats*. Open files relative to the
        descriptor (read_entry); it is valid until the next step of the walk.
        """
        stats = stats if stats is not None else WalkStats()
        fd, resolved = self._open_dir(rel)
        # Reported under the path asked for (a symlinked start keeps its name).
        start = resolved if ".." in rel.split("/") else posixpath.normpath(rel or ".")
        opened: tuple[str, int, int, int] | None = (start, fd, 0, 0)  # path, descriptor, depth, hops
        stack: list[_WalkDir] = []
        try:
            while opened is not None:
                current = self._walk_list(*opened, stats)
                if current is not None:
                    try:
                        yield current.path, current.dirnames, current.filenames, current.fd
                    except BaseException:
                        os.close(current.fd)
                        raise
                    self._walk_keep(current, stack, follow_dir_symlinks, max_depth, stats)
                opened = self._walk_next(stack, stats)
        finally:
            for walked in stack:
                if walked.fd >= 0:
                    os.close(walked.fd)

    def _walk_list(self, path: str, fd: int, depth: int, hops: int, stats: WalkStats) -> _WalkDir | None:
        """List the walked directory *path* (its descriptor is closed when that fails)."""
        try:
            ident = _ident(fd)
            with os.scandir(fd) as it:
                found = [(entry.name, entry.is_symlink(), entry.is_dir(follow_symlinks=False)) for entry in it]
        except OSError:
            os.close(fd)
            stats.errors += 1
            return None
        walked = _WalkDir(path=path, fd=fd, depth=depth, hops=hops, ident=ident)
        for name, is_symlink, is_dir in found:
            if is_symlink and self._symlinked_dir(name if path == "." else f"{path}/{name}"):
                walked.dirnames.append(name)
                walked.links.add(name)
            elif is_dir and not is_symlink:
                walked.dirnames.append(name)
            else:
                walked.filenames.append(name)
        return walked

    @staticmethod
    def _walk_keep(
        walked: _WalkDir, stack: list[_WalkDir], follow_dir_symlinks: bool, max_depth: int, stats: WalkStats
    ) -> None:
        """Queue the subdirectories the caller kept in dirnames (closing the directory when there are none)."""
        names = [name for name in walked.dirnames if follow_dir_symlinks or name not in walked.links]
        if names and walked.depth >= max_depth:
            stats.too_deep += len(names)
            names = []
        if not names:
            os.close(walked.fd)
            return
        walked.pending = names[::-1]
        if len(stack) >= _MAX_HELD_DIRS:
            os.close(walked.fd)  # its subdirectories are reopened from the root
            walked.fd = -1
        stack.append(walked)

    def _walk_next(self, stack: list[_WalkDir], stats: WalkStats) -> tuple[str, int, int, int] | None:
        """Open the next directory of the walk: (path, descriptor, depth, hops), None when it is done."""
        while stack:
            top = stack[-1]
            if not top.pending:
                if top.fd >= 0:
                    os.close(top.fd)
                stack.pop()
                continue
            name = top.pending.pop()
            path = name if top.path == "." else f"{top.path}/{name}"
            hops = top.hops + (name in top.links)
            if hops > MAX_SYMLINKS:
                stats.loops += 1
                continue
            fd = self._walk_open(top, name, path)
            if fd < 0:
                stats.errors += 1
            elif _ident(fd) in {walked.ident for walked in stack}:
                os.close(fd)
                stats.loops += 1
            else:
                return path, fd, top.depth + 1, hops
        return None

    def _walk_open(self, parent: _WalkDir, name: str, path: str) -> int:
        """A descriptor of the subdirectory *name* of *parent*, -1 when it cannot be opened.

        A real directory is opened relative to its parent's descriptor without
        following a symlink; a symlinked one, or one whose parent was closed,
        is resolved through the workspace.
        """
        try:
            if parent.fd >= 0 and name not in parent.links:
                return os.open(name, _DIR_FLAGS, dir_fd=parent.fd)
            return self._open_dir(path)[0]
        except OSError:
            return -1


@dataclass
class _WalkDir:
    """A directory of a walk; while pending holds subdirectories to enter, its descriptor stays open
    (fd is -1 when the walk closed it to bound its descriptors: they are reopened from the root)."""

    path: str
    fd: int
    depth: int
    hops: int  # symlinked directories followed on the way here
    ident: tuple[int, int]
    dirnames: list[str] = field(default_factory=list)
    filenames: list[str] = field(default_factory=list)
    links: set[str] = field(default_factory=set)  # the dirnames that are symlinks
    pending: list[str] = field(default_factory=list)  # reversed: the next one is last


def _ident(fd: int) -> tuple[int, int]:
    info = os.fstat(fd)
    return info.st_dev, info.st_ino


def _read_regular(fd: int, rel: str, max_bytes: int | None) -> bytes:
    """Read the open file *fd* (closed afterwards): a regular file of at most max_bytes."""
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode):
            raise NotRegularFileError(rel)
        if max_bytes is not None and info.st_size > max_bytes:
            raise FileTooLargeError(rel, info.st_size, max_bytes)
        chunks: list[bytes] = []
        size = 0
        while chunk := os.read(fd, _READ_CHUNK):
            size += len(chunk)
            if max_bytes is not None and size > max_bytes:
                raise FileTooLargeError(rel, size, max_bytes)  # grew after the fstat
            chunks.append(chunk)
        return b"".join(chunks)
    finally:
        os.close(fd)


def _symlink_target(name: str, dir_fd: int) -> str | None:
    """The target of *name* when it is a symlink, else None (a missing name included)."""
    try:
        info = os.stat(name, dir_fd=dir_fd, follow_symlinks=False)
    except FileNotFoundError:
        return None
    if not stat.S_ISLNK(info.st_mode):
        return None
    return os.readlink(name, dir_fd=dir_fd)


def _enter(name: str, dir_fd: int, rel: str, *, make_parents: bool) -> int | str | None:
    """Open the directory *name* without following it: its descriptor, or the target when it is a symlink.

    None when it was missing and has been created (make_parents).
    """
    try:
        return os.open(name, _DIR_FLAGS, dir_fd=dir_fd)
    except FileNotFoundError:
        if not make_parents:
            raise _not_found(rel) from None
        with contextlib.suppress(FileExistsError):
            os.mkdir(name, 0o777, dir_fd=dir_fd)
        return None
    except OSError as exc:
        # O_DIRECTORY | O_NOFOLLOW answers ENOTDIR (or ELOOP) for a symlink.
        if exc.errno not in (errno.ELOOP, errno.ENOTDIR):
            raise
        try:
            return os.readlink(name, dir_fd=dir_fd)
        except OSError:
            raise _not_a_directory(rel) from None


def _is_symlink(name: str, dir_fd: int) -> bool:
    try:
        return stat.S_ISLNK(os.stat(name, dir_fd=dir_fd, follow_symlinks=False).st_mode)
    except OSError:
        return False


def _write_all(fd: int, data: bytes) -> None:
    view = memoryview(data)
    while view:
        written = os.write(fd, view)
        view = view[written:]
