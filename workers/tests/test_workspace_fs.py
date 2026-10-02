"""Symlink-safe workspace file access (KI-95): codeforge.workspace_fs.

Agents (the tool user) can create symlinks, FIFOs and sockets anywhere in a
workspace and swap them at any time. The worker reads and writes workspace
files in its own process, so no path may resolve outside the workspace and no
special file may block or be read.
"""

from __future__ import annotations

import os
import socket
import threading
from typing import TYPE_CHECKING

import pytest

from codeforge import workspace_fs
from codeforge.workspace_fs import (
    MAX_SYMLINKS,
    BlockedPathError,
    FileTooLargeError,
    NotRegularFileError,
    PathLeavesWorkspaceError,
    WorkspacePathError,
    WorkspaceRoot,
    workspace_relative,
)

if TYPE_CHECKING:
    from collections.abc import Callable, Iterator
    from pathlib import Path

OUTSIDE_TEXT = "outside secret"


@pytest.fixture
def tree(tmp_path: Path) -> tuple[Path, Path]:
    """A workspace next to a directory outside it that holds a secret."""
    ws = tmp_path / "ws"
    out = tmp_path / "out"
    (ws / "src" / "sub").mkdir(parents=True)
    out.mkdir()
    (out / "secret.txt").write_text(OUTSIDE_TEXT)
    (ws / "src" / "a.py").write_text("inside\n")
    (ws / "src" / "sub" / "b.py").write_text("deep\n")
    return ws, out


@pytest.fixture
def root(tree: tuple[Path, Path]) -> Iterator[WorkspaceRoot]:
    with WorkspaceRoot(str(tree[0])) as opened:
        yield opened


def _no_block(fn: Callable[[], object], seconds: float = 5.0) -> BaseException | None:
    """Run fn in a thread; fail the test when it blocks (a FIFO opened without O_NONBLOCK)."""
    outcome: list[BaseException | None] = []

    def target() -> None:
        try:
            fn()
            outcome.append(None)
        except BaseException as exc:
            outcome.append(exc)

    thread = threading.Thread(target=target, daemon=True)
    thread.start()
    thread.join(seconds)
    assert not thread.is_alive(), "the call blocked"
    return outcome[0]


# ---------------------------------------------------------------------------
# Reads
# ---------------------------------------------------------------------------


