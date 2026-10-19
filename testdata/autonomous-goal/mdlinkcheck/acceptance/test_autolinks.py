"""SPEC "What counts as a link": autolinks ``<https://...>``, and what is not a link."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, column_of, make_tree, md, run_cli

if TYPE_CHECKING:
    from pathlib import Path

    from conftest import LocalServer


def test_autolink_is_checked_and_reported_at_its_angle_bracket(tmp_path: Path, http_server: LocalServer) -> None:
    ok = http_server.url("/ok")
    broken = http_server.url("/status/404")
    line = f"Docs: <{ok}> and <{broken}>."
    make_tree(tmp_path, {"page.md": f"{line}\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_problems(run, [("page.md", 1, column_of(line, f"<{broken}"), "external-error", broken)])
    assert ("HEAD", "/ok") in http_server.requests


def test_html_tags_are_not_autolinks(tmp_path: Path, http_server: LocalServer) -> None:
    page = md(
        """
        <a name="top"></a>
        <span id="note">A note</span><br>
        <div class="box">Text</div>
        """
    )
    make_tree(tmp_path, {"page.md": page})

    run = run_cli(["--check-external"], tmp_path)

    assert_clean(run)
    assert http_server.requests == []


def test_bare_url_without_angle_brackets_is_not_a_link(tmp_path: Path, http_server: LocalServer) -> None:
    make_tree(tmp_path, {"page.md": f"Visit {http_server.url('/status/404')} today.\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_clean(run)
    assert http_server.requests == []
