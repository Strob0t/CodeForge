"""POSIX ACLs as the kernel's extended attributes, without libacl or setfacl (KI-96, ADR-018).

``system.posix_acl_access`` and ``system.posix_acl_default`` hold a version
(2) and entries of tag, permissions and ID (linux/posix_acl_xattr.h), sorted
by tag, then ID. The worker sets them on descriptors only (``os.setxattr``
on an fd), never by a path a tool could redirect. The Go Core has the same
codec (internal/workspaceacl); both are tested against
internal/workspaceacl/testdata/acl_vectors.json.

Standard library only: the owner-run walker (tool_walk.py) loads this file
as the tool user, without CodeForge on its path.
"""

from __future__ import annotations

import errno
import os
import struct
from dataclasses import dataclass

USER_OBJ = 0x01
USER = 0x02
GROUP_OBJ = 0x04
GROUP = 0x08
MASK = 0x10
OTHER = 0x20
UNDEFINED_ID = 0xFFFFFFFF

ACCESS = "system.posix_acl_access"
DEFAULT = "system.posix_acl_default"

# The workspace group of the Go Core and the worker.
WORKSPACE_GID = 10010

_VERSION = 2
_HEADER = struct.Struct("<I")
_ENTRY = struct.Struct("<HHI")


@dataclass(frozen=True, order=True)
class Entry:
    """One ACL entry: tag, permissions (r=4, w=2, x=1) and, for named users and groups, the ID."""

    tag: int
    perm: int
    id: int = UNDEFINED_ID


def _sorted(entries: list[Entry]) -> list[Entry]:
    return sorted(entries, key=lambda e: (e.tag, e.id))


def encode(entries: list[Entry]) -> bytes:
    return _HEADER.pack(_VERSION) + b"".join(_ENTRY.pack(e.tag, e.perm, e.id) for e in _sorted(entries))


def decode(data: bytes) -> list[Entry]:
    if len(data) < _HEADER.size or (len(data) - _HEADER.size) % _ENTRY.size:
        raise ValueError(f"{len(data)} bytes are no POSIX ACL")
    (version,) = _HEADER.unpack_from(data)
    if version != _VERSION:
        raise ValueError(f"POSIX ACL version {version}, expected {_VERSION}")
    return [Entry(*_ENTRY.unpack_from(data, offset)) for offset in range(_HEADER.size, len(data), _ENTRY.size)]


def equal(a: list[Entry] | None, b: list[Entry] | None) -> bool:
    return (a is None) == (b is None) and _sorted(a or []) == _sorted(b or [])


def tenant_access(tool_uid: int) -> list[Entry]:
    """A tenant directory's access ACL: the tool UID may only search it."""
    return [Entry(USER_OBJ, 7), Entry(USER, 1, tool_uid), Entry(GROUP_OBJ, 7), Entry(MASK, 7), Entry(OTHER, 0)]


def tenant_default(tool_uid: int, workspace_gid: int = WORKSPACE_GID) -> list[Entry]:
    """A tenant directory's default ACL (what its projects inherit); a project's access ACL."""
    return [
        Entry(USER_OBJ, 7),
        Entry(USER, 7, tool_uid),
        Entry(GROUP_OBJ, 7),
        Entry(GROUP, 7, workspace_gid),
        Entry(MASK, 7),
        Entry(OTHER, 0),
    ]


def home_access(tool_uid: int) -> list[Entry]:
    """A tool HOME's access ACL: the worker owns it, the tool UID uses it."""
    return [Entry(USER_OBJ, 7), Entry(USER, 7, tool_uid), Entry(GROUP_OBJ, 0), Entry(MASK, 7), Entry(OTHER, 0)]


def get_acl(target: int | str, name: str) -> list[Entry] | None:
    """The ACL *name* of a descriptor (or a /proc/self/fd path); None when it has none."""
    try:
        data = os.getxattr(target, name)
    except OSError as exc:
        if exc.errno == errno.ENODATA:
            return None
        raise
    return decode(data)


def set_acl(target: int | str, name: str, entries: list[Entry]) -> None:
    """Set the ACL *name* on a descriptor (or a /proc/self/fd path); the caller must own the entry."""
    os.setxattr(target, name, encode(entries))


def remove_acl(target: int | str, name: str) -> None:
    try:
        os.removexattr(target, name)
    except OSError as exc:
        if exc.errno != errno.ENODATA:
            raise
