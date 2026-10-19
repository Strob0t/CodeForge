"""SPEC "Exit codes": 0 without problems, 1 with problems, 2 for usage errors (message on stderr)."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_usage_error, make_tree, run_cli

if TYPE_CHECKING:
    from pathlib import Path


def test_exit_code_is_0_without_problems_and_1_with_problems_in_both_formats(tmp_path: Path) -> None:
    make_tree(tmp_path, {"clean/a.md": "# A\n", "broken/b.md": "[x](gone.md)\n"})

    for output_format in ("text", "json"):
        clean = run_cli(["clean", "--format", output_format], tmp_path)
        broken = run_cli(["broken", "--format", output_format], tmp_path)
        assert clean.exit_code == 0, clean.describe()
        assert broken.exit_code == 1, broken.describe()


def test_unknown_option_is_a_usage_error(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "# A\n"})

    assert_usage_error(run_cli(["--no-such-option"], tmp_path))


def test_path_that_does_not_exist_is_a_usage_error(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "# A\n"})

    assert_usage_error(run_cli(["no-such-dir"], tmp_path))


def test_config_file_that_cannot_be_read_is_a_usage_error(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "# A\n"})

    assert_usage_error(run_cli(["--config", "missing.toml"], tmp_path))


def test_config_file_that_cannot_be_parsed_is_a_usage_error(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "# A\n", "broken.toml": "exclude = [unclosed\n"})

    assert_usage_error(run_cli(["--config", "broken.toml"], tmp_path))


def test_implicit_config_file_that_cannot_be_parsed_is_a_usage_error(tmp_path: Path) -> None:
    make_tree(tmp_path, {"a.md": "# A\n", ".mdlinkcheck.toml": "check_external = = true\n"})

    assert_usage_error(run_cli([], tmp_path))
