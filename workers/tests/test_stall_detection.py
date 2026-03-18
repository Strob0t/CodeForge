"""Tests for the StallDetector in the agent loop (Measure A1)."""

from __future__ import annotations

import hashlib
import json

from codeforge.agent_loop import StallDetector

ESCAPE_PROMPT = (
    "<SYSTEM: You are repeating the same action without progress. "
    "Stop and try a fundamentally different approach. If you were reading, "
    "start writing. If you were searching, use what you found.>"
)


def _args_hash(args: dict) -> str:
    """Compute the same hash the StallDetector uses internally."""
    raw = json.dumps(args, sort_keys=True)[:200]
    return hashlib.sha256(raw.encode()).hexdigest()


# ---- Test 1: Empty history returns False ----


class TestDetectStallEmpty:
    def test_empty_history_not_stalled(self) -> None:
        detector = StallDetector()
        assert detector.is_stalled() is False

    def test_empty_history_no_repeated_action(self) -> None:
        detector = StallDetector()
        assert detector.get_repeated_action() is None


# ---- Test 2: Single entry returns False ----


class TestDetectStallSingleEntry:
    def test_single_entry_not_stalled(self) -> None:
        detector = StallDetector()
        detector.record("Read", {"file": "main.py"})
        assert detector.is_stalled() is False


# ---- Test 3: 3 identical consecutive tool calls -> is_stalled() True ----


class TestThreeIdenticalCalls:
    def test_three_identical_calls_stalled(self) -> None:
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True

    def test_three_identical_calls_repeated_action(self) -> None:
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.get_repeated_action() == "Read"


# ---- Test 4: 5 different tool calls -> is_stalled() False ----


class TestFiveDifferentCalls:
    def test_five_different_calls_not_stalled(self) -> None:
        detector = StallDetector()
        detector.record("Read", {"file": "a.py"})
        detector.record("Write", {"file": "b.py"})
        detector.record("Edit", {"file": "c.py"})
        detector.record("Search", {"query": "hello"})
        detector.record("Glob", {"pattern": "*.py"})
        assert detector.is_stalled() is False


# ---- Test 5: Same tool with different args (different hash) -> not stalled ----


class TestSameToolDifferentArgs:
    def test_same_tool_different_args_not_stalled(self) -> None:
        detector = StallDetector()
        detector.record("Read", {"file": "a.py"})
        detector.record("Read", {"file": "b.py"})
        detector.record("Read", {"file": "c.py"})
        detector.record("Read", {"file": "d.py"})
        detector.record("Read", {"file": "e.py"})
        assert detector.is_stalled() is False


# ---- Test 6: Same tool with same args hash -> is_stalled() True ----


class TestSameToolSameArgs:
    def test_same_tool_same_args_stalled(self) -> None:
        detector = StallDetector()
        args = {"query": "find the bug"}
        detector.record("Search", args)
        detector.record("Search", args)
        detector.record("Search", args)
        assert detector.is_stalled() is True

    def test_three_of_five_identical_stalled(self) -> None:
        """3 of last 5 entries identical should trigger stall."""
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Write", {"file": "other.py"})
        detector.record("Read", args)
        detector.record("Edit", {"file": "x.py"})
        detector.record("Read", args)
        assert detector.is_stalled() is True


# ---- Test 7: After stall detected, escape prompt should be injectable ----


class TestEscapePromptInjection:
    def test_escape_prompt_content(self) -> None:
        """Verify the escape prompt constant matches the expected text."""
        from codeforge.agent_loop import STALL_ESCAPE_PROMPT

        assert "repeating the same action" in STALL_ESCAPE_PROMPT
        assert "fundamentally different approach" in STALL_ESCAPE_PROMPT

    def test_record_escape_increments(self) -> None:
        detector = StallDetector()
        assert detector.should_abort() is False
        detector.record_escape()
        assert detector.should_abort() is False  # only 1 escape so far

    def test_escape_resets_not_stalled(self) -> None:
        """After escape + new different calls, stall should clear."""
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True
        detector.record_escape()
        # Now record different calls to move the window
        detector.record("Write", {"file": "output.py"})
        detector.record("Edit", {"file": "other.py"})
        detector.record("Search", {"query": "test"})
        assert detector.is_stalled() is False


