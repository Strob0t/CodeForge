"""SPEC "Checks" / missing-file: relative paths, paths starting with "/", and directories."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, column_of, make_tree, run_cli

if TYPE_CHECKING:
    from pathlib import Path


def test_relative_path_resolves_against_the_directory_of_the_linking_file(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {
            "README.md": "# Readme\n",
            "docs/guide.md": "[up](../README.md) [sibling](api.md) [deeper](ref/cli.md) [dot](./api.md)\n",
            "docs/api.md": "# API\n",
            "docs/ref/cli.md": "# CLI\n",
        },
    )

    assert_clean(run_cli([], tmp_path))


def test_relative_path_is_not_resolved_against_the_working_directory(tmp_path: Path) -> None:
    make_tree(tmp_path, {"top.md": "# Top\n", "docs/guide.md": "[top](top.md)\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("docs/guide.md", 1, 1, "missing-file", "top.md")])


def test_path_starting_with_slash_resolves_against_the_path_argument(tmp_path: Path) -> None:
    line = "[home](/index.md) [deep](/sub/page.md) [outside](/elsewhere.md)"
    make_tree(
        tmp_path,
        {"site/index.md": "# Index\n", "site/sub/page.md": f"{line}\n", "elsewhere.md": "# Elsewhere\n"},
    )

    run = run_cli(["site"], tmp_path)

    assert_problems(run, [("site/sub/page.md", 1, column_of(line, "[outside]"), "missing-file", "/elsewhere.md")])


def test_path_starting_with_slash_in_a_file_given_directly_resolves_against_its_directory(tmp_path: Path) -> None:
    line = "[sibling](/sibling.md) [parent](/index.md)"
    make_tree(
        tmp_path,
        {"docs/sub/page.md": f"{line}\n", "docs/sub/sibling.md": "# Sibling\n", "docs/index.md": "# Index\n"},
    )

    run = run_cli(["docs/sub/page.md"], tmp_path)

    assert_problems(run, [("docs/sub/page.md", 1, column_of(line, "[parent]"), "missing-file", "/index.md")])


def test_existing_directory_is_fine_and_missing_directory_is_reported(tmp_path: Path) -> None:
    line = "[a](docs) [b](docs/) [c](./) [d](no-such-dir/)"
    make_tree(tmp_path, {"page.md": f"{line}\n", "docs/guide.md": "# Guide\n"})

    run = run_cli(["page.md"], tmp_path)

    assert_problems(run, [("page.md", 1, column_of(line, "[d]"), "missing-file", "no-such-dir/")])


def test_links_into_skipped_directories_still_resolve(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {
            ".github/CONTRIBUTING.md": "# Contributing\n",
            "node_modules/pkg/README.md": "[broken](gone.md)\n",
            "page.md": "[c](.github/CONTRIBUTING.md) [p](node_modules/pkg/README.md)\n",
        },
    )

    run = run_cli([], tmp_path)

    assert_clean(run)
    assert run.summary_file_count() == 1, run.describe()
