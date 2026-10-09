"""SPEC "What counts as a link": reference definitions and their uses, undefined references."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, assert_problems_any_of, column_of, make_tree, md, run_cli

if TYPE_CHECKING:
    from pathlib import Path


def test_definition_with_a_missing_target_is_reported_at_the_definition(tmp_path: Path) -> None:
    make_tree(tmp_path, {"page.md": "Intro.\n\n[unused]: gone.md\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 3, 1, "missing-file", "gone.md")])


def test_used_definition_with_a_missing_target_is_reported_once(tmp_path: Path) -> None:
    use = "Read [the guide][guide] first."
    make_tree(tmp_path, {"page.md": f"{use}\n\n[guide]: gone.md\n"})

    run = run_cli([], tmp_path)

    # SPEC: reported at the definition ("for a problem found at a definition"); at the use is also accepted.
    assert_problems_any_of(
        run,
        [
            [("page.md", 3, 1, "missing-file", "gone.md")],
            [("page.md", 1, column_of(use, "[the guide]"), "missing-file", "gone.md")],
        ],
    )


def test_definition_titles_and_angle_brackets_are_not_part_of_the_target(tmp_path: Path) -> None:
    uses = "[A][a] [B][b] [C][c] [D][d]"
    page = md(
        """
        [a]: exists.md "A title"
        [b]: exists.md 'A title'
        [c]: <my notes.md>
        [d]: gone.md "A title"
        """
    )
    make_tree(tmp_path, {"page.md": f"{uses}\n\n{page}", "exists.md": "# Exists\n", "my notes.md": "# Notes\n"})

    run = run_cli([], tmp_path)

    assert_problems_any_of(
        run,
        [
            [("page.md", 6, 1, "missing-file", "gone.md")],
            [("page.md", 1, column_of(uses, "[D]"), "missing-file", "gone.md")],
        ],
    )


def test_reference_ids_match_case_insensitively_and_with_whitespace_collapsed(tmp_path: Path) -> None:
    page = md(
        """
        Read [this][setup guide], [SETUP GUIDE][] or [that][sEtUp   GuIdE].
        Also [the API][api  reference] and [Api Reference][].

        [Setup Guide]: setup.md
        [api    reference]: api.md
        """
    )
    make_tree(tmp_path, {"page.md": page, "setup.md": "# Setup\n", "api.md": "# API\n"})

    assert_clean(run_cli([], tmp_path))


def test_full_and_collapsed_references_without_definition_are_undefined(tmp_path: Path) -> None:
    full = "Read [the docs][missing-id] now."
    collapsed = "Also [Other Id][] here."
    make_tree(tmp_path, {"page.md": f"{full}\n{collapsed}\n"})

    run = run_cli([], tmp_path)

    assert_problems(
        run,
        [
            ("page.md", 1, column_of(full, "[the docs]"), "undefined-reference", "missing-id"),
            ("page.md", 2, column_of(collapsed, "[Other Id]"), "undefined-reference", "Other Id"),
        ],
    )


def test_bracketed_text_without_definition_is_plain_text(tmp_path: Path) -> None:
    page = md(
        """
        - [x] done
        - [ ] open

        See [this] and [that thing], or [TODO].
        """
    )
    make_tree(tmp_path, {"page.md": page})

    assert_clean(run_cli([], tmp_path))


def test_definition_targets_get_the_same_checks_as_inline_links(tmp_path: Path) -> None:
    uses = "Jump [up][ok], [down][bad] or [away][other]."
    page = md(
        """
        # Top

        [ok]: #top
        [bad]: #bottom
        [other]: other.md#nope
        """
    )
    make_tree(tmp_path, {"page.md": f"{uses}\n\n{page}", "other.md": "# Other\n"})

    run = run_cli([], tmp_path)

    assert_problems_any_of(
        run,
        [
            [
                ("page.md", 6, 1, "missing-anchor", "#bottom"),
                ("page.md", 7, 1, "missing-anchor", "other.md#nope"),
            ],
            [
                ("page.md", 1, column_of(uses, "[down]"), "missing-anchor", "#bottom"),
                ("page.md", 1, column_of(uses, "[away]"), "missing-anchor", "other.md#nope"),
            ],
        ],
    )
