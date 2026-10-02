"""Knowledge-base indexing stays in the tenant's area below the knowledge content root (KI-105).

A knowledge base ("kb:<id>") is indexed from knowledge_path, relative to the
area <content root>/<tenant_id>/ of the request's tenant; the worker refuses a
request without a tenant, a workspace_path, an absolute knowledge_path and one
that leaves the area (a symlink included), and answers "outside" and "missing"
the same way.
"""

from __future__ import annotations

import json
import os
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge import config
from codeforge.config import WorkerSettings
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._retrieval import RetrievalHandlerMixin
from codeforge.models import RetrievalIndexRequest
from codeforge.retrieval import CodeChunker
from codeforge.workspace_fs import PathLeavesWorkspaceError, WorkspaceRoot

if TYPE_CHECKING:
    from pathlib import Path

TENANT_A = "11111111-1111-4111-8111-111111111111"
TENANT_B = "22222222-2222-4222-8222-222222222222"


@pytest.fixture
def layout(tmp_path: Path) -> tuple[Path, Path]:
    root = tmp_path / "knowledge"
    area = root / TENANT_A
    outside = tmp_path / "outside"
    (area / "docs" / "sub").mkdir(parents=True)
    (root / TENANT_B).mkdir()
    outside.mkdir()
    (area / "docs" / "guide.py").write_text("def guide():\n    return 'inside'\n")
    (area / "docs" / "sub" / "more.py").write_text("def more():\n    return 'inside'\n")
    (area / "notes.py").write_text("def notes():\n    return 'inside'\n")
    (root / TENANT_B / "secret.py").write_text("def tenant_b_secret():\n    return 'b'\n")
    (root / "top.py").write_text("def root_level():\n    return 'root'\n")
    (outside / "secret.py").write_text("def secret():\n    return 'outside'\n")
    os.symlink(str(outside), area / "abs-dir")
    os.symlink("../../outside", area / "rel-dir")
    os.symlink(f"../{TENANT_B}", area / "cross")
    os.symlink("docs", area / "in-dir")
    os.symlink(str(outside / "secret.py"), area / "docs" / "leak.py")
    return root, outside


class _Mixin(RetrievalHandlerMixin, ConsumerBaseMixin):
    def __init__(self, content_root: str) -> None:
        self._js = AsyncMock()
        self._retriever = MagicMock()
        self._retriever.build_index = AsyncMock(return_value=MagicMock(status="ready", error=""))
        self._subagent = MagicMock()
        self._knowledge_content_root = content_root


def _msg(**fields: str) -> MagicMock:
    msg = MagicMock()
    msg.data = json.dumps(RetrievalIndexRequest(**fields).model_dump()).encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.headers = {}
    return msg


def _published(mixin: _Mixin) -> dict:
    mixin._js.publish.assert_called_once()
    return json.loads(mixin._js.publish.call_args.args[1])


@pytest.mark.parametrize("knowledge_path", ["docs", "notes.py", "in-dir", "docs/sub", "."])
async def test_knowledge_index_walks_the_tenant_area(layout: tuple[Path, Path], knowledge_path: str) -> None:
    root, _ = layout
    mixin = _Mixin(str(root))
    await mixin._handle_retrieval_index(
        _msg(project_id="kb:1", tenant_id=TENANT_A, workspace_path="", knowledge_path=knowledge_path)
    )

    mixin._retriever.build_index.assert_awaited_once()
    kwargs = mixin._retriever.build_index.call_args.kwargs
    assert kwargs["workspace_path"] == str(root)
    assert kwargs["tenant"] == TENANT_A
    assert kwargs["below"] == knowledge_path


_UNAVAILABLE = [
    "/etc",
    "OUTSIDE",  # the absolute outside directory, filled in below
    "../../outside",
    "docs/../../../outside",
    "abs-dir",
    "rel-dir",
    "cross",
    f"../{TENANT_B}",
    f"../{TENANT_B}/secret.py",
    f"../{TENANT_B}/missing.py",
    "..",
    "../top.py",
    "missing",
]


async def test_knowledge_index_answers_outside_and_missing_alike(layout: tuple[Path, Path]) -> None:
    root, outside = layout
    errors = set()
    for index, knowledge_path in enumerate(_UNAVAILABLE):
        mixin = _Mixin(str(root))
        path = str(outside) if knowledge_path == "OUTSIDE" else knowledge_path
        msg = _msg(project_id=f"kb:{index}", tenant_id=TENANT_A, workspace_path="", knowledge_path=path)
        await mixin._handle_retrieval_index(msg)
        mixin._retriever.build_index.assert_not_awaited()
        result = _published(mixin)
        assert result["status"] == "error", knowledge_path
        errors.add(result["error"])
        msg.ack.assert_called_once()
    assert len(errors) == 1, errors
    assert str(outside) not in errors.pop()


