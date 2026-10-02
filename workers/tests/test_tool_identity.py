"""Per-tenant tool identities (KI-96, codeforge.tool_identity).

A handler's tool work runs as the tenant's tool UID once the accept checks
pass: the UID is a tenant UID bound to this tenant on the volume, the
workspace lies in the tenant's own directory (or is an adopted workspace
with the UID's ACL), the tenant directory has exactly the tenant's ACLs.
"""

from __future__ import annotations

import asyncio
import errno
import re
from pathlib import Path

import pytest

from codeforge import posix_acl, tool_identity, tool_migration, tool_process, tool_state
from codeforge.tool_identity import (
    LEGACY_TOOL_UID,
    SYSTEM_TOOL_UID,
    TOOL_UID_MAX,
    TOOL_UID_MIN,
    ToolIdentity,
    ToolIsolationError,
    accept_identity,
    check_tool_uid,
    current_identity,
    identity_env,
    tool_tenant,
    use_identity,
)
from codeforge.tool_process import IsolationConfig, IsolationStatus

GO_CONSTANTS = Path(__file__).resolve().parents[2] / "internal" / "domain" / "tenant" / "tooluid.go"
UID = 20004


def test_the_uid_constants_match_the_go_core() -> None:
    source = GO_CONSTANTS.read_text()
    go = {name: int(value) for name, value in re.findall(r"^\s*(\w+)\s*=\s*(\d+)\s*$", source, re.MULTILINE)}
    assert go == {
        "ToolUIDMin": TOOL_UID_MIN,
        "ToolUIDMax": TOOL_UID_MAX,
        "SystemToolUID": SYSTEM_TOOL_UID,
        "LegacyToolUID": LEGACY_TOOL_UID,
    }


@pytest.mark.parametrize(
    ("tenant", "uid", "problem"),
    [
        ("t", 20000, ""),
        ("t", 29999, ""),
        ("t", 25000, ""),
        ("", 20000, "without tenant_id"),
        ("t", 0, "without tool_uid"),
        ("t", 19999, "outside"),
        ("t", 30000, "outside"),
        ("t", 10001, "outside"),
        ("t", 10002, "outside"),
        ("t", -1, "outside"),
    ],
)
def test_check_tool_uid(tenant: str, uid: int, problem: str) -> None:
    if not problem:
        check_tool_uid(tenant, uid)
        return
    with pytest.raises(ToolIsolationError, match=problem):
        check_tool_uid(tenant, uid)


def test_identity_paths() -> None:
    identity = ToolIdentity(tenant_id="t", uid=UID, home="/h/20004", work_id="abc")
    assert identity.gid == UID
    assert not identity.is_system
    assert identity.tmpdir == "/h/20004/tmp/abc"
    assert identity.claude_config_dir == "/h/20004/claude/abc"
    assert identity.prepare() == ["tmp/abc"]
    claude = identity.with_paths(read=("/r",), write=("/w",), claude_config=True)
    assert claude.prepare() == ["tmp/abc", "claude/abc"]
    assert (claude.read_paths, claude.write_paths) == (("/r",), ("/w",))
    assert identity.with_workspace("/ws").workspace == "/ws"
    assert ToolIdentity(tenant_id="", uid=SYSTEM_TOOL_UID, home="/h", work_id="x").is_system


def test_identity_env_names_the_user() -> None:
    tenant = identity_env(ToolIdentity(tenant_id="t", uid=UID, home="/h/20004", work_id="abc"), "/usr/bin")
    system = identity_env(ToolIdentity(tenant_id="", uid=SYSTEM_TOOL_UID, home="/h/19999", work_id="x"), "/usr/bin")
    assert tenant["USER"] == "codeforge-t20004"
    assert system["USER"] == "codeforge-system"
    assert set(tenant) == tool_identity.IDENTITY_ENV_NAMES


# ---------------------------------------------------------------------------
# Accepting tool work (real directories with ACLs)
# ---------------------------------------------------------------------------


