"""SPEC "Checks" / missing-anchor: heading slugs (GitHub rules), HTML name/id, fragments into other files."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_problems, column_of, make_tree, md, run_cli

if TYPE_CHECKING:
    from pathlib import Path


def test_same_file_fragment_names_atx_headings_of_every_level(tmp_path: Path) -> None:
    headings = "# One\n## Two\n### Three\n#### Four\n##### Five\n###### Six\n"
    links = "[1](#one) [2](#two) [3](#three) [4](#four) [5](#five) [6](#six) [7](#seven)"
    make_tree(tmp_path, {"page.md": f"{headings}\n{links}\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 8, column_of(links, "[7]"), "missing-anchor", "#seven")])


def test_setext_headings_are_anchors(tmp_path: Path) -> None:
    page = md(
        """
        Main Title
        ==========

        Sub Title
        ---------

        [a](#main-title) [b](#sub-title) [c](#other-title)
        """
    )
    make_tree(tmp_path, {"page.md": page})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 7, 34, "missing-anchor", "#other-title")])


def test_duplicate_headings_get_numbered_suffixes(tmp_path: Path) -> None:
    links = "[a](#example) [b](#example-1) [c](#example-2) [d](#example-3)"
    page = f"## Example\n\nText.\n\n## Example\n\n## Example\n\n{links}\n"
    make_tree(tmp_path, {"page.md": page})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 9, column_of(links, "[d]"), "missing-anchor", "#example-3")])


def test_punctuation_is_removed_from_slugs(tmp_path: Path) -> None:
    headings = "## What's new? (v2.0)\n## Step 1: Install\n## Hello, World!\n## pre-commit hooks\n"
    links = "[a](#whats-new-v20) [b](#step-1-install) [c](#hello-world) [d](#pre-commit-hooks) [e](#whats-new-v2.0)"
    make_tree(tmp_path, {"page.md": f"{headings}\n{links}\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 6, column_of(links, "[e]"), "missing-anchor", "#whats-new-v2.0")])


def test_each_space_becomes_a_hyphen_after_trimming(tmp_path: Path) -> None:
    headings = "## Foo & Bar\n##   Padded heading   \n"
    links = "[a](#foo--bar) [b](#padded-heading) [c](#foo-bar)"
    make_tree(tmp_path, {"page.md": f"{headings}\n{links}\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 4, column_of(links, "[c]"), "missing-anchor", "#foo-bar")])


def test_emphasis_and_code_markers_are_stripped_from_slugs(tmp_path: Path) -> None:
    headings = "## The *quick* **brown** fox\n## The `run` command\n## _Italic_ heading\n"
    links = "[a](#the-quick-brown-fox) [b](#the-run-command) [c](#italic-heading) [d](#the-command)"
    make_tree(tmp_path, {"page.md": f"{headings}\n{links}\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 5, column_of(links, "[d]"), "missing-anchor", "#the-command")])


def test_unicode_letters_are_kept_in_slugs(tmp_path: Path) -> None:
    links = "[a](#über-uns) [b](#ber-uns)"
    make_tree(tmp_path, {"page.md": f"## Über uns\n\n{links}\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 3, column_of(links, "[b]"), "missing-anchor", "#ber-uns")])


def test_fragment_comparison_is_exact_after_percent_decoding(tmp_path: Path) -> None:
    links = "[a](#install-guide) [b](#install%2Dguide) [c](#Install-Guide)"
    make_tree(tmp_path, {"page.md": f"# Install Guide\n\n{links}\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 3, column_of(links, "[c]"), "missing-anchor", "#Install-Guide")])


def test_html_name_and_id_attributes_are_anchors(tmp_path: Path) -> None:
    links = "[a](#legacy-anchor) [b](#custom-id) [c](#other-id)"
    page = f'<a name="legacy-anchor"></a>\n<span id="custom-id">Note</span>\n\n{links}\n'
    make_tree(tmp_path, {"page.md": page})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 4, column_of(links, "[c]"), "missing-anchor", "#other-id")])


def test_fragment_into_another_markdown_file_is_checked(tmp_path: Path) -> None:
    line = "[a](api.md#usage) [b](api.md#install) [c](notes.markdown#notes) [d](notes.markdown#todo)"
    query = "[e](api.md?plain=1#usage) [f](api.md?plain=1#nope)"
    make_tree(
        tmp_path,
        {"page.md": f"{line}\n{query}\n", "api.md": "# API\n\n## Usage\n", "notes.markdown": "# Notes\n"},
    )

    run = run_cli(["page.md"], tmp_path)

    assert_problems(
        run,
        [
            ("page.md", 1, column_of(line, "[b]"), "missing-anchor", "api.md#install"),
            ("page.md", 1, column_of(line, "[d]"), "missing-anchor", "notes.markdown#todo"),
            ("page.md", 2, column_of(query, "[f]"), "missing-anchor", "api.md?plain=1#nope"),
        ],
    )


def test_fragment_into_a_markdown_file_outside_the_checked_paths_is_checked(tmp_path: Path) -> None:
    line = "[a](../other.md#present) [b](../other.md#absent)"
    make_tree(tmp_path, {"docs/page.md": f"{line}\n", "other.md": "# Present\n"})

    run = run_cli(["docs"], tmp_path)

    assert_problems(run, [("docs/page.md", 1, column_of(line, "[b]"), "missing-anchor", "../other.md#absent")])


def test_fragment_into_a_non_markdown_file_is_not_checked_but_the_file_is(tmp_path: Path) -> None:
    line = "[a](tool.py#L10) [b](data.json#/items/0) [c](gone.py#L1) [d](gone.md#intro)"
    make_tree(tmp_path, {"page.md": f"{line}\n", "tool.py": "print('hi')\n", "data.json": "{}\n"})

    run = run_cli([], tmp_path)

    assert_problems(
        run,
        [
            ("page.md", 1, column_of(line, "[c]"), "missing-file", "gone.py#L1"),
            ("page.md", 1, column_of(line, "[d]"), "missing-file", "gone.md#intro"),
        ],
    )


def test_heading_inside_fenced_code_is_not_an_anchor(tmp_path: Path) -> None:
    page = md(
        """
        # Real

        ```bash
        # Install
        ```

        [a](#real) [b](#install)
        """
    )
    make_tree(tmp_path, {"page.md": page})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 7, 12, "missing-anchor", "#install")])
