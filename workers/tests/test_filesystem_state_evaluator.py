"""Tests for filesystem state evaluator.

The filesystem state evaluator compares expected filesystem state vs actual
state after agent execution. Used by Terminal-Bench to verify CLI task results.

Tests cover: matching, partial matches, missing files, extra files, content
comparison, empty expected state, edge cases.
"""

from __future__ import annotations

import json
import os
import tempfile

import pytest

from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec

# ---------------------------------------------------------------------------
# Helper to create a TaskSpec with filesystem expectations
# ---------------------------------------------------------------------------


def _make_task(
    expected_files: dict[str, str] | None = None,
    expected_missing: list[str] | None = None,
    task_id: str = "test_001",
) -> TaskSpec:
    """Build a TaskSpec with filesystem expectations in metadata."""
    metadata: dict[str, str] = {}
    if expected_files is not None:
        metadata["expected_files"] = json.dumps(expected_files)
    if expected_missing is not None:
        metadata["expected_missing"] = json.dumps(expected_missing)
    return TaskSpec(
        id=task_id,
        name=task_id,
        input="Test task",
        test_command="verify_filesystem_state",
        metadata=metadata,
    )


def _make_result(working_dir: str = "") -> ExecutionResult:
    """Build a minimal ExecutionResult with optional working_dir in metadata."""
    metadata: dict[str, str] = {}
    if working_dir:
        metadata["working_dir"] = working_dir
    return ExecutionResult(metadata=metadata)


# ---------------------------------------------------------------------------
# Evaluator property tests
# ---------------------------------------------------------------------------


class TestFilesystemStateEvaluatorProperties:
    def test_name(self) -> None:
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        e = FilesystemStateEvaluator()
        assert e.name == "filesystem_state"

    def test_stage_is_filter(self) -> None:
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        e = FilesystemStateEvaluator()
        assert e.stage == "filter"


# ---------------------------------------------------------------------------
# Evaluation tests with real filesystem
# ---------------------------------------------------------------------------


class TestFilesystemStateEvaluation:
    @pytest.mark.asyncio
    async def test_all_files_match_score_1(self) -> None:
        """All expected files exist with correct content -> score 1.0."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            # Create expected files
            os.makedirs(os.path.join(tmpdir, "src"), exist_ok=True)
            with open(os.path.join(tmpdir, "src", "main.py"), "w") as f:
                f.write("print('hello')")
            with open(os.path.join(tmpdir, "README.md"), "w") as f:
                f.write("# Project")

            task = _make_task(
                expected_files={
                    "src/main.py": "print('hello')",
                    "README.md": "# Project",
                }
            )
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert len(dims) == 1
            assert dims[0].name == "filesystem_state"
            assert dims[0].score == 1.0

    @pytest.mark.asyncio
    async def test_no_files_match_score_0(self) -> None:
        """No expected files exist -> score 0.0."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            task = _make_task(
                expected_files={
                    "missing.txt": "content",
                    "also_missing.txt": "more content",
                }
            )
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert dims[0].score == 0.0

    @pytest.mark.asyncio
    async def test_partial_match(self) -> None:
        """Some files match, some don't -> partial score."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "exists.txt"), "w") as f:
                f.write("content")

            task = _make_task(
                expected_files={
                    "exists.txt": "content",
                    "missing.txt": "other",
                }
            )
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert 0.0 < dims[0].score < 1.0

    @pytest.mark.asyncio
    async def test_wrong_content_reduces_score(self) -> None:
        """File exists but with wrong content -> that check fails."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "file.txt"), "w") as f:
                f.write("wrong content")

            task = _make_task(expected_files={"file.txt": "expected content"})
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert dims[0].score == 0.0

    @pytest.mark.asyncio
    async def test_expected_missing_files(self) -> None:
        """Files that should NOT exist are checked."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            # File that should be missing is actually missing -> pass
            task = _make_task(
                expected_files={},
                expected_missing=["should_not_exist.txt"],
            )
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert dims[0].score == 1.0

    @pytest.mark.asyncio
    async def test_expected_missing_but_exists_reduces_score(self) -> None:
        """File that should NOT exist but does -> score reduced."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "should_not_exist.txt"), "w") as f:
                f.write("oops")

            task = _make_task(
                expected_files={},
                expected_missing=["should_not_exist.txt"],
            )
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert dims[0].score == 0.0

    @pytest.mark.asyncio
    async def test_combined_expected_and_missing(self) -> None:
        """Both expected_files and expected_missing contribute to score."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "keep.txt"), "w") as f:
                f.write("content")
            # 'remove.txt' should be absent and IS absent

            task = _make_task(
                expected_files={"keep.txt": "content"},
                expected_missing=["remove.txt"],
            )
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            # 2 checks, both pass -> 1.0
            assert dims[0].score == 1.0

    @pytest.mark.asyncio
    async def test_empty_expected_content_checks_existence_only(self) -> None:
        """Expected file with empty string content -> only check existence."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "file.bin"), "w") as f:
                f.write("anything")

            task = _make_task(expected_files={"file.bin": ""})
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert dims[0].score == 1.0

    @pytest.mark.asyncio
    async def test_no_expectations_score_1(self) -> None:
        """Task with no expected_files and no expected_missing -> score 1.0."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            task = _make_task(expected_files=None, expected_missing=None)
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert dims[0].score == 1.0

    @pytest.mark.asyncio
    async def test_missing_working_dir_returns_error(self) -> None:
        """If no working_dir in result metadata, evaluator returns 0 with error."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        task = _make_task(expected_files={"file.txt": "content"})
        result = _make_result(working_dir="")
        evaluator = FilesystemStateEvaluator()
        dims = await evaluator.evaluate(task, result)

        assert dims[0].score == 0.0
        assert "error" in dims[0].details

    @pytest.mark.asyncio
    async def test_nonexistent_working_dir_returns_error(self) -> None:
        """If working_dir does not exist on disk, evaluator returns 0 with error."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        task = _make_task(expected_files={"file.txt": "content"})
        result = _make_result(working_dir="/nonexistent/path/abc123")
        evaluator = FilesystemStateEvaluator()
        dims = await evaluator.evaluate(task, result)

        assert dims[0].score == 0.0
        assert "error" in dims[0].details

    @pytest.mark.asyncio
    async def test_details_contain_check_counts(self) -> None:
        """Details should include passed/total check counts."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "a.txt"), "w") as f:
                f.write("ok")

            task = _make_task(
                expected_files={"a.txt": "ok", "b.txt": "missing"},
                expected_missing=["c.txt"],
            )
            result = _make_result(working_dir=tmpdir)
            evaluator = FilesystemStateEvaluator()
            dims = await evaluator.evaluate(task, result)

            assert "passed" in dims[0].details
            assert "total" in dims[0].details
            assert dims[0].details["passed"] == "2"  # a.txt match + c.txt absent
            assert dims[0].details["total"] == "3"

    @pytest.mark.asyncio
    async def test_working_dir_via_constructor(self) -> None:
        """Working dir can be provided via constructor instead of result metadata."""
        from codeforge.evaluation.evaluators.filesystem_state import (
            FilesystemStateEvaluator,
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "file.txt"), "w") as f:
                f.write("data")

            task = _make_task(expected_files={"file.txt": "data"})
            result = _make_result(working_dir="")  # no working_dir in result
            evaluator = FilesystemStateEvaluator(working_dir=tmpdir)
            dims = await evaluator.evaluate(task, result)

            assert dims[0].score == 1.0
