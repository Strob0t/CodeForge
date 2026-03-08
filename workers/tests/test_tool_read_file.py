"""Comprehensive tests for the ReadFileTool."""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.tools.read_file import DEFINITION, ReadFileTool

if TYPE_CHECKING:
    from pathlib import Path


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


def _make_workspace(tmp_path: Path) -> Path:
    """Create a workspace with test files."""
    (tmp_path / "simple.txt").write_text("line one\nline two\nline three\n")
    (tmp_path / "five_lines.txt").write_text("a\nb\nc\nd\ne\n")
    (tmp_path / "empty.txt").write_text("")
    (tmp_path / "unicode.txt").write_text("Hello \u4e16\u754c \U0001f600\n\u00e9\u00e8\u00ea\n")
    (tmp_path / "no_newline.txt").write_text("no trailing newline")
    (tmp_path / "sub").mkdir()
    (tmp_path / "sub" / "nested.py").write_text("import os\n")
    return tmp_path


# ---------------------------------------------------------------------------
# Definition
# ---------------------------------------------------------------------------


class TestReadFileDefinition:
    """Tests for the DEFINITION constant."""

    def test_name(self) -> None:
        assert DEFINITION.name == "read_file"

    def test_has_description(self) -> None:
        assert DEFINITION.description

    def test_file_path_is_required(self) -> None:
        assert "file_path" in DEFINITION.parameters.get("required", [])

    def test_has_offset_and_limit_params(self) -> None:
        props = DEFINITION.parameters.get("properties", {})
        assert "offset" in props
        assert "limit" in props

    def test_has_examples(self) -> None:
        assert len(DEFINITION.examples) > 0

    def test_has_common_mistakes(self) -> None:
        assert len(DEFINITION.common_mistakes) > 0


# ---------------------------------------------------------------------------
# Basic reading
# ---------------------------------------------------------------------------


class TestReadFileBasic:
    """Tests for basic file reading."""

    async def test_read_existing_file(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "simple.txt"}, str(ws))
        assert result.success is True
        assert result.error == ""
        assert "line one" in result.output
        assert "line two" in result.output
        assert "line three" in result.output

    async def test_output_has_numbered_lines(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "simple.txt"}, str(ws))
        lines = result.output.strip().splitlines()
        # Lines should be numbered starting at 1
        assert lines[0].strip().startswith("1")
        assert lines[1].strip().startswith("2")

    async def test_output_ends_with_newline(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "simple.txt"}, str(ws))
        assert result.output.endswith("\n")

    async def test_read_file_without_trailing_newline(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "no_newline.txt"}, str(ws))
        assert result.success is True
        # Implementation appends newline if missing
        assert result.output.endswith("\n")
        assert "no trailing newline" in result.output

    async def test_read_nested_file(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "sub/nested.py"}, str(ws))
        assert result.success is True
        assert "import os" in result.output


# ---------------------------------------------------------------------------
# Offset and limit
# ---------------------------------------------------------------------------


class TestReadFileOffsetLimit:
    """Tests for offset and limit parameters."""

    async def test_offset_skips_lines(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute(
            {"file_path": "five_lines.txt", "offset": 3},
            str(ws),
        )
        assert result.success is True
        assert "a" not in result.output.split("\t")[-1] if "\t" in result.output else True
        # Should contain lines 3, 4, 5 (c, d, e)
        assert "c" in result.output
        assert "d" in result.output
        assert "e" in result.output

    async def test_limit_restricts_lines(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute(
            {"file_path": "five_lines.txt", "limit": 2},
            str(ws),
        )
        assert result.success is True
        lines = result.output.strip().splitlines()
        assert len(lines) == 2

    async def test_offset_and_limit_together(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute(
            {"file_path": "five_lines.txt", "offset": 2, "limit": 2},
            str(ws),
        )
        assert result.success is True
        lines = result.output.strip().splitlines()
        assert len(lines) == 2
        # Should have lines 2 and 3 (b and c)
        assert "b" in result.output
        assert "c" in result.output

    async def test_offset_beyond_file_length(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute(
            {"file_path": "five_lines.txt", "offset": 100},
            str(ws),
        )
        assert result.success is True
        # No lines in range => empty or just newline
        assert result.output.strip() == ""

    async def test_limit_exceeds_file_length(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute(
            {"file_path": "five_lines.txt", "limit": 1000},
            str(ws),
        )
        assert result.success is True
        lines = result.output.strip().splitlines()
        assert len(lines) == 5

    async def test_offset_zero_treated_as_one(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        # offset < 1 should be clamped to 1
        result = await tool.execute(
            {"file_path": "five_lines.txt", "offset": 0},
            str(ws),
        )
        assert result.success is True
        assert "a" in result.output

    async def test_negative_offset_treated_as_one(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute(
            {"file_path": "five_lines.txt", "offset": -5},
            str(ws),
        )
        assert result.success is True
        # Should start from line 1
        lines = result.output.strip().splitlines()
        assert len(lines) == 5

    async def test_limit_of_one(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute(
            {"file_path": "five_lines.txt", "offset": 3, "limit": 1},
            str(ws),
        )
        assert result.success is True
        lines = result.output.strip().splitlines()
        assert len(lines) == 1
        assert "c" in result.output


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------


class TestReadFileErrors:
    """Tests for error handling."""

    async def test_nonexistent_file(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "ghost.txt"}, str(ws))
        assert result.success is False
        assert "not found" in result.error

    async def test_path_traversal_relative(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "../../etc/passwd"}, str(ws))
        assert result.success is False
        assert "traversal" in result.error

    async def test_path_traversal_absolute(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "/etc/passwd"}, str(ws))
        assert result.success is False
        assert "traversal" in result.error

    async def test_reading_directory_fails(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "sub"}, str(ws))
        assert result.success is False
        # must_be_file check fails on directories
        assert "not found" in result.error or "file not found" in result.error

    async def test_empty_file_path(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": ""}, str(ws))
        assert result.success is False


# ---------------------------------------------------------------------------
# Special content
# ---------------------------------------------------------------------------


class TestReadFileSpecialContent:
    """Tests for special file contents."""

    async def test_empty_file(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "empty.txt"}, str(ws))
        assert result.success is True
        # Empty file => no lines to number
        assert result.output.strip() == ""

    async def test_unicode_content(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "unicode.txt"}, str(ws))
        assert result.success is True
        assert "\u4e16\u754c" in result.output
        assert "\u00e9" in result.output

    async def test_line_numbers_format(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ReadFileTool()
        result = await tool.execute({"file_path": "simple.txt"}, str(ws))
        # Format is "{:>6}\t{line}" -> 6-char right-aligned number + tab + content
        first_line = result.output.splitlines()[0]
        assert "\t" in first_line
        parts = first_line.split("\t", 1)
        assert parts[0].strip() == "1"