# ---- Test 8: Double stall -> should_abort() True ----


class TestDoubleStallAbort:
    def test_double_stall_aborts(self) -> None:
        detector = StallDetector()
        # First stall
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True
        detector.record_escape()
        assert detector.should_abort() is False

        # Second stall
        detector.record_escape()
        assert detector.should_abort() is True

    def test_single_escape_no_abort(self) -> None:
        detector = StallDetector()
        detector.record_escape()
        assert detector.should_abort() is False


# ---- Test 9: After should_abort(), error payload contains details ----


class TestAbortErrorPayload:
    def test_abort_error_payload(self) -> None:
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record_escape()
        detector.record_escape()
        assert detector.should_abort() is True

        payload = detector.get_abort_info()
        assert payload["repeated_action"] == "Read"
        assert payload["escape_count"] == 2

    def test_abort_info_when_not_stalled(self) -> None:
        detector = StallDetector()
        payload = detector.get_abort_info()
        assert payload["repeated_action"] is None
        assert payload["escape_count"] == 0


# ---- Edge cases ----


class TestStallDetectorEdgeCases:
    def test_window_slides_correctly(self) -> None:
        """Old entries should fall out of the sliding window."""
        detector = StallDetector(window_size=5, stall_threshold=3)
        args = {"file": "main.py"}
        # Fill window with 3 identical
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True
        # Push 3 different to shift window
        detector.record("Write", {"file": "a.py"})
        detector.record("Edit", {"file": "b.py"})
        detector.record("Search", {"query": "c"})
        # Now window has [Read, Read, Write, Edit, Search] -> only 2 Read
        # Wait - deque maxlen=5, so after 6 entries: [Read, Read, Write, Edit, Search]
        # Actually: we recorded 6 items with maxlen=5, so window is
        # [Read, Read, Write, Edit, Search] which has 2 Reads
        # But actually: the deque drops the oldest, so after:
        # record 1: [Read]
        # record 2: [Read, Read]
        # record 3: [Read, Read, Read]
        # record 4: [Read, Read, Read, Write]
        # record 5: [Read, Read, Read, Write, Edit]
        # record 6: [Read, Read, Write, Edit, Search]  (first Read dropped)
        # 2 Reads in window of 5 -> not stalled
        assert detector.is_stalled() is False

    def test_empty_args(self) -> None:
        """Empty args dict should be handled."""
        detector = StallDetector()
        detector.record("Read", {})
        detector.record("Read", {})
        detector.record("Read", {})
        assert detector.is_stalled() is True

    def test_custom_thresholds(self) -> None:
        """Custom window_size and stall_threshold work."""
        detector = StallDetector(window_size=3, stall_threshold=2)
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True

    def test_exactly_at_threshold(self) -> None:
        """Exactly threshold matches should trigger stall."""
        detector = StallDetector(window_size=5, stall_threshold=3)
        args = {"file": "x.py"}
        detector.record("Read", args)
        detector.record("Write", {"a": 1})
        detector.record("Read", args)
        detector.record("Write", {"b": 2})
        detector.record("Read", args)
        # 3 of 5 are identical Read(x.py) -> stalled
        assert detector.is_stalled() is True

    def test_below_threshold(self) -> None:
        """Just below threshold should not trigger."""
        detector = StallDetector(window_size=5, stall_threshold=3)
        args = {"file": "x.py"}
        detector.record("Read", args)
        detector.record("Write", {"a": 1})
        detector.record("Read", args)
        detector.record("Write", {"b": 2})
        detector.record("Edit", {"c": 3})
        # 2 of 5 are identical Read(x.py) -> not stalled
        assert detector.is_stalled() is False

    def test_args_hash_truncation(self) -> None:
        """Args longer than 200 chars should be truncated before hashing."""
        detector = StallDetector()
        long_val = "x" * 500
        args = {"content": long_val}
        detector.record("Write", args)
        detector.record("Write", args)
        detector.record("Write", args)
        assert detector.is_stalled() is True
