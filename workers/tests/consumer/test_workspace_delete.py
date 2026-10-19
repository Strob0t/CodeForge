"""Deleting a project workspace as the tenant's tool UID (KI-96 D11).

Under default ACLs a tool can create entries (mkdtemp, mkdir -m 0700, a
stripped ACL) that only its UID can remove. With workspace.tool_acls:
required the Go Core publishes workspace.delete.request and the worker
removes the tree as the tenant: accept checks, a full sharing pass, the
removal of the contents as the tool UID (confined to the workspace), then
the worker removes the empty directory relative to the tenant directory.
Delivered at least once: a workspace that is already gone counts as removed.
"""

from __future__ import annotations

import contextlib
import errno
import json
import os
import shutil
import time
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge import tool_identity, tool_process, tool_state, workspace_deletion
from codeforge.consumer import TaskConsumer
from codeforge.models import WorkspaceDeleteRequest, WorkspaceDeleteResult
from codeforge.nats_subjects import SUBJECT_WORKSPACE_DELETE_REQUEST, SUBJECT_WORKSPACE_DELETE_RESULT
from codeforge.tool_identity import ToolIdentity, ToolIsolationError
from codeforge.tool_process import IsolationConfig, IsolationStatus

if TYPE_CHECKING:
    from collections.abc import AsyncIterator
    from pathlib import Path

TENANT = "tenant-a"
UID = 20021


