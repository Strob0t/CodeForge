"""The worker's POSIX ACL codec (KI-96) against the vectors the Go Core is tested with."""

from __future__ import annotations

import errno
import json
import os
from pathlib import Path

import pytest

from codeforge import posix_acl
from codeforge.posix_acl import Entry

VECTORS = Path(__file__).resolve().parents[2] / "internal" / "workspaceacl" / "testdata" / "acl_vectors.json"
_TAGS = {
    "user_obj": posix_acl.USER_OBJ,
    "user": posix_acl.USER,
    "group_obj": posix_acl.GROUP_OBJ,
    "group": posix_acl.GROUP,
    "mask": posix_acl.MASK,
    "other": posix_acl.OTHER,
}


def _vectors() -> dict[str, dict[str, object]]:
    return {v["name"]: v for v in json.loads(VECTORS.read_text())["vectors"]}


def _entries(vector: dict[str, object]) -> list[Entry]:
    return [
        Entry(_TAGS[e["tag"]], e["perm"], e.get("id", posix_acl.UNDEFINED_ID))  # type: ignore[index, union-attr]
        for e in vector["entries"]  # type: ignore[union-attr]
    ]


@pytest.mark.parametrize("name", sorted(_vectors()))
def test_codec_matches_the_shared_vectors(name: str) -> None:
    vector = _vectors()[name]
    entries = _entries(vector)
    assert posix_acl.encode(entries).hex() == vector["hex"]
    assert posix_acl.equal(posix_acl.decode(bytes.fromhex(vector["hex"])), entries)  # type: ignore[arg-type]


def test_builders_match_the_vectors() -> None:
    vectors = _vectors()
    assert posix_acl.equal(posix_acl.tenant_access(20000), _entries(vectors["tenant_access"]))
    assert posix_acl.equal(posix_acl.tenant_default(20000, 10010), _entries(vectors["tenant_default"]))
    assert posix_acl.equal(posix_acl.home_access(29999), _entries(vectors["home_access"]))


@pytest.mark.parametrize("data", [b"", b"\x01\x02\x03", b"\x02\x00\x00\x00\x01", b"\x03\x00\x00\x00"])
def test_decode_rejects_garbage(data: bytes) -> None:
    with pytest.raises(ValueError):
        posix_acl.decode(data)


def test_equal_treats_none_as_no_acl() -> None:
    assert posix_acl.equal(None, None)
    assert not posix_acl.equal(None, [])
    assert posix_acl.equal([], [])


def test_set_and_get_on_a_descriptor(tmp_path: Path) -> None:
    directory = tmp_path / "d"
    directory.mkdir()
    fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
    try:
        try:
            posix_acl.set_acl(fd, posix_acl.DEFAULT, posix_acl.tenant_default(20000))
        except OSError as exc:
            if exc.errno == errno.EOPNOTSUPP:
                pytest.skip("no POSIX ACLs on the test file system")
            raise
        posix_acl.set_acl(fd, posix_acl.ACCESS, posix_acl.tenant_access(20000))
        assert posix_acl.equal(posix_acl.get_acl(fd, posix_acl.ACCESS), posix_acl.tenant_access(20000))
        assert posix_acl.equal(posix_acl.get_acl(fd, posix_acl.DEFAULT), posix_acl.tenant_default(20000))
        posix_acl.remove_acl(fd, posix_acl.DEFAULT)
        posix_acl.remove_acl(fd, posix_acl.DEFAULT)  # already gone: no error
        assert posix_acl.get_acl(fd, posix_acl.DEFAULT) is None
    finally:
        os.close(fd)
