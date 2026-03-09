"""Tests for universal task filter function."""

from codeforge.evaluation.providers.base import TaskSpec
from codeforge.evaluation.task_filter import apply_task_filters


def _make_tasks(n: int, difficulties: list[str] | None = None) -> list[TaskSpec]:
    diffs = difficulties or ["easy", "medium", "hard"]
    return [
        TaskSpec(
            id=f"t-{i}",
            name=f"task-{i}",
            input=f"input-{i}",
            difficulty=diffs[i % len(diffs)],
        )
        for i in range(n)
    ]


def test_no_filters_returns_all():
    tasks = _make_tasks(10)
    result = apply_task_filters(tasks, {})
    assert len(result) == 10


def test_difficulty_filter():
    tasks = _make_tasks(9)
    result = apply_task_filters(tasks, {"difficulty_filter": ["easy"], "shuffle": False})
    assert all(t.difficulty == "easy" for t in result)
    assert len(result) == 3


def test_max_tasks():
    tasks = _make_tasks(20)
    result = apply_task_filters(tasks, {"max_tasks": 5, "shuffle": False})
    assert len(result) == 5


def test_task_percentage():
    tasks = _make_tasks(100)
    result = apply_task_filters(tasks, {"task_percentage": 10, "shuffle": False})
    assert len(result) == 10


def test_max_tasks_and_percentage_more_restrictive_wins():
    tasks = _make_tasks(100)
    # max_tasks=20 is more restrictive than 50%=50
    result = apply_task_filters(tasks, {"max_tasks": 20, "task_percentage": 50, "shuffle": False})
    assert len(result) == 20
    # task_percentage=10%=10 is more restrictive than max_tasks=50
    result2 = apply_task_filters(tasks, {"max_tasks": 50, "task_percentage": 10, "shuffle": False})
    assert len(result2) == 10


def test_shuffle_with_seed_is_deterministic():
    tasks = _make_tasks(20)
    r1 = apply_task_filters(tasks, {"shuffle": True, "seed": 42})
    r2 = apply_task_filters(tasks, {"shuffle": True, "seed": 42})
    assert [t.id for t in r1] == [t.id for t in r2]


def test_shuffle_with_different_seeds_differs():
    tasks = _make_tasks(20)
    r1 = apply_task_filters(tasks, {"shuffle": True, "seed": 1})
    r2 = apply_task_filters(tasks, {"shuffle": True, "seed": 2})
    assert [t.id for t in r1] != [t.id for t in r2]


def test_empty_tasks_returns_empty():
    result = apply_task_filters([], {"max_tasks": 5})
    assert result == []


def test_percentage_at_least_one():
    tasks = _make_tasks(3)
    result = apply_task_filters(tasks, {"task_percentage": 1, "shuffle": False})
    assert len(result) >= 1
