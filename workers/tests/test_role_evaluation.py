"""Role evaluation tests — deterministic testing of agent roles via FakeLLM."""

from __future__ import annotations

from codeforge.executor import AgentExecutor
from codeforge.models import ModeConfig, TaskMessage, TaskStatus
from tests.conftest import load_scenario
from tests.evaluation import EvaluationMetrics
from tests.fake_llm import FakeLLM
from tests.role_matrix import ROLE_MATRIX

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_task(input_data: dict[str, str]) -> TaskMessage:
    """Build a TaskMessage from fixture input data."""
    return TaskMessage(
        id="eval-001",
        project_id=input_data.get("project_id", "proj-test"),
        title=input_data.get("title", "evaluation task"),
        prompt=input_data["prompt"],
    )


def _check_output(output: str, expected: dict[str, object]) -> EvaluationMetrics:
    """Validate output against expected_output.json assertions."""
    passed = True

    for keyword in expected.get("contains", []):
        if keyword.lower() not in output.lower():
            passed = False

    for keyword in expected.get("not_contains", []):
        if keyword.lower() in output.lower():
            passed = False

    min_length = expected.get("min_length", 0)
    if isinstance(min_length, int) and len(output) < min_length:
        passed = False

    return EvaluationMetrics(passed=passed)


# ---------------------------------------------------------------------------
# 1. Architect: generates a structured plan
# ---------------------------------------------------------------------------


async def test_architect_generates_plan() -> None:
    """Architect role should produce output with Architecture/Components sections."""
    input_data, expected, fake_llm = load_scenario("architect", "generate_plan")
    executor = AgentExecutor(llm=fake_llm)  # type: ignore[arg-type]
    result = await executor.execute(_make_task(input_data))

    assert result.status == TaskStatus.COMPLETED
    metrics = _check_output(result.output, expected)
    assert metrics.passed, f"Architect output missing expected content: {result.output[:200]}"

    # Verify the role spec exists
    assert "architect" in ROLE_MATRIX
    assert ROLE_MATRIX["architect"].output_artifact == "PLAN.md"


# ---------------------------------------------------------------------------
# 2. Coder: produces a diff
# ---------------------------------------------------------------------------


async def test_coder_produces_diff() -> None:
    """Coder role should produce output containing diff markers and code."""
    input_data, expected, fake_llm = load_scenario("coder", "produce_diff")
    executor = AgentExecutor(llm=fake_llm)  # type: ignore[arg-type]
    result = await executor.execute(_make_task(input_data))

    assert result.status == TaskStatus.COMPLETED
    metrics = _check_output(result.output, expected)
    assert metrics.passed, f"Coder output missing expected content: {result.output[:200]}"

    assert "coder" in ROLE_MATRIX
    assert ROLE_MATRIX["coder"].output_artifact == "DIFF"


# ---------------------------------------------------------------------------
# 3. Reviewer: catches a bug
# ---------------------------------------------------------------------------


async def test_reviewer_catches_bug() -> None:
    """Reviewer role should flag the division-by-zero bug."""
    input_data, expected, fake_llm = load_scenario("reviewer", "catch_bug")
    executor = AgentExecutor(llm=fake_llm)  # type: ignore[arg-type]
    result = await executor.execute(_make_task(input_data))

    assert result.status == TaskStatus.COMPLETED
    metrics = _check_output(result.output, expected)
    assert metrics.passed, f"Reviewer output missing expected content: {result.output[:200]}"

    assert "reviewer" in ROLE_MATRIX
    assert ROLE_MATRIX["reviewer"].output_artifact == "REVIEW.md"


# ---------------------------------------------------------------------------
# 4. Tester: reports pass/fail results
# ---------------------------------------------------------------------------


async def test_tester_reports_results() -> None:
    """Tester role should produce a TEST_REPORT with pass/fail status."""
    input_data, expected, fake_llm = load_scenario("tester", "report_pass_fail")
    executor = AgentExecutor(llm=fake_llm)  # type: ignore[arg-type]
    result = await executor.execute(_make_task(input_data))

    assert result.status == TaskStatus.COMPLETED
    metrics = _check_output(result.output, expected)
    assert metrics.passed, f"Tester output missing expected content: {result.output[:200]}"

    assert "tester" in ROLE_MATRIX
    assert ROLE_MATRIX["tester"].output_artifact == "TEST_REPORT"


