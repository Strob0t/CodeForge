"""SPEC "What counts as a link": inline links and images, titles, angle-bracket targets."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_problems, column_of, make_tree, run_cli

if TYPE_CHECKING:
    from pathlib import Path


def test_inline_link_to_a_missing_file_is_reported_at_its_opening_bracket(tmp_path: Path) -> None:
    line = "Read [the setup](setup.md) and [the missing page](nope.md) first."
    make_tree(tmp_path, {"guide.md": f"# Guide\n\n{line}\n", "setup.md": "# Setup\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("guide.md", 3, column_of(line, "[the missing"), "missing-file", "nope.md")])


def test_image_is_reported_at_its_exclamation_mark(tmp_path: Path) -> None:
    line = "Logo: ![the logo](img/logo.png) and ![gone](img/gone.png)"
    make_tree(tmp_path, {"page.md": f"{line}\n", "img/logo.png": "png"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 1, column_of(line, "![gone"), "missing-file", "img/gone.png")])


def test_titles_in_double_and_single_quotes_are_not_part_of_the_target(tmp_path: Path) -> None:
    first = "See [a](exists.md \"A title\") and [b](exists.md 'Another title')."
    second = "Then [c](gone.md \"Title\") and [d](gone-too.md 'Title')."
    make_tree(tmp_path, {"page.md": f"{first}\n{second}\n", "exists.md": "# Exists\n"})

    run = run_cli([], tmp_path)

    assert_problems(
        run,
        [
            ("page.md", 2, column_of(second, "[c]"), "missing-file", "gone.md"),
            ("page.md", 2, column_of(second, "[d]"), "missing-file", "gone-too.md"),
        ],
    )


def test_target_in_angle_brackets_may_contain_spaces(tmp_path: Path) -> None:
    line = "[ok](<my notes.md>) and [broken](<no such file.md>)"
    make_tree(tmp_path, {"page.md": f"{line}\n", "my notes.md": "# Notes\n"})

    run = run_cli([], tmp_path)

    # SPEC: the target "as written"; with or without the angle brackets are both accepted.
    assert run.exit_code == 1, run.describe()
    [problem] = run.problems
    assert (problem.path, problem.line, problem.column, problem.kind) == (
        "page.md",
        1,
        column_of(line, "[broken"),
        "missing-file",
    )
    assert problem.target in {"no such file.md", "<no such file.md>"}, run.describe()


def test_each_link_on_a_line_is_reported_at_its_own_column(tmp_path: Path) -> None:
    line = "[a](x.md) then [b](y.md), and ![c](z.png)"
    make_tree(tmp_path, {"page.md": f"{line}\n"})

    run = run_cli([], tmp_path)

    assert_problems(
        run,
        [
            ("page.md", 1, 1, "missing-file", "x.md"),
            ("page.md", 1, column_of(line, "[b]"), "missing-file", "y.md"),
            ("page.md", 1, column_of(line, "![c]"), "missing-file", "z.png"),
        ],
    )


def test_linked_image_is_both_an_image_and_a_link(tmp_path: Path) -> None:
    make_tree(tmp_path, {"page.md": "[![build badge](badge.svg)](status.md)\n"})

    run = run_cli([], tmp_path)

    assert_problems(
        run,
        [
            ("page.md", 1, 1, "missing-file", "status.md"),
            ("page.md", 1, 2, "missing-file", "badge.svg"),
        ],
    )
