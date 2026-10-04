"""SPEC "Output" / Text: problem lines, positions, sorting, relative POSIX paths and the summary line."""

from __future__ import annotations

import re
from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, make_tree, md, run_cli

if TYPE_CHECKING:
    from pathlib import Path


def test_problem_line_has_path_line_column_kind_target_and_message(tmp_path: Path) -> None:
    make_tree(tmp_path, {"docs/guide.md": "# Guide\n\nSee [setup](../setup.md).\n", "docs/api.md": "# API\n"})

    run = run_cli([], tmp_path)

    assert run.exit_code == 1, run.describe()
    assert len(run.lines) == 2, run.describe()
    assert re.fullmatch(r"docs/guide\.md:3:5: missing-file: \.\./setup\.md \(.+\)", run.lines[0]), run.describe()
    assert run.summary == "1 problem in 2 files", run.describe()


def test_problems_are_sorted_by_path_then_line_then_column(tmp_path: Path) -> None:
    # Kinds alternate within b.md, so checking one kind after the other does not come out sorted.
    filler = "text\n" * 7
    make_tree(
        tmp_path,
        {
            "b.md": f"# B\n[p](g1.md) and [u][nope]\n{filler}[v][nope2] and [r](g3.md)\n",
            "c/d.md": "[s](g4.md)\n",
            "a.md": md(
                """
                # A
                [t](g5.md)
                """
            ),
        },
    )

    run = run_cli([], tmp_path)

    assert_problems(
        run,
        [
            ("a.md", 2, 1, "missing-file", "g5.md"),
            ("b.md", 2, 1, "missing-file", "g1.md"),
            ("b.md", 2, 16, "undefined-reference", "nope"),
            ("b.md", 10, 1, "undefined-reference", "nope2"),
            ("b.md", 10, 16, "missing-file", "g3.md"),
            ("c/d.md", 1, 1, "missing-file", "g4.md"),
        ],
    )


def test_summary_counts_problems_and_files_in_the_plural(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "[x](gone.md)\n", "b.md": "[y](gone.md)\n", "c.md": "# Clean\n"})

    run = run_cli([], tmp_path)

    assert run.summary == "2 problems in 3 files", run.describe()


def test_summary_says_problem_in_the_singular_for_one_problem(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "[x](gone.md)\n", "b.md": "# Clean\n"})

    run = run_cli([], tmp_path)

    assert run.summary == "1 problem in 2 files", run.describe()


def test_without_problems_the_only_line_is_the_no_problems_line(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "# A\n", "b.md": "[a](a.md)\n", "c/d.md": "[b](../b.md)\n", "empty/": ""})

    run = run_cli([], tmp_path)
    empty = run_cli(["empty"], tmp_path)

    assert_clean(run)
    assert run.summary == "No problems found in 3 files", run.describe()
    assert_clean(empty)
    assert empty.summary == "No problems found in 0 files", empty.describe()


def test_paths_are_relative_to_the_working_directory_for_an_absolute_path_argument(tmp_path: Path) -> None:
    make_tree(tmp_path, {"docs/sub/page.md": "[x](gone.md)\n"})

    run = run_cli([str(tmp_path / "docs")], tmp_path)

    assert_problems(run, [("docs/sub/page.md", 1, 1, "missing-file", "gone.md")])