@pytest.fixture
def volumes(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> tuple[Path, Path]:
    """A workspace root and HOME base, isolation required and ready for them."""
    root = tmp_path / "workspaces"
    root.mkdir()
    root.chmod(0o2771)
    homes = tmp_path / "homes"
    homes.mkdir(mode=0o711)
    try:
        posix_acl.set_acl(str(root), posix_acl.DEFAULT, posix_acl.tenant_default(UID))
        posix_acl.remove_acl(str(root), posix_acl.DEFAULT)
    except OSError as exc:
        if exc.errno == errno.EOPNOTSUPP:
            pytest.skip("no POSIX ACLs on the test file system")
        raise
    config = IsolationConfig(mode="required", workspace_root=str(root), home_base=str(homes))
    monkeypatch.setattr(
        tool_process, "_status", IsolationStatus(config=config, ready=True, launcher="/x", interpreter="/y")
    )
    monkeypatch.setattr(tool_identity, "_homes", {})
    return root, homes


def _tenant(root: Path, tenant: str, uid: int = UID) -> Path:
    """A tenant directory as the Go Core makes it, with one project."""
    path = root / tenant
    path.mkdir()
    path.chmod(0o2770)
    posix_acl.set_acl(str(path), posix_acl.ACCESS, posix_acl.tenant_access(uid))
    posix_acl.set_acl(str(path), posix_acl.DEFAULT, posix_acl.tenant_default(uid))
    (path / "p1").mkdir()
    return path / "p1"


def test_accepted_work_runs_as_the_tenants_uid(volumes: tuple[Path, Path]) -> None:
    root, homes = volumes
    workspace = _tenant(root, "tenant-a")
    identity = accept_identity("tenant-a", UID, str(workspace) + "/")
    assert identity.uid == UID
    assert identity.workspace == str(workspace)
    assert identity.home == f"{homes}/{UID}"
    assert len(identity.work_id) == 16
    assert tool_state.bound_tenant(str(root), UID) == "tenant-a"
    assert (homes / str(UID)).is_dir()
    # Each accept gets its own TMPDIR token.
    assert accept_identity("tenant-a", UID, str(workspace)).work_id != identity.work_id


def test_work_without_a_workspace_is_accepted(volumes: tuple[Path, Path]) -> None:
    assert accept_identity("tenant-a", UID, None).workspace is None


@pytest.mark.parametrize(
    "workspace",
    [
        "{root}/tenant-b/p1",
        "{root}/tenant-a/../tenant-b/p1",
        "{root}/tenant-a",
        "{root}",
        "{root}/p1",
        "relative/path",
    ],
)
def test_a_workspace_outside_the_tenants_directory_is_refused(workspace: str, volumes: tuple[Path, Path]) -> None:
    root, _ = volumes
    _tenant(root, "tenant-a")
    _tenant(root, "tenant-b", uid=UID + 1)
    with pytest.raises(ToolIsolationError):
        accept_identity("tenant-a", UID, workspace.format(root=root))


def test_a_tenant_directory_without_its_acls_is_migrated_not_refused(volumes: tuple[Path, Path]) -> None:
    root, _ = volumes
    workspace = _tenant(root, "tenant-a", uid=UID + 1)  # another UID's ACLs: the worker's own directory
    identity = accept_identity("tenant-a", UID, str(workspace))
    assert tool_migration.needs_migration(str(root), identity)
    # Every launch still refuses it until it is migrated.
    with pytest.raises(ToolIsolationError, match="lacks the ACLs"):
        tool_identity.verify_tenant_dir(str(root), "tenant-a", UID)


def test_a_migrated_tenant_directory_needs_its_stamp(volumes: tuple[Path, Path]) -> None:
    root, _ = volumes
    workspace = _tenant(root, "tenant-a")
    identity = accept_identity("tenant-a", UID, str(workspace))
    assert tool_migration.needs_migration(str(root), identity)
    tool_migration.write_stamp(str(root), "tenants", "tenant-a", UID, (root / "tenant-a").stat())
    assert not tool_migration.needs_migration(str(root), identity)
    assert not tool_migration.needs_migration(str(root), identity.with_workspace(""))


def test_a_symlinked_tenant_directory_is_refused(volumes: tuple[Path, Path], tmp_path: Path) -> None:
    root, _ = volumes
    real = _tenant(tmp_path, "elsewhere")
    (root / "tenant-a").symlink_to(real.parent)
    with pytest.raises(ToolIsolationError, match="symlink"):
        accept_identity("tenant-a", UID, f"{root}/tenant-a/p1")


def test_a_uid_bound_to_another_tenant_is_refused(volumes: tuple[Path, Path]) -> None:
    root, _ = volumes
    workspace = _tenant(root, "tenant-a")
    tool_state.bind_tool_uid(str(root), UID, "tenant-old")
    with pytest.raises(ToolIsolationError, match="bound to tenant tenant-old"):
        accept_identity("tenant-a", UID, str(workspace))


def test_an_adopted_workspace_needs_the_uids_acl(volumes: tuple[Path, Path], tmp_path: Path) -> None:
    adopted = tmp_path / "adopted"
    adopted.mkdir()
    with pytest.raises(ToolIsolationError, match="setfacl"):
        accept_identity("tenant-a", UID, str(adopted))
    posix_acl.set_acl(str(adopted), posix_acl.ACCESS, posix_acl.tenant_default(UID))
    assert accept_identity("tenant-a", UID, str(adopted)).workspace == str(adopted)


def test_a_replaced_home_is_refused(volumes: tuple[Path, Path]) -> None:
    _, homes = volumes
    accept_identity("tenant-a", UID, None)
    (homes / str(UID)).rename(homes / "old")
    (homes / str(UID)).mkdir()
    with pytest.raises(ToolIsolationError, match="replaced"):
        accept_identity("tenant-a", UID, None)


# ---------------------------------------------------------------------------
# tool_tenant
# ---------------------------------------------------------------------------


async def test_tool_tenant_is_a_no_op_when_off(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_process, "_status", IsolationStatus(config=IsolationConfig(mode="off"), ready=True))
    async with tool_tenant("", 0, "/anything") as identity:
        assert identity is None
        assert current_identity.get() is None


async def test_tool_tenant_when_not_ready_refuses(monkeypatch: pytest.MonkeyPatch) -> None:
    config = IsolationConfig(mode="required", workspace_root="/w")
    monkeypatch.setattr(tool_process, "_status", IsolationStatus(config=config, ready=False, reason="no setpriv"))
    with pytest.raises(ToolIsolationError, match="no setpriv"):
        async with tool_tenant("tenant-a", UID, None):
            pytest.fail("entered")


async def test_tool_tenant_migrates_a_tree_first(volumes: tuple[Path, Path], monkeypatch: pytest.MonkeyPatch) -> None:
    root, _ = volumes
    workspace = _tenant(root, "tenant-a", uid=UID + 1)
    migrated: list[tuple[str, str, ToolIdentity | None]] = []

    def migrate(root_path: str, identity: ToolIdentity) -> None:
        migrated.append((root_path, identity.tenant_id, current_identity.get()))
        tenant = root / "tenant-a"
        posix_acl.set_acl(str(tenant), posix_acl.ACCESS, posix_acl.tenant_access(UID))
        posix_acl.set_acl(str(tenant), posix_acl.DEFAULT, posix_acl.tenant_default(UID))
        tool_migration.write_stamp(root_path, "tenants", "tenant-a", UID, tenant.stat())

    async def no_share(_path: str, _identity: ToolIdentity | None = None) -> None:
        return None

    monkeypatch.setattr(tool_migration, "migrate", migrate)
    monkeypatch.setattr(tool_process, "share_tool_files", no_share)
    async with tool_tenant("tenant-a", UID, str(workspace)) as identity:
        assert identity is not None
    # Before any tool process of the tenant: no identity was set yet.
    assert migrated == [(str(root), "tenant-a", None)]
    async with tool_tenant("tenant-a", UID, str(workspace)):
        pass
    assert len(migrated) == 1, "migrated once"


async def test_tool_tenant_sets_the_identity_and_shares_on_exit(
    volumes: tuple[Path, Path], monkeypatch: pytest.MonkeyPatch
) -> None:
    root, _ = volumes
    workspace = _tenant(root, "tenant-a")
    tool_migration.write_stamp(str(root), "tenants", "tenant-a", UID, (root / "tenant-a").stat())
    shared: list[tuple[str, ToolIdentity | None]] = []

    async def share(path: str, identity: ToolIdentity | None = None) -> None:
        shared.append((path, current_identity.get()))

    monkeypatch.setattr(tool_process, "share_tool_files", share)

    async def in_a_task() -> ToolIdentity | None:
        return current_identity.get()

    async with tool_tenant("tenant-a", UID, str(workspace)) as identity:
        assert identity is not None
        assert current_identity.get() is identity
        # Tasks started below inherit it (sub-agents, MCP sessions).
        assert await asyncio.create_task(in_a_task()) is identity
        assert shared == []
    assert current_identity.get() is None
    # The sharing pass ran as the tenant, before the identity was left.
    assert shared == [(str(workspace), identity)]


async def test_tool_tenant_refuses_before_entering(volumes: tuple[Path, Path]) -> None:
    root, _ = volumes
    _tenant(root, "tenant-b", uid=UID + 1)
    with pytest.raises(ToolIsolationError):
        async with tool_tenant("tenant-a", UID, f"{root}/tenant-b/p1"):
            pytest.fail("entered")
    assert current_identity.get() is None


def test_use_identity_restores_the_previous_one() -> None:
    outer = ToolIdentity(tenant_id="a", uid=UID, home="/h", work_id="1")
    inner = ToolIdentity(tenant_id="", uid=SYSTEM_TOOL_UID, home="/s", work_id="2")
    with use_identity(outer):
        with use_identity(inner):
            assert current_identity.get() is inner
        assert current_identity.get() is outer
    assert current_identity.get() is None
