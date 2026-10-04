"""Review round of the symlink-safe workspace access (S7-B round 2: C1-C3, C5, C6, C9, C11, C14).

C1  edit_file matches LF text against CRLF files and keeps the file's line endings.
C2  workspace_relative accepts the resolved workspace path.
C3  read_file streams offset/limit and caps only what it returns; edit_file keeps the file cap.
C5  walks are iterative, hold a bounded number of descriptors, stop at a depth limit
    (counted), and the indexers never fail on a walk error.
C6  glob_files and list_directory follow relative directory symlinks inside the workspace
    (cycle-safe, at most MAX_SYMLINKS per path); listings mark them "-> target".
C9  filesystem_state compares with universal newlines.
C11 indexers read files relative to the walk's descriptor; only symlinks go through the root.
C14 edit_file's helper takes the strings it needs.
"""

from __future__ import annotations

import inspect
import os
from typing import TYPE_CHECKING

import pytest
from structlog.testing import capture_logs

from codeforge import workspace_fs
from codeforge._tree_sitter_common import _EXTENSION_MAP, SourceScan, iter_source_files
from codeforge.constants import MAX_WORKSPACE_FILE_BYTES
from codeforge.evaluation.evaluators.filesystem_state import FilesystemStateEvaluator
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec
from codeforge.retrieval import CodeChunker
from codeforge.tools.edit_file import EditFileTool
from codeforge.tools.glob_files import GlobFilesTool
from codeforge.tools.list_directory import ListDirectoryTool
from codeforge.tools.read_file import ReadFileTool
from codeforge.workspace_fs import MAX_SYMLINKS, MAX_WALK_DEPTH, WalkStats, WorkspaceRoot, workspace_relative

if TYPE_CHECKING:
    from pathlib import Path


def _open_fds() -> int:
    return len(os.listdir("/proc/self/fd"))


# ---------------------------------------------------------------------------
# C1, C14: edit_file
# ---------------------------------------------------------------------------


class TestEditFileLineEndings:
    async def test_lf_text_edits_a_crlf_file(self, tmp_path: Path) -> None:
        (tmp_path / "f.txt").write_bytes(b"a\r\nb\r\nc\r\n")
        result = await EditFileTool().execute(
            {"file_path": "f.txt", "old_text": "a\nb", "new_text": "x\ny"}, str(tmp_path)
        )
        assert result.success, result.error
        assert (tmp_path / "f.txt").read_bytes() == b"x\r\ny\r\nc\r\n"
        assert result.diff is not None

    async def test_crlf_text_edits_an_lf_file(self, tmp_path: Path) -> None:
        (tmp_path / "f.txt").write_bytes(b"a\nb\nc\n")
        result = await EditFileTool().execute(
            {"file_path": "f.txt", "old_text": "a\r\nb", "new_text": "x\r\ny"}, str(tmp_path)
        )
        assert result.success, result.error
        assert (tmp_path / "f.txt").read_bytes() == b"x\ny\nc\n"

    @pytest.mark.parametrize("newline", [b"\n", b"\r\n"])
    async def test_trailing_whitespace_match_keeps_the_line_break(self, tmp_path: Path, newline: bytes) -> None:
        nl = newline
        (tmp_path / "f.txt").write_bytes(b"line one  " + nl + b"  keep  " + nl + b"end" + nl)
        result = await EditFileTool().execute(
            {"file_path": "f.txt", "old_text": "line one\n  keep", "new_text": "L1\n  K"}, str(tmp_path)
        )
        assert result.success, result.error
        assert (tmp_path / "f.txt").read_bytes() == b"L1" + nl + b"  K" + nl + b"end" + nl

    async def test_exact_crlf_match_still_works(self, tmp_path: Path) -> None:
        (tmp_path / "f.txt").write_bytes(b"a\r\nb\r\n")
        result = await EditFileTool().execute(
            {"file_path": "f.txt", "old_text": "a\r\nb", "new_text": "z"}, str(tmp_path)
        )
        assert result.success, result.error
        assert (tmp_path / "f.txt").read_bytes() == b"z\r\n"

    async def test_ambiguous_normalized_match_refused(self, tmp_path: Path) -> None:
        (tmp_path / "f.txt").write_bytes(b"a\r\nb\r\na\r\nb\r\n")
        result = await EditFileTool().execute(
            {"file_path": "f.txt", "old_text": "a\nb", "new_text": "x"}, str(tmp_path)
        )
        assert not result.success
        assert (tmp_path / "f.txt").read_bytes() == b"a\r\nb\r\na\r\nb\r\n"

    def test_edit_helper_takes_strings(self) -> None:
        params = inspect.signature(EditFileTool._edit).parameters
        assert "arguments" not in params
        assert {params[name].annotation for name in ("old_text", "new_text")} == {"str"}


