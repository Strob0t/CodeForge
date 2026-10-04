"""SPEC "Deliverable": the console script and ``python -m mdlinkcheck`` behave the same."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import assert_usage_error, make_tree, run_cli

if TYPE_CHECKING:
    from pathlib import Path


def test_console_script_and_module_print_the_same_report(tmp_path: Path) -> None:
    make_tree(tmp_path, {"broken/page.md": "[x](missing.md)\n", "clean/page.md": "# Clean\n"})
    for directory, expected_exit in (("broken", 1), ("clean", 0)):
        script = run_cli([directory], tmp_path)
        module = run_cli([directory], tmp_path, module=True)
        assert script.exit_code == expected_exit, script.describe()
        assert (module.exit_code, module.stdout) == (script.exit_code, script.stdout), module.describe()


def test_console_script_and_module_report_usage_errors_the_same_way(tmp_path: Path) -> None:
    for args in (["--no-such-option"], ["no-such-directory"]):
        assert_usage_error(run_cli(args, tmp_path))
        assert_usage_error(run_cli(args, tmp_path, module=True))
