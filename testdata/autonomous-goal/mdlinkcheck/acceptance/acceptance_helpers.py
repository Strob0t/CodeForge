"""Helpers for the hidden mdlinkcheck acceptance suite: run the CLI and read its report.

The suite tests the tool only from the outside: it starts the console script ``mdlinkcheck`` (or
``python -m mdlinkcheck``) in a fresh directory tree and reads stdout, stderr and the exit code. It never
imports the package. Where SPEC.md leaves room for more than one reading, the assertions accept each
reasonable reading.

Environment:
- ``MDLINKCHECK_BIN``: the console script to run (default: ``mdlinkcheck`` next to ``sys.executable``,
  then on ``PATH``).
- ``MDLINKCHECK_PYTHON``: the interpreter for ``python -m mdlinkcheck`` (default: ``sys.executable``).
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import textwrap
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from collections.abc import Mapping, Sequence

CLI_TIMEOUT_SECONDS = 60
PROBLEM_LINE = re.compile(r"^(?P<path>[^:\n]+):(?P<line>\d+):(?P<column>\d+): (?P<kind>[a-z][a-z-]*): (?P<detail>.+)$")
SUMMARY_FILES = re.compile(r" in (?P<files>\d+) files?$")
PROXY_VARIABLES = {"http_proxy", "https_proxy", "all_proxy", "no_proxy"}

# (path, line, column, kind, target): what a problem is compared by. The message is free text.
ProblemKey = tuple[str, int, int, str, str]


def md(text: str) -> str:
    """Dedents a triple-quoted Markdown snippet and drops its leading newline."""
    return textwrap.dedent(text).lstrip("\n")


def make_tree(root: Path, files: Mapping[str, str]) -> Path:
    """Creates files below root; a key ending in "/" creates an empty directory."""
    for relative, content in files.items():
        path = root / relative
        if relative.endswith("/"):
            path.mkdir(parents=True, exist_ok=True)
            continue
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
    return root


def column_of(line: str, needle: str) -> int:
    """The 1-based column of the first occurrence of needle in line."""
    return line.index(needle) + 1


@dataclass(frozen=True)
class Problem:
    path: str
    line: int
    column: int
    kind: str
    target: str
    message: str

    @property
    def key(self) -> ProblemKey:
        return (self.path, self.line, self.column, self.kind, self.target)


def _parse_problem(text: str) -> Problem | None:
    match = PROBLEM_LINE.match(text)
    if match is None:
        return None
    detail = match["detail"]
    # "<target> (<message>)": the suite's targets never contain " (", so the first one starts the message.
    target, separator, message = detail.partition(" (")
    if not separator or not message.endswith(")"):
        return None
    return Problem(
        path=match["path"],
        line=int(match["line"]),
        column=int(match["column"]),
        kind=match["kind"],
        target=target,
        message=message[:-1],
    )


@dataclass(frozen=True)
class Run:
    args: tuple[str, ...]
    exit_code: int
    stdout: str
    stderr: str

    def describe(self) -> str:
        return (
            f"args={list(self.args)}\nexit={self.exit_code}\n--- stdout ---\n{self.stdout}--- stderr ---\n{self.stderr}"
        )

    @property
    def lines(self) -> list[str]:
        return self.stdout.rstrip("\n").split("\n") if self.stdout.strip() else []

    @property
    def summary(self) -> str:
        assert self.lines, f"expected a summary line\n{self.describe()}"
        return self.lines[-1]

    @property
    def problems(self) -> list[Problem]:
        """The text report's problem lines; every line but the summary must be one."""
        parsed: list[Problem] = []
        for text in self.lines[:-1]:
            problem = _parse_problem(text)
            assert problem is not None, f"not a problem line: {text!r}\n{self.describe()}"
            parsed.append(problem)
        return parsed

    @property
    def problem_keys(self) -> list[ProblemKey]:
        return [problem.key for problem in self.problems]

    def json(self) -> dict[str, object]:
        try:
            data = json.loads(self.stdout)
        except json.JSONDecodeError as exc:
            pytest.fail(f"stdout is not one JSON document: {exc}\n{self.describe()}")
        assert isinstance(data, dict), f"the JSON report must be an object\n{self.describe()}"
        return data

    def summary_file_count(self) -> int:
        match = SUMMARY_FILES.search(self.summary)
        assert match is not None, f"the summary does not name a file count: {self.summary!r}\n{self.describe()}"
        return int(match["files"])


def _console_script() -> str:
    override = os.environ.get("MDLINKCHECK_BIN")
    if override:
        return override
    name = "mdlinkcheck.exe" if os.name == "nt" else "mdlinkcheck"
    beside_interpreter = Path(sys.executable).parent / name
    if beside_interpreter.is_file():
        return str(beside_interpreter)
    found = shutil.which("mdlinkcheck")
    if found is None:
        pytest.fail("the console script 'mdlinkcheck' is not installed (pip install -e . in the workspace)")
    return found


def _clean_environment() -> dict[str, str]:
    """The tool's environment: no proxies, so requests to 127.0.0.1 stay local and nothing leaves the host."""
    env = {name: value for name, value in os.environ.items() if name.lower() not in PROXY_VARIABLES}
    env["NO_PROXY"] = env["no_proxy"] = "*"
    env.pop("PYTHONPATH", None)
    return env


def run_cli(args: Sequence[str], cwd: Path, *, module: bool = False) -> Run:
    """Runs mdlinkcheck with args in cwd, as the console script or as ``python -m mdlinkcheck``."""
    if module:
        command = [os.environ.get("MDLINKCHECK_PYTHON", sys.executable), "-m", "mdlinkcheck", *args]
    else:
        command = [_console_script(), *args]
    completed = subprocess.run(
        command,
        cwd=cwd,
        env=_clean_environment(),
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=CLI_TIMEOUT_SECONDS,
        check=False,
    )
    return Run(tuple(args), completed.returncode, completed.stdout, completed.stderr)


def assert_problems(run: Run, expected: Sequence[ProblemKey]) -> None:
    """The text report lists exactly these problems, in this order, and the exit code is 1."""
    assert run.problem_keys == list(expected), run.describe()
    assert run.exit_code == 1, run.describe()


def assert_problems_any_of(run: Run, alternatives: Sequence[Sequence[ProblemKey]]) -> None:
    """Like assert_problems, for a case the spec allows more than one reading of."""
    assert run.problem_keys in [list(option) for option in alternatives], run.describe()
    assert run.exit_code == 1, run.describe()


def assert_clean(run: Run) -> None:
    """No problems: exit code 0 and the only line is the no-problems summary."""
    assert run.exit_code == 0, run.describe()
    assert len(run.lines) == 1, run.describe()
    assert run.summary.startswith("No problems found in "), run.describe()


def assert_usage_error(run: Run) -> None:
    """A usage error: exit code 2, a message on stderr and no report on stdout."""
    assert run.exit_code == 2, run.describe()
    assert run.stderr.strip(), run.describe()
    assert not run.stdout.strip(), run.describe()
