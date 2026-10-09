"""SPEC "Checks" / external-error: --check-external against a local HTTP server (no internet needed)."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, column_of, make_tree, run_cli

if TYPE_CHECKING:
    from pathlib import Path

    from conftest import LocalServer


def test_reachable_url_is_checked_with_a_single_head_request(tmp_path: Path, http_server: LocalServer) -> None:
    make_tree(tmp_path, {"page.md": f"[ok]({http_server.url('/ok')})\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_clean(run)
    assert http_server.requests == [("HEAD", "/ok")]


def test_status_400_and_above_is_an_external_error(tmp_path: Path, http_server: LocalServer) -> None:
    urls = [http_server.url(f"/status/{status}") for status in (400, 404, 500)]
    line = f"[a]({urls[0]}) [b]({urls[1]}) [c]({urls[2]}) [d]({http_server.url('/status/204')})"
    make_tree(tmp_path, {"page.md": f"{line}\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_problems(
        run,
        [
            ("page.md", 1, 1, "external-error", urls[0]),
            ("page.md", 1, column_of(line, "[b]"), "external-error", urls[1]),
            ("page.md", 1, column_of(line, "[c]"), "external-error", urls[2]),
        ],
    )


def test_head_answered_with_405_is_retried_with_get(tmp_path: Path, http_server: LocalServer) -> None:
    make_tree(tmp_path, {"page.md": f"[a]({http_server.url('/head-not-allowed')})\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_clean(run)
    assert http_server.requests == [("HEAD", "/head-not-allowed"), ("GET", "/head-not-allowed")]


def test_redirect_is_followed(tmp_path: Path, http_server: LocalServer) -> None:
    make_tree(tmp_path, {"page.md": f"[a]({http_server.url('/redirect')})\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_clean(run)
    assert http_server.requests[0] == ("HEAD", "/redirect")
    assert [path for _, path in http_server.requests[1:]] == ["/ok"]


def test_redirect_to_a_missing_page_is_an_external_error(tmp_path: Path, http_server: LocalServer) -> None:
    url = http_server.url("/redirect-to-missing")
    make_tree(tmp_path, {"page.md": f"See [a]({url}).\n"})

    run = run_cli(["--check-external"], tmp_path)

    assert_problems(run, [("page.md", 1, 5, "external-error", url)])


def test_request_that_times_out_is_an_external_error(tmp_path: Path, http_server: LocalServer) -> None:
    url = http_server.url("/slow?seconds=3")
    make_tree(tmp_path, {"page.md": f"[slow]({url})\n"})

    run = run_cli(["--check-external", "--timeout", "0.5"], tmp_path)

    assert_problems(run, [("page.md", 1, 1, "external-error", url)])


def test_refused_connection_is_an_external_error(tmp_path: Path, closed_port_url: str) -> None:
    make_tree(tmp_path, {"page.md": f"[down]({closed_port_url})\n"})

    run = run_cli(["--check-external", "--timeout", "2"], tmp_path)

    assert_problems(run, [("page.md", 1, 1, "external-error", closed_port_url)])


def test_external_links_count_as_checked_only_with_check_external(tmp_path: Path, http_server: LocalServer) -> None:
    url = http_server.url("/ok")
    make_tree(tmp_path, {"page.md": f"[a]({url}) <{url}> [b](local.txt)\n", "local.txt": "text\n"})

    without = run_cli(["--format", "json"], tmp_path)
    with_check = run_cli(["--format", "json", "--check-external"], tmp_path)

    assert without.json() == {"files_checked": 1, "links_checked": 1, "problems": []}, without.describe()
    assert with_check.json() == {"files_checked": 1, "links_checked": 3, "problems": []}, with_check.describe()
