"""The (command, path) the Go policy evaluates for agent loop and Claude Code tool calls.

One mapping for both: paths are sent relative to the real workspace (Go
compares them with the project's workspace path, which may be a symlink), a
path outside the workspace is sent as its real absolute path (Go denies it),
and search tools without a path or with an escaping glob pattern send the
directory they can reach.
"""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

import pytest

from codeforge.policy_args import policy_request_args

if TYPE_CHECKING:
    from pathlib import Path


@pytest.fixture
def ws(tmp_path: Path) -> Path:
    workspace = tmp_path / "ws"
    (workspace / "src").mkdir(parents=True)
    return workspace


def _path(tool: str, arguments: dict[str, object], workspace: Path) -> str:
    return policy_request_args(tool, arguments, str(workspace))[1]


class TestCommand:
    @pytest.mark.parametrize("tool", ["bash", "Bash", "Monitor"])
    def test_command_tools_send_the_full_command(self, tool: str, ws: Path) -> None:
        command = "go test ./... && curl https://evil.example"
        assert policy_request_args(tool, {"command": command, "timeout": 5}, str(ws)) == (command, "")

    @pytest.mark.parametrize("tool", ["Read", "mcp__x__y", "propose_goal", "SomeNewTool"])
    def test_other_tools_send_no_command(self, tool: str, ws: Path) -> None:
        assert policy_request_args(tool, {"command": "curl x", "file_path": "a"}, str(ws))[0] == ""

    @pytest.mark.parametrize("value", [["rm", "-rf"], None, 42])
    def test_command_must_be_a_string(self, value: object, ws: Path) -> None:
        assert policy_request_args("Bash", {"command": value}, str(ws)) == ("", "")