@pytest.fixture
def root(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    root = tmp_path / "workspaces"
    (root / TENANT / "p1" / "sub").mkdir(parents=True)
    (root / TENANT / "p1" / "sub" / "f").write_text("x")
    config = IsolationConfig(mode="required", workspace_root=str(root))
    monkeypatch.setattr(tool_process, "_status", IsolationStatus(config=config, ready=True, launcher="/x"))
    return root


class _Steps:
    def __init__(self) -> None:
        self.steps: list[tuple[str, ...]] = []
        self.remove_ok = True


@pytest.fixture
def steps(monkeypatch: pytest.MonkeyPatch) -> _Steps:
    """The tenant's identity, the sharing pass and the removal as the tool UID, recorded."""
    recorded = _Steps()

    @contextlib.asynccontextmanager
    async def tenant(
        tenant_id: str, tool_uid: int, workspace: str | None, **_kw: object
    ) -> AsyncIterator[ToolIdentity]:
        tool_identity.check_tool_uid(tenant_id, tool_uid)
        recorded.steps.append(("enter", tenant_id, str(tool_uid), str(workspace)))
        yield ToolIdentity(tenant_id=tenant_id, uid=tool_uid, home="/h", work_id="w", workspace=workspace)
        recorded.steps.append(("leave",))

    async def share(path: str, _identity: object = None, **_kw: object) -> None:
        recorded.steps.append(("share", path))

    async def remove(paths: list[str], _identity: object, *, confine: str, contents_only: bool = False) -> bool:
        recorded.steps.append(("remove", *paths, confine, str(contents_only)))
        if recorded.remove_ok:
            for path in paths:
                import shutil
                from pathlib import Path

                for entry in Path(path).iterdir():
                    shutil.rmtree(entry)
        return recorded.remove_ok

    monkeypatch.setattr(workspace_deletion, "tool_tenant", tenant)
    monkeypatch.setattr(workspace_deletion, "share_tool_files", share)
    monkeypatch.setattr(workspace_deletion, "remove_as_tool", remove)
    monkeypatch.setattr(tool_identity, "_activity", {})
    return recorded


async def test_the_workspace_is_removed_as_the_tenants_tool_uid(root: Path, steps: _Steps) -> None:
    workspace = str(root / TENANT / "p1")
    await workspace_deletion.delete_workspace(TENANT, UID, workspace)
    assert steps.steps == [
        ("enter", TENANT, str(UID), workspace),
        ("share", workspace),
        ("remove", workspace, workspace, "True"),
        ("leave",),
    ]
    assert not (root / TENANT / "p1").exists()
    assert (root / TENANT).is_dir()
    # Build caches can hold data derived from the project: removed at the tenant's idle.
    assert tool_identity._activity[TENANT].clear_cache


async def test_a_workspace_that_is_gone_counts_as_removed(root: Path, steps: _Steps) -> None:
    await workspace_deletion.delete_workspace(TENANT, UID, str(root / TENANT / "gone"))
    await workspace_deletion.delete_workspace("tenant-without-dir", UID, str(root / "tenant-without-dir" / "p1"))
    assert steps.steps == []


def _core_patch_dir(workspace: Path) -> Path:
    """What the Go Core's writePatch leaves in a workspace: .git/codeforge/patches 0700, the patch 0600.

    Under the tenant directory's default ACL these modes make the mask ---: the tool UID can neither
    enter nor remove them, only their owner (the Go Core's UID, which is the worker's).
    """
    patches = workspace / ".git" / "codeforge" / "patches"
    patches.mkdir(parents=True)
    for directory in (patches.parent, patches):
        directory.chmod(0o700)
    patch = patches / "run-1.patch"
    patch.write_text("diff --git a/f b/f\n")
    patch.chmod(0o600)
    return patch


async def test_the_go_cores_private_entries_are_removed_by_the_worker(
    root: Path, steps: _Steps, monkeypatch: pytest.MonkeyPatch
) -> None:
    """KI-96 review: the tool UID's removal cannot reach the Go Core's private patch directory (its
    find fails, it reports failure); the worker removes its own entries by descriptor."""
    workspace = root / TENANT / "p1"
    patch = _core_patch_dir(workspace)

    async def tool_removes_what_it_can(
        paths: list[str], _identity: object, *, confine: str, contents_only: bool = False
    ) -> bool:
        steps.steps.append(("remove", *paths, confine, str(contents_only)))
        shutil.rmtree(workspace / "sub")
        return False  # find: '.git/codeforge': Permission denied

    monkeypatch.setattr(workspace_deletion, "remove_as_tool", tool_removes_what_it_can)
    await workspace_deletion.delete_workspace(TENANT, UID, str(workspace))
    assert not workspace.exists()
    assert not patch.exists()
    assert (root / TENANT).is_dir()


async def test_the_worker_removes_only_its_own_entries_and_never_follows_a_symlink(
    root: Path, steps: _Steps, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    workspace = root / TENANT / "p1"
    _core_patch_dir(workspace)
    outside = tmp_path / "outside"
    outside.mkdir()
    (outside / "keep").write_text("not the workspace's")
    (workspace / ".git" / "codeforge" / "to-outside").symlink_to(outside)
    (workspace / "dir-link").symlink_to(outside, target_is_directory=True)

    async def tool_removes_nothing(
        paths: list[str], _identity: object, *, confine: str, contents_only: bool = False
    ) -> bool:
        return False

    monkeypatch.setattr(workspace_deletion, "remove_as_tool", tool_removes_nothing)
    await workspace_deletion.delete_workspace(TENANT, UID, str(workspace))
    assert not workspace.exists()
    assert (outside / "keep").read_text() == "not the workspace's"


async def test_a_failed_removal_keeps_the_directory_and_fails(
    root: Path, steps: _Steps, monkeypatch: pytest.MonkeyPatch
) -> None:
    """What neither the tool UID nor the worker could remove stays, and the deletion fails (retried)."""
    steps.remove_ok = False
    # The files are the test's; as far as the worker's own pass is concerned they are another user's.
    monkeypatch.setattr(tool_state, "worker_uid", lambda: 4242)
    with pytest.raises(OSError, match=rf"removal as tool uid {UID} failed.*sub/f") as raised:
        await workspace_deletion.delete_workspace(TENANT, UID, str(root / TENANT / "p1"))
    assert raised.value.errno == errno.ENOTEMPTY
    assert (root / TENANT / "p1" / "sub" / "f").exists()


# ---------------------------------------------------------------------------
# The worker's own pass is bounded: a live process of the tenant (in another
# worker) can keep changing the tree while it walks it.
# ---------------------------------------------------------------------------


async def _tool_removes_nothing(
    paths: list[str], _identity: object, *, confine: str, contents_only: bool = False
) -> bool:
    return False


class _Grower:
    """A live process of the tenant: every directory the walk opens gets *per_dir* new subdirectories
    before it is listed, until *cap* were made. Records the open descriptors' peak."""

    def __init__(self, per_dir: int, cap: int) -> None:
        self.per_dir, self.cap = per_dir, cap
        self.made = 0
        self.peak_fds = 0
        self._open = workspace_deletion._open_listed_dir

    def open_listed_dir(self, dir_fd: int, name: str, listed: os.stat_result, dev: int) -> int | None:
        fd = self._open(dir_fd, name, listed, dev)
        if fd is not None:
            for index in range(self.per_dir):
                if self.made < self.cap:
                    os.mkdir(f"n{index}", dir_fd=fd)
                    self.made += 1
            self.peak_fds = max(self.peak_fds, _open_fds())
        return fd


def _open_fds() -> int:
    return len(os.listdir("/proc/self/fd"))


async def test_a_chain_below_the_depth_limit_is_left_and_the_walk_holds_few_descriptors(
    root: Path, steps: _Steps, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A chain the tenant keeps extending below the walk: the walk enters at most the depth limit's
    levels (two descriptors each: the directory and its listing), names the rest, and the deletion fails."""
    workspace = root / TENANT / "p1"
    shutil.rmtree(workspace / "sub")
    (workspace / "chain").mkdir()
    grower = _Grower(per_dir=1, cap=1000)
    monkeypatch.setattr(workspace_deletion, "remove_as_tool", _tool_removes_nothing)
    monkeypatch.setattr(workspace_deletion, "_open_listed_dir", grower.open_listed_dir)
    before = _open_fds()
    with pytest.raises(OSError, match=r"deeper than 128 levels") as raised:
        await workspace_deletion.delete_workspace(TENANT, UID, str(workspace))
    assert raised.value.errno == errno.ENOTEMPTY
    assert grower.made <= 129
    assert grower.peak_fds <= before + 2 * 129 + 4
    assert _open_fds() == before
    assert workspace.is_dir()


async def test_a_tree_that_keeps_growing_stops_the_walk_at_its_entry_budget(
    root: Path, steps: _Steps, monkeypatch: pytest.MonkeyPatch
) -> None:
    workspace = root / TENANT / "p1"
    grower = _Grower(per_dir=3, cap=20000)
    monkeypatch.setattr(workspace_deletion, "remove_as_tool", _tool_removes_nothing)
    monkeypatch.setattr(workspace_deletion, "_open_listed_dir", grower.open_listed_dir)
    monkeypatch.setattr(
        workspace_deletion, "OWN_ENTRY_LIMITS", workspace_deletion.WalkLimits(seconds=60, entries=200, depth=128)
    )
    with pytest.raises(OSError, match=r"stopped after 200 entries") as raised:
        await workspace_deletion.delete_workspace(TENANT, UID, str(workspace))
    assert raised.value.errno == errno.ENOTEMPTY
    assert grower.made <= 3 * 201
    assert workspace.is_dir()


async def test_an_endless_listing_stops_the_walk_at_its_deadline(
    root: Path, steps: _Steps, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Entries that keep coming (each gone again when the walk looks at it): the deadline ends the walk."""
    workspace = root / TENANT / "p1"
    real_scandir = os.scandir
    listed = 0

    class _Endless:
        def __init__(self, fd: int) -> None:
            self._listing = real_scandir(fd)

        def __iter__(self) -> _Endless:
            return self

        def __next__(self) -> MagicMock:
            nonlocal listed
            listed += 1
            time.sleep(0.001)
            entry = MagicMock()
            entry.name = f"gone-{listed}"
            return entry

        def close(self) -> None:
            self._listing.close()

    monkeypatch.setattr(workspace_deletion.os, "scandir", _Endless)
    monkeypatch.setattr(workspace_deletion, "remove_as_tool", _tool_removes_nothing)
    monkeypatch.setattr(
        workspace_deletion, "OWN_ENTRY_LIMITS", workspace_deletion.WalkLimits(seconds=0.5, entries=10**9, depth=128)
    )
    before = _open_fds()
    started = time.monotonic()
    with pytest.raises(OSError, match=r"stopped after 0.5 s") as raised:
        await workspace_deletion.delete_workspace(TENANT, UID, str(workspace))
    assert time.monotonic() - started < 5
    assert raised.value.errno == errno.ENOTEMPTY
    assert listed > 10
    assert _open_fds() == before


async def test_the_handler_reports_a_bounded_walk_as_a_failed_deletion(
    root: Path, steps: _Steps, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The Go Core records the error and publishes the deletion again (every 10 minutes) until it is done."""
    shutil.rmtree(root / TENANT / "p1" / "sub")
    (root / TENANT / "p1" / "chain").mkdir()
    monkeypatch.setattr(workspace_deletion, "remove_as_tool", _tool_removes_nothing)
    monkeypatch.setattr(workspace_deletion, "_open_listed_dir", _Grower(per_dir=1, cap=1000).open_listed_dir)
    request = WorkspaceDeleteRequest(
        deletion_id="d3", tenant_id=TENANT, tool_uid=UID, project_id="p1", workspace_path=str(root / TENANT / "p1")
    )
    result, msg = await _handle(request)
    assert not result.ok
    assert "was not removed completely" in result.error
    assert "deeper than 128 levels" in result.error
    msg.ack.assert_awaited_once()


@pytest.mark.parametrize(
    "workspace",
    [
        "{root}/tenant-b/p1",
        "{root}/tenant-a",
        "{root}/tenant-a/p1/sub",
        "{root}/tenant-a/../tenant-b/p1",
        "/elsewhere/p1",
        "relative/p1",
    ],
)
async def test_only_a_project_directory_of_the_tenant_is_removed(workspace: str, root: Path, steps: _Steps) -> None:
    (root / "tenant-b" / "p1").mkdir(parents=True)
    with pytest.raises(ToolIsolationError):
        await workspace_deletion.delete_workspace(TENANT, UID, workspace.format(root=root))
    assert steps.steps == []


async def test_with_isolation_off_the_worker_refuses(monkeypatch: pytest.MonkeyPatch, steps: _Steps) -> None:
    monkeypatch.setattr(tool_process, "_status", IsolationStatus(config=IsolationConfig(mode="off"), ready=True))
    with pytest.raises(ToolIsolationError, match="off"):
        await workspace_deletion.delete_workspace(TENANT, UID, "/data/workspaces/tenant-a/p1")


# ---------------------------------------------------------------------------
# The NATS handler
# ---------------------------------------------------------------------------


def _msg(request: WorkspaceDeleteRequest) -> MagicMock:
    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.subject = SUBJECT_WORKSPACE_DELETE_REQUEST
    msg.headers = {}
    msg.metadata = MagicMock(num_delivered=1, sequence=MagicMock(stream=7))
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.term = AsyncMock()
    msg.in_progress = AsyncMock()
    return msg


async def _handle(request: WorkspaceDeleteRequest) -> tuple[WorkspaceDeleteResult, MagicMock]:
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    consumer._js = AsyncMock()
    msg = _msg(request)
    await consumer._handle_workspace_delete(msg)
    calls = [c for c in consumer._js.publish.call_args_list if c.args[0] == SUBJECT_WORKSPACE_DELETE_RESULT]
    assert len(calls) == 1, consumer._js.publish.call_args_list
    return WorkspaceDeleteResult.model_validate_json(calls[0].args[1]), msg


async def test_the_handler_reports_a_removal(root: Path, steps: _Steps) -> None:
    request = WorkspaceDeleteRequest(
        deletion_id="d1", tenant_id=TENANT, tool_uid=UID, project_id="p1", workspace_path=str(root / TENANT / "p1")
    )
    result, msg = await _handle(request)
    assert (result.deletion_id, result.tenant_id, result.ok, result.error) == ("d1", TENANT, True, "")
    msg.ack.assert_awaited_once()


async def test_the_handler_reports_a_refusal_with_its_reason(root: Path, steps: _Steps) -> None:
    request = WorkspaceDeleteRequest(
        deletion_id="d2", tenant_id=TENANT, tool_uid=0, project_id="p1", workspace_path=str(root / TENANT / "p1")
    )
    result, _ = await _handle(request)
    assert not result.ok
    assert "without tool_uid" in result.error
    assert (root / TENANT / "p1").exists()


def test_the_subject_is_consumed() -> None:
    assert SUBJECT_WORKSPACE_DELETE_REQUEST == "workspace.delete.request"
    assert json.loads(WorkspaceDeleteResult(deletion_id="d", tenant_id="t", ok=True).model_dump_json())["ok"] is True
