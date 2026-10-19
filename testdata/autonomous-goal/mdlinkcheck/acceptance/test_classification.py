"""SPEC "Classification of a target": ignored schemes, external links, percent-decoding, queries."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, column_of, make_tree, md, run_cli

if TYPE_CHECKING:
    from pathlib import Path

    from conftest import LocalServer


def test_mailto_and_other_schemes_are_ignored(tmp_path: Path) -> None:
    page = md(
        """
        [mail](mailto:someone@example.com) and <mailto:someone@example.com>
        [ftp](ftp://example.invalid/file.txt) [phone](tel:+15550100)
        [chat](irc://irc.example.invalid/channel) [news](news:comp.lang.python)
        """
    )
    make_tree(tmp_path, {"page.md": page})

    assert_clean(run_cli(["--check-external", "--timeout", "1"], tmp_path))


def test_http_links_are_not_requested_without_check_external(tmp_path: Path, http_server: LocalServer) -> None:
    page = f"[a]({http_server.url('/status/404')}) and <{http_server.url('/status/500')}>\n"
    make_tree(tmp_path, {"page.md": page})

    run = run_cli([], tmp_path)

    assert_clean(run)
    assert http_server.requests == []


def test_percent_encoding_in_the_path_is_decoded(tmp_path: Path) -> None:
    line = "[ok](my%20notes.md) and [broken](no%20such%20file.md)"
    make_tree(tmp_path, {"page.md": f"{line}\n", "my notes.md": "# Notes\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 1, column_of(line, "[broken]"), "missing-file", "no%20such%20file.md")])


def test_query_is_ignored_when_resolving_the_path(tmp_path: Path) -> None:
    line = "[plain](exists.md?plain=1) and [broken](gone.md?ref=main)"
    make_tree(tmp_path, {"page.md": f"{line}\n", "exists.md": "# Exists\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 1, column_of(line, "[broken]"), "missing-file", "gone.md?ref=main")])