# ---------------------------------------------------------------------------
# C2: workspace_relative
# ---------------------------------------------------------------------------


def test_workspace_relative_accepts_the_resolved_workspace(tmp_path: Path) -> None:
    real = tmp_path / "real"
    (real / "src").mkdir(parents=True)
    link = tmp_path / "link"
    link.symlink_to(real)
    assert workspace_relative(str(link), f"{real}/src/a.py") == "src/a.py"
    assert workspace_relative(str(link), f"{link}/src/a.py") == "src/a.py"
    assert workspace_relative(str(link), str(real)) == "."
    assert workspace_relative(str(link), f"{tmp_path}/other/a.py") == f"{tmp_path}/other/a.py"


# ---------------------------------------------------------------------------
# C3: read_file streams, edit_file keeps the cap
# ---------------------------------------------------------------------------


@pytest.fixture
def big_file(tmp_path: Path) -> Path:
    line = b"x" * 99 + b"\n"
    lines = MAX_WORKSPACE_FILE_BYTES // len(line) + 2000
    (tmp_path / "big.txt").write_bytes(line * lines)
    return tmp_path


class TestReadFileStreaming:
    async def test_offset_and_limit_beyond_the_cap(self, big_file: Path) -> None:
        total = (big_file / "big.txt").stat().st_size // 100
        result = await ReadFileTool().execute({"file_path": "big.txt", "offset": total - 1, "limit": 5}, str(big_file))
        assert result.success, result.error
        assert result.output.splitlines() == [f"{total - 1:>6}\t{'x' * 99}", f"{total:>6}\t{'x' * 99}"]

    async def test_whole_file_output_is_capped(self, big_file: Path) -> None:
        result = await ReadFileTool().execute({"file_path": "big.txt"}, str(big_file))
        assert result.success, result.error
        assert len(result.output.encode()) <= MAX_WORKSPACE_FILE_BYTES + 200
        assert "truncated" in result.output.splitlines()[-1]

    async def test_universal_newlines(self, tmp_path: Path) -> None:
        (tmp_path / "f.txt").write_bytes(b"a\r\nb\rc\n")
        result = await ReadFileTool().execute({"file_path": "f.txt", "offset": 2}, str(tmp_path))
        assert result.output == f"{2:>6}\tb\n{3:>6}\tc\n"

    async def test_edit_keeps_the_whole_file_cap(self, big_file: Path) -> None:
        result = await EditFileTool().execute({"file_path": "big.txt", "old_text": "x", "new_text": "y"}, str(big_file))
        assert not result.success
        assert "too large" in result.error


# ---------------------------------------------------------------------------
# C5: iterative, bounded walks
# ---------------------------------------------------------------------------


def _deep(tmp_path: Path, depth: int) -> Path:
    ws = tmp_path / "ws"
    path = ws
    for _ in range(depth):
        path = path / "d"
    path.mkdir(parents=True)
    (path / "leaf.py").write_text("def leaf():\n    return 1\n")
    return ws


