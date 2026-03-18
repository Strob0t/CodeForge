"""Filesystem state evaluator for Terminal-Bench.

Compares expected filesystem state vs actual state after agent execution.
Checks both file existence/content and absence of files that should be removed.

Score: percentage of checks that pass (0.0 to 1.0).
"""

from __future__ import annotations

import json
import os

import structlog

from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

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

        if not os.path.isdir(working_dir):
            return [
                EvalDimension(
                    name="filesystem_state",
                    score=0.0,
                    details={"error": f"working_dir does not exist: {working_dir}"},
                )
            ]

        passed = 0
        total = len(expected_files) + len(expected_missing)
        failures: list[str] = []

        # Check expected files: existence and content
        for rel_path, expected_content in expected_files.items():
            abs_path = os.path.join(working_dir, rel_path)
            if not os.path.isfile(abs_path):
                failures.append(f"missing: {rel_path}")
                continue
            # Empty expected content means only check existence
            if not expected_content:
                passed += 1
                continue
            try:
                with open(abs_path, encoding="utf-8") as f:
                    actual_content = f.read()
            except Exception as exc:
                failures.append(f"read error {rel_path}: {exc}")
                continue
            if actual_content == expected_content:
                passed += 1
            else:
                failures.append(f"content mismatch: {rel_path}")

        # Check expected missing files: should NOT exist
        for rel_path in expected_missing:
            abs_path = os.path.join(working_dir, rel_path)
            if os.path.exists(abs_path):
                failures.append(f"should not exist: {rel_path}")
            else:
                passed += 1

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
