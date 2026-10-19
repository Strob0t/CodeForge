#!/usr/bin/env python3
"""Grades an mdlinkcheck workspace for the autonomous goal benchmark.

Usage: python grade.py WORKSPACE [--judge-model MODEL] [--out REPORT.json] [--interventions N]

The grader copies the workspace into a temporary sandbox, creates a fresh virtual environment there,
runs ``pip install -e .`` and every check of docs/testing/autonomous-goal-benchmark.md, then prints a
JSON report (and writes it with --out). It needs Python 3.12+ with pytest, pytest-cov, ruff, mypy,
radon and bandit installed in the interpreter that runs it; the fresh environment gets the same
pytest, pytest-cov and coverage versions from the package index.

The workspace's code runs during grading (install, tests): grade only inside a disposable machine or
container.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
import urllib.request
import xml.etree.ElementTree as ElementTree
from dataclasses import dataclass, field
from datetime import UTC, datetime
from importlib import metadata
from pathlib import Path
from statistics import mean
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterator, Mapping, Sequence

GRADER_VERSION = "1"
HERE = Path(__file__).resolve().parent
ACCEPTANCE_DIR = HERE / "acceptance"
JUDGE_PROMPT = HERE / "judge_prompt.md"

WEIGHTS = {
    "acceptance": 40,
    "own_tests": 10,
    "coverage": 10,
    "lint": 10,
    "types": 10,
    "complexity": 5,
    "security": 5,
    "review": 10,
}
FULL_POINTS = 10.0
RUFF_RULES = "E,F,W,I,B,UP,SIM,RUF"
RUFF_DEFAULT_LINE_LENGTH = 88
RUFF_LINE_LENGTH_RANGE = (79, 120)
MIN_OWN_TESTS = 20
COVERAGE_FULL = 85.0
COVERAGE_ZERO = 50.0
COMPLEXITY_LIMIT = 10
SECURITY_PENALTY_PER_FINDING = 5.0
JUDGE_RUNS = 3
JUDGE_MAX_ATTEMPTS = 6
JUDGE_CRITERIA = ("structure", "naming", "error_handling", "tests", "readme")
JUDGE_MAX_FILE_CHARS = 30_000
JUDGE_MAX_TOTAL_CHARS = 150_000
SUCCESS_MIN_ACCEPTANCE = 0.90
SUCCESS_MIN_TOTAL = 70.0
GRADER_TOOLS = ("pytest", "pytest-cov", "coverage", "ruff", "mypy", "radon", "bandit")
VENV_TOOLS = ("pytest", "pytest-cov", "coverage")
INSTALL_TIMEOUT = 600
TEST_TIMEOUT = 1200
TOOL_TIMEOUT = 600
JUDGE_TIMEOUT = 300
COPY_IGNORE = shutil.ignore_patterns(
    ".git",
    ".venv",
    "venv",
    ".tox",
    "__pycache__",
    "*.pyc",
    ".mypy_cache",
    ".pytest_cache",
    ".ruff_cache",
    "node_modules",
    "*.egg-info",
    "build",
    "dist",
    "htmlcov",
    ".coverage",
    ".coverage.*",
)
# Empty tool configurations: the project's own settings must not switch rules or files off.
COVERAGE_RC = "[run]\nbranch = True\npatch = subprocess\n"
BANDIT_INI = "[bandit]\n"


class GraderError(Exception):
    """The grader itself cannot run (missing tools, no package index); not the workspace's fault."""


@dataclass
class Check:
    """One row of the rubric: the raw result, the points on a 10-point scale and the weighted score."""

    name: str
    summary: str
    raw: dict[str, object]
    points: float | None
    note: str = ""

    @property
    def weight(self) -> int:
        return WEIGHTS[self.name]

    @property
    def scored(self) -> bool:
        return self.points is not None

    @property
    def score(self) -> float | None:
        return None if self.points is None else self.points / FULL_POINTS * self.weight

    def to_json(self) -> dict[str, object]:
        result: dict[str, object] = {
            "weight": self.weight,
            "scored": self.scored,
            "summary": self.summary,
            "points": _round(self.points),
            "score": _round(self.score),
            "raw": self.raw,
        }
        if self.note:
            result["note"] = self.note
        return result


