"""The worker's state on the shared volumes for per-tenant tool identities (KI-96, codeforge.tool_state).

Everything works on descriptors and never follows a symlink a tool could
have planted (rule W1). Needs POSIX ACLs on the test file system.
"""

from __future__ import annotations

import errno
import os
import stat
from typing import TYPE_CHECKING

import pytest

from codeforge import posix_acl, tool_state
from codeforge.tool_identity import ToolIsolationError
from codeforge.tool_state import TenantDir

if TYPE_CHECKING:
    from pathlib import Path

UID = 20005


@pytest.fixture(autouse=True)
def _needs_acls(tmp_path: Path) -> None:
    probe = tmp_path / ".acl-probe"
    probe.mkdir()
    try:
        posix_acl.set_acl(str(probe), posix_acl.DEFAULT, posix_acl.tenant_default(UID))
    except OSError as exc:
        if exc.errno == errno.EOPNOTSUPP:
            pytest.skip("no POSIX ACLs on the test file system")
        raise
    finally:
        probe.rmdir()


@pytest.fixture
def root(tmp_path: Path) -> Path:
    path = tmp_path / "workspaces"
    path.mkdir(mode=0o755)
    return path


def _tenant_dir(root: Path, tenant: str, uid: int = UID) -> Path:
    path = root / tenant
    path.mkdir()
    path.chmod(0o2770)
    posix_acl.set_acl(str(path), posix_acl.ACCESS, posix_acl.tenant_access(uid))
    posix_acl.set_acl(str(path), posix_acl.DEFAULT, posix_acl.tenant_default(uid))
    return path


# ---------------------------------------------------------------------------
# The workspace root and the state directory
# ---------------------------------------------------------------------------


def test_root_problems_fixes_the_mode(root: Path) -> None:
    assert tool_state.root_problems(str(root), fix=True) == []
    assert stat.S_IMODE(root.stat().st_mode) == 0o2771


def test_root_problems_without_fix_reports_the_mode(root: Path) -> None:
    problems = tool_state.root_problems(str(root), fix=False)
    assert problems
    assert "2771" in problems[0]


def test_a_root_of_another_user_is_refused(root: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_state, "worker_uid", lambda: root.stat().st_uid + 1)
    problems = tool_state.root_problems(str(root), fix=True)
    assert "not the worker" in problems[0]
    assert stat.S_IMODE(root.stat().st_mode) == 0o755  # untouched


def test_a_root_with_acl_entries_is_refused(root: Path) -> None:
    posix_acl.set_acl(str(root), posix_acl.DEFAULT, posix_acl.tenant_default(UID))
    problems = tool_state.root_problems(str(root), fix=True)
    assert any("setfacl -b" in p for p in problems)


def test_state_dirs_are_private(root: Path) -> None:
    tool_state.ensure_state_dirs(str(root))
    for name in ("", *tool_state.STATE_SUBDIRS):
        path = root / ".codeforge" / name
        assert stat.S_IMODE(path.stat().st_mode) == 0o700, name


@pytest.mark.parametrize("plant", ["symlink", "acl", "mode"])
def test_a_tampered_state_dir_is_refused(plant: str, root: Path, tmp_path: Path) -> None:
    state = root / ".codeforge"
    if plant == "symlink":
        elsewhere = tmp_path / "elsewhere"
        elsewhere.mkdir(mode=0o700)
        state.symlink_to(elsewhere)
    else:
        state.mkdir(mode=0o700)
        if plant == "acl":
            posix_acl.set_acl(str(state), posix_acl.ACCESS, posix_acl.home_access(UID))
        else:
            state.chmod(0o770)
    with pytest.raises(ToolIsolationError):
        tool_state.ensure_state_dirs(str(root))


# ---------------------------------------------------------------------------
# UID bindings
# ---------------------------------------------------------------------------


