"""SPEC "Command line" / --config: the TOML configuration file and its precedence."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_clean, assert_problems, make_tree, run_cli

if TYPE_CHECKING:
    from pathlib import Path

    from conftest import LocalServer

BROKEN = "[x](gone.md)\n"


def test_implicit_config_file_in_the_working_directory_is_used(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {".mdlinkcheck.toml": 'exclude = ["drafts/*"]\n', "drafts/wip.md": BROKEN, "a.md": "# A\n", "b.md": "# B\n"},
    )

    run = run_cli([], tmp_path)

    assert_clean(run)
    assert run.summary == "No problems found in 2 files", run.describe()


def test_config_option_names_the_file_and_the_implicit_file_is_not_read(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {
            ".mdlinkcheck.toml": 'exclude = ["b.md"]\n',
            "conf/custom.toml": 'exclude = ["a.md"]\n',
            "a.md": BROKEN,
            "b.md": BROKEN,
        },
    )

    run = run_cli(["--config", "conf/custom.toml"], tmp_path)

    assert_problems(run, [("b.md", 1, 1, "missing-file", "gone.md")])


def test_config_excludes_are_added_to_command_line_excludes(tmp_path: Path) -> None:
    make_tree(
        tmp_path,
        {".mdlinkcheck.toml": 'exclude = ["a.md"]\n', "a.md": BROKEN, "b.md": BROKEN, "c.md": BROKEN},
    )

    run = run_cli(["--exclude", "b.md"], tmp_path)

    assert_problems(run, [("c.md", 1, 1, "missing-file", "gone.md")])


def test_config_check_external_and_timeout_are_used(tmp_path: Path, http_server: LocalServer) -> None:
    url = http_server.url("/slow?seconds=3")
    config = "check_external = true\ntimeout = 0.5\n"
    make_tree(tmp_path, {".mdlinkcheck.toml": config, "page.md": f"[x]({url})\n"})

    run = run_cli([], tmp_path)

    assert_problems(run, [("page.md", 1, 1, "external-error", url)])


def test_command_line_options_override_the_config_file(tmp_path: Path, http_server: LocalServer) -> None:
    url = http_server.url("/slow?seconds=1.5")
    config = "check_external = false\ntimeout = 0.5\n"
    make_tree(tmp_path, {".mdlinkcheck.toml": config, "page.md": f"[x]({url})\n"})

    run = run_cli(["--check-external", "--timeout", "10"], tmp_path)

    assert_clean(run)
    assert http_server.requests, "--check-external on the command line must turn the check on"