@dataclass(frozen=True)
class Completed:
    returncode: int
    stdout: str
    stderr: str
    timed_out: bool

    def tail(self, lines: int = 25) -> str:
        return "\n".join((self.stdout + self.stderr).strip().splitlines()[-lines:])


@dataclass(frozen=True)
class Sandbox:
    root: Path
    workspace: Path
    acceptance: Path
    python: Path

    @property
    def src(self) -> Path:
        return self.workspace / "src"


@dataclass
class JUnitCounts:
    passed: int = 0
    failed: list[str] = field(default_factory=list)
    skipped: int = 0

    @property
    def executed(self) -> int:
        return self.passed + len(self.failed)

    @property
    def total(self) -> int:
        return self.executed + self.skipped


def main(argv: Sequence[str] | None = None) -> int:
    args = _parse_args(argv)
    workspace = Path(args.workspace).resolve()
    if not workspace.is_dir():
        sys.stderr.write(f"grade.py: error: workspace is not a directory: {workspace}\n")
        return 2
    try:
        _require_grader_tools()
        checks, install = grade(workspace, args)
    except GraderError as exc:
        sys.stderr.write(f"grade.py: error: {exc}\n")
        return 2
    total = _total(checks)
    success = _success(checks, total, args.interventions)
    report = _report(workspace, args.interventions, install, checks, total, success)
    text = json.dumps(report, indent=2) + "\n"
    sys.stdout.write(text)
    if args.out:
        Path(args.out).write_text(text, encoding="utf-8")
    sys.stderr.write(_table(checks, total, success))
    return 0 if success["value"] else 1


