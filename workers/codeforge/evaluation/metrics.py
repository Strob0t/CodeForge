"""DeepEval metric wrappers for agent evaluation.

Each function accepts task-level inputs, builds the appropriate DeepEval
test case and metric, then returns a score in [0, 1].
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from deepeval.metrics import AnswerRelevancyMetric, FaithfulnessMetric, GEval
from deepeval.test_case import LLMTestCase, LLMTestCaseParams, ToolCall

if TYPE_CHECKING:
    from codeforge.evaluation.litellm_judge import LiteLLMJudge


def _build_judge(judge: LiteLLMJudge | None) -> LiteLLMJudge | None:
    """Return the provided judge or lazily create a default one."""
    if judge is not None:
        return judge
    from codeforge.evaluation.litellm_judge import LiteLLMJudge as _DefaultJudge

    return _DefaultJudge()


async def evaluate_correctness(
    user_input: str,
    actual_output: str,
    expected_output: str,
    judge: LiteLLMJudge | None = None,
) -> float:
    """Evaluate task completion correctness using G-Eval."""
    model = _build_judge(judge)
    metric = GEval(
        name="Correctness",
        criteria="Determine whether the actual output is factually correct and matches the expected output.",
        evaluation_params=[
            LLMTestCaseParams.ACTUAL_OUTPUT,
            LLMTestCaseParams.EXPECTED_OUTPUT,
        ],
        model=model,
        threshold=0.5,
    )
    test_case = LLMTestCase(
        input=user_input,
        actual_output=actual_output,
        expected_output=expected_output,
    )
    await metric.a_measure(test_case)
    return metric.score


async def evaluate_tool_correctness(
    user_input: str,
    actual_output: str,
    expected_tools: list[dict[str, str]],
    actual_tools: list[dict[str, str]],
    judge: LiteLLMJudge | None = None,
) -> float:
    """Evaluate whether the agent used the correct tools in the right order."""
    model = _build_judge(judge)
    metric = GEval(
        name="Tool Correctness",
        criteria=(
            "Evaluate whether the actual tool calls match the expected tool calls "
            "in terms of tool names, arguments, and ordering."
        ),
        evaluation_params=[LLMTestCaseParams.ACTUAL_OUTPUT],
        model=model,
        threshold=0.5,
    )
    # Encode tool call sequences as structured text in the output
    expected_tc = [
        ToolCall(name=t.get("name", ""), input_parameters={"args": t.get("args", "")}) for t in expected_tools
    ]
    actual_tc = [ToolCall(name=t.get("name", ""), input_parameters={"args": t.get("args", "")}) for t in actual_tools]
    test_case = LLMTestCase(
        input=user_input,
        actual_output=actual_output,
        expected_tools=expected_tc,
        tools_called=actual_tc,
    )
    await metric.a_measure(test_case)
    return metric.score


async def evaluate_faithfulness(
    user_input: str,
    actual_output: str,
    retrieval_context: list[str],
    judge: LiteLLMJudge | None = None,
) -> float:
    """Evaluate faithfulness of the output to the retrieval context."""
    model = _build_judge(judge)
    metric = FaithfulnessMetric(model=model, threshold=0.5)
    test_case = LLMTestCase(
        input=user_input,
        actual_output=actual_output,
        retrieval_context=retrieval_context,
    )
    await metric.a_measure(test_case)
    return metric.score


async def evaluate_answer_relevancy(
    user_input: str,
    actual_output: str,
    judge: LiteLLMJudge | None = None,
) -> float:
    """Evaluate how relevant the answer is to the input question."""
    model = _build_judge(judge)
    metric = AnswerRelevancyMetric(model=model, threshold=0.5)
    test_case = LLMTestCase(
        input=user_input,
        actual_output=actual_output,
    )
    await metric.a_measure(test_case)
    return metric.score
