"""Comprehensive tests for SearchFilesTool, GlobFilesTool, and ListDirectoryTool."""

from __future__ import annotations

from typing import TYPE_CHECKING

import pytest

from codeforge.tools.glob_files import DEFINITION as GLOB_DEFINITION
from codeforge.tools.glob_files import GlobFilesTool
from codeforge.tools.list_directory import DEFINITION as LISTDIR_DEFINITION
from codeforge.tools.list_directory import ListDirectoryTool
from codeforge.tools.search_files import DEFINITION as SEARCH_DEFINITION
from codeforge.tools.search_files import SearchFilesTool

if TYPE_CHECKING:
    from pathlib import Path


# ---------------------------------------------------------------------------
# Shared workspace fixture
# ---------------------------------------------------------------------------


def _make_workspace(tmp_path: Path) -> Path:
    """Create a workspace with files for searching/globbing/listing."""
    # Root files
    (tmp_path / "readme.md").write_text("# Project\nWelcome to the project.\n")
    (tmp_path / "main.py").write_text("import os\nimport sys\n\ndef main():\n    print('hello')\n")
    (tmp_path / "config.yaml").write_text("key: value\nport: 8080\n")

    # src/ directory
    (tmp_path / "src").mkdir()
    (tmp_path / "src" / "app.py").write_text("class App:\n    def run(self):\n        pass\n")
    (tmp_path / "src" / "utils.py").write_text("def helper():\n    return 42\n\ndef helper2():\n    return 99\n")
    (tmp_path / "src" / "data.json").write_text('{"items": []}\n')

    # tests/ directory
    (tmp_path / "tests").mkdir()
    (tmp_path / "tests" / "test_app.py").write_text("def test_run():\n    assert True\n")
    (tmp_path / "tests" / "test_utils.py").write_text("def test_helper():\n    assert helper() == 42\n")

    # Nested directory
    (tmp_path / "src" / "sub").mkdir()
    (tmp_path / "src" / "sub" / "module.py").write_text("MOD_VAR = 'hello'\n")

    # Empty directory
    (tmp_path / "empty_dir").mkdir()

    return tmp_path


# ===========================================================================
# SearchFilesTool
# ===========================================================================


class TestSearchFilesDefinition:
    """Tests for the search_files DEFINITION."""

    def test_name(self) -> None:
        assert SEARCH_DEFINITION.name == "search_files"

    def test_pattern_is_required(self) -> None:
        assert "pattern" in SEARCH_DEFINITION.parameters.get("required", [])

    def test_has_optional_params(self) -> None:
        props = SEARCH_DEFINITION.parameters.get("properties", {})
        assert "path" in props
        assert "include" in props
        assert "regex" in props


class TestSearchFilesBasic:
    """Tests for basic search functionality."""

    async def test_fixed_string_match(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute({"pattern": "def main"}, str(ws))
        assert result.success is True
        assert "def main" in result.output
        assert "main.py" in result.output

    async def test_no_matches_returns_no_matches_message(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute({"pattern": "zzz_nonexistent_zzz"}, str(ws))
        assert result.success is True
        assert "no matches" in result.output

    async def test_results_include_line_numbers(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute({"pattern": "import os"}, str(ws))
        assert result.success is True
        # grep -n output has format: filepath:linenum:content
        parts = result.output.strip().split(":")
        assert len(parts) >= 3

    async def test_search_in_subdirectory(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "def helper", "path": "src"},
            str(ws),
        )
        assert result.success is True
        assert "helper" in result.output
        # Should find in utils.py under src/
        assert "utils.py" in result.output

    async def test_search_with_include_filter(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "def", "include": "*.py"},
            str(ws),
        )
        assert result.success is True
        assert "def" in result.output
        # Should not match in non-py files
        assert "config.yaml" not in result.output

    async def test_include_restricts_to_file_type(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "key", "include": "*.yaml"},
            str(ws),
        )
        assert result.success is True
        assert "config.yaml" in result.output