def _parse_args(argv: Sequence[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Grade an mdlinkcheck workspace.")
    parser.add_argument("workspace", help="the frozen copy of the agent's workspace")
    parser.add_argument("--judge-model", help="LiteLLM model for the review (LITELLM_BASE_URL)")
    parser.add_argument("--out", help="also write the JSON report to this file")
    parser.add_argument("--interventions", type=int, default=0, help="human interventions logged")
    parser.add_argument("--python", default=sys.executable, help="interpreter for the fresh venv (3.12+)")
    parser.add_argument("--work-dir", help="where to create the sandbox (default: system temp)")
    parser.add_argument("--keep", action="store_true", help="keep the sandbox for inspection")
    return parser.parse_args(argv)


def _require_grader_tools() -> None:
    missing = [tool for tool in GRADER_TOOLS if _version(tool) is None]
    if missing:
        raise GraderError(f"missing grading tools: {' '.join(missing)} (pip install them)")
    if not ACCEPTANCE_DIR.is_dir():
        raise GraderError(f"acceptance suite not found: {ACCEPTANCE_DIR}")


def _version(distribution: str) -> str | None:
    try:
        return metadata.version(distribution)
    except metadata.PackageNotFoundError:
        return None


def grade(workspace: Path, args: argparse.Namespace) -> tuple[list[Check], dict[str, object]]:
    """Runs every check on a sandboxed copy of the workspace; returns the checks and the install result."""
    root = Path(tempfile.mkdtemp(prefix="mdlinkcheck-grade-", dir=args.work_dir))
    try:
        sandbox = _prepare(workspace, root, Path(args.python))
        install = _install(sandbox)
        return _run_checks(sandbox, args.judge_model), install
    finally:
        if args.keep:
            sys.stderr.write(f"grade.py: sandbox kept at {root}\n")
        else:
            shutil.rmtree(root, ignore_errors=True)


def _prepare(workspace: Path, root: Path, python: Path) -> Sandbox:
    sandbox = Sandbox(root, root / "workspace", root / "acceptance", _venv_python(root / "venv"))
    shutil.copytree(workspace, sandbox.workspace, ignore=COPY_IGNORE, symlinks=True)
    shutil.copytree(ACCEPTANCE_DIR, sandbox.acceptance, ignore=COPY_IGNORE)
    (root / "coverage.rc").write_text(COVERAGE_RC, encoding="utf-8")
    (root / "bandit.ini").write_text(BANDIT_INI, encoding="utf-8")
    created = _run([python, "-m", "venv", root / "venv"], cwd=root, timeout=INSTALL_TIMEOUT)
    if created.returncode != 0:
        raise GraderError(f"cannot create a virtual environment:\n{created.tail()}")
    return sandbox


def _venv_python(venv: Path) -> Path:
    return venv / ("Scripts/python.exe" if os.name == "nt" else "bin/python")


def _install(sandbox: Sandbox) -> dict[str, object]:
    pip: list[str | Path] = [sandbox.python, "-m", "pip", "install", "--disable-pip-version-check", "-q"]
    pinned = [f"{tool}=={_version(tool)}" for tool in VENV_TOOLS]
    tools = _run([*pip, *pinned], cwd=sandbox.root, timeout=INSTALL_TIMEOUT)
    if tools.returncode != 0:
        raise GraderError(f"cannot install {' '.join(pinned)} into the fresh venv:\n{tools.tail()}")
    project = _run([*pip, "-e", sandbox.workspace], cwd=sandbox.workspace, timeout=INSTALL_TIMEOUT)
    return {"ok": project.returncode == 0, "output": project.tail()}


def _run_checks(sandbox: Sandbox, judge_model: str | None) -> list[Check]:
    """Static checks run before the project's own tests, so nothing a test writes can change them."""
    checks = [_check_acceptance(sandbox)]
    if any(sandbox.src.rglob("*.py")):
        checks += [
            _check_lint(sandbox),
            _check_types(sandbox),
            _check_complexity(sandbox),
            _check_security(sandbox),
        ]
    else:
        reason = "no Python files in src/"
        checks += [Check(name, reason, {}, 0.0) for name in ("lint", "types", "complexity", "security")]
    checks += [*_check_own_tests(sandbox), _check_review(sandbox, judge_model)]
    rubric_order = list(WEIGHTS)
    return sorted(checks, key=lambda check: rubric_order.index(check.name))


# -- Acceptance and own tests ----------------------------------------------------------------------


def _check_acceptance(sandbox: Sandbox) -> Check:
    report = sandbox.root / "acceptance.xml"
    env = {**os.environ, "PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1"}
    command: list[str | Path] = [sandbox.python, "-m", "pytest", "-q", f"--junitxml={report}", "."]
    done = _run(command, cwd=sandbox.acceptance, timeout=TEST_TIMEOUT, env=env)
    counts = _junit_counts(report)
    if counts is None or counts.total == 0:
        return Check("acceptance", "no results", {"timed_out": done.timed_out, "output": done.tail()}, 0.0)
    rate = counts.passed / counts.total
    raw: dict[str, object] = {
        "total": counts.total,
        "passed": counts.passed,
        "pass_rate": round(rate, 4),
        "failed": counts.failed,
        "skipped": counts.skipped,
        "timed_out": done.timed_out,
    }
    return Check("acceptance", f"{counts.passed}/{counts.total} passed", raw, FULL_POINTS * rate)


def _check_own_tests(sandbox: Sandbox) -> tuple[Check, Check]:
    report = sandbox.root / "own-tests.xml"
    coverage_json = sandbox.root / "coverage.json"
    command: list[str | Path] = [
        sandbox.python,
        "-m",
        "pytest",
        "-q",
        "-p",
        "no:cacheprovider",
        f"--junitxml={report}",
        f"--cov={sandbox.src}",
        "--cov-branch",
        f"--cov-config={sandbox.root / 'coverage.rc'}",
        f"--cov-report=json:{coverage_json}",
    ]
    done = _run(command, cwd=sandbox.workspace, timeout=TEST_TIMEOUT)
    return _own_tests_check(_junit_counts(report), done), _coverage_check(coverage_json)


def _own_tests_check(counts: JUnitCounts | None, done: Completed) -> Check:
    raw: dict[str, object]
    if counts is None or counts.executed == 0:
        raw = {"exit_code": done.returncode, "timed_out": done.timed_out, "output": done.tail()}
        return Check("own_tests", "no tests ran", raw, 0.0)
    # Skipped tests neither pass nor fail, and they do not count towards MIN_OWN_TESTS.
    share_passed = counts.passed / counts.executed
    points = FULL_POINTS * share_passed * min(1.0, counts.executed / MIN_OWN_TESTS)
    raw = {
        "executed": counts.executed,
        "passed": counts.passed,
        "failed": counts.failed,
        "skipped": counts.skipped,
        "exit_code": done.returncode,
        "timed_out": done.timed_out,
    }
    summary = f"{counts.passed}/{counts.executed} passed"
    return Check("own_tests", summary, raw, points)


def _coverage_check(coverage_json: Path) -> Check:
    try:
        totals = json.loads(coverage_json.read_text(encoding="utf-8"))["totals"]
    except (OSError, ValueError, KeyError):
        return Check("coverage", "no coverage data", {}, 0.0)
    percent = float(totals["percent_covered"])
    branches = int(totals.get("num_branches", 0))
    raw: dict[str, object] = {
        "percent": round(percent, 2),
        "statements": totals.get("num_statements"),
        "covered_statements": totals.get("covered_lines"),
        "branches": branches,
        "covered_branches": totals.get("covered_branches"),
    }
    span = COVERAGE_FULL - COVERAGE_ZERO
    points = FULL_POINTS * min(1.0, max(0.0, (percent - COVERAGE_ZERO) / span))
    return Check("coverage", f"{percent:.1f}% (lines and branches)", raw, points)


def _junit_counts(report: Path) -> JUnitCounts | None:
    """Per-test outcomes from pytest's JUnit XML, which the grader's own pytest run wrote."""
    try:
        tree = ElementTree.parse(report)  # noqa: S314 - written by the pytest run this grader started
    except (OSError, ElementTree.ParseError):
        return None
    counts = JUnitCounts()
    for case in tree.iter("testcase"):
        name = f"{case.get('classname', '')}::{case.get('name', '')}"
        if case.find("failure") is not None or case.find("error") is not None:
            counts.failed.append(name)
        elif case.find("skipped") is not None:
            counts.skipped += 1
        else:
            counts.passed += 1
    return counts


# -- Static checks ---------------------------------------------------------------------------------


def _check_lint(sandbox: Sandbox) -> Check:
    line_length = _project_line_length(sandbox.workspace)
    command = [
        sys.executable,
        "-m",
        "ruff",
        "check",
        "--isolated",
        "--no-cache",
        "--exit-zero",
        "--output-format=json",
        f"--select={RUFF_RULES}",
        "--target-version=py312",
        f"--line-length={line_length}",
        ".",
    ]
    done = _run(command, cwd=sandbox.workspace, timeout=TOOL_TIMEOUT)
    findings = _json_list(done.stdout)
    if findings is None:
        return Check("lint", "ruff failed", {"output": done.tail()}, 0.0)
    rules = sorted({str(finding.get("code")) for finding in findings})
    raw: dict[str, object] = {
        "findings": len(findings),
        "by_rule": {rule: sum(1 for f in findings if str(f.get("code")) == rule) for rule in rules},
        "line_length": line_length,
        "noqa_comments": _count_in_python(sandbox.workspace, "# noqa"),
        "first": [_ruff_line(finding, sandbox.workspace) for finding in findings[:20]],
    }
    return Check("lint", f"{len(findings)} findings", raw, _minus_one_each(len(findings)))


def _project_line_length(workspace: Path) -> int:
    """The project's ruff line length, within RUFF_LINE_LENGTH_RANGE; the rule set stays fixed."""
    for name, table_path in (("ruff.toml", ()), (".ruff.toml", ()), ("pyproject.toml", ("tool", "ruff"))):
        try:
            table: object = tomllib.loads((workspace / name).read_text(encoding="utf-8"))
        except (OSError, ValueError):
            continue
        for key in table_path:
            table = table.get(key, {}) if isinstance(table, dict) else {}
        value = table.get("line-length") if isinstance(table, dict) else None
        if isinstance(value, int) and not isinstance(value, bool):
            low, high = RUFF_LINE_LENGTH_RANGE
            return min(high, max(low, value))
    return RUFF_DEFAULT_LINE_LENGTH


def _ruff_line(finding: dict[str, object], workspace: Path) -> str:
    location = finding.get("location")
    row = location.get("row") if isinstance(location, dict) else "?"
    filename = Path(str(finding.get("filename", "?")))
    shown = filename.relative_to(workspace) if filename.is_relative_to(workspace) else filename
    return f"{shown.as_posix()}:{row}: {finding.get('code')} {finding.get('message')}"


def _check_types(sandbox: Sandbox) -> Check:
    command = [
        sys.executable,
        "-m",
        "mypy",
        "--strict",
        "--config-file=",
        f"--python-executable={sandbox.python}",
        f"--cache-dir={sandbox.root / 'mypy-cache'}",
        "--no-error-summary",
        "--show-error-codes",
        "src",
    ]
    done = _run(command, cwd=sandbox.workspace, timeout=TOOL_TIMEOUT)
    errors = [line for line in done.stdout.splitlines() if ": error:" in line]
    raw: dict[str, object] = {
        "errors": len(errors),
        "exit_code": done.returncode,
        "type_ignore_comments": _count_in_python(sandbox.src, "type: ignore"),
        "first": errors[:20],
    }
    if done.returncode not in (0, 1) and not errors:
        return Check("types", "mypy failed", {**raw, "output": done.tail()}, 0.0)
    return Check("types", f"{len(errors)} errors", raw, _minus_one_each(len(errors)))


def _check_complexity(sandbox: Sandbox) -> Check:
    # cwd is the sandbox root, so no radon configuration from the workspace applies.
    command = [sys.executable, "-m", "radon", "cc", "--json", str(sandbox.src)]
    done = _run(command, cwd=sandbox.root, timeout=TOOL_TIMEOUT)
    try:
        data = json.loads(done.stdout)
    except ValueError:
        return Check("complexity", "radon failed", {"output": done.tail()}, 0.0)
    blocks = list(_radon_blocks(data, sandbox.src))
    over = [block for block in blocks if int(str(block["complexity"])) > COMPLEXITY_LIMIT]
    unparsable = sorted(
        Path(path).relative_to(sandbox.src).as_posix() for path, value in data.items() if isinstance(value, dict)
    )
    raw: dict[str, object] = {
        "functions": len(blocks),
        "max_complexity": max((int(str(block["complexity"])) for block in blocks), default=0),
        "over_limit": over,
        "unparsable_files": unparsable,
    }
    deductions = len(over) + len(unparsable)
    return Check("complexity", f"{len(over)} functions above {COMPLEXITY_LIMIT}", raw, _minus_one_each(deductions))


def _radon_blocks(data: dict[str, object], src: Path) -> Iterator[dict[str, object]]:
    """Functions, methods and closures, once each (radon lists methods twice)."""
    seen: set[tuple[str, object, object]] = set()
    pending: list[tuple[str, object]] = [
        (Path(path).relative_to(src).as_posix(), block)
        for path, blocks in data.items()
        if isinstance(blocks, list)
        for block in blocks
    ]
    while pending:
        path, block = pending.pop()
        if not isinstance(block, dict):
            continue
        pending += [(path, child) for key in ("methods", "closures") for child in block.get(key, [])]
        key = (path, block.get("lineno"), block.get("col_offset"))
        if block.get("type") in ("function", "method") and key not in seen:
            seen.add(key)
            yield {
                "file": path,
                "line": block.get("lineno"),
                "name": block.get("name"),
                "complexity": block.get("complexity"),
            }


def _check_security(sandbox: Sandbox) -> Check:
    # An empty --ini keeps a .bandit file in the workspace from skipping tests.
    command = [
        sys.executable,
        "-m",
        "bandit",
        "-q",
        "-r",
        str(sandbox.src),
        "-f",
        "json",
        "--ini",
        str(sandbox.root / "bandit.ini"),
    ]
    done = _run(command, cwd=sandbox.root, timeout=TOOL_TIMEOUT)
    try:
        data = json.loads(done.stdout)
        results = list(data["results"])
    except (ValueError, KeyError, TypeError):
        return Check("security", "bandit failed", {"output": done.tail()}, 0.0)
    serious = [result for result in results if result.get("issue_severity") in ("MEDIUM", "HIGH")]
    raw: dict[str, object] = {
        "medium_or_high": len(serious),
        "low": len(results) - len(serious),
        "nosec_comments": data.get("metrics", {}).get("_totals", {}).get("nosec", 0),
        "issues": [_bandit_issue(result, sandbox.src) for result in results],
    }
    points = max(0.0, FULL_POINTS - SECURITY_PENALTY_PER_FINDING * len(serious))
    return Check("security", f"{len(serious)} medium/high findings", raw, points)


def _bandit_issue(result: dict[str, object], src: Path) -> str:
    filename = Path(str(result.get("filename", "?")))
    shown = filename.relative_to(src) if filename.is_relative_to(src) else filename
    severity = result.get("issue_severity")
    return f"{shown.as_posix()}:{result.get('line_number')}: {result.get('test_id')} {severity}"


# -- Review (LLM judge) ----------------------------------------------------------------------------


def _check_review(sandbox: Sandbox, model: str | None) -> Check:
    if model is None:
        return Check("review", "not scored", {}, None, note="no --judge-model given")
    base_url = os.environ.get("LITELLM_BASE_URL", "")
    api_key = os.environ.get("LITELLM_MASTER_KEY", "")
    if not base_url.startswith(("http://", "https://")) or not api_key:
        note = "LITELLM_BASE_URL (http or https) and LITELLM_MASTER_KEY must be set"
        return Check("review", "not scored", {"model": model}, None, note=note)
    prompt = JUDGE_PROMPT.read_text(encoding="utf-8")
    bundle = _code_bundle(sandbox.workspace)
    runs: list[dict[str, object]] = []
    errors: list[str] = []
    # A failed run (no answer, no valid JSON) is retried, up to JUDGE_MAX_ATTEMPTS calls in all.
    while len(runs) < JUDGE_RUNS and len(runs) + len(errors) < JUDGE_MAX_ATTEMPTS:
        try:
            runs.append(_ask_judge(base_url, api_key, model, prompt, bundle))
        except (OSError, ValueError, KeyError, TypeError) as exc:
            errors.append(f"{type(exc).__name__}: {exc}")
    raw: dict[str, object] = {"model": model, "runs": runs, "errors": errors}
    if not runs:
        return Check("review", "not scored", raw, None, note="every judge run failed")
    points = mean(float(str(run["score"])) for run in runs)
    return Check("review", f"{points:.1f}/10 over {len(runs)} runs", raw, points)


def _code_bundle(workspace: Path) -> str:
    files = [workspace / "README.md", workspace / "pyproject.toml"]
    for directory in ("src", "tests", "test"):
        files += sorted((workspace / directory).rglob("*.py"))
    parts: list[str] = []
    budget = JUDGE_MAX_TOTAL_CHARS
    for path in files:
        if not path.is_file() or budget <= 0:
            continue
        text = path.read_text(encoding="utf-8", errors="replace")
        limit = min(JUDGE_MAX_FILE_CHARS, budget)
        clipped = text if len(text) <= limit else text[:limit] + "\n[... truncated by the grader ...]\n"
        parts.append(f"===== {path.relative_to(workspace).as_posix()} =====\n{clipped}")
        budget -= len(clipped)
    return "\n".join(parts) if parts else "(the workspace has no README, pyproject.toml or Python files)"


def _ask_judge(base_url: str, api_key: str, model: str, prompt: str, bundle: str) -> dict[str, object]:
    messages = [{"role": "system", "content": prompt}, {"role": "user", "content": bundle}]
    # The scheme of base_url was checked to be http or https in _check_review.
    request = urllib.request.Request(  # noqa: S310
        f"{base_url.rstrip('/')}/chat/completions",
        data=json.dumps({"model": model, "messages": messages}).encode(),
        headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=JUDGE_TIMEOUT) as response:  # noqa: S310
        answer = json.loads(response.read())
    content = str(answer["choices"][0]["message"]["content"])
    return _judge_scores(content)


def _judge_scores(content: str) -> dict[str, object]:
    match = re.search(r"\{.*\}", content, re.DOTALL)
    if match is None:
        raise ValueError(f"no JSON object in the judge's answer: {content[:200]!r}")
    verdict = json.loads(match.group())
    scores = {criterion: float(verdict[criterion]) for criterion in JUDGE_CRITERIA}
    if any(not 1 <= value <= FULL_POINTS for value in scores.values()):
        raise ValueError(f"judge scores outside 1..10: {scores}")
    return {**scores, "score": mean(scores.values()), "summary": str(verdict.get("summary", ""))}


# -- Report ----------------------------------------------------------------------------------------


@dataclass(frozen=True)
class Total:
    score: float
    max_score: int
    percent: float


def _total(checks: Sequence[Check]) -> Total:
    """Sums the scored checks; a check that was not scored (review without a judge) scales the total."""
    scored = [check for check in checks if check.scored]
    max_score = sum(check.weight for check in scored)
    score = sum(check.score or 0.0 for check in scored)
    return Total(score, max_score, 100.0 * score / max_score if max_score else 0.0)


def _success(checks: Sequence[Check], total: Total, interventions: int) -> dict[str, bool]:
    acceptance = next(check for check in checks if check.name == "acceptance")
    pass_rate = float(str(acceptance.raw.get("pass_rate", 0.0)))
    criteria = {
        "autonomous": interventions == 0,
        "acceptance_at_least_90_percent": pass_rate >= SUCCESS_MIN_ACCEPTANCE,
        "total_at_least_70_percent": total.percent >= SUCCESS_MIN_TOTAL,
    }
    return {"value": all(criteria.values()), **criteria}


def _report(
    workspace: Path,
    interventions: int,
    install: dict[str, object],
    checks: Sequence[Check],
    total: Total,
    success: dict[str, bool],
) -> dict[str, object]:
    return {
        "grader_version": GRADER_VERSION,
        "graded_at": datetime.now(UTC).isoformat(timespec="seconds"),
        "workspace": str(workspace),
        "python": sys.version.split()[0],
        "tools": {tool: _version(tool) for tool in GRADER_TOOLS},
        "interventions": interventions,
        "install": install,
        "checks": {check.name: check.to_json() for check in checks},
        "total": {
            "score": round(total.score, 2),
            "max": total.max_score,
            "percent": round(total.percent, 2),
            "scaled_over_scored_checks": total.max_score < sum(WEIGHTS.values()),
        },
        "success": success,
    }


def _table(checks: Sequence[Check], total: Total, success: dict[str, bool]) -> str:
    lines = [f"{'check':<11} {'weight':>6}  {'result':<36} {'points':>6} {'score':>6}"]
    for check in checks:
        points = "-" if check.points is None else f"{check.points:.2f}"
        score = "-" if check.score is None else f"{check.score:.2f}"
        lines.append(f"{check.name:<11} {check.weight:>6}  {check.summary:<36} {points:>6} {score:>6}")
    lines.append(f"total: {total.score:.2f} of {total.max_score} = {total.percent:.2f} %")
    lines.append(f"success: {'yes' if success['value'] else 'no'}")
    return "\n".join(lines) + "\n"


# -- Helpers ---------------------------------------------------------------------------------------


def _run(
    command: Sequence[str | Path],
    *,
    cwd: Path,
    timeout: float,
    env: Mapping[str, str] | None = None,
) -> Completed:
    try:
        done = subprocess.run(  # noqa: S603 - the grader runs fixed tool commands on its own sandbox
            [str(part) for part in command],
            cwd=cwd,
            env=dict(env) if env is not None else None,
            capture_output=True,
            text=True,
            errors="replace",
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        return Completed(-1, _decoded(exc.stdout), _decoded(exc.stderr) + f"\n[timed out after {timeout} s]", True)
    return Completed(done.returncode, done.stdout, done.stderr, False)


def _decoded(output: str | bytes | None) -> str:
    if isinstance(output, bytes):
        return output.decode(errors="replace")
    return output or ""


def _json_list(text: str) -> list[dict[str, object]] | None:
    try:
        data = json.loads(text)
    except ValueError:
        return None
    return [item for item in data if isinstance(item, dict)] if isinstance(data, list) else None


def _count_in_python(directory: Path, needle: str) -> int:
    return sum(path.read_text(encoding="utf-8", errors="replace").count(needle) for path in directory.rglob("*.py"))


def _minus_one_each(count: int) -> float:
    return max(0.0, FULL_POINTS - count)


def _round(value: float | None) -> float | None:
    return None if value is None else round(value, 2)


if __name__ == "__main__":
    sys.exit(main())
