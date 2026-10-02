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
import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge import tool_identity, tool_process, workspace_deletion
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


async def test_a_failed_removal_keeps_the_directory_and_fails(root: Path, steps: _Steps) -> None:
    steps.remove_ok = False
    with pytest.raises(OSError):
        await workspace_deletion.delete_workspace(TENANT, UID, str(root / TENANT / "p1"))
    assert (root / TENANT / "p1" / "sub" / "f").exists()


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
