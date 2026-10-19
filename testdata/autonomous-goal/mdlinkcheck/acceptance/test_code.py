"""SPEC "What counts as a link": nothing inside fenced code blocks or inline code spans is a link."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_problems, column_of, make_tree, md, run_cli

if TYPE_CHECKING:
    from pathlib import Path

    from conftest import LocalServer


def test_links_in_a_backtick_fence_are_ignored_and_links_after_it_are_checked(tmp_path: Path) -> None:
    page = md(
        """
        # Code

        ```markdown
        [inside](gone-inside.md)
        ![image](gone.png)
        [ref]: gone-ref.md
        ```

        After [the fence](gone-after.md).
        """
    )
    make_tree(tmp_path, {"page.md": page})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 9, 7, "missing-file", "gone-after.md")])


def test_links_in_a_tilde_fence_are_ignored_even_across_a_backtick_line(tmp_path: Path) -> None:
    page = md(
        """
        ~~~
        [a](gone-1.md)
        ```
        [b](gone-2.md)
        ~~~
        [c](gone-3.md)
        """
    )
    make_tree(tmp_path, {"page.md": page})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 6, 1, "missing-file", "gone-3.md")])


def test_links_in_inline_code_are_ignored_and_the_rest_of_the_line_is_checked(
    tmp_path: Path, http_server: LocalServer
) -> None:
    line = f"Write `[text](target.md)` or `<{http_server.url('/status/404')}>`, then read [the guide](gone.md)."
    make_tree(tmp_path, {"page.md": f"{line}\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_problems(run, [("page.md", 1, column_of(line, "[the guide]"), "missing-file", "gone.md")])
    assert http_server.requests == []


def test_reference_definition_inside_a_fence_does_not_count(tmp_path: Path) -> None:
    page = md(
        """
        Read [the guide][guide].

        ```
        [guide]: guide.md
        ```
        """
    )
    make_tree(tmp_path, {"page.md": page, "guide.md": "# Guide\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 1, 6, "undefined-reference", "guide")])
