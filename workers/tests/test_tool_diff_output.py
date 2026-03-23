"""Tests for edit_file and write_file diff output."""

from __future__ import annotations

import pytest

from codeforge.tools.edit_file import EditFileTool
from codeforge.tools.write_file import WriteFileTool


@pytest.mark.asyncio
async def test_edit_file_returns_diff(tmp_path: object) -> None:
    f = tmp_path / "test.go"  # type: ignore[operator]
    f.write_text("func hello() bool {\n  return false\n}\n")  # type: ignore[union-attr]

    tool = EditFileTool()
    result = await tool.execute(
        {"file_path": "test.go", "old_text": "return false", "new_text": "return true"},
        workspace_path=str(tmp_path),
    )
    assert result.success
    assert result.diff is not None
    assert result.diff["path"] == "test.go"
    assert len(result.diff["hunks"]) == 1
    hunk = result.diff["hunks"][0]
    assert hunk["old_content"] == "return false"
    assert hunk["new_content"] == "return true"
    assert hunk["old_start"] == 2  # "return false" is on line 2


@pytest.mark.asyncio
async def test_edit_file_error_no_diff(tmp_path: object) -> None:
    f = tmp_path / "test.txt"  # type: ignore[operator]
    f.write_text("hello")  # type: ignore[union-attr]

    tool = EditFileTool()
    result = await tool.execute(
        {"file_path": "test.txt", "old_text": "nonexistent", "new_text": "x"},
        workspace_path=str(tmp_path),
    )
    assert not result.success
    assert result.diff is None


@pytest.mark.asyncio
async def test_write_file_new_returns_diff(tmp_path: object) -> None:
    tool = WriteFileTool()
    result = await tool.execute(
        {"file_path": "new.txt", "content": "line1\nline2\n"},
        workspace_path=str(tmp_path),
    )
    assert result.success
    assert result.diff is not None
    assert result.diff["path"] == "new.txt"
    hunk = result.diff["hunks"][0]
    assert hunk["old_content"] == ""
    assert hunk["old_lines"] == 0
    assert hunk["new_content"] == "line1\nline2\n"


@pytest.mark.asyncio
async def test_write_file_overwrite_returns_diff(tmp_path: object) -> None:
    f = tmp_path / "existing.txt"  # type: ignore[operator]
    f.write_text("old content")  # type: ignore[union-attr]

    tool = WriteFileTool()
    result = await tool.execute(
        {"file_path": "existing.txt", "content": "new content"},
        workspace_path=str(tmp_path),
    )
    assert result.success
    assert result.diff is not None
    hunk = result.diff["hunks"][0]
    assert hunk["old_content"] == "old content"
    assert hunk["new_content"] == "new content"