# ---------------------------------------------------------------------------
# 5. Security: flags a vulnerability
# ---------------------------------------------------------------------------


async def test_security_flags_risk() -> None:
    """Security role should flag the SQL injection vulnerability."""
    input_data, expected, fake_llm = load_scenario("security", "flag_risk")
    executor = AgentExecutor(llm=fake_llm)  # type: ignore[arg-type]
    result = await executor.execute(_make_task(input_data))

    assert result.status == TaskStatus.COMPLETED
    metrics = _check_output(result.output, expected)
    assert metrics.passed, f"Security output missing expected content: {result.output[:200]}"

    assert "security" in ROLE_MATRIX
    assert ROLE_MATRIX["security"].output_artifact == "AUDIT_REPORT"


# ---------------------------------------------------------------------------
# 6. Debate: proponent/opponent convergence (simplified)
# ---------------------------------------------------------------------------


async def test_debate_reaches_convergence() -> None:
    """Debate simulation: proponent argues, opponent rebuts, moderator concludes."""
    from codeforge.llm import CompletionResponse

    responses = [
        # Proponent argument
        CompletionResponse(
            content="I propose using JWT for authentication because it is stateless, "
            "scalable, and widely supported. The token contains all necessary claims.",
            tokens_in=30,
            tokens_out=40,
            model="fake-model",
        ),
        # Opponent rebuttal
        CompletionResponse(
            content="While JWT has advantages, I agree that JWT with short-lived tokens "
            "and refresh rotation is acceptable. The stateless benefit outweighs "
            "the token size concern for this use case.",
            tokens_in=50,
            tokens_out=45,
            model="fake-model",
        ),
    ]
    fake_llm = FakeLLM(responses)
    executor = AgentExecutor(llm=fake_llm)  # type: ignore[arg-type]

    # Round 1: proponent
    proponent_task = TaskMessage(
        id="debate-prop",
        project_id="proj-debate",
        title="Propose auth approach",
        prompt="Argue for using JWT for authentication.",
    )
    prop_result = await executor.execute(proponent_task)
    assert prop_result.status == TaskStatus.COMPLETED
    assert "jwt" in prop_result.output.lower()

    # Round 2: opponent (with proponent's argument as context)
    opponent_task = TaskMessage(
        id="debate-opp",
        project_id="proj-debate",
        title="Counter auth proposal",
        prompt=f"Evaluate this proposal and respond:\n{prop_result.output}",
    )
    opp_result = await executor.execute(opponent_task)
    assert opp_result.status == TaskStatus.COMPLETED

    # Convergence check: opponent agrees (contains "agree" or "acceptable")
    output_lower = opp_result.output.lower()
    assert "agree" in output_lower or "acceptable" in output_lower

    # Verify both calls were recorded
    assert len(fake_llm.calls) == 2


# ---------------------------------------------------------------------------
# 7. Orchestrator strategy boundary (Go-side, documented here)
# ---------------------------------------------------------------------------


def test_orchestrator_strategy_boundary() -> None:
    """Validate that ModeConfig can represent orchestrator settings.

    Orchestrator strategy selection (sequential, parallel, consensus, ping_pong)
    happens in Go (internal/service/orchestrator.go). This test documents the
    boundary and verifies that ModeConfig fields used by the Go presets are
    correctly modeled in Python.
    """
    # Verify ModeConfig has all fields used by Go orchestrator modes
    mode = ModeConfig(
        id="architect",
        prompt_prefix="You are an architect.",
        tools=["read", "list", "search", "graph"],
        denied_tools=["write", "shell", "browser"],
        denied_actions=[],
        required_artifact="PLAN.md",
        llm_scenario="think",
    )
    assert mode.id == "architect"
    assert mode.llm_scenario == "think"
    assert mode.required_artifact == "PLAN.md"
    assert "write" in mode.denied_tools
    assert "read" in mode.tools

    # Verify orchestrator role is marked as Go-side
    assert "orchestrator" in ROLE_MATRIX
    assert ROLE_MATRIX["orchestrator"].test_location == "go"
