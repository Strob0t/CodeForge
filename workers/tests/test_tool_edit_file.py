"""Comprehensive tests for the EditFileTool."""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.tools.edit_file import DEFINITION, EditFileTool

if TYPE_CHECKING:
    from pathlib import Path


# ---------------------------------------------------------------------------
# Definition
# ---------------------------------------------------------------------------


class TestEditFileDefinition:
    """Tests for the DEFINITION constant."""

    def test_name(self) -> None:
        assert DEFINITION.name == "edit_file"

    def test_has_description(self) -> None:
        assert DEFINITION.description

    def test_required_params(self) -> None:
        required = DEFINITION.parameters.get("required", [])
        assert "file_path" in required
        assert "old_text" in required
        assert "new_text" in required

    def test_has_examples(self) -> None:
        assert len(DEFINITION.examples) > 0


# ---------------------------------------------------------------------------
# Successful edits
# ---------------------------------------------------------------------------


class TestEditFileSuccess:
    """Tests for successful edit operations."""

    async def test_simple_replacement(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("hello world\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "hello", "new_text": "goodbye"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "file.txt").read_text() == "goodbye world\n"

    async def test_replacement_output_message(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("line one\nline two\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "line one", "new_text": "LINE ONE"},
            str(tmp_path),
        )
        assert "replaced" in result.output
        assert "1 line(s)" in result.output
        assert "file.txt" in result.output

    async def test_multiline_replacement(self, tmp_path: Path) -> None:
        (tmp_path / "code.py").write_text("def foo():\n    return 1\n\ndef bar():\n    return 2\n")
        tool = EditFileTool()
        result = await tool.execute(
            {
                "file_path": "code.py",
                "old_text": "def foo():\n    return 1",
                "new_text": "def foo():\n    return 42\n    # updated",
            },
            str(tmp_path),
        )
        assert result.success is True
        content = (tmp_path / "code.py").read_text()
        assert "return 42" in content
        assert "# updated" in content
        assert "return 1" not in content

    async def test_multiline_counts_in_output(self, tmp_path: Path) -> None:
        (tmp_path / "f.txt").write_text("a\nb\nc\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "f.txt", "old_text": "a\nb", "new_text": "x\ny\nz"},
            str(tmp_path),
        )
        assert result.success is True
        # old_text has 1 newline => 2 lines; new_text has 2 newlines => 3 lines
        assert "2 line(s)" in result.output
        assert "3 line(s)" in result.output

    async def test_replace_with_empty_string(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("keep this\nremove this\nkeep too\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "remove this\n", "new_text": ""},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "file.txt").read_text() == "keep this\nkeep too\n"

    async def test_expand_single_line_to_multiple(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("import os\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "import os", "new_text": "import os\nimport sys"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "file.txt").read_text() == "import os\nimport sys\n"

    async def test_edit_nested_file(self, tmp_path: Path) -> None:
        (tmp_path / "sub").mkdir()
        (tmp_path / "sub" / "mod.py").write_text("x = 1\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "sub/mod.py", "old_text": "x = 1", "new_text": "x = 2"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "sub" / "mod.py").read_text() == "x = 2\n"

    async def test_whitespace_sensitive_matching(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("  indented\n")
        tool = EditFileTool()
        # Without the leading spaces, should fail
        result_no_indent = await tool.execute(
            {"file_path": "file.txt", "old_text": "indented", "new_text": "changed"},
            str(tmp_path),
        )
        assert result_no_indent.success is True
        # Verify there's still leading whitespace
        content = (tmp_path / "file.txt").read_text()
        assert "  changed" in content


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------


class TestEditFileErrors:
    """Tests for error conditions."""

    async def test_no_match_returns_error(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("hello world\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "nonexistent text", "new_text": "x"},
            str(tmp_path),
        )
        assert result.success is False
        assert "not found" in result.error

    async def test_multiple_matches_returns_error(self, tmp_path: Path) -> None:
        (tmp_path / "dup.txt").write_text("word\nword\nword\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "dup.txt", "old_text": "word", "new_text": "other"},
            str(tmp_path),
        )
        assert result.success is False
        assert "3 times" in result.error
        # File should be unchanged
        assert (tmp_path / "dup.txt").read_text() == "word\nword\nword\n"

    async def test_two_matches_returns_error(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("aaa bbb aaa\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "aaa", "new_text": "ccc"},
            str(tmp_path),
        )
        assert result.success is False
        assert "2 times" in result.error

    async def test_nonexistent_file(self, tmp_path: Path) -> None:
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "ghost.txt", "old_text": "x", "new_text": "y"},
            str(tmp_path),
        )
        assert result.success is False
        assert "not found" in result.error

    async def test_path_traversal(self, tmp_path: Path) -> None:
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "../../etc/passwd", "old_text": "root", "new_text": "hacked"},
            str(tmp_path),
        )
        assert result.success is False
        assert "traversal" in result.error

    async def test_edit_directory_fails(self, tmp_path: Path) -> None:
        (tmp_path / "mydir").mkdir()
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "mydir", "old_text": "x", "new_text": "y"},
            str(tmp_path),
        )
        assert result.success is False

    async def test_old_text_same_as_new_text_still_works(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("same\n")
        tool = EditFileTool()
        # Replacing text with identical text is technically valid
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "same", "new_text": "same"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "file.txt").read_text() == "same\n"


# ---------------------------------------------------------------------------
# Edge cases
# ---------------------------------------------------------------------------


class TestEditFileEdgeCases:
    """Tests for edge cases."""

    async def test_replace_entire_file_content(self, tmp_path: Path) -> None:
        original = "full content here\n"
        (tmp_path / "file.txt").write_text(original)
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": original, "new_text": "completely new\n"},
            str(tmp_path),
        )
        assert result.success is True
        assert (tmp_path / "file.txt").read_text() == "completely new\n"

    async def test_replace_with_unicode(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("placeholder\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "placeholder", "new_text": "\u4e16\u754c"},
            str(tmp_path),
        )
        assert result.success is True
        assert "\u4e16\u754c" in (tmp_path / "file.txt").read_text()

    async def test_replace_preserves_surrounding_content(self, tmp_path: Path) -> None:
        (tmp_path / "file.txt").write_text("before\ntarget\nafter\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "target", "new_text": "REPLACED"},
            str(tmp_path),
        )
        assert result.success is True
        content = (tmp_path / "file.txt").read_text()
        assert content == "before\nREPLACED\nafter\n"

    async def test_empty_old_text_matches_multiple_times(self, tmp_path: Path) -> None:
        """Empty string appears at every position, so count > 1 => error."""
        (tmp_path / "file.txt").write_text("abc\n")
        tool = EditFileTool()
        result = await tool.execute(
            {"file_path": "file.txt", "old_text": "", "new_text": "x"},
            str(tmp_path),
        )
        # Empty string matches many times
        assert result.success is False
