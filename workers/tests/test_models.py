"""Tests for domain models serialization."""

from codeforge.models import TaskMessage, TaskResult, TaskStatus


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
