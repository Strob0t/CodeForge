"""Benchmark test commands leave no process behind and keep a bounded output (KI-194, R9-8).

On a timeout the functional-test evaluator and the agent runner stopped
waiting but never killed the command: a solution with an endless loop left
one running tool process per task (the agent runner deleted its workspace
underneath it), and the whole output was read into memory.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.constants import MAX_OUTPUT_CHARS
from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec
from codeforge.evaluation.runners.agent import _run_test_command
from tests.processes import SPAWN, gone, spawned

if TYPE_CHECKING:
    from pathlib import Path

# Two million bytes of output, more than MAX_OUTPUT_CHARS, ending in a marker.
_FLOOD = "head -c 2000000 /dev/zero | tr '\\0' x; echo; echo LAST-LINE"


async def test_a_timed_out_functional_test_leaves_no_process(tmp_path: Path) -> None:
    task = TaskSpec(id="t1", name="t", input="", test_command=SPAWN)

    dims = await FunctionalTestEvaluator(timeout=1, working_dir=str(tmp_path)).evaluate(task, ExecutionResult())

    assert dims[0].score == 0.0
    assert "timeout" in dims[0].details["error"]
    shell, child = await spawned(tmp_path)
    assert await gone(shell)
    assert await gone(child), "a process the test command started survived the timeout"


async def test_the_functional_test_output_is_capped(tmp_path: Path) -> None:
    _score, output, exit_code = await FunctionalTestEvaluator(working_dir=str(tmp_path))._run_command(_FLOOD)

    assert exit_code == 0
    assert len(output) <= MAX_OUTPUT_CHARS + 200
    assert output.rstrip().endswith("LAST-LINE"), "the end of a test run's output (its summary) is kept"
    assert "truncated" in output


async def test_a_timed_out_benchmark_test_command_leaves_no_process(tmp_path: Path) -> None:
    output, exit_code = await _run_test_command(SPAWN, tmp_path, timeout=1)

    assert exit_code == 124
    assert "timed out" in output
    shell, child = await spawned(tmp_path)
    assert await gone(shell)
    assert await gone(child), "a process the test command started survived the timeout"


async def test_the_benchmark_test_output_is_capped(tmp_path: Path) -> None:
    output, exit_code = await _run_test_command(_FLOOD, tmp_path)

    assert exit_code == 0
    assert len(output) <= MAX_OUTPUT_CHARS + 200
    assert output.rstrip().endswith("LAST-LINE")


async def test_a_short_output_is_kept_whole(tmp_path: Path) -> None:
    output, exit_code = await _run_test_command("echo one; echo two >&2; exit 3", tmp_path)

    assert exit_code == 3
    assert output == "one\ntwo\n"
