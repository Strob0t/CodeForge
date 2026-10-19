"""Filesystem state evaluator for Terminal-Bench.

Compares expected filesystem state vs actual state after agent execution.
Checks both file existence/content and absence of files that should be removed.

Score: percentage of checks that pass (0.0 to 1.0).
"""

from __future__ import annotations

import json

import structlog

from codeforge.constants import MAX_WORKSPACE_FILE_BYTES
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec
from codeforge.workspace_fs import WorkspaceRoot

logger = structlog.get_logger(__name__)


class FilesystemStateEvaluator:
    """Evaluator that verifies filesystem state matches expectations."""

    stage = "filter"

    def __init__(self, working_dir: str | None = None) -> None:
        self._working_dir = working_dir

    @property
    def name(self) -> str:
        return "filesystem_state"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        """Compare expected filesystem state with actual state on disk."""
        expected_files_raw = task.metadata.get("expected_files", "")
        expected_missing_raw = task.metadata.get("expected_missing", "")

        expected_files: dict[str, str] = json.loads(expected_files_raw) if expected_files_raw else {}
        expected_missing: list[str] = json.loads(expected_missing_raw) if expected_missing_raw else []

        # No expectations -> perfect score
        if not expected_files and not expected_missing:
            return [
                EvalDimension(
                    name="filesystem_state",
                    score=1.0,
                    details={"passed": "0", "total": "0", "note": "no expectations defined"},
                )
            ]

        # Resolve working directory: constructor > result metadata
        working_dir = self._working_dir or result.metadata.get("working_dir", "")
        if not working_dir:
            return [
                EvalDimension(
                    name="filesystem_state",
                    score=0.0,
                    details={"error": "no working_dir specified in result metadata or constructor"},
                )
            ]

        # The agent wrote the working directory: files are read through the
        # workspace helper (KI-95), never through a symlink that leaves it.
        try:
            root = WorkspaceRoot(working_dir)
        except OSError:
            return [
                EvalDimension(
                    name="filesystem_state",
                    score=0.0,
                    details={"error": f"working_dir does not exist: {working_dir}"},
                )
            ]

        with root:
            passed, failures = _check_files(root, expected_files, expected_missing)
        total = len(expected_files) + len(expected_missing)

        score = passed / total if total > 0 else 1.0

        details: dict[str, str] = {
            "passed": str(passed),
            "total": str(total),
        }
        if failures:
            details["failures"] = "; ".join(failures[:10])

        return [
            EvalDimension(
                name="filesystem_state",
                score=score,
                details=details,
            )
        ]


def _check_files(
    root: WorkspaceRoot, expected_files: dict[str, str], expected_missing: list[str]
) -> tuple[int, list[str]]:
    """How many expectations hold, and the failures."""
    passed = 0
    failures: list[str] = []

    # Check expected files: existence and content
    for rel_path, expected_content in expected_files.items():
        if not root.is_file(rel_path):
            failures.append(f"missing: {rel_path}")
            continue
        # Empty expected content means only check existence
        if not expected_content:
            passed += 1
            continue
        try:
            actual_content = root.read_text(rel_path, max_bytes=MAX_WORKSPACE_FILE_BYTES)
        except Exception as exc:
            failures.append(f"read error {rel_path}: {exc}")
            continue
        # Universal newlines, as the file was read before KI-95.
        if actual_content.replace("\r\n", "\n").replace("\r", "\n") == expected_content:
            passed += 1
        else:
            failures.append(f"content mismatch: {rel_path}")

    # Check expected missing files: should NOT exist (a symlink that leaves
    # the workspace exists as an entry)
    for rel_path in expected_missing:
        try:
            root.stat(rel_path)
        except FileNotFoundError:
            passed += 1
            continue
        except OSError:
            pass
        failures.append(f"should not exist: {rel_path}")

    return passed, failures
