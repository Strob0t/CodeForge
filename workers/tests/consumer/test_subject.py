"""Tests for _subjects.py NATS subject constants (FIX-033).

Verifies:
- All subject constants are non-empty strings
- consumer_name() generates valid durable names
- No duplicate subject values
"""

from __future__ import annotations

from codeforge.consumer import _subjects


class TestSubjectConstants:
    """All SUBJECT_* constants must be non-empty strings."""

    def test_all_subjects_are_nonempty_strings(self) -> None:
        """Every SUBJECT_* constant must be a non-empty string."""
        for name in dir(_subjects):
            if not name.startswith("SUBJECT_"):
                continue
            value = getattr(_subjects, name)
            assert isinstance(value, str), f"{name} must be a string, got {type(value)}"
            assert len(value) > 0, f"{name} must not be empty"

    def test_no_duplicate_subject_values(self) -> None:
        """No two SUBJECT_* constants should have the same value."""
        seen: dict[str, str] = {}
        for name in dir(_subjects):
            if not name.startswith("SUBJECT_"):
                continue
            value = getattr(_subjects, name)
            if value in seen:
                msg = f"Duplicate subject value {value!r}: {seen[value]} and {name}"
                raise AssertionError(msg)
            seen[value] = name

    def test_stream_name_is_codeforge(self) -> None:
        assert _subjects.STREAM_NAME == "CODEFORGE"

    def test_stream_subjects_not_empty(self) -> None:
        assert len(_subjects.STREAM_SUBJECTS) > 0
        for subj in _subjects.STREAM_SUBJECTS:
            assert isinstance(subj, str)
            assert len(subj) > 0


class TestConsumerName:
    """consumer_name() generates valid durable consumer names."""

    def test_dots_replaced_with_dashes(self) -> None:
        result = _subjects.consumer_name("conversation.run.start")
        assert "." not in result
        assert "-" in result

    def test_wildcards_replaced(self) -> None:
        result = _subjects.consumer_name("tasks.agent.*")
        assert "*" not in result
        assert "all" in result

    def test_prefix(self) -> None:
        result = _subjects.consumer_name("runs.start")
        assert result.startswith("codeforge-py-")

    def test_chevron_replaced(self) -> None:
        result = _subjects.consumer_name("benchmark.>")
        assert ">" not in result