def test_binding_a_tool_uid(root: Path) -> None:
    assert tool_state.bound_tenant(str(root), UID) is None
    tool_state.bind_tool_uid(str(root), UID, "tenant-a")
    tool_state.bind_tool_uid(str(root), UID, "tenant-a")  # idempotent
    assert tool_state.bound_tenant(str(root), UID) == "tenant-a"
    binding = root / ".codeforge" / "uids" / str(UID)
    assert stat.S_IMODE(binding.stat().st_mode) == 0o600


def test_a_uid_bound_to_another_tenant_is_refused(root: Path) -> None:
    tool_state.bind_tool_uid(str(root), UID, "tenant-a")
    with pytest.raises(ToolIsolationError, match="bound to tenant tenant-a"):
        tool_state.bind_tool_uid(str(root), UID, "tenant-b")
    assert tool_state.bound_tenant(str(root), UID) == "tenant-a"


@pytest.mark.parametrize("plant", ["symlink", "fifo", "big"])
def test_a_planted_binding_is_never_read(plant: str, root: Path, tmp_path: Path) -> None:
    tool_state.ensure_state_dirs(str(root))
    binding = root / ".codeforge" / "uids" / str(UID)
    if plant == "symlink":
        target = tmp_path / "target"
        target.write_text("tenant-a")
        binding.symlink_to(target)
    elif plant == "fifo":
        os.mkfifo(binding)
    else:
        binding.write_text("x" * 4096)
    with pytest.raises((ToolIsolationError, OSError)):
        tool_state.bind_tool_uid(str(root), UID, "tenant-a")


# ---------------------------------------------------------------------------
# Tenant directories
# ---------------------------------------------------------------------------


def test_a_tenant_directory_with_its_acls_is_ok(root: Path) -> None:
    path = _tenant_dir(root, "tenant-a")
    check = tool_state.check_tenant_dir(str(root), "tenant-a", UID)
    assert check.state is TenantDir.OK
    assert (check.dev, check.ino) == (path.stat().st_dev, path.stat().st_ino)


def test_a_tenant_directory_of_another_uid_must_be_migrated(root: Path) -> None:
    _tenant_dir(root, "tenant-a", uid=UID + 1)
    check = tool_state.check_tenant_dir(str(root), "tenant-a", UID)
    assert check.state is TenantDir.MIGRATE
    assert str(UID) in check.reason


def test_a_plain_tenant_directory_must_be_migrated(root: Path) -> None:
    (root / "tenant-a").mkdir(mode=0o2775)
    assert tool_state.check_tenant_dir(str(root), "tenant-a", UID).state is TenantDir.MIGRATE


def test_a_tenant_directory_with_a_wrong_mode_must_be_migrated(root: Path) -> None:
    path = _tenant_dir(root, "tenant-a")
    path.chmod(0o2775)
    assert tool_state.check_tenant_dir(str(root), "tenant-a", UID).state is TenantDir.MIGRATE


@pytest.mark.parametrize("tenant", ["missing", "symlink", "file", "..", "a/b", ""])
def test_a_bad_tenant_directory_is_refused(tenant: str, root: Path, tmp_path: Path) -> None:
    if tenant == "symlink":
        (root / tenant).symlink_to(_tenant_dir(tmp_path, "real"))
    elif tenant == "file":
        (root / tenant).write_text("")
    check = tool_state.check_tenant_dir(str(root), tenant, UID)
    assert check.state is TenantDir.REFUSED