class TestSearchFilesRegex:
    """Tests for regex search mode."""

    async def test_regex_pattern(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "def\\s+\\w+", "regex": True},
            str(ws),
        )
        assert result.success is True
        assert "def " in result.output

    async def test_regex_or_pattern(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "import os|import sys", "regex": True},
            str(ws),
        )
        assert result.success is True
        assert "import" in result.output

    async def test_invalid_regex_returns_error(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "[invalid", "regex": True},
            str(ws),
        )
        assert result.success is False
        assert "invalid regex" in result.error

    async def test_fixed_string_does_not_interpret_regex(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        # Create a file with literal regex chars
        (ws / "regex_test.txt").write_text("match [this] pattern\n")
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "[this]", "regex": False},
            str(ws),
        )
        assert result.success is True
        assert "[this]" in result.output


class TestSearchFilesErrors:
    """Tests for error handling in search."""

    async def test_path_traversal_relative(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "test", "path": "../../etc"},
            str(ws),
        )
        assert result.success is False
        assert "traversal" in result.error

    async def test_path_traversal_absolute(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "test", "path": "/etc"},
            str(ws),
        )
        assert result.success is False
        assert "traversal" in result.error

    async def test_nonexistent_subdirectory(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = SearchFilesTool()
        result = await tool.execute(
            {"pattern": "test", "path": "no_such_dir"},
            str(ws),
        )
        assert result.success is False
        assert "not found" in result.error


# ===========================================================================
# GlobFilesTool
# ===========================================================================


class TestGlobFilesDefinition:
    """Tests for the glob_files DEFINITION."""

    def test_name(self) -> None:
        assert GLOB_DEFINITION.name == "glob_files"

    def test_pattern_is_required(self) -> None:
        assert "pattern" in GLOB_DEFINITION.parameters.get("required", [])


class TestGlobFilesBasic:
    """Tests for basic glob functionality."""

    async def test_glob_all_python_files(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*.py"}, str(ws))
        assert result.success is True
        assert "main.py" in result.output
        assert "app.py" in result.output
        assert "module.py" in result.output

    async def test_glob_root_level_only(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "*.py"}, str(ws))
        assert result.success is True
        assert "main.py" in result.output
        # Nested files should NOT be included with *.py (non-recursive)
        assert "app.py" not in result.output

    async def test_glob_yaml_files(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*.yaml"}, str(ws))
        assert result.success is True
        assert "config.yaml" in result.output

    async def test_glob_specific_directory(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "tests/*.py"}, str(ws))
        assert result.success is True
        assert "test_app.py" in result.output
        assert "test_utils.py" in result.output
        # Should NOT include src/ files
        assert "src/app.py" not in result.output

    async def test_no_matches_returns_message(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*.rs"}, str(ws))
        assert result.success is True
        assert "no matches" in result.output

    async def test_returns_only_files_not_directories(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*"}, str(ws))
        assert result.success is True
        # Directories should not be in the results
        for line in result.output.splitlines():
            if line and not line.startswith("..."):
                assert not (ws / line).is_dir()

    async def test_results_are_sorted(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*.py"}, str(ws))
        lines = [ln for ln in result.output.splitlines() if ln and not ln.startswith("...")]
        assert lines == sorted(lines)

    async def test_results_are_relative_paths(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*.py"}, str(ws))
        for line in result.output.splitlines():
            if line and not line.startswith("..."):
                assert not line.startswith("/")

    async def test_json_files(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*.json"}, str(ws))
        assert result.success is True
        assert "data.json" in result.output


class TestGlobFilesEdgeCases:
    """Tests for glob edge cases."""

    async def test_empty_workspace(self, tmp_path: Path) -> None:
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*"}, str(tmp_path))
        assert result.success is True
        assert "no matches" in result.output

    async def test_star_star_pattern(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = GlobFilesTool()
        result = await tool.execute({"pattern": "**/*"}, str(ws))
        assert result.success is True
        # Should find files across all directories
        assert "main.py" in result.output
        assert "module.py" in result.output


# ===========================================================================
# ListDirectoryTool
# ===========================================================================


class TestListDirectoryDefinition:
    """Tests for the list_directory DEFINITION."""

    def test_name(self) -> None:
        assert LISTDIR_DEFINITION.name == "list_directory"

    def test_has_path_and_recursive_params(self) -> None:
        props = LISTDIR_DEFINITION.parameters.get("properties", {})
        assert "path" in props
        assert "recursive" in props


class TestListDirectoryBasic:
    """Tests for basic directory listing."""

    async def test_list_root(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "."}, str(ws))
        assert result.success is True
        assert "[DIR]" in result.output
        assert "[FILE]" in result.output

    async def test_directories_listed_before_files(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "."}, str(ws))
        lines = result.output.strip().splitlines()
        # Find the transition point from DIR to FILE
        saw_file = False
        for line in lines:
            if "[FILE]" in line:
                saw_file = True
            if saw_file and "[DIR]" in line:
                pytest.fail("Directory listed after a file; expected dirs first")

    async def test_lists_subdirectory(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "src"}, str(ws))
        assert result.success is True
        assert "app.py" in result.output
        assert "utils.py" in result.output

    async def test_default_path_is_root(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({}, str(ws))
        assert result.success is True
        # Should list root directory contents
        assert "main.py" in result.output or "[DIR]" in result.output

    async def test_recursive_listing(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": ".", "recursive": True}, str(ws))
        assert result.success is True
        # Should find nested files
        assert "module.py" in result.output
        assert "app.py" in result.output

    async def test_non_recursive_does_not_show_nested(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": ".", "recursive": False}, str(ws))
        assert result.success is True
        # Should list "src" dir but not its contents like app.py directly
        assert "[DIR]" in result.output
        # module.py is inside src/sub, should not appear in flat listing of root
        assert "module.py" not in result.output


class TestListDirectoryErrors:
    """Tests for error handling."""

    async def test_nonexistent_directory(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "no_such_dir"}, str(ws))
        assert result.success is False
        assert "not a directory" in result.error

    async def test_file_instead_of_directory(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "main.py"}, str(ws))
        assert result.success is False
        assert "not a directory" in result.error

    async def test_path_traversal(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "../../"}, str(ws))
        assert result.success is False
        assert "traversal" in result.error

    async def test_path_traversal_absolute(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "/etc"}, str(ws))
        assert result.success is False
        assert "traversal" in result.error


class TestListDirectoryEdgeCases:
    """Tests for edge cases."""

    async def test_empty_directory(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "empty_dir"}, str(ws))
        assert result.success is True
        assert "empty directory" in result.output

    async def test_dir_prefix_format(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "."}, str(ws))
        # Check format: "[DIR]  name" with two spaces
        for line in result.output.splitlines():
            if "[DIR]" in line:
                assert "[DIR]  " in line
            if "[FILE]" in line:
                assert "[FILE] " in line

    async def test_relative_paths_in_output(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "src"}, str(ws))
        # Paths should be relative to workspace
        for line in result.output.splitlines():
            if "[DIR]" in line or "[FILE]" in line:
                # Path portion should start with "src/"
                parts = line.split("  ", 1) if "[DIR]" in line else line.split(" ", 1)
                path_part = parts[-1].strip()
                assert path_part.startswith("src/") or path_part == "src"

    async def test_recursive_shows_nested_dirs(self, tmp_path: Path) -> None:
        ws = _make_workspace(tmp_path)
        tool = ListDirectoryTool()
        result = await tool.execute({"path": "src", "recursive": True}, str(ws))
        assert result.success is True
        assert "[DIR]" in result.output
        assert "sub" in result.output
        assert "module.py" in result.output
