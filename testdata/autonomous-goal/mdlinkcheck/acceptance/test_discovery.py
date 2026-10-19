"""SPEC "Command line": PATH arguments, recursive scanning, skipped directories and --exclude."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, make_tree, run_cli

if TYPE_CHECKING:
    from pathlib import Path

BROKEN = "[x](gone.md)\n"


def test_directory_is_scanned_recursively_for_md_and_markdown_files(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {
            "docs/a.md": BROKEN,
            "docs/sub/b.markdown": BROKEN,
            "docs/sub/deep/c.md": BROKEN,
            "docs/notes.txt": BROKEN,
            "docs/page.rst": BROKEN,
            "docs/md": BROKEN,
        },
    )

    run = run_cli(["docs"], tmp_path)

    assert_problems(
        run,
        [
            ("docs/a.md", 1, 1, "missing-file", "gone.md"),
            ("docs/sub/b.markdown", 1, 1, "missing-file", "gone.md"),
            ("docs/sub/deep/c.md", 1, 1, "missing-file", "gone.md"),
        ],
    )
    assert run.summary == "3 problems in 3 files", run.describe()


def test_default_path_is_the_current_directory(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": BROKEN, "sub/b.md": BROKEN})

    run = run_cli([], tmp_path)

    assert_problems(
        run,
        [("a.md", 1, 1, "missing-file", "gone.md"), ("sub/b.md", 1, 1, "missing-file", "gone.md")],
    )


def test_hidden_directories_and_node_modules_are_skipped(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {
            ".hidden/a.md": BROKEN,
            ".git/b.md": BROKEN,
            "node_modules/pkg/c.md": BROKEN,
            "docs/node_modules/d.md": BROKEN,
            "docs/.cache/e.md": BROKEN,
            "docs/ok.md": "# OK\n",
            "page.md": "# Page\n",
        },
    )

    run = run_cli([], tmp_path)

    assert_clean(run)
    assert run.summary == "No problems found in 2 files", run.describe()


def test_several_paths_can_be_given(tmp_path: Path) -> None:
    make_tree(tmp_path, {"one/a.md": BROKEN, "two/b.md": BROKEN, "two/c.md": BROKEN, "three/d.md": BROKEN})

    run = run_cli(["one", "two/b.md"], tmp_path)

    assert_problems(
        run,
        [("one/a.md", 1, 1, "missing-file", "gone.md"), ("two/b.md", 1, 1, "missing-file", "gone.md")],
    )
    assert run.summary == "2 problems in 2 files", run.describe()


def test_exclude_glob_matches_the_path_relative_to_the_path_argument(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {"docs/drafts/wip.md": BROKEN, "docs/drafts/deep/more.md": BROKEN, "docs/guide.md": BROKEN},
    )

    # fnmatch rules: "*" also matches "/", so drafts/deep/more.md is excluded as well.
    run = run_cli(["docs", "--exclude", "drafts/*"], tmp_path)

    assert_problems(run, [("docs/guide.md", 1, 1, "missing-file", "gone.md")])


def test_exclude_can_be_repeated(tmp_path: Path) -> None:
    make_tree(tmp_path, {"docs/a.md": BROKEN, "docs/b.md": BROKEN, "docs/c.md": BROKEN})

    run = run_cli(["docs", "--exclude", "a.md", "--exclude", "b*"], tmp_path)

    assert_problems(run, [("docs/c.md", 1, 1, "missing-file", "gone.md")])


def test_exclude_glob_is_not_matched_against_the_working_directory_path(tmp_path: Path) -> None:
    make_tree(tmp_path, {"docs/a.md": BROKEN})

    run = run_cli(["docs", "--exclude", "docs/*"], tmp_path)

    assert_problems(run, [("docs/a.md", 1, 1, "missing-file", "gone.md")])