def test_a_tenant_directory_of_another_user_is_refused(root: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    path = _tenant_dir(root, "tenant-a")
    monkeypatch.setattr(tool_state, "worker_uid", lambda: path.stat().st_uid + 1)
    check = tool_state.check_tenant_dir(str(root), "tenant-a", UID)
    assert check.state is TenantDir.REFUSED
    assert "never migrated" in check.reason


def test_an_adopted_workspace_needs_the_tool_uids_acl(tmp_path: Path) -> None:
    adopted = tmp_path / "adopted"
    adopted.mkdir()
    reason = tool_state.adopted_workspace_ready(str(adopted), UID)
    assert f"setfacl -R -m u:{UID}:rwX" in reason
    posix_acl.set_acl(str(adopted), posix_acl.ACCESS, posix_acl.tenant_default(UID))
    assert tool_state.adopted_workspace_ready(str(adopted), UID) == ""
    assert tool_state.adopted_workspace_ready(str(adopted), UID + 1) != ""


def test_an_adopted_workspace_symlink_is_refused(tmp_path: Path) -> None:
    adopted = tmp_path / "adopted"
    adopted.mkdir()
    posix_acl.set_acl(str(adopted), posix_acl.ACCESS, posix_acl.tenant_default(UID))
    link = tmp_path / "link"
    link.symlink_to(adopted)
    assert "cannot be opened" in tool_state.adopted_workspace_ready(str(link), UID)


# ---------------------------------------------------------------------------
# Tool HOMEs
# ---------------------------------------------------------------------------


@pytest.fixture
def home_base(tmp_path: Path) -> Path:
    path = tmp_path / "homes"
    path.mkdir(mode=0o755)
    return path


def test_home_base_problems_fix_the_mode(home_base: Path) -> None:
    assert tool_state.home_base_problems(str(home_base)) == []
    assert stat.S_IMODE(home_base.stat().st_mode) == 0o711


def test_home_base_problems(home_base: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    assert "cannot be opened" in tool_state.home_base_problems(str(home_base / "missing"))[0]
    posix_acl.set_acl(str(home_base), posix_acl.ACCESS, posix_acl.home_access(UID))
    assert any("ACL" in p for p in tool_state.home_base_problems(str(home_base)))
    posix_acl.remove_acl(str(home_base), posix_acl.ACCESS)

    real = os.fstatvfs

    def noexec(fd: int) -> os.statvfs_result:
        result = real(fd)
        values = list(result)
        values[8] |= os.ST_NOEXEC  # f_flag
        return os.statvfs_result(values)

    monkeypatch.setattr(tool_state.os, "fstatvfs", noexec)
    assert any("noexec" in p for p in tool_state.home_base_problems(str(home_base)))


def test_ensure_home(home_base: Path) -> None:
    dev_ino = tool_state.ensure_home(str(home_base), UID)
    home = home_base / str(UID)
    info = home.stat()
    assert (info.st_dev, info.st_ino) == dev_ino
    assert stat.S_IMODE(info.st_mode) == 0o770  # the group bits are the ACL mask
    assert posix_acl.equal(posix_acl.get_acl(str(home), posix_acl.ACCESS), posix_acl.home_access(UID))
    assert posix_acl.get_acl(str(home), posix_acl.DEFAULT) is None
    # Idempotent; repairs mode and ACL.
    home.chmod(0o755)
    posix_acl.set_acl(str(home), posix_acl.DEFAULT, posix_acl.tenant_default(UID))
    assert tool_state.ensure_home(str(home_base), UID) == dev_ino
    assert stat.S_IMODE(home.stat().st_mode) == 0o770
    assert posix_acl.equal(posix_acl.get_acl(str(home), posix_acl.ACCESS), posix_acl.home_access(UID))
    assert posix_acl.get_acl(str(home), posix_acl.DEFAULT) is None


def test_a_planted_home_is_refused(home_base: Path, tmp_path: Path) -> None:
    elsewhere = tmp_path / "elsewhere"
    elsewhere.mkdir()
    (home_base / str(UID)).symlink_to(elsewhere)
    with pytest.raises(ToolIsolationError, match="cannot be opened"):
        tool_state.ensure_home(str(home_base), UID)


def test_verify_home(home_base: Path) -> None:
    dev_ino = tool_state.ensure_home(str(home_base), UID)
    tool_state.verify_home(str(home_base), UID, dev_ino)

    home = home_base / str(UID)
    posix_acl.set_acl(str(home), posix_acl.ACCESS, posix_acl.home_access(UID + 1))
    with pytest.raises(ToolIsolationError, match="another ACL"):
        tool_state.verify_home(str(home_base), UID, dev_ino)

    home.rename(home_base / "moved")
    tool_state.ensure_home(str(home_base), UID)  # a new directory, a new inode
    with pytest.raises(ToolIsolationError, match="replaced"):
        tool_state.verify_home(str(home_base), UID, dev_ino)

    (home_base / str(UID)).rmdir()
    with pytest.raises(ToolIsolationError, match="gone or replaced"):
        tool_state.verify_home(str(home_base), UID, dev_ino)


def test_acl_support(tmp_path: Path) -> None:
    assert tool_state.acl_support_problem(str(tmp_path)) == ""
    assert sorted(os.listdir(tmp_path)) == []  # the check directory is removed
    assert "cannot be opened" in tool_state.acl_support_problem(str(tmp_path / "missing"))


def test_the_acl_check_leaves_another_workers_check_directory_alone(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """KI-96 review: worker replicas share the volumes and each is PID 1 of its container. A check
    named by the PID took over and removed another replica's directory mid-check, which then
    reported no POSIX ACLs (ENOENT) and stayed not ready."""
    monkeypatch.setattr(tool_state.os, "getpid", lambda: 1)
    others = tmp_path / ".cf-acl-check-1"
    others.mkdir(mode=0o700)  # another replica between its mkdir and its ACL check
    assert tool_state.acl_support_problem(str(tmp_path)) == ""
    assert others.is_dir()
    assert sorted(os.listdir(tmp_path)) == [".cf-acl-check-1"]


def _no_acls(*_args: object) -> object:
    raise OSError(errno.EOPNOTSUPP, os.strerror(errno.EOPNOTSUPP))


def test_a_volume_without_acls_is_named_with_its_remedy(
    root: Path, home_base: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """KI-96 D12: ramfs, ZFS with acltype=off, NFSv4 answer EOPNOTSUPP; the reason names the volume
    and the remedy, next to the other problems (a ramfs root belongs to root)."""
    monkeypatch.setattr(tool_state.posix_acl, "get_acl", _no_acls)
    monkeypatch.setattr(tool_state, "worker_uid", lambda: os.getuid() + 1)
    problems = tool_state.home_base_problems(str(home_base))
    assert any("belongs to uid" in p for p in problems), problems
    assert any(f"no POSIX ACLs on {home_base}" in p and "acltype=posixacl" in p for p in problems), problems
    monkeypatch.setattr(tool_state, "worker_uid", os.getuid)
    problems = tool_state.root_problems(str(root), fix=True)
    assert problems == [f"no POSIX ACLs on {root} (Operation not supported): " + tool_state.ACL_REMEDY], problems


def test_acl_support_tells_a_foreign_directory_from_missing_acls(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    def denied(*_args: object, **_kwargs: object) -> None:
        raise PermissionError(errno.EACCES, "Permission denied")

    monkeypatch.setattr(tool_state.os, "mkdir", denied)
    problem = tool_state.acl_support_problem(str(tmp_path))
    assert problem.startswith(f"the worker cannot create a directory in {tmp_path} (Permission denied)"), problem


def test_a_read_only_home_base_names_the_missing_volume(home_base: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """The KI-71 compose file mounts no tool_homes volume: the base is a directory of the read-only root."""
    real = os.fstatvfs

    def read_only(fd: int) -> os.statvfs_result:
        values = list(real(fd))
        values[8] |= os.ST_RDONLY  # f_flag
        return os.statvfs_result(values)

    monkeypatch.setattr(tool_state.os, "fstatvfs", read_only)
    problems = tool_state.home_base_problems(str(home_base))
    assert any("read-only" in p and "tool_homes volume" in p for p in problems), problems
