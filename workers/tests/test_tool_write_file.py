"""Comprehensive tests for the WriteFileTool."""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.tools.write_file import DEFINITION, WriteFileTool

if TYPE_CHECKING:
    from pathlib import Path


# ---------------------------------------------------------------------------
# Definition
# ---------------------------------------------------------------------------


class TestWriteFileDefinition:
    """Tests for the DEFINITION constant."""

    def test_name(self) -> None:
        assert DEFINITION.name == "write_file"

    def test_has_description(self) -> None:
        assert DEFINITION.description

    def test_required_params(self) -> None:
        required = DEFINITION.parameters.get("required", [])
        assert "file_path" in required
        assert "content" in required

    def test_has_examples(self) -> None:
        assert len(DEFINITION.examples) > 0


# ---------------------------------------------------------------------------
# Basic writing
# ---------------------------------------------------------------------------


class TestWriteFileBasic:
    """Tests for basic file creation and writing."""

    async def test_create_new_file(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "hello.txt", "content": "hello world"},
            str(tmp_path),
        )
        assert result.success is True
        assert result.error == ""
        assert (tmp_path / "hello.txt").read_text() == "hello world"

    async def test_output_message_format(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        content = "some content"
        result = await tool.execute(
            {"file_path": "out.txt", "content": content},
            str(tmp_path),
        )
        assert f"wrote {len(content)} bytes" in result.output
        assert "out.txt" in result.output

    async def test_overwrite_existing_file(self, tmp_path: Path) -> None:
        (tmp_path / "existing.txt").write_text("old content")
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "existing.txt", "content": "new content"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "existing.txt").read_text() == "new content"

    async def test_write_preserves_exact_content(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        content = "line1\nline2\n\ttabbed\n"
        result = await tool.execute(
            {"file_path": "exact.txt", "content": content},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "exact.txt").read_text() == content


# ---------------------------------------------------------------------------
# Directory creation
# ---------------------------------------------------------------------------


class TestWriteFileDirectoryCreation:
    """Tests for automatic parent directory creation."""

    async def test_creates_single_parent(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "newdir/file.txt", "content": "data"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "newdir" / "file.txt").read_text() == "data"

    async def test_creates_deeply_nested_parents(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "a/b/c/d/file.txt", "content": "deep"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "a" / "b" / "c" / "d" / "file.txt").read_text() == "deep"

    async def test_existing_parent_directory_no_error(self, tmp_path: Path) -> None:
        (tmp_path / "existing_dir").mkdir()
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "existing_dir/file.txt", "content": "ok"},
            str(tmp_path),
        )
        assert result.success is True


# ---------------------------------------------------------------------------
# Edge cases
# ---------------------------------------------------------------------------


class TestWriteFileEdgeCases:
    """Tests for edge cases and special content."""

    async def test_empty_content(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "empty.txt", "content": ""},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "empty.txt").read_text() == ""
        assert "wrote 0 bytes" in result.output

    async def test_unicode_content(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        content = "Hello \u4e16\u754c\n\u00e9\u00e8\u00ea\n"
        result = await tool.execute(
            {"file_path": "unicode.txt", "content": content},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "unicode.txt").read_text(encoding="utf-8") == content

    async def test_large_content(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        content = "x" * 100_000
        result = await tool.execute(
            {"file_path": "large.txt", "content": content},
            str(tmp_path),
        )
        assert result.success is True
        assert len((tmp_path / "large.txt").read_text()) == 100_000

    async def test_content_with_special_characters(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        content = "tab\there\nnull\x00byte\nquote\"double\nsingle'quote\n"
        result = await tool.execute(
            {"file_path": "special.txt", "content": content},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "special.txt").read_text() == content

    async def test_missing_content_defaults_to_empty(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        # content key missing => defaults to ""
        result = await tool.execute({"file_path": "no_content.txt"}, str(tmp_path))
        assert result.success is True
        assert (tmp_path / "no_content.txt").read_text() == ""


# ---------------------------------------------------------------------------
# Security
# ---------------------------------------------------------------------------


class TestWriteFileSecurity:
    """Tests for path traversal protection."""

    async def test_path_traversal_relative(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "../outside.txt", "content": "bad"},
            str(tmp_path),
        )
        assert result.success is False
        assert "traversal" in result.error

    async def test_path_traversal_deep(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "../../etc/evil.txt", "content": "bad"},
            str(tmp_path),
        )
        assert result.success is False
        assert "traversal" in result.error

    async def test_path_traversal_absolute(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "/tmp/evil.txt", "content": "bad"},
            str(tmp_path),
        )
        assert result.success is False
        assert "traversal" in result.error

    async def test_path_traversal_via_dot_dot_in_middle(self, tmp_path: Path) -> None:
        tool = WriteFileTool()
        result = await tool.execute(
            {"file_path": "sub/../../outside.txt", "content": "bad"},
            str(tmp_path),
        )
        assert result.success is False
        assert "traversal" in result.error