@pytest.mark.parametrize(
    "fields",
    [
        {"knowledge_path": ""},
        {"knowledge_path": "docs", "workspace_path": "/tmp"},
        {"knowledge_path": "", "workspace_path": "/etc"},
        {"knowledge_path": "docs", "tenant_id": ""},
        {"knowledge_path": "docs", "tenant_id": ".."},
        {"knowledge_path": "docs", "tenant_id": f"{TENANT_A}/.."},
        {"knowledge_path": "docs", "tenant_id": "not-a-tenant"},
    ],
)
async def test_knowledge_index_refuses_malformed_requests(layout: tuple[Path, Path], fields: dict[str, str]) -> None:
    root, _ = layout
    fields = {"workspace_path": "", "tenant_id": TENANT_A, **fields}
    mixin = _Mixin(str(root))
    await mixin._handle_retrieval_index(_msg(project_id="kb:1", **fields))
    mixin._retriever.build_index.assert_not_awaited()
    assert _published(mixin)["status"] == "error"


async def test_knowledge_path_is_only_for_knowledge_bases(tmp_path: Path) -> None:
    mixin = _Mixin(str(tmp_path))
    await mixin._handle_retrieval_index(_msg(project_id="proj-1", workspace_path=str(tmp_path), knowledge_path="x"))
    mixin._retriever.build_index.assert_not_awaited()
    assert _published(mixin)["status"] == "error"


async def test_knowledge_index_without_a_content_root_is_refused(layout: tuple[Path, Path]) -> None:
    mixin = _Mixin("")
    await mixin._handle_retrieval_index(
        _msg(project_id="kb:1", tenant_id=TENANT_A, workspace_path="", knowledge_path="docs")
    )
    mixin._retriever.build_index.assert_not_awaited()
    assert _published(mixin)["status"] == "error"


def test_chunker_indexes_only_the_tenants_knowledge_directory(layout: tuple[Path, Path]) -> None:
    root, _ = layout
    per_file = CodeChunker().chunk_workspace_by_file(str(root), tenant=TENANT_A, below="docs")
    assert sorted(per_file) == ["guide.py", "sub/more.py"]
    whole_area = CodeChunker().chunk_workspace_by_file(str(root), tenant=TENANT_A, below=".")
    assert sorted(whole_area) == ["docs/guide.py", "docs/sub/more.py", "notes.py"]
    contents = [c.content for _hash, chunks in [*per_file.values(), *whole_area.values()] for c in chunks]
    assert all("outside" not in text and "tenant_b" not in text and "root_level" not in text for text in contents)


def test_chunker_refuses_a_knowledge_directory_outside_the_area(layout: tuple[Path, Path]) -> None:
    root, _ = layout
    for below in ("abs-dir", "rel-dir", "cross", "..", f"../{TENANT_B}", "../../outside"):
        assert CodeChunker().chunk_workspace_by_file(str(root), tenant=TENANT_A, below=below) == {}, below


def test_subroot_stays_below_its_directory(layout: tuple[Path, Path]) -> None:
    root, _ = layout
    with WorkspaceRoot(str(root / TENANT_A)) as base:
        with base.subroot("docs") as docs:
            assert docs.read_text("guide.py").startswith("def guide")
            assert docs.read_text("sub/more.py").startswith("def more")
            with pytest.raises(PathLeavesWorkspaceError):
                docs.read_text("../notes.py")
            with pytest.raises(PathLeavesWorkspaceError):
                docs.read_text("leak.py")
        with base.subroot("in-dir") as linked:
            assert linked.is_file("guide.py")
        for below in ("abs-dir", "rel-dir", "cross", "../outside"):
            with pytest.raises(PathLeavesWorkspaceError):
                base.subroot(below)
        with pytest.raises(NotADirectoryError):
            base.subroot("notes.py")
        with pytest.raises(FileNotFoundError):
            base.subroot("missing")


def test_knowledge_content_root_setting(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("CODEFORGE_KNOWLEDGE_CONTENT_ROOT", raising=False)
    monkeypatch.setattr(config, "load_yaml_config", dict)
    assert WorkerSettings().knowledge_content_root == "data/knowledge"
    monkeypatch.setattr(config, "load_yaml_config", lambda: {"knowledge": {"content_root": "/srv/kb"}})
    assert WorkerSettings().knowledge_content_root == "/srv/kb"
    monkeypatch.setenv("CODEFORGE_KNOWLEDGE_CONTENT_ROOT", "/data/knowledge")
    assert WorkerSettings().knowledge_content_root == "/data/knowledge"