class TestFilePaths:
    @pytest.mark.parametrize(
        ("tool", "key"),
        [
            ("read_file", "file_path"),
            ("write_file", "file_path"),
            ("edit_file", "file_path"),
            ("Read", "file_path"),
            ("Write", "file_path"),
            ("Edit", "file_path"),
            ("MultiEdit", "file_path"),
            ("NotebookEdit", "notebook_path"),
        ],
    )
    def test_file_tools_send_their_file(self, tool: str, key: str, ws: Path) -> None:
        assert _path(tool, {key: "src/a.py"}, ws) == "src/a.py"

    def test_absolute_path_inside_is_relative(self, ws: Path) -> None:
        assert _path("Read", {"file_path": str(ws / "src" / "a.py")}, ws) == "src/a.py"

    def test_workspace_root_is_dot(self, ws: Path) -> None:
        assert _path("LS", {"path": str(ws)}, ws) == "."

    @pytest.mark.parametrize(("raw", "expected"), [("a/../.env", ".env"), ("./secrets/a", "secrets/a")])
    def test_relative_path_is_normalized(self, raw: str, expected: str, ws: Path) -> None:
        assert _path("edit_file", {"file_path": raw}, ws) == expected

    def test_path_outside_is_absolute(self, ws: Path) -> None:
        assert _path("Read", {"file_path": "/etc/passwd"}, ws) == os.path.realpath("/etc/passwd")

    def test_climbing_path_is_absolute_outside(self, ws: Path) -> None:
        assert _path("Read", {"file_path": "../../x"}, ws) == os.path.realpath(ws.parent.parent / "x")

    def test_sibling_with_common_prefix_is_outside(self, ws: Path) -> None:
        sibling = ws.parent / "ws-other" / "a"
        assert _path("Read", {"file_path": str(sibling)}, ws) == str(sibling)

    def test_home_is_expanded(self, ws: Path, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
        home = tmp_path / "home"
        home.mkdir()
        monkeypatch.setenv("HOME", str(home))
        assert _path("Read", {"file_path": "~/.ssh/id_rsa"}, ws) == str(home / ".ssh" / "id_rsa")

    @pytest.mark.parametrize("value", [None, 42, ""])
    def test_missing_path_is_empty(self, value: object, ws: Path) -> None:
        assert _path("Edit", {"file_path": value}, ws) == ""

    @pytest.mark.parametrize("raw", ["a\x00b", "src/\x00", "~\x00/x"])
    def test_unresolvable_path_is_outside(self, raw: str, ws: Path) -> None:
        # A path the OS cannot resolve (NUL byte from the model) must not
        # raise out of the agent loop; it is reported as outside, which Go denies.
        assert _path("Write", {"file_path": raw}, ws) == "/"
        assert _path("Glob", {"pattern": raw}, ws) == "/"


class TestSymlinks:
    def test_symlinked_workspace(self, ws: Path, tmp_path: Path) -> None:
        """Go compares with the project's workspace path, which may be a symlink to the real directory."""
        link = tmp_path / "link-to-ws"
        link.symlink_to(ws)

        assert _path("Read", {"file_path": str(link / "src" / "a.py")}, link) == "src/a.py"
        assert _path("Read", {"file_path": str(ws / "src" / "a.py")}, link) == "src/a.py"
        assert _path("Read", {"file_path": "src/a.py"}, link) == "src/a.py"

    def test_symlink_inside_pointing_outside_is_outside(self, ws: Path, tmp_path: Path) -> None:
        outside = tmp_path / "secret"
        outside.mkdir()
        (ws / "escape").symlink_to(outside)

        assert _path("Read", {"file_path": str(ws / "escape" / "key")}, ws) == str(outside / "key")


class TestDirectoryTools:
    @pytest.mark.parametrize(
        ("tool", "arguments"),
        [
            ("search_files", {"pattern": "TODO"}),
            ("list_directory", {}),
            ("glob_files", {"pattern": "**/*.go"}),
            ("Grep", {"pattern": "TODO"}),
            ("Glob", {"pattern": "**/*.py"}),
            ("LS", {}),
        ],
    )
    def test_without_path_the_workspace_root(self, tool: str, arguments: dict[str, object], ws: Path) -> None:
        assert _path(tool, arguments, ws) == "."

    @pytest.mark.parametrize("tool", ["search_files", "list_directory", "Grep", "Glob", "LS"])
    def test_path_argument(self, tool: str, ws: Path) -> None:
        assert _path(tool, {"pattern": "*", "path": "src"}, ws) == "src"

    @pytest.mark.parametrize(
        ("arguments", "expected"),
        [
            ({"pattern": "src/**/*.py"}, "src"),
            ({"pattern": "*.py", "path": "src"}, "src"),
            ({"pattern": "{src,lib}/*.ts"}, "."),
            # A wildcard may stand for any directory, so each ".." may climb one level.
            ({"pattern": "a/*/../b/*"}, "."),
        ],
    )
    def test_glob_inside(self, arguments: dict[str, object], expected: str, ws: Path) -> None:
        assert _path("Glob", arguments, ws) == expected

    @pytest.mark.parametrize(
        "pattern",
        [
            "/home/worker/.ssh/*",
            "/*",
            "../*",
            "*/../../x/*",
            "**/../../*",
            "src/../../*",
            "{/etc,src}/*",
            "{src,../..}/*",
            "~/.ssh/*",
        ],
    )
    def test_escaping_glob_pattern_is_outside(self, pattern: str, ws: Path) -> None:
        path = _path("Glob", {"pattern": pattern}, ws)
        assert os.path.isabs(path), path

    def test_absolute_glob_pattern_sends_its_directory(self, ws: Path) -> None:
        assert _path("Glob", {"pattern": "/home/worker/.ssh/*"}, ws) == os.path.realpath("/home/worker/.ssh")

    @pytest.mark.parametrize(
        ("tool", "arguments"),
        [
            ("Grep", {"pattern": "x", "glob": "/etc/*"}),
            ("Grep", {"pattern": "x", "path": "src", "glob": "../../../*"}),
            ("search_files", {"pattern": "x", "include": "../../*"}),
            ("glob_files", {"pattern": "../*"}),
        ],
    )
    def test_escaping_filter_glob_is_outside(self, tool: str, arguments: dict[str, object], ws: Path) -> None:
        assert os.path.isabs(_path(tool, arguments, ws))

    def test_filter_glob_inside_keeps_the_directory(self, ws: Path) -> None:
        assert _path("Grep", {"pattern": "x", "path": "src", "glob": "*.{ts,tsx}"}, ws) == "src"


def test_unknown_tools_send_nothing(ws: Path) -> None:
    arguments = {"command": "curl x", "path": "/etc", "file_path": "/etc/passwd", "pattern": "/*"}
    for tool in ("mcp__github__create_issue", "propose_goal", "WebFetch", "EnterWorktree"):
        assert policy_request_args(tool, arguments, str(ws)) == ("", "")
