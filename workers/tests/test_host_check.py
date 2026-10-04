"""The host preflight of per-tenant tool isolation (KI-96 D12, scripts/check-host.sh).

codeforge.host_check runs in the new worker image with the production
service definition and reports what the host gives tool isolation: the
Landlock ABI (and whether a seccomp profile hides it), POSIX ACLs on both
volumes, their file systems and mount flags, the /tmp tmpfs and the result
of the isolation check itself. Any failure exits non-zero.
"""

from __future__ import annotations

import errno
import json
import os
from typing import TYPE_CHECKING

import pytest

from codeforge import host_check
from codeforge.tool_process import IsolationConfig, IsolationStatus

if TYPE_CHECKING:
    from pathlib import Path

MOUNTINFO = """\
22 1 0:21 / / ro,relatime master:1 - overlay overlay rw,lowerdir=/x
23 22 253:0 /docker/volumes/w/_data /data/workspaces rw,relatime - ext4 /dev/vda rw
24 22 0:30 / /home/codeforge-tools rw,nosuid,nodev,noexec,relatime - tmpfs tmpfs rw,size=65536k
25 22 0:31 / /tmp rw,nosuid,nodev,noexec,relatime - tmpfs tmpfs rw,mode=1771,uid=10001,gid=10010
26 23 253:0 /docker/volumes/w/_data/sub /data/workspaces/sub\\040dir rw - ext4 /dev/vda rw
"""
# The test's directories (below /tmp) on an executable root file system.
ROOT_ONLY = "22 1 0:21 / / rw,relatime - ext4 /dev/vda rw\n"


def test_mounts_are_parsed_and_the_longest_mount_point_wins() -> None:
    mounts = host_check.parse_mountinfo(MOUNTINFO)
    assert host_check.mount_of("/data/workspaces/t1/p", mounts).fstype == "ext4"
    assert host_check.mount_of("/data/workspaces/sub dir/x", mounts).point == "/data/workspaces/sub dir"
    homes = host_check.mount_of("/home/codeforge-tools", mounts)
    assert (homes.fstype, homes.noexec) == ("tmpfs", True)
    assert host_check.mount_of("/etc", mounts).point == "/"
    assert not host_check.mount_of("/data/workspaces", mounts).noexec


@pytest.mark.parametrize(
    ("code", "fragment"),
    [
        (errno.ENOSYS, "seccomp"),
        (errno.EOPNOTSUPP, "lsm="),
        (errno.EPERM, "errno"),
    ],
)
def test_a_missing_landlock_is_explained(code: int, fragment: str) -> None:
    assert fragment in host_check.landlock_explanation(0, code)


def test_a_landlock_abi_is_named() -> None:
    assert host_check.landlock_explanation(7, 0) == "ABI 7"


@pytest.mark.parametrize(
    ("mode", "uid", "problem"),
    [
        (0o1771, 10001, ""),
        (0o1777, 0, "/tmp has mode 1777, expected 1771"),
        (0o1771, 0, "/tmp belongs to uid 0, expected the worker (10001)"),
    ],
)
def test_tmp_must_be_closed_to_tools(mode: int, uid: int, problem: str) -> None:
    result = host_check.tmp_problem(mode, uid, worker_uid=10001)
    if problem:
        assert problem in result
    else:
        assert result == ""


def _config(tmp_path: Path) -> IsolationConfig:
    root = tmp_path / "workspaces"
    homes = tmp_path / "homes"
    root.mkdir()
    homes.mkdir()
    return IsolationConfig(mode="required", workspace_root=str(root), home_base=str(homes))


def test_a_ready_host_passes(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    config = _config(tmp_path)
    monkeypatch.setattr(host_check, "landlock_state", lambda: (7, 0))
    monkeypatch.setattr(host_check, "acl_problem", lambda _path: "")
    monkeypatch.setattr(host_check, "tmp_state", lambda: (0o1771, os.getuid()))
    monkeypatch.setattr(host_check, "read_mounts", lambda: host_check.parse_mountinfo(ROOT_ONLY))
    status = IsolationStatus(config=config, ready=True, landlock_abi=7)
    monkeypatch.setattr(host_check, "configure_tool_isolation", lambda _config: status)
    report, problems = host_check.check(config)
    assert problems == []
    assert report["landlock"] == "ABI 7"
    assert report["tool isolation"] == "ready (Landlock ABI 7)"


def test_every_failure_is_reported(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    config = _config(tmp_path)
    mounts = host_check.parse_mountinfo(MOUNTINFO.replace(str(config.home_base), "/x"))
    monkeypatch.setattr(host_check, "landlock_state", lambda: (0, errno.ENOSYS))
    monkeypatch.setattr(host_check, "acl_problem", lambda path: f"no POSIX ACLs on {path}")
    monkeypatch.setattr(host_check, "tmp_state", lambda: (0o1777, 0))
    monkeypatch.setattr(host_check, "read_mounts", lambda: mounts)
    status = IsolationStatus(config=config, ready=False, reason="the probe failed")
    monkeypatch.setattr(host_check, "configure_tool_isolation", lambda _config: status)
    _, problems = host_check.check(config)
    text = "\n".join(problems)
    assert "seccomp" in text
    assert f"no POSIX ACLs on {config.workspace_root}" in text
    assert f"no POSIX ACLs on {config.home_base}" in text
    assert "/tmp has mode 1777" in text
    assert "tool isolation not ready: the probe failed" in text


def test_a_noexec_home_base_fails(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    config = _config(tmp_path)
    mounts = host_check.parse_mountinfo(f"24 22 0:30 / {config.home_base} rw,noexec - tmpfs tmpfs rw\n")
    monkeypatch.setattr(host_check, "landlock_state", lambda: (7, 0))
    monkeypatch.setattr(host_check, "acl_problem", lambda _path: "")
    monkeypatch.setattr(host_check, "tmp_state", lambda: (0o1771, os.getuid()))
    monkeypatch.setattr(host_check, "read_mounts", lambda: mounts)
    monkeypatch.setattr(
        host_check, "configure_tool_isolation", lambda _config: IsolationStatus(config=config, ready=True)
    )
    _, problems = host_check.check(config)
    assert any("noexec" in p and "tool_homes" in p for p in problems)


def test_isolation_off_is_a_failure(tmp_path: Path) -> None:
    config = IsolationConfig(mode="off")
    _, problems = host_check.check(config)
    assert problems == ["CODEFORGE_TOOL_ISOLATION is not required: run the check with the production worker service"]


def test_main_prints_json_and_exits_non_zero_on_failure(
    capsys: pytest.CaptureFixture[str], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(host_check, "check", lambda _config: ({"kernel": "6.18"}, ["broken"]))
    monkeypatch.setattr(host_check, "_config", lambda: IsolationConfig(mode="required"))
    assert host_check.main(["--json"]) == 1
    out = json.loads(capsys.readouterr().out)
    assert out == {"kernel": "6.18", "problems": ["broken"]}
    monkeypatch.setattr(host_check, "check", lambda _config: ({"kernel": "6.18"}, []))
    assert host_check.main([]) == 0
    assert "result: OK" in capsys.readouterr().out