class TestRead:
    def test_regular_file(self, root: WorkspaceRoot) -> None:
        assert root.read_text("src/a.py") == "inside\n"
        assert root.read_bytes("./src//sub/b.py") == b"deep\n"

    def test_dot_dot_inside_the_workspace(self, root: WorkspaceRoot) -> None:
        assert root.read_text("src/sub/../a.py") == "inside\n"

    @pytest.mark.parametrize(
        "rel", ["../out/secret.txt", "src/../../out/secret.txt", "src/sub/../../../out/secret.txt"]
    )
    def test_dot_dot_escape_refused(self, root: WorkspaceRoot, rel: str) -> None:
        with pytest.raises(PathLeavesWorkspaceError, match="path leaves the workspace"):
            root.read_bytes(rel)

    def test_absolute_path_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        with pytest.raises(PathLeavesWorkspaceError):
            root.read_bytes(str(tree[1] / "secret.txt"))
        # Even one that names a file inside: the helper takes workspace-relative paths only.
        with pytest.raises(PathLeavesWorkspaceError):
            root.read_bytes(str(tree[0] / "src" / "a.py"))

    def test_symlink_to_file_outside_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, out = tree
        (ws / "rel").symlink_to("../out/secret.txt")
        (ws / "abs").symlink_to(out / "secret.txt")
        for rel in ("rel", "abs"):
            with pytest.raises(PathLeavesWorkspaceError):
                root.read_bytes(rel)

    def test_symlinked_directory_outside_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, out = tree
        (ws / "src" / "outdir").symlink_to("../../out")
        (ws / "absdir").symlink_to(out)
        for rel in ("src/outdir/secret.txt", "absdir/secret.txt"):
            with pytest.raises(PathLeavesWorkspaceError):
                root.read_bytes(rel)

    def test_symlink_inside_followed(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, _ = tree
        (ws / "link.py").symlink_to("src/a.py")
        (ws / "src" / "up").symlink_to("..")
        (ws / "srclink").symlink_to("src")
        assert root.read_text("link.py") == "inside\n"
        assert root.read_text("srclink/sub/b.py") == "deep\n"
        assert root.read_text("src/up/src/a.py") == "inside\n"
        assert root.resolve("srclink/sub/b.py") == "src/sub/b.py"

    def test_symlink_inside_that_climbs_out_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, _ = tree
        (ws / "src" / "up").symlink_to("..")
        with pytest.raises(PathLeavesWorkspaceError):
            root.read_bytes("src/up/../out/secret.txt")

    def test_symlink_chain_limit(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, _ = tree
        previous = "src/a.py"
        for i in range(MAX_SYMLINKS):
            (ws / f"l{i}").symlink_to(previous)
            previous = f"l{i}"
        assert root.read_text(f"l{MAX_SYMLINKS - 1}") == "inside\n"
        (ws / "one-too-many").symlink_to(previous)
        with pytest.raises(WorkspacePathError, match="symbolic links"):
            root.read_bytes("one-too-many")

    def test_symlink_loop_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, _ = tree
        (ws / "x").symlink_to("y")
        (ws / "y").symlink_to("x")
        with pytest.raises(WorkspacePathError, match="symbolic links"):
            root.read_bytes("x")

    def test_missing_file(self, root: WorkspaceRoot) -> None:
        with pytest.raises(FileNotFoundError):
            root.read_bytes("src/nope.py")
        with pytest.raises(FileNotFoundError):
            root.read_bytes("nope/a.py")

    def test_dangling_symlink_is_missing(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "dangling").symlink_to("gone")
        with pytest.raises(FileNotFoundError):
            root.read_bytes("dangling")

    def test_directory_is_not_a_regular_file(self, root: WorkspaceRoot) -> None:
        with pytest.raises(NotRegularFileError, match="not a regular file"):
            root.read_bytes("src")
        with pytest.raises(NotRegularFileError):
            root.read_bytes(".")

    def test_file_as_directory(self, root: WorkspaceRoot) -> None:
        with pytest.raises(NotADirectoryError):
            root.read_bytes("src/a.py/x")

    def test_fifo_does_not_block(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        os.mkfifo(tree[0] / "pipe.py")
        (tree[0] / "pipelink").symlink_to("pipe.py")
        for rel in ("pipe.py", "pipelink"):
            exc = _no_block(lambda rel=rel: root.read_bytes(rel))
            assert isinstance(exc, NotRegularFileError)

    def test_socket_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        path = tree[0] / "s.sock"
        if len(os.fsencode(path)) > 100:
            pytest.skip("socket path too long")
        with socket.socket(socket.AF_UNIX) as sock:
            sock.bind(str(path))
            exc = _no_block(lambda: root.read_bytes("s.sock"))
        assert isinstance(exc, NotRegularFileError)

    def test_size_cap(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "big.txt").write_bytes(b"x" * 101)
        assert len(root.read_bytes("big.txt", max_bytes=101)) == 101
        with pytest.raises(FileTooLargeError, match="file too large"):
            root.read_bytes("big.txt", max_bytes=100)

    def test_nul_byte_refused(self, root: WorkspaceRoot) -> None:
        with pytest.raises(WorkspacePathError):
            root.read_bytes("src/a.py\x00")

    def test_read_text_errors(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "bin").write_bytes(b"\xff\xfe ok")
        with pytest.raises(UnicodeDecodeError):
            root.read_text("bin")
        assert root.read_text("bin", errors="replace").endswith(" ok")


class TestStatAndResolve:
    def test_stat_follows_inside_symlinks(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "srclink").symlink_to("src")
        assert root.is_dir("srclink")
        assert root.is_file("srclink/a.py")
        assert root.is_dir(".")
        assert not root.is_file("src")

    def test_stat_refuses_outside(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "outlink").symlink_to("../out")
        with pytest.raises(PathLeavesWorkspaceError):
            root.stat("outlink")
        assert not root.is_dir("outlink")
        assert not root.is_file("outlink/secret.txt")

    def test_resolve(self, root: WorkspaceRoot) -> None:
        assert root.resolve(".") == "."
        assert root.resolve("") == "."
        assert root.resolve("src/sub/..") == "src"
        with pytest.raises(FileNotFoundError):
            root.resolve("missing")


# ---------------------------------------------------------------------------
# Writes
# ---------------------------------------------------------------------------


class TestWrite:
    def test_create_and_overwrite(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        root.write_text("new.txt", "one")
        assert (tree[0] / "new.txt").read_text() == "one"
        root.write_text("new.txt", "two!")
        assert (tree[0] / "new.txt").read_text() == "two!"

    def test_overwrite_keeps_mode(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        script = tree[0] / "run.sh"
        script.write_text("#!/bin/sh\n")
        script.chmod(0o755)
        root.write_text("run.sh", "#!/bin/sh\necho hi\n")
        assert script.stat().st_mode & 0o777 == 0o755

    def test_make_parents(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        root.write_text("a/b/c.txt", "x", make_parents=True)
        assert (tree[0] / "a" / "b" / "c.txt").read_text() == "x"
        with pytest.raises(FileNotFoundError):
            root.write_text("d/e.txt", "x")

    def test_write_through_inside_symlink(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "link.py").symlink_to("src/a.py")
        root.write_text("link.py", "changed")
        assert (tree[0] / "src" / "a.py").read_text() == "changed"
        assert (tree[0] / "link.py").is_symlink()

    def test_write_through_symlink_outside_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, out = tree
        (ws / "evil.txt").symlink_to("../out/secret.txt")
        (ws / "evildir").symlink_to(out)
        (ws / "dangling-out").symlink_to("../out/new.txt")
        for rel in ("evil.txt", "evildir/secret.txt", "evildir/created.txt", "dangling-out"):
            with pytest.raises(PathLeavesWorkspaceError):
                root.write_text(rel, "pwned", make_parents=True)
        assert (out / "secret.txt").read_text() == OUTSIDE_TEXT
        assert sorted(p.name for p in out.iterdir()) == ["secret.txt"]

    def test_make_parents_through_symlink_outside_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "evildir").symlink_to("../out")
        with pytest.raises(PathLeavesWorkspaceError):
            root.write_text("evildir/x/y.txt", "pwned", make_parents=True)
        assert not (tree[1] / "x").exists()

    def test_write_to_fifo_does_not_block(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        os.mkfifo(tree[0] / "pipe")
        exc = _no_block(lambda: root.write_text("pipe", "x"))
        assert isinstance(exc, NotRegularFileError)

    def test_write_to_directory_refused(self, root: WorkspaceRoot) -> None:
        with pytest.raises(NotRegularFileError):
            root.write_text("src", "x")
        with pytest.raises(NotRegularFileError):
            root.write_text(".", "x")

    def test_replace_does_not_follow(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, out = tree
        (ws / "solution.py").symlink_to("../out/secret.txt")
        os.mkfifo(ws / "pipe")
        root.replace_bytes("solution.py", b"harness")
        _no_block(lambda: root.replace_bytes("pipe", b"data"))
        assert (ws / "solution.py").read_bytes() == b"harness"
        assert not (ws / "solution.py").is_symlink()
        assert (ws / "pipe").read_bytes() == b"data"
        assert (out / "secret.txt").read_text() == OUTSIDE_TEXT
        assert not [p.name for p in ws.iterdir() if p.name.endswith(".tmp")]

    def test_replace_through_symlinked_parent_outside_refused(
        self, root: WorkspaceRoot, tree: tuple[Path, Path]
    ) -> None:
        (tree[0] / "evildir").symlink_to("../out")
        with pytest.raises(PathLeavesWorkspaceError):
            root.replace_bytes("evildir/secret.txt", b"pwned")
        assert (tree[1] / "secret.txt").read_text() == OUTSIDE_TEXT


# ---------------------------------------------------------------------------
# Blocked names
# ---------------------------------------------------------------------------


class TestBlockedNames:
    @pytest.mark.parametrize("rel", [".git/config", ".git", ".GIT/config", "sub/.git/x", "src/../.git/config"])
    def test_blocked(self, tree: tuple[Path, Path], rel: str) -> None:
        (tree[0] / ".git").mkdir()
        (tree[0] / ".git" / "config").write_text("[core]\n")
        with WorkspaceRoot(str(tree[0]), blocked_names=frozenset({".git"})) as blocking:
            with pytest.raises(BlockedPathError, match=r"access to \.git is blocked"):
                blocking.read_bytes(rel)
            with pytest.raises(BlockedPathError):
                blocking.write_text(rel, "x", make_parents=True)

    def test_symlink_into_blocked_refused(self, tree: tuple[Path, Path]) -> None:
        (tree[0] / ".git").mkdir()
        (tree[0] / ".git" / "config").write_text("[core]\n")
        (tree[0] / "innocent").symlink_to(".git")
        (tree[0] / "cfg").symlink_to(".git/config")
        with WorkspaceRoot(str(tree[0]), blocked_names=frozenset({".git"})) as blocking:
            for rel in ("innocent/config", "cfg"):
                with pytest.raises(BlockedPathError):
                    blocking.read_bytes(rel)
            with pytest.raises(BlockedPathError):
                blocking.write_text("cfg", "x")
        assert (tree[0] / ".git" / "config").read_text() == "[core]\n"

    @pytest.mark.parametrize("rel", [".gitignore", ".github/ci.yml", "git/config"])
    def test_similar_names_allowed(self, tree: tuple[Path, Path], rel: str) -> None:
        with WorkspaceRoot(str(tree[0]), blocked_names=frozenset({".git"})) as blocking:
            blocking.write_text(rel, "x", make_parents=True)
            assert blocking.read_text(rel) == "x"


# ---------------------------------------------------------------------------
# The workspace directory itself
# ---------------------------------------------------------------------------


class TestWorkspaceDirectory:
    def test_symlinked_workspace_refused(self, tree: tuple[Path, Path], tmp_path: Path) -> None:
        # The tool user can replace its workspace directory (the tenant
        # directory is group-writable): the workspace itself must not be a symlink.
        link = tmp_path / "swapped"
        link.symlink_to(tree[1])
        with pytest.raises(WorkspacePathError, match="symlink"):
            WorkspaceRoot(str(link))

    def test_trailing_slash_does_not_follow(self, tree: tuple[Path, Path], tmp_path: Path) -> None:
        link = tmp_path / "swapped"
        link.symlink_to(tree[1])
        with pytest.raises(WorkspacePathError):
            WorkspaceRoot(f"{link}/")

    def test_missing_workspace(self, tmp_path: Path) -> None:
        with pytest.raises(FileNotFoundError):
            WorkspaceRoot(str(tmp_path / "missing"))

    def test_closed_after_context(self, tree: tuple[Path, Path]) -> None:
        with WorkspaceRoot(str(tree[0])) as opened:
            fd = opened.fd
        with pytest.raises(OSError):
            os.fstat(fd)


# ---------------------------------------------------------------------------
# Listing and walking
# ---------------------------------------------------------------------------


class TestListAndWalk:
    def test_list_dir(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, _ = tree
        (ws / "src" / "link_in").symlink_to("sub")
        (ws / "src" / "link_out").symlink_to("../../out")
        (ws / "src" / "dangling").symlink_to("gone")
        os.mkfifo(ws / "src" / "pipe")
        entries = {e.name: e for e in root.list_dir("src")}
        assert set(entries) == {"a.py", "sub", "link_in", "link_out", "dangling", "pipe"}
        kinds = {name: (e.is_dir, e.is_symlink) for name, e in entries.items()}
        assert kinds == {
            "a.py": (False, False),
            "sub": (True, False),
            "link_in": (True, True),
            "link_out": (False, True),
            "dangling": (False, True),
            "pipe": (False, False),
        }

    def test_list_dir_through_inside_symlink(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "srclink").symlink_to("src")
        assert sorted(e.name for e in root.list_dir("srclink")) == ["a.py", "sub"]

    def test_list_dir_outside_refused(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        (tree[0] / "outlink").symlink_to("../out")
        with pytest.raises(PathLeavesWorkspaceError):
            root.list_dir("outlink")
        with pytest.raises(NotADirectoryError):
            root.list_dir("src/a.py")

    def test_walk_never_follows_symlinks(self, root: WorkspaceRoot, tree: tuple[Path, Path]) -> None:
        ws, _ = tree
        (ws / "outlink").symlink_to("../out")
        (ws / "loop").symlink_to(".")
        seen = {}
        for dirpath, dirnames, filenames, dir_fd in root.walk():
            assert isinstance(dir_fd, int)
            seen[dirpath] = (sorted(dirnames), sorted(filenames))
        assert set(seen) == {".", "src", "src/sub"}
        assert "secret.txt" not in str(seen)

    def test_walk_below_a_subdirectory(self, root: WorkspaceRoot) -> None:
        assert [d for d, _, _, _ in root.walk("src")] == ["src", "src/sub"]


# ---------------------------------------------------------------------------
# Swap races: impossible by construction (every step is relative to a held descriptor)
# ---------------------------------------------------------------------------


class TestSwapRace:
    def test_directory_swapped_for_symlink_after_it_was_opened(
        self, root: WorkspaceRoot, tree: tuple[Path, Path], monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The agent replaces src with a symlink to the outside right after the walk opened src.

        The next component is opened relative to the descriptor of the real src,
        so the swap cannot redirect the read.
        """
        ws, _ = tree
        (tree[1] / "a.py").write_text(OUTSIDE_TEXT)
        real_open = os.open
        swapped: list[bool] = []

        def racing_open(path: str, flags: int, mode: int = 0o777, *, dir_fd: int | None = None) -> int:
            fd = real_open(path, flags, mode, dir_fd=dir_fd)
            if path == "src" and not swapped:
                os.rename(ws / "src", ws / "src-real")
                (ws / "src").symlink_to("../out")
                swapped.append(True)
            return fd

        monkeypatch.setattr(workspace_fs.os, "open", racing_open)
        assert root.read_text("src/a.py") == "inside\n"
        assert swapped

    def test_final_component_swapped_for_symlink(
        self, root: WorkspaceRoot, tree: tuple[Path, Path], monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """A file swapped for a symlink after it was looked at is opened with O_NOFOLLOW (ELOOP) and resolved again."""
        ws, _ = tree
        real_open = os.open
        swapped: list[bool] = []

        def racing_open(path: str, flags: int, mode: int = 0o777, *, dir_fd: int | None = None) -> int:
            if path == "a.py" and not swapped:
                os.unlink(ws / "src" / "a.py")
                (ws / "src" / "a.py").symlink_to("../../out/secret.txt")
                swapped.append(True)
            return real_open(path, flags, mode, dir_fd=dir_fd)

        monkeypatch.setattr(workspace_fs.os, "open", racing_open)
        with pytest.raises(PathLeavesWorkspaceError):
            root.read_bytes("src/a.py")
        assert swapped

    def test_every_open_is_relative_and_no_follow(
        self, root: WorkspaceRoot, tree: tuple[Path, Path], monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Construction: after the root, every open is relative to a directory descriptor and never follows a symlink."""
        (tree[0] / "srclink").symlink_to("src")
        real_open = os.open
        calls: list[tuple[str, int, int | None]] = []

        def recording_open(path: str, flags: int, mode: int = 0o777, *, dir_fd: int | None = None) -> int:
            calls.append((path, flags, dir_fd))
            return real_open(path, flags, mode, dir_fd=dir_fd)

        monkeypatch.setattr(workspace_fs.os, "open", recording_open)
        root.read_text("srclink/sub/../a.py")
        root.write_text("src/new/x.txt", "x", make_parents=True)
        root.replace_bytes("src/new/y.txt", b"y")
        root.list_dir("srclink")
        assert calls
        for path, flags, dir_fd in calls:
            assert dir_fd is not None, path
            assert "/" not in path, path
            assert flags & os.O_NOFOLLOW, path

        # os.fwalk opens each directory relative to its parent's descriptor
        # after an lstat and descends only when the opened directory is the
        # one it saw (samestat), so a swapped-in symlink is never walked.
        calls.clear()
        list(root.walk("src"))
        assert calls
        for path, _flags, dir_fd in calls:
            assert dir_fd is not None, path
            assert "/" not in path, path


# ---------------------------------------------------------------------------
# workspace_relative (the tools accept absolute paths that name a place inside the workspace)
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("path", "expected"),
    [
        ("src/a.py", "src/a.py"),
        ("/ws/src/a.py", "src/a.py"),
        ("/ws", "."),
        ("/ws/", "."),
        ("/ws2/a.py", "/ws2/a.py"),
        ("/etc/passwd", "/etc/passwd"),
        ("/ws/../etc/passwd", "/ws/../etc/passwd"),
        ("", ""),
    ],
)
def test_workspace_relative(path: str, expected: str) -> None:
    assert workspace_relative("/ws", path) == expected
