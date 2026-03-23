"""Tests for post-write syntax checking module."""

from __future__ import annotations

from codeforge.tools._lint import post_write_check


class TestPostWriteCheckPython:
    """Syntax checking for Python files via ast.parse."""

    def test_valid_python(self) -> None:
        result = post_write_check("test.py", "def hello():\n    return 42\n")
        assert result is None

    def test_invalid_python_syntax(self) -> None:
        result = post_write_check("test.py", "def hello()\n    return 42\n")
        assert result is not None
        assert "SyntaxError" in result or "syntax" in result.lower()

    def test_empty_python_file(self) -> None:
        result = post_write_check("test.py", "")
        assert result is None

    def test_python_with_only_comments(self) -> None:
        result = post_write_check("test.py", "# just a comment\n")
        assert result is None

    def test_pyw_extension(self) -> None:
        result = post_write_check("gui.pyw", "import tkinter\n")
        assert result is None

    def test_html_escape_in_python(self) -> None:
        """The </n artifact from local models should trigger syntax error."""
        result = post_write_check("test.py", "x = 1</n    y = 2\n")
        assert result is not None

    def test_syntax_error_includes_line_number(self) -> None:
        result = post_write_check("test.py", "x = 1\ny = 2\ndef bad(\n")
        assert result is not None
        assert "line" in result.lower() or "3" in result


class TestPostWriteCheckUnknown:
    """Unknown extensions should be silently skipped."""

    def test_unknown_extension_skips(self) -> None:
        result = post_write_check("test.xyz", "gibberish{{{")
        assert result is None

    def test_no_extension_skips(self) -> None:
        result = post_write_check("Makefile", "all:\n\techo hello")
        assert result is None

    def test_text_file_skips(self) -> None:
        result = post_write_check("readme.txt", "just text")
        assert result is None

    def test_go_file_skips_gracefully(self) -> None:
        """Go files should not error even if content is invalid."""
        result = post_write_check("main.go", "package main{{{")
        assert result is None

    def test_ts_file_skips_gracefully(self) -> None:
        """TS files should not error even if content is invalid."""
        result = post_write_check("app.ts", "const x: = invalid{{{")
        assert result is None