class TestWalkBounds:
    def test_depth_limit_is_counted(self, tmp_path: Path) -> None:
        ws = _deep(tmp_path, MAX_WALK_DEPTH + 5)
        stats = WalkStats()
        with WorkspaceRoot(str(ws)) as root:
            depths = [0 if d == "." else d.count("/") + 1 for d, _, _, _ in root.walk(stats=stats)]
        assert max(depths) == MAX_WALK_DEPTH
        assert stats.too_deep == 1

    def test_descriptors_are_bounded(self, tmp_path: Path) -> None:
        ws = _deep(tmp_path, 100)
        before = _open_fds()
        most = 0
        with WorkspaceRoot(str(ws)) as root:
            for _ in root.walk():
                most = max(most, _open_fds())
        assert most - before <= workspace_fs._MAX_HELD_DIRS + 4
        assert _open_fds() == before

    def test_walk_closes_everything_when_stopped_early(self, tmp_path: Path) -> None:
        ws = _deep(tmp_path, 10)
        with WorkspaceRoot(str(ws)) as root:
            before = _open_fds()
            walk = root.walk()
            for _ in range(5):
                next(walk)
            walk.close()
            assert _open_fds() == before

    def test_indexers_survive_a_walk_error(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        (tmp_path / "ok").mkdir()
        (tmp_path / "ok" / "a.py").write_text("def a():\n    return 1\n")
        (tmp_path / "bad").mkdir()
        (tmp_path / "bad" / "b.py").write_text("def b():\n    return 1\n")
        real_scandir = os.scandir
        bad_ino = (tmp_path / "bad").stat().st_ino

        def failing_scandir(fd: int) -> object:
            if os.fstat(fd).st_ino == bad_ino:
                raise PermissionError("denied")
            return real_scandir(fd)

        monkeypatch.setattr(workspace_fs.os, "scandir", failing_scandir)
        with capture_logs() as logs:
            per_file = CodeChunker().chunk_workspace_by_file(str(tmp_path))
        assert sorted(per_file) == ["ok/a.py"]
        skipped = [entry for entry in logs if entry.get("event") == "skipped workspace entries"]
        assert skipped
        assert skipped[0]["unreadable_dirs"] == 1

    def test_depth_limit_is_logged_by_the_indexers(self, tmp_path: Path) -> None:
        ws = _deep(tmp_path, MAX_WALK_DEPTH + 2)
        with capture_logs() as logs:
            CodeChunker().chunk_workspace_by_file(str(ws))
        skipped = [entry for entry in logs if entry.get("event") == "skipped workspace entries"]
        assert skipped
        assert skipped[0]["too_deep"] == 1


# ---------------------------------------------------------------------------
# C6: the glob and listing tools follow relative directory symlinks inside
# ---------------------------------------------------------------------------


@pytest.fixture
def monorepo(tmp_path: Path) -> Path:
    ws = tmp_path / "ws"
    outside = tmp_path / "out"
    (ws / "packages" / "pkg").mkdir(parents=True)
    (ws / "apps").mkdir()
    outside.mkdir()
    (outside / "secret.ts").write_text("outside")
    (ws / "packages" / "pkg" / "index.ts").write_text("export const pkg = 1\n")
    (ws / "apps" / "web").symlink_to("../packages/pkg")
    (ws / "packages" / "pkg" / "self").symlink_to(".")  # back to its own directory
    (ws / "apps" / "abs").symlink_to(ws / "packages" / "pkg")  # absolute: leaves the workspace
    (ws / "apps" / "out").symlink_to("../../out")
    return ws


class TestFollowDirectorySymlinks:
    async def test_glob(self, monorepo: Path) -> None:
        result = await GlobFilesTool().execute({"pattern": "**/*.ts"}, str(monorepo))
        assert result.success, result.error
        assert result.output.splitlines() == ["apps/web/index.ts", "packages/pkg/index.ts"]

    async def test_glob_below_a_symlinked_prefix(self, monorepo: Path) -> None:
        result = await GlobFilesTool().execute({"pattern": "apps/web/*.ts"}, str(monorepo))
        assert result.output.splitlines() == ["apps/web/index.ts"]

    def test_hop_limit(self, tmp_path: Path) -> None:
        for i in range(MAX_SYMLINKS + 3):
            (tmp_path / f"d{i}").mkdir()
            (tmp_path / f"d{i}" / "f.txt").write_text(str(i))
            (tmp_path / f"d{i}" / "next").symlink_to(f"../d{i + 1}")
        with WorkspaceRoot(str(tmp_path)) as root:
            walked = {d: (dirs, files) for d, dirs, files, _ in root.walk("d0", follow_dir_symlinks=True)}
        deepest = max(walked, key=lambda d: d.count("next"))
        assert deepest.count("next") == MAX_SYMLINKS
        # One more symlink on the path cannot be resolved: it is no directory of the workspace.
        dirnames, filenames = walked[deepest]
        assert dirnames == []
        assert sorted(filenames) == ["f.txt", "next"]

    async def test_listing_marks_and_enters_symlinked_directories(self, monorepo: Path) -> None:
        result = await ListDirectoryTool().execute({"path": "apps", "recursive": True}, str(monorepo))
        assert result.success, result.error
        lines = result.output.splitlines()
        assert "[DIR]  apps/web -> packages/pkg" in lines
        assert "[FILE] apps/web/index.ts" in lines
        assert "[DIR]  apps/web/self -> packages/pkg" in lines
        assert not [line for line in lines if line.startswith("[FILE] apps/web/self/")]
        assert "[FILE] apps/abs" in lines
        assert "[FILE] apps/out" in lines
        assert "outside" not in result.output

    def test_indexers_still_skip_symlinked_directories(self, monorepo: Path) -> None:
        with WorkspaceRoot(str(monorepo)) as root:
            files = [rel for rel, _ in iter_source_files(root, {".ts"}, SourceScan("test"))]
        assert files == ["packages/pkg/index.ts"]


# ---------------------------------------------------------------------------
# C9: filesystem_state
# ---------------------------------------------------------------------------


async def test_filesystem_state_compares_with_universal_newlines(tmp_path: Path) -> None:
    (tmp_path / "out.txt").write_bytes(b"a\r\nb\rc\n")
    task = TaskSpec(id="t", name="t", input="", metadata={"expected_files": '{"out.txt": "a\\nb\\nc\\n"}'})
    dims = await FilesystemStateEvaluator(str(tmp_path)).evaluate(task, ExecutionResult(actual_output=""))
    assert dims[0].score == 1.0, dims[0].details


# ---------------------------------------------------------------------------
# C11: indexers read relative to the walk
# ---------------------------------------------------------------------------


def test_indexers_resolve_only_symlinks_through_the_root(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    (tmp_path / "a" / "b").mkdir(parents=True)
    (tmp_path / "a" / "b" / "deep.py").write_text("def deep():\n    return 1\n")
    (tmp_path / "top.py").write_text("def top():\n    return 1\n")
    (tmp_path / "alias.py").symlink_to("a/b/deep.py")
    resolved: list[str] = []
    real_read_bytes = WorkspaceRoot.read_bytes

    def recording_read_bytes(self: WorkspaceRoot, rel: str, *, max_bytes: int | None = None) -> bytes:
        resolved.append(rel)
        return real_read_bytes(self, rel, max_bytes=max_bytes)

    monkeypatch.setattr(WorkspaceRoot, "read_bytes", recording_read_bytes)
    with WorkspaceRoot(str(tmp_path)) as root:
        files = sorted(rel for rel, _ in iter_source_files(root, _EXTENSION_MAP, SourceScan("test")))
    assert files == ["a/b/deep.py", "alias.py", "top.py"]
    assert resolved == ["alias.py"]
