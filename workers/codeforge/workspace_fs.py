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
- Walks (os.fwalk) and listings (os.scandir) work on directory descriptors and
  never descend into a symlink.

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
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterator
    from types import TracebackType

# Symlinks followed per path, as in Go's os.Root.
MAX_SYMLINKS = 8
# Upper bound for the steps of one resolution and for re-resolving a path an
# agent keeps swapping, whatever happens in the workspace meanwhile.
_MAX_STEPS = 255
_MAX_ATTEMPTS = 8

_DIR_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC
_FILE_FLAGS = os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC
_READ_CHUNK = 1 << 20


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

    Models often pass absolute paths. Anything else is returned unchanged, and
    the helper refuses absolute paths.
    """
    if not path.startswith("/"):
        return path
    base = posixpath.normpath(os.path.abspath(workspace))
    target = posixpath.normpath(path)
    if target == base:
        return "."
    if target.startswith(base.rstrip("/") + "/"):
        return target[len(base.rstrip("/")) + 1 :]
    return path


@dataclass(frozen=True)
class ListedEntry:
    """One entry of a directory listing.

    is_dir is true for a directory, and for a symlink that resolves to a
    directory inside the workspace (listings never descend into symlinks).
    """

    name: str
    is_dir: bool
    is_symlink: bool


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
        fd = self._open_final(rel, os.O_RDONLY)
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
            if is_symlink:
                is_dir = self.is_dir(name if base == "." else f"{base}/{name}")
            entries.append(ListedEntry(name=name, is_dir=is_dir, is_symlink=is_symlink))
        return entries

    def walk(self, rel: str = ".") -> Iterator[tuple[str, list[str], list[str], int]]:
        """os.fwalk below the directory *rel*, top-down, never following a symlink.

        Yields (workspace-relative directory, dirnames, filenames, directory
        descriptor); prune dirnames in place to skip directories. As with
        os.fwalk, dirnames includes symlinks to directories (not descended) and
        filenames everything else: open files through this root, or relative
        to the descriptor with O_NOFOLLOW.
        """
        fd, base = self._open_dir(rel)
        try:
            for dirpath, dirnames, filenames, dir_fd in os.fwalk(".", follow_symlinks=False, dir_fd=fd):
                yield posixpath.normpath(posixpath.join(base, dirpath)), dirnames, filenames, dir_fd
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
