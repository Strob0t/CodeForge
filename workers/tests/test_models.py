"""Tests for domain models serialization."""

from codeforge.models import ModeConfig, RunStartMessage, TaskMessage, TaskResult, TaskStatus


def test_task_message_from_json() -> None:
    """TaskMessage should deserialize from JSON correctly."""
    raw = '{"id": "abc-123", "project_id": "proj-1", "title": "Fix bug", "prompt": "Fix the login bug"}'
    msg = TaskMessage.model_validate_json(raw)
    assert msg.id == "abc-123"
    assert msg.project_id == "proj-1"
    assert msg.title == "Fix bug"
    assert msg.prompt == "Fix the login bug"
    assert msg.config == {}


def test_task_message_with_config() -> None:
    """TaskMessage should handle optional config field."""
    raw = '{"id": "1", "project_id": "2", "title": "t", "prompt": "p", "config": {"model": "gpt-4o"}}'
    msg = TaskMessage.model_validate_json(raw)
    assert msg.config == {"model": "gpt-4o"}


def test_task_result_to_json() -> None:
    """TaskResult should serialize to JSON correctly."""
    result = TaskResult(
        task_id="abc-123",
        status=TaskStatus.COMPLETED,
        output="Done",
        tokens_in=100,
        tokens_out=50,
        cost_usd=0.005,
    )
    data = result.model_dump()
    assert data["task_id"] == "abc-123"
    assert data["status"] == "completed"
    assert data["output"] == "Done"
    assert data["tokens_in"] == 100
    assert data["tokens_out"] == 50
    assert data["cost_usd"] == 0.005


def test_task_result_defaults() -> None:
    """TaskResult default values should be sensible."""
    result = TaskResult(task_id="1", status=TaskStatus.FAILED, error="timeout")
    assert result.output == ""
    assert result.files == []
    assert result.tokens_in == 0
    assert result.cost_usd == 0.0


def test_task_status_values() -> None:
    """TaskStatus enum should have all expected values."""
    assert TaskStatus.PENDING == "pending"
    assert TaskStatus.QUEUED == "queued"
    assert TaskStatus.RUNNING == "running"
    assert TaskStatus.COMPLETED == "completed"
    assert TaskStatus.FAILED == "failed"
    assert TaskStatus.CANCELLED == "cancelled"


# --- ModeConfig Tests ---


def test_mode_config_defaults() -> None:
    """ModeConfig should have sensible defaults."""
    mode = ModeConfig()
    assert mode.id == ""
    assert mode.prompt_prefix == ""
    assert mode.tools == []
    assert mode.denied_tools == []
    assert mode.denied_actions == []
    assert mode.required_artifact == ""
    assert mode.llm_scenario == ""


def test_mode_config_from_json() -> None:
    """ModeConfig should deserialize from JSON correctly."""
    raw = '{"id": "reviewer", "prompt_prefix": "You are a reviewer.", "tools": ["Read", "Grep"], "denied_tools": ["Write", "Edit"], "denied_actions": ["rm"], "required_artifact": "REVIEW.md", "llm_scenario": "review"}'
    mode = ModeConfig.model_validate_json(raw)
    assert mode.id == "reviewer"
    assert mode.prompt_prefix == "You are a reviewer."
    assert mode.tools == ["Read", "Grep"]
    assert mode.denied_tools == ["Write", "Edit"]
    assert mode.denied_actions == ["rm"]
    assert mode.required_artifact == "REVIEW.md"
    assert mode.llm_scenario == "review"


def test_mode_config_partial_json() -> None:
    """ModeConfig should handle partial JSON with defaults for missing fields."""
    raw = '{"id": "coder", "prompt_prefix": "You are a coder."}'
    mode = ModeConfig.model_validate_json(raw)
    assert mode.id == "coder"
    assert mode.prompt_prefix == "You are a coder."
    assert mode.tools == []
    assert mode.denied_tools == []
    assert mode.denied_actions == []
    assert mode.required_artifact == ""


def test_run_start_message_with_mode() -> None:
    """RunStartMessage should include mode configuration."""
    raw = '{"run_id": "r1", "task_id": "t1", "project_id": "p1", "agent_id": "a1", "prompt": "Fix bug", "mode": {"id": "reviewer", "prompt_prefix": "Review this.", "denied_tools": ["Write"]}}'
    msg = RunStartMessage.model_validate_json(raw)
    assert msg.mode.id == "reviewer"
    assert msg.mode.prompt_prefix == "Review this."
    assert msg.mode.denied_tools == ["Write"]


def test_run_start_message_default_mode() -> None:
    """RunStartMessage should have default empty ModeConfig when mode is not provided."""
    raw = '{"run_id": "r1", "task_id": "t1", "project_id": "p1", "agent_id": "a1", "prompt": "Fix bug"}'
    msg = RunStartMessage.model_validate_json(raw)
    assert msg.mode.id == ""
    assert msg.mode.prompt_prefix == ""
    assert msg.mode.tools == []
